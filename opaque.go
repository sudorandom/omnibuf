package omnibuf

import "encoding/binary"

// ============================================================================
// Opaque API Runtime Support: Zero-Decode Wire Splicing
// ============================================================================

// SpliceFields splices unmodified contiguous byte ranges from raw and invokes appendDirty
// only for fields whose bit is set in dirtyMask, avoiding any decoding of untouched fields.
func SpliceFields(dst, raw []byte, dirtyMask uint16, maxField uint16, appendDirty func(dst []byte, fnum uint16) []byte) []byte {
	n := len(raw)
	pos := 0
	cleanStart := 0
	var emitted uint16

	for pos < n {
		fieldStart := pos
		var fnum uint16
		var wt uint8
		b0 := raw[pos]
		if (b0 & 1) != 0 {
			fnum = uint16(b0 >> 5)
			wt = (b0 >> 1) & 0x0F
			pos++
		} else if (b0&2) != 0 && pos+2 <= n {
			u := binary.LittleEndian.Uint16(raw[pos:pos+2]) >> 2
			fnum = u >> 4
			wt = uint8(u & 0x0F)
			pos += 2
		} else {
			break
		}

		// Fast-skip field value to find fieldEnd
		switch wt {
		case WireUint, WireSint:
			if pos >= n {
				break
			}
			vb := raw[pos]
			if (vb & 1) != 0 {
				pos++
			} else if (vb & 2) != 0 {
				pos += 2
			} else if (vb & 4) != 0 {
				pos += 4
			} else {
				pos += 9
			}
		case WireBoolFalse, WireBoolTrue:
		case WireString, WireBytes, WireSubMsg, WireStringArr, WireUintArr, WireSubMsgArr, WireMap, WireSintArr, WireFixedArr, WireExtended:
			if pos >= n {
				break
			}
			lb := raw[pos]
			if (lb & 1) != 0 {
				pos += 1 + int(lb>>1)
			} else if (lb&2) != 0 && pos+2 <= n {
				pos += 2 + int(binary.LittleEndian.Uint16(raw[pos:pos+2])>>2)
			} else {
				bLen, bn := DecodeVarint(raw[pos:])
				pos += bn + int(bLen)
			}
		case WireFixed32:
			pos += 4
		case WireFixed64:
			pos += 8
		}
		if pos > n {
			break
		}
		fieldEnd := pos

		bit := uint16(1) << fnum
		if (dirtyMask & bit) != 0 {
			if cleanStart < fieldStart {
				dst = append(dst, raw[cleanStart:fieldStart]...)
			}
			dst = appendDirty(dst, fnum)
			emitted |= bit
			cleanStart = fieldEnd
		} else {
			emitted |= bit
		}
	}

	if cleanStart < n {
		dst = append(dst, raw[cleanStart:n]...)
	}

	// Append any newly added dirty fields that were not present in raw
	rem := dirtyMask &^ emitted
	if rem != 0 {
		for fnum := uint16(1); fnum <= maxField; fnum++ {
			if (rem & (1 << fnum)) != 0 {
				dst = appendDirty(dst, fnum)
			}
		}
	}
	return dst
}
