package omnibuf

import (
	"encoding/binary"
	"fmt"
	"maps"
	"math"
	"reflect"
)

// SchemaMeta is the optional inline schema header encoded as Field #0
// (Tag = 0x19, wireSubMsg = 12) at byte 0 of an OmniBuf message (omnibuf.v1.SchemaMeta).
type SchemaMeta struct {
	TypeName string                 `protobuf:"bytes,1,opt,name=type_name,proto3" json:"type_name,omitempty"`
	Fields   map[uint32]string      `protobuf:"bytes,2,rep,name=fields,proto3" json:"fields,omitempty"`
	Children map[uint32]*SchemaMeta `protobuf:"bytes,3,rep,name=children,proto3" json:"children,omitempty"`
}

// AppendField0 appends Field #0 (Tag = 0x19, wireSubMsg = 12) encoding meta to dst.
// When deterministic is true, map keys in Fields and Children are sorted in ascending order.
func (meta *SchemaMeta) AppendField0(dst []byte, deterministic bool) []byte {
	return appendField0SchemaMetaDeterministic(dst, meta, deterministic)
}

func appendField0SchemaMetaDeterministic(dst []byte, meta *SchemaMeta, deterministic bool) []byte {
	if meta == nil {
		return dst
	}
	dst = append(dst, byte(((0<<4)|wireSubMsg)<<1)|1) // Tag = (0 << 4) | wireSubMsg(12) = 12 -> (12 << 1) | 1 = 0x19
	lenPos := len(dst)
	dst = append(dst, 0, 0)
	dst = appendSchemaMetaBody(dst, meta, deterministic)
	return backpatchContainerLen(dst, lenPos)
}

func sortedUint32Keys[V any](m map[uint32]V, stack *[32]uint32) []uint32 {
	keys := stack[:0]
	if len(m) > len(stack) {
		keys = make([]uint32, 0, len(m))
	}
	for k := range m {
		keys = append(keys, k)
	}
	sortUint32Slice(keys)
	return keys
}

func appendSchemaMetaBody(dst []byte, meta *SchemaMeta, deterministic bool) []byte {
	if meta == nil {
		return dst
	}
	// Field #1: type_name (wireString = 10 -> Tag (1 << 4) | 10 = 26 -> 0x35)
	if meta.TypeName != "" {
		dst = append(dst, byte(((1<<4)|wireString)<<1)|1)
		dst = appendVarint(dst, uint64(len(meta.TypeName)))
		dst = append(dst, meta.TypeName...)
	}
	// Field #2: fields map<uint32, string> (wireMap = 14 -> Tag (2 << 4) | 14 = 46 -> 0x5D)
	if len(meta.Fields) > 0 {
		dst = append(dst, byte(((2<<4)|wireMap)<<1)|1)
		lenPos := len(dst)
		dst = append(dst, 0, 0, wireUint, wireString)
		dst = appendVarint(dst, uint64(len(meta.Fields)))
		if deterministic {
			var stackKeys [32]uint32
			for _, k := range sortedUint32Keys(meta.Fields, &stackKeys) {
				name := meta.Fields[k]
				dst = appendVarint(dst, uint64(k))
				dst = appendVarint(dst, uint64(len(name)))
				dst = append(dst, name...)
			}
		} else {
			for k, name := range meta.Fields {
				dst = appendVarint(dst, uint64(k))
				dst = appendVarint(dst, uint64(len(name)))
				dst = append(dst, name...)
			}
		}
		dst = backpatchContainerLen(dst, lenPos)
	}
	// Field #3: children map<uint32, SchemaMeta> (wireMap = 14 -> Tag (3 << 4) | 14 = 62 -> 0x7D)
	if len(meta.Children) > 0 {
		dst = append(dst, byte(((3<<4)|wireMap)<<1)|1)
		lenPos := len(dst)
		dst = append(dst, 0, 0, wireUint, wireSubMsg)
		dst = appendVarint(dst, uint64(len(meta.Children)))
		if deterministic {
			var stackKeys [32]uint32
			for _, k := range sortedUint32Keys(meta.Children, &stackKeys) {
				dst = appendSchemaMetaChildEntry(dst, k, meta.Children[k], deterministic)
			}
		} else {
			for k, child := range meta.Children {
				dst = appendSchemaMetaChildEntry(dst, k, child, deterministic)
			}
		}
		dst = backpatchContainerLen(dst, lenPos)
	}
	return dst
}

