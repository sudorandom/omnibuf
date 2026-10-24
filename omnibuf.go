package omnibuf

import (
	"encoding/binary"
	"fmt"
	"maps"
	"math"
	"sort"
	"unsafe"
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

// MapHeaderByte encodes the 1-byte WireMap type header: (KeyWireType << 4) | ValWireType.
func MapHeaderByte(keyWT, valWT uint8) byte {
	return ((keyWT & 0x0F) << 4) | (valWT & 0x0F)
}

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

func (e *Encoder) writeTag(fieldNum uint16, wireType uint8) {
	tag := (uint32(fieldNum) << 4) | uint32(wireType)
	if tag <= 127 {
		e.buf = append(e.buf, byte(tag<<1)|1)
		return
	}
	e.appendTagMulti(tag)
}

//go:noinline
func (e *Encoder) appendTagMulti(tag uint32) {
	if tag <= 16383 {
		u := uint16(tag<<2) | 2
		e.buf = append(e.buf, byte(u), byte(u>>8))
		return
	}
	e.buf = AppendVarint(e.buf, uint64(tag))
}

// SchemaMeta is the optional inline schema header encoded as Field #0
// (Tag = 0x0B, WireSubMsg = 5) at byte 0 of an OmniBuf message (omnibuf.v1.SchemaMeta).
type SchemaMeta struct {
	TypeName string                 `protobuf:"bytes,1,opt,name=type_name,proto3" json:"type_name,omitempty"`
	Fields   map[uint32]string      `protobuf:"bytes,2,rep,name=fields,proto3" json:"fields,omitempty"`
	Children map[uint32]*SchemaMeta `protobuf:"bytes,3,rep,name=children,proto3" json:"children,omitempty"`
}

// AppendField0SchemaMeta appends Field #0 (Tag = 0x0B, WireSubMsg = 5) encoding meta to dst.
func AppendField0SchemaMeta(dst []byte, meta *SchemaMeta) []byte {
	if meta == nil {
		return dst
	}
	dst = append(dst, 0x0B) // Tag = (0 << 4) | WireSubMsg(5) = 5 -> (5 << 1) | 1 = 0x0B
	lenPos := len(dst)
	dst = append(dst, 0, 0)
	dst = appendSchemaMetaBody(dst, meta)
	return BackpatchContainerLen(dst, lenPos)
}

func appendSchemaMetaBody(dst []byte, meta *SchemaMeta) []byte {
	if meta == nil {
		return dst
	}
	// Field #1: type_name (WireString = 3 -> Tag (1 << 4) | 3 = 19 -> 0x27)
	if meta.TypeName != "" {
		dst = append(dst, 0x27)
		dst = AppendVarint(dst, uint64(len(meta.TypeName)))
		dst = append(dst, meta.TypeName...)
	}
	// Field #2: fields map<uint32, string> (WireMap = 9 -> Tag (2 << 4) | 9 = 41 -> 0x53)
	if len(meta.Fields) > 0 {
		dst = append(dst, 0x53)
		lenPos := len(dst)
		dst = append(dst, 0, 0, MapHeaderByte(WireUint, WireString))
		dst = AppendVarint(dst, uint64(len(meta.Fields)))
		keys := make([]int, 0, len(meta.Fields))
		for k := range meta.Fields {
			keys = append(keys, int(k))
		}
		sort.Ints(keys)
		for _, k := range keys {
			name := meta.Fields[uint32(k)]
			dst = AppendVarint(dst, uint64(uint32(k)))
			dst = AppendVarint(dst, uint64(len(name)))
			dst = append(dst, name...)
		}
		dst = BackpatchContainerLen(dst, lenPos)
	}
	// Field #3: children map<uint32, SchemaMeta> (WireMap = 9 -> Tag (3 << 4) | 9 = 57 -> 0x73)
	if len(meta.Children) > 0 {
		dst = append(dst, 0x73)
		lenPos := len(dst)
		dst = append(dst, 0, 0, MapHeaderByte(WireUint, WireSubMsg))
		dst = AppendVarint(dst, uint64(len(meta.Children)))
		keys := make([]int, 0, len(meta.Children))
		for k := range meta.Children {
			keys = append(keys, int(k))
		}
		sort.Ints(keys)
		for _, k := range keys {
			child := meta.Children[uint32(k)]
			dst = AppendVarint(dst, uint64(uint32(k)))
			cLenPos := len(dst)
			dst = append(dst, 0, 0)
			dst = appendSchemaMetaBody(dst, child)
			dst = BackpatchContainerLen(dst, cLenPos)
		}
		dst = BackpatchContainerLen(dst, lenPos)
	}
	return dst
}

// WriteField0SchemaMeta writes Field #0 (Tag = 0x0B, WireSubMsg = 5) containing meta.
func (e *Encoder) WriteField0SchemaMeta(meta *SchemaMeta) {
	e.buf = AppendField0SchemaMeta(e.buf, meta)
}

// WriteField0Fields writes Field #0 (Tag = 0x0B, WireSubMsg = 5) mapping FieldNumber -> FieldName.
func (e *Encoder) WriteField0Fields(fields map[uint32]string) {
	e.WriteField0SchemaMeta(&SchemaMeta{Fields: fields})
}

// WriteField0Schema writes Field #0 (Tag = 0x0B, WireSubMsg = 5) mapping field numbers 1..len(names)
// to names[0..len(names)-1].
func (e *Encoder) WriteField0Schema(names []string) {
	fields := make(map[uint32]string, len(names))
	for i, name := range names {
		if name != "" {
			fields[uint32(i+1)] = name
		}
	}
	e.WriteField0SchemaMeta(&SchemaMeta{Fields: fields})
}

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
	e.WriteFixed32(fieldNum, math.Float32bits(v))
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
	e.WriteFixed64(fieldNum, math.Float64bits(v))
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

// WriteStringArr writes a repeated string array with an outer byte-length prefix (WireStringArr).
func (e *Encoder) WriteStringArr(fieldNum uint16, items []string) {
	e.writeTag(fieldNum, WireStringArr)
	lenPos := len(e.buf)
	e.buf = append(e.buf, 0, 0) // Reserve 2B Prefix-Varint byte length
	e.buf = AppendVarint(e.buf, uint64(len(items)))
	for i := range items {
		s := items[i]
		e.buf = AppendVarint(e.buf, uint64(len(s)))
		e.buf = append(e.buf, s...)
	}
	e.buf = BackpatchContainerLen(e.buf, lenPos)
}

// WriteUint32Arr writes a packed uint32 array with a backpatched byte-skip length prefix (WireUintArr).
func (e *Encoder) WriteUint32Arr(fieldNum uint16, items []uint32) {
	e.writeTag(fieldNum, WireUintArr)
	lenPos := len(e.buf)
	e.buf = append(e.buf, 0, 0) // Reserve 2B Prefix-Varint byte length
	e.buf = AppendVarint(e.buf, uint64(len(items)))
	for i := range items {
		e.buf = AppendVarint(e.buf, uint64(items[i]))
	}
	e.buf = BackpatchContainerLen(e.buf, lenPos)
}

// WriteSintArr writes a packed ZigZag-encoded signed integer array (WireSintArr).
func (e *Encoder) WriteSintArr(fieldNum uint16, items []int64) {
	e.writeTag(fieldNum, WireSintArr)
	lenPos := len(e.buf)
	e.buf = append(e.buf, 0, 0) // Reserve 2B Prefix-Varint byte length
	e.buf = AppendVarint(e.buf, uint64(len(items)))
	for i := range items {
		e.buf = AppendVarint(e.buf, EncodeZigZag(items[i]))
	}
	e.buf = BackpatchContainerLen(e.buf, lenPos)
}

// WriteFloat32Arr writes a packed float32 array (WireFixedArr with ElemType = WireFixed32).
func (e *Encoder) WriteFloat32Arr(fieldNum uint16, items []float32) {
	e.writeTag(fieldNum, WireFixedArr)
	e.buf = AppendVarint(e.buf, uint64(1+len(items)*4))
	e.buf = append(e.buf, WireFixed32)
	for i := range items {
		bits := math.Float32bits(items[i])
		e.buf = append(e.buf, byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24))
	}
}

