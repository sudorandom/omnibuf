package omnibuf

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// ============================================================================
// Generic Table-Driven Marshal / Unmarshal for Go Structs & .pb.go Structs
// ============================================================================
//
// Works directly on:
//   1. Plain Go structs annotated with `omnibuf:"<fieldNum>[,name=<name>][,wire=<wireType>][,omitempty]"`
//   2. Existing `protoc-gen-go` generated structs (e.g. *pb.PeerUpdate) with `protobuf:"..."` tags
//
// Just like google.golang.org/protobuf's internal `protoimpl` table-driven engine,
// OmniBuf inspects the struct's reflection metadata ONCE per reflect.Type, compiles a
// flat array of `fieldCodec` descriptors with precomputed `unsafe.Pointer` byte offsets,
// and executes Marshal / Unmarshal using direct pointer arithmetic.

const maxDirectFieldSlot = 16

type fieldKind uint8

const (
	kindUint8 fieldKind = iota
	kindUint16
	kindUint32
	kindUint64
	kindUint8Sint
	kindUint16Sint
	kindUint32Sint
	kindUint64Sint
	kindInt8
	kindInt16
	kindInt32
	kindInt64
	kindSint32
	kindSint64
	kindInt8Uint
	kindInt16Uint
	kindInt32Uint
	kindInt64Uint
	kindBool
	kindFloat32
	kindFloat64
	kindFixed32_8
	kindFixed32_16
	kindFixed32_64
	kindSfixed32_8
	kindSfixed32_16
	kindSfixed32_64
	kindFixed64_8
	kindFixed64_16
	kindFixed64_32
	kindSfixed64_8
	kindSfixed64_16
	kindSfixed64_32
	kindString
	kindBytes
	kindStringSlice
	kindBytesSlice
	kindBoolSlice
	kindUint8Slice
	kindUint16Slice
	kindUint32Slice
	kindUint64Slice
	kindUint8SintSlice
	kindUint16SintSlice
	kindInt8Slice
	kindInt16Slice
	kindInt32Slice
	kindInt64Slice
	kindFloat32Slice
	kindFloat64Slice
	kindUint8Fixed32Slice
	kindUint16Fixed32Slice
	kindUint64Fixed32Slice
	kindInt8Fixed32Slice
	kindInt16Fixed32Slice
	kindInt64Fixed32Slice
	kindUint8Fixed64Slice
	kindUint16Fixed64Slice
	kindUint32Fixed64Slice
	kindInt8Fixed64Slice
	kindInt16Fixed64Slice
	kindInt32Fixed64Slice
	kindStructVal
	kindStructPtr
	kindScalarPtr
	kindStructValSlice
	kindStructPtrSlice
	kindMapStringUint
	kindMap
)

type fieldCodec struct {
	fieldNum  uint16
	wireType  uint8
	tag1B     byte   // Precomputed 1-byte Prefix-Varint tag if fieldNum <= 7
	tag2B     uint16 // Precomputed 2-byte Prefix-Varint tag if 8 <= fieldNum <= 1023
	tag4B     uint32 // Precomputed 4-byte Prefix-Varint tag if fieldNum >= 1024
	is1BTag   bool
	is4BTag   bool
	omitEmpty bool
	kind      fieldKind
	name      string
	offset    uintptr
	subCodec  *structCodec
	elemCodec *fieldCodec
	elemSize  uintptr
	sliceType reflect.Type
	elemType  reflect.Type
	mapKeyWT  uint8
	mapValWT  uint8
}

type structCodec struct {
	typ         reflect.Type
	size        uintptr
	fields      []fieldCodec
	byNum       [maxDirectFieldSlot + 1]*fieldCodec
	overflow    map[uint16]*fieldCodec
	schemaNames []string
	metaOnce    sync.Once
	schemaMeta  *SchemaMeta
	field0Wire  []byte
}

type emptyInterface struct {
	typ uintptr
	ptr unsafe.Pointer
}

var codecCache sync.Map // map[reflect.Type]*structCodec

func getStructCodec(t reflect.Type) (*structCodec, error) {
	if c, ok := codecCache.Load(t); ok {
		return c.(*structCodec), nil
	}
	building := make(map[reflect.Type]*structCodec)
	c, err := compileStructCodec(t, building)
	if err != nil {
		return nil, err
	}
	for bt, bc := range building {
		codecCache.LoadOrStore(bt, bc)
	}
	actual, _ := codecCache.LoadOrStore(t, c)
	return actual.(*structCodec), nil
}

func (sc *structCodec) getField0Wire() []byte {
	sc.metaOnce.Do(func() {
		sc.schemaMeta = buildStructSchemaMeta(sc, 0)
		sc.field0Wire = AppendField0SchemaMetaDeterministic(nil, sc.schemaMeta, true)
	})
	return sc.field0Wire
}

func buildStructSchemaMeta(sc *structCodec, depth int) *SchemaMeta {
	if sc == nil || depth > 8 {
		return nil
	}
	meta := &SchemaMeta{
		TypeName: sc.typ.Name(),
		Fields:   make(map[uint32]string, len(sc.fields)),
	}
	for i := range sc.fields {
		fc := &sc.fields[i]
		fnum := uint32(fc.fieldNum)
		meta.Fields[fnum] = fc.name
		if fc.subCodec != nil {
			if child := buildStructSchemaMeta(fc.subCodec, depth+1); child != nil {
				if meta.Children == nil {
					meta.Children = make(map[uint32]*SchemaMeta)
				}
				meta.Children[fnum] = child
			}
		}
	}
	return meta
}

func compileStructCodec(t reflect.Type, building map[reflect.Type]*structCodec) (*structCodec, error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("omnibuf: expected struct type, got %v", t)
	}
	if c, ok := codecCache.Load(t); ok {
		return c.(*structCodec), nil
	}
	if c, ok := building[t]; ok {
		return c, nil
	}
	sc := &structCodec{
		typ:  t,
		size: t.Size(),
	}
	building[t] = sc

	seenNums := make(map[uint16]string, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		var (
			fc  fieldCodec
			err error
		)
		if obTag, hasOB := sf.Tag.Lookup("omnibuf"); hasOB {
			if obTag == "" || obTag == "-" {
				continue
			}
			fc, err = parseOmniBufField(sf, obTag, building)
		} else {
			pbTag := sf.Tag.Get("protobuf")
			if pbTag == "" || pbTag == "-" {
				continue
			}
			fc, err = parseProtobufField(sf, pbTag, building)
		}
		if err != nil {
			return nil, err
		}
		if prevName, exists := seenNums[fc.fieldNum]; exists {
			return nil, fmt.Errorf("omnibuf: duplicate field number %d on %s (already used by %s)", fc.fieldNum, sf.Name, prevName)
		}
		seenNums[fc.fieldNum] = sf.Name
		sc.fields = append(sc.fields, fc)
	}

	sort.Slice(sc.fields, func(i, j int) bool {
		return sc.fields[i].fieldNum < sc.fields[j].fieldNum
	})

	sc.schemaNames = make([]string, len(sc.fields))
	for i := range sc.fields {
		fc := &sc.fields[i]
		sc.schemaNames[i] = fc.name
		if fc.fieldNum <= maxDirectFieldSlot {
			sc.byNum[fc.fieldNum] = fc
		} else {
			if sc.overflow == nil {
				sc.overflow = make(map[uint16]*fieldCodec)
			}
			sc.overflow[fc.fieldNum] = fc
		}
	}
	return sc, nil
}

// parseOmniBufField parses an `omnibuf:"<fieldNum>[,name=<name>][,wire=<wireType>][,omitempty]"` tag.
func parseOmniBufField(sf reflect.StructField, obTag string, building map[reflect.Type]*structCodec) (fieldCodec, error) {
	parts := strings.Split(obTag, ",")
	fnumStr := strings.TrimSpace(parts[0])
	fnum64, err := strconv.ParseUint(fnumStr, 10, 16)
	if err != nil || fnum64 == 0 {
		return fieldCodec{}, fmt.Errorf("omnibuf: invalid field number in tag %q on %s", obTag, sf.Name)
	}
	fnum := uint16(fnum64)
	name := sf.Name
	wireOpt := ""
	omitEmpty := false

	for _, rawOpt := range parts[1:] {
		opt := strings.TrimSpace(rawOpt)
		switch {
		case opt == "omitempty":
			omitEmpty = true
		case strings.HasPrefix(opt, "name="):
			val := strings.TrimPrefix(opt, "name=")
			if val == "" {
				return fieldCodec{}, fmt.Errorf("omnibuf: empty name option in tag %q on %s", obTag, sf.Name)
			}
			name = val
		case strings.HasPrefix(opt, "wire="):
			val := strings.TrimPrefix(opt, "wire=")
			if val == "" {
				return fieldCodec{}, fmt.Errorf("omnibuf: empty wire option in tag %q on %s", obTag, sf.Name)
			}
			wireOpt = val
		default:
			return fieldCodec{}, fmt.Errorf("omnibuf: invalid option %q in tag %q on %s", rawOpt, obTag, sf.Name)
		}
	}

	fc := fieldCodec{
		fieldNum:  fnum,
		name:      name,
		offset:    sf.Offset,
		omitEmpty: omitEmpty,
	}
	if err := configureFieldCodec(&fc, sf, sf.Type, wireOpt, building); err != nil {
		return fieldCodec{}, err
	}
	precomputeFieldTag(&fc)
	return fc, nil
}