func appendSchemaMetaChildEntry(dst []byte, k uint32, child *SchemaMeta, deterministic bool) []byte {
	dst = appendVarint(dst, uint64(k))
	cLenPos := len(dst)
	dst = append(dst, 0, 0)
	dst = appendSchemaMetaBody(dst, child, deterministic)
	return backpatchContainerLen(dst, cLenPos)
}

// ReadField0SchemaMeta decodes and merges all Field #0 (Tag = 0x0B, wireSubMsg = 5)
// SchemaMeta blocks in packet.
func ReadField0SchemaMeta(packet []byte) (*SchemaMeta, bool) {
	var merged *SchemaMeta
	for pos := 0; ; {
		body, nextPos, matched, valid := nextContainerField(packet, pos, 0, wireSubMsg, 0)
		if !valid {
			return nil, false
		}
		if !matched {
			return merged, merged != nil
		}
		pos = nextPos
		m, err := decodeSchemaMeta(body, 0)
		if err != nil {
			return nil, false
		}
		merged = mergeSchemaMeta(merged, m)
	}
}

func decodeSchemaMeta(data []byte, depth int) (*SchemaMeta, error) {
	if depth > MaxNestingDepth {
		return nil, fmt.Errorf("omnibuf: SchemaMeta nesting depth %d exceeds MaxNestingDepth (%d)", depth, MaxNestingDepth)
	}
	meta := &SchemaMeta{}
	pos := 0
	n := len(data)
	for pos < n {
		tag, tn := decodeVarint(data[pos:])
		if tn == 0 {
			return nil, fmt.Errorf("omnibuf: truncated tag in Field #0 SchemaMeta at %d", pos)
		}
		pos += tn
		fnum := tag >> 4
		wt := uint8(tag & 0x0F)
		switch fnum {
		case 1: // type_name (string)
			if wt != wireString {
				return nil, fmt.Errorf("omnibuf: SchemaMeta field #1 (type_name) must be wireString, got %d", wt)
			}
			s, nextPos, err := readStrSlice(data, pos)
			if err != nil {
				return nil, err
			}
			meta.TypeName = string(s)
			pos = nextPos
		case 2: // field names
			if wt != wireMap {
				return nil, fmt.Errorf("omnibuf: SchemaMeta field #2 (fields) must be wireMap, got %d", wt)
			}
			body, nextPos, err := readLenSlice(data, pos, 3)
			if err != nil {
				return nil, err
			}
			pos = nextPos
			keyWT, valWT, count, p, ok := parseMapHeader(body)
			if !ok || keyWT != wireUint || valWT != wireString {
				return nil, fmt.Errorf("omnibuf: invalid SchemaMeta fields map header")
			}
			if meta.Fields == nil {
				meta.Fields = make(map[uint32]string, count)
			}

			for range count {
				k64, kn := decodeVarint(body[p:])
				if kn == 0 || k64 == 0 || k64 > math.MaxUint32 {
					return nil, fmt.Errorf("omnibuf: invalid SchemaMeta field number %d at %d", k64, p)
				}
				p += kn
				nameBytes, nextP, err := readStrSlice(body, p)
				if err != nil {
					return nil, err
				}
				meta.Fields[uint32(k64)] = string(nameBytes)
				p = nextP
			}
			if p != len(body) {
				return nil, fmt.Errorf("omnibuf: SchemaMeta fields map trailing bytes (parsed %d, body %d)", p, len(body))
			}
		case 3: // children (map<uint32, SchemaMeta>)
			if wt != wireMap {
				return nil, fmt.Errorf("omnibuf: SchemaMeta field #3 (children) must be wireMap, got %d", wt)
			}
			body, nextPos, err := readLenSlice(data, pos, 3)
			if err != nil {
				return nil, err
			}
			pos = nextPos
			keyWT, valWT, count, p, ok := parseMapHeader(body)
			if !ok || keyWT != wireUint || valWT != wireSubMsg {
				return nil, fmt.Errorf("omnibuf: invalid SchemaMeta children map header")
			}
			if meta.Children == nil {
				meta.Children = make(map[uint32]*SchemaMeta, count)
			}
			for range count {
				k64, kn := decodeVarint(body[p:])
				if kn == 0 || k64 == 0 || k64 > math.MaxUint32 {
					return nil, fmt.Errorf("omnibuf: invalid SchemaMeta child field number %d at %d", k64, p)
				}
				p += kn
				childBytes, nextP, err := readLenSlice(body, p, 0)
				if err != nil {
					return nil, err
				}
				childMeta, err := decodeSchemaMeta(childBytes, depth+1)
				if err != nil {
					return nil, err
				}
				meta.Children[uint32(k64)] = mergeSchemaMeta(meta.Children[uint32(k64)], childMeta)
				p = nextP
			}
			if p != len(body) {
				return nil, fmt.Errorf("omnibuf: SchemaMeta children map trailing bytes (parsed %d, body %d)", p, len(body))
			}
		default:
			if fnum == 0 {
				return nil, fmt.Errorf("omnibuf: invalid field #0 inside SchemaMeta")
			}
			nextPos, err := skipValue(data, pos, wt)
			if err != nil {
				return nil, err
			}
			pos = nextPos
		}
	}
	return meta, nil
}

