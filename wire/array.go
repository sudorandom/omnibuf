package wire

import (
	"encoding/binary"
	"math"
)

// ParseArrayHeader reads [ElementType][Count] and bounds the count before any
// allocation. Booleans use WireBoolFalse as their element descriptor and one
// byte (0 or 1) per value. WireBoolTrue is a field-tag literal, not a value type.
func ParseArrayHeader(body []byte) (elem uint8, count, pos int, ok bool) {
	if len(body) < 2 {
		return 0, 0, 0, false
	}
	elem = body[0]
	if elem > 15 || elem == WireBoolTrue {
		return 0, 0, 0, false
	}
	c, n := DecodeVarint(body[1:])
	if n == 0 {
		return 0, 0, 0, false
	}
	pos = 1 + n
	remaining := len(body) - pos
	width := 0
	switch elem {
	case WireBoolFalse:
		width = 1
	case WireFixed32, WireFloat32, WireSfixed32:
		width = 4
	case WireFixed64, WireFloat64, WireSfixed64:
		width = 8
	}
	if width > 0 {
		if remaining%width != 0 || c != uint64(remaining/width) {
			return 0, 0, 0, false
		}
	} else if c > uint64(remaining) {
		return 0, 0, 0, false
	}
	return elem, int(c), pos, true
}

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

type intOrUint interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uint | ~int8 | ~int16 | ~int32 | ~int64 | ~int
}

// AppendArrayHeader starts a container and returns its reserved length position.
func AppendArrayHeader(dst []byte, elem uint8, count int) ([]byte, int) {
	pos := len(dst)
	dst = append(dst, 0, 0, elem)
	return AppendVarint(dst, uint64(count)), pos
}
func EncodeStringArr(dst []byte, items []string) []byte {
	p := len(dst)
	dst = append(dst, 0, 0, WireString)
	dst = AppendVarint(dst, uint64(len(items)))
	for _, v := range items {
		dst = AppendVarint(dst, uint64(len(v)))
		dst = append(dst, v...)
	}
	if d := len(dst) - p; d <= 16385 {
		binary.LittleEndian.PutUint16(dst[p:], uint16(d<<2-6))
		return dst
	}
	return BackpatchContainerLen(dst, p)
}
func EncodeBytesArr(dst []byte, items [][]byte) []byte {
	p := len(dst)
	dst = append(dst, 0, 0, WireBytes)
	dst = AppendVarint(dst, uint64(len(items)))
	for _, v := range items {
		dst = AppendVarint(dst, uint64(len(v)))
		dst = append(dst, v...)
	}
	if d := len(dst) - p; d <= 16385 {
		binary.LittleEndian.PutUint16(dst[p:], uint16(d<<2-6))
		return dst
	}
	return BackpatchContainerLen(dst, p)
}
func EncodeBoolArr(dst []byte, items []bool) []byte {
	p := len(dst)
	dst = append(dst, 0, 0, WireBoolFalse)
	dst = AppendVarint(dst, uint64(len(items)))
	for _, v := range items {
		if v {
			dst = append(dst, 1)
		} else {
			dst = append(dst, 0)
		}
	}
	if d := len(dst) - p; d <= 16385 {
		binary.LittleEndian.PutUint16(dst[p:], uint16(d<<2-6))
		return dst
	}
	return BackpatchContainerLen(dst, p)
}
func EncodeUintArr[T intOrUint](dst []byte, items []T) []byte {
	p := len(dst)
	dst = append(dst, 0, 0, WireUint)
	dst = AppendVarint(dst, uint64(len(items)))
	for _, v := range items {
		dst = AppendVarint(dst, uint64(v))
	}
	if d := len(dst) - p; d <= 16385 {
		binary.LittleEndian.PutUint16(dst[p:], uint16(d<<2-6))
		return dst
	}
	return BackpatchContainerLen(dst, p)
}
func EncodeSintArr[T intOrUint](dst []byte, items []T) []byte {
	p := len(dst)
	dst = append(dst, 0, 0, WireSint)
	dst = AppendVarint(dst, uint64(len(items)))
	for _, v := range items {
		dst = AppendVarint(dst, EncodeZigZag(int64(v)))
	}
	if d := len(dst) - p; d <= 16385 {
		binary.LittleEndian.PutUint16(dst[p:], uint16(d<<2-6))
		return dst
	}
	return BackpatchContainerLen(dst, p)
}
func EncodeFixed32Arr[T intOrUint](dst []byte, items []T, elem uint8) []byte {
	p := len(dst)
	dst = append(dst, 0, 0, elem)
	dst = AppendVarint(dst, uint64(len(items)))
	for _, v := range items {
		dst = binary.LittleEndian.AppendUint32(dst, uint32(v))
	}
	if d := len(dst) - p; d <= 16385 {
		binary.LittleEndian.PutUint16(dst[p:], uint16(d<<2-6))
		return dst
	}
	return BackpatchContainerLen(dst, p)
}
func EncodeFixed64Arr[T intOrUint](dst []byte, items []T, elem uint8) []byte {
	p := len(dst)
	dst = append(dst, 0, 0, elem)
	dst = AppendVarint(dst, uint64(len(items)))
	for _, v := range items {
		dst = binary.LittleEndian.AppendUint64(dst, uint64(v))
	}
	if d := len(dst) - p; d <= 16385 {
		binary.LittleEndian.PutUint16(dst[p:], uint16(d<<2-6))
		return dst
	}
	return BackpatchContainerLen(dst, p)
}
func EncodeFloat32Arr(dst []byte, items []float32) []byte {
	p := len(dst)
	dst = append(dst, 0, 0, WireFloat32)
	dst = AppendVarint(dst, uint64(len(items)))
	for _, v := range items {
		dst = binary.LittleEndian.AppendUint32(dst, math.Float32bits(v))
	}
	if d := len(dst) - p; d <= 16385 {
		binary.LittleEndian.PutUint16(dst[p:], uint16(d<<2-6))
		return dst
	}
	return BackpatchContainerLen(dst, p)
}
func EncodeFloat64Arr(dst []byte, items []float64) []byte {
	p := len(dst)
	dst = append(dst, 0, 0, WireFloat64)
	dst = AppendVarint(dst, uint64(len(items)))
	for _, v := range items {
		dst = binary.LittleEndian.AppendUint64(dst, math.Float64bits(v))
	}
	if d := len(dst) - p; d <= 16385 {
		binary.LittleEndian.PutUint16(dst[p:], uint16(d<<2-6))
		return dst
	}
	return BackpatchContainerLen(dst, p)
}