// WriteFloat64Arr writes a packed float64 array (WireFixedArr with ElemType = WireFixed64).
func (e *Encoder) WriteFloat64Arr(fieldNum uint16, items []float64) {
	e.writeTag(fieldNum, WireFixedArr)
	e.buf = AppendVarint(e.buf, uint64(1+len(items)*8))
	e.buf = append(e.buf, WireFixed64)
	for i := range items {
		bits := math.Float64bits(items[i])
		e.buf = append(e.buf,
			byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24),
			byte(bits>>32), byte(bits>>40), byte(bits>>48), byte(bits>>56),
		)
	}
}

// BeginSubMsg starts a nested sub-message (WireSubMsg) in a single pass by reserving a 2-byte
// Prefix-Varint length slot.
func (e *Encoder) BeginSubMsg(fieldNum uint16) {
	e.writeTag(fieldNum, WireSubMsg)
	e.lenStack[e.depth] = len(e.buf)
	e.depth++
	e.buf = append(e.buf, 0, 0) // Reserve 2B Prefix-Varint length
}

// EndSubMsg finishes the current nested sub-message and backpatches its byte length.
func (e *Encoder) EndSubMsg() {
	e.depth--
	lenPos := e.lenStack[e.depth]
	e.buf = BackpatchContainerLen(e.buf, lenPos)
}

// BeginSubMsgArr starts a repeated sub-message array (WireSubMsgArr) in a single pass.
func (e *Encoder) BeginSubMsgArr(fieldNum uint16) {
	e.writeTag(fieldNum, WireSubMsgArr)
	e.lenStack[e.depth] = len(e.buf)
	e.depth++
	e.buf = append(e.buf, 0, 0) // Reserve 2B TotalByteLen Prefix-Varint
}

// BeginSubMsgElem starts the next element inside an open WireSubMsgArr by reserving its 2-byte length prefix.
func (e *Encoder) BeginSubMsgElem() {
	e.lenStack[e.depth] = len(e.buf)
	e.depth++
	e.buf = append(e.buf, 0, 0) // Reserve 2B ElemByteLen Prefix-Varint
}

// EndSubMsgElem finishes the current element inside an open WireSubMsgArr and backpatches its length.
func (e *Encoder) EndSubMsgElem() {
	e.depth--
	lenPos := e.lenStack[e.depth]
	e.buf = BackpatchContainerLen(e.buf, lenPos)
}

// EndSubMsgArr backpatches the total byte length of the open WireSubMsgArr.
func (e *Encoder) EndSubMsgArr() {
	e.depth--
	lenPos := e.lenStack[e.depth]
	e.buf = BackpatchContainerLen(e.buf, lenPos)
}

// WriteMapStringUint writes a map<string, uint64> (WireMap) in a single pass with zero allocations.
func (e *Encoder) WriteMapStringUint(fieldNum uint16, m map[string]uint64) {
	e.writeTag(fieldNum, WireMap)
	lenPos := len(e.buf)
	e.buf = append(e.buf, 0, 0) // Reserve 2B TotalByteLen
	e.buf = append(e.buf, MapHeaderByte(WireString, WireUint))
	e.buf = AppendVarint(e.buf, uint64(len(m)))
	for k, v := range m {
		e.buf = AppendVarint(e.buf, uint64(len(k)))
		e.buf = append(e.buf, k...)
		e.buf = AppendVarint(e.buf, v)
	}
	e.buf = BackpatchContainerLen(e.buf, lenPos)
}

// WriteMapStringUintDeterministic writes a map<string, uint64> (WireMap) with keys sorted in lexicographical order.
func (e *Encoder) WriteMapStringUintDeterministic(fieldNum uint16, m map[string]uint64) {
	e.writeTag(fieldNum, WireMap)
	lenPos := len(e.buf)
	e.buf = append(e.buf, 0, 0) // Reserve 2B TotalByteLen
	e.buf = append(e.buf, MapHeaderByte(WireString, WireUint))
	e.buf = AppendVarint(e.buf, uint64(len(m)))
	if len(m) > 1 {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		for i := 1; i < len(keys); i++ {
			for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
				keys[j], keys[j-1] = keys[j-1], keys[j]
			}
		}
		for _, k := range keys {
			e.buf = AppendVarint(e.buf, uint64(len(k)))
			e.buf = append(e.buf, k...)
			e.buf = AppendVarint(e.buf, m[k])
		}
	} else {
		for k, v := range m {
			e.buf = AppendVarint(e.buf, uint64(len(k)))
			e.buf = append(e.buf, k...)
			e.buf = AppendVarint(e.buf, v)
		}
	}
	e.buf = BackpatchContainerLen(e.buf, lenPos)
}

