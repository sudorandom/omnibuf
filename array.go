package omnibuf

import "github.com/sudorandom/omnibuf/wire"

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
func appendStringArrBody(d []string, b []byte, zero bool) ([]string, bool) {
	return wire.AppendStringArrBody(d, b, zero)
}
func appendBytesArrBody(d [][]byte, b []byte, zero bool) ([][]byte, bool) {
	return wire.AppendBytesArrBody(d, b, zero)
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
