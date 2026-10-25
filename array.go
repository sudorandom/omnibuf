package omnibuf

import (
	"encoding/binary"
	"math"
)

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
	dst = appendVarint(dst, uint64(len(items)))
	for i := range items {
		s := items[i]
		dst = appendVarint(dst, uint64(len(s)))
		dst = append(dst, s...)
	}
	return backpatchContainerLen(dst, lenPos)
}

type intOrUint interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uint | ~int8 | ~int16 | ~int32 | ~int64 | ~int
}

func encodeUintArr[T intOrUint](dst []byte, items []T) []byte {
	lenPos := len(dst)
	dst = append(dst, 0, 0)
	dst = appendVarint(dst, uint64(len(items)))
	for i := range items {
		dst = appendVarint(dst, uint64(items[i]))
	}
	return backpatchContainerLen(dst, lenPos)
}

func encodeSintArr[T intOrUint](dst []byte, items []T) []byte {
	lenPos := len(dst)
	dst = append(dst, 0, 0)
	dst = appendVarint(dst, uint64(len(items)))
	for i := range items {
		dst = appendVarint(dst, encodeZigZag(int64(items[i])))
	}
	return backpatchContainerLen(dst, lenPos)
}

func encodeBoolArr(dst []byte, items []bool) []byte {
	lenPos := len(dst)
	dst = append(dst, 0, 0)
	dst = appendVarint(dst, uint64(len(items)))
	for i := range items {
		if items[i] {
			dst = appendVarint(dst, 1)
		} else {
			dst = appendVarint(dst, 0)
		}
	}
	return backpatchContainerLen(dst, lenPos)
}

func encodeBytesArr(dst []byte, items [][]byte) []byte {
	lenPos := len(dst)
	dst = append(dst, 0, 0)
	dst = appendVarint(dst, uint64(len(items)))
	for i := range items {
		b := items[i]
		dst = appendVarint(dst, uint64(len(b)))
		dst = append(dst, b...)
	}
	return backpatchContainerLen(dst, lenPos)
}

func encodeFixed32CastArr[T intOrUint](dst []byte, items []T) []byte {
	dst = appendVarint(dst, uint64(1+len(items)*4))
	dst = append(dst, wireFixed32)
	for i := range items {
		v := uint32(items[i])
		dst = append(dst, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
	}
	return dst
}

func encodeFixed64CastArr[T intOrUint](dst []byte, items []T) []byte {
	dst = appendVarint(dst, uint64(1+len(items)*8))
	dst = append(dst, wireFixed64)
	for i := range items {
		v := uint64(items[i])
		dst = append(dst,
			byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
			byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56),
		)
	}
	return dst
}

func encodeFixed32Arr(dst []byte, items []uint32) []byte {
	dst = appendVarint(dst, uint64(1+len(items)*4))
	dst = append(dst, wireFixed32)
	for i := range items {
		v := items[i]
		dst = append(dst, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
	}
	return dst
}

func encodeFixed64Arr(dst []byte, items []uint64) []byte {
	dst = appendVarint(dst, uint64(1+len(items)*8))
	dst = append(dst, wireFixed64)
	for i := range items {
		v := items[i]
		dst = append(dst,
			byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
			byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56),
		)
	}
	return dst
}

func appendStringArrBody(dst []string, body []byte, zeroCopy bool) ([]string, bool) {
	count, cn := decodeVarint(body)
	if cn == 0 || count > uint64(len(body)-cn) {
		return nil, false
	}
	c := int(count)
	dst, start := growSlice(dst, c)
	p := cn
	for i := range c {
		sLen, sn := decodeVarint(body[p:])
		if sn == 0 || sLen > uint64(len(body)-(p+sn)) {
			return nil, false
		}
		p += sn
		dst[start+i] = decodeStr(body[p:p+int(sLen)], zeroCopy)
		p += int(sLen)
	}
	if p != len(body) {
		return nil, false
	}
	return dst, true
}

func appendBytesArrBody(dst [][]byte, body []byte, zeroCopy bool) ([][]byte, bool) {
	count, cn := decodeVarint(body)
	if cn == 0 || count > uint64(len(body)-cn) {
		return nil, false
	}
	c := int(count)
	dst, start := growSlice(dst, c)
	p := cn
	for i := range c {
		sLen, sn := decodeVarint(body[p:])
		if sn == 0 || sLen > uint64(len(body)-(p+sn)) {
			return nil, false
		}
		p += sn
		b := body[p : p+int(sLen)]
		if zeroCopy {
			dst[start+i] = b
		} else {
			cp := make([]byte, len(b))
			copy(cp, b)
			dst[start+i] = cp
		}
		p += int(sLen)
	}
	if p != len(body) {
		return nil, false
	}
	return dst, true
}