// parseProtobufField parses a `protoc-gen-go` `protobuf:"..."` tag.
func parseProtobufField(sf reflect.StructField, pbTag string, building map[reflect.Type]*structCodec) (fieldCodec, error) {
	parts := strings.Split(pbTag, ",")
	if len(parts) < 2 {
		return fieldCodec{}, fmt.Errorf("omnibuf: invalid protobuf tag %q on %s", pbTag, sf.Name)
	}
	encType := parts[0]
	fnum64, err := strconv.ParseUint(parts[1], 10, 16)
	if err != nil || fnum64 == 0 {
		return fieldCodec{}, fmt.Errorf("omnibuf: invalid field number in tag %q on %s", pbTag, sf.Name)
	}
	fnum := uint16(fnum64)
	name := strings.ToLower(sf.Name)
	for _, p := range parts[2:] {
		if after, ok := strings.CutPrefix(p, "name="); ok {
			name = after
		}
	}

	wireOpt := ""
	switch encType {
	case "fixed32":
		wireOpt = "fixed32"
	case "fixed64":
		wireOpt = "fixed64"
	case "zigzag32", "zigzag64":
		wireOpt = "sint"
	}

	fc := fieldCodec{
		fieldNum: fnum,
		name:     name,
		offset:   sf.Offset,
	}
	if err := configureFieldCodec(&fc, sf, sf.Type, wireOpt, building); err != nil {
		return fieldCodec{}, err
	}
	if encType == "zigzag32" && fc.kind == kindInt32 {
		fc.kind = kindSint32
	} else if encType == "zigzag64" && fc.kind == kindInt64 {
		fc.kind = kindSint64
	}
	precomputeFieldTag(&fc)
	return fc, nil
}

func precomputeFieldTag(fc *fieldCodec) {
	rawTag := (uint32(fc.fieldNum) << 4) | uint32(fc.wireType)
	if rawTag <= 127 {
		fc.is1BTag = true
		fc.tag1B = byte(rawTag<<1) | 1
	} else if rawTag <= 16383 {
		fc.tag2B = uint16(rawTag<<2) | 2
	} else {
		fc.is4BTag = true
		fc.tag4B = (rawTag << 3) | 4
	}
}

func configureFieldCodec(fc *fieldCodec, sf reflect.StructField, ft reflect.Type, wireOpt string, building map[reflect.Type]*structCodec) error {
	switch ft.Kind() {
	case reflect.Uint8:
		switch wireOpt {
		case "", "uint":
			fc.kind = kindUint8
			fc.wireType = WireUint
		case "sint":
			fc.kind = kindUint8Sint
			fc.wireType = WireSint
		case "fixed32":
			fc.kind = kindFixed32_8
			fc.wireType = WireFixed32
		case "fixed64":
			fc.kind = kindFixed64_8
			fc.wireType = WireFixed64
		default:
			return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
		}
	case reflect.Uint16:
		switch wireOpt {
		case "", "uint":
			fc.kind = kindUint16
			fc.wireType = WireUint
		case "sint":
			fc.kind = kindUint16Sint
			fc.wireType = WireSint
		case "fixed32":
			fc.kind = kindFixed32_16
			fc.wireType = WireFixed32
		case "fixed64":
			fc.kind = kindFixed64_16
			fc.wireType = WireFixed64
		default:
			return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
		}
	case reflect.Uint32:
		switch wireOpt {
		case "", "uint":
			fc.kind = kindUint32
			fc.wireType = WireUint
		case "sint":
			fc.kind = kindUint32Sint
			fc.wireType = WireSint
		case "fixed32":
			fc.kind = kindFloat32
			fc.wireType = WireFixed32
		case "fixed64":
			fc.kind = kindFixed64_32
			fc.wireType = WireFixed64
		default:
			return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
		}
	case reflect.Uint64, reflect.Uint:
		switch wireOpt {
		case "", "uint":
			fc.kind = kindUint64
			fc.wireType = WireUint
		case "sint":
			fc.kind = kindUint64Sint
			fc.wireType = WireSint
		case "fixed32":
			fc.kind = kindFixed32_64
			fc.wireType = WireFixed32
		case "fixed64":
			fc.kind = kindFloat64
			fc.wireType = WireFixed64
		default:
			return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
		}
	case reflect.Int8:
		switch wireOpt {
		case "", "sint":
			fc.kind = kindInt8
			fc.wireType = WireSint
		case "uint":
			fc.kind = kindInt8Uint
			fc.wireType = WireUint
		case "fixed32":
			fc.kind = kindSfixed32_8
			fc.wireType = WireFixed32
		case "fixed64":
			fc.kind = kindSfixed64_8
			fc.wireType = WireFixed64
		default:
			return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
		}
	case reflect.Int16:
		switch wireOpt {
		case "", "sint":
			fc.kind = kindInt16
			fc.wireType = WireSint
		case "uint":
			fc.kind = kindInt16Uint
			fc.wireType = WireUint
		case "fixed32":
			fc.kind = kindSfixed32_16
			fc.wireType = WireFixed32
		case "fixed64":
			fc.kind = kindSfixed64_16
			fc.wireType = WireFixed64
		default:
			return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
		}
	case reflect.Int32:
		switch wireOpt {
		case "", "sint":
			fc.kind = kindInt32
			fc.wireType = WireSint
		case "uint":
			fc.kind = kindInt32Uint
			fc.wireType = WireUint
		case "fixed32":
			fc.kind = kindFloat32
			fc.wireType = WireFixed32
		case "fixed64":
			fc.kind = kindSfixed64_32
			fc.wireType = WireFixed64
		default:
			return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
		}
	case reflect.Int64, reflect.Int:
		switch wireOpt {
		case "", "sint":
			fc.kind = kindInt64
			fc.wireType = WireSint
		case "uint":
			fc.kind = kindInt64Uint
			fc.wireType = WireUint
		case "fixed32":
			fc.kind = kindSfixed32_64
			fc.wireType = WireFixed32
		case "fixed64":
			fc.kind = kindFloat64
			fc.wireType = WireFixed64
		default:
			return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
		}
	case reflect.Bool:
		if wireOpt != "" && wireOpt != "bool" {
			return fmt.Errorf("omnibuf: unsupported wire type %q for bool on %s", wireOpt, sf.Name)
		}
		fc.kind = kindBool
		fc.wireType = WireBoolTrue
	case reflect.Float32:
		if wireOpt != "" && wireOpt != "fixed32" {
			return fmt.Errorf("omnibuf: unsupported wire type %q for float32 on %s", wireOpt, sf.Name)
		}
		fc.kind = kindFloat32
		fc.wireType = WireFixed32
	case reflect.Float64:
		if wireOpt != "" && wireOpt != "fixed64" {
			return fmt.Errorf("omnibuf: unsupported wire type %q for float64 on %s", wireOpt, sf.Name)
		}
		fc.kind = kindFloat64
		fc.wireType = WireFixed64
	case reflect.String:
		switch wireOpt {
		case "", "string":
			fc.kind = kindString
			fc.wireType = WireString
		case "bytes":
			fc.kind = kindString
			fc.wireType = WireBytes
		default:
			return fmt.Errorf("omnibuf: unsupported wire type %q for string on %s", wireOpt, sf.Name)
		}
	case reflect.Struct:
		if wireOpt != "" && wireOpt != "submsg" {
			return fmt.Errorf("omnibuf: unsupported wire type %q for struct on %s", wireOpt, sf.Name)
		}
		sub, err := compileStructCodec(ft, building)
		if err != nil {
			return err
		}
		fc.kind = kindStructVal
		fc.wireType = WireSubMsg
		fc.subCodec = sub
	case reflect.Pointer:
		if ft.Elem().Kind() == reflect.Struct {
			if wireOpt != "" && wireOpt != "submsg" {
				return fmt.Errorf("omnibuf: unsupported wire type %q for struct pointer on %s", wireOpt, sf.Name)
			}
			sub, err := compileStructCodec(ft.Elem(), building)
			if err != nil {
				return err
			}
			fc.kind = kindStructPtr
			fc.wireType = WireSubMsg
			fc.subCodec = sub
			fc.elemType = ft.Elem()
			return nil
		}
		elemCodec := &fieldCodec{
			fieldNum: fc.fieldNum,
			name:     fc.name,
			offset:   0,
		}
		if err := configureFieldCodec(elemCodec, sf, ft.Elem(), wireOpt, building); err != nil {
			return fmt.Errorf("omnibuf: unsupported pointer field %s: %w", sf.Name, err)
		}
		precomputeFieldTag(elemCodec)
		fc.kind = kindScalarPtr
		fc.wireType = elemCodec.wireType
		fc.elemType = ft.Elem()
		fc.elemCodec = elemCodec
	case reflect.Slice:
		return configureSliceFieldCodec(fc, sf, ft, wireOpt, building)
	case reflect.Map:
		return configureMapFieldCodec(fc, sf, ft, wireOpt, building)
	default:
		return fmt.Errorf("omnibuf: unsupported field type %v on %s", ft, sf.Name)
	}
	return nil
}