func mergeSchemaMeta(base, next *SchemaMeta) *SchemaMeta {
	if next == nil {
		return base
	}
	if base == nil {
		cp := &SchemaMeta{TypeName: next.TypeName}
		if len(next.Fields) > 0 {
			cp.Fields = make(map[uint32]string, len(next.Fields))
			maps.Copy(cp.Fields, next.Fields)
		}
		if len(next.Children) > 0 {
			cp.Children = make(map[uint32]*SchemaMeta, len(next.Children))
			for k, c := range next.Children {
				cp.Children[k] = mergeSchemaMeta(nil, c)
			}
		}
		return cp
	}
	if next.TypeName != "" {
		base.TypeName = next.TypeName
	}
	if len(next.Fields) > 0 {
		if base.Fields == nil {
			base.Fields = make(map[uint32]string, len(next.Fields))
		}
		maps.Copy(base.Fields, next.Fields)
	}
	if len(next.Children) > 0 {
		if base.Children == nil {
			base.Children = make(map[uint32]*SchemaMeta, len(next.Children))
		}
		for k, c := range next.Children {
			base.Children[k] = mergeSchemaMeta(base.Children[k], c)
		}
	}
	return base
}

// blindSchema overlays message-local metadata on immutable inherited metadata.
// One layer is added per message, not per Field #0 block, so lookup work is
// bounded by MaxNestingDepth rather than the number or width of schema blocks.
type blindSchema struct {
	local  *SchemaMeta
	parent *blindSchema
}

func (s *blindSchema) fieldName(field uint32) string {
	for ; s != nil; s = s.parent {
		if name, ok := s.local.Fields[field]; ok {
			return name
		}
	}
	return ""
}

func (s *blindSchema) child(field uint32) *blindSchema {
	if s == nil {
		return nil
	}
	parent := s.parent.child(field)
	if child := s.local.Children[field]; child != nil {
		return &blindSchema{local: child, parent: parent}
	}
	return parent
}

