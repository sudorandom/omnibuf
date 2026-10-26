package wire

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
		nextPos, ok := fastSkipValue(raw, pos, wt)
		if !ok {
			break
		}
		pos = nextPos
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