// Finish returns the encoded message bytes.
func (e *Encoder) Finish() []byte {
	return e.buf
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

// SkipValue advances pos past a single field payload of the given 4-bit wireType (0..15),
// performing full bounds validation in O(1) time without looping inside containers.
func SkipValue(b []byte, pos int, wireType uint8) (int, error) {
	if pos < 0 || pos > len(b) {
		return 0, fmt.Errorf("omnibuf: position %d out of bounds", pos)
	}
	switch wireType {
	case WireUint, WireSint:
		if pos >= len(b) {
			return 0, fmt.Errorf("omnibuf: truncated WireUint at %d", pos)
		}
		b0 := b[pos]
		if (b0 & 1) != 0 {
			return pos + 1, nil
		}
		if (b0 & 2) != 0 {
			if pos+2 > len(b) {
				return 0, fmt.Errorf("omnibuf: truncated 2B WireUint at %d", pos)
			}
			return pos + 2, nil
		}
		if (b0 & 4) != 0 {
			if pos+4 > len(b) {
				return 0, fmt.Errorf("omnibuf: truncated 4B WireUint at %d", pos)
			}
			return pos + 4, nil
		}
		if pos+9 > len(b) {
			return 0, fmt.Errorf("omnibuf: truncated 9B WireUint at %d", pos)
		}
		return pos + 9, nil
	case WireBoolFalse, WireBoolTrue:
		return pos, nil
	case WireString, WireBytes, WireSubMsg, WireStringArr, WireUintArr, WireSubMsgArr, WireMap, WireSintArr, WireFixedArr, WireExtended:
		bLen, bn := DecodeVarint(b[pos:])
		if bn == 0 {
			return 0, fmt.Errorf("omnibuf: truncated length varint for wireType %d at %d", wireType, pos)
		}
		pos += bn
		if bLen > uint64(len(b)-pos) {
			return 0, fmt.Errorf("omnibuf: length %d exceeds remaining buffer %d at %d", bLen, len(b)-pos, pos)
		}
		if wireType == WireFixedArr {
			if bLen < 1 {
				return 0, fmt.Errorf("omnibuf: WireFixedArr byteLen must be >= 1 at %d", pos)
			}
			elemType := b[pos]
			switch elemType {
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
		} else if wireType == WireExtended && bLen < 1 {
			return 0, fmt.Errorf("omnibuf: WireExtended byteLen must be >= 1 at %d", pos)
		}
		return pos + int(bLen), nil
	case WireFixed32:
		if len(b)-pos < 4 {
			return 0, fmt.Errorf("omnibuf: truncated WireFixed32 at %d", pos)
		}
		return pos + 4, nil
	case WireFixed64:
		if len(b)-pos < 8 {
			return 0, fmt.Errorf("omnibuf: truncated WireFixed64 at %d", pos)
		}
		return pos + 8, nil
	default:
		return 0, fmt.Errorf("omnibuf: unknown wire type %d at %d", wireType, pos)
	}
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
			if tn == 0 {
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

		// Branchless O(1) per-field skip over field value
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
			// 0 payload bytes
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
	if !ok || wt != WireFixed32 || len(s) < 4 {
		return 0, false
	}
	return math.Float32frombits(binary.LittleEndian.Uint32(s[:4])), true
}

// ReadFloat64 reads a float64 field with 0 heap allocations.
func ReadFloat64(packet []byte, fieldNum uint16) (float64, bool) {
	s, wt, ok := LocateField(packet, fieldNum)
	if !ok || wt != WireFixed64 || len(s) < 8 {
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
		if 1+strLen > len(s) {
			return nil, false
		}
		return s[1 : 1+strLen], true
	}
	sLen, sn := DecodeVarint(s)
	if sn == 0 || sLen > uint64(len(s)-sn) {
		return nil, false
	}
	return s[sn : sn+int(sLen)], true
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

// ReadStringArr reads a WireStringArr field, concatenating elements if the field appears multiple times.
func ReadStringArr(packet []byte, fieldNum uint16) ([]string, bool) {
	var out []string
	found := false
	pos := 0
	n := len(packet)
	for pos < n {
		tag, tn := DecodeVarint(packet[pos:])
		if tn == 0 {
			return nil, false
		}
		pos += tn
		fnum := uint16(tag >> 4)
		wt := uint8(tag & 0x0F)
		if fnum == fieldNum {
			if wt != WireStringArr {
				return nil, false
			}
			found = true
			byteLen, bn := DecodeVarint(packet[pos:])
			if bn == 0 || byteLen == 0 || byteLen > uint64(n-(pos+bn)) {
				return nil, false
			}
			pos += bn
			body := packet[pos : pos+int(byteLen)]
			pos += int(byteLen)
			count, cn := DecodeVarint(body)
			if cn == 0 || count > uint64(len(body)-cn) {
				return nil, false
			}
			p := cn
			if out == nil && count > 0 {
				out = make([]string, 0, int(count))
			}
			for i := 0; i < int(count); i++ {
				sLen, sn := DecodeVarint(body[p:])
				if sn == 0 || sLen > uint64(len(body)-(p+sn)) {
					return nil, false
				}
				p += sn
				out = append(out, string(body[p:p+int(sLen)]))
				p += int(sLen)
			}
			if p != len(body) {
				return nil, false
			}
			continue
		}
		nextPos, err := SkipValue(packet, pos, wt)
		if err != nil {
			return nil, false
		}
		pos = nextPos
	}
	return out, found
}

// ReadUint32Arr reads a WireUintArr field, concatenating elements if the field appears multiple times.
func ReadUint32Arr(packet []byte, fieldNum uint16) ([]uint32, bool) {
	var out []uint32
	found := false
	pos := 0
	n := len(packet)
	for pos < n {
		tag, tn := DecodeVarint(packet[pos:])
		if tn == 0 {
			return nil, false
		}
		pos += tn
		fnum := uint16(tag >> 4)
		wt := uint8(tag & 0x0F)
		if fnum == fieldNum {
			if wt != WireUintArr {
				return nil, false
			}
			found = true
			byteLen, bn := DecodeVarint(packet[pos:])
			if bn == 0 || byteLen == 0 || byteLen > uint64(n-(pos+bn)) {
				return nil, false
			}
			pos += bn
			body := packet[pos : pos+int(byteLen)]
			pos += int(byteLen)
			count, cn := DecodeVarint(body)
			if cn == 0 || count > uint64(len(body)-cn) {
				return nil, false
			}
			p := cn
			if out == nil && count > 0 {
				out = make([]uint32, 0, int(count))
			}
			for i := 0; i < int(count); i++ {
				v, vn := DecodeVarint(body[p:])
				if vn == 0 {
					return nil, false
				}
				p += vn
				out = append(out, uint32(v))
			}
			if p != len(body) {
				return nil, false
			}
			continue
		}
		nextPos, err := SkipValue(packet, pos, wt)
		if err != nil {
			return nil, false
		}
		pos = nextPos
	}
	return out, found
}

// ReadSintArr reads a WireSintArr field (packed ZigZag signed integers), concatenating across segments.
func ReadSintArr(packet []byte, fieldNum uint16) ([]int64, bool) {
	var out []int64
	found := false
	pos := 0
	n := len(packet)
	for pos < n {
		tag, tn := DecodeVarint(packet[pos:])
		if tn == 0 {
			return nil, false
		}
		pos += tn
		fnum := uint16(tag >> 4)
		wt := uint8(tag & 0x0F)
		if fnum == fieldNum {
			if wt != WireSintArr {
				return nil, false
			}
			found = true
			byteLen, bn := DecodeVarint(packet[pos:])
			if bn == 0 || byteLen == 0 || byteLen > uint64(n-(pos+bn)) {
				return nil, false
			}
			pos += bn
			body := packet[pos : pos+int(byteLen)]
			pos += int(byteLen)
			count, cn := DecodeVarint(body)
			if cn == 0 || count > uint64(len(body)-cn) {
				return nil, false
			}
			p := cn
			if out == nil && count > 0 {
				out = make([]int64, 0, int(count))
			}
			for i := 0; i < int(count); i++ {
				v, vn := DecodeVarint(body[p:])
				if vn == 0 {
					return nil, false
				}
				p += vn
				out = append(out, DecodeZigZag(v))
			}
			if p != len(body) {
				return nil, false
			}
			continue
		}
		nextPos, err := SkipValue(packet, pos, wt)
		if err != nil {
			return nil, false
		}
		pos = nextPos
	}
	return out, found
}

// ReadFloat32Arr reads a WireFixedArr field (ElemType = WireFixed32, packed 4B LE float32/fixed32 values), concatenating across segments.
func ReadFloat32Arr(packet []byte, fieldNum uint16) ([]float32, bool) {
	var out []float32
	found := false
	pos := 0
	n := len(packet)
	for pos < n {
		tag, tn := DecodeVarint(packet[pos:])
		if tn == 0 {
			return nil, false
		}
		pos += tn
		fnum := uint16(tag >> 4)
		wt := uint8(tag & 0x0F)
		if fnum == fieldNum {
			if wt != WireFixedArr {
				return nil, false
			}
			found = true
			byteLen, bn := DecodeVarint(packet[pos:])
			if bn == 0 || byteLen < 1 || byteLen > uint64(n-(pos+bn)) || (byteLen-1)%4 != 0 {
				return nil, false
			}
			pos += bn
			if packet[pos] != WireFixed32 {
				return nil, false
			}
			body := packet[pos+1 : pos+int(byteLen)]
			pos += int(byteLen)
			count := len(body) / 4
			if out == nil && count > 0 {
				out = make([]float32, 0, count)
			}
			for p := 0; p < len(body); p += 4 {
				out = append(out, math.Float32frombits(binary.LittleEndian.Uint32(body[p:p+4])))
			}
			continue
		}
		nextPos, err := SkipValue(packet, pos, wt)
		if err != nil {
			return nil, false
		}
		pos = nextPos
	}
	return out, found
}

// ReadFloat64Arr reads a WireFixedArr field (ElemType = WireFixed64, packed 8B LE float64/fixed64 values), concatenating across segments.
func ReadFloat64Arr(packet []byte, fieldNum uint16) ([]float64, bool) {
	var out []float64
	found := false
	pos := 0
	n := len(packet)
	for pos < n {
		tag, tn := DecodeVarint(packet[pos:])
		if tn == 0 {
			return nil, false
		}
		pos += tn
		fnum := uint16(tag >> 4)
		wt := uint8(tag & 0x0F)
		if fnum == fieldNum {
			if wt != WireFixedArr {
				return nil, false
			}
			found = true
			byteLen, bn := DecodeVarint(packet[pos:])
			if bn == 0 || byteLen < 1 || byteLen > uint64(n-(pos+bn)) || (byteLen-1)%8 != 0 {
				return nil, false
			}
			pos += bn
			if packet[pos] != WireFixed64 {
				return nil, false
			}
			body := packet[pos+1 : pos+int(byteLen)]
			pos += int(byteLen)
			count := len(body) / 8
			if out == nil && count > 0 {
				out = make([]float64, 0, count)
			}
			for p := 0; p < len(body); p += 8 {
				out = append(out, math.Float64frombits(binary.LittleEndian.Uint64(body[p:p+8])))
			}
			continue
		}
		nextPos, err := SkipValue(packet, pos, wt)
		if err != nil {
			return nil, false
		}
		pos = nextPos
	}
	return out, found
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

// ReadSubMsg returns the inner packet slice of a nested WireSubMsg field.
func ReadSubMsg(packet []byte, fieldNum uint16) ([]byte, bool) {
	s, wt, ok := LocateField(packet, fieldNum)
	if !ok || wt != WireSubMsg {
		return nil, false
	}
	subLen, sn := DecodeVarint(s)
	if sn == 0 || subLen > uint64(len(s)-sn) {
		return nil, false
	}
	return s[sn : sn+int(subLen)], true
}

// ReadElem slices the i-th sub-message element out of a WireSubMsgArr payload by hopping element length prefixes.
func ReadElem(arrPayload []byte, index int) ([]byte, bool) {
	if index < 0 {
		return nil, false
	}
	pos := 0
	n := len(arrPayload)
	for i := 0; pos < n; i++ {
		var elemLen int
		var en int
		b0 := arrPayload[pos]
		if (b0 & 2) != 0 {
			if pos+2 > n {
				return nil, false
			}
			elemLen = int(binary.LittleEndian.Uint16(arrPayload[pos:pos+2]) >> 2)
			en = 2
		} else {
			l64, ln := DecodeVarint(arrPayload[pos:])
			if ln == 0 || l64 > uint64(n-(pos+ln)) {
				return nil, false
			}
			elemLen = int(l64)
			en = ln
		}
		pos += en
		if elemLen > n-pos {
			return nil, false
		}
		if i == index {
			return arrPayload[pos : pos+elemLen], true
		}
		pos += elemLen
	}
	return nil, false
}

// SubMsgArrLen returns the total number of elements in a WireSubMsgArr field across any concatenated segments.
func SubMsgArrLen(packet []byte, fieldNum uint16) int {
	total := 0
	pos := 0
	n := len(packet)
	for pos < n {
		tag, tn := DecodeVarint(packet[pos:])
		if tn == 0 {
			return 0
		}
		pos += tn
		fnum := uint16(tag >> 4)
		wt := uint8(tag & 0x0F)
		if fnum == fieldNum {
			if wt != WireSubMsgArr {
				return 0
			}
			arrLen, an := DecodeVarint(packet[pos:])
			if an == 0 || arrLen > uint64(n-(pos+an)) {
				return 0
			}
			pos += an
			c, ok := CountSubMsgArrElems(packet[pos : pos+int(arrLen)])
			if !ok {
				return 0
			}
			total += c
			pos += int(arrLen)
			continue
		}
		nextPos, err := SkipValue(packet, pos, wt)
		if err != nil {
			return 0
		}
		pos = nextPos
	}
	return total
}

// CountSubMsgArrElems counts and validates the length-prefixed elements inside a WireSubMsgArr payload.
func CountSubMsgArrElems(arrPayload []byte) (int, bool) {
	pos := 0
	n := len(arrPayload)
	count := 0
	for pos < n {
		el, en := DecodeVarint(arrPayload[pos:])
		if en == 0 || el > uint64(n-(pos+en)) {
			return 0, false
		}
		pos += en + int(el)
		count++
	}
	return count, pos == n
}

// ReadSubMsgArrElem locates a WireSubMsgArr field in packet and slices its i-th element
// (supporting concatenated WireSubMsgArr fields across merged messages).
func ReadSubMsgArrElem(packet []byte, fieldNum uint16, index int) ([]byte, bool) {
	if index < 0 {
		return nil, false
	}
	pos := 0
	n := len(packet)
	remIdx := index
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
				return nil, false
			}
			u := binary.LittleEndian.Uint16(packet[pos:pos+2]) >> 2
			fnum = u >> 4
			wt = uint8(u & 0x0F)
			pos += 2
		} else {
			tag, tn := decodeVarintMulti(packet[pos:])
			if tn == 0 {
				return nil, false
			}
			fnum = uint16(tag >> 4)
			wt = uint8(tag & 0x0F)
			pos += tn
		}

		if fnum == fieldNum {
			if wt != WireSubMsgArr {
				return nil, false
			}
			arrLen, an := DecodeVarint(packet[pos:])
			if an == 0 || arrLen > uint64(n-(pos+an)) {
				return nil, false
			}
			pos += an
			arrPayload := packet[pos : pos+int(arrLen)]
			pos += int(arrLen)
			if elem, ok := ReadElem(arrPayload, remIdx); ok {
				return elem, true
			}
			c, ok := CountSubMsgArrElems(arrPayload)
			if !ok {
				return nil, false
			}
			remIdx -= c
			continue
		}

		nextPos, err := SkipValue(packet, pos, wt)
		if err != nil {
			return nil, false
		}
		pos = nextPos
	}
	return nil, false
}

// ReadNestedStringBytes locates targetField inside subMsgField, supporting merged sub-messages across concatenated packets.
func ReadNestedStringBytes(packet []byte, subMsgField, targetField uint16) ([]byte, bool) {
	var lastBytes []byte
	found := false
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
				return nil, false
			}
			u := binary.LittleEndian.Uint16(packet[pos:pos+2]) >> 2
			fnum = u >> 4
			wt = uint8(u & 0x0F)
			pos += 2
		} else {
			tag, tn := decodeVarintMulti(packet[pos:])
			if tn == 0 {
				return nil, false
			}
			fnum = uint16(tag >> 4)
			wt = uint8(tag & 0x0F)
			pos += tn
		}

		if fnum == subMsgField {
			if wt != WireSubMsg {
				return nil, false
			}
			subLen, sn := DecodeVarint(packet[pos:])
			if sn == 0 || subLen > uint64(n-(pos+sn)) {
				return nil, false
			}
			pos += sn
			sub := packet[pos : pos+int(subLen)]
			pos += int(subLen)
			if b, ok := ReadStringBytes(sub, targetField); ok {
				lastBytes = b
				found = true
			}
			continue
		}

		nextPos, err := SkipValue(packet, pos, wt)
		if err != nil {
			return nil, false
		}
		pos = nextPos
	}
	return lastBytes, found
}