// DecodeBlind decodes any OmniBuf packet without a compile-time schema.
// If Field #0 (Tag = 0x0B, wireSubMsg = 5, omnibuf.v1.SchemaMeta) is present, its field names
// (and any nested children schemas) are used to decode human-readable keys.
// Otherwise, synthetic field names ("field_1", "field_2", ...) are used.
func DecodeBlind(packet []byte) (map[string]any, error) {
	return decodeBlindWithSchema(packet, nil, 0)
}

func decodeBlindWithSchema(data []byte, inherited *blindSchema, depth int) (map[string]any, error) {
	if depth > MaxNestingDepth {
		return nil, fmt.Errorf("omnibuf: nesting depth %d exceeds MaxNestingDepth (%d)", depth, MaxNestingDepth)
	}
	n := len(data)
	if n == 0 {
		return map[string]any{}, nil
	}

	var localSchema *SchemaMeta

	// Resolve metadata before values so late Field #0 blocks and concatenated
	// schemas give every occurrence of a numeric field the same name and type.
	for scan := 0; scan < n; {
		tag, tn := decodeVarint(data[scan:])
		if tn == 0 || tag>>4 > (1<<29)-1 {
			return nil, fmt.Errorf("omnibuf: invalid tag at %d", scan)
		}
		scan += tn
		wt := uint8(tag & 15)
		if tag>>4 == 0 {
			if wt != wireSubMsg {
				return nil, fmt.Errorf("omnibuf: invalid Field #0 wire type %d", wt)
			}
			body, next, err := readLenSlice(data, scan, 0)
			if err != nil {
				return nil, err
			}
			meta, err := decodeSchemaMeta(body, depth+1)
			if err != nil {
				return nil, err
			}
			localSchema = mergeSchemaMeta(localSchema, meta)
			scan = next
		} else {
			next, err := skipValue(data, scan, wt)
			if err != nil {
				return nil, err
			}
			scan = next
		}
	}
	activeSchema := inherited
	if localSchema != nil {
		activeSchema = &blindSchema{local: localSchema, parent: inherited}
	}
	pos := 0
	out := make(map[string]any, 8)
	subPackets := make(map[string][]byte)
	subSchemas := make(map[string]*blindSchema)
	// shapes records each field's wire shape (wire type plus array element
	// type or map key/value types). Repeated occurrences of a field number
	// (e.g. from concatenation) MUST agree; changing a field's value type is
	// not a compatible change, so a mismatch is rejected instead of merged.
	shapes := make(map[int]uint32, 8)
	owners := make(map[string]int, 8)

	for pos < n {
		tag, tn := decodeVarint(data[pos:])
		if tn == 0 {
			return nil, fmt.Errorf("omnibuf: truncated tag at %d", pos)
		}
		pos += tn
		fieldNum := int(tag >> 4)
		wt := uint8(tag & 0x0F)

		if fieldNum == 0 {
			if wt != wireSubMsg {
				return nil, fmt.Errorf("omnibuf: Field #0 must have wireSubMsg wire type (5), got %d", wt)
			}
			_, nextPos, err := readLenSlice(data, pos, 0)
			if err != nil {
				return nil, err
			}
			pos = nextPos

			continue
		}

		name := activeSchema.fieldName(uint32(fieldNum))
		if name == "" {
			name = fmt.Sprintf("field_%d", fieldNum)
		}

		if owner, ok := owners[name]; ok && owner != fieldNum {
			return nil, fmt.Errorf("omnibuf: fields #%d and #%d both decode to name %q", owner, fieldNum, name)
		}
		owners[name] = fieldNum

		shape, err := blindFieldShape(data, pos, wt)
		if err != nil {
			return nil, err
		}
		if prev, ok := shapes[fieldNum]; ok && prev != shape {
			return nil, fmt.Errorf("omnibuf: field #%d changes wire shape within one message (0x%x -> 0x%x)", fieldNum, prev, shape)
		}
		shapes[fieldNum] = shape

		childSchema := activeSchema.child(uint32(fieldNum))

		switch wt {
		case wireUint:
			v, vn := decodeVarint(data[pos:])
			if vn == 0 {
				return nil, fmt.Errorf("omnibuf: truncated wireUint at %d", pos)
			}
			pos += vn
			out[name] = v
		case wireSint:
			v, vn := decodeVarint(data[pos:])
			if vn == 0 {
				return nil, fmt.Errorf("omnibuf: truncated wireSint at %d", pos)
			}
			pos += vn
			out[name] = decodeZigZag(v)
		case wireBoolFalse:
			out[name] = false
		case wireBoolTrue:
			out[name] = true
		case wireString:
			s, nextPos, err := readStrSlice(data, pos)
			if err != nil {
				return nil, err
			}
			out[name] = string(s)
			pos = nextPos
		case wireBytes:
			b, nextPos, err := readLenSlice(data, pos, 0)
			if err != nil {
				return nil, err
			}
			cp := make([]byte, len(b))
			copy(cp, b)
			out[name] = cp
			pos = nextPos
		case wireSubMsg:
			subBytes, nextPos, err := readLenSlice(data, pos, 0)
			if err != nil {
				return nil, err
			}
			subPackets[name] = append(subPackets[name], subBytes...)
			subSchemas[name] = childSchema

			pos = nextPos
		case wireArray:
			body, next, err := readLenSlice(data, pos, 2)
			if err != nil {
				return nil, err
			}
			arr, err := decodeArrayBlind(body, childSchema, depth+1)
			if err != nil {
				return nil, err
			}
			if prev, ok := out[name]; ok {
				a, b := reflect.ValueOf(prev), reflect.ValueOf(arr)
				if a.Type() != b.Type() {
					return nil, fmt.Errorf("omnibuf: array element type changed")
				}
				arr = reflect.AppendSlice(a, b).Interface()
			}
			out[name] = arr
			pos = next
		case wireExtended:
			return nil, fmt.Errorf("omnibuf: unknown extension")
		case wireMap:
			body, nextPos, err := readLenSlice(data, pos, 3)
			if err != nil {
				return nil, err
			}
			if depth+1 > MaxNestingDepth {
				return nil, fmt.Errorf("omnibuf: map nesting limit exceeded")
			}
			keyWT, valWT, count, p, ok := parseMapHeader(body)
			if !ok {
				return nil, fmt.Errorf("omnibuf: invalid wireMap header at %d", pos)
			}
			if keyWT == wireString && valWT == wireUint {
				prev, _ := out[name].(map[string]uint64)
				m, ok := decodeMapStringUintBody(prev, body, false)
				if !ok {
					return nil, fmt.Errorf("omnibuf: invalid wireMap(string, uint) at %d", pos)
				}
				out[name] = m
			} else {
				m, ok := out[name].(map[string]any)
				if !ok || m == nil {
					m = make(map[string]any, count)
				}
				for range count {
					k, nextP, err := decodeMapKeyBlind(body, p, keyWT)
					if err != nil {
						return nil, err
					}
					v, nextP2, err := decodeMapValBlind(body, nextP, valWT, childSchema, depth+1)
					if err != nil {
						return nil, err
					}
					p = nextP2
					m[k] = v
				}
				if p != len(body) {
					return nil, fmt.Errorf("omnibuf: wireMap trailing bytes (parsed %d, body %d)", p, len(body))
				}
				out[name] = m
			}
			pos = nextPos
		case wireFixed32, wireFloat32, wireSfixed32:
			if n-pos < 4 {
				return nil, fmt.Errorf("omnibuf: truncated wireFixed32 at %d", pos)
			}
			bits := binary.LittleEndian.Uint32(data[pos : pos+4])
			switch wt {
			case wireFloat32:
				out[name] = math.Float32frombits(bits)
			case wireSfixed32:
				out[name] = int32(bits)
			default:
				out[name] = bits
			}
			pos += 4
		case wireFixed64, wireFloat64, wireSfixed64:
			if n-pos < 8 {
				return nil, fmt.Errorf("omnibuf: truncated wireFixed64 at %d", pos)
			}
			bits := binary.LittleEndian.Uint64(data[pos : pos+8])
			switch wt {
			case wireFloat64:
				out[name] = math.Float64frombits(bits)
			case wireSfixed64:
				out[name] = int64(bits)
			default:
				out[name] = bits
			}
			pos += 8
		default:
			return nil, fmt.Errorf("omnibuf: unsupported wire type %d", wt)
		}
	}

	for name, packet := range subPackets {
		value, err := decodeBlindWithSchema(packet, subSchemas[name], depth+1)
		if err != nil {
			return nil, err
		}
		out[name] = value
	}

	return out, nil
}

