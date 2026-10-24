package omnibuf

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"math"
	"slices"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// MarshalOptions configures OmniBuf wire encoding.
type MarshalOptions struct {
	// Deterministic sorts WireMap keys in ascending order so identical messages
	// always produce byte-for-byte identical wire output (useful for golden files,
	// reproducible builds, and content-addressable hashing).
	Deterministic bool

	// SelfDescribing emits Field #0 (Tag = 0x0B, WireSubMsg = 5, omnibuf.v1.SchemaMeta)
	// at byte 0 of the root message with field names and nested children schemas.
	SelfDescribing bool
}

// Marshal encodes v (any struct pointer or protoreflect.Message) using the configured options.
func (o MarshalOptions) Marshal(v any) ([]byte, error) {
	return o.MarshalAppend(nil, v)
}

// MarshalAppend appends the OmniBuf encoding of v to dst[:0] using the configured options.
func (o MarshalOptions) MarshalAppend(dst []byte, v any) ([]byte, error) {
	if pr, ok := v.(protoreflect.Message); ok {
		return o.marshalProtoReflectAppend(dst, pr)
	}
	return marshalTableAppend(dst, v, o.Deterministic)
}

// MarshalDeterministic encodes any struct pointer or protoreflect.Message with
// deterministic WireMap key ordering.
func MarshalDeterministic(v any) ([]byte, error) {
	return MarshalOptions{Deterministic: true}.Marshal(v)
}

// MarshalProtoReflect encodes any protoreflect.Message (including *dynamicpb.Message)
// into an OmniBuf packet. When includeField0 is true, Field #0 (WireSubMsg, SchemaMeta) is
// emitted at byte 0 of the root message with the schema's field names and nested children schemas.
func MarshalProtoReflect(m protoreflect.Message, includeField0 bool) ([]byte, error) {
	return MarshalProtoReflectAppend(nil, m, includeField0)
}

// MarshalProtoReflectAppend appends the OmniBuf encoding of m to dst[:0].
func MarshalProtoReflectAppend(dst []byte, m protoreflect.Message, includeField0 bool) ([]byte, error) {
	return MarshalOptions{SelfDescribing: includeField0}.marshalProtoReflectAppend(dst, m)
}

func (o MarshalOptions) marshalProtoReflectAppend(dst []byte, m protoreflect.Message) ([]byte, error) {
	if m == nil {
		return nil, fmt.Errorf("omnibuf: MarshalProtoReflect requires non-nil protoreflect.Message")
	}
	if cap(dst) < 256 {
		dst = make([]byte, 0, 256)
	} else {
		dst = dst[:0]
	}
	if o.SelfDescribing {
		meta := buildProtoReflectSchemaMeta(m.Descriptor(), 0)
		dst = AppendField0SchemaMeta(dst, meta)
	}
	bodyOpts := o
	bodyOpts.SelfDescribing = false
	return encodeProtoReflectMsg(dst, m, bodyOpts)
}

func buildProtoReflectSchemaMeta(md protoreflect.MessageDescriptor, depth int) *SchemaMeta {
	if md == nil || depth > 8 {
		return nil
	}
	fields := md.Fields()
	nFields := fields.Len()
	meta := &SchemaMeta{
		TypeName: string(md.Name()),
		Fields:   make(map[uint32]string, nFields),
	}
	for i := range nFields {
		fd := fields.Get(i)
		fnum := uint32(fd.Number())
		meta.Fields[fnum] = string(fd.Name())
		if fd.IsMap() {
			if fd.MapValue().Kind() == protoreflect.MessageKind {
				if child := buildProtoReflectSchemaMeta(fd.MapValue().Message(), depth+1); child != nil {
					if meta.Children == nil {
						meta.Children = make(map[uint32]*SchemaMeta)
					}
					meta.Children[fnum] = child
				}
			}
		} else if fd.Kind() == protoreflect.MessageKind {
			if child := buildProtoReflectSchemaMeta(fd.Message(), depth+1); child != nil {
				if meta.Children == nil {
					meta.Children = make(map[uint32]*SchemaMeta)
				}
				meta.Children[fnum] = child
			}
		}
	}
	return meta
}

func appendRawTag(dst []byte, fieldNum uint16, wt uint8) []byte {
	tag := (uint64(fieldNum) << 4) | uint64(wt&0x0F)
	return AppendVarint(dst, tag)
}

func sortProtoMapKeys(kd protoreflect.FieldDescriptor, keys []protoreflect.MapKey) {
	kind := kd.Kind()
	slices.SortFunc(keys, func(a, b protoreflect.MapKey) int {
		switch kind {
		case protoreflect.BoolKind:
			ab, bb := a.Bool(), b.Bool()
			if ab == bb {
				return 0
			}
			if !ab && bb {
				return -1
			}
			return 1
		case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
			protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
			return cmp.Compare(a.Int(), b.Int())
		case protoreflect.Uint32Kind, protoreflect.Fixed32Kind,
			protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
			return cmp.Compare(a.Uint(), b.Uint())
		default:
			return cmp.Compare(a.String(), b.String())
		}
	})
}