func AppendStringArrBody(dst []string, body []byte, zeroCopy bool) ([]string, bool) {
	if len(body) < 2 || body[0] != WireString {
		return nil, false
	}
	cu, n := DecodeVarint(body[1:])
	p := 1 + n
	if n == 0 || cu > uint64(len(body)-p) {
		return nil, false
	}
	c := int(cu)
	dst, start := growSlice(dst, c)
	for i := 0; i < c; i++ {
		l, n := DecodeVarint(body[p:])
		if n == 0 || l > uint64(len(body)-p-n) {
			return nil, false
		}
		p += n
		dst[start+i] = DecodeStr(body[p:p+int(l)], zeroCopy)
		p += int(l)
	}
	return dst, p == len(body)
}
func AppendBytesArrBody(dst [][]byte, body []byte, zeroCopy bool) ([][]byte, bool) {
	if len(body) < 2 || body[0] != WireBytes {
		return nil, false
	}
	cu, n := DecodeVarint(body[1:])
	p := 1 + n
	if n == 0 || cu > uint64(len(body)-p) {
		return nil, false
	}
	c := int(cu)
	dst, start := growSlice(dst, c)
	for i := 0; i < c; i++ {
		l, n := DecodeVarint(body[p:])
		if n == 0 || l > uint64(len(body)-p-n) {
			return nil, false
		}
		p += n
		v := body[p : p+int(l)]
		if !zeroCopy {
			v = append([]byte{}, v...)
		}
		dst[start+i] = v
		p += int(l)
	}
	return dst, p == len(body)
}
func AppendBoolArrBody(dst []bool, body []byte) ([]bool, bool) {
	e, c, p, ok := ParseArrayHeader(body)
	if !ok || e != WireBoolFalse {
		return nil, false
	}
	dst, start := growSlice(dst, c)
	for i := 0; i < c; i++ {
		v := body[p+i]
		if v > 1 {
			return nil, false
		}
		dst[start+i] = v == 1
	}
	return dst, true
}
func AppendUintArrBody[T intOrUint](dst []T, body []byte) ([]T, bool) {
	if len(body) < 2 || body[0] != WireUint {
		return nil, false
	}
	cu, n := DecodeVarint(body[1:])
	p := 1 + n
	if n == 0 || cu > uint64(len(body)-p) {
		return nil, false
	}
	c := int(cu)
	dst, start := growSlice(dst, c)
	for i := 0; i < c; i++ {
		v, n := DecodeVarint(body[p:])
		if n == 0 {
			return nil, false
		}
		p += n
		dst[start+i] = T(v)
	}
	return dst, p == len(body)
}
func AppendSintArrBody[T intOrUint](dst []T, body []byte) ([]T, bool) {
	if len(body) < 2 || body[0] != WireSint {
		return nil, false
	}
	cu, n := DecodeVarint(body[1:])
	p := 1 + n
	if n == 0 || cu > uint64(len(body)-p) {
		return nil, false
	}
	c := int(cu)
	dst, start := growSlice(dst, c)
	for i := 0; i < c; i++ {
		v, n := DecodeVarint(body[p:])
		if n == 0 {
			return nil, false
		}
		p += n
		dst[start+i] = T(DecodeZigZag(v))
	}
	return dst, p == len(body)
}
func AppendFixed32ArrBody[T intOrUint](dst []T, body []byte) ([]T, bool) {
	e, c, p, ok := ParseArrayHeader(body)
	if !ok || (e != WireFixed32 && e != WireSfixed32 && e != WireFloat32) {
		return nil, false
	}
	dst, start := growSlice(dst, c)
	for i := 0; i < c; i++ {
		v := binary.LittleEndian.Uint32(body[p:])
		p += 4
		if e == WireSfixed32 {
			dst[start+i] = T(int32(v))
		} else {
			dst[start+i] = T(v)
		}
	}
	return dst, true
}
func AppendFixed64ArrBody[T intOrUint](dst []T, body []byte) ([]T, bool) {
	e, c, p, ok := ParseArrayHeader(body)
	if !ok || (e != WireFixed64 && e != WireSfixed64 && e != WireFloat64) {
		return nil, false
	}
	dst, start := growSlice(dst, c)
	for i := 0; i < c; i++ {
		dst[start+i] = T(binary.LittleEndian.Uint64(body[p:]))
		p += 8
	}
	return dst, true
}
func AppendFloat32ArrBody(dst []float32, body []byte) ([]float32, bool) {
	e, c, p, ok := ParseArrayHeader(body)
	if !ok || e != WireFloat32 {
		return nil, false
	}
	dst, start := growSlice(dst, c)
	for i := 0; i < c; i++ {
		dst[start+i] = math.Float32frombits(binary.LittleEndian.Uint32(body[p:]))
		p += 4
	}
	return dst, true
}
func AppendFloat64ArrBody(dst []float64, body []byte) ([]float64, bool) {
	e, c, p, ok := ParseArrayHeader(body)
	if !ok || e != WireFloat64 {
		return nil, false
	}
	dst, start := growSlice(dst, c)
	for i := 0; i < c; i++ {
		dst[start+i] = math.Float64frombits(binary.LittleEndian.Uint64(body[p:]))
		p += 8
	}
	return dst, true
}

