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
	"sort"
	"unsafe"

	"github.com/sudorandom/omnibuf/wire"
)

// Explicit 4-bit Wire Types (0..15) encoded in the low 4 bits of the field tag:
// Tag = (FieldNumber << 4) | WireType
const (
	wireUint      uint8 = 0
	wireSint      uint8 = 1
	wireBoolFalse uint8 = 2
	wireBoolTrue  uint8 = 3
	wireFixed32   uint8 = 4
	wireFixed64   uint8 = 5
	wireSfixed32  uint8 = 6
	wireSfixed64  uint8 = 7
	wireFloat32   uint8 = 8
	wireFloat64   uint8 = 9
	wireString    uint8 = 10
	wireBytes     uint8 = 11
	wireSubMsg    uint8 = 12
	wireArray     uint8 = 13
	wireMap       uint8 = 14
	wireExtended  uint8 = 15
)

// MaxNestingDepth is the maximum allowed sub-message / array nesting depth (prevents stack-overflow bombs).
const MaxNestingDepth = 32

const maxProtobufFieldNumber = 536870911

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

// Keep this hot path inlineable; just go-inline-check guards the compiler decision.
func decodeVarint(b []byte) (v uint64, n int) {
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
	return decodeVarint(b)
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
		} else if vb == 0 {
			pos += 9
		} else {
			return 0, false
		}
		return pos, pos <= n
	case wireBoolFalse, wireBoolTrue:
		return pos, true
	case wireString, wireBytes, wireSubMsg, wireArray, wireMap, wireExtended:
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
	case wireFixed32, wireFloat32, wireSfixed32:
		pos += 4
		return pos, pos <= n
	case wireFixed64, wireFloat64, wireSfixed64:
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
	if wireType == wireArray || wireType == wireMap || wireType == wireExtended {
		length, ln := decodeVarint(b[pos:])
		if ln == 0 || length < 1 || length > uint64(len(b)-pos-ln) {
			return 0, fmt.Errorf("omnibuf: invalid container length")
		}
		end := pos + ln + int(length)
		switch wireType {
		case wireArray:
			if _, _, _, ok := parseArrayHeader(b[pos+ln : end]); !ok {
				return 0, fmt.Errorf("omnibuf: invalid array header")
			}
		case wireMap:
			if _, _, _, _, ok := parseMapHeader(b[pos+ln : end]); !ok {
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
			if tn == 0 || tag>>4 > maxProtobufFieldNumber {
				return nil, 0, false, false
			}
			fnum = uint32(tag >> 4)
			wt = uint8(tag & 0x0F)
			pos += tn
		}

		if fnum == 0 && wt != wireSubMsg {
			return nil, 0, false, false
		}
		if fnum == uint32(targetField) {
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

// readStrSlice is readLenSlice for WireString payloads: it additionally
// rejects invalid UTF-8 as required by the spec (binary data uses WireBytes).
func readStrSlice(data []byte, pos int) (body []byte, nextPos int, err error) {
	body, nextPos, err = readLenSlice(data, pos, 0)
	if err != nil {
		return nil, 0, err
	}
	if !wire.ValidUTF8(body) {
		return nil, 0, fmt.Errorf("omnibuf: invalid UTF-8 in WireString at %d", pos)
	}
	return body, nextPos, nil
}

func countSubMsgArrElems(body []byte) (int, bool) {
	e, count, p, ok := parseArrayHeader(body)
	if !ok || e != wireSubMsg {
		return 0, false
	}
	for i := 0; i < count; i++ {
		l, n := decodeVarint(body[p:])
		if n == 0 || l > uint64(len(body)-p-n) {
			return 0, false
		}
		p += n + int(l)
	}
	return count, p == len(body)
}