// blindFieldShape returns a comparable signature of a field's wire shape.
// WireBoolFalse and WireBoolTrue share a shape since both encode a bool.
func blindFieldShape(data []byte, pos int, wt uint8) (uint32, error) {
	switch wt {
	case wireBoolTrue:
		return uint32(wireBoolFalse), nil
	case wireArray, wireMap:
		minLen := uint64(2)
		if wt == wireMap {
			minLen = 3
		}
		body, _, err := readLenSlice(data, pos, minLen)
		if err != nil {
			return 0, err
		}
		shape := uint32(wt) | uint32(body[0])<<8
		if wt == wireMap {
			shape |= uint32(body[1]) << 16
		}
		return shape, nil
	default:
		return uint32(wt), nil
	}
}

func decodeMapKeyBlind(body []byte, p int, keyWT uint8) (string, int, error) {
	switch keyWT {
	case wireString:
		kBytes, nextP, err := readStrSlice(body, p)
		if err != nil {
			return "", 0, err
		}
		return string(kBytes), nextP, nil
	case wireUint:
		v, vn := decodeVarint(body[p:])
		if vn == 0 {
			return "", 0, fmt.Errorf("omnibuf: truncated wireMap uint key at %d", p)
		}
		return fmt.Sprintf("%d", v), p + vn, nil
	case wireSint:
		v, vn := decodeVarint(body[p:])
		if vn == 0 {
			return "", 0, fmt.Errorf("omnibuf: truncated wireMap sint key at %d", p)
		}
		return fmt.Sprintf("%d", decodeZigZag(v)), p + vn, nil
	case wireFixed32, wireSfixed32:
		if len(body)-p < 4 {
			return "", 0, fmt.Errorf("omnibuf: truncated wireMap fixed32 key at %d", p)
		}
		bits := binary.LittleEndian.Uint32(body[p : p+4])
		if keyWT == wireSfixed32 {
			return fmt.Sprint(int32(bits)), p + 4, nil
		}
		return fmt.Sprint(bits), p + 4, nil
	case wireFixed64, wireSfixed64:
		if len(body)-p < 8 {
			return "", 0, fmt.Errorf("omnibuf: truncated wireMap fixed64 key at %d", p)
		}
		bits := binary.LittleEndian.Uint64(body[p : p+8])
		if keyWT == wireSfixed64 {
			return fmt.Sprint(int64(bits)), p + 8, nil
		}
		return fmt.Sprint(bits), p + 8, nil
	case wireBoolFalse:
		if len(body)-p < 1 {
			return "", 0, fmt.Errorf("omnibuf: truncated wireMap bool key at %d", p)
		}
		b := body[p]
		if b != 0 && b != 1 {
			return "", 0, fmt.Errorf("omnibuf: invalid wireMap bool key %d at %d", b, p)
		}
		if b == 1 {
			return "true", p + 1, nil
		}
		return "false", p + 1, nil
	default:
		return "", 0, fmt.Errorf("omnibuf: invalid wireMap key wire type %d", keyWT)
	}
}

