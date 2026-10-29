// Package wire provides low-level OmniBuf wire format primitives, O(1) zero-decode
// field hoppers, the streaming Encoder, and runtime helpers for protoc-gen-omnibuf.
package wire

import (
	"encoding/binary"
	"fmt"
)

// Explicit 4-bit Wire Types (0..15) encoded in the low 4 bits of the field tag:
// Tag = (FieldNumber << 4) | WireType
const (
	WireUint      uint8 = 0
	WireSint      uint8 = 1
	WireBoolFalse uint8 = 2
	WireBoolTrue  uint8 = 3
	WireFixed32   uint8 = 4
	WireFixed64   uint8 = 5
	WireSfixed32  uint8 = 6
	WireSfixed64  uint8 = 7
	WireFloat32   uint8 = 8
	WireFloat64   uint8 = 9
	WireString    uint8 = 10
	WireBytes     uint8 = 11
	WireSubMsg    uint8 = 12
	WireArray     uint8 = 13
	WireMap       uint8 = 14
	WireExtended  uint8 = 15
)

const maxNestingDepth = 32

// EncodeZigZag maps signed integers to unsigned integers so small negative values stay compact.
func EncodeZigZag(n int64) uint64 {
	return uint64((n << 1) ^ (n >> 63))
}

// DecodeZigZag maps a ZigZag-encoded unsigned integer back to a signed int64.
func DecodeZigZag(u uint64) int64 {
	return int64(u>>1) ^ -int64(u&1)
}

// AppendVarint appends a Prefix-Varint to dst and returns the extended slice.
// Fast-pathed for 1-byte values (0..127) so the Go compiler inlines it at every call site.
func AppendVarint(dst []byte, v uint64) []byte {
	if v <= 127 {
		return append(dst, byte(v<<1)|1)
	}
	return appendVarintMulti(dst, v)
}

//go:noinline
func appendVarintMulti(dst []byte, v uint64) []byte {
	if v <= 16383 {
		u := uint16(v<<2) | 2
		return append(dst, byte(u), byte(u>>8))
	}
	if v <= 536870911 {
		u := uint32(v<<3) | 4
		return append(dst, byte(u), byte(u>>8), byte(u>>16), byte(u>>24))
	}
	return append(dst,
		0x00,
		byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
		byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56),
	)
}

// DecodeVarint decodes a Prefix-Varint from b using first-byte bit tests
// and returns (value, bytesRead).
// Keep this hot path inlineable; just go-inline-check guards the compiler decision.
func DecodeVarint(b []byte) (v uint64, n int) {
	if len(b) == 0 {
		return
	}
	b0 := b[0]
	if b0&1 != 0 {
		return uint64(b0 >> 1), 1
	}
	if b0&2 != 0 && len(b) >= 2 {
		return uint64(binary.LittleEndian.Uint16(b) >> 2), 2
	}
	if b0&4 != 0 && len(b) >= 4 {
		return uint64(binary.LittleEndian.Uint32(b) >> 3), 4
	}
	// A nine-byte varint must start with zero; other low-three-bit-zero prefixes are reserved.
	if b0 == 0 && len(b) >= 9 {
		return binary.LittleEndian.Uint64(b[1:]), 9
	}
	return
}

func decodeVarintMulti(b []byte) (uint64, int) {
	return DecodeVarint(b)
}

// ReadVarintAt decodes a Prefix-Varint from b[pos:] without constructing a sub-slice.
// It performs no validation: callers MUST already know that b[pos:] holds a
// complete, well-formed varint (use DecodeVarint for untrusted input).
func ReadVarintAt(b []byte, pos int) (uint64, int) {
	b0 := b[pos]
	if (b0 & 1) != 0 {
		return uint64(b0 >> 1), pos + 1
	}
	if (b0 & 2) != 0 {
		return uint64(binary.LittleEndian.Uint16(b[pos:]) >> 2), pos + 2
	}
	if (b0 & 4) != 0 {
		return uint64(binary.LittleEndian.Uint32(b[pos:]) >> 3), pos + 4
	}
	return binary.LittleEndian.Uint64(b[pos+1:]), pos + 9
}

// PutOverlong2BVarint encodes a length (0..16383) as an explicit 2-byte Prefix-Varint
// (TZCNT=1: uint16(v<<2)|2) for single-pass sub-message and container length backpatching.
func PutOverlong2BVarint(dst []byte, v uint16) {
	binary.LittleEndian.PutUint16(dst[:2], (v<<2)|2)
}

