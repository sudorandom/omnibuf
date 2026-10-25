package omnibuf

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
)

// Explicit 4-bit Wire Types (0..15) encoded in the low 4 bits of the field tag:
// Tag = (FieldNumber << 4) | WireType
const (
	WireUint      uint8 = 0  // [Prefix-Varint] (unsigned int: uint32, uint64, enum)
	WireBoolFalse uint8 = 1  // 0 value bytes (false encoded directly in tag)
	WireBoolTrue  uint8 = 2  // 0 value bytes (true encoded directly in tag)
	WireString    uint8 = 3  // [ByteLen (Prefix-Varint)][UTF-8 Bytes]
	WireBytes     uint8 = 4  // [ByteLen (Prefix-Varint)][Raw Bytes]
	WireSubMsg    uint8 = 5  // [ByteLen (Prefix-Varint)][Sub-Packet Bytes]
	WireStringArr uint8 = 6  // [ByteLen (Prefix-Varint)][Count (Prefix-Varint)][Len + UTF-8 Bytes]...
	WireUintArr   uint8 = 7  // [ByteLen (Prefix-Varint)][Count (Prefix-Varint)][Prefix-Varint]...
	WireSubMsgArr uint8 = 8  // [ByteLen (Prefix-Varint)][ElemLen (Prefix-Varint) + Elem Bytes]...
	WireMap       uint8 = 9  // [ByteLen (Prefix-Varint)][MapType (1B: KeyWT<<4|ValWT)][Count (Prefix-Varint)][Key + Value]...
	WireFixed32   uint8 = 10 // [4 Bytes Little-Endian] (float32 / fixed32 / sfixed32)
	WireFixed64   uint8 = 11 // [8 Bytes Little-Endian] (float64 / fixed64 / sfixed64)
	WireSint      uint8 = 12 // [Prefix-Varint] (ZigZag signed int: int32, int64, sint32, sint64)
	WireSintArr   uint8 = 13 // [ByteLen (Prefix-Varint)][Count (Prefix-Varint)][ZigZag Prefix-Varint]...
	WireFixedArr  uint8 = 14 // [ByteLen (Prefix-Varint)][ElemType (1B: WireFixed32=10 or WireFixed64=11)][Fixed Elements...]
	WireExtended  uint8 = 15 // [ByteLen (Prefix-Varint)][ExtType (1B: 0..255)][Extended Payload...] (Reserved escape hatch)
)

// MaxNestingDepth is the maximum allowed sub-message / array nesting depth (prevents stack-overflow bombs).
const MaxNestingDepth = 32

// EncodeZigZag maps signed integers to unsigned integers so small negative values stay compact.
func EncodeZigZag(n int64) uint64 {
	return uint64((n << 1) ^ (n >> 63))
}

// DecodeZigZag maps a ZigZag-encoded unsigned integer back to a signed int64.
func DecodeZigZag(u uint64) int64 {
	return int64(u>>1) ^ -int64(u&1)
}

// AppendVarint appends a branchless Prefix-Varint (TZCNT) to dst and returns the extended slice.
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

// DecodeVarint decodes a Prefix-Varint from b using hardware TZCNT
// and returns (value, bytesRead). Fast-pathed for 1-byte values so it inlines at every call site.
func DecodeVarint(b []byte) (uint64, int) {
	if len(b) > 0 && (b[0]&1) != 0 {
		return uint64(b[0] >> 1), 1
	}
	return decodeVarintMulti(b)
}

//go:noinline
func decodeVarintMulti(b []byte) (uint64, int) {
	if len(b) < 2 {
		return 0, 0
	}
	b0 := b[0]
	if (b0 & 2) != 0 {
		return uint64(binary.LittleEndian.Uint16(b[:2]) >> 2), 2
	}
	if (b0 & 4) != 0 {
		if len(b) < 4 {
			return 0, 0
		}
		return uint64(binary.LittleEndian.Uint32(b[:4]) >> 3), 4
	}
	if len(b) < 9 {
		return 0, 0
	}
	return binary.LittleEndian.Uint64(b[1:9]), 9
}

// ReadVarintAt decodes a Prefix-Varint from b[pos:] without constructing a sub-slice.
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
	subLen := len(dst) - (lenPos + 2)
	if uint(subLen) <= 16383 {
		u := uint16(subLen<<2) | 2
		dst[lenPos] = byte(u)
		dst[lenPos+1] = byte(u >> 8)
		return dst
	}
	return backpatchLargeContainerLen(dst, lenPos, uint64(subLen))
}

