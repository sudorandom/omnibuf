package omnibuf

import (
	"encoding/binary"
	"fmt"
	"maps"
	"math"
)

// SchemaMeta is the optional inline schema header encoded as Field #0
// (Tag = 0x0B, wireSubMsg = 5) at byte 0 of an OmniBuf message (omnibuf.v1.SchemaMeta).
type SchemaMeta struct {
	TypeName string                 `protobuf:"bytes,1,opt,name=type_name,proto3" json:"type_name,omitempty"`
	Fields   map[uint32]string      `protobuf:"bytes,2,rep,name=fields,proto3" json:"fields,omitempty"`
	Children map[uint32]*SchemaMeta `protobuf:"bytes,3,rep,name=children,proto3" json:"children,omitempty"`
}

// AppendField0 appends Field #0 (Tag = 0x0B, wireSubMsg = 5) encoding meta to dst.
// When deterministic is true, map keys in Fields and Children are sorted in ascending order.
func (meta *SchemaMeta) AppendField0(dst []byte, deterministic bool) []byte {
	return appendField0SchemaMetaDeterministic(dst, meta, deterministic)
}

func appendField0SchemaMetaDeterministic(dst []byte, meta *SchemaMeta, deterministic bool) []byte {
	if meta == nil {
		return dst
	}
	dst = append(dst, 0x0B) // Tag = (0 << 4) | wireSubMsg(5) = 5 -> (5 << 1) | 1 = 0x0B
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
	// Field #1: type_name (wireString = 3 -> Tag (1 << 4) | 3 = 19 -> 0x27)
	if meta.TypeName != "" {
		dst = append(dst, 0x27)
		dst = appendVarint(dst, uint64(len(meta.TypeName)))
		dst = append(dst, meta.TypeName...)
	}
	// Field #2: fields map<uint32, string> (wireMap = 9 -> Tag (2 << 4) | 9 = 41 -> 0x53)
	if len(meta.Fields) > 0 {
		dst = append(dst, 0x53)
		lenPos := len(dst)
		dst = append(dst, 0, 0, mapHeaderByte(wireUint, wireString))
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
	// Field #3: children map<uint32, SchemaMeta> (wireMap = 9 -> Tag (3 << 4) | 9 = 57 -> 0x73)
	if len(meta.Children) > 0 {
		dst = append(dst, 0x73)
		lenPos := len(dst)
		dst = append(dst, 0, 0, mapHeaderByte(wireUint, wireSubMsg))
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
			s, nextPos, err := readLenSlice(data, pos, 0)
			if err != nil {
				return nil, err
			}
			meta.TypeName = string(s)
			pos = nextPos
		case 2: // fields (map<uint32, string>)
			if wt != wireMap {
				return nil, fmt.Errorf("omnibuf: SchemaMeta field #2 (fields) must be wireMap, got %d", wt)
			}
			body, nextPos, err := readLenSlice(data, pos, 2)
			if err != nil {
				return nil, err
			}
			pos = nextPos
			if body[0] != mapHeaderByte(wireUint, wireString) {
				return nil, fmt.Errorf("omnibuf: SchemaMeta fields map must have MapType 0x03, got 0x%02x", body[0])
			}
			count, cn := decodeVarint(body[1:])
			if cn == 0 || count*2 > uint64(len(body)-(1+cn)) {
				return nil, fmt.Errorf("omnibuf: SchemaMeta fields count %d exceeds body", count)
			}
			if meta.Fields == nil {
				meta.Fields = make(map[uint32]string, int(count))
			}
			p := 1 + cn
			for range int(count) {
				k64, kn := decodeVarint(body[p:])
				if kn == 0 || k64 == 0 || k64 > math.MaxUint32 {
					return nil, fmt.Errorf("omnibuf: invalid SchemaMeta field number %d at %d", k64, p)
				}
				p += kn
				nameBytes, nextP, err := readLenSlice(body, p, 0)
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
			body, nextPos, err := readLenSlice(data, pos, 2)
			if err != nil {
				return nil, err
			}
			pos = nextPos
			if body[0] != mapHeaderByte(wireUint, wireSubMsg) {
				return nil, fmt.Errorf("omnibuf: SchemaMeta children map must have MapType 0x05, got 0x%02x", body[0])
			}
			count, cn := decodeVarint(body[1:])
			if cn == 0 || count*2 > uint64(len(body)-(1+cn)) {
				return nil, fmt.Errorf("omnibuf: SchemaMeta children count %d exceeds body", count)
			}
			if meta.Children == nil {
				meta.Children = make(map[uint32]*SchemaMeta, int(count))
			}
			p := 1 + cn
			for range int(count) {
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

// DecodeBlind decodes any OmniBuf packet without a compile-time schema.
// If Field #0 (Tag = 0x0B, wireSubMsg = 5, omnibuf.v1.SchemaMeta) is present, its field names
// (and any nested children schemas) are used to decode human-readable keys.
// Otherwise, synthetic field names ("field_1", "field_2", ...) are used.
func DecodeBlind(packet []byte) (map[string]any, error) {
	return decodeBlindWithSchema(packet, nil, 0)
}

func decodeBlindWithSchema(data []byte, inherited *SchemaMeta, depth int) (map[string]any, error) {
	if depth > MaxNestingDepth {
		return nil, fmt.Errorf("omnibuf: nesting depth %d exceeds MaxNestingDepth (%d)", depth, MaxNestingDepth)
	}
	n := len(data)
	if n == 0 {
		return map[string]any{}, nil
	}

	var activeSchema *SchemaMeta
	if inherited != nil {
		activeSchema = mergeSchemaMeta(nil, inherited)
	}
	pos := 0
	out := make(map[string]any, 8)

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
			f0Body, nextPos, err := readLenSlice(data, pos, 0)
			if err != nil {
				return nil, err
			}
			pos = nextPos
			parsedMeta, err := decodeSchemaMeta(f0Body, depth+1)
			if err != nil {
				return nil, err
			}
			activeSchema = mergeSchemaMeta(activeSchema, parsedMeta)
			continue
		}

		var name string
		if activeSchema != nil && activeSchema.Fields != nil {
			name = activeSchema.Fields[uint32(fieldNum)]
		}
		if name == "" {
			name = fmt.Sprintf("field_%d", fieldNum)
		}

		var childSchema *SchemaMeta
		if activeSchema != nil && activeSchema.Children != nil {
			childSchema = activeSchema.Children[uint32(fieldNum)]
		}

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
			s, nextPos, err := readLenSlice(data, pos, 0)
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
			subMap, err := decodeBlindWithSchema(subBytes, childSchema, depth+1)
			if err != nil {
				return nil, err
			}
			if prevMap, ok := out[name].(map[string]any); ok {
				maps.Copy(prevMap, subMap)
			} else {
				out[name] = subMap
			}
			pos = nextPos
		case wireStringArr:
			body, nextPos, err := readLenSlice(data, pos, 1)
			if err != nil {
				return nil, err
			}
			prev, _ := out[name].([]string)
			arr, ok := appendStringArrBody(prev, body, false)
			if !ok {
				return nil, fmt.Errorf("omnibuf: invalid wireStringArr at %d", pos)
			}
			out[name] = arr
			pos = nextPos
		case wireUintArr:
			body, nextPos, err := readLenSlice(data, pos, 1)
			if err != nil {
				return nil, err
			}
			prev, _ := out[name].([]uint64)
			arr, ok := appendUintArrBody(prev, body)
			if !ok {
				return nil, fmt.Errorf("omnibuf: invalid wireUintArr at %d", pos)
			}
			out[name] = arr
			pos = nextPos
		case wireSintArr:
			body, nextPos, err := readLenSlice(data, pos, 1)
			if err != nil {
				return nil, err
			}
			prev, _ := out[name].([]int64)
			arr, ok := appendSintArrBody(prev, body)
			if !ok {
				return nil, fmt.Errorf("omnibuf: invalid wireSintArr at %d", pos)
			}
			out[name] = arr
			pos = nextPos
		case wireFixedArr:
			body, nextPos, err := readLenSlice(data, pos, 1)
			if err != nil {
				return nil, err
			}
			switch elemType := body[0]; elemType {
			case wireFixed32:
				prev, _ := out[name].([]float32)
				arr, ok := appendFloat32ArrBody(prev, body)
				if !ok {
					return nil, fmt.Errorf("omnibuf: invalid wireFixedArr(Fixed32) at %d", pos)
				}
				out[name] = arr
			case wireFixed64:
				prev, _ := out[name].([]float64)
				arr, ok := appendFloat64ArrBody(prev, body)
				if !ok {
					return nil, fmt.Errorf("omnibuf: invalid wireFixedArr(Fixed64) at %d", pos)
				}
				out[name] = arr
			default:
				return nil, fmt.Errorf("omnibuf: invalid wireFixedArr ElemType %d", elemType)
			}
			pos = nextPos
		case wireExtended:
			body, _, err := readLenSlice(data, pos, 1)
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("omnibuf: unrecognized wireExtended subtype 0x%02x", body[0])
		case wireSubMsgArr:
			arrPayload, nextPos, err := readLenSlice(data, pos, 0)
			if err != nil {
				return nil, err
			}
			elemCount, ok := countSubMsgArrElems(arrPayload)
			if !ok {
				return nil, fmt.Errorf("omnibuf: corrupt wireSubMsgArr element framing at %d", pos)
			}
			elems, _ := out[name].([]map[string]any)
			if elems == nil {
				elems = make([]map[string]any, 0, elemCount)
			}
			p := 0
			for range elemCount {
				el, en := decodeVarint(arrPayload[p:])
				p += en
				elemMap, err := decodeBlindWithSchema(arrPayload[p:p+int(el)], childSchema, depth+1)
				if err != nil {
					return nil, err
				}
				elems = append(elems, elemMap)
				p += int(el)
			}
			out[name] = elems
			pos = nextPos
		case wireMap:
			body, nextPos, err := readLenSlice(data, pos, 2)
			if err != nil {
				return nil, err
			}
			mapType := body[0]
			keyWT := mapType >> 4
			valWT := mapType & 0x0F
			if keyWT == wireString && valWT == wireUint {
				prev, _ := out[name].(map[string]uint64)
				m, ok := decodeMapStringUintBody(prev, body, false)
				if !ok {
					return nil, fmt.Errorf("omnibuf: invalid wireMap(string, uint) at %d", pos)
				}
				out[name] = m
			} else {
				count, cn := decodeVarint(body[1:])
				if cn == 0 || count > uint64(len(body)-(1+cn)) {
					return nil, fmt.Errorf("omnibuf: wireMap count %d exceeds body (%d B)", count, len(body))
				}
				m, ok := out[name].(map[string]any)
				if !ok || m == nil {
					m = make(map[string]any, int(count))
				}
				p := 1 + cn
				for range int(count) {
					k, nextP, err := decodeMapKeyBlind(body, p, keyWT)
					if err != nil {
						return nil, err
					}
					v, nextP2, err := decodeMapValBlind(body, nextP, valWT, childSchema, depth)
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
		case wireFixed32:
			if n-pos < 4 {
				return nil, fmt.Errorf("omnibuf: truncated wireFixed32 at %d", pos)
			}
			out[name] = math.Float32frombits(binary.LittleEndian.Uint32(data[pos : pos+4]))
			pos += 4
		case wireFixed64:
			if n-pos < 8 {
				return nil, fmt.Errorf("omnibuf: truncated wireFixed64 at %d", pos)
			}
			out[name] = math.Float64frombits(binary.LittleEndian.Uint64(data[pos : pos+8]))
			pos += 8
		default:
			return nil, fmt.Errorf("omnibuf: unsupported wire type %d", wt)
		}
	}

	return out, nil
}

func decodeMapKeyBlind(body []byte, p int, keyWT uint8) (string, int, error) {
	switch keyWT {
	case wireString:
		kBytes, nextP, err := readLenSlice(body, p, 0)
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
	case wireFixed32:
		if len(body)-p < 4 {
			return "", 0, fmt.Errorf("omnibuf: truncated wireMap fixed32 key at %d", p)
		}
		return fmt.Sprintf("%d", binary.LittleEndian.Uint32(body[p:p+4])), p + 4, nil
	case wireFixed64:
		if len(body)-p < 8 {
			return "", 0, fmt.Errorf("omnibuf: truncated wireMap fixed64 key at %d", p)
		}
		return fmt.Sprintf("%d", binary.LittleEndian.Uint64(body[p:p+8])), p + 8, nil
	case wireBoolTrue, wireBoolFalse:
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

func decodeMapValBlind(body []byte, p int, valWT uint8, childSchema *SchemaMeta, depth int) (any, int, error) {
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
		vBytes, nextP, err := readLenSlice(body, p, 0)
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
	case wireFixed32:
		if len(body)-p < 4 {
			return nil, 0, fmt.Errorf("omnibuf: truncated wireMap fixed32 value at %d", p)
		}
		return math.Float32frombits(binary.LittleEndian.Uint32(body[p : p+4])), p + 4, nil
	case wireFixed64:
		if len(body)-p < 8 {
			return nil, 0, fmt.Errorf("omnibuf: truncated wireMap fixed64 value at %d", p)
		}
		return math.Float64frombits(binary.LittleEndian.Uint64(body[p : p+8])), p + 8, nil
	case wireBoolTrue, wireBoolFalse:
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