// BackpatchContainerLen fills in the container byte length at dst[lenPos:lenPos+2]
// for the common fast path (subLen <= 16383). If subLen > 16383, it expands the
// reserved 2-byte slot to a 4-byte Prefix-Varint (up to 512 MB) or 9-byte Prefix-Varint
// by shifting the container body right in place.
func BackpatchContainerLen(dst []byte, lenPos int) []byte {
	if b := dst[lenPos:]; len(b) <= 16385 {
		binary.LittleEndian.PutUint16(b, uint16(len(b)<<2-6))
		return dst
	}
	return backpatchLargeContainerLen(dst, lenPos)
}

//go:noinline
func backpatchLargeContainerLen(dst []byte, lenPos int) []byte {
	subLen := uint64(len(dst) - (lenPos + 2))
	if subLen <= 536870911 {
		dst = append(dst, 0, 0)
		copy(dst[lenPos+4:], dst[lenPos+2:len(dst)-2])
		binary.LittleEndian.PutUint32(dst[lenPos:lenPos+4], uint32(subLen<<3)|4)
		return dst
	}
	dst = append(dst, 0, 0, 0, 0, 0, 0, 0)
	copy(dst[lenPos+9:], dst[lenPos+2:len(dst)-7])
	dst[lenPos] = 0
	binary.LittleEndian.PutUint64(dst[lenPos+1:lenPos+9], subLen)
	return dst
}

// fastSkipValue advances pos past a field value of wire type wt in O(1) time without error string allocation.
func fastSkipValue(packet []byte, pos int, wt uint8) (int, bool) {
	n := len(packet)
	switch wt {
	case WireUint, WireSint:
		if pos >= n {
			return 0, false
		}
		vb := packet[pos]
		if (vb & 1) != 0 {
			pos++
		} else if (vb & 2) != 0 {
			pos += 2
		} else if (vb & 4) != 0 {
			pos += 4
		} else if vb == 0 {
			pos += 9
		} else {
			return 0, false
		}
		return pos, pos <= n
	case WireBoolFalse, WireBoolTrue:
		return pos, true
	case WireString, WireBytes, WireSubMsg, WireArray, WireMap, WireExtended:
		if pos >= n {
			return 0, false
		}
		lb := packet[pos]
		if (lb & 1) != 0 {
			pos += 1 + int(lb>>1)
			return pos, pos <= n
		}
		if (lb & 2) != 0 {
			if pos+2 > n {
				return 0, false
			}
			pos += 2 + int(binary.LittleEndian.Uint16(packet[pos:pos+2])>>2)
			return pos, pos <= n
		}
		return skipLargeContainer(packet, pos)
	case WireFixed32, WireFloat32, WireSfixed32:
		pos += 4
		return pos, pos <= n
	case WireFixed64, WireFloat64, WireSfixed64:
		pos += 8
		return pos, pos <= n
	default:
		return 0, false
	}
}

//go:noinline
func skipLargeContainer(packet []byte, pos int) (int, bool) {
	bLen, bn := decodeVarintMulti(packet[pos:])
	if bn == 0 || bLen > uint64(len(packet)-(pos+bn)) {
		return 0, false
	}
	pos += bn + int(bLen)
	return pos, pos <= len(packet)
}

// SkipValue advances pos past a single field payload of the given 4-bit wireType (0..15),
// performing full bounds validation in O(1) time without looping inside containers.
func SkipValue(b []byte, pos int, wireType uint8) (int, error) {
	if pos < 0 || pos > len(b) {
		return 0, fmt.Errorf("omnibuf: position %d out of bounds", pos)
	}
	if wireType == WireArray || wireType == WireMap || wireType == WireExtended {
		length, ln := DecodeVarint(b[pos:])
		if ln == 0 || length < 1 || length > uint64(len(b)-pos-ln) {
			return 0, fmt.Errorf("omnibuf: invalid container length")
		}
		end := pos + ln + int(length)
		switch wireType {
		case WireArray:
			if _, _, _, ok := ParseArrayHeader(b[pos+ln : end]); !ok {
				return 0, fmt.Errorf("omnibuf: invalid array header")
			}
		case WireMap:
			if _, _, _, _, ok := ParseMapHeader(b[pos+ln : end]); !ok {
				return 0, fmt.Errorf("omnibuf: invalid map header")
			}
		}
		return end, nil
	}

	nextPos, ok := fastSkipValue(b, pos, wireType)
	if !ok {
		return 0, fmt.Errorf("omnibuf: truncated or invalid wireType %d at %d", wireType, pos)
	}
	return nextPos, nil
}

