package wire

import (
	"encoding/binary"
	"math"
	"unicode/utf8"
	"unsafe"
)

// ============================================================================
// Scalar Wire Types (WireUint, WireSint, WireBool*, WireFixed32/64, WireString, WireBytes)
// ============================================================================

// WriteUint writes an unsigned integer field (WireUint).
func (e *Encoder) WriteUint(fieldNum uint16, v uint64) {
	e.writeTag(fieldNum, WireUint)
	e.buf = AppendVarint(e.buf, v)
}

// WriteInt writes a ZigZag-encoded signed integer field (WireSint).
func (e *Encoder) WriteInt(fieldNum uint16, v int64) {
	e.writeTag(fieldNum, WireSint)
	e.buf = AppendVarint(e.buf, EncodeZigZag(v))
}

// WriteBool writes a boolean field using 0 payload bytes (WireBoolTrue or WireBoolFalse).
func (e *Encoder) WriteBool(fieldNum uint16, v bool) {
	wt := WireBoolFalse
	if v {
		wt = WireBoolTrue
	}
	e.writeTag(fieldNum, wt)
}

// WriteFixed32 writes a 4-byte little-endian uint32 field (WireFixed32).
func (e *Encoder) WriteFixed32(fieldNum uint16, v uint32) {
	e.writeTag(fieldNum, WireFixed32)
	e.buf = append(e.buf, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

// WriteFloat32 writes a 4-byte IEEE-754 float32 field (WireFixed32).
func (e *Encoder) WriteFloat32(fieldNum uint16, v float32) {
	e.writeTag(fieldNum, WireFloat32)
	e.buf = binary.LittleEndian.AppendUint32(e.buf, math.Float32bits(v))
}

// WriteFixed64 writes an 8-byte little-endian uint64 field (WireFixed64).
func (e *Encoder) WriteFixed64(fieldNum uint16, v uint64) {
	e.writeTag(fieldNum, WireFixed64)
	e.buf = append(e.buf,
		byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
		byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56),
	)
}

// WriteFloat64 writes an 8-byte IEEE-754 float64 field (WireFixed64).
func (e *Encoder) WriteFloat64(fieldNum uint16, v float64) {
	e.writeTag(fieldNum, WireFloat64)
	e.buf = binary.LittleEndian.AppendUint64(e.buf, math.Float64bits(v))
}

// WriteString writes a UTF-8 string field (WireString).
func (e *Encoder) WriteString(fieldNum uint16, s string) {
	e.writeTag(fieldNum, WireString)
	e.buf = AppendVarint(e.buf, uint64(len(s)))
	e.buf = append(e.buf, s...)
}

// WriteBytes writes a raw byte slice field (WireBytes).
func (e *Encoder) WriteBytes(fieldNum uint16, b []byte) {
	e.writeTag(fieldNum, WireBytes)
	e.buf = AppendVarint(e.buf, uint64(len(b)))
	e.buf = append(e.buf, b...)
}

// ValidUTF8 reports whether b is valid UTF-8, as required for WireString payloads.
func ValidUTF8(b []byte) bool {
	// Short ASCII identifiers are common on the wire. Overlapping word loads
	// cover every byte without reading outside the string. Non-ASCII input
	// still goes through the standard UTF-8 validator.
	n := len(b)
	switch {
	case n >= 8 && n <= 64:
		var bits uint64
		for i := 0; i < n-8; i += 8 {
			bits |= binary.LittleEndian.Uint64(b[i:])
		}
		bits |= binary.LittleEndian.Uint64(b[n-8:])
		if bits&0x8080808080808080 == 0 {
			return true
		}
	case n >= 4 && n < 8:
		if (binary.LittleEndian.Uint32(b)|binary.LittleEndian.Uint32(b[n-4:]))&0x80808080 == 0 {
			return true
		}
	case n < 4:
		var bits byte
		for _, c := range b {
			bits |= c
		}
		if bits < utf8.RuneSelf {
			return true
		}
	}
	return utf8.Valid(b)
}

// DecodeStr converts b into a Go string (zero-copy via unsafe.String if zeroCopy is true).
func DecodeStr(b []byte, zeroCopy bool) string {
	if len(b) == 0 {
		return ""
	}
	if zeroCopy {
		return unsafe.String(&b[0], len(b))
	}
	return string(b)
}

// ReadUint reads a uint64 field with 0 heap allocations.
func ReadUint(packet []byte, fieldNum uint16) (uint64, bool) {
	s, wt, ok := LocateField(packet, fieldNum)
	if !ok || wt != WireUint {
		return 0, false
	}
	v, n := DecodeVarint(s)
	return v, n > 0
}

// ReadInt reads a ZigZag-encoded signed int64 field with 0 heap allocations.
func ReadInt(packet []byte, fieldNum uint16) (int64, bool) {
	s, wt, ok := LocateField(packet, fieldNum)
	if !ok || wt != WireSint {
		return 0, false
	}
	u, n := DecodeVarint(s)
	if n == 0 {
		return 0, false
	}
	return DecodeZigZag(u), true
}

// ReadBool reads a boolean field directly from the 4-bit wire type (0 value bytes).
func ReadBool(packet []byte, fieldNum uint16) (bool, bool) {
	_, wt, ok := LocateField(packet, fieldNum)
	if !ok || (wt != WireBoolTrue && wt != WireBoolFalse) {
		return false, false
	}
	return wt == WireBoolTrue, true
}

// ReadFloat32 reads a float32 field with 0 heap allocations.
func ReadFloat32(packet []byte, fieldNum uint16) (float32, bool) {
	s, wt, ok := LocateField(packet, fieldNum)
	if !ok || wt != WireFloat32 || len(s) < 4 {
		return 0, false
	}
	return math.Float32frombits(binary.LittleEndian.Uint32(s[:4])), true
}

// ReadFloat64 reads a float64 field with 0 heap allocations.
func ReadFloat64(packet []byte, fieldNum uint16) (float64, bool) {
	s, wt, ok := LocateField(packet, fieldNum)
	if !ok || wt != WireFloat64 || len(s) < 8 {
		return 0, false
	}
	return math.Float64frombits(binary.LittleEndian.Uint64(s[:8])), true
}

// ReadStringBytes returns the raw UTF-8 byte slice of a string field.
func ReadStringBytes(packet []byte, fieldNum uint16) ([]byte, bool) {
	s, wt, ok := LocateField(packet, fieldNum)
	if !ok || wt != WireString || len(s) == 0 {
		return nil, false
	}
	b0 := s[0]
	if (b0 & 1) != 0 {
		strLen := int(b0 >> 1)
		if 1+strLen > len(s) || !ValidUTF8(s[1:1+strLen]) {
			return nil, false
		}
		return s[1 : 1+strLen], true
	}
	sLen, sn := DecodeVarint(s)
	if sn == 0 || sLen > uint64(len(s)-sn) || !ValidUTF8(s[sn:sn+int(sLen)]) {
		return nil, false
	}
	return s[sn : sn+int(sLen)], true
}

// ReadString reads a string field, allocating a safe Go string copy.
func ReadString(packet []byte, fieldNum uint16) (string, bool) {
	b, ok := ReadStringBytes(packet, fieldNum)
	if !ok {
		return "", false
	}
	return string(b), true
}

// ReadStringZeroCopy reads a string field with zero allocations using unsafe.String.
func ReadStringZeroCopy(packet []byte, fieldNum uint16) (string, bool) {
	b, ok := ReadStringBytes(packet, fieldNum)
	if !ok || len(b) == 0 {
		return "", ok
	}
	return unsafe.String(&b[0], len(b)), true
}

// ReadBytes returns the raw byte slice of a WireBytes field.
func ReadBytes(packet []byte, fieldNum uint16) ([]byte, bool) {
	s, wt, ok := LocateField(packet, fieldNum)
	if !ok || wt != WireBytes {
		return nil, false
	}
	bLen, bn := DecodeVarint(s)
	if bn == 0 || bLen > uint64(len(s)-bn) {
		return nil, false
	}
	return s[bn : bn+int(bLen)], true
}

// ReadFixed32 reads a 4-byte little-endian uint32 field (WireFixed32) with 0 heap allocations.
func ReadFixed32(packet []byte, fieldNum uint16) (uint32, bool) {
	s, wt, ok := LocateField(packet, fieldNum)
	if !ok || wt != WireFixed32 || len(s) < 4 {
		return 0, false
	}
	return binary.LittleEndian.Uint32(s[:4]), true
}

// ReadFixed64 reads an 8-byte little-endian uint64 field (WireFixed64) with 0 heap allocations.
func ReadFixed64(packet []byte, fieldNum uint16) (uint64, bool) {
	s, wt, ok := LocateField(packet, fieldNum)
	if !ok || wt != WireFixed64 || len(s) < 8 {
		return 0, false
	}
	return binary.LittleEndian.Uint64(s[:8]), true
}

// ReadSfixed32 reads a 4-byte little-endian signed int32 field (WireSfixed32) with 0 heap allocations.
func ReadSfixed32(packet []byte, fieldNum uint16) (int32, bool) {
	s, wt, ok := LocateField(packet, fieldNum)
	if !ok || wt != WireSfixed32 || len(s) < 4 {
		return 0, false
	}
	return int32(binary.LittleEndian.Uint32(s[:4])), true
}

// ReadSfixed64 reads an 8-byte little-endian signed int64 field (WireSfixed64) with 0 heap allocations.
func ReadSfixed64(packet []byte, fieldNum uint16) (int64, bool) {
	s, wt, ok := LocateField(packet, fieldNum)
	if !ok || wt != WireSfixed64 || len(s) < 8 {
		return 0, false
	}
	return int64(binary.LittleEndian.Uint64(s[:8])), true
}