func configureSliceFieldCodec(fc *fieldCodec, sf reflect.StructField, ft reflect.Type, wireOpt string, building map[reflect.Type]*structCodec) error {
	elem := ft.Elem()
	switch elem.Kind() {
	case reflect.Uint8:
		switch wireOpt {
		case "", "bytes":
			fc.kind = kindBytes
			fc.wireType = WireBytes
		case "string":
			fc.kind = kindBytes
			fc.wireType = WireString
		case "uint":
			fc.kind = kindUint8Slice
			fc.wireType = WireUintArr
		case "sint":
			fc.kind = kindUint8SintSlice
			fc.wireType = WireSintArr
		case "fixed32":
			fc.kind = kindUint8Fixed32Slice
			fc.wireType = WireFixedArr
		case "fixed64":
			fc.kind = kindUint8Fixed64Slice
			fc.wireType = WireFixedArr
		default:
			return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
		}
	case reflect.String:
		if wireOpt != "" && wireOpt != "string" && wireOpt != "bytes" {
			return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
		}
		fc.kind = kindStringSlice
		fc.wireType = WireStringArr
	case reflect.Slice:
		if elem.Elem().Kind() == reflect.Uint8 {
			if wireOpt != "" && wireOpt != "bytes" && wireOpt != "string" {
				return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
			}
			fc.kind = kindBytesSlice
			fc.wireType = WireStringArr
			return nil
		}
		return fmt.Errorf("omnibuf: unsupported slice type %v on %s", ft, sf.Name)
	case reflect.Bool:
		if wireOpt != "" && wireOpt != "bool" && wireOpt != "uint" {
			return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
		}
		fc.kind = kindBoolSlice
		fc.wireType = WireUintArr
	case reflect.Uint16:
		switch wireOpt {
		case "", "uint":
			fc.kind = kindUint16Slice
			fc.wireType = WireUintArr
		case "sint":
			fc.kind = kindUint16SintSlice
			fc.wireType = WireSintArr
		case "fixed32":
			fc.kind = kindUint16Fixed32Slice
			fc.wireType = WireFixedArr
		case "fixed64":
			fc.kind = kindUint16Fixed64Slice
			fc.wireType = WireFixedArr
		default:
			return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
		}
	case reflect.Uint32:
		switch wireOpt {
		case "", "uint":
			fc.kind = kindUint32Slice
			fc.wireType = WireUintArr
		case "sint":
			fc.kind = kindInt32Slice
			fc.wireType = WireSintArr
		case "fixed32":
			fc.kind = kindFloat32Slice
			fc.wireType = WireFixedArr
		case "fixed64":
			fc.kind = kindUint32Fixed64Slice
			fc.wireType = WireFixedArr
		default:
			return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
		}
	case reflect.Uint64, reflect.Uint:
		switch wireOpt {
		case "", "uint":
			fc.kind = kindUint64Slice
			fc.wireType = WireUintArr
		case "sint":
			fc.kind = kindInt64Slice
			fc.wireType = WireSintArr
		case "fixed32":
			fc.kind = kindUint64Fixed32Slice
			fc.wireType = WireFixedArr
		case "fixed64":
			fc.kind = kindFloat64Slice
			fc.wireType = WireFixedArr
		default:
			return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
		}
	case reflect.Int8:
		switch wireOpt {
		case "", "sint":
			fc.kind = kindInt8Slice
			fc.wireType = WireSintArr
		case "uint":
			fc.kind = kindUint8Slice
			fc.wireType = WireUintArr
		case "fixed32":
			fc.kind = kindInt8Fixed32Slice
			fc.wireType = WireFixedArr
		case "fixed64":
			fc.kind = kindInt8Fixed64Slice
			fc.wireType = WireFixedArr
		default:
			return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
		}
	case reflect.Int16:
		switch wireOpt {
		case "", "sint":
			fc.kind = kindInt16Slice
			fc.wireType = WireSintArr
		case "uint":
			fc.kind = kindUint16Slice
			fc.wireType = WireUintArr
		case "fixed32":
			fc.kind = kindInt16Fixed32Slice
			fc.wireType = WireFixedArr
		case "fixed64":
			fc.kind = kindInt16Fixed64Slice
			fc.wireType = WireFixedArr
		default:
			return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
		}
	case reflect.Int32:
		switch wireOpt {
		case "", "sint":
			fc.kind = kindInt32Slice
			fc.wireType = WireSintArr
		case "uint":
			fc.kind = kindUint32Slice
			fc.wireType = WireUintArr
		case "fixed32":
			fc.kind = kindFloat32Slice
			fc.wireType = WireFixedArr
		case "fixed64":
			fc.kind = kindInt32Fixed64Slice
			fc.wireType = WireFixedArr
		default:
			return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
		}
	case reflect.Int64, reflect.Int:
		switch wireOpt {
		case "", "sint":
			fc.kind = kindInt64Slice
			fc.wireType = WireSintArr
		case "uint":
			fc.kind = kindUint64Slice
			fc.wireType = WireUintArr
		case "fixed32":
			fc.kind = kindInt64Fixed32Slice
			fc.wireType = WireFixedArr
		case "fixed64":
			fc.kind = kindFloat64Slice
			fc.wireType = WireFixedArr
		default:
			return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
		}
	case reflect.Float32:
		if wireOpt != "" && wireOpt != "fixed32" {
			return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
		}
		fc.kind = kindFloat32Slice
		fc.wireType = WireFixedArr
	case reflect.Float64:
		if wireOpt != "" && wireOpt != "fixed64" {
			return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
		}
		fc.kind = kindFloat64Slice
		fc.wireType = WireFixedArr
	case reflect.Struct:
		if wireOpt != "" && wireOpt != "submsg" {
			return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
		}
		sub, err := compileStructCodec(elem, building)
		if err != nil {
			return err
		}
		fc.kind = kindStructValSlice
		fc.wireType = WireSubMsgArr
		fc.subCodec = sub
		fc.elemSize = elem.Size()
		fc.sliceType = ft
		fc.elemType = elem
	case reflect.Pointer:
		if elem.Elem().Kind() != reflect.Struct {
			return fmt.Errorf("omnibuf: unsupported slice element %v on %s", elem, sf.Name)
		}
		if wireOpt != "" && wireOpt != "submsg" {
			return fmt.Errorf("omnibuf: unsupported wire type %q for %v on %s", wireOpt, ft, sf.Name)
		}
		sub, err := compileStructCodec(elem.Elem(), building)
		if err != nil {
			return err
		}
		fc.kind = kindStructPtrSlice
		fc.wireType = WireSubMsgArr
		fc.subCodec = sub
		fc.elemSize = elem.Elem().Size()
		fc.sliceType = ft
		fc.elemType = elem.Elem()
	default:
		return fmt.Errorf("omnibuf: unsupported slice type %v on %s", ft, sf.Name)
	}
	return nil
}

func mapKeyWireType(kt reflect.Type, wireOpt string) (uint8, error) {
	switch kt.Kind() {
	case reflect.String:
		if wireOpt != "" && wireOpt != "string" {
			return 0, fmt.Errorf("omnibuf: unsupported map key wire type %q for %v", wireOpt, kt)
		}
		return WireString, nil
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint:
		switch wireOpt {
		case "", "uint":
			return WireUint, nil
		case "sint":
			return WireSint, nil
		case "fixed32":
			return WireFixed32, nil
		case "fixed64":
			return WireFixed64, nil
		default:
			return 0, fmt.Errorf("omnibuf: unsupported map key wire type %q for %v", wireOpt, kt)
		}
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
		switch wireOpt {
		case "", "sint":
			return WireSint, nil
		case "uint":
			return WireUint, nil
		case "fixed32":
			return WireFixed32, nil
		case "fixed64":
			return WireFixed64, nil
		default:
			return 0, fmt.Errorf("omnibuf: unsupported map key wire type %q for %v", wireOpt, kt)
		}
	case reflect.Bool:
		if wireOpt != "" && wireOpt != "bool" {
			return 0, fmt.Errorf("omnibuf: unsupported map key wire type %q for %v", wireOpt, kt)
		}
		return WireBoolTrue, nil
	default:
		return 0, fmt.Errorf("omnibuf: unsupported map key type %v", kt)
	}
}

func mapValWireType(vt reflect.Type, wireOpt string, building map[reflect.Type]*structCodec) (uint8, *structCodec, error) {
	switch vt.Kind() {
	case reflect.String:
		switch wireOpt {
		case "", "string":
			return WireString, nil, nil
		case "bytes":
			return WireBytes, nil, nil
		default:
			return 0, nil, fmt.Errorf("omnibuf: unsupported map value wire type %q for %v", wireOpt, vt)
		}
	case reflect.Slice:
		if vt.Elem().Kind() == reflect.Uint8 {
			switch wireOpt {
			case "", "bytes":
				return WireBytes, nil, nil
			case "string":
				return WireString, nil, nil
			default:
				return 0, nil, fmt.Errorf("omnibuf: unsupported map value wire type %q for %v", wireOpt, vt)
			}
		}
		return 0, nil, fmt.Errorf("omnibuf: unsupported map value type %v", vt)
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint:
		switch wireOpt {
		case "", "uint":
			return WireUint, nil, nil
		case "sint":
			return WireSint, nil, nil
		case "fixed32":
			return WireFixed32, nil, nil
		case "fixed64":
			return WireFixed64, nil, nil
		default:
			return 0, nil, fmt.Errorf("omnibuf: unsupported map value wire type %q for %v", wireOpt, vt)
		}
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
		switch wireOpt {
		case "", "sint":
			return WireSint, nil, nil
		case "uint":
			return WireUint, nil, nil
		case "fixed32":
			return WireFixed32, nil, nil
		case "fixed64":
			return WireFixed64, nil, nil
		default:
			return 0, nil, fmt.Errorf("omnibuf: unsupported map value wire type %q for %v", wireOpt, vt)
		}
	case reflect.Float32:
		if wireOpt != "" && wireOpt != "fixed32" {
			return 0, nil, fmt.Errorf("omnibuf: unsupported map value wire type %q for %v", wireOpt, vt)
		}
		return WireFixed32, nil, nil
	case reflect.Float64:
		if wireOpt != "" && wireOpt != "fixed64" {
			return 0, nil, fmt.Errorf("omnibuf: unsupported map value wire type %q for %v", wireOpt, vt)
		}
		return WireFixed64, nil, nil
	case reflect.Bool:
		if wireOpt != "" && wireOpt != "bool" {
			return 0, nil, fmt.Errorf("omnibuf: unsupported map value wire type %q for %v", wireOpt, vt)
		}
		return WireBoolTrue, nil, nil
	case reflect.Struct:
		if wireOpt != "" && wireOpt != "submsg" {
			return 0, nil, fmt.Errorf("omnibuf: unsupported map value wire type %q for %v", wireOpt, vt)
		}
		sub, err := compileStructCodec(vt, building)
		if err != nil {
			return 0, nil, err
		}
		return WireSubMsg, sub, nil
	case reflect.Pointer:
		if vt.Elem().Kind() == reflect.Struct {
			if wireOpt != "" && wireOpt != "submsg" {
				return 0, nil, fmt.Errorf("omnibuf: unsupported map value wire type %q for %v", wireOpt, vt)
			}
			sub, err := compileStructCodec(vt.Elem(), building)
			if err != nil {
				return 0, nil, err
			}
			return WireSubMsg, sub, nil
		}
		return 0, nil, fmt.Errorf("omnibuf: unsupported map value type %v", vt)
	default:
		return 0, nil, fmt.Errorf("omnibuf: unsupported map value type %v", vt)
	}
}

