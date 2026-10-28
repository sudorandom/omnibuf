package wire

import (
	"encoding/binary"
	"math"
	"unsafe"
)

// ============================================================================
// Opaque API Runtime Support: Zero-Decode Wire Splicing
// ============================================================================

// SpliceFields splices unmodified contiguous byte ranges from raw and invokes appendDirty
// only for fields whose bit is set in dirtyMask, avoiding any decoding of untouched fields.
func SpliceFields(dst, raw []byte, dirtyMask uint16, maxField uint16, appendDirty func(dst []byte, fnum uint16) []byte) []byte {
	n := len(raw)
	if n > 0 && cap(dst) > 0 {
		dstStart := uintptr(unsafe.Pointer(unsafe.SliceData(dst)))
		dstEnd := dstStart + uintptr(cap(dst))
		rawStart := uintptr(unsafe.Pointer(unsafe.SliceData(raw)))
		rawEnd := rawStart + uintptr(n)
		if dstStart < rawEnd && rawStart < dstEnd {
			dst = nil
		}
	}
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
			tag, tn := decodeVarintMulti(raw[pos:])
			if tn == 0 || tag>>4 > math.MaxUint16 {
				break
			}
			fnum = uint16(tag >> 4)
			wt = uint8(tag & 0x0F)
			pos += tn
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
			if (emitted & bit) == 0 {
				dst = appendDirty(dst, fnum)
				emitted |= bit
			}
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