// ReadNestedString reads a nested string (e.g. peer.location.city), allocating a safe string copy.
func ReadNestedString(packet []byte, subMsgField, targetField uint16) (string, bool) {
	b, ok := ReadNestedStringBytes(packet, subMsgField, targetField)
	if !ok {
		return "", false
	}
	return string(b), true
}

// ReadNestedStringZeroCopy reads a nested string (e.g. peer.location.city) with 0 allocations.
func ReadNestedStringZeroCopy(packet []byte, subMsgField, targetField uint16) (string, bool) {
	b, ok := ReadNestedStringBytes(packet, subMsgField, targetField)
	if !ok || len(b) == 0 {
		return "", ok
	}
	return unsafe.String(&b[0], len(b)), true
}

// ReadArrNestedString reads a string inside the i-th repeated sub-message (e.g. peer.neighbors[2].router).
func ReadArrNestedString(packet []byte, arrField uint16, index int, targetField uint16) (string, bool) {
	elem, ok := ReadSubMsgArrElem(packet, arrField, index)
	if !ok {
		return "", false
	}
	b, ok := ReadStringBytes(elem, targetField)
	if !ok {
		return "", false
	}
	return string(b), true
}

// ReadArrNestedStringZeroCopy reads a string inside the i-th repeated sub-message with 0 allocations.
func ReadArrNestedStringZeroCopy(packet []byte, arrField uint16, index int, targetField uint16) (string, bool) {
	elem, ok := ReadSubMsgArrElem(packet, arrField, index)
	if !ok {
		return "", false
	}
	return ReadStringZeroCopy(elem, targetField)
}