func pbTagEncToWireOpt(pbSubTag string) string {
	if pbSubTag == "" {
		return ""
	}
	enc := strings.Split(pbSubTag, ",")[0]
	switch enc {
	case "fixed32":
		return "fixed32"
	case "fixed64":
		return "fixed64"
	case "zigzag32", "zigzag64":
		return "sint"
	default:
		return ""
	}
}

func configureMapFieldCodec(fc *fieldCodec, sf reflect.StructField, ft reflect.Type, wireOpt string, building map[reflect.Type]*structCodec) error {
	if wireOpt == "map" {
		wireOpt = ""
	}
	keyWireOpt := pbTagEncToWireOpt(sf.Tag.Get("protobuf_key"))
	valWireOpt := pbTagEncToWireOpt(sf.Tag.Get("protobuf_val"))
	if wireOpt != "" {
		valWireOpt = wireOpt
	}
	keyWT, err := mapKeyWireType(ft.Key(), keyWireOpt)
	if err != nil {
		return fmt.Errorf("omnibuf: field %s: %w", sf.Name, err)
	}
	valWT, subCodec, err := mapValWireType(ft.Elem(), valWireOpt, building)
	if err != nil {
		return fmt.Errorf("omnibuf: field %s: %w", sf.Name, err)
	}

	fc.wireType = WireMap
	fc.mapKeyWT = keyWT
	fc.mapValWT = valWT
	fc.subCodec = subCodec
	fc.sliceType = ft
	fc.elemType = ft.Elem()

	if ft.Key() == reflect.TypeFor[string]() && ft.Elem() == reflect.TypeFor[uint64]() && keyWT == WireString && valWT == WireUint {
		fc.kind = kindMapStringUint
		return nil
	}
	fc.kind = kindMap
	return nil
}

// Marshal encodes any struct pointer (including plain Go structs with `omnibuf:"..."` tags
// and existing *pb.PeerUpdate protoc-gen-go structs) into a newly allocated OmniBuf packet.
func Marshal(v any) ([]byte, error) {
	return MarshalAppend(nil, v)
}

// MarshalAppend appends the OmniBuf encoding of struct pointer v to dst[:0] using the
// cached unsafe-offset table codec (0 reflection allocations on the hot path).
func MarshalAppend(dst []byte, v any) ([]byte, error) {
	return marshalTableAppend(dst, v, MarshalOptions{})
}

func marshalTableAppend(dst []byte, v any, opts MarshalOptions) ([]byte, error) {
	if pr, ok := v.(protoreflect.Message); ok {
		return opts.marshalProtoReflectAppend(dst, pr)
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return nil, fmt.Errorf("omnibuf: Marshal requires non-nil pointer to struct")
	}
	sc, err := getStructCodec(rv.Type().Elem())
	if err != nil {
		return nil, err
	}
	ptr := (*emptyInterface)(unsafe.Pointer(&v)).ptr
	if cap(dst) < 512 {
		dst = make([]byte, 0, 512)
	} else {
		dst = dst[:0]
	}
	if opts.SelfDescribing {
		dst = append(dst, sc.getField0Wire()...)
	}
	return sc.encodeStruct(dst, ptr, opts.Deterministic), nil
}

func isFieldEmpty(fc *fieldCodec, fPtr unsafe.Pointer) bool {
	switch fc.kind {
	case kindUint8, kindUint8Sint, kindInt8, kindInt8Uint, kindFixed32_8, kindSfixed32_8, kindFixed64_8, kindSfixed64_8:
		return *(*uint8)(fPtr) == 0
	case kindUint16, kindUint16Sint, kindInt16, kindInt16Uint, kindFixed32_16, kindSfixed32_16, kindFixed64_16, kindSfixed64_16:
		return *(*uint16)(fPtr) == 0
	case kindUint32, kindUint32Sint, kindInt32, kindSint32, kindInt32Uint, kindFixed64_32, kindSfixed64_32:
		return *(*uint32)(fPtr) == 0
	case kindUint64, kindUint64Sint, kindInt64, kindSint64, kindInt64Uint, kindFixed32_64, kindSfixed32_64:
		return *(*uint64)(fPtr) == 0
	case kindFloat32:
		return *(*float32)(fPtr) == 0
	case kindFloat64:
		return *(*float64)(fPtr) == 0
	case kindBool:
		return !*(*bool)(fPtr)
	case kindString:
		return len(*(*string)(fPtr)) == 0
	case kindBytes, kindStringSlice, kindBytesSlice, kindBoolSlice,
		kindUint8Slice, kindUint16Slice, kindUint32Slice, kindUint64Slice,
		kindUint8SintSlice, kindUint16SintSlice,
		kindInt8Slice, kindInt16Slice, kindInt32Slice, kindInt64Slice,
		kindFloat32Slice, kindFloat64Slice,
		kindUint8Fixed32Slice, kindUint16Fixed32Slice, kindUint64Fixed32Slice,
		kindInt8Fixed32Slice, kindInt16Fixed32Slice, kindInt64Fixed32Slice,
		kindUint8Fixed64Slice, kindUint16Fixed64Slice, kindUint32Fixed64Slice,
		kindInt8Fixed64Slice, kindInt16Fixed64Slice, kindInt32Fixed64Slice,
		kindStructValSlice, kindStructPtrSlice:
		return (*unsafeSliceHeader)(fPtr).Len == 0
	case kindStructPtr, kindScalarPtr:
		return *(*unsafe.Pointer)(fPtr) == nil
	case kindStructVal:
		return isStructEmpty(fc.subCodec, fPtr)
	case kindMapStringUint:
		return len(*(*map[string]uint64)(fPtr)) == 0
	case kindMap:
		return reflect.NewAt(fc.sliceType, fPtr).Elem().Len() == 0
	default:
		return false
	}
}

func isStructEmpty(sc *structCodec, base unsafe.Pointer) bool {
	for i := range sc.fields {
		fc := &sc.fields[i]
		if !isFieldEmpty(fc, unsafe.Add(base, fc.offset)) {
			return false
		}
	}
	return true
}