func encodeProtoReflectMsg(dst []byte, m protoreflect.Message, opts MarshalOptions) ([]byte, error) {
	fields := m.Descriptor().Fields()
	nFields := fields.Len()

	for i := range nFields {
		fd := fields.Get(i)
		fnum := uint16(fd.Number())
		val := m.Get(fd)

		if fd.IsMap() {
			mp := val.Map()
			keyWT, err := mapKindWireType(fd.MapKey().Kind())
			if err != nil {
				return nil, fmt.Errorf("omnibuf: field %s: %w", fd.Name(), err)
			}
			valWT, err := mapKindWireType(fd.MapValue().Kind())
			if err != nil {
				return nil, fmt.Errorf("omnibuf: field %s: %w", fd.Name(), err)
			}
			dst = appendRawTag(dst, fnum, WireMap)
			lenPos := len(dst)
			dst = append(dst, 0, 0, MapHeaderByte(keyWT, valWT))
			dst = AppendVarint(dst, uint64(mp.Len()))
			if opts.Deterministic && mp.Len() > 1 {
				keys := make([]protoreflect.MapKey, 0, mp.Len())
				mp.Range(func(k protoreflect.MapKey, _ protoreflect.Value) bool {
					keys = append(keys, k)
					return true
				})
				sortProtoMapKeys(fd.MapKey(), keys)
				for _, k := range keys {
					dst, err = appendMapEntryKey(dst, fd.MapKey(), k)
					if err != nil {
						return nil, err
					}
					dst, err = appendMapEntryVal(dst, fd.MapValue(), mp.Get(k), opts)
					if err != nil {
						return nil, err
					}
				}
			} else {
				var mapErr error
				mp.Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
					dst, mapErr = appendMapEntryKey(dst, fd.MapKey(), k)
					if mapErr != nil {
						return false
					}
					dst, mapErr = appendMapEntryVal(dst, fd.MapValue(), v, opts)
					return mapErr == nil
				})
				if mapErr != nil {
					return nil, mapErr
				}
			}
			dst = BackpatchContainerLen(dst, lenPos)
			continue
		}

		if fd.IsList() {
			list := val.List()
			c := list.Len()
			switch fd.Kind() {
			case protoreflect.StringKind:
				dst = appendRawTag(dst, fnum, WireStringArr)
				lenPos := len(dst)
				dst = append(dst, 0, 0)
				dst = AppendVarint(dst, uint64(c))
				for j := range c {
					s := list.Get(j).String()
					dst = AppendVarint(dst, uint64(len(s)))
					dst = append(dst, s...)
				}
				dst = BackpatchContainerLen(dst, lenPos)
			case protoreflect.BytesKind:
				dst = appendRawTag(dst, fnum, WireStringArr)
				lenPos := len(dst)
				dst = append(dst, 0, 0)
				dst = AppendVarint(dst, uint64(c))
				for j := range c {
					b := list.Get(j).Bytes()
					dst = AppendVarint(dst, uint64(len(b)))
					dst = append(dst, b...)
				}
				dst = BackpatchContainerLen(dst, lenPos)
			case protoreflect.Uint32Kind, protoreflect.Uint64Kind, protoreflect.EnumKind, protoreflect.BoolKind:
				dst = appendRawTag(dst, fnum, WireUintArr)
				lenPos := len(dst)
				dst = append(dst, 0, 0)
				dst = AppendVarint(dst, uint64(c))
				for j := range c {
					switch fd.Kind() {
					case protoreflect.EnumKind:
						dst = AppendVarint(dst, uint64(list.Get(j).Enum()))
					case protoreflect.BoolKind:
						if list.Get(j).Bool() {
							dst = AppendVarint(dst, 1)
						} else {
							dst = AppendVarint(dst, 0)
						}
					default:
						dst = AppendVarint(dst, list.Get(j).Uint())
					}
				}
				dst = BackpatchContainerLen(dst, lenPos)
			case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Int64Kind, protoreflect.Sint64Kind:
				dst = appendRawTag(dst, fnum, WireSintArr)
				lenPos := len(dst)
				dst = append(dst, 0, 0)
				dst = AppendVarint(dst, uint64(c))
				for j := range c {
					dst = AppendVarint(dst, EncodeZigZag(list.Get(j).Int()))
				}
				dst = BackpatchContainerLen(dst, lenPos)
			case protoreflect.FloatKind, protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind:
				dst = appendRawTag(dst, fnum, WireFixedArr)
				dst = AppendVarint(dst, uint64(1+c*4))
				dst = append(dst, WireFixed32)
				for j := range c {
					var bits uint32
					switch fd.Kind() {
					case protoreflect.FloatKind:
						bits = math.Float32bits(float32(list.Get(j).Float()))
					case protoreflect.Fixed32Kind:
						bits = uint32(list.Get(j).Uint())
					case protoreflect.Sfixed32Kind:
						bits = uint32(int32(list.Get(j).Int()))
					}
					dst = append(dst, byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24))
				}
			case protoreflect.DoubleKind, protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind:
				dst = appendRawTag(dst, fnum, WireFixedArr)
				dst = AppendVarint(dst, uint64(1+c*8))
				dst = append(dst, WireFixed64)
				for j := range c {
					var bits uint64
					switch fd.Kind() {
					case protoreflect.DoubleKind:
						bits = math.Float64bits(list.Get(j).Float())
					case protoreflect.Fixed64Kind:
						bits = list.Get(j).Uint()
					case protoreflect.Sfixed64Kind:
						bits = uint64(list.Get(j).Int())
					}
					dst = append(dst,
						byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24),
						byte(bits>>32), byte(bits>>40), byte(bits>>48), byte(bits>>56),
					)
				}
			case protoreflect.MessageKind:
				dst = appendRawTag(dst, fnum, WireSubMsgArr)
				arrLenPos := len(dst)
				dst = append(dst, 0, 0)
				for j := range c {
					elemLenPos := len(dst)
					dst = append(dst, 0, 0)
					var err error
					dst, err = encodeProtoReflectMsg(dst, list.Get(j).Message(), opts)
					if err != nil {
						return nil, err
					}
					dst = BackpatchContainerLen(dst, elemLenPos)
				}
				dst = BackpatchContainerLen(dst, arrLenPos)
			default:
				return nil, fmt.Errorf("omnibuf: unsupported repeated kind %v on field %s", fd.Kind(), fd.Name())
			}
			continue
		}

		// Singular field:
		if fd.Kind() == protoreflect.MessageKind {
			if !m.Has(fd) {
				continue
			}
			dst = appendRawTag(dst, fnum, WireSubMsg)
			lenPos := len(dst)
			dst = append(dst, 0, 0)
			var err error
			dst, err = encodeProtoReflectMsg(dst, val.Message(), opts)
			if err != nil {
				return nil, err
			}
			dst = BackpatchContainerLen(dst, lenPos)
			continue
		}

		switch fd.Kind() {
		case protoreflect.Uint32Kind, protoreflect.Uint64Kind:
			dst = appendRawTag(dst, fnum, WireUint)
			dst = AppendVarint(dst, val.Uint())
		case protoreflect.EnumKind:
			dst = appendRawTag(dst, fnum, WireUint)
			dst = AppendVarint(dst, uint64(val.Enum()))
		case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Int64Kind, protoreflect.Sint64Kind:
			dst = appendRawTag(dst, fnum, WireSint)
			dst = AppendVarint(dst, EncodeZigZag(val.Int()))
		case protoreflect.BoolKind:
			wt := WireBoolFalse
			if val.Bool() {
				wt = WireBoolTrue
			}
			dst = appendRawTag(dst, fnum, wt)
		case protoreflect.StringKind:
			s := val.String()
			dst = appendRawTag(dst, fnum, WireString)
			dst = AppendVarint(dst, uint64(len(s)))
			dst = append(dst, s...)
		case protoreflect.BytesKind:
			b := val.Bytes()
			dst = appendRawTag(dst, fnum, WireBytes)
			dst = AppendVarint(dst, uint64(len(b)))
			dst = append(dst, b...)
		case protoreflect.FloatKind:
			bits := math.Float32bits(float32(val.Float()))
			dst = appendRawTag(dst, fnum, WireFixed32)
			dst = append(dst, byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24))
		case protoreflect.Fixed32Kind:
			bits := uint32(val.Uint())
			dst = appendRawTag(dst, fnum, WireFixed32)
			dst = append(dst, byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24))
		case protoreflect.Sfixed32Kind:
			bits := uint32(int32(val.Int()))
			dst = appendRawTag(dst, fnum, WireFixed32)
			dst = append(dst, byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24))
		case protoreflect.DoubleKind:
			bits := math.Float64bits(val.Float())
			dst = appendRawTag(dst, fnum, WireFixed64)
			dst = append(dst,
				byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24),
				byte(bits>>32), byte(bits>>40), byte(bits>>48), byte(bits>>56),
			)
		case protoreflect.Fixed64Kind:
			bits := val.Uint()
			dst = appendRawTag(dst, fnum, WireFixed64)
			dst = append(dst,
				byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24),
				byte(bits>>32), byte(bits>>40), byte(bits>>48), byte(bits>>56),
			)
		case protoreflect.Sfixed64Kind:
			bits := uint64(val.Int())
			dst = appendRawTag(dst, fnum, WireFixed64)
			dst = append(dst,
				byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24),
				byte(bits>>32), byte(bits>>40), byte(bits>>48), byte(bits>>56),
			)
		default:
			return nil, fmt.Errorf("omnibuf: unsupported field kind %v on field %s", fd.Kind(), fd.Name())
		}
	}
	return dst, nil
}