// LookupMapStringUint searches a WireMap field without allocating a Go map,
// supporting merged WireMap fields and "last write wins" across entries and concatenated packets.
func LookupMapStringUint(packet []byte, fieldNum uint16, targetKey string) (uint64, bool) {
	var lastVal uint64
	found := false
	pos := 0
	n := len(packet)
	for pos < n {
		tag, tn := DecodeVarint(packet[pos:])
		if tn == 0 {
			return 0, false
		}
		pos += tn
		fnum := uint16(tag >> 4)
		wt := uint8(tag & 0x0F)
		if fnum == fieldNum {
			if wt != WireMap {
				return 0, false
			}
			mapLen, mn := DecodeVarint(packet[pos:])
			if mn == 0 || mapLen < 2 || mapLen > uint64(n-(pos+mn)) {
				return 0, false
			}
			pos += mn
			body := packet[pos : pos+int(mapLen)]
			pos += int(mapLen)
			if body[0] != MapHeaderByte(WireString, WireUint) {
				return 0, false
			}
			count, cn := DecodeVarint(body[1:])
			if cn == 0 || count > uint64(len(body)-(1+cn)) {
				return 0, false
			}
			p := 1 + cn
			for range count {
				kLen, kn := DecodeVarint(body[p:])
				if kn == 0 || kLen > uint64(len(body)-(p+kn)) {
					return 0, false
				}
				p += kn
				var k string
				if kLen > 0 {
					k = unsafe.String(&body[p], int(kLen))
				}
				p += int(kLen)
				v, vn := DecodeVarint(body[p:])
				if vn == 0 {
					return 0, false
				}
				p += vn
				if k == targetKey {
					lastVal = v
					found = true
				}
			}
			if p != len(body) {
				return 0, false
			}
			continue
		}
		nextPos, err := SkipValue(packet, pos, wt)
		if err != nil {
			return 0, false
		}
		pos = nextPos
	}
	return lastVal, found
}

// ReadField0SchemaMeta decodes and merges all Field #0 (Tag = 0x0B, WireSubMsg = 5)
// SchemaMeta blocks in packet.
func ReadField0SchemaMeta(packet []byte) (*SchemaMeta, bool) {
	var merged *SchemaMeta
	pos := 0
	n := len(packet)
	for pos < n {
		tag, tn := DecodeVarint(packet[pos:])
		if tn == 0 {
			return nil, false
		}
		pos += tn
		fnum := tag >> 4
		wt := uint8(tag & 0x0F)
		if fnum == 0 {
			if wt != WireSubMsg {
				return nil, false
			}
			bLen, bn := DecodeVarint(packet[pos:])
			if bn == 0 || bLen > uint64(n-(pos+bn)) {
				return nil, false
			}
			pos += bn
			m, err := decodeSchemaMeta(packet[pos:pos+int(bLen)], 0)
			if err != nil {
				return nil, false
			}
			merged = mergeSchemaMeta(merged, m)
			pos += int(bLen)
			continue
		}
		nextPos, err := SkipValue(packet, pos, wt)
		if err != nil {
			return nil, false
		}
		pos = nextPos
	}
	return merged, merged != nil
}

func decodeSchemaMeta(data []byte, depth int) (*SchemaMeta, error) {
	if depth > MaxNestingDepth {
		return nil, fmt.Errorf("omnibuf: SchemaMeta nesting depth %d exceeds MaxNestingDepth (%d)", depth, MaxNestingDepth)
	}
	meta := &SchemaMeta{}
	pos := 0
	n := len(data)
	for pos < n {
		tag, tn := DecodeVarint(data[pos:])
		if tn == 0 {
			return nil, fmt.Errorf("omnibuf: truncated tag in Field #0 SchemaMeta at %d", pos)
		}
		pos += tn
		fnum := tag >> 4
		wt := uint8(tag & 0x0F)
		switch fnum {
		case 1: // type_name (string)
			if wt != WireString {
				return nil, fmt.Errorf("omnibuf: SchemaMeta field #1 (type_name) must be WireString, got %d", wt)
			}
			sLen, sn := DecodeVarint(data[pos:])
			if sn == 0 || sLen > uint64(n-(pos+sn)) {
				return nil, fmt.Errorf("omnibuf: truncated SchemaMeta type_name at %d", pos)
			}
			pos += sn
			meta.TypeName = string(data[pos : pos+int(sLen)])
			pos += int(sLen)
		case 2: // fields (map<uint32, string>)
			if wt != WireMap {
				return nil, fmt.Errorf("omnibuf: SchemaMeta field #2 (fields) must be WireMap, got %d", wt)
			}
			bLen, bn := DecodeVarint(data[pos:])
			if bn == 0 || bLen < 2 || bLen > uint64(n-(pos+bn)) {
				return nil, fmt.Errorf("omnibuf: invalid SchemaMeta fields map byteLen at %d", pos)
			}
			pos += bn
			body := data[pos : pos+int(bLen)]
			pos += int(bLen)
			if body[0] != MapHeaderByte(WireUint, WireString) {
				return nil, fmt.Errorf("omnibuf: SchemaMeta fields map must have MapType 0x03, got 0x%02x", body[0])
			}
			count, cn := DecodeVarint(body[1:])
			if cn == 0 || count*2 > uint64(len(body)-(1+cn)) {
				return nil, fmt.Errorf("omnibuf: SchemaMeta fields count %d exceeds body", count)
			}
			if meta.Fields == nil {
				meta.Fields = make(map[uint32]string, int(count))
			}
			p := 1 + cn
			for i := 0; i < int(count); i++ {
				k64, kn := DecodeVarint(body[p:])
				if kn == 0 || k64 == 0 || k64 > math.MaxUint32 {
					return nil, fmt.Errorf("omnibuf: invalid SchemaMeta field number %d at %d", k64, p)
				}
				p += kn
				sLen, sn := DecodeVarint(body[p:])
				if sn == 0 || sLen > uint64(len(body)-(p+sn)) {
					return nil, fmt.Errorf("omnibuf: truncated SchemaMeta field name at %d", p)
				}
				p += sn
				meta.Fields[uint32(k64)] = string(body[p : p+int(sLen)])
				p += int(sLen)
			}
			if p != len(body) {
				return nil, fmt.Errorf("omnibuf: SchemaMeta fields map trailing bytes (parsed %d, body %d)", p, len(body))
			}
		case 3: // children (map<uint32, SchemaMeta>)
			if wt != WireMap {
				return nil, fmt.Errorf("omnibuf: SchemaMeta field #3 (children) must be WireMap, got %d", wt)
			}
			bLen, bn := DecodeVarint(data[pos:])
			if bn == 0 || bLen < 2 || bLen > uint64(n-(pos+bn)) {
				return nil, fmt.Errorf("omnibuf: invalid SchemaMeta children map byteLen at %d", pos)
			}
			pos += bn
			body := data[pos : pos+int(bLen)]
			pos += int(bLen)
			if body[0] != MapHeaderByte(WireUint, WireSubMsg) {
				return nil, fmt.Errorf("omnibuf: SchemaMeta children map must have MapType 0x05, got 0x%02x", body[0])
			}
			count, cn := DecodeVarint(body[1:])
			if cn == 0 || count*2 > uint64(len(body)-(1+cn)) {
				return nil, fmt.Errorf("omnibuf: SchemaMeta children count %d exceeds body", count)
			}
			if meta.Children == nil {
				meta.Children = make(map[uint32]*SchemaMeta, int(count))
			}
			p := 1 + cn
			for i := 0; i < int(count); i++ {
				k64, kn := DecodeVarint(body[p:])
				if kn == 0 || k64 == 0 || k64 > math.MaxUint32 {
					return nil, fmt.Errorf("omnibuf: invalid SchemaMeta child field number %d at %d", k64, p)
				}
				p += kn
				cLen, cn2 := DecodeVarint(body[p:])
				if cn2 == 0 || cLen > uint64(len(body)-(p+cn2)) {
					return nil, fmt.Errorf("omnibuf: truncated SchemaMeta child submsg at %d", p)
				}
				p += cn2
				childMeta, err := decodeSchemaMeta(body[p:p+int(cLen)], depth+1)
				if err != nil {
					return nil, err
				}
				meta.Children[uint32(k64)] = mergeSchemaMeta(meta.Children[uint32(k64)], childMeta)
				p += int(cLen)
			}
			if p != len(body) {
				return nil, fmt.Errorf("omnibuf: SchemaMeta children map trailing bytes (parsed %d, body %d)", p, len(body))
			}
		default:
			if fnum == 0 {
				return nil, fmt.Errorf("omnibuf: invalid field #0 inside SchemaMeta")
			}
			nextPos, err := SkipValue(data, pos, wt)
			if err != nil {
				return nil, err
			}
			pos = nextPos
		}
	}
	return meta, nil
}