func appendBoolArrBody(dst []bool, body []byte) ([]bool, bool) {
	count, cn := decodeVarint(body)
	if cn == 0 || count > uint64(len(body)-cn) {
		return nil, false
	}
	c := int(count)
	dst, start := growSlice(dst, c)
	p := cn
	for i := range c {
		v, vn := decodeVarint(body[p:])
		if vn == 0 {
			return nil, false
		}
		p += vn
		dst[start+i] = v != 0
	}
	if p != len(body) {
		return nil, false
	}
	return dst, true
}

func appendUintArrBody[T intOrUint](dst []T, body []byte) ([]T, bool) {
	count, cn := decodeVarint(body)
	if cn == 0 || count > uint64(len(body)-cn) {
		return nil, false
	}
	c := int(count)
	dst, start := growSlice(dst, c)
	p := cn
	for i := range c {
		v, vn := decodeVarint(body[p:])
		if vn == 0 {
			return nil, false
		}
		p += vn
		dst[start+i] = T(v)
	}
	if p != len(body) {
		return nil, false
	}
	return dst, true
}

func appendSintArrBody[T intOrUint](dst []T, body []byte) ([]T, bool) {
	count, cn := decodeVarint(body)
	if cn == 0 || count > uint64(len(body)-cn) {
		return nil, false
	}
	c := int(count)
	dst, start := growSlice(dst, c)
	p := cn
	for i := range c {
		v, vn := decodeVarint(body[p:])
		if vn == 0 {
			return nil, false
		}
		p += vn
		dst[start+i] = T(decodeZigZag(v))
	}
	if p != len(body) {
		return nil, false
	}
	return dst, true
}

func appendFixed32CastArrBody[T intOrUint](dst []T, body []byte) ([]T, bool) {
	if len(body) < 1 || body[0] != wireFixed32 || (len(body)-1)%4 != 0 {
		return nil, false
	}
	c := (len(body) - 1) / 4
	dst, start := growSlice(dst, c)
	for i := range c {
		dst[start+i] = T(binary.LittleEndian.Uint32(body[1+i*4 : 1+(i+1)*4]))
	}
	return dst, true
}

func appendSfixed32CastArrBody[T intOrUint](dst []T, body []byte) ([]T, bool) {
	if len(body) < 1 || body[0] != wireFixed32 || (len(body)-1)%4 != 0 {
		return nil, false
	}
	c := (len(body) - 1) / 4
	dst, start := growSlice(dst, c)
	for i := range c {
		dst[start+i] = T(int32(binary.LittleEndian.Uint32(body[1+i*4 : 1+(i+1)*4])))
	}
	return dst, true
}

func appendFixed64CastArrBody[T intOrUint](dst []T, body []byte) ([]T, bool) {
	if len(body) < 1 || body[0] != wireFixed64 || (len(body)-1)%8 != 0 {
		return nil, false
	}
	c := (len(body) - 1) / 8
	dst, start := growSlice(dst, c)
	for i := range c {
		dst[start+i] = T(binary.LittleEndian.Uint64(body[1+i*8 : 1+(i+1)*8]))
	}
	return dst, true
}

func appendFixed32ArrBody(dst []uint32, body []byte) ([]uint32, bool) {
	if len(body) < 1 || body[0] != wireFixed32 || (len(body)-1)%4 != 0 {
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
	if len(body) < 1 || body[0] != wireFixed32 || (len(body)-1)%4 != 0 {
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
	if len(body) < 1 || body[0] != wireFixed64 || (len(body)-1)%8 != 0 {
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
	if len(body) < 1 || body[0] != wireFixed64 || (len(body)-1)%8 != 0 {
		return nil, false
	}
	c := (len(body) - 1) / 8
	dst, start := growSlice(dst, c)
	for i := range c {
		dst[start+i] = math.Float64frombits(binary.LittleEndian.Uint64(body[1+i*8 : 1+(i+1)*8]))
	}
	return dst, true
}
