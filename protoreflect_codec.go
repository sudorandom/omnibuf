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
	return marshalTableAppend(dst, v, o)
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
		dst = AppendField0SchemaMetaDeterministic(dst, meta, o.Deterministic)
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
		var childMD protoreflect.MessageDescriptor
		if fd.IsMap() && fd.MapValue().Kind() == protoreflect.MessageKind {
			childMD = fd.MapValue().Message()
		} else if !fd.IsMap() && fd.Kind() == protoreflect.MessageKind {
			childMD = fd.Message()
		}
		if child := buildProtoReflectSchemaMeta(childMD, depth+1); child != nil {
			if meta.Children == nil {
				meta.Children = make(map[uint32]*SchemaMeta)
			}
			meta.Children[fnum] = child
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

func protoScalarWireType(k protoreflect.Kind) (uint8, error) {
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
		return 0, fmt.Errorf("omnibuf: unsupported kind %v", k)
	}
}

func protoFixed32Bits(k protoreflect.Kind, v protoreflect.Value) uint32 {
	switch k {
	case protoreflect.FloatKind:
		return math.Float32bits(float32(v.Float()))
	case protoreflect.Fixed32Kind:
		return uint32(v.Uint())
	default:
		return uint32(int32(v.Int()))
	}
}

func protoFixed64Bits(k protoreflect.Kind, v protoreflect.Value) uint64 {
	switch k {
	case protoreflect.DoubleKind:
		return math.Float64bits(v.Float())
	case protoreflect.Fixed64Kind:
		return v.Uint()
	default:
		return uint64(v.Int())
	}
}

func protoValueFromFixed32(k protoreflect.Kind, bits uint32) protoreflect.Value {
	switch k {
	case protoreflect.FloatKind:
		return protoreflect.ValueOfFloat32(math.Float32frombits(bits))
	case protoreflect.Fixed32Kind:
		return protoreflect.ValueOfUint32(bits)
	default:
		return protoreflect.ValueOfInt32(int32(bits))
	}
}

func protoValueFromFixed64(k protoreflect.Kind, bits uint64) protoreflect.Value {
	switch k {
	case protoreflect.DoubleKind:
		return protoreflect.ValueOfFloat64(math.Float64frombits(bits))
	case protoreflect.Fixed64Kind:
		return protoreflect.ValueOfUint64(bits)
	default:
		return protoreflect.ValueOfInt64(int64(bits))
	}
}

func appendProtoScalarPayload(dst []byte, k protoreflect.Kind, v protoreflect.Value) ([]byte, error) {
	switch k {
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
	case protoreflect.FloatKind, protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind:
		bits := protoFixed32Bits(k, v)
		return append(dst, byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24)), nil
	case protoreflect.DoubleKind, protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind:
		bits := protoFixed64Bits(k, v)
		return append(dst,
			byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24),
			byte(bits>>32), byte(bits>>40), byte(bits>>48), byte(bits>>56),
		), nil
	default:
		return nil, fmt.Errorf("omnibuf: unsupported scalar kind %v", k)
	}
}