func (sc *structCodec) encodeStruct(dst []byte, base unsafe.Pointer, deterministic bool) []byte {
	fields := sc.fields
	for i := range fields {
		fc := &fields[i]
		fPtr := unsafe.Add(base, fc.offset)

		if (fc.kind == kindStructPtr || fc.kind == kindScalarPtr) && *(*unsafe.Pointer)(fPtr) == nil {
			continue
		}
		if fc.omitEmpty && isFieldEmpty(fc, fPtr) {
			continue
		}
		if fc.kind == kindScalarPtr {
			fPtr = *(*unsafe.Pointer)(fPtr)
			fc = fc.elemCodec
		}
		if fc.kind == kindBool {
			wt := WireBoolFalse
			if *(*bool)(fPtr) {
				wt = WireBoolTrue
			}
			rawTag := (uint32(fc.fieldNum) << 4) | uint32(wt)
			if rawTag <= 127 {
				dst = append(dst, byte(rawTag<<1)|1)
			} else if rawTag <= 16383 {
				u := uint16(rawTag<<2) | 2
				dst = append(dst, byte(u), byte(u>>8))
			} else {
				u := (rawTag << 3) | 4
				dst = append(dst, byte(u), byte(u>>8), byte(u>>16), byte(u>>24))
			}
			continue
		}

		if fc.is1BTag {
			dst = append(dst, fc.tag1B)
		} else if !fc.is4BTag {
			dst = append(dst, byte(fc.tag2B), byte(fc.tag2B>>8))
		} else {
			dst = append(dst, byte(fc.tag4B), byte(fc.tag4B>>8), byte(fc.tag4B>>16), byte(fc.tag4B>>24))
		}

		switch fc.kind {
		case kindUint8:
			dst = AppendVarint(dst, uint64(*(*uint8)(fPtr)))
		case kindUint16:
			dst = AppendVarint(dst, uint64(*(*uint16)(fPtr)))
		case kindUint32:
			dst = AppendVarint(dst, uint64(*(*uint32)(fPtr)))
		case kindUint64:
			dst = AppendVarint(dst, *(*uint64)(fPtr))
		case kindUint8Sint:
			dst = AppendVarint(dst, EncodeZigZag(int64(*(*uint8)(fPtr))))
		case kindUint16Sint:
			dst = AppendVarint(dst, EncodeZigZag(int64(*(*uint16)(fPtr))))
		case kindUint32Sint:
			dst = AppendVarint(dst, EncodeZigZag(int64(*(*uint32)(fPtr))))
		case kindUint64Sint:
			dst = AppendVarint(dst, EncodeZigZag(int64(*(*uint64)(fPtr))))
		case kindInt8:
			dst = AppendVarint(dst, EncodeZigZag(int64(*(*int8)(fPtr))))
		case kindInt16:
			dst = AppendVarint(dst, EncodeZigZag(int64(*(*int16)(fPtr))))
		case kindInt32, kindSint32:
			dst = AppendVarint(dst, EncodeZigZag(int64(*(*int32)(fPtr))))
		case kindInt64, kindSint64:
			dst = AppendVarint(dst, EncodeZigZag(*(*int64)(fPtr)))
		case kindInt8Uint:
			dst = AppendVarint(dst, uint64(uint8(*(*int8)(fPtr))))
		case kindInt16Uint:
			dst = AppendVarint(dst, uint64(uint16(*(*int16)(fPtr))))
		case kindInt32Uint:
			dst = AppendVarint(dst, uint64(uint32(*(*int32)(fPtr))))
		case kindInt64Uint:
			dst = AppendVarint(dst, uint64(*(*int64)(fPtr)))
		case kindFloat32:
			v := *(*uint32)(fPtr)
			dst = append(dst, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
		case kindFixed32_8:
			v := uint32(*(*uint8)(fPtr))
			dst = append(dst, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
		case kindSfixed32_8:
			v := uint32(int32(*(*int8)(fPtr)))
			dst = append(dst, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
		case kindFixed32_16:
			v := uint32(*(*uint16)(fPtr))
			dst = append(dst, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
		case kindSfixed32_16:
			v := uint32(int32(*(*int16)(fPtr)))
			dst = append(dst, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
		case kindFixed32_64, kindSfixed32_64:
			v := uint32(*(*uint64)(fPtr))
			dst = append(dst, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
		case kindFloat64:
			v := *(*uint64)(fPtr)
			dst = append(dst,
				byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
				byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56),
			)
		case kindFixed64_8:
			v := uint64(*(*uint8)(fPtr))
			dst = append(dst,
				byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
				byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56),
			)
		case kindSfixed64_8:
			v := uint64(int64(*(*int8)(fPtr)))
			dst = append(dst,
				byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
				byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56),
			)
		case kindFixed64_16:
			v := uint64(*(*uint16)(fPtr))
			dst = append(dst,
				byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
				byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56),
			)
		case kindSfixed64_16:
			v := uint64(int64(*(*int16)(fPtr)))
			dst = append(dst,
				byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
				byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56),
			)
		case kindFixed64_32:
			v := uint64(*(*uint32)(fPtr))
			dst = append(dst,
				byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
				byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56),
			)
		case kindSfixed64_32:
			v := uint64(int64(*(*int32)(fPtr)))
			dst = append(dst,
				byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
				byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56),
			)
		case kindString:
			s := *(*string)(fPtr)
			dst = AppendVarint(dst, uint64(len(s)))
			dst = append(dst, s...)
		case kindBytes:
			b := *(*[]byte)(fPtr)
			dst = AppendVarint(dst, uint64(len(b)))
			dst = append(dst, b...)
		case kindStringSlice:
			dst = EncodeStringArr(dst, *(*[]string)(fPtr))
		case kindBytesSlice:
			dst = encodeBytesArr(dst, *(*[][]byte)(fPtr))
		case kindBoolSlice:
			dst = encodeBoolArr(dst, *(*[]bool)(fPtr))
		case kindUint8Slice:
			dst = EncodeUintArr(dst, *(*[]uint8)(fPtr))
		case kindUint16Slice:
			dst = EncodeUintArr(dst, *(*[]uint16)(fPtr))
		case kindUint32Slice:
			dst = EncodeUintArr(dst, *(*[]uint32)(fPtr))
		case kindUint64Slice:
			dst = EncodeUintArr(dst, *(*[]uint64)(fPtr))
		case kindUint8SintSlice:
			dst = EncodeSintArr(dst, *(*[]uint8)(fPtr))
		case kindUint16SintSlice:
			dst = EncodeSintArr(dst, *(*[]uint16)(fPtr))
		case kindInt8Slice:
			dst = EncodeSintArr(dst, *(*[]int8)(fPtr))
		case kindInt16Slice:
			dst = EncodeSintArr(dst, *(*[]int16)(fPtr))
		case kindInt32Slice:
			dst = EncodeSintArr(dst, *(*[]int32)(fPtr))
		case kindInt64Slice:
			dst = EncodeSintArr(dst, *(*[]int64)(fPtr))
		case kindFloat32Slice:
			dst = encodeFixed32Arr(dst, *(*[]uint32)(fPtr))
		case kindFloat64Slice:
			dst = encodeFixed64Arr(dst, *(*[]uint64)(fPtr))
		case kindUint8Fixed32Slice:
			dst = encodeFixed32CastArr(dst, *(*[]uint8)(fPtr))
		case kindUint16Fixed32Slice:
			dst = encodeFixed32CastArr(dst, *(*[]uint16)(fPtr))
		case kindUint64Fixed32Slice, kindInt64Fixed32Slice:
			dst = encodeFixed32CastArr(dst, *(*[]uint64)(fPtr))
		case kindInt8Fixed32Slice:
			dst = encodeFixed32CastArr(dst, *(*[]int8)(fPtr))
		case kindInt16Fixed32Slice:
			dst = encodeFixed32CastArr(dst, *(*[]int16)(fPtr))
		case kindUint8Fixed64Slice:
			dst = encodeFixed64CastArr(dst, *(*[]uint8)(fPtr))
		case kindUint16Fixed64Slice:
			dst = encodeFixed64CastArr(dst, *(*[]uint16)(fPtr))
		case kindUint32Fixed64Slice:
			dst = encodeFixed64CastArr(dst, *(*[]uint32)(fPtr))
		case kindInt8Fixed64Slice:
			dst = encodeFixed64CastArr(dst, *(*[]int8)(fPtr))
		case kindInt16Fixed64Slice:
			dst = encodeFixed64CastArr(dst, *(*[]int16)(fPtr))
		case kindInt32Fixed64Slice:
			dst = encodeFixed64CastArr(dst, *(*[]int32)(fPtr))
		case kindStructVal:
			lenPos := len(dst)
			dst = append(dst, 0, 0)
			dst = fc.subCodec.encodeStruct(dst, fPtr, deterministic)
			dst = BackpatchContainerLen(dst, lenPos)
		case kindStructPtr:
			lenPos := len(dst)
			dst = append(dst, 0, 0)
			dst = fc.subCodec.encodeStruct(dst, *(*unsafe.Pointer)(fPtr), deterministic)
			dst = BackpatchContainerLen(dst, lenPos)
		case kindStructValSlice:
			sh := (*unsafeSliceHeader)(fPtr)
			lenPos := len(dst)
			dst = append(dst, 0, 0)
			for j := range sh.Len {
				eLenPos := len(dst)
				dst = append(dst, 0, 0)
				elemPtr := unsafe.Add(sh.Data, uintptr(j)*fc.elemSize)
				dst = fc.subCodec.encodeStruct(dst, elemPtr, deterministic)
				dst = BackpatchContainerLen(dst, eLenPos)
			}
			dst = BackpatchContainerLen(dst, lenPos)
		case kindStructPtrSlice:
			ptrs := *(*[]unsafe.Pointer)(fPtr)
			lenPos := len(dst)
			dst = append(dst, 0, 0)
			for j := range ptrs {
				eLenPos := len(dst)
				dst = append(dst, 0, 0)
				if ptrs[j] != nil {
					dst = fc.subCodec.encodeStruct(dst, ptrs[j], deterministic)
				}
				dst = BackpatchContainerLen(dst, eLenPos)
			}
			dst = BackpatchContainerLen(dst, lenPos)
		case kindMapStringUint:
			dst = encodeMapStringUint(dst, *(*map[string]uint64)(fPtr), deterministic)
		case kindMap:
			dst = encodeGenericMap(dst, fc, fPtr, deterministic)
		}
	}
	return dst
}

func sortReflectMapKeys(keys []reflect.Value) {
	if len(keys) <= 1 {
		return
	}
	kind := keys[0].Kind()
	slices.SortFunc(keys, func(a, b reflect.Value) int {
		switch kind {
		case reflect.String:
			return cmp.Compare(a.String(), b.String())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return cmp.Compare(a.Int(), b.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return cmp.Compare(a.Uint(), b.Uint())
		case reflect.Bool:
			ab, bb := a.Bool(), b.Bool()
			if ab == bb {
				return 0
			}
			if !ab && bb {
				return -1
			}
			return 1
		default:
			return 0
		}
	})
}

func appendReflectMapEntryPayload(dst []byte, rv reflect.Value, wt uint8, subCodec *structCodec, deterministic bool) []byte {
	switch wt {
	case WireString, WireBytes:
		if rv.Kind() == reflect.String {
			s := rv.String()
			dst = AppendVarint(dst, uint64(len(s)))
			return append(dst, s...)
		}
		b := rv.Bytes()
		dst = AppendVarint(dst, uint64(len(b)))
		return append(dst, b...)
	case WireUint:
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return AppendVarint(dst, uint64(rv.Int()))
		default:
			return AppendVarint(dst, rv.Uint())
		}
	case WireSint:
		switch rv.Kind() {
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return AppendVarint(dst, EncodeZigZag(int64(rv.Uint())))
		default:
			return AppendVarint(dst, EncodeZigZag(rv.Int()))
		}
	case WireFixed32:
		var bits uint32
		switch rv.Kind() {
		case reflect.Float32:
			bits = math.Float32bits(float32(rv.Float()))
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			bits = uint32(rv.Int())
		default:
			bits = uint32(rv.Uint())
		}
		return append(dst, byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24))
	case WireFixed64:
		var bits uint64
		switch rv.Kind() {
		case reflect.Float64:
			bits = math.Float64bits(rv.Float())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			bits = uint64(rv.Int())
		default:
			bits = rv.Uint()
		}
		return append(dst,
			byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24),
			byte(bits>>32), byte(bits>>40), byte(bits>>48), byte(bits>>56),
		)
	case WireBoolTrue, WireBoolFalse:
		if rv.Bool() {
			return append(dst, 1)
		}
		return append(dst, 0)
	case WireSubMsg:
		lenPos := len(dst)
		dst = append(dst, 0, 0)
		if rv.Kind() == reflect.Pointer {
			if !rv.IsNil() {
				dst = subCodec.encodeStruct(dst, rv.UnsafePointer(), deterministic)
			}
		} else {
			cp := reflect.New(rv.Type())
			cp.Elem().Set(rv)
			dst = subCodec.encodeStruct(dst, cp.UnsafePointer(), deterministic)
		}
		return BackpatchContainerLen(dst, lenPos)
	default:
		return dst
	}
}

