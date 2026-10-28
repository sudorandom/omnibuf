package wire

import (
	"encoding/binary"
	"unsafe"
)

// ============================================================================
// Nested Sub-Message & Sub-Message Array Wire Types (WireSubMsg, WireArray)
// ============================================================================

// BeginSubMsg starts a nested sub-message (WireSubMsg) in a single pass by reserving a 2-byte
// Prefix-Varint length slot.
func (e *Encoder) BeginSubMsg(fieldNum uint16) {
	e.writeTag(fieldNum, WireSubMsg)
	e.lenStack[e.depth] = len(e.buf)
	e.depth++
	e.buf = append(e.buf, 0, 0) // Reserve 2B Prefix-Varint length
}

// EndSubMsg finishes the current nested sub-message and backpatches its byte length.
func (e *Encoder) EndSubMsg() {
	e.depth--
	lenPos := e.lenStack[e.depth]
	e.buf = BackpatchContainerLen(e.buf, lenPos)
}

// BeginSubMsgArr starts a repeated sub-message array (WireArray) in a single pass.
func (e *Encoder) BeginSubMsgArr(fieldNum uint16) {
	e.writeTag(fieldNum, WireArray)
	e.lenStack[e.depth] = len(e.buf)
	e.depth++
	e.countStack[e.depth-1] = 0
	e.buf = append(e.buf, 0, 0, WireSubMsg, 0) // 2B Length, 1B element type, 1B count
}

// BeginSubMsgElem starts the next element inside an open WireArray by reserving its 2-byte length prefix.
func (e *Encoder) BeginSubMsgElem() {
	e.countStack[e.depth-1]++
	e.lenStack[e.depth] = len(e.buf)
	e.depth++
	e.buf = append(e.buf, 0, 0) // Reserve 2B ElemByteLen Prefix-Varint
}

// EndSubMsgElem finishes the current element inside an open WireArray and backpatches its length.
func (e *Encoder) EndSubMsgElem() {
	e.depth--
	lenPos := e.lenStack[e.depth]
	e.buf = BackpatchContainerLen(e.buf, lenPos)
}

// EndSubMsgArr backpatches the total byte length of the open WireArray.
func (e *Encoder) EndSubMsgArr() {
	e.depth--
	lenPos := e.lenStack[e.depth]
	e.buf = PatchReservedVarint(e.buf, lenPos+3, uint64(e.countStack[e.depth]))
	e.buf = BackpatchContainerLen(e.buf, lenPos)
}

