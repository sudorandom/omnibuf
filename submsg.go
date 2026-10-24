package omnibuf

import (
	"encoding/binary"
	"unsafe"
)

// ============================================================================
// Nested Sub-Message & Sub-Message Array Wire Types (WireSubMsg, WireSubMsgArr)
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

// BeginSubMsgArr starts a repeated sub-message array (WireSubMsgArr) in a single pass.
func (e *Encoder) BeginSubMsgArr(fieldNum uint16) {
	e.writeTag(fieldNum, WireSubMsgArr)
	e.lenStack[e.depth] = len(e.buf)
	e.depth++
	e.buf = append(e.buf, 0, 0) // Reserve 2B TotalByteLen Prefix-Varint
}

// BeginSubMsgElem starts the next element inside an open WireSubMsgArr by reserving its 2-byte length prefix.
func (e *Encoder) BeginSubMsgElem() {
	e.lenStack[e.depth] = len(e.buf)
	e.depth++
	e.buf = append(e.buf, 0, 0) // Reserve 2B ElemByteLen Prefix-Varint
}

// EndSubMsgElem finishes the current element inside an open WireSubMsgArr and backpatches its length.
func (e *Encoder) EndSubMsgElem() {
	e.depth--
	lenPos := e.lenStack[e.depth]
	e.buf = BackpatchContainerLen(e.buf, lenPos)
}

// EndSubMsgArr backpatches the total byte length of the open WireSubMsgArr.
func (e *Encoder) EndSubMsgArr() {
	e.depth--
	lenPos := e.lenStack[e.depth]
	e.buf = BackpatchContainerLen(e.buf, lenPos)
}

// ReadSubMsg returns the inner packet slice of a nested WireSubMsg field.
func ReadSubMsg(packet []byte, fieldNum uint16) ([]byte, bool) {
	s, wt, ok := LocateField(packet, fieldNum)
	if !ok || wt != WireSubMsg {
		return nil, false
	}
	subLen, sn := DecodeVarint(s)
	if sn == 0 || subLen > uint64(len(s)-sn) {
		return nil, false
	}
	return s[sn : sn+int(subLen)], true
}

// ReadElem slices the i-th sub-message element out of a WireSubMsgArr payload by hopping element length prefixes.
func ReadElem(arrPayload []byte, index int) ([]byte, bool) {
	if index < 0 {
		return nil, false
	}
	pos := 0
	n := len(arrPayload)
	for i := 0; pos < n; i++ {
		var elemLen, en int
		b0 := arrPayload[pos]
		if (b0 & 2) != 0 {
			if pos+2 > n {
				return nil, false
			}
			elemLen = int(binary.LittleEndian.Uint16(arrPayload[pos:pos+2]) >> 2)
			en = 2
		} else {
			l64, ln := DecodeVarint(arrPayload[pos:])
			if ln == 0 || l64 > uint64(n-(pos+ln)) {
				return nil, false
			}
			elemLen = int(l64)
			en = ln
		}
		pos += en
		if elemLen > n-pos {
			return nil, false
		}
		if i == index {
			return arrPayload[pos : pos+elemLen], true
		}
		pos += elemLen
	}
	return nil, false
}

// CountSubMsgArrElems counts and validates the length-prefixed elements inside a WireSubMsgArr payload.
func CountSubMsgArrElems(arrPayload []byte) (int, bool) {
	pos := 0
	n := len(arrPayload)
	count := 0
	for pos < n {
		el, en := DecodeVarint(arrPayload[pos:])
		if en == 0 || el > uint64(n-(pos+en)) {
			return 0, false
		}
		pos += en + int(el)
		count++
	}
	return count, pos == n
}

// SubMsgArrLen returns the total number of elements in a WireSubMsgArr field across any concatenated segments.
func SubMsgArrLen(packet []byte, fieldNum uint16) int {
	total := 0
	for pos := 0; ; {
		body, nextPos, matched, valid := nextContainerField(packet, pos, fieldNum, WireSubMsgArr, 0)
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

// ReadSubMsgArrElem locates a WireSubMsgArr field in packet and slices its i-th element
// (supporting concatenated WireSubMsgArr fields across merged messages).
func ReadSubMsgArrElem(packet []byte, fieldNum uint16, index int) ([]byte, bool) {
	if index < 0 {
		return nil, false
	}
	remIdx := index
	for pos := 0; ; {
		body, nextPos, matched, valid := nextContainerField(packet, pos, fieldNum, WireSubMsgArr, 0)
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
