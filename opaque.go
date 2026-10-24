package omnibuf

import "encoding/binary"

// ============================================================================
// Opaque API Runtime Support: Zero-Decode Wire Splicing
// ============================================================================

// SpliceFields copies unmodified fields from raw into dst in O(1) time per field
// while calling encodeDirty(dst, fieldNum) for any field whose bit (1 << fieldNum) is set in dirtyMask.
func SpliceFields(dst, raw []byte, dirtyMask uint16, maxField uint16, encodeDirty func(dst []byte, fieldNum uint16) []byte) []byte {
	n := len(raw)
	pos := 0
	cleanStart := 0
	var writtenMask uint16

	for pos < n {
		fieldStart := pos
		b0 := raw[pos]
		var fnum uint16
		var wt uint8
		if (b0 & 1) != 0 {
			fnum = uint16(b0 >> 5)
			wt = (b0 >> 1) & 0x0F
			pos++
		} else if (b0 & 2) != 0 {
			if pos+2 > n {
				break
			}
			u := binary.LittleEndian.Uint16(raw[pos:pos+2]) >> 2
			fnum = u >> 4
			wt = uint8(u & 0x0F)
			pos += 2
		} else {
			var valid bool
			fnum, wt, pos, valid = readTagMultiAt(raw, pos)
			if !valid {
				break
			}
		}

		switch wt {
		case WireUint, WireSint:
			if pos >= n {
				pos = n
			} else {
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
			}
		case WireBoolFalse, WireBoolTrue:
		case WireString, WireBytes, WireSubMsg, WireStringArr, WireUintArr, WireSubMsgArr, WireMap, WireSintArr, WireFixedArr, WireExtended:
			if pos >= n {
				pos = n
			} else {
				lb := raw[pos]
				if (lb & 1) != 0 {
					pos += 1 + int(lb>>1)
				} else if (lb & 2) != 0 {
					if pos+2 <= n {
						pos += 2 + int(binary.LittleEndian.Uint16(raw[pos:pos+2])>>2)
					} else {
						pos = n
					}
				} else {
					nextPos, ok := skipLargeContainer(raw, pos)
					if ok {
						pos = nextPos
					} else {
						pos = n
					}
				}
			}
		case WireFixed32:
			pos += 4
		case WireFixed64:
			pos += 8
		}
		if pos > n {
			pos = n
		}

		if fnum < 16 && (dirtyMask&(1<<fnum)) != 0 {
			if fieldStart > cleanStart {
				dst = append(dst, raw[cleanStart:fieldStart]...)
			}
			if (writtenMask & (1 << fnum)) == 0 {
				dst = encodeDirty(dst, fnum)
				writtenMask |= (1 << fnum)
			}
			cleanStart = pos
		}
	}

	if n > cleanStart {
		dst = append(dst, raw[cleanStart:n]...)
	}

	remaining := dirtyMask &^ writtenMask
	if remaining != 0 {
		for fnum := uint16(1); fnum <= maxField; fnum++ {
			if (remaining & (1 << fnum)) != 0 {
				dst = encodeDirty(dst, fnum)
			}
		}
	}
	return dst
}