// ReadSubMsg returns the inner packet slice of a nested WireSubMsg field.
// When multiple occurrences of fieldNum exist (e.g. concatenated packets),
// their inner payloads are concatenated so sub-message fields merge.
func ReadSubMsg(packet []byte, fieldNum uint16) ([]byte, bool) {
	pos := 0
	n := len(packet)
	var valSlice []byte
	count := 0
	for pos < n {
		var fnum uint32
		var wt uint8
		b0 := packet[pos]
		if (b0 & 1) != 0 {
			fnum = uint32(b0 >> 5)
			wt = (b0 >> 1) & 0x0F
			pos++
		} else if (b0 & 2) != 0 {
			if pos+2 > n {
				return nil, false
			}
			u := binary.LittleEndian.Uint16(packet[pos:pos+2]) >> 2
			fnum = uint32(u >> 4)
			wt = uint8(u & 0x0F)
			pos += 2
		} else {
			tag, tn := decodeVarintMulti(packet[pos:])
			if tn == 0 || tag>>4 > 536870911 {
				return nil, false
			}
			fnum = uint32(tag >> 4)
			wt = uint8(tag & 0x0F)
			pos += tn
		}

		if fnum == uint32(fieldNum) {
			if wt != WireSubMsg {
				return nil, false
			}
			valSlice = packet[pos:]
			count++
		}

		switch wt {
		case WireUint, WireSint:
			if pos >= n {
				return nil, false
			}
			vb := packet[pos]
			if (vb & 1) != 0 {
				pos++
			} else if (vb & 2) != 0 {
				pos += 2
			} else if (vb & 4) != 0 {
				pos += 4
			} else {
				pos += 9
			}
			if pos > n {
				return nil, false
			}
		case WireBoolFalse, WireBoolTrue:
		case WireString, WireBytes, WireSubMsg, WireArray, WireMap, WireExtended:
			if pos >= n {
				return nil, false
			}
			lb := packet[pos]
			if (lb & 1) != 0 {
				pos += 1 + int(lb>>1)
			} else if (lb & 2) != 0 {
				if pos+2 > n {
					return nil, false
				}
				pos += 2 + int(binary.LittleEndian.Uint16(packet[pos:pos+2])>>2)
			} else {
				bLen, bn := decodeVarintMulti(packet[pos:])
				if bn == 0 || bLen > uint64(n-(pos+bn)) {
					return nil, false
				}
				pos += bn + int(bLen)
			}
			if pos > n {
				return nil, false
			}
		case WireFixed32, WireFloat32, WireSfixed32:
			pos += 4
			if pos > n {
				return nil, false
			}
		case WireFixed64, WireFloat64, WireSfixed64:
			pos += 8
			if pos > n {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	if count == 0 {
		return nil, false
	}
	if count > 1 {
		return mergeSubMsgSegments(packet, fieldNum)
	}
	subLen, sn := DecodeVarint(valSlice)
	if sn == 0 || subLen > uint64(len(valSlice)-sn) {
		return nil, false
	}
	return valSlice[sn : sn+int(subLen)], true
}

//go:noinline
func mergeSubMsgSegments(packet []byte, fieldNum uint16) ([]byte, bool) {
	var merged []byte
	for pos := 0; ; {
		body, nextPos, matched, valid := nextContainerField(packet, pos, fieldNum, WireSubMsg, 0)
		if !valid {
			return nil, false
		}
		if !matched {
			return merged, true
		}
		merged = append(merged, body...)
		pos = nextPos
	}
}

// ReadElem slices the i-th sub-message element out of a WireArray payload by hopping element length prefixes.
func ReadElem(body []byte, index int) ([]byte, bool) {
	e, count, p, ok := ParseArrayHeader(body)
	if !ok || e != WireSubMsg || index < 0 || index >= count {
		return nil, false
	}
	for i := 0; i < count; i++ {
		l, n := DecodeVarint(body[p:])
		if n == 0 || l > uint64(len(body)-p-n) {
			return nil, false
		}
		p += n
		if i == index {
			return body[p : p+int(l)], true
		}
		p += int(l)
	}
	return nil, false
}
func CountSubMsgArrElems(body []byte) (int, bool) {
	e, count, p, ok := ParseArrayHeader(body)
	if !ok || e != WireSubMsg {
		return 0, false
	}
	for i := 0; i < count; i++ {
		l, n := DecodeVarint(body[p:])
		if n == 0 || l > uint64(len(body)-p-n) {
			return 0, false
		}
		p += n + int(l)
	}
	return count, p == len(body)
}

// SubMsgArrLen returns the total number of elements in a WireArray field across any concatenated segments.
func SubMsgArrLen(packet []byte, fieldNum uint16) int {
	total := 0
	for pos := 0; ; {
		body, nextPos, matched, valid := nextContainerField(packet, pos, fieldNum, WireArray, 0)
		if !valid {
			return 0
		}
		if !matched {
			return total
		}
		pos = nextPos
		c, ok := CountSubMsgArrElems(body)
		if !ok {
			return 0
		}
		total += c
	}
}

// ReadSubMsgArrElem locates a WireArray field in packet and slices its i-th element
// (supporting concatenated WireArray fields across merged messages).
func ReadSubMsgArrElem(packet []byte, fieldNum uint16, index int) ([]byte, bool) {
	if index < 0 {
		return nil, false
	}
	remIdx := index
	for pos := 0; ; {
		body, nextPos, matched, valid := nextContainerField(packet, pos, fieldNum, WireArray, 0)
		if !valid || !matched {
			return nil, false
		}
		pos = nextPos
		if elem, ok := ReadElem(body, remIdx); ok {
			return elem, true
		}
		c, ok := CountSubMsgArrElems(body)
		if !ok {
			return nil, false
		}
		remIdx -= c
	}
}

// ReadNestedStringBytes locates targetField inside subMsgField, supporting merged sub-messages across concatenated packets.
func ReadNestedStringBytes(packet []byte, subMsgField, targetField uint16) ([]byte, bool) {
	var lastBytes []byte
	found := false
	for pos := 0; ; {
		sub, nextPos, matched, valid := nextContainerField(packet, pos, subMsgField, WireSubMsg, 0)
		if !valid {
			return nil, false
		}
		if !matched {
			return lastBytes, found
		}
		pos = nextPos
		if b, ok := ReadStringBytes(sub, targetField); ok {
			lastBytes = b
			found = true
		}
	}
}

// ReadNestedString reads a nested string (e.g. peer.location.city), allocating a safe string copy.
func ReadNestedString(packet []byte, subMsgField, targetField uint16) (string, bool) {
	b, ok := ReadNestedStringBytes(packet, subMsgField, targetField)
	if !ok {
		return "", false
	}
	return string(b), true
}

// ReadNestedStringZeroCopy reads a nested string (e.g. peer.location.city) with 0 allocations.
func ReadNestedStringZeroCopy(packet []byte, subMsgField, targetField uint16) (string, bool) {
	b, ok := ReadNestedStringBytes(packet, subMsgField, targetField)
	if !ok || len(b) == 0 {
		return "", ok
	}
	return unsafe.String(&b[0], len(b)), true
}

// ReadArrNestedString reads a string inside the i-th repeated sub-message (e.g. peer.neighbors[2].router).
func ReadArrNestedString(packet []byte, arrField uint16, index int, targetField uint16) (string, bool) {
	elem, ok := ReadSubMsgArrElem(packet, arrField, index)
	if !ok {
		return "", false
	}
	return ReadString(elem, targetField)
}

// ReadArrNestedStringZeroCopy reads a string inside the i-th repeated sub-message with 0 allocations.
func ReadArrNestedStringZeroCopy(packet []byte, arrField uint16, index int, targetField uint16) (string, bool) {
	elem, ok := ReadSubMsgArrElem(packet, arrField, index)
	if !ok {
		return "", false
	}
	return ReadStringZeroCopy(elem, targetField)
}