func (e *Encoder) WriteStringArr(f uint16, v []string) {
	e.writeTag(f, WireArray)
	e.buf = EncodeStringArr(e.buf, v)
}
func (e *Encoder) WriteUint32Arr(f uint16, v []uint32) {
	e.writeTag(f, WireArray)
	e.buf = EncodeUintArr(e.buf, v)
}
func (e *Encoder) WriteSintArr(f uint16, v []int64) {
	e.writeTag(f, WireArray)
	e.buf = EncodeSintArr(e.buf, v)
}
func (e *Encoder) WriteFloat32Arr(f uint16, v []float32) {
	e.writeTag(f, WireArray)
	var p int
	e.buf, p = AppendArrayHeader(e.buf, WireFloat32, len(v))
	for _, x := range v {
		e.buf = binary.LittleEndian.AppendUint32(e.buf, math.Float32bits(x))
	}
	e.buf = BackpatchContainerLen(e.buf, p)
}
func (e *Encoder) WriteFloat64Arr(f uint16, v []float64) {
	e.writeTag(f, WireArray)
	var p int
	e.buf, p = AppendArrayHeader(e.buf, WireFloat64, len(v))
	for _, x := range v {
		e.buf = binary.LittleEndian.AppendUint64(e.buf, math.Float64bits(x))
	}
	e.buf = BackpatchContainerLen(e.buf, p)
}