func encodeGenericMap(dst []byte, fc *fieldCodec, fPtr unsafe.Pointer, deterministic bool) []byte {
	rv := reflect.NewAt(fc.sliceType, fPtr).Elem()
	lenPos := len(dst)
	dst = append(dst, 0, 0, MapHeaderByte(fc.mapKeyWT, fc.mapValWT))
	dst = AppendVarint(dst, uint64(rv.Len()))
	if deterministic && rv.Len() > 1 {
		keys := rv.MapKeys()
		sortReflectMapKeys(keys)
		for _, k := range keys {
			dst = appendReflectMapEntryPayload(dst, k, fc.mapKeyWT, nil, deterministic)
			dst = appendReflectMapEntryPayload(dst, rv.MapIndex(k), fc.mapValWT, fc.subCodec, deterministic)
		}
	} else {
		iter := rv.MapRange()
		for iter.Next() {
			dst = appendReflectMapEntryPayload(dst, iter.Key(), fc.mapKeyWT, nil, deterministic)
			dst = appendReflectMapEntryPayload(dst, iter.Value(), fc.mapValWT, fc.subCodec, deterministic)
		}
	}
	return BackpatchContainerLen(dst, lenPos)
}

type unsafeSliceHeader struct {
	Data unsafe.Pointer
	Len  int
	Cap  int
}

// Unmarshal decodes an OmniBuf packet into any struct pointer v (including *pb.PeerUpdate)
// using safe heap-allocated strings.
func Unmarshal(packet []byte, v any) error {
	return unmarshalTable(packet, v, false)
}

// UnmarshalZeroCopy decodes an OmniBuf packet into any struct pointer v using zero-copy
// unsafe.String references into packet.
func UnmarshalZeroCopy(packet []byte, v any) error {
	return unmarshalTable(packet, v, true)
}

func unmarshalTable(packet []byte, v any, zeroCopy bool) error {
	if pr, ok := v.(protoreflect.Message); ok {
		return UnmarshalProtoReflect(packet, pr)
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("omnibuf: Unmarshal requires non-nil pointer to struct")
	}
	sc, err := getStructCodec(rv.Type().Elem())
	if err != nil {
		return err
	}
	ptr := (*emptyInterface)(unsafe.Pointer(&v)).ptr
	return sc.decodeStruct(packet, ptr, zeroCopy, 0)
}

func sliceForDecode[T any](p *[]T, alreadySeen bool) []T {
	sl := *p
	if !alreadySeen && sl != nil {
		return sl[:0]
	}
	return sl
}

func isBoolField(fc *fieldCodec) bool {
	if fc.kind == kindBool {
		return true
	}
	if fc.kind == kindScalarPtr && fc.elemCodec.kind == kindBool {
		return true
	}
	return false
}

func (sc *structCodec) decodeStruct(data []byte, base unsafe.Pointer, zeroCopy bool, depth int) error {
	if depth > MaxNestingDepth {
		return fmt.Errorf("omnibuf: nesting depth %d exceeds MaxNestingDepth (%d)", depth, MaxNestingDepth)
	}
	n := len(data)
	pos := 0
	var seenMask uint64
	var seenOverflow map[uint16]bool

	for pos < n {
		b0 := data[pos]
		var tag uint32
		if (b0 & 1) != 0 {
			tag = uint32(b0 >> 1)
			pos++
		} else if (b0 & 2) != 0 {
			if pos+2 > n {
				return fmt.Errorf("omnibuf: truncated tag at %d", pos)
			}
			tag = uint32(binary.LittleEndian.Uint16(data[pos:]) >> 2)
			pos += 2
		} else {
			t64, tn := decodeVarintMulti(data[pos:])
			if tn == 0 || t64>>4 > math.MaxUint16 {
				return fmt.Errorf("omnibuf: truncated or invalid multi-byte tag at %d", pos)
			}
			tag = uint32(t64)
			pos += tn
		}
		fieldNum := uint16(tag >> 4)
		wt := uint8(tag & 0x0F)

		if fieldNum == 0 {
			if wt != WireSubMsg {
				return fmt.Errorf("omnibuf: invalid Field #0 wire type %d", wt)
			}
			nextPos, err := SkipValue(data, pos, wt)
			if err != nil {
				return err
			}
			pos = nextPos
			continue
		}

		var fc *fieldCodec
		if fieldNum <= maxDirectFieldSlot {
			fc = sc.byNum[fieldNum]
		} else if sc.overflow != nil {
			fc = sc.overflow[fieldNum]
		}
		if fc == nil || (!isBoolField(fc) && fc.wireType != wt) || (isBoolField(fc) && wt != WireBoolFalse && wt != WireBoolTrue) {
			nextPos, err := SkipValue(data, pos, wt)
			if err != nil {
				return err
			}
			pos = nextPos
			continue
		}

		var alreadySeen bool
		if fieldNum < 64 {
			alreadySeen = (seenMask & (1 << fieldNum)) != 0
			seenMask |= (1 << fieldNum)
		} else {
			alreadySeen = seenOverflow[fieldNum]
			if seenOverflow == nil {
				seenOverflow = make(map[uint16]bool)
			}
			seenOverflow[fieldNum] = true
		}
		fPtr := unsafe.Add(base, fc.offset)

		nextPos, err := decodeFieldValue(data, pos, wt, fc, fPtr, alreadySeen, zeroCopy, depth)
		if err != nil {
			return err
		}
		pos = nextPos
	}
	return nil
}

