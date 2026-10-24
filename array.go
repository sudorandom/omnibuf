package omnibuf

import (
	"encoding/binary"
	"math"
)

// ============================================================================
// Packed & Length-Prefixed Array Wire Types
// (WireStringArr, WireUintArr, WireSintArr, WireFixedArr)
// ============================================================================

func growSlice[T any](dst []T, add int) ([]T, int) {
	start := len(dst)
	total := start + add
	if cap(dst) < total || dst == nil {
		next := make([]T, total)
		copy(next, dst)
		return next, start
	}
	return dst[:total], start
}

func encodeStringArr(dst []byte, items []string) []byte {
	lenPos := len(dst)
	dst = append(dst, 0, 0)
	dst = AppendVarint(dst, uint64(len(items)))
	for i := range items {
		s := items[i]
		dst = AppendVarint(dst, uint64(len(s)))
		dst = append(dst, s...)
	}
	return BackpatchContainerLen(dst, lenPos)
}

func encodeUint32Arr(dst []byte, items []uint32) []byte {
	lenPos := len(dst)
	dst = append(dst, 0, 0)
	dst = AppendVarint(dst, uint64(len(items)))
	for i := range items {
		dst = AppendVarint(dst, uint64(items[i]))
	}
	return BackpatchContainerLen(dst, lenPos)
}

func encodeUint64Arr(dst []byte, items []uint64) []byte {
	lenPos := len(dst)
	dst = append(dst, 0, 0)
	dst = AppendVarint(dst, uint64(len(items)))
	for i := range items {
		dst = AppendVarint(dst, items[i])
	}
	return BackpatchContainerLen(dst, lenPos)
}

func encodeSint32Arr(dst []byte, items []int32) []byte {
	lenPos := len(dst)
	dst = append(dst, 0, 0)
	dst = AppendVarint(dst, uint64(len(items)))
	for i := range items {
		dst = AppendVarint(dst, EncodeZigZag(int64(items[i])))
	}
	return BackpatchContainerLen(dst, lenPos)
}

func encodeSint64Arr(dst []byte, items []int64) []byte {
	lenPos := len(dst)
	dst = append(dst, 0, 0)
	dst = AppendVarint(dst, uint64(len(items)))
	for i := range items {
		dst = AppendVarint(dst, EncodeZigZag(items[i]))
	}
	return BackpatchContainerLen(dst, lenPos)
}

func encodeFixed32Arr(dst []byte, items []uint32) []byte {
	dst = AppendVarint(dst, uint64(1+len(items)*4))
	dst = append(dst, WireFixed32)
	for i := range items {
		v := items[i]
		dst = append(dst, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
	}
	return dst
}

func encodeFixed64Arr(dst []byte, items []uint64) []byte {
	dst = AppendVarint(dst, uint64(1+len(items)*8))
	dst = append(dst, WireFixed64)
	for i := range items {
		v := items[i]
		dst = append(dst,
			byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
			byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56),
		)
	}
	return dst
}

// WriteStringArr writes a repeated string array with an outer byte-length prefix (WireStringArr).
func (e *Encoder) WriteStringArr(fieldNum uint16, items []string) {
	e.writeTag(fieldNum, WireStringArr)
	e.buf = encodeStringArr(e.buf, items)
}

// WriteUint32Arr writes a packed uint32 array with a backpatched byte-skip length prefix (WireUintArr).
func (e *Encoder) WriteUint32Arr(fieldNum uint16, items []uint32) {
	e.writeTag(fieldNum, WireUintArr)
	e.buf = encodeUint32Arr(e.buf, items)
}

// WriteSintArr writes a packed ZigZag-encoded signed integer array (WireSintArr).
func (e *Encoder) WriteSintArr(fieldNum uint16, items []int64) {
	e.writeTag(fieldNum, WireSintArr)
	e.buf = encodeSint64Arr(e.buf, items)
}

// WriteFloat32Arr writes a packed float32 array (WireFixedArr with ElemType = WireFixed32).
func (e *Encoder) WriteFloat32Arr(fieldNum uint16, items []float32) {
	e.writeTag(fieldNum, WireFixedArr)
	e.buf = AppendVarint(e.buf, uint64(1+len(items)*4))
	e.buf = append(e.buf, WireFixed32)
	for i := range items {
		bits := math.Float32bits(items[i])
		e.buf = append(e.buf, byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24))
	}
}

