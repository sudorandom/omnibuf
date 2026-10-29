package omnibuf

import (
	"github.com/sudorandom/omnibuf/wire"
)

type intOrUint interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uint | ~int8 | ~int16 | ~int32 | ~int64 | ~int
}

func parseArrayHeader(body []byte) (uint8, int, int, bool) {
	return wire.ParseArrayHeader(body)
}

func encodeStringArr(d []byte, v []string) []byte       { return wire.EncodeStringArr(d, v) }
func encodeBytesArr(d []byte, v [][]byte) []byte        { return wire.EncodeBytesArr(d, v) }
func encodeBoolArr(d []byte, v []bool) []byte           { return wire.EncodeBoolArr(d, v) }
func encodeUintArr[T intOrUint](d []byte, v []T) []byte { return wire.EncodeUintArr(d, v) }
func encodeSintArr[T intOrUint](d []byte, v []T) []byte { return wire.EncodeSintArr(d, v) }
func encodeFixed32CastArr[T intOrUint](d []byte, v []T, elem uint8) []byte {
	return wire.EncodeFixed32Arr(d, v, elem)
}
func encodeFixed64CastArr[T intOrUint](d []byte, v []T, elem uint8) []byte {
	return wire.EncodeFixed64Arr(d, v, elem)
}
func encodeFixed32Arr(d []byte, v []uint32, elem uint8) []byte {
	return wire.EncodeFixed32Arr(d, v, elem)
}
func encodeFixed64Arr(d []byte, v []uint64, elem uint8) []byte {
	return wire.EncodeFixed64Arr(d, v, elem)
}
func appendBoolArrBody(d []bool, b []byte) ([]bool, bool)        { return wire.AppendBoolArrBody(d, b) }
func appendUintArrBody[T intOrUint](d []T, b []byte) ([]T, bool) { return wire.AppendUintArrBody(d, b) }
func appendSintArrBody[T intOrUint](d []T, b []byte) ([]T, bool) { return wire.AppendSintArrBody(d, b) }
func appendFixed32CastArrBody[T intOrUint](d []T, b []byte) ([]T, bool) {
	return wire.AppendFixed32ArrBody(d, b)
}
func appendSfixed32CastArrBody[T intOrUint](d []T, b []byte) ([]T, bool) {
	return wire.AppendFixed32ArrBody(d, b)
}
func appendFixed64CastArrBody[T intOrUint](d []T, b []byte) ([]T, bool) {
	return wire.AppendFixed64ArrBody(d, b)
}
func appendFixed32ArrBody(d []uint32, b []byte) ([]uint32, bool) {
	return wire.AppendFixed32ArrBody(d, b)
}
func appendFixed64ArrBody(d []uint64, b []byte) ([]uint64, bool) {
	return wire.AppendFixed64ArrBody(d, b)
}

// encodeStringArrAs encodes []string as a WireArray of elem (WireString or WireBytes).
func encodeStringArrAs(d []byte, v []string, elem uint8) []byte {
	if elem == wireString {
		return wire.EncodeStringArr(d, v)
	}
	p := len(d)
	d = append(d, 0, 0, elem)
	d = appendVarint(d, uint64(len(v)))
	for _, x := range v {
		d = appendVarint(d, uint64(len(x)))
		d = append(d, x...)
	}
	return backpatchContainerLen(d, p)
}

// encodeBytesArrAs encodes [][]byte as a WireArray of elem (WireBytes or WireString).
func encodeBytesArrAs(d []byte, v [][]byte, elem uint8) []byte {
	if elem == wireBytes {
		return wire.EncodeBytesArr(d, v)
	}
	p := len(d)
	d = append(d, 0, 0, elem)
	d = appendVarint(d, uint64(len(v)))
	for _, x := range v {
		d = appendVarint(d, uint64(len(x)))
		d = append(d, x...)
	}
	return backpatchContainerLen(d, p)
}

// appendLenDelimArrBody decodes a WireArray body whose elements are
// length-delimited (WireString or WireBytes), validating UTF-8 for WireString.
func appendLenDelimArrBody[T any](dst []T, body []byte, elem uint8, conv func([]byte) T) ([]T, bool) {
	e, c, p, ok := parseArrayHeader(body)
	if !ok || e != elem {
		return nil, false
	}
	dst, start := growSliceOmni(dst, c)
	for i := 0; i < c; i++ {
		l, n := decodeVarint(body[p:])
		if n == 0 || l > uint64(len(body)-p-n) {
			return nil, false
		}
		p += n
		b := body[p : p+int(l)]
		if elem == wireString && !wire.ValidUTF8(b) {
			return nil, false
		}
		dst[start+i] = conv(b)
		p += int(l)
	}
	return dst, p == len(body)
}

func growSliceOmni[T any](dst []T, add int) ([]T, int) {
	start := len(dst)
	total := start + add
	if cap(dst) < total || dst == nil {
		next := make([]T, total, wire.GrowCapacity(cap(dst), total))
		copy(next, dst)
		return next, start
	}
	return dst[:total], start
}

func appendStringArrBodyAs(d []string, b []byte, zero bool, elem uint8) ([]string, bool) {
	if elem == wireString {
		return wire.AppendStringArrBody(d, b, zero)
	}
	return appendLenDelimArrBody(d, b, elem, func(x []byte) string { return decodeStr(x, zero) })
}

func appendBytesArrBodyAs(d [][]byte, b []byte, zero bool, elem uint8) ([][]byte, bool) {
	if elem == wireBytes {
		return wire.AppendBytesArrBody(d, b, zero)
	}
	return appendLenDelimArrBody(d, b, elem, func(x []byte) []byte {
		if zero {
			return x
		}
		return append([]byte{}, x...)
	})
}