// LocateField locates targetField inside packet by hopping across field headers in O(1) time per field
// using TZCNT length prefixes (without parsing inside preceding strings, sub-messages, or arrays).
// To support raw message concatenation (append(msgA, msgB...)) with Protobuf "last write wins"
// semantics, it scans all field headers in the message and returns the last matching occurrence.
func LocateField(packet []byte, targetField uint16) (valSlice []byte, wireType uint8, ok bool) {
	pos := 0
	n := len(packet)
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
				return nil, 0, false
			}
			u := binary.LittleEndian.Uint16(packet[pos:pos+2]) >> 2
			fnum = uint32(u >> 4)
			wt = uint8(u & 0x0F)
			pos += 2
		} else {
			tag, tn := decodeVarintMulti(packet[pos:])
			if tn == 0 || tag>>4 > 536870911 {
				return nil, 0, false
			}
			fnum = uint32(tag >> 4)
			wt = uint8(tag & 0x0F)
			pos += tn
		}

		if fnum == 0 && wt != WireSubMsg {
			return nil, 0, false
		}
		if fnum == uint32(targetField) {
			valSlice = packet[pos:]
			wireType = wt
			ok = true
		}

		switch wt {
		case WireUint, WireSint:
			if pos >= n {
				return nil, 0, false
			}
			vb := packet[pos]
			if (vb & 1) != 0 {
				pos++
			} else if (vb & 2) != 0 {
				pos += 2
			} else if (vb & 4) != 0 {
				pos += 4
			} else if vb == 0 {
				pos += 9
			} else {
				return nil, 0, false
			}
			if pos > n {
				return nil, 0, false
			}
		case WireBoolFalse, WireBoolTrue:
		case WireString, WireBytes, WireSubMsg:
			if pos >= n {
				return nil, 0, false
			}
			lb := packet[pos]
			if (lb & 1) != 0 {
				pos += 1 + int(lb>>1)
			} else if (lb & 2) != 0 {
				if pos+2 > n {
					return nil, 0, false
				}
				pos += 2 + int(binary.LittleEndian.Uint16(packet[pos:pos+2])>>2)
			} else {
				bLen, bn := decodeVarintMulti(packet[pos:])
				if bn == 0 || bLen > uint64(n-(pos+bn)) {
					return nil, 0, false
				}
				pos += bn + int(bLen)
			}
			if pos > n {
				return nil, 0, false
			}
		case WireArray, WireMap, WireExtended:
			next, err := SkipValue(packet, pos, wt)
			if err != nil {
				return nil, 0, false
			}
			pos = next
		case WireFixed32, WireFloat32, WireSfixed32:
			pos += 4
			if pos > n {
				return nil, 0, false
			}
		case WireFixed64, WireFloat64, WireSfixed64:
			pos += 8
			if pos > n {
				return nil, 0, false
			}
		default:
			return nil, 0, false
		}
	}
	return valSlice, wireType, ok
}

func nextContainerField(packet []byte, pos int, targetField uint16, expWT uint8, minLen uint64) (body []byte, nextPos int, matched bool, valid bool) {
	n := len(packet)
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
				return nil, 0, false, false
			}
			u := binary.LittleEndian.Uint16(packet[pos:pos+2]) >> 2
			fnum = uint32(u >> 4)
			wt = uint8(u & 0x0F)
			pos += 2
		} else {
			tag, tn := decodeVarintMulti(packet[pos:])
			if tn == 0 || tag>>4 > 536870911 {
				return nil, 0, false, false
			}
			fnum = uint32(tag >> 4)
			wt = uint8(tag & 0x0F)
			pos += tn
		}

		if fnum == 0 && wt != WireSubMsg {
			return nil, 0, false, false
		}
		if fnum == uint32(targetField) {
			if wt != expWT {
				return nil, 0, false, false
			}
			bLen, bn := DecodeVarint(packet[pos:])
			if bn == 0 || bLen < minLen || bLen > uint64(n-(pos+bn)) {
				return nil, 0, false, false
			}
			pos += bn
			return packet[pos : pos+int(bLen)], pos + int(bLen), true, true
		}

		var err error
		pos, err = SkipValue(packet, pos, wt)
		if err != nil {
			return nil, 0, false, false
		}
	}
	return nil, n, false, true
}