// UnmarshalProtoReflect decodes an OmniBuf packet into any protoreflect.Message (including *dynamicpb.Message).
func UnmarshalProtoReflect(data []byte, m protoreflect.Message) error {
	if m == nil {
		return fmt.Errorf("omnibuf: UnmarshalProtoReflect requires non-nil protoreflect.Message")
	}
	return decodeProtoReflectMsg(data, m, 0)
}

func decodeProtoReflectMsg(data []byte, m protoreflect.Message, depth int) error {
	if depth > MaxNestingDepth {
		return fmt.Errorf("omnibuf: nesting depth %d exceeds MaxNestingDepth (%d)", depth, MaxNestingDepth)
	}
	fields := m.Descriptor().Fields()
	n := len(data)
	pos := 0
	seen := make(map[uint16]bool)

	for pos < n {
		tag, tn := DecodeVarint(data[pos:])
		if tn == 0 {
			return fmt.Errorf("omnibuf: truncated tag at %d", pos)
		}
		pos += tn
		fnum := uint16(tag >> 4)
		wt := uint8(tag & 0x0F)

		if fnum == 0 {
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

		fd := fields.ByNumber(protoreflect.FieldNumber(fnum))
		if fd == nil {
			nextPos, err := SkipValue(data, pos, wt)
			if err != nil {
				return err
			}
			pos = nextPos
			continue
		}

		alreadySeen := seen[fnum]
		seen[fnum] = true

		if fd.IsMap() {
			if wt != WireMap {
				nextPos, err := SkipValue(data, pos, wt)
				if err != nil {
					return err
				}
				pos = nextPos
				continue
			}
			bLen, bn := DecodeVarint(data[pos:])
			if bn == 0 || bLen < 2 || bLen > uint64(n-(pos+bn)) {
				return fmt.Errorf("omnibuf: invalid WireMap length at %d", pos)
			}
			pos += bn
			end := pos + int(bLen)
			mapType := data[pos]
			keyWT := mapType >> 4
			valWT := mapType & 0x0F
			expKeyWT, err := mapKindWireType(fd.MapKey().Kind())
			if err != nil || keyWT != expKeyWT {
				return fmt.Errorf("omnibuf: WireMap key wire type mismatch at %d", pos)
			}
			expValWT, err := mapKindWireType(fd.MapValue().Kind())
			if err != nil || valWT != expValWT {
				return fmt.Errorf("omnibuf: WireMap val wire type mismatch at %d", pos)
			}
			count, cn := DecodeVarint(data[pos+1 : end])
			if cn == 0 || count > uint64(end-(pos+1+cn)) {
				return fmt.Errorf("omnibuf: invalid WireMap count at %d", pos)
			}
			p := pos + 1 + cn
			mp := m.Mutable(fd).Map()
			if !alreadySeen {
				mp.Range(func(k protoreflect.MapKey, _ protoreflect.Value) bool {
					mp.Clear(k)
					return true
				})
			}
			for range count {
				mk, nextP, err := decodeMapEntryKey(data, p, end, fd.MapKey())
				if err != nil {
					return err
				}
				p = nextP
				mv, nextP2, err := decodeMapEntryVal(data, p, end, fd.MapValue(), mp, depth)
				if err != nil {
					return err
				}
				p = nextP2
				mp.Set(mk, mv)
			}
			if p != end {
				return fmt.Errorf("omnibuf: WireMap trailing bytes at %d", p)
			}
			pos = end
			continue
		}

		if fd.IsList() {
			list := m.Mutable(fd).List()
			if !alreadySeen {
				list.Truncate(0)
			}
			switch fd.Kind() {
			case protoreflect.StringKind, protoreflect.BytesKind:
				if wt != WireStringArr {
					nextPos, err := SkipValue(data, pos, wt)
					if err != nil {
						return err
					}
					pos = nextPos
					continue
				}
				bLen, bn := DecodeVarint(data[pos:])
				if bn == 0 || bLen == 0 || bLen > uint64(n-(pos+bn)) {
					return fmt.Errorf("omnibuf: invalid WireStringArr length at %d", pos)
				}
				pos += bn
				end := pos + int(bLen)
				count, cn := DecodeVarint(data[pos:end])
				if cn == 0 || count > uint64(end-(pos+cn)) {
					return fmt.Errorf("omnibuf: invalid WireStringArr count at %d", pos)
				}
				p := pos + cn
				for range count {
					sLen, sn := DecodeVarint(data[p:end])
					if sn == 0 || sLen > uint64(end-(p+sn)) {
						return fmt.Errorf("omnibuf: invalid WireStringArr elem at %d", p)
					}
					p += sn
					if fd.Kind() == protoreflect.BytesKind {
						cp := make([]byte, int(sLen))
						copy(cp, data[p:p+int(sLen)])
						list.Append(protoreflect.ValueOfBytes(cp))
					} else {
						list.Append(protoreflect.ValueOfString(string(data[p : p+int(sLen)])))
					}
					p += int(sLen)
				}
				if p != end {
					return fmt.Errorf("omnibuf: WireStringArr byteLen mismatch at %d", p)
				}
				pos = end
			case protoreflect.Uint32Kind, protoreflect.Uint64Kind, protoreflect.EnumKind, protoreflect.BoolKind:
				if wt != WireUintArr {
					nextPos, err := SkipValue(data, pos, wt)
					if err != nil {
						return err
					}
					pos = nextPos
					continue
				}
				bLen, bn := DecodeVarint(data[pos:])
				if bn == 0 || bLen == 0 || bLen > uint64(n-(pos+bn)) {
					return fmt.Errorf("omnibuf: invalid WireUintArr length at %d", pos)
				}
				pos += bn
				end := pos + int(bLen)
				count, cn := DecodeVarint(data[pos:end])
				if cn == 0 || count > uint64(end-(pos+cn)) {
					return fmt.Errorf("omnibuf: invalid WireUintArr count at %d", pos)
				}
				p := pos + cn
				for range count {
					v, vn := DecodeVarint(data[p:end])
					if vn == 0 {
						return fmt.Errorf("omnibuf: truncated WireUintArr elem at %d", p)
					}
					p += vn
					switch fd.Kind() {
					case protoreflect.Uint32Kind:
						list.Append(protoreflect.ValueOfUint32(uint32(v)))
					case protoreflect.Uint64Kind:
						list.Append(protoreflect.ValueOfUint64(v))
					case protoreflect.EnumKind:
						list.Append(protoreflect.ValueOfEnum(protoreflect.EnumNumber(v)))
					case protoreflect.BoolKind:
						list.Append(protoreflect.ValueOfBool(v != 0))
					}
				}
				if p != end {
					return fmt.Errorf("omnibuf: WireUintArr byteLen mismatch at %d", p)
				}
				pos = end
			case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Int64Kind, protoreflect.Sint64Kind:
				if wt != WireSintArr {
					nextPos, err := SkipValue(data, pos, wt)
					if err != nil {
						return err
					}
					pos = nextPos
					continue
				}
				bLen, bn := DecodeVarint(data[pos:])
				if bn == 0 || bLen == 0 || bLen > uint64(n-(pos+bn)) {
					return fmt.Errorf("omnibuf: invalid WireSintArr length at %d", pos)
				}
				pos += bn
				end := pos + int(bLen)
				count, cn := DecodeVarint(data[pos:end])
				if cn == 0 || count > uint64(end-(pos+cn)) {
					return fmt.Errorf("omnibuf: invalid WireSintArr count at %d", pos)
				}
				p := pos + cn
				for range count {
					v, vn := DecodeVarint(data[p:end])
					if vn == 0 {
						return fmt.Errorf("omnibuf: truncated WireSintArr elem at %d", p)
					}
					p += vn
					switch fd.Kind() {
					case protoreflect.Int32Kind, protoreflect.Sint32Kind:
						list.Append(protoreflect.ValueOfInt32(int32(DecodeZigZag(v))))
					case protoreflect.Int64Kind, protoreflect.Sint64Kind:
						list.Append(protoreflect.ValueOfInt64(DecodeZigZag(v)))
					}
				}
				if p != end {
					return fmt.Errorf("omnibuf: WireSintArr byteLen mismatch at %d", p)
				}
				pos = end
			case protoreflect.FloatKind, protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind:
				if wt != WireFixedArr {
					nextPos, err := SkipValue(data, pos, wt)
					if err != nil {
						return err
					}
					pos = nextPos
					continue
				}
				bLen, bn := DecodeVarint(data[pos:])
				if bn == 0 || bLen < 1 || bLen > uint64(n-(pos+bn)) || (bLen-1)%4 != 0 {
					return fmt.Errorf("omnibuf: invalid WireFixedArr(Fixed32) length at %d", pos)
				}
				pos += bn
				if data[pos] != WireFixed32 {
					return fmt.Errorf("omnibuf: unexpected WireFixedArr elemType %d for fixed32 list at %d", data[pos], pos)
				}
				end := pos + int(bLen)
				for p := pos + 1; p < end; p += 4 {
					bits := binary.LittleEndian.Uint32(data[p : p+4])
					switch fd.Kind() {
					case protoreflect.FloatKind:
						list.Append(protoreflect.ValueOfFloat32(math.Float32frombits(bits)))
					case protoreflect.Fixed32Kind:
						list.Append(protoreflect.ValueOfUint32(bits))
					case protoreflect.Sfixed32Kind:
						list.Append(protoreflect.ValueOfInt32(int32(bits)))
					}
				}
				pos = end
			case protoreflect.DoubleKind, protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind:
				if wt != WireFixedArr {
					nextPos, err := SkipValue(data, pos, wt)
					if err != nil {
						return err
					}
					pos = nextPos
					continue
				}
				bLen, bn := DecodeVarint(data[pos:])
				if bn == 0 || bLen < 1 || bLen > uint64(n-(pos+bn)) || (bLen-1)%8 != 0 {
					return fmt.Errorf("omnibuf: invalid WireFixedArr(Fixed64) length at %d", pos)
				}
				pos += bn
				if data[pos] != WireFixed64 {
					return fmt.Errorf("omnibuf: unexpected WireFixedArr elemType %d for fixed64 list at %d", data[pos], pos)
				}
				end := pos + int(bLen)
				for p := pos + 1; p < end; p += 8 {
					bits := binary.LittleEndian.Uint64(data[p : p+8])
					switch fd.Kind() {
					case protoreflect.DoubleKind:
						list.Append(protoreflect.ValueOfFloat64(math.Float64frombits(bits)))
					case protoreflect.Fixed64Kind:
						list.Append(protoreflect.ValueOfUint64(bits))
					case protoreflect.Sfixed64Kind:
						list.Append(protoreflect.ValueOfInt64(int64(bits)))
					}
				}
				pos = end
			case protoreflect.MessageKind:
				if wt != WireSubMsgArr {
					nextPos, err := SkipValue(data, pos, wt)
					if err != nil {
						return err
					}
					pos = nextPos
					continue
				}
				arrLen, an := DecodeVarint(data[pos:])
				if an == 0 || arrLen > uint64(n-(pos+an)) {
					return fmt.Errorf("omnibuf: invalid WireSubMsgArr length at %d", pos)
				}
				pos += an
				arrPayload := data[pos : pos+int(arrLen)]
				elemCount, ok := CountSubMsgArrElems(arrPayload)
				if !ok {
					return fmt.Errorf("omnibuf: corrupt WireSubMsgArr at %d", pos)
				}
				p := 0
				for range elemCount {
					el, en := DecodeVarint(arrPayload[p:])
					p += en
					elemMsg := list.NewElement().Message()
					if err := decodeProtoReflectMsg(arrPayload[p:p+int(el)], elemMsg, depth+1); err != nil {
						return err
					}
					list.Append(protoreflect.ValueOfMessage(elemMsg))
					p += int(el)
				}
				pos += int(arrLen)
			}
			continue
		}

		// Singular field:
		switch fd.Kind() {
		case protoreflect.Uint32Kind:
			if wt != WireUint {
				nextPos, err := SkipValue(data, pos, wt)
				if err != nil {
					return err
				}
				pos = nextPos
				continue
			}
			v, vn := DecodeVarint(data[pos:])
			if vn == 0 {
				return fmt.Errorf("omnibuf: truncated uint32 at %d", pos)
			}
			pos += vn
			m.Set(fd, protoreflect.ValueOfUint32(uint32(v)))
		case protoreflect.Uint64Kind:
			if wt != WireUint {
				nextPos, err := SkipValue(data, pos, wt)
				if err != nil {
					return err
				}
				pos = nextPos
				continue
			}
			v, vn := DecodeVarint(data[pos:])
			if vn == 0 {
				return fmt.Errorf("omnibuf: truncated uint64 at %d", pos)
			}
			pos += vn
			m.Set(fd, protoreflect.ValueOfUint64(v))
		case protoreflect.EnumKind:
			if wt != WireUint {
				nextPos, err := SkipValue(data, pos, wt)
				if err != nil {
					return err
				}
				pos = nextPos
				continue
			}
			v, vn := DecodeVarint(data[pos:])
			if vn == 0 {
				return fmt.Errorf("omnibuf: truncated enum at %d", pos)
			}
			pos += vn
			m.Set(fd, protoreflect.ValueOfEnum(protoreflect.EnumNumber(v)))
		case protoreflect.Int32Kind, protoreflect.Sint32Kind:
			if wt != WireSint {
				nextPos, err := SkipValue(data, pos, wt)
				if err != nil {
					return err
				}
				pos = nextPos
				continue
			}
			v, vn := DecodeVarint(data[pos:])
			if vn == 0 {
				return fmt.Errorf("omnibuf: truncated int32 at %d", pos)
			}
			pos += vn
			m.Set(fd, protoreflect.ValueOfInt32(int32(DecodeZigZag(v))))
		case protoreflect.Int64Kind, protoreflect.Sint64Kind:
			if wt != WireSint {
				nextPos, err := SkipValue(data, pos, wt)
				if err != nil {
					return err
				}
				pos = nextPos
				continue
			}
			v, vn := DecodeVarint(data[pos:])
			if vn == 0 {
				return fmt.Errorf("omnibuf: truncated int64 at %d", pos)
			}
			pos += vn
			m.Set(fd, protoreflect.ValueOfInt64(DecodeZigZag(v)))
		case protoreflect.BoolKind:
			if wt != WireBoolFalse && wt != WireBoolTrue {
				nextPos, err := SkipValue(data, pos, wt)
				if err != nil {
					return err
				}
				pos = nextPos
				continue
			}
			m.Set(fd, protoreflect.ValueOfBool(wt == WireBoolTrue))
		case protoreflect.StringKind:
			if wt != WireString {
				nextPos, err := SkipValue(data, pos, wt)
				if err != nil {
					return err
				}
				pos = nextPos
				continue
			}
			sLen, sn := DecodeVarint(data[pos:])
			if sn == 0 || sLen > uint64(n-(pos+sn)) {
				return fmt.Errorf("omnibuf: invalid string length at %d", pos)
			}
			pos += sn
			m.Set(fd, protoreflect.ValueOfString(string(data[pos:pos+int(sLen)])))
			pos += int(sLen)
		case protoreflect.BytesKind:
			if wt != WireBytes {
				nextPos, err := SkipValue(data, pos, wt)
				if err != nil {
					return err
				}
				pos = nextPos
				continue
			}
			bLen, bn := DecodeVarint(data[pos:])
			if bn == 0 || bLen > uint64(n-(pos+bn)) {
				return fmt.Errorf("omnibuf: invalid bytes length at %d", pos)
			}
			pos += bn
			cp := make([]byte, int(bLen))
			copy(cp, data[pos:pos+int(bLen)])
			m.Set(fd, protoreflect.ValueOfBytes(cp))
			pos += int(bLen)
		case protoreflect.FloatKind:
			if wt != WireFixed32 {
				nextPos, err := SkipValue(data, pos, wt)
				if err != nil {
					return err
				}
				pos = nextPos
				continue
			}
			if n-pos < 4 {
				return fmt.Errorf("omnibuf: truncated fixed32 at %d", pos)
			}
			bits := binary.LittleEndian.Uint32(data[pos:])
			pos += 4
			m.Set(fd, protoreflect.ValueOfFloat32(math.Float32frombits(bits)))
		case protoreflect.Fixed32Kind:
			if wt != WireFixed32 {
				nextPos, err := SkipValue(data, pos, wt)
				if err != nil {
					return err
				}
				pos = nextPos
				continue
			}
			if n-pos < 4 {
				return fmt.Errorf("omnibuf: truncated fixed32 at %d", pos)
			}
			bits := binary.LittleEndian.Uint32(data[pos:])
			pos += 4
			m.Set(fd, protoreflect.ValueOfUint32(bits))
		case protoreflect.Sfixed32Kind:
			if wt != WireFixed32 {
				nextPos, err := SkipValue(data, pos, wt)
				if err != nil {
					return err
				}
				pos = nextPos
				continue
			}
			if n-pos < 4 {
				return fmt.Errorf("omnibuf: truncated fixed32 at %d", pos)
			}
			bits := binary.LittleEndian.Uint32(data[pos:])
			pos += 4
			m.Set(fd, protoreflect.ValueOfInt32(int32(bits)))
		case protoreflect.DoubleKind:
			if wt != WireFixed64 {
				nextPos, err := SkipValue(data, pos, wt)
				if err != nil {
					return err
				}
				pos = nextPos
				continue
			}
			if n-pos < 8 {
				return fmt.Errorf("omnibuf: truncated fixed64 at %d", pos)
			}
			bits := binary.LittleEndian.Uint64(data[pos:])
			pos += 8
			m.Set(fd, protoreflect.ValueOfFloat64(math.Float64frombits(bits)))
		case protoreflect.Fixed64Kind:
			if wt != WireFixed64 {
				nextPos, err := SkipValue(data, pos, wt)
				if err != nil {
					return err
				}
				pos = nextPos
				continue
			}
			if n-pos < 8 {
				return fmt.Errorf("omnibuf: truncated fixed64 at %d", pos)
			}
			bits := binary.LittleEndian.Uint64(data[pos:])
			pos += 8
			m.Set(fd, protoreflect.ValueOfUint64(bits))
		case protoreflect.Sfixed64Kind:
			if wt != WireFixed64 {
				nextPos, err := SkipValue(data, pos, wt)
				if err != nil {
					return err
				}
				pos = nextPos
				continue
			}
			if n-pos < 8 {
				return fmt.Errorf("omnibuf: truncated fixed64 at %d", pos)
			}
			bits := binary.LittleEndian.Uint64(data[pos:])
			pos += 8
			m.Set(fd, protoreflect.ValueOfInt64(int64(bits)))
		case protoreflect.MessageKind:
			if wt != WireSubMsg {
				nextPos, err := SkipValue(data, pos, wt)
				if err != nil {
					return err
				}
				pos = nextPos
				continue
			}
			subLen, sn := DecodeVarint(data[pos:])
			if sn == 0 || subLen > uint64(n-(pos+sn)) {
				return fmt.Errorf("omnibuf: invalid submsg length at %d", pos)
			}
			pos += sn
			subMsg := m.Mutable(fd).Message()
			if err := decodeProtoReflectMsg(data[pos:pos+int(subLen)], subMsg, depth+1); err != nil {
				return err
			}
			pos += int(subLen)
		}
	}
	return nil
}

func mapKindWireType(k protoreflect.Kind) (uint8, error) {
	switch k {
	case protoreflect.Uint32Kind, protoreflect.Uint64Kind, protoreflect.EnumKind:
		return WireUint, nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Int64Kind, protoreflect.Sint64Kind:
		return WireSint, nil
	case protoreflect.StringKind:
		return WireString, nil
	case protoreflect.BytesKind:
		return WireBytes, nil
	case protoreflect.MessageKind:
		return WireSubMsg, nil
	case protoreflect.FloatKind, protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind:
		return WireFixed32, nil
	case protoreflect.DoubleKind, protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind:
		return WireFixed64, nil
	case protoreflect.BoolKind:
		return WireBoolTrue, nil
	default:
		return 0, fmt.Errorf("unsupported map kind %v", k)
	}
}

func appendMapEntryKey(dst []byte, kd protoreflect.FieldDescriptor, k protoreflect.MapKey) ([]byte, error) {
	switch kd.Kind() {
	case protoreflect.StringKind:
		s := k.String()
		dst = AppendVarint(dst, uint64(len(s)))
		return append(dst, s...), nil
	case protoreflect.Uint32Kind, protoreflect.Uint64Kind:
		return AppendVarint(dst, k.Uint()), nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Int64Kind, protoreflect.Sint64Kind:
		return AppendVarint(dst, EncodeZigZag(k.Int())), nil
	case protoreflect.Fixed32Kind:
		v := uint32(k.Uint())
		return append(dst, byte(v), byte(v>>8), byte(v>>16), byte(v>>24)), nil
	case protoreflect.Sfixed32Kind:
		v := uint32(int32(k.Int()))
		return append(dst, byte(v), byte(v>>8), byte(v>>16), byte(v>>24)), nil
	case protoreflect.Fixed64Kind:
		v := k.Uint()
		return append(dst,
			byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
			byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56),
		), nil
	case protoreflect.Sfixed64Kind:
		v := uint64(k.Int())
		return append(dst,
			byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
			byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56),
		), nil
	default:
		return nil, fmt.Errorf("omnibuf: unsupported map key kind %v", kd.Kind())
	}
}