func decodeProtoScalarPayload(data []byte, pos int, k protoreflect.Kind) (protoreflect.Value, int, error) {
	switch k {
	case protoreflect.Uint32Kind:
		v, vn := DecodeVarint(data[pos:])
		if vn == 0 {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated uint32 at %d", pos)
		}
		return protoreflect.ValueOfUint32(uint32(v)), pos + vn, nil
	case protoreflect.Uint64Kind:
		v, vn := DecodeVarint(data[pos:])
		if vn == 0 {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated uint64 at %d", pos)
		}
		return protoreflect.ValueOfUint64(v), pos + vn, nil
	case protoreflect.EnumKind:
		v, vn := DecodeVarint(data[pos:])
		if vn == 0 {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated enum at %d", pos)
		}
		return protoreflect.ValueOfEnum(protoreflect.EnumNumber(v)), pos + vn, nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind:
		v, vn := DecodeVarint(data[pos:])
		if vn == 0 {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated int32 at %d", pos)
		}
		return protoreflect.ValueOfInt32(int32(DecodeZigZag(v))), pos + vn, nil
	case protoreflect.Int64Kind, protoreflect.Sint64Kind:
		v, vn := DecodeVarint(data[pos:])
		if vn == 0 {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated int64 at %d", pos)
		}
		return protoreflect.ValueOfInt64(DecodeZigZag(v)), pos + vn, nil
	case protoreflect.StringKind:
		s, nextPos, err := readLenSlice(data, pos, 0)
		if err != nil {
			return protoreflect.Value{}, 0, err
		}
		return protoreflect.ValueOfString(string(s)), nextPos, nil
	case protoreflect.BytesKind:
		b, nextPos, err := readLenSlice(data, pos, 0)
		if err != nil {
			return protoreflect.Value{}, 0, err
		}
		cp := make([]byte, len(b))
		copy(cp, b)
		return protoreflect.ValueOfBytes(cp), nextPos, nil
	case protoreflect.FloatKind, protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind:
		if len(data)-pos < 4 {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated fixed32 at %d", pos)
		}
		return protoValueFromFixed32(k, binary.LittleEndian.Uint32(data[pos:pos+4])), pos + 4, nil
	case protoreflect.DoubleKind, protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind:
		if len(data)-pos < 8 {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated fixed64 at %d", pos)
		}
		return protoValueFromFixed64(k, binary.LittleEndian.Uint64(data[pos:pos+8])), pos + 8, nil
	default:
		return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: unsupported scalar kind %v", k)
	}
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
			keyWT, err := protoScalarWireType(fd.MapKey().Kind())
			if err != nil {
				return nil, fmt.Errorf("omnibuf: field %s: %w", fd.Name(), err)
			}
			valWT, err := protoScalarWireType(fd.MapValue().Kind())
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
					dst, err = appendProtoScalarPayload(dst, fd.MapKey().Kind(), k.Value())
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
					dst, mapErr = appendProtoScalarPayload(dst, fd.MapKey().Kind(), k.Value())
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
			case protoreflect.StringKind, protoreflect.BytesKind:
				dst = appendRawTag(dst, fnum, WireStringArr)
				lenPos := len(dst)
				dst = append(dst, 0, 0)
				dst = AppendVarint(dst, uint64(c))
				for j := range c {
					var err error
					dst, err = appendProtoScalarPayload(dst, fd.Kind(), list.Get(j))
					if err != nil {
						return nil, err
					}
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
					bits := protoFixed32Bits(fd.Kind(), list.Get(j))
					dst = append(dst, byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24))
				}
			case protoreflect.DoubleKind, protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind:
				dst = appendRawTag(dst, fnum, WireFixedArr)
				dst = AppendVarint(dst, uint64(1+c*8))
				dst = append(dst, WireFixed64)
				for j := range c {
					bits := protoFixed64Bits(fd.Kind(), list.Get(j))
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
		switch fd.Kind() {
		case protoreflect.MessageKind:
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
		case protoreflect.BoolKind:
			wt := WireBoolFalse
			if val.Bool() {
				wt = WireBoolTrue
			}
			dst = appendRawTag(dst, fnum, wt)
		default:
			wt, err := protoScalarWireType(fd.Kind())
			if err != nil {
				return nil, err
			}
			dst = appendRawTag(dst, fnum, wt)
			dst, err = appendProtoScalarPayload(dst, fd.Kind(), val)
			if err != nil {
				return nil, err
			}
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

func expectedListWireType(k protoreflect.Kind) uint8 {
	switch k {
	case protoreflect.StringKind, protoreflect.BytesKind:
		return WireStringArr
	case protoreflect.Uint32Kind, protoreflect.Uint64Kind, protoreflect.EnumKind, protoreflect.BoolKind:
		return WireUintArr
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Int64Kind, protoreflect.Sint64Kind:
		return WireSintArr
	case protoreflect.FloatKind, protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind,
		protoreflect.DoubleKind, protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind:
		return WireFixedArr
	default:
		return WireSubMsgArr
	}
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
			body, nextPos, err := readLenSlice(data, pos, 2)
			if err != nil {
				return err
			}
			mapType := body[0]
			keyWT := mapType >> 4
			valWT := mapType & 0x0F
			expKeyWT, err := protoScalarWireType(fd.MapKey().Kind())
			if err != nil || keyWT != expKeyWT {
				return fmt.Errorf("omnibuf: WireMap key wire type mismatch at %d", pos)
			}
			expValWT, err := protoScalarWireType(fd.MapValue().Kind())
			if err != nil || valWT != expValWT {
				return fmt.Errorf("omnibuf: WireMap val wire type mismatch at %d", pos)
			}
			count, cn := DecodeVarint(body[1:])
			if cn == 0 || count > uint64(len(body)-(1+cn)) {
				return fmt.Errorf("omnibuf: invalid WireMap count at %d", pos)
			}
			p := 1 + cn
			mp := m.Mutable(fd).Map()
			if !alreadySeen {
				mp.Range(func(k protoreflect.MapKey, _ protoreflect.Value) bool {
					mp.Clear(k)
					return true
				})
			}
			for range int(count) {
				kv, nextP, err := decodeProtoScalarPayload(body, p, fd.MapKey().Kind())
				if err != nil {
					return err
				}
				mv, nextP2, err := decodeMapEntryVal(body, nextP, fd.MapValue(), mp, depth)
				if err != nil {
					return err
				}
				p = nextP2
				mp.Set(kv.MapKey(), mv)
			}
			if p != len(body) {
				return fmt.Errorf("omnibuf: WireMap trailing bytes at %d", p)
			}
			pos = nextPos
			continue
		}

		if fd.IsList() {
			if wt != expectedListWireType(fd.Kind()) {
				nextPos, err := SkipValue(data, pos, wt)
				if err != nil {
					return err
				}
				pos = nextPos
				continue
			}
			list := m.Mutable(fd).List()
			if !alreadySeen {
				list.Truncate(0)
			}
			switch fd.Kind() {
			case protoreflect.StringKind, protoreflect.BytesKind:
				body, nextPos, err := readLenSlice(data, pos, 1)
				if err != nil {
					return err
				}
				count, cn := DecodeVarint(body)
				if cn == 0 || count > uint64(len(body)-cn) {
					return fmt.Errorf("omnibuf: invalid WireStringArr count at %d", pos)
				}
				p := cn
				for range int(count) {
					ev, nextP, err := decodeProtoScalarPayload(body, p, fd.Kind())
					if err != nil {
						return err
					}
					list.Append(ev)
					p = nextP
				}
				if p != len(body) {
					return fmt.Errorf("omnibuf: WireStringArr byteLen mismatch at %d", pos)
				}
				pos = nextPos
			case protoreflect.Uint32Kind, protoreflect.Uint64Kind, protoreflect.EnumKind, protoreflect.BoolKind:
				body, nextPos, err := readLenSlice(data, pos, 1)
				if err != nil {
					return err
				}
				count, cn := DecodeVarint(body)
				if cn == 0 || count > uint64(len(body)-cn) {
					return fmt.Errorf("omnibuf: invalid WireUintArr count at %d", pos)
				}
				p := cn
				for range int(count) {
					v, vn := DecodeVarint(body[p:])
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
				if p != len(body) {
					return fmt.Errorf("omnibuf: WireUintArr byteLen mismatch at %d", pos)
				}
				pos = nextPos
			case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Int64Kind, protoreflect.Sint64Kind:
				body, nextPos, err := readLenSlice(data, pos, 1)
				if err != nil {
					return err
				}
				count, cn := DecodeVarint(body)
				if cn == 0 || count > uint64(len(body)-cn) {
					return fmt.Errorf("omnibuf: invalid WireSintArr count at %d", pos)
				}
				p := cn
				for range int(count) {
					v, vn := DecodeVarint(body[p:])
					if vn == 0 {
						return fmt.Errorf("omnibuf: truncated WireSintArr elem at %d", p)
					}
					p += vn
					if fd.Kind() == protoreflect.Int32Kind || fd.Kind() == protoreflect.Sint32Kind {
						list.Append(protoreflect.ValueOfInt32(int32(DecodeZigZag(v))))
					} else {
						list.Append(protoreflect.ValueOfInt64(DecodeZigZag(v)))
					}
				}
				if p != len(body) {
					return fmt.Errorf("omnibuf: WireSintArr byteLen mismatch at %d", pos)
				}
				pos = nextPos
			case protoreflect.FloatKind, protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind:
				body, nextPos, err := readLenSlice(data, pos, 1)
				if err != nil {
					return err
				}
				if body[0] != WireFixed32 || (len(body)-1)%4 != 0 {
					return fmt.Errorf("omnibuf: invalid WireFixedArr(Fixed32) at %d", pos)
				}
				for p := 1; p < len(body); p += 4 {
					list.Append(protoValueFromFixed32(fd.Kind(), binary.LittleEndian.Uint32(body[p:p+4])))
				}
				pos = nextPos
			case protoreflect.DoubleKind, protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind:
				body, nextPos, err := readLenSlice(data, pos, 1)
				if err != nil {
					return err
				}
				if body[0] != WireFixed64 || (len(body)-1)%8 != 0 {
					return fmt.Errorf("omnibuf: invalid WireFixedArr(Fixed64) at %d", pos)
				}
				for p := 1; p < len(body); p += 8 {
					list.Append(protoValueFromFixed64(fd.Kind(), binary.LittleEndian.Uint64(body[p:p+8])))
				}
				pos = nextPos
			case protoreflect.MessageKind:
				arrPayload, nextPos, err := readLenSlice(data, pos, 0)
				if err != nil {
					return err
				}
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
				pos = nextPos
			}
			continue
		}

		// Singular field:
		if fd.Kind() == protoreflect.BoolKind {
			if wt != WireBoolFalse && wt != WireBoolTrue {
				nextPos, err := SkipValue(data, pos, wt)
				if err != nil {
					return err
				}
				pos = nextPos
				continue
			}
			m.Set(fd, protoreflect.ValueOfBool(wt == WireBoolTrue))
			continue
		}

		expWT, err := protoScalarWireType(fd.Kind())
		if err != nil || wt != expWT {
			nextPos, err := SkipValue(data, pos, wt)
			if err != nil {
				return err
			}
			pos = nextPos
			continue
		}

		if fd.Kind() == protoreflect.MessageKind {
			subBytes, nextPos, err := readLenSlice(data, pos, 0)
			if err != nil {
				return err
			}
			subMsg := m.Mutable(fd).Message()
			if err := decodeProtoReflectMsg(subBytes, subMsg, depth+1); err != nil {
				return err
			}
			pos = nextPos
			continue
		}

		sv, nextPos, err := decodeProtoScalarPayload(data, pos, fd.Kind())
		if err != nil {
			return err
		}
		m.Set(fd, sv)
		pos = nextPos
	}
	return nil
}

func appendMapEntryVal(dst []byte, vd protoreflect.FieldDescriptor, v protoreflect.Value, opts MarshalOptions) ([]byte, error) {
	switch vd.Kind() {
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
		return appendProtoScalarPayload(dst, vd.Kind(), v)
	}
}

func decodeMapEntryVal(body []byte, p int, vd protoreflect.FieldDescriptor, mp protoreflect.Map, depth int) (protoreflect.Value, int, error) {
	switch vd.Kind() {
	case protoreflect.BoolKind:
		if len(body)-p < 1 {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: truncated WireMap bool val at %d", p)
		}
		b := body[p]
		if b != 0 && b != 1 {
			return protoreflect.Value{}, 0, fmt.Errorf("omnibuf: invalid WireMap bool val %d at %d", b, p)
		}
		return protoreflect.ValueOfBool(b == 1), p + 1, nil
	case protoreflect.MessageKind:
		subBytes, nextP, err := readLenSlice(body, p, 0)
		if err != nil {
			return protoreflect.Value{}, 0, err
		}
		subMsg := mp.NewValue().Message()
		if err := decodeProtoReflectMsg(subBytes, subMsg, depth+1); err != nil {
			return protoreflect.Value{}, 0, err
		}
		return protoreflect.ValueOfMessage(subMsg), nextP, nil
	default:
		return decodeProtoScalarPayload(body, p, vd.Kind())
	}
}