// Encoder encodes OmniBuf messages into a reusable contiguous byte buffer in a single forward pass.
type Encoder struct {
	buf        []byte
	lenStack   [maxNestingDepth * 2]int
	depth      int
	countStack [maxNestingDepth * 2]int
}

// NewEncoder creates an Encoder with the specified initial buffer capacity.
func NewEncoder(capacity int) *Encoder {
	return &Encoder{
		buf: make([]byte, 0, capacity),
	}
}

// Reset clears the encoder buffer for zero-allocation reuse.
func (e *Encoder) Reset() {
	e.buf = e.buf[:0]
	e.depth = 0
}

// Bytes returns the encoded packet bytes.
func (e *Encoder) Bytes() []byte {
	return e.buf
}

// Finish returns the encoded message bytes.
func (e *Encoder) Finish() []byte {
	return e.buf
}

// WriteRaw appends pre-encoded raw bytes (such as a pre-encoded Field #0 SchemaMeta block) to e.
func (e *Encoder) WriteRaw(raw []byte) {
	e.buf = append(e.buf, raw...)
}

func (e *Encoder) writeTag(fieldNum uint16, wireType uint8) {
	tag := (uint32(fieldNum) << 4) | uint32(wireType)
	if tag <= 127 {
		e.buf = append(e.buf, byte(tag<<1)|1)
		return
	}
	if tag <= 16383 {
		u := uint16(tag<<2) | 2
		e.buf = append(e.buf, byte(u), byte(u>>8))
		return
	}
	e.buf = appendVarintMulti(e.buf, uint64(tag))
}

// WriteField0Fields writes Field #0 (Tag = 0x19, WireSubMsg = 12) mapping FieldNumber -> FieldName.
func (e *Encoder) WriteField0Fields(fields map[uint32]string) {
	if len(fields) == 0 {
		return
	}
	e.buf = append(e.buf, byte(((0<<4)|WireSubMsg)<<1)|1)
	lenPos := len(e.buf)
	e.buf = append(e.buf, 0, 0)
	e.buf = append(e.buf, byte(((2<<4)|WireMap)<<1)|1)
	mLenPos := len(e.buf)
	e.buf = append(e.buf, 0, 0, WireUint, WireString)
	e.buf = AppendVarint(e.buf, uint64(len(fields)))
	for k, name := range fields {
		e.buf = AppendVarint(e.buf, uint64(k))
		e.buf = AppendVarint(e.buf, uint64(len(name)))
		e.buf = append(e.buf, name...)
	}
	e.buf = BackpatchContainerLen(e.buf, mLenPos)
	e.buf = BackpatchContainerLen(e.buf, lenPos)
}

// WriteField0Schema writes Field #0 (Tag = 0x19, WireSubMsg = 12) mapping field numbers 1..len(names)
// to names[0..len(names)-1].
func (e *Encoder) WriteField0Schema(names []string) {
	fields := make(map[uint32]string, len(names))
	for i, name := range names {
		if name != "" {
			fields[uint32(i+1)] = name
		}
	}
	e.WriteField0Fields(fields)
}

// PatchReservedVarint fills a 1-byte reserved varint slot and expands it if value > 127.
func PatchReservedVarint(dst []byte, pos int, value uint64) []byte {
	if value <= 127 {
		dst[pos] = byte(value<<1) | 1
		return dst
	}
	width := 2
	if value > 536870911 {
		width = 9
	} else if value > 16383 {
		width = 4
	}
	oldLen := len(dst)
	dst = append(dst, make([]byte, width-1)...)
	copy(dst[pos+width:], dst[pos+1:oldLen])
	switch width {
	case 2:
		binary.LittleEndian.PutUint16(dst[pos:pos+2], uint16(value<<2)|2)
	case 4:
		binary.LittleEndian.PutUint32(dst[pos:pos+4], uint32(value<<3)|4)
	default:
		dst[pos] = 0
		binary.LittleEndian.PutUint64(dst[pos+1:pos+9], value)
	}
	return dst
}