func appendMapEntryVal(dst []byte, vd protoreflect.FieldDescriptor, v protoreflect.Value, opts MarshalOptions) ([]byte, error) {
	switch vd.Kind() {
	case protoreflect.Uint32Kind, protoreflect.Uint64Kind:
		return AppendVarint(dst, v.Uint()), nil
	case protoreflect.EnumKind:
		return AppendVarint(dst, uint64(v.Enum())), nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Int64Kind, protoreflect.Sint64Kind:
		return AppendVarint(dst, EncodeZigZag(v.Int())), nil
	case protoreflect.StringKind:
		s := v.String()
		dst = AppendVarint(dst, uint64(len(s)))
		return append(dst, s...), nil
	case protoreflect.BytesKind:
		b := v.Bytes()
		dst = AppendVarint(dst, uint64(len(b)))
		return append(dst, b...), nil
	case protoreflect.FloatKind:
		bits := math.Float32bits(float32(v.Float()))
		return append(dst, byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24)), nil
	case protoreflect.Fixed32Kind:
		bits := uint32(v.Uint())
		return append(dst, byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24)), nil
	case protoreflect.Sfixed32Kind:
		bits := uint32(int32(v.Int()))
		return append(dst, byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24)), nil
	case protoreflect.DoubleKind:
		bits := math.Float64bits(v.Float())
		return append(dst,
			byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24),
			byte(bits>>32), byte(bits>>40), byte(bits>>48), byte(bits>>56),
		), nil
	case protoreflect.Fixed64Kind:
		bits := v.Uint()
		return append(dst,
			byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24),
			byte(bits>>32), byte(bits>>40), byte(bits>>48), byte(bits>>56),
		), nil
	case protoreflect.Sfixed64Kind:
		bits := uint64(v.Int())
		return append(dst,
			byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24),
			byte(bits>>32), byte(bits>>40), byte(bits>>48), byte(bits>>56),
		), nil
	case protoreflect.BoolKind:
		if v.Bool() {
			return append(dst, 1), nil
		}
		return append(dst, 0), nil
	case protoreflect.MessageKind:
		lenPos := len(dst)
		dst = append(dst, 0, 0)
		var err error
		dst, err = encodeProtoReflectMsg(dst, v.Message(), opts)
		if err != nil {
			return nil, err
		}
		return BackpatchContainerLen(dst, lenPos), nil
	default:
		return nil, fmt.Errorf("omnibuf: unsupported map value kind %v", vd.Kind())
	}
}