// CountedArrLen returns the combined element count for arrays of elemType.
func CountedArrLen(packet []byte, field uint16, elemType uint8) int {
	total := 0
	for pos := 0; ; {
		body, next, match, valid := nextContainerField(packet, pos, field, WireArray, 2)
		if !valid {
			return 0
		}
		if !match {
			return total
		}
		pos = next
		e, c, _, ok := ParseArrayHeader(body)
		if !ok || e != elemType {
			return 0
		}
		total += c
	}
}
func readArrays[T any](dst []T, packet []byte, field uint16, decode func([]T, []byte) ([]T, bool)) ([]T, bool) {
	found := false
	for pos := 0; ; {
		body, next, match, valid := nextContainerField(packet, pos, field, WireArray, 2)
		if !valid {
			return nil, false
		}
		if !match {
			return dst, found
		}
		pos = next
		var ok bool
		dst, ok = decode(dst, body)
		if !ok {
			return nil, false
		}
		found = true
	}
}
func ReadStringArr(p []byte, f uint16) ([]string, bool) {
	return readArrays(nil, p, f, func(d []string, b []byte) ([]string, bool) { return AppendStringArrBody(d, b, false) })
}
func AppendStringArrZeroCopy(d []string, p []byte, f uint16) []string {
	d, _ = readArrays(d, p, f, func(d []string, b []byte) ([]string, bool) { return AppendStringArrBody(d, b, true) })
	return d
}
func ReadUint32Arr(p []byte, f uint16) ([]uint32, bool) {
	return readArrays(nil, p, f, AppendUintArrBody[uint32])
}
func AppendUint32Arr(d []uint32, p []byte, f uint16) []uint32 {
	d, _ = readArrays(d, p, f, AppendUintArrBody[uint32])
	return d
}
func ReadSintArr(p []byte, f uint16) ([]int64, bool) {
	return readArrays(nil, p, f, AppendSintArrBody[int64])
}
func ReadFloat32Arr(p []byte, f uint16) ([]float32, bool) {
	return readArrays(nil, p, f, AppendFloat32ArrBody)
}
func ReadFloat64Arr(p []byte, f uint16) ([]float64, bool) {
	return readArrays(nil, p, f, AppendFloat64ArrBody)
}
func AppendBytesArr(d [][]byte, p []byte, f uint16, zeroCopy bool) [][]byte {
	d, _ = readArrays(d, p, f, func(d [][]byte, b []byte) ([][]byte, bool) { return AppendBytesArrBody(d, b, zeroCopy) })
	return d
}
func AppendBoolArr(d []bool, p []byte, f uint16) []bool {
	d, _ = readArrays(d, p, f, AppendBoolArrBody)
	return d
}
func AppendUint64Arr(d []uint64, p []byte, f uint16) []uint64 {
	d, _ = readArrays(d, p, f, AppendUintArrBody[uint64])
	return d
}
func AppendSint32Arr(d []int32, p []byte, f uint16) []int32 {
	d, _ = readArrays(d, p, f, AppendSintArrBody[int32])
	return d
}
func AppendSint64Arr(d []int64, p []byte, f uint16) []int64 {
	d, _ = readArrays(d, p, f, AppendSintArrBody[int64])
	return d
}
func AppendFixed32Arr(d []uint32, p []byte, f uint16) []uint32 {
	d, _ = readArrays(d, p, f, func(d []uint32, b []byte) ([]uint32, bool) {
		if len(b) < 2 || b[0] != WireFixed32 {
			return nil, false
		}
		return AppendFixed32ArrBody(d, b)
	})
	return d
}
func AppendFixed64Arr(d []uint64, p []byte, f uint16) []uint64 {
	d, _ = readArrays(d, p, f, func(d []uint64, b []byte) ([]uint64, bool) {
		if len(b) < 2 || b[0] != WireFixed64 {
			return nil, false
		}
		return AppendFixed64ArrBody(d, b)
	})
	return d
}
func AppendSfixed32Arr(d []int32, p []byte, f uint16) []int32 {
	d, _ = readArrays(d, p, f, func(d []int32, b []byte) ([]int32, bool) {
		if len(b) < 2 || b[0] != WireSfixed32 {
			return nil, false
		}
		return AppendFixed32ArrBody(d, b)
	})
	return d
}
func AppendSfixed64Arr(d []int64, p []byte, f uint16) []int64 {
	d, _ = readArrays(d, p, f, func(d []int64, b []byte) ([]int64, bool) {
		if len(b) < 2 || b[0] != WireSfixed64 {
			return nil, false
		}
		return AppendFixed64ArrBody(d, b)
	})
	return d
}
func AppendFloat32Arr(d []float32, p []byte, f uint16) []float32 {
	d, _ = readArrays(d, p, f, AppendFloat32ArrBody)
	return d
}
func AppendFloat64Arr(d []float64, p []byte, f uint16) []float64 {
	d, _ = readArrays(d, p, f, AppendFloat64ArrBody)
	return d
}