//go:noinline
func backpatchLargeContainerLen(dst []byte, lenPos int, subLen uint64) []byte {
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
		} else {
			pos += 9
		}
		return pos, pos <= n
	case WireBoolFalse, WireBoolTrue:
		return pos, true
	case WireString, WireBytes, WireSubMsg, WireStringArr, WireUintArr, WireSubMsgArr, WireMap, WireSintArr, WireFixedArr, WireExtended:
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
	case WireFixed32:
		pos += 4
		return pos, pos <= n
	case WireFixed64:
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
	if wireType == WireFixedArr || wireType == WireExtended {
		bLen, bn := DecodeVarint(b[pos:])
		if bn == 0 || bLen < 1 || bLen > uint64(len(b)-(pos+bn)) {
			return 0, fmt.Errorf("omnibuf: invalid container length at %d", pos)
		}
		pos += bn
		if wireType == WireFixedArr {
			switch elemType := b[pos]; elemType {
			case WireFixed32:
				if (bLen-1)%4 != 0 {
					return 0, fmt.Errorf("omnibuf: WireFixedArr(Fixed32) payload %d is not a multiple of 4 at %d", bLen-1, pos)
				}
			case WireFixed64:
				if (bLen-1)%8 != 0 {
					return 0, fmt.Errorf("omnibuf: WireFixedArr(Fixed64) payload %d is not a multiple of 8 at %d", bLen-1, pos)
				}
			default:
				return 0, fmt.Errorf("omnibuf: invalid WireFixedArr ElemType %d at %d", elemType, pos)
			}
		}
		return pos + int(bLen), nil
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
		var fnum uint16
		var wt uint8
		b0 := packet[pos]
		if (b0 & 1) != 0 {
			fnum = uint16(b0 >> 5)
			wt = (b0 >> 1) & 0x0F
			pos++
		} else if (b0 & 2) != 0 {
			if pos+2 > n {
				return nil, 0, false
			}
			u := binary.LittleEndian.Uint16(packet[pos:pos+2]) >> 2
			fnum = u >> 4
			wt = uint8(u & 0x0F)
			pos += 2
		} else {
			tag, tn := decodeVarintMulti(packet[pos:])
			if tn == 0 || tag>>4 > math.MaxUint16 {
				return nil, 0, false
			}
			fnum = uint16(tag >> 4)
			wt = uint8(tag & 0x0F)
			pos += tn
		}

		if fnum == targetField {
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
			} else {
				pos += 9
			}
			if pos > n {
				return nil, 0, false
			}
		case WireBoolFalse, WireBoolTrue:
		case WireString, WireBytes, WireSubMsg, WireStringArr, WireUintArr, WireSubMsgArr, WireMap, WireSintArr, WireFixedArr, WireExtended:
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
		case WireFixed32:
			pos += 4
			if pos > n {
				return nil, 0, false
			}
		case WireFixed64:
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

// nextContainerField scans packet starting at pos for the next occurrence of targetField.
// When found, it validates wt == expWT and byteLen >= minLen, returning the container body slice,
// the position immediately after the container, matched=true, and valid=true.
// When packet is exhausted cleanly without another match, it returns matched=false, valid=true.
func nextContainerField(packet []byte, pos int, targetField uint16, expWT uint8, minLen uint64) (body []byte, nextPos int, matched bool, valid bool) {
	n := len(packet)
	for pos < n {
		var fnum uint16
		var wt uint8
		b0 := packet[pos]
		if (b0 & 1) != 0 {
			fnum = uint16(b0 >> 5)
			wt = (b0 >> 1) & 0x0F
			pos++
		} else if (b0 & 2) != 0 {
			if pos+2 > n {
				return nil, 0, false, false
			}
			u := binary.LittleEndian.Uint16(packet[pos:pos+2]) >> 2
			fnum = u >> 4
			wt = uint8(u & 0x0F)
			pos += 2
		} else {
			tag, tn := decodeVarintMulti(packet[pos:])
			if tn == 0 || tag>>4 > math.MaxUint16 {
				return nil, 0, false, false
			}
			fnum = uint16(tag >> 4)
			wt = uint8(tag & 0x0F)
			pos += tn
		}

		if fnum == targetField {
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

func sortUint32Slice(s []uint32) {
	if len(s) <= 32 {
		for i := 1; i < len(s); i++ {
			v := s[i]
			j := i - 1
			for j >= 0 && s[j] > v {
				s[j+1] = s[j]
				j--
			}
			s[j+1] = v
		}
		return
	}
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
}

// Encoder encodes OmniBuf messages into a reusable contiguous byte buffer in a single forward pass.
type Encoder struct {
	buf      []byte
	lenStack [MaxNestingDepth * 2]int
	depth    int
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
	e.buf = appendTagMulti(e.buf, tag)
}

//go:noinline
func appendTagMulti(b []byte, tag uint32) []byte {
	if tag <= 16383 {
		u := uint16(tag<<2) | 2
		return append(b, byte(u), byte(u>>8))
	}
	return AppendVarint(b, uint64(tag))
}