func decodeFieldValue(data []byte, pos int, wt uint8, fc *fieldCodec, fPtr unsafe.Pointer, alreadySeen, zeroCopy bool, depth int) (int, error) {
	n := len(data)
	if fc.kind == kindScalarPtr {
		subPtr := *(*unsafe.Pointer)(fPtr)
		if subPtr == nil {
			subPtr = reflect.New(fc.elemType).UnsafePointer()
			*(*unsafe.Pointer)(fPtr) = subPtr
		}
		return decodeFieldValue(data, pos, wt, fc.elemCodec, subPtr, alreadySeen, zeroCopy, depth)
	}

	switch fc.kind {
	case kindUint8, kindInt8Uint:
		v, vn := DecodeVarint(data[pos:])
		if vn == 0 {
			return 0, fmt.Errorf("omnibuf: truncated uint8 at %d", pos)
		}
		*(*uint8)(fPtr) = uint8(v)
		return pos + vn, nil
	case kindUint16, kindInt16Uint:
		v, vn := DecodeVarint(data[pos:])
		if vn == 0 {
			return 0, fmt.Errorf("omnibuf: truncated uint16 at %d", pos)
		}
		*(*uint16)(fPtr) = uint16(v)
		return pos + vn, nil
	case kindUint32, kindInt32Uint:
		v, vn := DecodeVarint(data[pos:])
		if vn == 0 {
			return 0, fmt.Errorf("omnibuf: truncated uint32 at %d", pos)
		}
		*(*uint32)(fPtr) = uint32(v)
		return pos + vn, nil
	case kindUint64, kindInt64Uint:
		v, vn := DecodeVarint(data[pos:])
		if vn == 0 {
			return 0, fmt.Errorf("omnibuf: truncated uint64 at %d", pos)
		}
		*(*uint64)(fPtr) = v
		return pos + vn, nil
	case kindInt8, kindUint8Sint:
		v, vn := DecodeVarint(data[pos:])
		if vn == 0 {
			return 0, fmt.Errorf("omnibuf: truncated int8 at %d", pos)
		}
		*(*int8)(fPtr) = int8(DecodeZigZag(v))
		return pos + vn, nil
	case kindInt16, kindUint16Sint:
		v, vn := DecodeVarint(data[pos:])
		if vn == 0 {
			return 0, fmt.Errorf("omnibuf: truncated int16 at %d", pos)
		}
		*(*int16)(fPtr) = int16(DecodeZigZag(v))
		return pos + vn, nil
	case kindInt32, kindSint32, kindUint32Sint:
		v, vn := DecodeVarint(data[pos:])
		if vn == 0 {
			return 0, fmt.Errorf("omnibuf: truncated int32 at %d", pos)
		}
		*(*int32)(fPtr) = int32(DecodeZigZag(v))
		return pos + vn, nil
	case kindInt64, kindSint64, kindUint64Sint:
		v, vn := DecodeVarint(data[pos:])
		if vn == 0 {
			return 0, fmt.Errorf("omnibuf: truncated int64 at %d", pos)
		}
		*(*int64)(fPtr) = DecodeZigZag(v)
		return pos + vn, nil
	case kindBool:
		*(*bool)(fPtr) = (wt == WireBoolTrue)
		return pos, nil
	case kindFloat32:
		if n-pos < 4 {
			return 0, fmt.Errorf("omnibuf: truncated fixed32 at %d", pos)
		}
		*(*uint32)(fPtr) = binary.LittleEndian.Uint32(data[pos:])
		return pos + 4, nil
	case kindFixed32_8, kindSfixed32_8:
		if n-pos < 4 {
			return 0, fmt.Errorf("omnibuf: truncated fixed32 at %d", pos)
		}
		*(*uint8)(fPtr) = uint8(binary.LittleEndian.Uint32(data[pos:]))
		return pos + 4, nil
	case kindFixed32_16, kindSfixed32_16:
		if n-pos < 4 {
			return 0, fmt.Errorf("omnibuf: truncated fixed32 at %d", pos)
		}
		*(*uint16)(fPtr) = uint16(binary.LittleEndian.Uint32(data[pos:]))
		return pos + 4, nil
	case kindFixed32_64:
		if n-pos < 4 {
			return 0, fmt.Errorf("omnibuf: truncated fixed32 at %d", pos)
		}
		*(*uint64)(fPtr) = uint64(binary.LittleEndian.Uint32(data[pos:]))
		return pos + 4, nil
	case kindSfixed32_64:
		if n-pos < 4 {
			return 0, fmt.Errorf("omnibuf: truncated fixed32 at %d", pos)
		}
		*(*int64)(fPtr) = int64(int32(binary.LittleEndian.Uint32(data[pos:])))
		return pos + 4, nil
	case kindFloat64:
		if n-pos < 8 {
			return 0, fmt.Errorf("omnibuf: truncated fixed64 at %d", pos)
		}
		*(*uint64)(fPtr) = binary.LittleEndian.Uint64(data[pos:])
		return pos + 8, nil
	case kindFixed64_8, kindSfixed64_8:
		if n-pos < 8 {
			return 0, fmt.Errorf("omnibuf: truncated fixed64 at %d", pos)
		}
		*(*uint8)(fPtr) = uint8(binary.LittleEndian.Uint64(data[pos:]))
		return pos + 8, nil
	case kindFixed64_16, kindSfixed64_16:
		if n-pos < 8 {
			return 0, fmt.Errorf("omnibuf: truncated fixed64 at %d", pos)
		}
		*(*uint16)(fPtr) = uint16(binary.LittleEndian.Uint64(data[pos:]))
		return pos + 8, nil
	case kindFixed64_32, kindSfixed64_32:
		if n-pos < 8 {
			return 0, fmt.Errorf("omnibuf: truncated fixed64 at %d", pos)
		}
		*(*uint32)(fPtr) = uint32(binary.LittleEndian.Uint64(data[pos:]))
		return pos + 8, nil
	case kindString:
		s, nextPos, err := readLenSlice(data, pos, 0)
		if err != nil {
			return 0, err
		}
		*(*string)(fPtr) = DecodeStr(s, zeroCopy)
		return nextPos, nil
	case kindBytes:
		b, nextPos, err := readLenSlice(data, pos, 0)
		if err != nil {
			return 0, err
		}
		if zeroCopy {
			*(*[]byte)(fPtr) = b
		} else {
			cp := make([]byte, len(b))
			copy(cp, b)
			*(*[]byte)(fPtr) = cp
		}
		return nextPos, nil
	case kindStringSlice:
		body, nextPos, err := readLenSlice(data, pos, 1)
		if err != nil {
			return 0, err
		}
		p := (*[]string)(fPtr)
		sl, ok := appendStringArrBody(sliceForDecode(p, alreadySeen), body, zeroCopy)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid string array at %d", pos)
		}
		*p = sl
		return nextPos, nil
	case kindBytesSlice:
		body, nextPos, err := readLenSlice(data, pos, 1)
		if err != nil {
			return 0, err
		}
		p := (*[][]byte)(fPtr)
		sl, ok := appendBytesArrBody(sliceForDecode(p, alreadySeen), body, zeroCopy)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid bytes array at %d", pos)
		}
		*p = sl
		return nextPos, nil
	case kindBoolSlice:
		body, nextPos, err := readLenSlice(data, pos, 1)
		if err != nil {
			return 0, err
		}
		p := (*[]bool)(fPtr)
		sl, ok := appendBoolArrBody(sliceForDecode(p, alreadySeen), body)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid bool array at %d", pos)
		}
		*p = sl
		return nextPos, nil
	case kindUint8Slice:
		body, nextPos, err := readLenSlice(data, pos, 1)
		if err != nil {
			return 0, err
		}
		p := (*[]uint8)(fPtr)
		sl, ok := appendUintArrBody(sliceForDecode(p, alreadySeen), body)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid uint8 array at %d", pos)
		}
		*p = sl
		return nextPos, nil
	case kindUint16Slice:
		body, nextPos, err := readLenSlice(data, pos, 1)
		if err != nil {
			return 0, err
		}
		p := (*[]uint16)(fPtr)
		sl, ok := appendUintArrBody(sliceForDecode(p, alreadySeen), body)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid uint16 array at %d", pos)
		}
		*p = sl
		return nextPos, nil
	case kindUint32Slice:
		body, nextPos, err := readLenSlice(data, pos, 1)
		if err != nil {
			return 0, err
		}
		p := (*[]uint32)(fPtr)
		sl, ok := appendUintArrBody(sliceForDecode(p, alreadySeen), body)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid uint32 array at %d", pos)
		}
		*p = sl
		return nextPos, nil
	case kindUint64Slice:
		body, nextPos, err := readLenSlice(data, pos, 1)
		if err != nil {
			return 0, err
		}
		p := (*[]uint64)(fPtr)
		sl, ok := appendUintArrBody(sliceForDecode(p, alreadySeen), body)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid uint64 array at %d", pos)
		}
		*p = sl
		return nextPos, nil
	case kindUint8SintSlice:
		body, nextPos, err := readLenSlice(data, pos, 1)
		if err != nil {
			return 0, err
		}
		p := (*[]uint8)(fPtr)
		sl, ok := appendSintArrBody(sliceForDecode(p, alreadySeen), body)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid uint8 sint array at %d", pos)
		}
		*p = sl
		return nextPos, nil
	case kindUint16SintSlice:
		body, nextPos, err := readLenSlice(data, pos, 1)
		if err != nil {
			return 0, err
		}
		p := (*[]uint16)(fPtr)
		sl, ok := appendSintArrBody(sliceForDecode(p, alreadySeen), body)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid uint16 sint array at %d", pos)
		}
		*p = sl
		return nextPos, nil
	case kindInt8Slice:
		body, nextPos, err := readLenSlice(data, pos, 1)
		if err != nil {
			return 0, err
		}
		p := (*[]int8)(fPtr)
		sl, ok := appendSintArrBody(sliceForDecode(p, alreadySeen), body)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid int8 array at %d", pos)
		}
		*p = sl
		return nextPos, nil
	case kindInt16Slice:
		body, nextPos, err := readLenSlice(data, pos, 1)
		if err != nil {
			return 0, err
		}
		p := (*[]int16)(fPtr)
		sl, ok := appendSintArrBody(sliceForDecode(p, alreadySeen), body)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid int16 array at %d", pos)
		}
		*p = sl
		return nextPos, nil
	case kindInt32Slice:
		body, nextPos, err := readLenSlice(data, pos, 1)
		if err != nil {
			return 0, err
		}
		p := (*[]int32)(fPtr)
		sl, ok := appendSintArrBody(sliceForDecode(p, alreadySeen), body)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid int32 array at %d", pos)
		}
		*p = sl
		return nextPos, nil
	case kindInt64Slice:
		body, nextPos, err := readLenSlice(data, pos, 1)
		if err != nil {
			return 0, err
		}
		p := (*[]int64)(fPtr)
		sl, ok := appendSintArrBody(sliceForDecode(p, alreadySeen), body)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid int64 array at %d", pos)
		}
		*p = sl
		return nextPos, nil
	case kindFloat32Slice:
		body, nextPos, err := readLenSlice(data, pos, 1)
		if err != nil {
			return 0, err
		}
		p := (*[]uint32)(fPtr)
		sl, ok := appendFixed32ArrBody(sliceForDecode(p, alreadySeen), body)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid fixed32 array at %d", pos)
		}
		*p = sl
		return nextPos, nil
	case kindFloat64Slice:
		body, nextPos, err := readLenSlice(data, pos, 1)
		if err != nil {
			return 0, err
		}
		p := (*[]uint64)(fPtr)
		sl, ok := appendFixed64ArrBody(sliceForDecode(p, alreadySeen), body)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid fixed64 array at %d", pos)
		}
		*p = sl
		return nextPos, nil
	case kindUint8Fixed32Slice, kindInt8Fixed32Slice:
		body, nextPos, err := readLenSlice(data, pos, 1)
		if err != nil {
			return 0, err
		}
		p := (*[]uint8)(fPtr)
		sl, ok := appendFixed32CastArrBody(sliceForDecode(p, alreadySeen), body)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid fixed32 array at %d", pos)
		}
		*p = sl
		return nextPos, nil
	case kindUint16Fixed32Slice, kindInt16Fixed32Slice:
		body, nextPos, err := readLenSlice(data, pos, 1)
		if err != nil {
			return 0, err
		}
		p := (*[]uint16)(fPtr)
		sl, ok := appendFixed32CastArrBody(sliceForDecode(p, alreadySeen), body)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid fixed32 array at %d", pos)
		}
		*p = sl
		return nextPos, nil
	case kindUint64Fixed32Slice:
		body, nextPos, err := readLenSlice(data, pos, 1)
		if err != nil {
			return 0, err
		}
		p := (*[]uint64)(fPtr)
		sl, ok := appendFixed32CastArrBody(sliceForDecode(p, alreadySeen), body)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid fixed32 array at %d", pos)
		}
		*p = sl
		return nextPos, nil
	case kindInt64Fixed32Slice:
		body, nextPos, err := readLenSlice(data, pos, 1)
		if err != nil {
			return 0, err
		}
		p := (*[]int64)(fPtr)
		sl, ok := appendSfixed32CastArrBody(sliceForDecode(p, alreadySeen), body)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid fixed32 array at %d", pos)
		}
		*p = sl
		return nextPos, nil
	case kindUint8Fixed64Slice, kindInt8Fixed64Slice:
		body, nextPos, err := readLenSlice(data, pos, 1)
		if err != nil {
			return 0, err
		}
		p := (*[]uint8)(fPtr)
		sl, ok := appendFixed64CastArrBody(sliceForDecode(p, alreadySeen), body)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid fixed64 array at %d", pos)
		}
		*p = sl
		return nextPos, nil
	case kindUint16Fixed64Slice, kindInt16Fixed64Slice:
		body, nextPos, err := readLenSlice(data, pos, 1)
		if err != nil {
			return 0, err
		}
		p := (*[]uint16)(fPtr)
		sl, ok := appendFixed64CastArrBody(sliceForDecode(p, alreadySeen), body)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid fixed64 array at %d", pos)
		}
		*p = sl
		return nextPos, nil
	case kindUint32Fixed64Slice, kindInt32Fixed64Slice:
		body, nextPos, err := readLenSlice(data, pos, 1)
		if err != nil {
			return 0, err
		}
		p := (*[]uint32)(fPtr)
		sl, ok := appendFixed64CastArrBody(sliceForDecode(p, alreadySeen), body)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid fixed64 array at %d", pos)
		}
		*p = sl
		return nextPos, nil
	case kindStructVal:
		subBytes, nextPos, err := readLenSlice(data, pos, 0)
		if err != nil {
			return 0, err
		}
		if err := fc.subCodec.decodeStruct(subBytes, fPtr, zeroCopy, depth+1); err != nil {
			return 0, err
		}
		return nextPos, nil
	case kindStructPtr:
		subBytes, nextPos, err := readLenSlice(data, pos, 0)
		if err != nil {
			return 0, err
		}
		subPtr := *(*unsafe.Pointer)(fPtr)
		if subPtr == nil {
			subPtr = reflect.New(fc.elemType).UnsafePointer()
			*(*unsafe.Pointer)(fPtr) = subPtr
		}
		if err := fc.subCodec.decodeStruct(subBytes, subPtr, zeroCopy, depth+1); err != nil {
			return 0, err
		}
		return nextPos, nil
	case kindStructValSlice:
		arrPayload, nextPos, err := readLenSlice(data, pos, 0)
		if err != nil {
			return 0, err
		}
		elemCount, ok := CountSubMsgArrElems(arrPayload)
		if !ok {
			return 0, fmt.Errorf("omnibuf: corrupt submsg array at %d", pos)
		}
		sh := (*unsafeSliceHeader)(fPtr)
		start := 0
		if alreadySeen {
			start = sh.Len
		}
		total := start + elemCount
		if sh.Cap < total {
			newSlice := reflect.MakeSlice(fc.sliceType, total, total)
			newPtr := newSlice.UnsafePointer()
			if start > 0 && sh.Data != nil {
				reflect.Copy(newSlice, reflect.NewAt(fc.sliceType, unsafe.Pointer(sh)).Elem())
			}
			sh.Data = newPtr
			sh.Len = total
			sh.Cap = total
		} else {
			sh.Len = total
		}
		p := 0
		for j := range elemCount {
			el, en := DecodeVarint(arrPayload[p:])
			p += en
			elemPtr := unsafe.Add(sh.Data, uintptr(start+j)*fc.elemSize)
			if err := fc.subCodec.decodeStruct(arrPayload[p:p+int(el)], elemPtr, zeroCopy, depth+1); err != nil {
				return 0, err
			}
			p += int(el)
		}
		return nextPos, nil
	case kindStructPtrSlice:
		arrPayload, nextPos, err := readLenSlice(data, pos, 0)
		if err != nil {
			return 0, err
		}
		elemCount, ok := CountSubMsgArrElems(arrPayload)
		if !ok {
			return 0, fmt.Errorf("omnibuf: corrupt submsg array at %d", pos)
		}
		ptrs := *(*[]unsafe.Pointer)(fPtr)
		start := 0
		if alreadySeen {
			start = len(ptrs)
		}
		total := start + elemCount
		if cap(ptrs) < total {
			slab := reflect.MakeSlice(reflect.SliceOf(fc.elemType), elemCount, elemCount)
			slabPtr := slab.UnsafePointer()
			next := make([]unsafe.Pointer, total)
			copy(next, ptrs[:start])
			for j := range elemCount {
				next[start+j] = unsafe.Add(slabPtr, uintptr(j)*fc.elemSize)
			}
			ptrs = next
			*(*[]unsafe.Pointer)(fPtr) = ptrs
		} else {
			ptrs = ptrs[:total]
			*(*[]unsafe.Pointer)(fPtr) = ptrs
		}
		p := 0
		for j := range elemCount {
			el, en := DecodeVarint(arrPayload[p:])
			p += en
			if ptrs[start+j] == nil {
				ptrs[start+j] = reflect.New(fc.elemType).UnsafePointer()
			}
			if err := fc.subCodec.decodeStruct(arrPayload[p:p+int(el)], ptrs[start+j], zeroCopy, depth+1); err != nil {
				return 0, err
			}
			p += int(el)
		}
		return nextPos, nil
	case kindMapStringUint:
		body, nextPos, err := readLenSlice(data, pos, 2)
		if err != nil {
			return 0, err
		}
		var prev map[string]uint64
		if alreadySeen {
			prev = *(*map[string]uint64)(fPtr)
		}
		m, ok := decodeMapStringUintBody(prev, body, zeroCopy)
		if !ok {
			return 0, fmt.Errorf("omnibuf: invalid WireMap at %d", pos)
		}
		*(*map[string]uint64)(fPtr) = m
		return nextPos, nil
	case kindMap:
		body, nextPos, err := readLenSlice(data, pos, 2)
		if err != nil {
			return 0, err
		}
		if err := decodeGenericMapBody(fc, fPtr, body, alreadySeen, zeroCopy, depth); err != nil {
			return 0, err
		}
		return nextPos, nil
	default:
		return 0, fmt.Errorf("omnibuf: unsupported field kind %d", fc.kind)
	}
}