func decodeMapValBlind(body []byte, p int, valWT uint8, childSchema *blindSchema, depth int) (any, int, error) {
	switch valWT {
	case wireUint:
		v, vn := decodeVarint(body[p:])
		if vn == 0 {
			return nil, 0, fmt.Errorf("omnibuf: truncated wireMap uint value at %d", p)
		}
		return v, p + vn, nil
	case wireSint:
		v, vn := decodeVarint(body[p:])
		if vn == 0 {
			return nil, 0, fmt.Errorf("omnibuf: truncated wireMap sint value at %d", p)
		}
		return decodeZigZag(v), p + vn, nil
	case wireString:
		vBytes, nextP, err := readStrSlice(body, p)
		if err != nil {
			return nil, 0, err
		}
		return string(vBytes), nextP, nil
	case wireBytes:
		vBytes, nextP, err := readLenSlice(body, p, 0)
		if err != nil {
			return nil, 0, err
		}
		cp := make([]byte, len(vBytes))
		copy(cp, vBytes)
		return cp, nextP, nil
	case wireSubMsg:
		vBytes, nextP, err := readLenSlice(body, p, 0)
		if err != nil {
			return nil, 0, err
		}
		subMap, err := decodeBlindWithSchema(vBytes, childSchema, depth+1)
		if err != nil {
			return nil, 0, err
		}
		return subMap, nextP, nil
	case wireArray:
		nested, nextP, err := readLenSlice(body, p, 2)
		if err != nil {
			return nil, 0, err
		}
		arr, err := decodeArrayBlind(nested, childSchema, depth+1)
		if err != nil {
			return nil, 0, err
		}
		return arr, nextP, nil
	case wireMap:
		nested, nextP, err := readLenSlice(body, p, 3)
		if err != nil {
			return nil, 0, err
		}
		mp, err := decodeMapBodyBlind(nested, childSchema, depth+1)
		if err != nil {
			return nil, 0, err
		}
		return mp, nextP, nil
	case wireFixed32, wireFloat32, wireSfixed32:
		if len(body)-p < 4 {
			return nil, 0, fmt.Errorf("omnibuf: truncated wireMap fixed32 value at %d", p)
		}
		bits := binary.LittleEndian.Uint32(body[p : p+4])
		switch valWT {
		case wireFloat32:
			return math.Float32frombits(bits), p + 4, nil
		case wireSfixed32:
			return int32(bits), p + 4, nil
		default:
			return bits, p + 4, nil
		}
	case wireFixed64, wireFloat64, wireSfixed64:
		if len(body)-p < 8 {
			return nil, 0, fmt.Errorf("omnibuf: truncated wireMap fixed64 value at %d", p)
		}
		bits := binary.LittleEndian.Uint64(body[p : p+8])
		switch valWT {
		case wireFloat64:
			return math.Float64frombits(bits), p + 8, nil
		case wireSfixed64:
			return int64(bits), p + 8, nil
		default:
			return bits, p + 8, nil
		}
	case wireBoolFalse:
		if len(body)-p < 1 {
			return nil, 0, fmt.Errorf("omnibuf: truncated wireMap bool value at %d", p)
		}
		b := body[p]
		if b != 0 && b != 1 {
			return nil, 0, fmt.Errorf("omnibuf: invalid wireMap bool value %d at %d", b, p)
		}
		return b == 1, p + 1, nil
	default:
		return nil, 0, fmt.Errorf("omnibuf: invalid wireMap value wire type %d", valWT)
	}
}

