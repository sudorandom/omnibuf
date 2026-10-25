// Package omnibuf implements the OmniBuf high-throughput binary wire format.
//
// It supports three usage modes in Go:
//   - Plain Go structs annotated with `omnibuf:"<fieldNum>[,name=<name>][,wire=<wireType>][,omitempty]"`
//   - Existing protoc-gen-go generated structs (*pb.Message) and dynamicpb.Message via Marshal and Unmarshal
//   - Generated single-pass codecs and Opaque zero-decode lazy readers via protoc-gen-omnibuf (using github.com/sudorandom/omnibuf/wire)
package omnibuf

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"unsafe"
)

// Explicit 4-bit Wire Types (0..15) encoded in the low 4 bits of the field tag:
// Tag = (FieldNumber << 4) | WireType
const (
	wireUint      uint8 = 0  // [Prefix-Varint] (unsigned int: uint32, uint64, enum)
	wireBoolFalse uint8 = 1  // 0 value bytes (false encoded directly in tag)
	wireBoolTrue  uint8 = 2  // 0 value bytes (true encoded directly in tag)
	wireString    uint8 = 3  // [ByteLen (Prefix-Varint)][UTF-8 Bytes]
	wireBytes     uint8 = 4  // [ByteLen (Prefix-Varint)][Raw Bytes]
	wireSubMsg    uint8 = 5  // [ByteLen (Prefix-Varint)][Sub-Packet Bytes]
	wireStringArr uint8 = 6  // [ByteLen (Prefix-Varint)][Count (Prefix-Varint)][Len + UTF-8 Bytes]...
	wireUintArr   uint8 = 7  // [ByteLen (Prefix-Varint)][Count (Prefix-Varint)][Prefix-Varint]...
	wireSubMsgArr uint8 = 8  // [ByteLen (Prefix-Varint)][ElemLen (Prefix-Varint) + Elem Bytes]...
	wireMap       uint8 = 9  // [ByteLen (Prefix-Varint)][MapType (1B: KeyWT<<4|ValWT)][Count (Prefix-Varint)][Key + Value]...
	wireFixed32   uint8 = 10 // [4 Bytes Little-Endian] (float32 / fixed32 / sfixed32)
	wireFixed64   uint8 = 11 // [8 Bytes Little-Endian] (float64 / fixed64 / sfixed64)
	wireSint      uint8 = 12 // [Prefix-Varint] (ZigZag signed int: int32, int64, sint32, sint64)
	wireSintArr   uint8 = 13 // [ByteLen (Prefix-Varint)][Count (Prefix-Varint)][ZigZag Prefix-Varint]...
	wireFixedArr  uint8 = 14 // [ByteLen (Prefix-Varint)][ElemType (1B: wireFixed32=10 or wireFixed64=11)][Fixed Elements...]
	wireExtended  uint8 = 15 // [ByteLen (Prefix-Varint)][ExtType (1B: 0..255)][Extended Payload...] (Reserved escape hatch)
)

// MaxNestingDepth is the maximum allowed sub-message / array nesting depth (prevents stack-overflow bombs).
const MaxNestingDepth = 32

func encodeZigZag(n int64) uint64 {
	return uint64((n << 1) ^ (n >> 63))
}

func decodeZigZag(u uint64) int64 {
	return int64(u>>1) ^ -int64(u&1)
}

func appendVarint(dst []byte, v uint64) []byte {
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

func decodeVarint(b []byte) (uint64, int) {
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

func backpatchContainerLen(dst []byte, lenPos int) []byte {
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

func fastSkipValue(packet []byte, pos int, wt uint8) (int, bool) {
	n := len(packet)
	switch wt {
	case wireUint, wireSint:
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
	case wireBoolFalse, wireBoolTrue:
		return pos, true
	case wireString, wireBytes, wireSubMsg, wireStringArr, wireUintArr, wireSubMsgArr, wireMap, wireSintArr, wireFixedArr, wireExtended:
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
	case wireFixed32:
		pos += 4
		return pos, pos <= n
	case wireFixed64:
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

func skipValue(b []byte, pos int, wireType uint8) (int, error) {
	if pos < 0 || pos > len(b) {
		return 0, fmt.Errorf("omnibuf: position %d out of bounds", pos)
	}
	if wireType == wireFixedArr || wireType == wireExtended {
		bLen, bn := decodeVarint(b[pos:])
		if bn == 0 || bLen < 1 || bLen > uint64(len(b)-(pos+bn)) {
			return 0, fmt.Errorf("omnibuf: invalid container length at %d", pos)
		}
		pos += bn
		if wireType == wireFixedArr {
			switch elemType := b[pos]; elemType {
			case wireFixed32:
				if (bLen-1)%4 != 0 {
					return 0, fmt.Errorf("omnibuf: wireFixedArr(Fixed32) payload %d is not a multiple of 4 at %d", bLen-1, pos)
				}
			case wireFixed64:
				if (bLen-1)%8 != 0 {
					return 0, fmt.Errorf("omnibuf: wireFixedArr(Fixed64) payload %d is not a multiple of 8 at %d", bLen-1, pos)
				}
			default:
				return 0, fmt.Errorf("omnibuf: invalid wireFixedArr ElemType %d at %d", elemType, pos)
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
			bLen, bn := decodeVarint(packet[pos:])
			if bn == 0 || bLen < minLen || bLen > uint64(n-(pos+bn)) {
				return nil, 0, false, false
			}
			pos += bn
			return packet[pos : pos+int(bLen)], pos + int(bLen), true, true
		}

		var err error
		pos, err = skipValue(packet, pos, wt)
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

func decodeStr(b []byte, zeroCopy bool) string {
	if len(b) == 0 {
		return ""
	}
	if zeroCopy {
		return unsafe.String(&b[0], len(b))
	}
	return string(b)
}

func readLenSlice(data []byte, pos int, minLen uint64) (body []byte, nextPos int, err error) {
	bLen, bn := decodeVarint(data[pos:])
	if bn == 0 {
		return nil, 0, fmt.Errorf("omnibuf: truncated length varint at %d", pos)
	}
	pos += bn
	if bLen < minLen || bLen > uint64(len(data)-pos) {
		return nil, 0, fmt.Errorf("omnibuf: invalid byteLen %d at %d", bLen, pos)
	}
	return data[pos : pos+int(bLen)], pos + int(bLen), nil
}

func countSubMsgArrElems(arrPayload []byte) (int, bool) {
	pos := 0
	n := len(arrPayload)
	count := 0
	for pos < n {
		el, en := decodeVarint(arrPayload[pos:])
		if en == 0 || el > uint64(n-(pos+en)) {
			return 0, false
		}
		pos += en + int(el)
		count++
	}
	return count, pos == n
}