func mergeSchemaMeta(base, next *SchemaMeta) *SchemaMeta {
	if next == nil {
		return base
	}
	if base == nil {
		cp := &SchemaMeta{TypeName: next.TypeName}
		if len(next.Fields) > 0 {
			cp.Fields = make(map[uint32]string, len(next.Fields))
			maps.Copy(cp.Fields, next.Fields)
		}
		if len(next.Children) > 0 {
			cp.Children = make(map[uint32]*SchemaMeta, len(next.Children))
			for k, c := range next.Children {
				cp.Children[k] = mergeSchemaMeta(nil, c)
			}
		}
		return cp
	}
	if next.TypeName != "" {
		base.TypeName = next.TypeName
	}
	if len(next.Fields) > 0 {
		if base.Fields == nil {
			base.Fields = make(map[uint32]string, len(next.Fields))
		}
		maps.Copy(base.Fields, next.Fields)
	}
	if len(next.Children) > 0 {
		if base.Children == nil {
			base.Children = make(map[uint32]*SchemaMeta, len(next.Children))
		}
		for k, c := range next.Children {
			base.Children[k] = mergeSchemaMeta(base.Children[k], c)
		}
	}
	return base
}

// DecodeBlind decodes any OmniBuf packet without a compile-time schema.
// If Field #0 (Tag = 0x0B, WireSubMsg = 5, omnibuf.v1.SchemaMeta) is present, its field names
// (and any nested children schemas) are used to decode human-readable keys.
// Otherwise, synthetic field names ("field_1", "field_2", ...) are used.
func DecodeBlind(packet []byte) (map[string]any, error) {
	return decodeBlindWithSchema(packet, nil, 0)
}