func decodeReflectMapEntryPayload(body []byte, p int, targetType reflect.Type, wt uint8, subCodec *structCodec, zeroCopy bool, depth int) (reflect.Value, int, error) {
	rv := reflect.New(targetType).Elem()
	switch wt {
	case WireString, WireBytes:
		sBytes, nextP, err := readLenSlice(body, p, 0)
		if err != nil {
			return reflect.Value{}, 0, err
		}
		if targetType.Kind() == reflect.String {
			rv.SetString(DecodeStr(sBytes, zeroCopy))
		} else {
			if zeroCopy {
				rv.SetBytes(sBytes)
			} else {
				cp := make([]byte, len(sBytes))
				copy(cp, sBytes)
				rv.SetBytes(cp)
			}
		}
		return rv, nextP, nil
	case WireUint:
		v, vn := DecodeVarint(body[p:])
		if vn == 0 {
			return reflect.Value{}, 0, fmt.Errorf("omnibuf: truncated WireMap uint at %d", p)
		}
		switch targetType.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			rv.SetInt(int64(v))
		default:
			rv.SetUint(v)
		}
		return rv, p + vn, nil
	case WireSint:
		v, vn := DecodeVarint(body[p:])
		if vn == 0 {
			return reflect.Value{}, 0, fmt.Errorf("omnibuf: truncated WireMap sint at %d", p)
		}
		sv := DecodeZigZag(v)
		switch targetType.Kind() {
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			rv.SetUint(uint64(sv))
		default:
			rv.SetInt(sv)
		}
		return rv, p + vn, nil
	case WireFixed32:
		if len(body)-p < 4 {
			return reflect.Value{}, 0, fmt.Errorf("omnibuf: truncated WireMap fixed32 at %d", p)
		}
		bits := binary.LittleEndian.Uint32(body[p : p+4])
		switch targetType.Kind() {
		case reflect.Float32:
			rv.SetFloat(float64(math.Float32frombits(bits)))
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			rv.SetInt(int64(int32(bits)))
		default:
			rv.SetUint(uint64(bits))
		}
		return rv, p + 4, nil
	case WireFixed64:
		if len(body)-p < 8 {
			return reflect.Value{}, 0, fmt.Errorf("omnibuf: truncated WireMap fixed64 at %d", p)
		}
		bits := binary.LittleEndian.Uint64(body[p : p+8])
		switch targetType.Kind() {
		case reflect.Float64:
			rv.SetFloat(math.Float64frombits(bits))
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			rv.SetInt(int64(bits))
		default:
			rv.SetUint(bits)
		}
		return rv, p + 8, nil
	case WireBoolTrue, WireBoolFalse:
		if len(body)-p < 1 {
			return reflect.Value{}, 0, fmt.Errorf("omnibuf: truncated WireMap bool at %d", p)
		}
		b := body[p]
		if b != 0 && b != 1 {
			return reflect.Value{}, 0, fmt.Errorf("omnibuf: invalid WireMap bool %d at %d", b, p)
		}
		rv.SetBool(b == 1)
		return rv, p + 1, nil
	case WireSubMsg:
		subBytes, nextP, err := readLenSlice(body, p, 0)
		if err != nil {
			return reflect.Value{}, 0, err
		}
		if targetType.Kind() == reflect.Pointer {
			ptrVal := reflect.New(targetType.Elem())
			if err := subCodec.decodeStruct(subBytes, ptrVal.UnsafePointer(), zeroCopy, depth+1); err != nil {
				return reflect.Value{}, 0, err
			}
			return ptrVal, nextP, nil
		}
		ptrVal := reflect.New(targetType)
		if err := subCodec.decodeStruct(subBytes, ptrVal.UnsafePointer(), zeroCopy, depth+1); err != nil {
			return reflect.Value{}, 0, err
		}
		return ptrVal.Elem(), nextP, nil
	default:
		return reflect.Value{}, 0, fmt.Errorf("omnibuf: invalid WireMap entry wire type %d", wt)
	}
}

func decodeGenericMapBody(fc *fieldCodec, fPtr unsafe.Pointer, body []byte, alreadySeen, zeroCopy bool, depth int) error {
	if len(body) < 2 || body[0] != MapHeaderByte(fc.mapKeyWT, fc.mapValWT) {
		return fmt.Errorf("omnibuf: WireMap header mismatch (got 0x%02x, want 0x%02x)", body[0], MapHeaderByte(fc.mapKeyWT, fc.mapValWT))
	}
	count, cn := DecodeVarint(body[1:])
	if cn == 0 || count*2 > uint64(len(body)-(1+cn)) {
		return fmt.Errorf("omnibuf: invalid WireMap count %d", count)
	}
	mapVal := reflect.NewAt(fc.sliceType, fPtr).Elem()
	if !alreadySeen || mapVal.IsNil() {
		mapVal.Set(reflect.MakeMapWithSize(fc.sliceType, int(count)))
	}
	keyType := fc.sliceType.Key()
	valType := fc.sliceType.Elem()
	p := 1 + cn
	for range int(count) {
		kv, nextP, err := decodeReflectMapEntryPayload(body, p, keyType, fc.mapKeyWT, nil, zeroCopy, depth)
		if err != nil {
			return err
		}
		vv, nextP2, err := decodeReflectMapEntryPayload(body, nextP, valType, fc.mapValWT, fc.subCodec, zeroCopy, depth)
		if err != nil {
			return err
		}
		p = nextP2
		mapVal.SetMapIndex(kv, vv)
	}
	if p != len(body) {
		return fmt.Errorf("omnibuf: WireMap trailing bytes (%d != %d)", p, len(body))
	}
	return nil
}
