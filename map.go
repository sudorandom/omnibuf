package omnibuf

import "unsafe"

// ============================================================================
// Map Wire Type (WireMap)
// ============================================================================

// MapHeaderByte encodes the 1-byte WireMap type header: (KeyWireType << 4) | ValWireType.
func MapHeaderByte(keyWT, valWT uint8) byte {
	return ((keyWT & 0x0F) << 4) | (valWT & 0x0F)
}

func encodeMapStringUint(dst []byte, m map[string]uint64, deterministic bool) []byte {
	lenPos := len(dst)
	dst = append(dst, 0, 0, MapHeaderByte(WireString, WireUint))
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

func decodeMapStringUintBody(dst map[string]uint64, body []byte, zeroCopy bool) (map[string]uint64, bool) {
	if len(body) < 2 || body[0] != MapHeaderByte(WireString, WireUint) {
		return nil, false
	}
	count, cn := DecodeVarint(body[1:])
	if cn == 0 || count > uint64(len(body)-(1+cn)) {
		return nil, false
	}
	if dst == nil {
		dst = make(map[string]uint64, int(count))
	}
	p := 1 + cn
	for range int(count) {
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
		dst[k] = v
	}
	if p != len(body) {
		return nil, false
	}
	return dst, true
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
		body, nextPos, matched, valid := nextContainerField(packet, pos, fieldNum, WireMap, 2)
		if !valid {
			return 0, false
		}
		if !matched {
			return lastVal, found
		}
		pos = nextPos
		if body[0] != MapHeaderByte(WireString, WireUint) {
			return 0, false
		}
		count, cn := DecodeVarint(body[1:])
		if cn == 0 || count > uint64(len(body)-(1+cn)) {
			return 0, false
		}
		p := 1 + cn
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