func decodeBlindWithSchema(data []byte, inherited *SchemaMeta, depth int) (map[string]any, error) {
	if depth > MaxNestingDepth {
		return nil, fmt.Errorf("omnibuf: nesting depth %d exceeds MaxNestingDepth (%d)", depth, MaxNestingDepth)
	}
	n := len(data)
	if n == 0 {
		return map[string]any{}, nil
	}

	var activeSchema *SchemaMeta
	if inherited != nil {
		activeSchema = mergeSchemaMeta(nil, inherited)
	}
	pos := 0
	out := make(map[string]any, 8)

	for pos < n {
		tag, tn := DecodeVarint(data[pos:])
		if tn == 0 {
			return nil, fmt.Errorf("omnibuf: truncated tag at %d", pos)
		}
		pos += tn
		fieldNum := int(tag >> 4)
		wt := uint8(tag & 0x0F)

		if fieldNum == 0 {
			if wt != WireSubMsg {
				return nil, fmt.Errorf("omnibuf: Field #0 must have WireSubMsg wire type (5), got %d", wt)
			}
			byteLen, bn := DecodeVarint(data[pos:])
			if bn == 0 {
				return nil, fmt.Errorf("omnibuf: truncated Field #0 byteLen")
			}
			pos += bn
			if byteLen > uint64(n-pos) {
				return nil, fmt.Errorf("omnibuf: Field #0 byteLen %d invalid", byteLen)
			}
			f0Body := data[pos : pos+int(byteLen)]
			pos += int(byteLen)
			parsedMeta, err := decodeSchemaMeta(f0Body, depth+1)
			if err != nil {
				return nil, err
			}
			activeSchema = mergeSchemaMeta(activeSchema, parsedMeta)
			continue
		}

		var name string
		if activeSchema != nil && activeSchema.Fields != nil {
			name = activeSchema.Fields[uint32(fieldNum)]
		}
		if name == "" {
			name = fmt.Sprintf("field_%d", fieldNum)
		}

		var childSchema *SchemaMeta
		if activeSchema != nil && activeSchema.Children != nil {
			childSchema = activeSchema.Children[uint32(fieldNum)]
		}

		switch wt {
		case WireUint:
			v, vn := DecodeVarint(data[pos:])
			if vn == 0 {
				return nil, fmt.Errorf("omnibuf: truncated WireUint at %d", pos)
			}
			pos += vn
			out[name] = v
		case WireSint:
			v, vn := DecodeVarint(data[pos:])
			if vn == 0 {
				return nil, fmt.Errorf("omnibuf: truncated WireSint at %d", pos)
			}
			pos += vn
			out[name] = DecodeZigZag(v)
		case WireBoolFalse:
			out[name] = false
		case WireBoolTrue:
			out[name] = true
		case WireString:
			sLen, sn := DecodeVarint(data[pos:])
			if sn == 0 {
				return nil, fmt.Errorf("omnibuf: truncated WireString length at %d", pos)
			}
			pos += sn
			if sLen > uint64(n-pos) {
				return nil, fmt.Errorf("omnibuf: WireString length %d exceeds buffer at %d", sLen, pos)
			}
			out[name] = string(data[pos : pos+int(sLen)])
			pos += int(sLen)
		case WireBytes:
			bLen, bn := DecodeVarint(data[pos:])
			if bn == 0 {
				return nil, fmt.Errorf("omnibuf: truncated WireBytes length at %d", pos)
			}
			pos += bn
			if bLen > uint64(n-pos) {
				return nil, fmt.Errorf("omnibuf: WireBytes length %d exceeds buffer at %d", bLen, pos)
			}
			cp := make([]byte, int(bLen))
			copy(cp, data[pos:pos+int(bLen)])
			out[name] = cp
			pos += int(bLen)
		case WireSubMsg:
			subLen, sn := DecodeVarint(data[pos:])
			if sn == 0 {
				return nil, fmt.Errorf("omnibuf: truncated WireSubMsg length at %d", pos)
			}
			pos += sn
			if subLen > uint64(n-pos) {
				return nil, fmt.Errorf("omnibuf: WireSubMsg length %d exceeds buffer at %d", subLen, pos)
			}
			subMap, err := decodeBlindWithSchema(data[pos:pos+int(subLen)], childSchema, depth+1)
			if err != nil {
				return nil, err
			}
			if prevMap, ok := out[name].(map[string]any); ok {
				maps.Copy(prevMap, subMap)
			} else {
				out[name] = subMap
			}
			pos += int(subLen)
		case WireStringArr:
			byteLen, bn := DecodeVarint(data[pos:])
			if bn == 0 {
				return nil, fmt.Errorf("omnibuf: truncated WireStringArr byteLen at %d", pos)
			}
			pos += bn
			if byteLen == 0 || byteLen > uint64(n-pos) {
				return nil, fmt.Errorf("omnibuf: WireStringArr byteLen %d invalid at %d", byteLen, pos)
			}
			end := pos + int(byteLen)
			count, cn := DecodeVarint(data[pos:end])
			if cn == 0 || count > uint64(end-(pos+cn)) {
				return nil, fmt.Errorf("omnibuf: invalid WireStringArr count at %d", pos)
			}
			p := pos + cn
			arr, _ := out[name].([]string)
			if arr == nil && count > 0 {
				arr = make([]string, 0, int(count))
			} else if arr == nil {
				arr = []string{}
			}
			for i := 0; i < int(count); i++ {
				sLen, sn := DecodeVarint(data[p:end])
				if sn == 0 {
					return nil, fmt.Errorf("omnibuf: truncated WireStringArr elem length at %d", p)
				}
				p += sn
				if sLen > uint64(end-p) {
					return nil, fmt.Errorf("omnibuf: WireStringArr elem length %d exceeds container at %d", sLen, p)
				}
				arr = append(arr, string(data[p:p+int(sLen)]))
				p += int(sLen)
			}
			if p != end {
				return nil, fmt.Errorf("omnibuf: WireStringArr byteLen mismatch (parsed %d, expected end %d)", p, end)
			}
			out[name] = arr
			pos = end
		case WireUintArr:
			byteLen, bn := DecodeVarint(data[pos:])
			if bn == 0 {
				return nil, fmt.Errorf("omnibuf: truncated WireUintArr byteLen at %d", pos)
			}
			pos += bn
			if byteLen == 0 || byteLen > uint64(n-pos) {
				return nil, fmt.Errorf("omnibuf: WireUintArr byteLen %d invalid at %d", byteLen, pos)
			}
			end := pos + int(byteLen)
			count, cn := DecodeVarint(data[pos:end])
			if cn == 0 || count > uint64(end-(pos+cn)) {
				return nil, fmt.Errorf("omnibuf: invalid WireUintArr count at %d", pos)
			}
			p := pos + cn
			arr, _ := out[name].([]uint64)
			if arr == nil && count > 0 {
				arr = make([]uint64, 0, int(count))
			} else if arr == nil {
				arr = []uint64{}
			}
			for i := 0; i < int(count); i++ {
				v, vn := DecodeVarint(data[p:end])
				if vn == 0 {
					return nil, fmt.Errorf("omnibuf: truncated WireUintArr elem at %d", p)
				}
				p += vn
				arr = append(arr, v)
			}
			if p != end {
				return nil, fmt.Errorf("omnibuf: WireUintArr byteLen mismatch (parsed %d, expected end %d)", p, end)
			}
			out[name] = arr
			pos = end
		case WireSintArr:
			byteLen, bn := DecodeVarint(data[pos:])
			if bn == 0 {
				return nil, fmt.Errorf("omnibuf: truncated WireSintArr byteLen at %d", pos)
			}
			pos += bn
			if byteLen == 0 || byteLen > uint64(n-pos) {
				return nil, fmt.Errorf("omnibuf: WireSintArr byteLen %d invalid at %d", byteLen, pos)
			}
			end := pos + int(byteLen)
			count, cn := DecodeVarint(data[pos:end])
			if cn == 0 || count > uint64(end-(pos+cn)) {
				return nil, fmt.Errorf("omnibuf: invalid WireSintArr count at %d", pos)
			}
			p := pos + cn
			arr, _ := out[name].([]int64)
			if arr == nil && count > 0 {
				arr = make([]int64, 0, int(count))
			} else if arr == nil {
				arr = []int64{}
			}
			for i := 0; i < int(count); i++ {
				v, vn := DecodeVarint(data[p:end])
				if vn == 0 {
					return nil, fmt.Errorf("omnibuf: truncated WireSintArr elem at %d", p)
				}
				p += vn
				arr = append(arr, DecodeZigZag(v))
			}
			if p != end {
				return nil, fmt.Errorf("omnibuf: WireSintArr byteLen mismatch (parsed %d, expected end %d)", p, end)
			}
			out[name] = arr
			pos = end
		case WireFixedArr:
			byteLen, bn := DecodeVarint(data[pos:])
			if bn == 0 {
				return nil, fmt.Errorf("omnibuf: truncated WireFixedArr byteLen at %d", pos)
			}
			pos += bn
			if byteLen < 1 || byteLen > uint64(n-pos) {
				return nil, fmt.Errorf("omnibuf: WireFixedArr byteLen %d invalid at %d", byteLen, pos)
			}
			end := pos + int(byteLen)
			elemType := data[pos]
			pStart := pos + 1
			switch elemType {
			case WireFixed32:
				if (byteLen-1)%4 != 0 {
					return nil, fmt.Errorf("omnibuf: WireFixedArr(Fixed32) payload %d is not a multiple of 4 at %d", byteLen-1, pos)
				}
				count := int(byteLen-1) / 4
				arr, _ := out[name].([]float32)
				if arr == nil && count > 0 {
					arr = make([]float32, 0, count)
				} else if arr == nil {
					arr = []float32{}
				}
				for p := pStart; p < end; p += 4 {
					arr = append(arr, math.Float32frombits(binary.LittleEndian.Uint32(data[p:p+4])))
				}
				out[name] = arr
			case WireFixed64:
				if (byteLen-1)%8 != 0 {
					return nil, fmt.Errorf("omnibuf: WireFixedArr(Fixed64) payload %d is not a multiple of 8 at %d", byteLen-1, pos)
				}
				count := int(byteLen-1) / 8
				arr, _ := out[name].([]float64)
				if arr == nil && count > 0 {
					arr = make([]float64, 0, count)
				} else if arr == nil {
					arr = []float64{}
				}
				for p := pStart; p < end; p += 8 {
					arr = append(arr, math.Float64frombits(binary.LittleEndian.Uint64(data[p:p+8])))
				}
				out[name] = arr
			default:
				return nil, fmt.Errorf("omnibuf: invalid WireFixedArr ElemType %d at %d", elemType, pos)
			}
			pos = end
		case WireExtended:
			byteLen, bn := DecodeVarint(data[pos:])
			if bn == 0 {
				return nil, fmt.Errorf("omnibuf: truncated WireExtended byteLen at %d", pos)
			}
			pos += bn
			if byteLen < 1 || byteLen > uint64(n-pos) {
				return nil, fmt.Errorf("omnibuf: WireExtended byteLen %d invalid at %d", byteLen, pos)
			}
			extType := data[pos]
			return nil, fmt.Errorf("omnibuf: unrecognized WireExtended subtype 0x%02x at %d", extType, pos)
		case WireSubMsgArr:
			byteLen, bn := DecodeVarint(data[pos:])
			if bn == 0 {
				return nil, fmt.Errorf("omnibuf: truncated WireSubMsgArr byteLen at %d", pos)
			}
			pos += bn
			if byteLen > uint64(n-pos) {
				return nil, fmt.Errorf("omnibuf: invalid WireSubMsgArr byteLen %d at %d", byteLen, pos)
			}
			arrPayload := data[pos : pos+int(byteLen)]
			elemCount, ok := CountSubMsgArrElems(arrPayload)
			if !ok {
				return nil, fmt.Errorf("omnibuf: corrupt WireSubMsgArr element framing at %d", pos)
			}
			elems, _ := out[name].([]map[string]any)
			if elems == nil && elemCount > 0 {
				elems = make([]map[string]any, 0, elemCount)
			} else if elems == nil {
				elems = []map[string]any{}
			}
			p := 0
			for range elemCount {
				el, en := DecodeVarint(arrPayload[p:])
				p += en
				elemMap, err := decodeBlindWithSchema(arrPayload[p:p+int(el)], childSchema, depth+1)
				if err != nil {
					return nil, err
				}
				elems = append(elems, elemMap)
				p += int(el)
			}
			out[name] = elems
			pos += int(byteLen)
		case WireMap:
			byteLen, bn := DecodeVarint(data[pos:])
			if bn == 0 {
				return nil, fmt.Errorf("omnibuf: truncated WireMap byteLen at %d", pos)
			}
			pos += bn
			if byteLen < 2 || byteLen > uint64(n-pos) {
				return nil, fmt.Errorf("omnibuf: invalid WireMap byteLen %d at %d", byteLen, pos)
			}
			body := data[pos : pos+int(byteLen)]
			mapType := body[0]
			keyWT := mapType >> 4
			valWT := mapType & 0x0F
			count, cn := DecodeVarint(body[1:])
			if cn == 0 || count > uint64(len(body)-(1+cn)) {
				return nil, fmt.Errorf("omnibuf: WireMap count %d exceeds body (%d B)", count, len(body))
			}
			p := 1 + cn
			if keyWT == WireString && valWT == WireUint {
				m, ok := out[name].(map[string]uint64)
				if !ok || m == nil {
					m = make(map[string]uint64, int(count))
				}
				for i := 0; i < int(count); i++ {
					kLen, kn := DecodeVarint(body[p:])
					if kn == 0 || kLen > uint64(len(body)-(p+kn)) {
						return nil, fmt.Errorf("omnibuf: truncated WireMap key at %d", p)
					}
					p += kn
					k := string(body[p : p+int(kLen)])
					p += int(kLen)
					v, vn := DecodeVarint(body[p:])
					if vn == 0 {
						return nil, fmt.Errorf("omnibuf: truncated WireMap value at %d", p)
					}
					p += vn
					m[k] = v
				}
				if p != len(body) {
					return nil, fmt.Errorf("omnibuf: WireMap trailing bytes (parsed %d, body %d)", p, len(body))
				}
				out[name] = m
			} else {
				m, ok := out[name].(map[string]any)
				if !ok || m == nil {
					m = make(map[string]any, int(count))
				}
				for i := 0; i < int(count); i++ {
					k, nextP, err := decodeMapKeyBlind(body, p, keyWT)
					if err != nil {
						return nil, err
					}
					p = nextP
					v, nextP2, err := decodeMapValBlind(body, p, valWT, childSchema, depth)
					if err != nil {
						return nil, err
					}
					p = nextP2
					m[k] = v
				}
				if p != len(body) {
					return nil, fmt.Errorf("omnibuf: WireMap trailing bytes (parsed %d, body %d)", p, len(body))
				}
				out[name] = m
			}
			pos += int(byteLen)
		case WireFixed32:
			if n-pos < 4 {
				return nil, fmt.Errorf("omnibuf: truncated WireFixed32 at %d", pos)
			}
			bits32 := binary.LittleEndian.Uint32(data[pos : pos+4])
			out[name] = math.Float32frombits(bits32)
			pos += 4
		case WireFixed64:
			if n-pos < 8 {
				return nil, fmt.Errorf("omnibuf: truncated WireFixed64 at %d", pos)
			}
			bits64 := binary.LittleEndian.Uint64(data[pos : pos+8])
			out[name] = math.Float64frombits(bits64)
			pos += 8
		default:
			return nil, fmt.Errorf("omnibuf: unsupported wire type %d", wt)
		}
	}

	return out, nil
}