func decodeMapEntryKey(data []byte, p, end int, kd protoreflect.FieldDescriptor) (protoreflect.MapKey, int, error) {
	switch kd.Kind() {
	case protoreflect.StringKind:
		kLen, kn := DecodeVarint(data[p:end])
		if kn == 0 || kLen > uint64(end-(p+kn)) {
			return protoreflect.MapKey{}, 0, fmt.Errorf("omnibuf: truncated WireMap key at %d", p)
		}
		p += kn
		return protoreflect.ValueOfString(string(data[p : p+int(kLen)])).MapKey(), p + int(kLen), nil
	case protoreflect.Uint32Kind:
		v, vn := DecodeVarint(data[p:end])
		if vn == 0 {
			return protoreflect.MapKey{}, 0, fmt.Errorf("omnibuf: truncated WireMap uint32 key at %d", p)
		}
		return protoreflect.ValueOfUint32(uint32(v)).MapKey(), p + vn, nil
	case protoreflect.Uint64Kind:
		v, vn := DecodeVarint(data[p:end])
		if vn == 0 {
			return protoreflect.MapKey{}, 0, fmt.Errorf("omnibuf: truncated WireMap uint64 key at %d", p)
		}
		return protoreflect.ValueOfUint64(v).MapKey(), p + vn, nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind:
		v, vn := DecodeVarint(data[p:end])
		if vn == 0 {
			return protoreflect.MapKey{}, 0, fmt.Errorf("omnibuf: truncated WireMap int32 key at %d", p)
		}
		return protoreflect.ValueOfInt32(int32(DecodeZigZag(v))).MapKey(), p + vn, nil
	case protoreflect.Int64Kind, protoreflect.Sint64Kind:
		v, vn := DecodeVarint(data[p:end])
		if vn == 0 {
			return protoreflect.MapKey{}, 0, fmt.Errorf("omnibuf: truncated WireMap int64 key at %d", p)
		}
		return protoreflect.ValueOfInt64(DecodeZigZag(v)).MapKey(), p + vn, nil
	case protoreflect.Fixed32Kind:
		if end-p < 4 {
			return protoreflect.MapKey{}, 0, fmt.Errorf("omnibuf: truncated WireMap fixed32 key at %d", p)
		}
		return protoreflect.ValueOfUint32(binary.LittleEndian.Uint32(data[p : p+4])).MapKey(), p + 4, nil
	case protoreflect.Sfixed32Kind:
		if end-p < 4 {
			return protoreflect.MapKey{}, 0, fmt.Errorf("omnibuf: truncated WireMap sfixed32 key at %d", p)
		}
		return protoreflect.ValueOfInt32(int32(binary.LittleEndian.Uint32(data[p : p+4]))).MapKey(), p + 4, nil
	case protoreflect.Fixed64Kind:
		if end-p < 8 {
			return protoreflect.MapKey{}, 0, fmt.Errorf("omnibuf: truncated WireMap fixed64 key at %d", p)
		}
		return protoreflect.ValueOfUint64(binary.LittleEndian.Uint64(data[p : p+8])).MapKey(), p + 8, nil
	case protoreflect.Sfixed64Kind:
		if end-p < 8 {
			return protoreflect.MapKey{}, 0, fmt.Errorf("omnibuf: truncated WireMap sfixed64 key at %d", p)
		}
		return protoreflect.ValueOfInt64(int64(binary.LittleEndian.Uint64(data[p : p+8]))).MapKey(), p + 8, nil
	default:
		return protoreflect.MapKey{}, 0, fmt.Errorf("omnibuf: unsupported WireMap key kind %v", kd.Kind())
	}
}