// ValidateField0 validates the Field #0 SchemaMeta payload that starts at
// data[pos] (immediately after a Field #0 WireSubMsg tag) at the given nesting
// depth and returns the position after it. It is used by generated decoders so
// that Field #0 is validated in place rather than by rescanning the packet.
func ValidateField0(data []byte, pos, depth int) (int, error) {
	if pos < 0 || pos > len(data) {
		return 0, fmt.Errorf("omnibuf: Field #0 position %d out of bounds", pos)
	}
	return validateField0(data, pos, wireSubMsg, depth)
}

// validateField0 validates metadata even in schema-aware decoders.
func validateField0(data []byte, pos int, wt uint8, depth int) (int, error) {
	if wt != wireSubMsg {
		return 0, fmt.Errorf("omnibuf: invalid Field #0 wire type %d", wt)
	}
	body, next, err := readLenSlice(data, pos, 0)
	if err != nil {
		return 0, err
	}
	if _, err := decodeSchemaMeta(body, depth+1); err != nil {
		return 0, err
	}
	return next, nil
}

func decodeArrayBlind(body []byte, schema *blindSchema, depth int) (any, error) {
	if depth > MaxNestingDepth {
		return nil, fmt.Errorf("omnibuf: array nesting limit exceeded")
	}
	elem, count, p, ok := parseArrayHeader(body)
	if !ok {
		return nil, fmt.Errorf("omnibuf: invalid array header")
	}
	var typ reflect.Type
	switch elem {
	case wireUint:
		typ = reflect.TypeOf(uint64(0))
	case wireSint:
		typ = reflect.TypeOf(int64(0))
	case wireBoolFalse:
		typ = reflect.TypeOf(false)
	case wireString:
		typ = reflect.TypeOf("")
	case wireBytes:
		typ = reflect.TypeOf([]byte{})
	case wireFixed32:
		typ = reflect.TypeOf(uint32(0))
	case wireFixed64:
		typ = reflect.TypeOf(uint64(0))
	case wireSfixed32:
		typ = reflect.TypeOf(int32(0))
	case wireSfixed64:
		typ = reflect.TypeOf(int64(0))
	case wireFloat32:
		typ = reflect.TypeOf(float32(0))
	case wireFloat64:
		typ = reflect.TypeOf(float64(0))
	case wireSubMsg, wireMap:
		typ = reflect.TypeOf(map[string]any{})
	case wireArray:
		typ = reflect.TypeOf((*any)(nil)).Elem()
	default:
		return nil, fmt.Errorf("omnibuf: unsupported array element type %d", elem)
	}
	out := reflect.MakeSlice(reflect.SliceOf(typ), count, count)
	for i := 0; i < count; i++ {
		var value any
		var next int
		var err error
		switch elem {
		case wireArray:
			var nested []byte
			nested, next, err = readLenSlice(body, p, 2)
			if err == nil {
				value, err = decodeArrayBlind(nested, schema, depth+1)
			}
		case wireMap:
			var nested []byte
			nested, next, err = readLenSlice(body, p, 3)
			if err == nil {
				value, err = decodeMapBodyBlind(nested, schema, depth+1)
			}
		default:
			value, next, err = decodeMapValBlind(body, p, elem, schema, depth)
		}
		if err != nil {
			return nil, err
		}
		out.Index(i).Set(reflect.ValueOf(value))
		p = next
	}
	if p != len(body) {
		return nil, fmt.Errorf("omnibuf: array count/length mismatch")
	}
	return out.Interface(), nil
}

func decodeMapBodyBlind(body []byte, schema *blindSchema, depth int) (map[string]any, error) {
	if depth > MaxNestingDepth {
		return nil, fmt.Errorf("omnibuf: invalid map")
	}
	keyWT, valWT, count, p, ok := parseMapHeader(body)
	if !ok {
		return nil, fmt.Errorf("omnibuf: invalid map")
	}
	out := make(map[string]any, count)
	for range count {
		k, next, err := decodeMapKeyBlind(body, p, keyWT)
		if err != nil {
			return nil, err
		}
		v, next2, err := decodeMapValBlind(body, next, valWT, schema, depth)
		if err != nil {
			return nil, err
		}
		out[k] = v
		p = next2
	}
	if p != len(body) {
		return nil, fmt.Errorf("omnibuf: map count/length mismatch")
	}
	return out, nil
}