// WriteFloat64Arr writes a packed float64 array (WireFixedArr with ElemType = WireFixed64).
func (e *Encoder) WriteFloat64Arr(fieldNum uint16, items []float64) {
	e.writeTag(fieldNum, WireFixedArr)
	e.buf = AppendVarint(e.buf, uint64(1+len(items)*8))
	e.buf = append(e.buf, WireFixed64)
	for i := range items {
		bits := math.Float64bits(items[i])
		e.buf = append(e.buf,
			byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24),
			byte(bits>>32), byte(bits>>40), byte(bits>>48), byte(bits>>56),
		)
	}
}

func appendStringArrBody(dst []string, body []byte, zeroCopy bool) ([]string, bool) {
	count, cn := DecodeVarint(body)
	if cn == 0 || count > uint64(len(body)-cn) {
		return nil, false
	}
	c := int(count)
	dst, start := growSlice(dst, c)
	p := cn
	for i := range c {
		sLen, sn := DecodeVarint(body[p:])
		if sn == 0 || sLen > uint64(len(body)-(p+sn)) {
			return nil, false
		}
		p += sn
		dst[start+i] = DecodeStr(body[p:p+int(sLen)], zeroCopy)
		p += int(sLen)
	}
	if p != len(body) {
		return nil, false
	}
	return dst, true
}

func appendUint32ArrBody(dst []uint32, body []byte) ([]uint32, bool) {
	count, cn := DecodeVarint(body)
	if cn == 0 || count > uint64(len(body)-cn) {
		return nil, false
	}
	c := int(count)
	dst, start := growSlice(dst, c)
	p := cn
	for i := range c {
		v, vn := DecodeVarint(body[p:])
		if vn == 0 {
			return nil, false
		}
		p += vn
		dst[start+i] = uint32(v)
	}
	if p != len(body) {
		return nil, false
	}
	return dst, true
}

func appendUint64ArrBody(dst []uint64, body []byte) ([]uint64, bool) {
	count, cn := DecodeVarint(body)
	if cn == 0 || count > uint64(len(body)-cn) {
		return nil, false
	}
	c := int(count)
	dst, start := growSlice(dst, c)
	p := cn
	for i := range c {
		v, vn := DecodeVarint(body[p:])
		if vn == 0 {
			return nil, false
		}
		p += vn
		dst[start+i] = v
	}
	if p != len(body) {
		return nil, false
	}
	return dst, true
}

func appendSint32ArrBody(dst []int32, body []byte) ([]int32, bool) {
	count, cn := DecodeVarint(body)
	if cn == 0 || count > uint64(len(body)-cn) {
		return nil, false
	}
	c := int(count)
	dst, start := growSlice(dst, c)
	p := cn
	for i := range c {
		v, vn := DecodeVarint(body[p:])
		if vn == 0 {
			return nil, false
		}
		p += vn
		dst[start+i] = int32(DecodeZigZag(v))
	}
	if p != len(body) {
		return nil, false
	}
	return dst, true
}

func appendSint64ArrBody(dst []int64, body []byte) ([]int64, bool) {
	count, cn := DecodeVarint(body)
	if cn == 0 || count > uint64(len(body)-cn) {
		return nil, false
	}
	c := int(count)
	dst, start := growSlice(dst, c)
	p := cn
	for i := range c {
		v, vn := DecodeVarint(body[p:])
		if vn == 0 {
			return nil, false
		}
		p += vn
		dst[start+i] = DecodeZigZag(v)
	}
	if p != len(body) {
		return nil, false
	}
	return dst, true
}

func appendFixed32ArrBody(dst []uint32, body []byte) ([]uint32, bool) {
	if len(body) < 1 || body[0] != WireFixed32 || (len(body)-1)%4 != 0 {
		return nil, false
	}
	c := (len(body) - 1) / 4
	dst, start := growSlice(dst, c)
	for i := range c {
		dst[start+i] = binary.LittleEndian.Uint32(body[1+i*4 : 1+(i+1)*4])
	}
	return dst, true
}

func appendFloat32ArrBody(dst []float32, body []byte) ([]float32, bool) {
	if len(body) < 1 || body[0] != WireFixed32 || (len(body)-1)%4 != 0 {
		return nil, false
	}
	c := (len(body) - 1) / 4
	dst, start := growSlice(dst, c)
	for i := range c {
		dst[start+i] = math.Float32frombits(binary.LittleEndian.Uint32(body[1+i*4 : 1+(i+1)*4]))
	}
	return dst, true
}

