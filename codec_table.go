package omnibuf

import (
	"encoding/binary"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// ============================================================================
// Generic Table-Driven Marshal / Unmarshal for Existing Go / .pb.go Structs
// ============================================================================
//
// Works directly on existing `protoc-gen-go` generated structs (e.g. *pb.PeerUpdate)
// or any Go struct annotated with `protobuf:"..."` or `omnibuf:"..."` struct tags,
// WITHOUT requiring a custom protoc plugin.
//
// Just like google.golang.org/protobuf's internal `protoimpl` table-driven engine,
// OmniBuf inspects the struct's reflection metadata ONCE per reflect.Type, compiles a
// flat array of `fieldCodec` descriptors with precomputed `unsafe.Pointer` byte offsets,
// and executes Marshal / Unmarshal using direct pointer arithmetic.

const maxDirectFieldSlot = 16

type fieldKind uint8

const (
	kindUint32 fieldKind = iota
	kindUint64
	kindInt32
	kindInt64
	kindSint32
	kindSint64
	kindBool
	kindFloat32
	kindFloat64
	kindString
	kindBytes
	kindStringSlice
	kindUint32Slice
	kindUint64Slice
	kindInt32Slice
	kindInt64Slice
	kindFloat32Slice
	kindFloat64Slice
	kindStructVal
	kindStructPtr
	kindStructValSlice
	kindStructPtrSlice
	kindMapStringUint
)

type fieldCodec struct {
	fieldNum  uint16
	wireType  uint8
	tag1B     byte   // Precomputed 1-byte Prefix-Varint tag if fieldNum <= 7
	tag2B     uint16 // Precomputed 2-byte Prefix-Varint tag if fieldNum >= 8
	is1BTag   bool
	kind      fieldKind
	name      string
	offset    uintptr
	subCodec  *structCodec
	elemSize  uintptr
	sliceType reflect.Type
	elemType  reflect.Type
}

type structCodec struct {
	typ         reflect.Type
	size        uintptr
	fields      []fieldCodec
	byNum       [maxDirectFieldSlot + 1]*fieldCodec
	overflow    map[uint16]*fieldCodec
	schemaNames []string
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

	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		pbTag := sf.Tag.Get("protobuf")
		if pbTag == "" {
			pbTag = sf.Tag.Get("omnibuf")
		}
		if pbTag == "" || pbTag == "-" {
			continue
		}
		fc, err := parseStructField(sf, pbTag, building)
		if err != nil {
			return nil, err
		}
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

func parseStructField(sf reflect.StructField, pbTag string, building map[reflect.Type]*structCodec) (fieldCodec, error) {
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

	fc := fieldCodec{
		fieldNum: fnum,
		name:     name,
		offset:   sf.Offset,
	}

	ft := sf.Type
	switch ft.Kind() {
	case reflect.Uint32:
		if encType == "fixed32" {
			fc.kind = kindFloat32
			fc.wireType = WireFixed32
		} else {
			fc.kind = kindUint32
			fc.wireType = WireUint
		}
	case reflect.Uint64:
		if encType == "fixed64" {
			fc.kind = kindFloat64
			fc.wireType = WireFixed64
		} else {
			fc.kind = kindUint64
			fc.wireType = WireUint
		}
	case reflect.Int32:
		if encType == "zigzag32" {
			fc.kind = kindSint32
		} else {
			fc.kind = kindInt32
		}
		fc.wireType = WireSint
	case reflect.Int64:
		if encType == "zigzag64" {
			fc.kind = kindSint64
		} else {
			fc.kind = kindInt64
		}
		fc.wireType = WireSint
	case reflect.Bool:
		fc.kind = kindBool
		fc.wireType = WireBoolTrue
	case reflect.Float32:
		fc.kind = kindFloat32
		fc.wireType = WireFixed32
	case reflect.Float64:
		fc.kind = kindFloat64
		fc.wireType = WireFixed64
	case reflect.String:
		fc.kind = kindString
		fc.wireType = WireString
	case reflect.Struct:
		sub, err := compileStructCodec(ft, building)
		if err != nil {
			return fieldCodec{}, err
		}
		fc.kind = kindStructVal
		fc.wireType = WireSubMsg
		fc.subCodec = sub
	case reflect.Pointer:
		if ft.Elem().Kind() != reflect.Struct {
			return fieldCodec{}, fmt.Errorf("omnibuf: unsupported pointer field %s", sf.Name)
		}
		sub, err := compileStructCodec(ft.Elem(), building)
		if err != nil {
			return fieldCodec{}, err
		}
		fc.kind = kindStructPtr
		fc.wireType = WireSubMsg
		fc.subCodec = sub
		fc.elemType = ft.Elem()
	case reflect.Slice:
		elem := ft.Elem()
		switch elem.Kind() {
		case reflect.Uint8:
			fc.kind = kindBytes
			fc.wireType = WireBytes
		case reflect.String:
			fc.kind = kindStringSlice
			fc.wireType = WireStringArr
		case reflect.Uint32:
			if encType == "fixed32" {
				fc.kind = kindFloat32Slice
				fc.wireType = WireFixedArr
			} else {
				fc.kind = kindUint32Slice
				fc.wireType = WireUintArr
			}
		case reflect.Uint64:
			if encType == "fixed64" {
				fc.kind = kindFloat64Slice
				fc.wireType = WireFixedArr
			} else {
				fc.kind = kindUint64Slice
				fc.wireType = WireUintArr
			}
		case reflect.Int32:
			if encType == "fixed32" {
				fc.kind = kindFloat32Slice
				fc.wireType = WireFixedArr
			} else {
				fc.kind = kindInt32Slice
				fc.wireType = WireSintArr
			}
		case reflect.Int64:
			if encType == "fixed64" {
				fc.kind = kindFloat64Slice
				fc.wireType = WireFixedArr
			} else {
				fc.kind = kindInt64Slice
				fc.wireType = WireSintArr
			}
		case reflect.Float32:
			fc.kind = kindFloat32Slice
			fc.wireType = WireFixedArr
		case reflect.Float64:
			fc.kind = kindFloat64Slice
			fc.wireType = WireFixedArr
		case reflect.Struct:
			sub, err := compileStructCodec(elem, building)
			if err != nil {
				return fieldCodec{}, err
			}
			fc.kind = kindStructValSlice
			fc.wireType = WireSubMsgArr
			fc.subCodec = sub
			fc.elemSize = elem.Size()
			fc.sliceType = ft
			fc.elemType = elem
		case reflect.Pointer:
			if elem.Elem().Kind() != reflect.Struct {
				return fieldCodec{}, fmt.Errorf("omnibuf: unsupported slice element %v", elem)
			}
			sub, err := compileStructCodec(elem.Elem(), building)
			if err != nil {
				return fieldCodec{}, err
			}
			fc.kind = kindStructPtrSlice
			fc.wireType = WireSubMsgArr
			fc.subCodec = sub
			fc.elemSize = elem.Elem().Size()
			fc.sliceType = ft
			fc.elemType = elem.Elem()
		default:
			return fieldCodec{}, fmt.Errorf("omnibuf: unsupported slice type %v on %s", ft, sf.Name)
		}
	case reflect.Map:
		if ft.Key().Kind() == reflect.String && ft.Elem().Kind() == reflect.Uint64 {
			fc.kind = kindMapStringUint
			fc.wireType = WireMap
		} else {
			return fieldCodec{}, fmt.Errorf("omnibuf: unsupported map type %v on %s", ft, sf.Name)
		}
	default:
		return fieldCodec{}, fmt.Errorf("omnibuf: unsupported field type %v on %s", ft, sf.Name)
	}

	rawTag := (fnum << 4) | uint16(fc.wireType)
	if rawTag <= 127 {
		fc.is1BTag = true
		fc.tag1B = byte(rawTag<<1) | 1
	} else {
		fc.is1BTag = false
		fc.tag2B = (rawTag << 2) | 2
	}
	return fc, nil
}

// Marshal encodes any struct pointer (including existing *pb.PeerUpdate protoc-gen-go structs)
// into a newly allocated OmniBuf packet.
func Marshal(v any) ([]byte, error) {
	return MarshalAppend(nil, v)
}

// MarshalAppend appends the OmniBuf encoding of struct pointer v to dst[:0] using the
// cached unsafe-offset table codec (0 reflection allocations on the hot path).
func MarshalAppend(dst []byte, v any) ([]byte, error) {
	return marshalTableAppend(dst, v, false)
}

func marshalTableAppend(dst []byte, v any, deterministic bool) ([]byte, error) {
	if pr, ok := v.(protoreflect.Message); ok {
		return MarshalOptions{Deterministic: deterministic}.marshalProtoReflectAppend(dst, pr)
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
	return sc.encodeStruct(dst, ptr, deterministic), nil
}

func (sc *structCodec) encodeStruct(dst []byte, base unsafe.Pointer, deterministic bool) []byte {
	fields := sc.fields
	for i := range fields {
		fc := &fields[i]
		fPtr := unsafe.Add(base, fc.offset)

		if fc.kind == kindStructPtr && *(*unsafe.Pointer)(fPtr) == nil {
			continue
		}

		if fc.kind == kindBool {
			wt := WireBoolFalse
			if *(*bool)(fPtr) {
				wt = WireBoolTrue
			}
			rawTag := (fc.fieldNum << 4) | uint16(wt)
			if rawTag <= 127 {
				dst = append(dst, byte(rawTag<<1)|1)
			} else {
				u := (rawTag << 2) | 2
				dst = append(dst, byte(u), byte(u>>8))
			}
			continue
		}

		if fc.is1BTag {
			dst = append(dst, fc.tag1B)
		} else {
			dst = append(dst, byte(fc.tag2B), byte(fc.tag2B>>8))
		}

		switch fc.kind {
		case kindUint32:
			dst = AppendVarint(dst, uint64(*(*uint32)(fPtr)))
		case kindUint64:
			dst = AppendVarint(dst, *(*uint64)(fPtr))
		case kindInt32, kindSint32:
			dst = AppendVarint(dst, EncodeZigZag(int64(*(*int32)(fPtr))))
		case kindInt64, kindSint64:
			dst = AppendVarint(dst, EncodeZigZag(*(*int64)(fPtr)))
		case kindFloat32:
			v := *(*uint32)(fPtr)
			dst = append(dst, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
		case kindFloat64:
			v := *(*uint64)(fPtr)
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
			dst = encodeStringArr(dst, *(*[]string)(fPtr))
		case kindUint32Slice:
			dst = encodeUint32Arr(dst, *(*[]uint32)(fPtr))
		case kindUint64Slice:
			dst = encodeUint64Arr(dst, *(*[]uint64)(fPtr))
		case kindInt32Slice:
			dst = encodeSint32Arr(dst, *(*[]int32)(fPtr))
		case kindInt64Slice:
			dst = encodeSint64Arr(dst, *(*[]int64)(fPtr))
		case kindFloat32Slice:
			dst = encodeFixed32Arr(dst, *(*[]uint32)(fPtr))
		case kindFloat64Slice:
			dst = encodeFixed64Arr(dst, *(*[]uint64)(fPtr))
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
		}
	}
	return dst
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

func (sc *structCodec) decodeStruct(data []byte, base unsafe.Pointer, zeroCopy bool, depth int) error {
	if depth > MaxNestingDepth {
		return fmt.Errorf("omnibuf: nesting depth %d exceeds MaxNestingDepth (%d)", depth, MaxNestingDepth)
	}
	n := len(data)
	pos := 0
	var seenMask uint64

	for pos < n {
		b0 := data[pos]
		var tag uint16
		if (b0 & 1) != 0 {
			tag = uint16(b0 >> 1)
			pos++
		} else if (b0 & 2) != 0 {
			if pos+2 > n {
				return fmt.Errorf("omnibuf: truncated tag at %d", pos)
			}
			tag = binary.LittleEndian.Uint16(data[pos:]) >> 2
			pos += 2
		} else {
			t64, tn := decodeVarintMulti(data[pos:])
			if tn == 0 {
				return fmt.Errorf("omnibuf: truncated multi-byte tag at %d", pos)
			}
			tag = uint16(t64)
			pos += tn
		}
		fieldNum := tag >> 4
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
		if fc == nil || (fc.kind != kindBool && fc.wireType != wt) || (fc.kind == kindBool && wt != WireBoolFalse && wt != WireBoolTrue) {
			nextPos, err := SkipValue(data, pos, wt)
			if err != nil {
				return err
			}
			pos = nextPos
			continue
		}

		alreadySeen := fieldNum < 64 && (seenMask&(1<<fieldNum)) != 0
		if fieldNum < 64 {
			seenMask |= (1 << fieldNum)
		}
		fPtr := unsafe.Add(base, fc.offset)

		switch fc.kind {
		case kindUint32:
			v, vn := DecodeVarint(data[pos:])
			if vn == 0 {
				return fmt.Errorf("omnibuf: truncated uint32 at %d", pos)
			}
			pos += vn
			*(*uint32)(fPtr) = uint32(v)
		case kindUint64:
			v, vn := DecodeVarint(data[pos:])
			if vn == 0 {
				return fmt.Errorf("omnibuf: truncated uint64 at %d", pos)
			}
			pos += vn
			*(*uint64)(fPtr) = v
		case kindInt32, kindSint32:
			v, vn := DecodeVarint(data[pos:])
			if vn == 0 {
				return fmt.Errorf("omnibuf: truncated int32 at %d", pos)
			}
			pos += vn
			*(*int32)(fPtr) = int32(DecodeZigZag(v))
		case kindInt64, kindSint64:
			v, vn := DecodeVarint(data[pos:])
			if vn == 0 {
				return fmt.Errorf("omnibuf: truncated int64 at %d", pos)
			}
			pos += vn
			*(*int64)(fPtr) = DecodeZigZag(v)
		case kindBool:
			*(*bool)(fPtr) = (wt == WireBoolTrue)
		case kindFloat32:
			if n-pos < 4 {
				return fmt.Errorf("omnibuf: truncated fixed32 at %d", pos)
			}
			*(*uint32)(fPtr) = binary.LittleEndian.Uint32(data[pos:])
			pos += 4
		case kindFloat64:
			if n-pos < 8 {
				return fmt.Errorf("omnibuf: truncated fixed64 at %d", pos)
			}
			*(*uint64)(fPtr) = binary.LittleEndian.Uint64(data[pos:])
			pos += 8
		case kindString:
			s, nextPos, err := readLenSlice(data, pos, 0)
			if err != nil {
				return err
			}
			*(*string)(fPtr) = DecodeStr(s, zeroCopy)
			pos = nextPos
		case kindBytes:
			b, nextPos, err := readLenSlice(data, pos, 0)
			if err != nil {
				return err
			}
			if zeroCopy {
				*(*[]byte)(fPtr) = b
			} else {
				cp := make([]byte, len(b))
				copy(cp, b)
				*(*[]byte)(fPtr) = cp
			}
			pos = nextPos
		case kindStringSlice:
			body, nextPos, err := readLenSlice(data, pos, 1)
			if err != nil {
				return err
			}
			p := (*[]string)(fPtr)
			sl, ok := appendStringArrBody(sliceForDecode(p, alreadySeen), body, zeroCopy)
			if !ok {
				return fmt.Errorf("omnibuf: invalid string array at %d", pos)
			}
			*p = sl
			pos = nextPos
		case kindUint32Slice:
			body, nextPos, err := readLenSlice(data, pos, 1)
			if err != nil {
				return err
			}
			p := (*[]uint32)(fPtr)
			sl, ok := appendUint32ArrBody(sliceForDecode(p, alreadySeen), body)
			if !ok {
				return fmt.Errorf("omnibuf: invalid uint32 array at %d", pos)
			}
			*p = sl
			pos = nextPos
		case kindUint64Slice:
			body, nextPos, err := readLenSlice(data, pos, 1)
			if err != nil {
				return err
			}
			p := (*[]uint64)(fPtr)
			sl, ok := appendUint64ArrBody(sliceForDecode(p, alreadySeen), body)
			if !ok {
				return fmt.Errorf("omnibuf: invalid uint64 array at %d", pos)
			}
			*p = sl
			pos = nextPos
		case kindInt32Slice:
			body, nextPos, err := readLenSlice(data, pos, 1)
			if err != nil {
				return err
			}
			p := (*[]int32)(fPtr)
			sl, ok := appendSint32ArrBody(sliceForDecode(p, alreadySeen), body)
			if !ok {
				return fmt.Errorf("omnibuf: invalid int32 array at %d", pos)
			}
			*p = sl
			pos = nextPos
		case kindInt64Slice:
			body, nextPos, err := readLenSlice(data, pos, 1)
			if err != nil {
				return err
			}
			p := (*[]int64)(fPtr)
			sl, ok := appendSint64ArrBody(sliceForDecode(p, alreadySeen), body)
			if !ok {
				return fmt.Errorf("omnibuf: invalid int64 array at %d", pos)
			}
			*p = sl
			pos = nextPos
		case kindFloat32Slice:
			body, nextPos, err := readLenSlice(data, pos, 1)
			if err != nil {
				return err
			}
			p := (*[]uint32)(fPtr)
			sl, ok := appendFixed32ArrBody(sliceForDecode(p, alreadySeen), body)
			if !ok {
				return fmt.Errorf("omnibuf: invalid fixed32 array at %d", pos)
			}
			*p = sl
			pos = nextPos
		case kindFloat64Slice:
			body, nextPos, err := readLenSlice(data, pos, 1)
			if err != nil {
				return err
			}
			p := (*[]uint64)(fPtr)
			sl, ok := appendFixed64ArrBody(sliceForDecode(p, alreadySeen), body)
			if !ok {
				return fmt.Errorf("omnibuf: invalid fixed64 array at %d", pos)
			}
			*p = sl
			pos = nextPos
		case kindStructVal:
			subBytes, nextPos, err := readLenSlice(data, pos, 0)
			if err != nil {
				return err
			}
			if err := fc.subCodec.decodeStruct(subBytes, fPtr, zeroCopy, depth+1); err != nil {
				return err
			}
			pos = nextPos
		case kindStructPtr:
			subBytes, nextPos, err := readLenSlice(data, pos, 0)
			if err != nil {
				return err
			}
			subPtr := *(*unsafe.Pointer)(fPtr)
			if subPtr == nil {
				subPtr = reflect.New(fc.elemType).UnsafePointer()
				*(*unsafe.Pointer)(fPtr) = subPtr
			}
			if err := fc.subCodec.decodeStruct(subBytes, subPtr, zeroCopy, depth+1); err != nil {
				return err
			}
			pos = nextPos
		case kindStructValSlice:
			arrPayload, nextPos, err := readLenSlice(data, pos, 0)
			if err != nil {
				return err
			}
			elemCount, ok := CountSubMsgArrElems(arrPayload)
			if !ok {
				return fmt.Errorf("omnibuf: corrupt submsg array at %d", pos)
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
					return err
				}
				p += int(el)
			}
			pos = nextPos
		case kindStructPtrSlice:
			arrPayload, nextPos, err := readLenSlice(data, pos, 0)
			if err != nil {
				return err
			}
			elemCount, ok := CountSubMsgArrElems(arrPayload)
			if !ok {
				return fmt.Errorf("omnibuf: corrupt submsg array at %d", pos)
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
					return err
				}
				p += int(el)
			}
			pos = nextPos
		case kindMapStringUint:
			body, nextPos, err := readLenSlice(data, pos, 2)
			if err != nil {
				return err
			}
			var prev map[string]uint64
			if alreadySeen {
				prev = *(*map[string]uint64)(fPtr)
			}
			m, ok := decodeMapStringUintBody(prev, body, zeroCopy)
			if !ok {
				return fmt.Errorf("omnibuf: invalid WireMap at %d", pos)
			}
			*(*map[string]uint64)(fPtr) = m
			pos = nextPos
		}
	}
	return nil
}