func decodeMapKeyBlind(body []byte, p int, keyWT uint8) (string, int, error) {
	switch keyWT {
	case WireString:
		kLen, kn := DecodeVarint(body[p:])
		if kn == 0 || kLen > uint64(len(body)-(p+kn)) {
			return "", 0, fmt.Errorf("omnibuf: truncated WireMap string key at %d", p)
		}
		p += kn
		k := string(body[p : p+int(kLen)])
		return k, p + int(kLen), nil
	case WireUint:
		v, vn := DecodeVarint(body[p:])
		if vn == 0 {
			return "", 0, fmt.Errorf("omnibuf: truncated WireMap uint key at %d", p)
		}
		return fmt.Sprintf("%d", v), p + vn, nil
	case WireSint:
		v, vn := DecodeVarint(body[p:])
		if vn == 0 {
			return "", 0, fmt.Errorf("omnibuf: truncated WireMap sint key at %d", p)
		}
		return fmt.Sprintf("%d", DecodeZigZag(v)), p + vn, nil
	case WireFixed32:
		if len(body)-p < 4 {
			return "", 0, fmt.Errorf("omnibuf: truncated WireMap fixed32 key at %d", p)
		}
		v := binary.LittleEndian.Uint32(body[p : p+4])
		return fmt.Sprintf("%d", v), p + 4, nil
	case WireFixed64:
		if len(body)-p < 8 {
			return "", 0, fmt.Errorf("omnibuf: truncated WireMap fixed64 key at %d", p)
		}
		v := binary.LittleEndian.Uint64(body[p : p+8])
		return fmt.Sprintf("%d", v), p + 8, nil
	default:
		return "", 0, fmt.Errorf("omnibuf: invalid WireMap key wire type %d", keyWT)
	}
}

func decodeMapValBlind(body []byte, p int, valWT uint8, childSchema *SchemaMeta, depth int) (any, int, error) {
	switch valWT {
	case WireUint:
		v, vn := DecodeVarint(body[p:])
		if vn == 0 {
			return nil, 0, fmt.Errorf("omnibuf: truncated WireMap uint value at %d", p)
		}
		return v, p + vn, nil
	case WireSint:
		v, vn := DecodeVarint(body[p:])
		if vn == 0 {
			return nil, 0, fmt.Errorf("omnibuf: truncated WireMap sint value at %d", p)
		}
		return DecodeZigZag(v), p + vn, nil
	case WireString:
		vLen, vn := DecodeVarint(body[p:])
		if vn == 0 || vLen > uint64(len(body)-(p+vn)) {
			return nil, 0, fmt.Errorf("omnibuf: truncated WireMap string value at %d", p)
		}
		p += vn
		return string(body[p : p+int(vLen)]), p + int(vLen), nil
	case WireBytes:
		vLen, vn := DecodeVarint(body[p:])
		if vn == 0 || vLen > uint64(len(body)-(p+vn)) {
			return nil, 0, fmt.Errorf("omnibuf: truncated WireMap bytes value at %d", p)
		}
		p += vn
		cp := make([]byte, int(vLen))
		copy(cp, body[p:p+int(vLen)])
		return cp, p + int(vLen), nil
	case WireSubMsg:
		vLen, vn := DecodeVarint(body[p:])
		if vn == 0 || vLen > uint64(len(body)-(p+vn)) {
			return nil, 0, fmt.Errorf("omnibuf: truncated WireMap submsg value at %d", p)
		}
		p += vn
		subMap, err := decodeBlindWithSchema(body[p:p+int(vLen)], childSchema, depth+1)
		if err != nil {
			return nil, 0, err
		}
		return subMap, p + int(vLen), nil
	case WireFixed32:
		if len(body)-p < 4 {
			return nil, 0, fmt.Errorf("omnibuf: truncated WireMap fixed32 value at %d", p)
		}
		return math.Float32frombits(binary.LittleEndian.Uint32(body[p : p+4])), p + 4, nil
	case WireFixed64:
		if len(body)-p < 8 {
			return nil, 0, fmt.Errorf("omnibuf: truncated WireMap fixed64 value at %d", p)
		}
		return math.Float64frombits(binary.LittleEndian.Uint64(body[p : p+8])), p + 8, nil
	case WireBoolTrue, WireBoolFalse:
		if len(body)-p < 1 {
			return nil, 0, fmt.Errorf("omnibuf: truncated WireMap bool value at %d", p)
		}
		b := body[p]
		if b != 0 && b != 1 {
			return nil, 0, fmt.Errorf("omnibuf: invalid WireMap bool value %d at %d", b, p)
		}
		return b == 1, p + 1, nil
	default:
		return nil, 0, fmt.Errorf("omnibuf: invalid WireMap value wire type %d", valWT)
	}
}