func decodeMapEntryVal(data []byte, p, end int, vd protoreflect.FieldDescriptor, mp protoreflect.Map, depth int) (protoreflect.Value, int, error) {
	switch vd.Kind() {
	case protoreflect.Uint32Kind:
		v, vn := DecodeVarint(data[p:end])
		if vn == 0 {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated WireMap uint32 val at %d", p)
		}
		return protoreflect.ValueOfUint32(uint32(v)), p + vn, nil
	case protoreflect.Uint64Kind:
		v, vn := DecodeVarint(data[p:end])
		if vn == 0 {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated WireMap uint64 val at %d", p)
		}
		return protoreflect.ValueOfUint64(v), p + vn, nil
	case protoreflect.EnumKind:
		v, vn := DecodeVarint(data[p:end])
		if vn == 0 {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated WireMap enum val at %d", p)
		}
		return protoreflect.ValueOfEnum(protoreflect.EnumNumber(v)), p + vn, nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind:
		v, vn := DecodeVarint(data[p:end])
		if vn == 0 {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated WireMap int32 val at %d", p)
		}
		return protoreflect.ValueOfInt32(int32(DecodeZigZag(v))), p + vn, nil
	case protoreflect.Int64Kind, protoreflect.Sint64Kind:
		v, vn := DecodeVarint(data[p:end])
		if vn == 0 {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated WireMap int64 val at %d", p)
		}
		return protoreflect.ValueOfInt64(DecodeZigZag(v)), p + vn, nil
	case protoreflect.StringKind:
		vLen, vn := DecodeVarint(data[p:end])
		if vn == 0 || vLen > uint64(end-(p+vn)) {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated WireMap string val at %d", p)
		}
		p += vn
		return protoreflect.ValueOfString(string(data[p : p+int(vLen)])), p + int(vLen), nil
	case protoreflect.BytesKind:
		vLen, vn := DecodeVarint(data[p:end])
		if vn == 0 || vLen > uint64(end-(p+vn)) {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated WireMap bytes val at %d", p)
		}
		p += vn
		cp := make([]byte, int(vLen))
		copy(cp, data[p:p+int(vLen)])
		return protoreflect.ValueOfBytes(cp), p + int(vLen), nil
	case protoreflect.FloatKind:
		if end-p < 4 {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated WireMap float val at %d", p)
		}
		return protoreflect.ValueOfFloat32(math.Float32frombits(binary.LittleEndian.Uint32(data[p : p+4]))), p + 4, nil
	case protoreflect.Fixed32Kind:
		if end-p < 4 {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated WireMap fixed32 val at %d", p)
		}
		return protoreflect.ValueOfUint32(binary.LittleEndian.Uint32(data[p : p+4])), p + 4, nil
	case protoreflect.Sfixed32Kind:
		if end-p < 4 {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated WireMap sfixed32 val at %d", p)
		}
		return protoreflect.ValueOfInt32(int32(binary.LittleEndian.Uint32(data[p : p+4]))), p + 4, nil
	case protoreflect.DoubleKind:
		if end-p < 8 {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated WireMap double val at %d", p)
		}
		return protoreflect.ValueOfFloat64(math.Float64frombits(binary.LittleEndian.Uint64(data[p : p+8]))), p + 8, nil
	case protoreflect.Fixed64Kind:
		if end-p < 8 {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated WireMap fixed64 val at %d", p)
		}
		return protoreflect.ValueOfUint64(binary.LittleEndian.Uint64(data[p : p+8])), p + 8, nil
	case protoreflect.Sfixed64Kind:
		if end-p < 8 {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated WireMap sfixed64 val at %d", p)
		}
		return protoreflect.ValueOfInt64(int64(binary.LittleEndian.Uint64(data[p : p+8]))), p + 8, nil
	case protoreflect.BoolKind:
		if end-p < 1 {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated WireMap bool val at %d", p)
		}
		b := data[p]
		if b != 0 && b != 1 {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: invalid WireMap bool val %d at %d", b, p)
		}
		return protoreflect.ValueOfBool(b == 1), p + 1, nil
	case protoreflect.MessageKind:
		vLen, vn := DecodeVarint(data[p:end])
		if vn == 0 || vLen > uint64(end-(p+vn)) {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated WireMap message val at %d", p)
		}
		p += vn
		subMsg := mp.NewValue().Message()
		if err := decodeProtoReflectMsg(data[p:p+int(vLen)], subMsg, depth+1); err != nil {
			return protoreflect.Value{}, 0, err
		}
		return protoreflect.ValueOfMessage(subMsg), p + int(vLen), nil
	default:
		return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: unsupported WireMap val kind %v", vd.Kind())
	}
}