func appendFixed64ArrBody(dst []uint64, body []byte) ([]uint64, bool) {
	if len(body) < 1 || body[0] != WireFixed64 || (len(body)-1)%8 != 0 {
		return nil, false
	}
	c := (len(body) - 1) / 8
	dst, start := growSlice(dst, c)
	for i := range c {
		dst[start+i] = binary.LittleEndian.Uint64(body[1+i*8 : 1+(i+1)*8])
	}
	return dst, true
}

func appendFloat64ArrBody(dst []float64, body []byte) ([]float64, bool) {
	if len(body) < 1 || body[0] != WireFixed64 || (len(body)-1)%8 != 0 {
		return nil, false
	}
	c := (len(body) - 1) / 8
	dst, start := growSlice(dst, c)
	for i := range c {
		dst[start+i] = math.Float64frombits(binary.LittleEndian.Uint64(body[1+i*8 : 1+(i+1)*8]))
	}
	return dst, true
}

// ReadStringArr reads a WireStringArr field, concatenating elements if the field appears multiple times.
func ReadStringArr(packet []byte, fieldNum uint16) ([]string, bool) {
	var out []string
	found := false
	for pos := 0; ; {
		body, nextPos, matched, valid := nextContainerField(packet, pos, fieldNum, WireStringArr, 1)
		if !valid {
			return nil, false
		}
		if !matched {
			return out, found
		}
		pos = nextPos
		found = true
		var ok bool
		out, ok = appendStringArrBody(out, body, false)
		if !ok {
			return nil, false
		}
	}
}

// ReadUint32Arr reads a WireUintArr field, concatenating elements if the field appears multiple times.
func ReadUint32Arr(packet []byte, fieldNum uint16) ([]uint32, bool) {
	var out []uint32
	found := false
	for pos := 0; ; {
		body, nextPos, matched, valid := nextContainerField(packet, pos, fieldNum, WireUintArr, 1)
		if !valid {
			return nil, false
		}
		if !matched {
			return out, found
		}
		pos = nextPos
		found = true
		var ok bool
		out, ok = appendUint32ArrBody(out, body)
		if !ok {
			return nil, false
		}
	}
}

// ReadSintArr reads a WireSintArr field (packed ZigZag signed integers), concatenating across segments.
func ReadSintArr(packet []byte, fieldNum uint16) ([]int64, bool) {
	var out []int64
	found := false
	for pos := 0; ; {
		body, nextPos, matched, valid := nextContainerField(packet, pos, fieldNum, WireSintArr, 1)
		if !valid {
			return nil, false
		}
		if !matched {
			return out, found
		}
		pos = nextPos
		found = true
		var ok bool
		out, ok = appendSint64ArrBody(out, body)
		if !ok {
			return nil, false
		}
	}
}

// ReadFloat32Arr reads a WireFixedArr field (ElemType = WireFixed32, packed 4B LE float32/fixed32 values), concatenating across segments.
func ReadFloat32Arr(packet []byte, fieldNum uint16) ([]float32, bool) {
	var out []float32
	found := false
	for pos := 0; ; {
		body, nextPos, matched, valid := nextContainerField(packet, pos, fieldNum, WireFixedArr, 1)
		if !valid {
			return nil, false
		}
		if !matched {
			return out, found
		}
		pos = nextPos
		found = true
		var ok bool
		out, ok = appendFloat32ArrBody(out, body)
		if !ok {
			return nil, false
		}
	}
}

// ReadFloat64Arr reads a WireFixedArr field (ElemType = WireFixed64, packed 8B LE float64/fixed64 values), concatenating across segments.
func ReadFloat64Arr(packet []byte, fieldNum uint16) ([]float64, bool) {
	var out []float64
	found := false
	for pos := 0; ; {
		body, nextPos, matched, valid := nextContainerField(packet, pos, fieldNum, WireFixedArr, 1)
		if !valid {
			return nil, false
		}
		if !matched {
			return out, found
		}
		pos = nextPos
		found = true
		var ok bool
		out, ok = appendFloat64ArrBody(out, body)
		if !ok {
			return nil, false
		}
	}
}
