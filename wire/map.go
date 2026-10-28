package wire

import "unsafe"

// ============================================================================
// Map Wire Type (WireMap)
// ============================================================================

// IsValidMapKeyWireType reports whether wt is a valid 1-byte KeyType in a WireMap header.
func IsValidMapKeyWireType(wt uint8) bool {
	switch wt {
	case WireUint, WireSint, WireBoolFalse, WireFixed32, WireFixed64, WireSfixed32, WireSfixed64, WireString:
		return true
	default:
		return false
	}
}

// IsValidMapValWireType reports whether wt is a valid 1-byte ValueType in a WireMap header.
func IsValidMapValWireType(wt uint8) bool {
	return wt <= WireExtended && wt != WireBoolTrue
}

// ParseMapHeader parses and validates the [KeyType: 1B][ValueType: 1B][Count: Prefix-Varint] header
// at the start of a WireMap payload body.
func ParseMapHeader(body []byte) (keyWT, valWT uint8, count, pos int, ok bool) {
	if len(body) < 3 {
		return 0, 0, 0, 0, false
	}
	keyWT = body[0]
	valWT = body[1]
	if !IsValidMapKeyWireType(keyWT) || !IsValidMapValWireType(valWT) {
		return 0, 0, 0, 0, false
	}
	c, n := DecodeVarint(body[2:])
	if n == 0 || c > uint64(len(body)-(2+n))/2 {
		return 0, 0, 0, 0, false
	}
	return keyWT, valWT, int(c), 2 + n, true
}

func encodeMapStringUint(dst []byte, m map[string]uint64, deterministic bool) []byte {
	lenPos := len(dst)
	dst = append(dst, 0, 0, WireString, WireUint)
	dst = AppendVarint(dst, uint64(len(m)))
	if deterministic && len(m) > 1 {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		for i := 1; i < len(keys); i++ {
			for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
				keys[j], keys[j-1] = keys[j-1], keys[j]
			}
		}
		for _, k := range keys {
			dst = AppendVarint(dst, uint64(len(k)))
			dst = append(dst, k...)
			dst = AppendVarint(dst, m[k])
		}
	} else {
		for k, v := range m {
			dst = AppendVarint(dst, uint64(len(k)))
			dst = append(dst, k...)
			dst = AppendVarint(dst, v)
		}
	}
	return BackpatchContainerLen(dst, lenPos)
}

// EncodeMapStringUint appends a WireMap<WireString, WireUint> container to dst.
func EncodeMapStringUint(dst []byte, m map[string]uint64, deterministic bool) []byte {
	return encodeMapStringUint(dst, m, deterministic)
}

// ReadMapStringUint decodes a WireMap<WireString, WireUint> field across any concatenated segments.
func ReadMapStringUint(packet []byte, fieldNum uint16, zeroCopy bool) (map[string]uint64, bool) {
	var out map[string]uint64
	found := false
	for pos := 0; ; {
		body, nextPos, matched, valid := nextContainerField(packet, pos, fieldNum, WireMap, 3)
		if !valid {
			return nil, false
		}
		if !matched {
			return out, found
		}
		pos = nextPos
		keyWT, valWT, count, p, ok := ParseMapHeader(body)
		if !ok || keyWT != WireString || valWT != WireUint {
			return nil, false
		}
		if out == nil {
			out = make(map[string]uint64, count)
		}
		for range count {
			kLen, kn := DecodeVarint(body[p:])
			if kn == 0 || kLen > uint64(len(body)-(p+kn)) {
				return nil, false
			}
			p += kn
			k := DecodeStr(body[p:p+int(kLen)], zeroCopy)
			p += int(kLen)
			v, vn := DecodeVarint(body[p:])
			if vn == 0 {
				return nil, false
			}
			p += vn
			out[k] = v
		}
		if p != len(body) {
			return nil, false
		}
		found = true
	}
}

// WriteMapStringUint writes a map<string, uint64> (WireMap) in a single pass with zero allocations.
func (e *Encoder) WriteMapStringUint(fieldNum uint16, m map[string]uint64) {
	e.writeTag(fieldNum, WireMap)
	e.buf = encodeMapStringUint(e.buf, m, false)
}

// WriteMapStringUintDeterministic writes a map<string, uint64> (WireMap) with keys sorted in lexicographical order.
func (e *Encoder) WriteMapStringUintDeterministic(fieldNum uint16, m map[string]uint64) {
	e.writeTag(fieldNum, WireMap)
	e.buf = encodeMapStringUint(e.buf, m, true)
}

// LookupMapStringUint searches a WireMap field without allocating a Go map,
// supporting merged WireMap fields and "last write wins" across entries and concatenated packets.
func LookupMapStringUint(packet []byte, fieldNum uint16, targetKey string) (uint64, bool) {
	var lastVal uint64
	found := false
	for pos := 0; ; {
		body, nextPos, matched, valid := nextContainerField(packet, pos, fieldNum, WireMap, 3)
		if !valid {
			return 0, false
		}
		if !matched {
			return lastVal, found
		}
		pos = nextPos
		keyWT, valWT, count, p, ok := ParseMapHeader(body)
		if !ok || keyWT != WireString || valWT != WireUint {
			return 0, false
		}
		for range count {
			kLen, kn := DecodeVarint(body[p:])
			if kn == 0 || kLen > uint64(len(body)-(p+kn)) {
				return 0, false
			}
			p += kn
			var k string
			if kLen > 0 {
				k = unsafe.String(&body[p], int(kLen))
			}
			p += int(kLen)
			v, vn := DecodeVarint(body[p:])
			if vn == 0 {
				return 0, false
			}
			p += vn
			if k == targetKey {
				lastVal = v
				found = true
			}
		}
		if p != len(body) {
			return 0, false
		}
	}
}
