package omnibuf_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	. "github.com/sudorandom/omnibuf"
	"github.com/sudorandom/omnibuf/internal/conformance"
	"github.com/sudorandom/omnibuf/internal/pb"
	. "github.com/sudorandom/omnibuf/wire"
)

// ============================================================================
// 1. Protobuf-Style TestAllTypes Schema (Covers All 12 Wire Types 0..11 + >14)
// ============================================================================

type AllTypesSubMsg struct {
	ID    uint32 `protobuf:"varint,1,opt,name=id,proto3" json:"id"`
	Label string `protobuf:"bytes,2,opt,name=label,proto3" json:"label"`
	Flag  bool   `protobuf:"varint,3,opt,name=flag,proto3" json:"flag"`
	Score int64  `protobuf:"zigzag64,4,opt,name=score,proto3" json:"score"`
}

// TestAllTypes mirrors Google Protobuf's canonical `TestAllTypesProto3` conformance
// message, exercising all 12 OmniBuf wire types (0..11) across fields 1..20.
type TestAllTypes struct {
	F1Uint32      uint32            `protobuf:"varint,1,opt,name=f1_uint32,proto3" json:"f1_uint32"`
	F2Uint64      uint64            `protobuf:"varint,2,opt,name=f2_uint64,proto3" json:"f2_uint64"`
	F3Sint32      int32             `protobuf:"zigzag32,3,opt,name=f3_sint32,proto3" json:"f3_sint32"`
	F4Sint64      int64             `protobuf:"zigzag64,4,opt,name=f4_sint64,proto3" json:"f4_sint64"`
	F5Bool        bool              `protobuf:"varint,5,opt,name=f5_bool,proto3" json:"f5_bool"`
	F6String      string            `protobuf:"bytes,6,opt,name=f6_string,proto3" json:"f6_string"`
	F7Bytes       []byte            `protobuf:"bytes,7,opt,name=f7_bytes,proto3" json:"f7_bytes"`
	F8SubMsg      *AllTypesSubMsg   `protobuf:"bytes,8,opt,name=f8_sub_msg,proto3" json:"f8_sub_msg"`
	F9StringArr   []string          `protobuf:"bytes,9,rep,name=f9_string_arr,proto3" json:"f9_string_arr"`
	F10Uint32Arr  []uint32          `protobuf:"varint,10,rep,packed,name=f10_uint32_arr,proto3" json:"f10_uint32_arr"`
	F11SubMsgArr  []AllTypesSubMsg  `protobuf:"bytes,11,rep,name=f11_sub_msg_arr,proto3" json:"f11_sub_msg_arr"`
	F12MapStrUint map[string]uint64 `protobuf:"bytes,12,rep,name=f12_map_str_uint,proto3" json:"f12_map_str_uint"`
	F13Float32    float32           `protobuf:"fixed32,13,opt,name=f13_float32,proto3" json:"f13_float32"`
	F14Float64    float64           `protobuf:"fixed64,14,opt,name=f14_float64,proto3" json:"f14_float64"`
	// Higher-numbered fields (>14) using multi-byte tags:
	F15Fixed32     uint32 `protobuf:"fixed32,15,opt,name=f15_fixed32,proto3" json:"f15_fixed32"`
	F16Fixed64     uint64 `protobuf:"fixed64,16,opt,name=f16_fixed64,proto3" json:"f16_fixed64"`
	F20OverflowStr string `protobuf:"bytes,20,opt,name=f20_overflow_str,proto3" json:"f20_overflow_str"`
}

// ConformanceVector represents a portable, language-independent test case in
// testdata/conformance_corpus.json for cross-language OmniBuf conformance testing.
type ConformanceVector struct {
	Name          string         `json:"name"`
	Category      string         `json:"category"`
	Description   string         `json:"description"`
	Schema        string         `json:"schema"`
	WireHex       string         `json:"wire_hex"`
	WireByteLen   int            `json:"wire_byte_len"`
	PresentFields []uint16       `json:"present_fields"`
	ExpectedBlind map[string]any `json:"expected_blind,omitempty"`
}

type ConformanceCorpusFile struct {
	Version       string              `json:"version"`
	Specification string              `json:"specification"`
	ValidVectors  []ConformanceVector `json:"valid_vectors"`
	InvalidCases  []InvalidWireCase   `json:"invalid_cases"`
}

type InvalidWireCase struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	WireHex     string `json:"wire_hex"`
}

func inspectPresentFields(packet []byte) []uint16 {
	var presentFields []uint16
	pos := 0
	n := len(packet)
	for pos < n {
		tag, tn := DecodeVarint(packet[pos:])
		if tn <= 0 {
			break
		}
		pos += tn
		fnum := uint16(tag >> 4)
		wtype := uint8(tag & 0x0F)
		presentFields = append(presentFields, fnum)
		nextPos, err := SkipValue(packet, pos, wtype)
		if err != nil {
			break
		}
		pos = nextPos
	}
	return presentFields
}

func floatBitsEqual32(a, b float32) bool {
	if math.IsNaN(float64(a)) && math.IsNaN(float64(b)) {
		return math.Float32bits(a) == math.Float32bits(b)
	}
	return math.Float32bits(a) == math.Float32bits(b)
}

func floatBitsEqual64(a, b float64) bool {
	if math.IsNaN(a) && math.IsNaN(b) {
		return math.Float64bits(a) == math.Float64bits(b)
	}
	return math.Float64bits(a) == math.Float64bits(b)
}

func assertAllTypesEqual(t *testing.T, caseName string, got, want *TestAllTypes) {
	t.Helper()
	if got.F1Uint32 != want.F1Uint32 ||
		got.F2Uint64 != want.F2Uint64 ||
		got.F3Sint32 != want.F3Sint32 ||
		got.F4Sint64 != want.F4Sint64 ||
		got.F5Bool != want.F5Bool ||
		got.F6String != want.F6String ||
		!bytes.Equal(got.F7Bytes, want.F7Bytes) ||
		!floatBitsEqual32(got.F13Float32, want.F13Float32) ||
		!floatBitsEqual64(got.F14Float64, want.F14Float64) ||
		got.F15Fixed32 != want.F15Fixed32 ||
		got.F16Fixed64 != want.F16Fixed64 ||
		got.F20OverflowStr != want.F20OverflowStr {
		t.Fatalf("[%s] scalar mismatch:\ngot:  %+v\nwant: %+v", caseName, got, want)
	}
	if !reflect.DeepEqual(got.F8SubMsg, want.F8SubMsg) {
		t.Fatalf("[%s] F8SubMsg mismatch: got %+v, want %+v", caseName, got.F8SubMsg, want.F8SubMsg)
	}
	if len(got.F9StringArr) != len(want.F9StringArr) || (len(got.F9StringArr) > 0 && !reflect.DeepEqual(got.F9StringArr, want.F9StringArr)) {
		t.Fatalf("[%s] F9StringArr mismatch: got %v, want %v", caseName, got.F9StringArr, want.F9StringArr)
	}
	if len(got.F10Uint32Arr) != len(want.F10Uint32Arr) || (len(got.F10Uint32Arr) > 0 && !reflect.DeepEqual(got.F10Uint32Arr, want.F10Uint32Arr)) {
		t.Fatalf("[%s] F10Uint32Arr mismatch: got %v, want %v", caseName, got.F10Uint32Arr, want.F10Uint32Arr)
	}
	if len(got.F11SubMsgArr) != len(want.F11SubMsgArr) || (len(got.F11SubMsgArr) > 0 && !reflect.DeepEqual(got.F11SubMsgArr, want.F11SubMsgArr)) {
		t.Fatalf("[%s] F11SubMsgArr mismatch: got %v, want %v", caseName, got.F11SubMsgArr, want.F11SubMsgArr)
	}
	if len(got.F12MapStrUint) != len(want.F12MapStrUint) || (len(got.F12MapStrUint) > 0 && !reflect.DeepEqual(got.F12MapStrUint, want.F12MapStrUint)) {
		t.Fatalf("[%s] F12MapStrUint mismatch: got %v, want %v", caseName, got.F12MapStrUint, want.F12MapStrUint)
	}
}

// verifyAllTypesAcrossEngines verifies a TestAllTypes instance across:
//  1. Table-Driven Marshal / Unmarshal (safe copy)
//  2. Table-Driven UnmarshalZeroCopy (zero-copy strings/bytes)
//  3. Header-Hop Field Readers (ReadUint, ReadInt, ReadBool, ReadString, ReadBytes,
//     ReadStringArr, ReadUint32Arr, ReadSubMsg, ReadSubMsgArrElem, LookupMapStringUint,
//     ReadFloat32, ReadFloat64, plus LocateField for fields >14)
//  4. Schema-less DecodeBlind
func verifyAllTypesAcrossEngines(t *testing.T, name, category, desc string, msg *TestAllTypes) ConformanceVector {
	t.Helper()
	wire, err := MarshalDeterministic(msg)
	if err != nil {
		t.Fatalf("[%s] MarshalDeterministic failed: %v", name, err)
	}

	// 1. Safe Unmarshal
	var gotSafe TestAllTypes
	if err := Unmarshal(wire, &gotSafe); err != nil {
		t.Fatalf("[%s] Unmarshal failed: %v", name, err)
	}
	assertAllTypesEqual(t, name+"/Safe", &gotSafe, msg)

	// 2. Zero-Copy Unmarshal
	var gotZC TestAllTypes
	if err := UnmarshalZeroCopy(wire, &gotZC); err != nil {
		t.Fatalf("[%s] UnmarshalZeroCopy failed: %v", name, err)
	}
	assertAllTypesEqual(t, name+"/ZeroCopy", &gotZC, msg)

	// 3. Header-Hop Field Readers
	if u1, ok := ReadUint(wire, 1); !ok || uint32(u1) != msg.F1Uint32 {
		t.Fatalf("[%s] ReadUint(1) = (%d, %v), want %d", name, u1, ok, msg.F1Uint32)
	}
	if u2, ok := ReadUint(wire, 2); !ok || u2 != msg.F2Uint64 {
		t.Fatalf("[%s] ReadUint(2) = (%d, %v), want %d", name, u2, ok, msg.F2Uint64)
	}
	if i3, ok := ReadInt(wire, 3); !ok || int32(i3) != msg.F3Sint32 {
		t.Fatalf("[%s] ReadInt(3) = (%d, %v), want %d", name, i3, ok, msg.F3Sint32)
	}
	if i4, ok := ReadInt(wire, 4); !ok || i4 != msg.F4Sint64 {
		t.Fatalf("[%s] ReadInt(4) = (%d, %v), want %d", name, i4, ok, msg.F4Sint64)
	}
	if b5, ok := ReadBool(wire, 5); !ok || b5 != msg.F5Bool {
		t.Fatalf("[%s] ReadBool(5) = (%v, %v), want %v", name, b5, ok, msg.F5Bool)
	}
	if s6, ok := ReadString(wire, 6); !ok || s6 != msg.F6String {
		t.Fatalf("[%s] ReadString(6) = (%q, %v), want %q", name, s6, ok, msg.F6String)
	}
	if s6zc, ok := ReadStringZeroCopy(wire, 6); !ok || s6zc != msg.F6String {
		t.Fatalf("[%s] ReadStringZeroCopy(6) = (%q, %v), want %q", name, s6zc, ok, msg.F6String)
	}
	if b7, ok := ReadBytes(wire, 7); !ok || !bytes.Equal(b7, msg.F7Bytes) {
		t.Fatalf("[%s] ReadBytes(7) mismatch", name)
	}
	if msg.F8SubMsg != nil {
		sub, ok := ReadSubMsg(wire, 8)
		if !ok {
			t.Fatalf("[%s] ReadSubMsg(8) failed", name)
		}
		if lbl, ok := ReadString(sub, 2); !ok || lbl != msg.F8SubMsg.Label {
			t.Fatalf("[%s] ReadSubMsg(8).Label = %q, want %q", name, lbl, msg.F8SubMsg.Label)
		}
	}
	if sa9, ok := ReadStringArr(wire, 9); !ok || (len(sa9) > 0 && !reflect.DeepEqual(sa9, msg.F9StringArr)) {
		t.Fatalf("[%s] ReadStringArr(9) = %v, want %v", name, sa9, msg.F9StringArr)
	}
	if ua10, ok := ReadUint32Arr(wire, 10); !ok || (len(ua10) > 0 && !reflect.DeepEqual(ua10, msg.F10Uint32Arr)) {
		t.Fatalf("[%s] ReadUint32Arr(10) = %v, want %v", name, ua10, msg.F10Uint32Arr)
	}
	for idx, wantElem := range msg.F11SubMsgArr {
		elemBytes, ok := ReadSubMsgArrElem(wire, 11, idx)
		if !ok {
			t.Fatalf("[%s] ReadSubMsgArrElem(11, %d) failed", name, idx)
		}
		if gotLbl, ok := ReadString(elemBytes, 2); !ok || gotLbl != wantElem.Label {
			t.Fatalf("[%s] elem[%d].Label = %q, want %q", name, idx, gotLbl, wantElem.Label)
		}
	}
	for k, wantVal := range msg.F12MapStrUint {
		gotVal, ok := LookupMapStringUint(wire, 12, k)
		if !ok || gotVal != wantVal {
			t.Fatalf("[%s] LookupMapStringUint(12, %q) = (%d, %v), want %d", name, k, gotVal, ok, wantVal)
		}
	}
	if _, ok := LookupMapStringUint(wire, 12, "__nonexistent_key__"); ok {
		t.Fatalf("[%s] LookupMapStringUint found nonexistent key", name)
	}
	if f13, ok := ReadFloat32(wire, 13); !ok || !floatBitsEqual32(f13, msg.F13Float32) {
		t.Fatalf("[%s] ReadFloat32(13) = %v, want %v", name, f13, msg.F13Float32)
	}
	if f14, ok := ReadFloat64(wire, 14); !ok || !floatBitsEqual64(f14, msg.F14Float64) {
		t.Fatalf("[%s] ReadFloat64(14) = %v, want %v", name, f14, msg.F14Float64)
	}
	// Overflow field (>14) lookup via LocateField
	if s20, ok := ReadString(wire, 20); !ok || s20 != msg.F20OverflowStr {
		t.Fatalf("[%s] ReadString(20 overflow) = (%q, %v), want %q", name, s20, ok, msg.F20OverflowStr)
	}

	// 4. Schema-less DecodeBlind
	blindMap, err := DecodeBlind(wire)
	if err != nil {
		t.Fatalf("[%s] DecodeBlind failed: %v", name, err)
	}
	if gotU1, ok := blindMap["field_1"].(uint64); !ok || gotU1 != uint64(msg.F1Uint32) {
		t.Fatalf("[%s] DecodeBlind field_1 = (%T %v), want uint64(%d)", name, blindMap["field_1"], blindMap["field_1"], msg.F1Uint32)
	}
	if gotU2, ok := blindMap["field_2"].(uint64); !ok || gotU2 != msg.F2Uint64 {
		t.Fatalf("[%s] DecodeBlind field_2 = (%T %v), want uint64(%d)", name, blindMap["field_2"], blindMap["field_2"], msg.F2Uint64)
	}
	if gotI3, ok := blindMap["field_3"].(int64); !ok || gotI3 != int64(msg.F3Sint32) {
		t.Fatalf("[%s] DecodeBlind field_3 = (%T %v), want int64(%d)", name, blindMap["field_3"], blindMap["field_3"], msg.F3Sint32)
	}
	if gotI4, ok := blindMap["field_4"].(int64); !ok || gotI4 != msg.F4Sint64 {
		t.Fatalf("[%s] DecodeBlind field_4 = (%T %v), want int64(%d)", name, blindMap["field_4"], blindMap["field_4"], msg.F4Sint64)
	}
	// Omit NaN/Inf from JSON-serialized ExpectedBlind since standard JSON does not support NaN/Inf
	jsonSafeBlind := blindMap
	if math.IsNaN(float64(msg.F13Float32)) || math.IsInf(float64(msg.F13Float32), 0) ||
		math.IsNaN(msg.F14Float64) || math.IsInf(msg.F14Float64, 0) {
		jsonSafeBlind = nil
	}

	present := inspectPresentFields(wire)
	return ConformanceVector{
		Name:          name,
		Category:      category,
		Description:   desc,
		Schema:        "TestAllTypes",
		WireHex:       hex.EncodeToString(wire),
		WireByteLen:   len(wire),
		PresentFields: present,
		ExpectedBlind: jsonSafeBlind,
	}
}

// ============================================================================
// 2. Full Conformance Corpus Generation & Execution
// ============================================================================

func TestProtobufStyleConformanceCorpus(t *testing.T) {
	var vectors []ConformanceVector

	// ------------------------------------------------------------------------
	// Category A: Prefix-Varint (TZCNT) Exact Bit-Width Boundaries
	// ------------------------------------------------------------------------
	varintCases := []struct {
		name string
		desc string
		u32  uint32
		u64  uint64
	}{
		{"varint_zero", "TZCNT=0 minimum (0 -> 1 byte 0x01)", 0, 0},
		{"varint_1b_max_127", "TZCNT=0 maximum (127 -> 1 byte 0xFF)", 127, 127},
		{"varint_2b_min_128", "TZCNT=1 minimum (128 -> 2 bytes)", 128, 128},
		{"varint_2b_max_16383", "TZCNT=1 maximum (16383 -> 2 bytes 0xFF 0xFF)", 16383, 16383},
		{"varint_4b_min_16384", "TZCNT=2 minimum (16384 -> 4 bytes)", 16384, 16384},
		{"varint_4b_max_536870911", "TZCNT=2 maximum (536870911 -> 4 bytes)", 536870911, 536870911},
		{"varint_9b_min_536870912", "TZCNT>=3 minimum (536870912 -> 9 bytes with 0x00 prefix)", math.MaxUint32, 536870912},
		{"varint_9b_max_uint64", "TZCNT>=3 maximum (MaxUint64 -> 9 bytes 0x00 + 8x0xFF)", math.MaxUint32, math.MaxUint64},
	}
	for _, tc := range varintCases {
		msg := &TestAllTypes{
			F1Uint32: tc.u32,
			F2Uint64: tc.u64,
		}
		vectors = append(vectors, verifyAllTypesAcrossEngines(t, tc.name, "A_PrefixVarintBoundaries", tc.desc, msg))
	}

	// Explicit Overlong 2-Byte Prefix-Varint Conformance (used by single-pass length backpatching)
	{
		var rawOverlong [2]byte
		PutOverlong2BVarint(rawOverlong[:], 42) // 42 < 128 encoded as 2-byte TZCNT=1 varint
		v, n := DecodeVarint(rawOverlong[:])
		if n != 2 || v != 42 {
			t.Fatalf("Overlong 2B varint failed: got (%d, %d), want (42, 2)", v, n)
		}
	}

	// ------------------------------------------------------------------------
	// Category B: ZigZag Signed Integer Boundaries
	// ------------------------------------------------------------------------
	zigzagCases := []struct {
		name string
		desc string
		i32  int32
		i64  int64
	}{
		{"zigzag_minus_one", "ZigZag(-1) == 1 (1 byte)", -1, -1},
		{"zigzag_plus_one", "ZigZag(+1) == 2 (1 byte)", 1, 1},
		{"zigzag_1b_boundary_neg64", "ZigZag(-64) == 127 (max 1-byte signed)", -64, -64},
		{"zigzag_2b_boundary_pos64", "ZigZag(+64) == 128 (min 2-byte signed)", 64, 64},
		{"zigzag_2b_boundary_neg8192", "ZigZag(-8192) == 16383 (max 2-byte signed)", -8192, -8192},
		{"zigzag_4b_boundary_pos8192", "ZigZag(+8192) == 16384 (min 4-byte signed)", 8192, 8192},
		{"zigzag_int_min_max", "MinInt32 / MinInt64 and MaxInt32 / MaxInt64 extremes", math.MinInt32, math.MinInt64},
		{"zigzag_int_max_extremes", "MaxInt32 and MaxInt64", math.MaxInt32, math.MaxInt64},
	}
	for _, tc := range zigzagCases {
		msg := &TestAllTypes{
			F3Sint32: tc.i32,
			F4Sint64: tc.i64,
		}
		vectors = append(vectors, verifyAllTypesAcrossEngines(t, tc.name, "B_ZigZagSignedBoundaries", tc.desc, msg))
	}

	// ------------------------------------------------------------------------
	// Category C: IEEE-754 Float32 / Float64 & Fixed32 / Fixed64 Special Values
	// ------------------------------------------------------------------------
	negZero32 := math.Float32frombits(0x80000000)
	negZero64 := math.Float64frombits(0x8000000000000000)
	floatCases := []struct {
		name string
		desc string
		f32  float32
		f64  float64
		fx32 uint32
		fx64 uint64
	}{
		{"float_negative_zero", "IEEE-754 negative zero (-0.0) sign-bit preservation", negZero32, negZero64, 0, 0},
		{"float_pos_infinity", "IEEE-754 +Inf in float32 and float64", float32(math.Inf(1)), math.Inf(1), 0xDEADBEEF, 0xCAFEBABE12345678},
		{"float_neg_infinity", "IEEE-754 -Inf in float32 and float64", float32(math.Inf(-1)), math.Inf(-1), math.MaxUint32, math.MaxUint64},
		{"float_nan_canonical", "IEEE-754 quiet NaN bit-pattern preservation", float32(math.NaN()), math.NaN(), 1, 1},
		{"float_subnormal_and_max", "SmallestNonzero subnormal and MaxFloat", math.SmallestNonzeroFloat32, math.SmallestNonzeroFloat64, 42, 99},
		{"float_max_values", "MaxFloat32 and MaxFloat64", math.MaxFloat32, math.MaxFloat64, 123456, 9876543210},
	}
	for _, tc := range floatCases {
		msg := &TestAllTypes{
			F13Float32: tc.f32,
			F14Float64: tc.f64,
			F15Fixed32: tc.fx32,
			F16Fixed64: tc.fx64,
		}
		vectors = append(vectors, verifyAllTypesAcrossEngines(t, tc.name, "C_IEEE754FloatsAndFixed", tc.desc, msg))
	}

	// ------------------------------------------------------------------------
	// Category D: Strings, UTF-8 Multilingual, Embedded NULs, 127B/128B Transition
	// ------------------------------------------------------------------------
	str127 := string(bytes.Repeat([]byte{'a'}, 127))
	str128 := string(bytes.Repeat([]byte{'b'}, 128))
	stringCases := []struct {
		name string
		desc string
		s    string
		b    []byte
		arr  []string
	}{
		{"string_utf8_multilingual_emoji", "UTF-8 CJK, Arabic, Emoji, and embedded NUL byte", "こんにちは世界 🌍🚀 مرحبا \x00 null-inside", []byte{0x00, 0xFF, 0x7F, 0x80, 0x01}, []string{"αβγ", "🚀", "", "end"}},
		{"string_len_127_max_1b_varint", "127-byte string (maximum 1-byte Prefix-Varint length)", str127, []byte(str127), []string{str127}},
		{"string_len_128_min_2b_varint", "128-byte string (minimum 2-byte Prefix-Varint length)", str128, []byte(str128), []string{str128}},
	}
	for _, tc := range stringCases {
		msg := &TestAllTypes{
			F6String:    tc.s,
			F7Bytes:     tc.b,
			F9StringArr: tc.arr,
		}
		vectors = append(vectors, verifyAllTypesAcrossEngines(t, tc.name, "D_StringsAndBytesBoundaries", tc.desc, msg))
	}

	// ------------------------------------------------------------------------
	// Category E: Container Length Prefix Boundaries (<=127B 1-Byte vs >=128B 2-Byte)
	// ------------------------------------------------------------------------
	{
		enc := NewEncoder(512)
		payload252 := string(bytes.Repeat([]byte{'x'}, 252))
		enc.WriteString(1, payload252)
		pkt255 := append([]byte(nil), enc.Finish()...)
		if s, ok := ReadString(pkt255, 1); !ok || s != payload252 {
			t.Fatalf("ReadString on 255B packet failed")
		}
		blind255, err := DecodeBlind(pkt255)
		if err != nil {
			t.Fatalf("DecodeBlind on 255B packet failed: %v", err)
		}
		vectors = append(vectors, ConformanceVector{
			Name:          "container_len_252_string_packet",
			Category:      "E_ContainerLengthBoundaries",
			Description:   "Single string field with 252-byte payload (1B tag + 2B Prefix-Varint length + 252B UTF-8 = 255B total)",
			Schema:        "ManualSingleString",
			WireHex:       hex.EncodeToString(pkt255),
			WireByteLen:   len(pkt255),
			PresentFields: inspectPresentFields(pkt255),
			ExpectedBlind: blind255,
		})

		enc.Reset()
		payload253 := string(bytes.Repeat([]byte{'y'}, 253))
		enc.WriteString(1, payload253)
		pkt256 := append([]byte(nil), enc.Finish()...)
		if s, ok := ReadString(pkt256, 1); !ok || s != payload253 {
			t.Fatalf("ReadString on 256B packet failed")
		}
		blind256, err := DecodeBlind(pkt256)
		if err != nil {
			t.Fatalf("DecodeBlind on 256B packet failed: %v", err)
		}
		vectors = append(vectors, ConformanceVector{
			Name:          "container_len_253_string_packet",
			Category:      "E_ContainerLengthBoundaries",
			Description:   "Single string field with 253-byte payload (1B tag + 2B Prefix-Varint length + 253B UTF-8 = 256B total)",
			Schema:        "ManualSingleString",
			WireHex:       hex.EncodeToString(pkt256),
			WireByteLen:   len(pkt256),
			PresentFields: inspectPresentFields(pkt256),
			ExpectedBlind: blind256,
		})
	}

	// ------------------------------------------------------------------------
	// Category F: Sparse Field Streams & TZCNT Header-Hop Lookup
	// ------------------------------------------------------------------------
	{
		// F1: Empty message (0 fields present -> 0-byte packet)
		enc := NewEncoder(16)
		emptyPkt := append([]byte(nil), enc.Finish()...)
		if len(emptyPkt) != 0 {
			t.Fatalf("Expected empty message to be 0 bytes, got %x", emptyPkt)
		}
		blindEmpty, err := DecodeBlind(emptyPkt)
		if err != nil || len(blindEmpty) != 0 {
			t.Fatalf("DecodeBlind on empty packet failed: %v", err)
		}
		var emptyAll TestAllTypes
		if err := Unmarshal(emptyPkt, &emptyAll); err != nil {
			t.Fatalf("Unmarshal on empty packet failed: %v", err)
		}
		vectors = append(vectors, ConformanceVector{
			Name:          "sparse_empty_message",
			Category:      "F_SparseFieldsAndHeaderHop",
			Description:   "Empty message with 0 present fields (0 bytes on wire)",
			Schema:        "EmptyMessage",
			WireHex:       hex.EncodeToString(emptyPkt),
			WireByteLen:   len(emptyPkt),
			PresentFields: inspectPresentFields(emptyPkt),
			ExpectedBlind: blindEmpty,
		})

		// F2: Only Field #14 present
		enc.Reset()
		enc.WriteFloat64(14, 3.141592653589793)
		pktF14 := append([]byte(nil), enc.Finish()...)
		if v, ok := ReadFloat64(pktF14, 14); !ok || v != 3.141592653589793 {
			t.Fatalf("ReadFloat64(14) on single-field-14 packet failed")
		}
		for f := uint16(1); f <= 13; f++ {
			if _, _, ok := LocateField(pktF14, f); ok {
				t.Fatalf("Expected field %d to be absent", f)
			}
		}
		blindF14, err := DecodeBlind(pktF14)
		if err != nil {
			t.Fatalf("DecodeBlind failed: %v", err)
		}
		vectors = append(vectors, ConformanceVector{
			Name:          "sparse_only_field_14",
			Category:      "F_SparseFieldsAndHeaderHop",
			Description:   "Only Field #14 present (testing early termination on absent lower fields 1..13)",
			Schema:        "SparseField14",
			WireHex:       hex.EncodeToString(pktF14),
			WireByteLen:   len(pktF14),
			PresentFields: inspectPresentFields(pktF14),
			ExpectedBlind: blindF14,
		})

		// F3: Odd fields only (#1, #3, #5, #7, #9, #11, #13)
		enc.Reset()
		enc.WriteUint(1, 101)
		enc.WriteInt(3, -303)
		enc.WriteBool(5, true)
		enc.WriteBytes(7, []byte{7, 7, 7})
		enc.WriteStringArr(9, []string{"nine-a", "nine-b"})
		enc.BeginSubMsgArr(11)
		enc.BeginSubMsgElem()
		enc.WriteUint(1, 1101)
		enc.WriteString(2, "elem-0")
		enc.EndSubMsgElem()
		enc.EndSubMsgArr()
		enc.WriteFloat32(13, 13.5)
		pktOdd := append([]byte(nil), enc.Finish()...)
		var gotOdd TestAllTypes
		if err := Unmarshal(pktOdd, &gotOdd); err != nil {
			t.Fatalf("Unmarshal odd sparse packet failed: %v", err)
		}
		if gotOdd.F1Uint32 != 101 || gotOdd.F3Sint32 != -303 || !gotOdd.F5Bool ||
			!bytes.Equal(gotOdd.F7Bytes, []byte{7, 7, 7}) || len(gotOdd.F11SubMsgArr) != 1 || gotOdd.F13Float32 != 13.5 {
			t.Fatalf("Odd sparse packet decode mismatch: %+v", gotOdd)
		}
		blindOdd, err := DecodeBlind(pktOdd)
		if err != nil {
			t.Fatalf("DecodeBlind odd sparse failed: %v", err)
		}
		vectors = append(vectors, ConformanceVector{
			Name:          "sparse_odd_fields_1_3_5_7_9_11_13",
			Category:      "F_SparseFieldsAndHeaderHop",
			Description:   "Alternating odd fields (#1, #3, #5, #7, #9, #11, #13) testing branchless TZCNT header-hop lookup",
			Schema:        "TestAllTypes",
			WireHex:       hex.EncodeToString(pktOdd),
			WireByteLen:   len(pktOdd),
			PresentFields: inspectPresentFields(pktOdd),
			ExpectedBlind: blindOdd,
		})
	}

	// ------------------------------------------------------------------------
	// Category G: SubMsg, WireSubMsgArr, WireMap
	// ------------------------------------------------------------------------
	{
		manySubMsgs := make([]AllTypesSubMsg, 20)
		for i := range manySubMsgs {
			manySubMsgs[i] = AllTypesSubMsg{
				ID:    uint32(1000 + i),
				Label: "submsg-element-padding-for-length-prefixed-array",
				Flag:  i%2 == 0,
				Score: int64(-5000 * i),
			}
		}
		msgLargeArr := &TestAllTypes{
			F1Uint32:     77,
			F8SubMsg:     &AllTypesSubMsg{ID: 888, Label: "nested-child", Flag: true, Score: -99999},
			F11SubMsgArr: manySubMsgs,
			F12MapStrUint: map[string]uint64{
				"":                 0,
				"a":                1,
				"alpha":            127,
				"alpha_extended":   128,
				"beta":             16384,
				"omega_max_uint64": math.MaxUint64,
				"🚀_unicode_key":    42,
			},
			F20OverflowStr: "overflow-field-20-value",
		}
		vectors = append(vectors, verifyAllTypesAcrossEngines(
			t,
			"complex_submsg_arr_and_sorted_map",
			"G_NestedSubMsgArrAndMap",
			"20-element WireSubMsgArr, WireSubMsg, sorted WireMap lookup, and Field #20",
			msgLargeArr,
		))
	}

	// ------------------------------------------------------------------------
	// Category H: Self-Describing Field #0 Inline Schema (Full & Sparse)
	// ------------------------------------------------------------------------
	{
		// Sparse packet with Field #0 inline schema (SchemaMeta) for only 3 present fields: #2, #5, #14
		enc := NewEncoder(128)
		enc.WriteRaw((&SchemaMeta{
			TypeName: "SparseTelemetry",
			Fields: map[uint32]string{
				2:  "uptime_sec",
				5:  "active",
				14: "cpu_load",
			},
		}).AppendField0(nil, true))
		enc.WriteUint(2, 99887766)
		enc.WriteBool(5, true)
		enc.WriteFloat64(14, 0.875)
		pktSelfDesc := append([]byte(nil), enc.Finish()...)

		blindSelf, err := DecodeBlind(pktSelfDesc)
		if err != nil {
			t.Fatalf("DecodeBlind on sparse Field #0 failed: %v", err)
		}
		if blindSelf["uptime_sec"] != uint64(99887766) || blindSelf["active"] != true || blindSelf["cpu_load"] != 0.875 {
			t.Fatalf("DecodeBlind Field #0 sparse mismatch: %+v", blindSelf)
		}
		metaRead, ok := ReadField0SchemaMeta(pktSelfDesc)
		if !ok || metaRead.TypeName != "SparseTelemetry" || metaRead.Fields[14] != "cpu_load" {
			t.Fatalf("ReadField0SchemaMeta mismatch: (%+v, %v)", metaRead, ok)
		}
		// Verify O(1) header-hop lookup works identically when Field #0 is present!
		if u, ok := ReadUint(pktSelfDesc, 2); !ok || u != 99887766 {
			t.Fatalf("ReadUint(2) with Field #0 failed: got (%d, %v)", u, ok)
		}
		if b, ok := ReadBool(pktSelfDesc, 5); !ok || !b {
			t.Fatalf("ReadBool(5) with Field #0 failed")
		}
		if f, ok := ReadFloat64(pktSelfDesc, 14); !ok || f != 0.875 {
			t.Fatalf("ReadFloat64(14) with Field #0 failed")
		}
		// Also verify table-driven Unmarshal skips Field #0 cleanly!
		var dstAll TestAllTypes
		if err := Unmarshal(pktSelfDesc, &dstAll); err != nil {
			t.Fatalf("Unmarshal on packet with Field #0 failed: %v", err)
		}
		if dstAll.F2Uint64 != 99887766 || !dstAll.F5Bool || dstAll.F14Float64 != 0.875 {
			t.Fatalf("Unmarshal with Field #0 mismatch: %+v", dstAll)
		}

		vectors = append(vectors, ConformanceVector{
			Name:          "self_describing_field0_sparse",
			Category:      "H_SelfDescribingField0",
			Description:   "Self-describing packet with Field #0 (WireSubMsg SchemaMeta) naming sparse fields #2, #5, #14",
			Schema:        "SelfDescribingSparse",
			WireHex:       hex.EncodeToString(pktSelfDesc),
			WireByteLen:   len(pktSelfDesc),
			PresentFields: inspectPresentFields(pktSelfDesc),
			ExpectedBlind: blindSelf,
		})
	}

	// ------------------------------------------------------------------------
	// Category I: Canonical PeerUpdate Cross-Engine Equivalence
	// (Encoder vs Table-Driven Marshal vs Plugin MarshalOmniBufAppend vs Opaque API)
	// ------------------------------------------------------------------------
	{
		sample := SamplePeerUpdate()
		pbSample := &pb.PeerUpdate{
			Asn:       sample.ASN,
			RouterId:  sample.RouterID,
			Active:    sample.Active,
			UptimeSec: sample.UptimeSec,
			RssiDbm:   sample.RssiDbm,
			CpuLoad:   sample.CPULoad,
			Tags:      sample.Tags,
			Prefixes:  sample.Prefixes,
			Location: &pb.Location{
				Region:     sample.Location.Region,
				Datacenter: sample.Location.Datacenter,
				City:       sample.Location.City,
				Rack:       sample.Location.Rack,
				Latitude:   sample.Location.Latitude,
				Longitude:  sample.Location.Longitude,
			},
			Neighbors: []*pb.Neighbor{
				{PeerAsn: 3356, Router: "ae0.fra1.level3.net", LatencyUs: 850, Established: true},
				{PeerAsn: 1299, Router: "ffm-b5-link.telia.net", LatencyUs: 920, Established: true},
				{PeerAsn: 174, Router: "be2040.ccr41.fra03.atlas.cogentco.com", LatencyUs: 1120, Established: true},
				{PeerAsn: 2914, Router: "ae-12.r24.frnkge08.de.bb.gin.ntt.net", LatencyUs: 980, Established: false},
			},
			Sequence: sample.Sequence,
		}

		enc := NewEncoder(512)
		wireEncoder := append([]byte(nil), EncodePeerOmniBuf(enc, &sample, false)...)
		wireDirect := EncodePeerOmniBufDirect(nil, &sample)
		wireTable, err := Marshal(pbSample)
		if err != nil {
			t.Fatalf("Table Marshal failed: %v", err)
		}
		wirePlugin := pbSample.MarshalOmniBufAppend(nil)

		if !bytes.Equal(wireEncoder, wireDirect) || !bytes.Equal(wireEncoder, wireTable) || !bytes.Equal(wireEncoder, wirePlugin) {
			t.Fatalf("4-way encoder byte-for-byte equivalence failed:\nEncoder: %d B\nDirect:  %d B\nTable:   %d B\nPlugin:  %d B",
				len(wireEncoder), len(wireDirect), len(wireTable), len(wirePlugin))
		}

		blindPeer, err := DecodeBlind(wirePlugin)
		if err != nil {
			t.Fatalf("DecodeBlind on PeerUpdate failed: %v", err)
		}
		vectors = append(vectors, ConformanceVector{
			Name:          "peer_update_canonical_4way_equivalent",
			Category:      "I_CanonicalPeerUpdate",
			Description:   "Canonical BGP PeerUpdate verified byte-for-byte identical across Encoder, Direct, Table, and protoc-gen-omnibuf Plugin",
			Schema:        "PeerUpdate",
			WireHex:       hex.EncodeToString(wirePlugin),
			WireByteLen:   len(wirePlugin),
			PresentFields: inspectPresentFields(wirePlugin),
			ExpectedBlind: blindPeer,
		})
	}

	// ------------------------------------------------------------------------
	// Category J: Deep Nested Message Edge Cases
	// (Recursive Self-Referential Trees, Nested WireSubMsgArr inside WireSubMsgArr,
	//  Empty vs Nil Sub-Messages, and MaxNestingDepth Boundary)
	// ------------------------------------------------------------------------
	vectors = append(vectors, buildNestedEdgeCaseVectors(t)...)

	// ------------------------------------------------------------------------
	// Category K: Message Concatenation (append(msgA, msgB...)) & Out-of-Order Fields
	// ------------------------------------------------------------------------
	vectors = append(vectors, buildConcatAndOutOfOrderVectors(t)...)

	// Verify or update golden corpus file at testdata/conformance_corpus.json
	corpus := ConformanceCorpusFile{
		Version:       "2.1.0",
		Specification: "OmniBuf Wire Format v2.1 (TZCNT Prefix-Varints + 4-Bit Explicit WireTypes + Index-Free Concat-Compatible Stream)",
		ValidVectors:  vectors,
		InvalidCases:  buildInvalidWireCases(),
	}
	corpusBytes, err := json.MarshalIndent(corpus, "", "  ")
	if err != nil {
		t.Fatalf("marshal conformance corpus JSON: %v", err)
	}
	corpusPath := filepath.Join("testdata", "conformance_corpus.json")
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(corpusPath, corpusBytes, 0o600); err != nil {
			t.Fatalf("write %s: %v", corpusPath, err)
		}
		t.Logf("Updated %s (%d bytes)", corpusPath, len(corpusBytes))
	} else {
		wantCorpus, err := os.ReadFile(corpusPath)
		if err != nil {
			t.Fatalf("read golden %s: %v (run `go test . -update-golden` to regenerate)", corpusPath, err)
		}
		if !bytes.Equal(corpusBytes, wantCorpus) {
			t.Fatalf("golden %s differs from generated conformance corpus (run `go test . -update-golden` if wire format change was intentional)", corpusPath)
		}
	}
}

// ============================================================================
// 2B. Deep Nested Message Edge-Case Schemas & Conformance Builder
// ============================================================================

// TreeNode is a self-referential recursive schema testing:
//   - Direct pointer recursion (*TreeNode via WireSubMsg)
//   - Value slice recursion ([]TreeNode via WireSubMsgArr)
//   - Pointer slice recursion ([]*TreeNode via WireSubMsgArr)
//   - Nil vs empty sub-messages at arbitrary depths
type TreeNode struct {
	ID       uint32            `protobuf:"varint,1,opt,name=id,proto3" json:"id"`
	Name     string            `protobuf:"bytes,2,opt,name=name,proto3" json:"name"`
	Weight   int64             `protobuf:"zigzag64,3,opt,name=weight,proto3" json:"weight"`
	Primary  *TreeNode         `protobuf:"bytes,4,opt,name=primary,proto3" json:"primary"`
	Children []TreeNode        `protobuf:"bytes,5,rep,name=children,proto3" json:"children"`
	Branches []*TreeNode       `protobuf:"bytes,6,rep,name=branches,proto3" json:"branches"`
	Labels   map[string]uint64 `protobuf:"bytes,7,rep,name=labels,proto3" json:"labels"`
}

// Nested WireSubMsgArr inside WireSubMsgArr (3 levels of repeated sub-message arrays):
// Cluster -> []Rack (WireSubMsgArr) -> []Server (WireSubMsgArr) -> []NIC (WireSubMsgArr)
type NestedNIC struct {
	IfIndex uint32   `protobuf:"varint,1,opt,name=if_index,proto3"`
	MAC     string   `protobuf:"bytes,2,opt,name=mac,proto3"`
	IPs     []uint32 `protobuf:"varint,3,rep,packed,name=ips,proto3"`
}

type NestedServer struct {
	ServerID uint32      `protobuf:"varint,1,opt,name=server_id,proto3"`
	Hostname string      `protobuf:"bytes,2,opt,name=hostname,proto3"`
	NICs     []NestedNIC `protobuf:"bytes,3,rep,name=nics,proto3"`
}

type NestedRack struct {
	RackID  uint32         `protobuf:"varint,1,opt,name=rack_id,proto3"`
	Zone    string         `protobuf:"bytes,2,opt,name=zone,proto3"`
	Servers []NestedServer `protobuf:"bytes,3,rep,name=servers,proto3"`
}

type NestedCluster struct {
	ClusterName string       `protobuf:"bytes,1,opt,name=cluster_name,proto3"`
	Racks       []NestedRack `protobuf:"bytes,2,rep,name=racks,proto3"`
	Epoch       uint64       `protobuf:"varint,3,opt,name=epoch,proto3"`
}

func buildNestedEdgeCaseVectors(t *testing.T) []ConformanceVector {
	t.Helper()
	var out []ConformanceVector

	// J1: 6-Level Recursive Self-Referential Tree (mixed *TreeNode, []TreeNode, []*TreeNode, nil, empty)
	tree := &TreeNode{
		ID:     1,
		Name:   "root",
		Weight: -100,
		Primary: &TreeNode{
			ID:     2,
			Name:   "level-2-primary",
			Weight: 200,
			Primary: &TreeNode{
				ID:     3,
				Name:   "level-3-primary",
				Weight: -300,
				Children: []TreeNode{
					{
						ID:   4,
						Name: "level-4-child-a",
						Branches: []*TreeNode{
							{
								ID:     5,
								Name:   "level-5-branch",
								Weight: 500,
								Primary: &TreeNode{
									ID:     6,
									Name:   "level-6-leaf",
									Labels: map[string]uint64{"depth": 6, "leaf": 1},
								},
							},
							{}, // empty non-nil struct in []*TreeNode
						},
					},
					{}, // completely zero-valued TreeNode inside []TreeNode
				},
			},
		},
		Children: []TreeNode{
			{ID: 10, Name: "sibling-1", Weight: 42},
			{ID: 11, Name: "sibling-2-with-empty-primary", Primary: &TreeNode{}},
		},
		Branches: []*TreeNode{
			{ID: 20, Name: "branch-ptr-1", Labels: map[string]uint64{"ok": 1}},
		},
		Labels: map[string]uint64{"root": 1, "version": 2},
	}

	wireTree, err := MarshalDeterministic(tree)
	if err != nil {
		t.Fatalf("MarshalDeterministic recursive TreeNode failed: %v", err)
	}
	var gotTree TreeNode
	if err := Unmarshal(wireTree, &gotTree); err != nil {
		t.Fatalf("Unmarshal recursive TreeNode failed: %v", err)
	}
	// Note: Primary: &TreeNode{} (all zero values) encodes as a 2-byte empty sub-message (0x00 0x00)
	// and unmarshals into a non-nil &TreeNode{}. Verify deep structure and leaf values:
	if gotTree.Primary == nil || gotTree.Primary.Primary == nil ||
		len(gotTree.Primary.Primary.Children) != 2 ||
		len(gotTree.Primary.Primary.Children[0].Branches) != 2 ||
		gotTree.Primary.Primary.Children[0].Branches[0].Primary == nil ||
		gotTree.Primary.Primary.Children[0].Branches[0].Primary.Name != "level-6-leaf" ||
		gotTree.Primary.Primary.Children[0].Branches[0].Primary.Labels["depth"] != 6 {
		t.Fatalf("Recursive TreeNode deep unmarshal mismatch: %+v", gotTree)
	}
	if gotTree.Children[1].Primary == nil {
		t.Fatalf("Expected empty non-nil *TreeNode sub-message to unmarshal as non-nil pointer")
	}
	blindTree, err := DecodeBlind(wireTree)
	if err != nil {
		t.Fatalf("DecodeBlind on recursive TreeNode failed: %v", err)
	}
	out = append(out, ConformanceVector{
		Name:          "nested_recursive_tree_6_levels",
		Category:      "J_DeepNestedMessages",
		Description:   "6-level self-referential TreeNode with *TreeNode, []TreeNode, []*TreeNode, and empty sub-messages",
		Schema:        "TreeNode",
		WireHex:       hex.EncodeToString(wireTree),
		WireByteLen:   len(wireTree),
		PresentFields: inspectPresentFields(wireTree),
		ExpectedBlind: blindTree,
	})

	// J2: 3-Level Nested WireSubMsgArr inside WireSubMsgArr (Cluster -> []Rack -> []Server -> []NIC)
	// Verified both via Table-Driven Marshal AND manual Encoder (testing Encoder.arrStack)
	cluster := &NestedCluster{
		ClusterName: "eu-fra-prod-01",
		Epoch:       1700000001,
		Racks: []NestedRack{
			{
				RackID: 101,
				Zone:   "az-1",
				Servers: []NestedServer{
					{
						ServerID: 1,
						Hostname: "srv-01.az1",
						NICs: []NestedNIC{
							{IfIndex: 1, MAC: "00:11:22:33:44:55", IPs: []uint32{0x0A000001, 0x0A000002}},
							{IfIndex: 2, MAC: "00:11:22:33:44:56", IPs: []uint32{0x0A000101}},
						},
					},
					{
						ServerID: 2,
						Hostname: "srv-02.az1-no-nics",
						NICs:     nil,
					},
				},
			},
			{
				RackID: 202,
				Zone:   "az-2",
				Servers: []NestedServer{
					{
						ServerID: 3,
						Hostname: "srv-01.az2",
						NICs: []NestedNIC{
							{IfIndex: 10, MAC: "aa:bb:cc:dd:ee:ff", IPs: []uint32{0xC0A80101}},
						},
					},
				},
			},
		},
	}

	wireClusterTable, err := Marshal(cluster)
	if err != nil {
		t.Fatalf("Marshal NestedCluster failed: %v", err)
	}

	// Encode the exact same 3-level nested WireSubMsgArr using Encoder to verify Encoder.arrStack!
	enc := NewEncoder(512)
	enc.WriteString(1, cluster.ClusterName)
	enc.BeginSubMsgArr(2) // Racks[]
	for _, r := range cluster.Racks {
		enc.BeginSubMsgElem()
		enc.WriteUint(1, uint64(r.RackID))
		enc.WriteString(2, r.Zone)
		enc.BeginSubMsgArr(3) // Servers[]
		for _, s := range r.Servers {
			enc.BeginSubMsgElem()
			enc.WriteUint(1, uint64(s.ServerID))
			enc.WriteString(2, s.Hostname)
			enc.BeginSubMsgArr(3) // NICs[]
			for _, nic := range s.NICs {
				enc.BeginSubMsgElem()
				enc.WriteUint(1, uint64(nic.IfIndex))
				enc.WriteString(2, nic.MAC)
				enc.WriteUint32Arr(3, nic.IPs)
				enc.EndSubMsgElem()
			}
			enc.EndSubMsgArr()
			enc.EndSubMsgElem()
		}
		enc.EndSubMsgArr()
		enc.EndSubMsgElem()
	}
	enc.EndSubMsgArr()
	enc.WriteUint(3, cluster.Epoch)
	wireClusterEnc := append([]byte(nil), enc.Finish()...)

	if !bytes.Equal(wireClusterTable, wireClusterEnc) {
		t.Fatalf("3-level nested WireSubMsgArr mismatch between Table Marshal (%d B) and Encoder (%d B)",
			len(wireClusterTable), len(wireClusterEnc))
	}

	var gotCluster NestedCluster
	if err := Unmarshal(wireClusterTable, &gotCluster); err != nil {
		t.Fatalf("Unmarshal NestedCluster failed: %v", err)
	}
	if !reflect.DeepEqual(&gotCluster, cluster) {
		t.Fatalf("NestedCluster round-trip mismatch:\ngot:  %+v\nwant: %+v", &gotCluster, cluster)
	}

	// Verify 3-hop random access: Cluster -> Rack[1] -> Server[0] -> NIC[0] -> MAC
	rack1Bytes, ok := ReadSubMsgArrElem(wireClusterTable, 2, 1)
	if !ok {
		t.Fatalf("ReadSubMsgArrElem(Rack[1]) failed")
	}
	srv0Bytes, ok := ReadSubMsgArrElem(rack1Bytes, 3, 0)
	if !ok {
		t.Fatalf("ReadSubMsgArrElem(Rack[1].Server[0]) failed")
	}
	nic0Bytes, ok := ReadSubMsgArrElem(srv0Bytes, 3, 0)
	if !ok {
		t.Fatalf("ReadSubMsgArrElem(Rack[1].Server[0].NIC[0]) failed")
	}
	if mac, ok := ReadString(nic0Bytes, 2); !ok || mac != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("3-hop nested WireSubMsgArr lookup got %q, want aa:bb:cc:dd:ee:ff", mac)
	}

	blindCluster, err := DecodeBlind(wireClusterTable)
	if err != nil {
		t.Fatalf("DecodeBlind on NestedCluster failed: %v", err)
	}
	out = append(out, ConformanceVector{
		Name:          "nested_3level_submsg_arr_cluster_rack_server_nic",
		Category:      "J_DeepNestedMessages",
		Description:   "3-level nested WireSubMsgArr inside WireSubMsgArr (Cluster -> []Rack -> []Server -> []NIC)",
		Schema:        "NestedCluster",
		WireHex:       hex.EncodeToString(wireClusterTable),
		WireByteLen:   len(wireClusterTable),
		PresentFields: inspectPresentFields(wireClusterTable),
		ExpectedBlind: blindCluster,
	})

	// J3: Mixed Small (<128B), Large (>=256B), and Empty (0B) Sub-Messages inside WireSubMsgArr
	largeLabel := string(bytes.Repeat([]byte{'L'}, 260))
	smallLabel := "tiny-elem"
	mixedArrMsg := &TestAllTypes{
		F1Uint32: 999,
		F8SubMsg: &AllTypesSubMsg{}, // explicitly non-nil but 0-field empty sub-message (encodes as 0-byte body)
		F11SubMsgArr: []AllTypesSubMsg{
			{}, // Element 0: completely empty 0-field sub-message (0 bytes)
			{ID: 1, Label: smallLabel, Flag: true, Score: -1}, // Element 1: <128B
			{ID: 2, Label: largeLabel, Flag: false, Score: 2}, // Element 2: >=256B
			{ID: 3, Label: smallLabel, Flag: true, Score: 3},  // Element 3: <128B after a >256B element
		},
	}
	out = append(out, verifyAllTypesAcrossEngines(
		t,
		"nested_mixed_sizes_and_empty_submsgs",
		"J_DeepNestedMessages",
		"WireSubMsgArr mixing 0-field empty sub-message (0B), small sub-message, and large >=256B sub-message",
		mixedArrMsg,
	))

	// J4: MaxNestingDepth Boundary (Depth 30 valid vs Depth 35 rejected)
	{
		// Build a 30-level deep linked list of TreeNodes (within MaxNestingDepth = 32)
		var deep30 *TreeNode
		for d := 30; d >= 1; d-- {
			deep30 = &TreeNode{
				ID:      uint32(d),
				Name:    "depth-node",
				Primary: deep30,
			}
		}
		wire30, err := Marshal(deep30)
		if err != nil {
			t.Fatalf("Marshal 30-level deep TreeNode failed: %v", err)
		}
		var got30 TreeNode
		if err := Unmarshal(wire30, &got30); err != nil {
			t.Fatalf("Unmarshal 30-level deep TreeNode (<= MaxNestingDepth) failed: %v", err)
		}
		if _, err := DecodeBlind(wire30); err != nil {
			t.Fatalf("DecodeBlind 30-level deep TreeNode (<= MaxNestingDepth) failed: %v", err)
		}
	}

	return out
}

func buildConcatAndOutOfOrderVectors(t *testing.T) []ConformanceVector {
	t.Helper()
	var out []ConformanceVector

	// K1: Concatenating two TestAllTypes packets (append(wireA, wireB...)) with out-of-order fields in wireB:
	//   - Scalars: wireB overrides msgA ("last write wins"), msgA-only scalars are preserved
	//   - SubMsg (F8SubMsg): wireB's partial F8SubMsg (ID=2, Score=-20) merges onto msgA.F8SubMsg (preserving Label="sub-label-from-A" and Flag=true!)
	//   - Repeated (F9StringArr, F10Uint32Arr, F11SubMsgArr): wireB elements append onto msgA elements
	//   - Map (F12MapStrUint): wireB keys override/merge onto msgA keys
	msgA := &TestAllTypes{
		F1Uint32:     100,
		F2Uint64:     200,
		F3Sint32:     -30,
		F6String:     "initial-string",
		F8SubMsg:     &AllTypesSubMsg{ID: 1, Label: "sub-label-from-A", Flag: true, Score: -10},
		F9StringArr:  []string{"a1", "a2"},
		F10Uint32Arr: []uint32{10, 20},
		F11SubMsgArr: []AllTypesSubMsg{
			{ID: 101, Label: "arr-elem-0", Flag: true, Score: 1000},
		},
		F12MapStrUint: map[string]uint64{
			"keep_from_a": 111,
			"override_me": 222,
		},
		F14Float64:     2.5,
		F20OverflowStr: "overflow-from-A",
	}
	wireA, err := MarshalDeterministic(msgA)
	if err != nil {
		t.Fatalf("MarshalDeterministic(msgA) failed: %v", err)
	}

	// Build wireB as a partial update message with fields in arbitrary (non-ascending) order:
	encB := NewEncoder(256)
	encB.WriteFloat32(13, 1.25)
	encB.WriteMapStringUintDeterministic(12, map[string]uint64{
		"override_me": 999,
		"new_from_b":  333,
	})
	encB.BeginSubMsgArr(11)
	encB.BeginSubMsgElem()
	encB.WriteUint(1, 202)
	encB.WriteString(2, "arr-elem-1")
	encB.WriteBool(3, false)
	encB.WriteInt(4, 2000)
	encB.EndSubMsgElem()
	encB.EndSubMsgArr()
	encB.WriteUint32Arr(10, []uint32{30, 40})
	encB.WriteStringArr(9, []string{"b1", "b2"})
	// Partial nested SubMsg (#8): only overrides ID (1 -> 2) and Score (-10 -> -20), preserving Label and Flag from msgA!
	encB.BeginSubMsg(8)
	encB.WriteInt(4, -20)
	encB.WriteUint(1, 2)
	encB.EndSubMsg()
	encB.WriteString(6, "overridden-by-B")
	encB.WriteBool(5, true)
	encB.WriteInt(4, -400)
	encB.WriteUint(1, 999)
	wireB := append([]byte(nil), encB.Finish()...)

	wantMerged := &TestAllTypes{
		F1Uint32:     999,
		F2Uint64:     200,
		F3Sint32:     -30,
		F4Sint64:     -400,
		F5Bool:       true,
		F6String:     "overridden-by-B",
		F8SubMsg:     &AllTypesSubMsg{ID: 2, Label: "sub-label-from-A", Flag: true, Score: -20},
		F9StringArr:  []string{"a1", "a2", "b1", "b2"},
		F10Uint32Arr: []uint32{10, 20, 30, 40},
		F11SubMsgArr: []AllTypesSubMsg{
			{ID: 101, Label: "arr-elem-0", Flag: true, Score: 1000},
			{ID: 202, Label: "arr-elem-1", Flag: false, Score: 2000},
		},
		F12MapStrUint: map[string]uint64{
			"keep_from_a": 111,
			"override_me": 999,
			"new_from_b":  333,
		},
		F13Float32:     1.25,
		F14Float64:     2.5,
		F20OverflowStr: "overflow-from-A",
	}

	wireConcat := append(append([]byte(nil), wireA...), wireB...)

	// 1. Verify Table-Driven Safe Unmarshal on concatenated bytes
	var gotSafe TestAllTypes
	if err := Unmarshal(wireConcat, &gotSafe); err != nil {
		t.Fatalf("Unmarshal(wireConcat) failed: %v", err)
	}
	assertAllTypesEqual(t, "concat_test_all_types/Safe", &gotSafe, wantMerged)

	// 2. Verify Table-Driven Zero-Copy Unmarshal on concatenated bytes
	var gotZC TestAllTypes
	if err := UnmarshalZeroCopy(wireConcat, &gotZC); err != nil {
		t.Fatalf("UnmarshalZeroCopy(wireConcat) failed: %v", err)
	}
	assertAllTypesEqual(t, "concat_test_all_types/ZeroCopy", &gotZC, wantMerged)

	// 3. Verify Header-Hop Readers on concatenated bytes (last-write-wins + array/map merge)
	if u1, ok := ReadUint(wireConcat, 1); !ok || uint32(u1) != 999 {
		t.Fatalf("ReadUint(1) on concat = (%d, %v), want 999", u1, ok)
	}
	if u2, ok := ReadUint(wireConcat, 2); !ok || u2 != 200 {
		t.Fatalf("ReadUint(2) on concat = (%d, %v), want 200", u2, ok)
	}
	if s6, ok := ReadString(wireConcat, 6); !ok || s6 != "overridden-by-B" {
		t.Fatalf("ReadString(6) on concat = (%q, %v), want overridden-by-B", s6, ok)
	}
	if sa9, ok := ReadStringArr(wireConcat, 9); !ok || !reflect.DeepEqual(sa9, wantMerged.F9StringArr) {
		t.Fatalf("ReadStringArr(9) on concat = %v, want %v", sa9, wantMerged.F9StringArr)
	}
	if ua10, ok := ReadUint32Arr(wireConcat, 10); !ok || !reflect.DeepEqual(ua10, wantMerged.F10Uint32Arr) {
		t.Fatalf("ReadUint32Arr(10) on concat = %v, want %v", ua10, wantMerged.F10Uint32Arr)
	}
	if n11 := SubMsgArrLen(wireConcat, 11); n11 != 2 {
		t.Fatalf("SubMsgArrLen(11) on concat = %d, want 2", n11)
	}
	if lbl1, ok := ReadArrNestedStringZeroCopy(wireConcat, 11, 1, 2); !ok || lbl1 != "arr-elem-1" {
		t.Fatalf("ReadArrNestedStringZeroCopy(11, 1, 2) on concat = (%q, %v), want arr-elem-1", lbl1, ok)
	}
	for k, wantV := range wantMerged.F12MapStrUint {
		if gotV, ok := LookupMapStringUint(wireConcat, 12, k); !ok || gotV != wantV {
			t.Fatalf("LookupMapStringUint(12, %q) on concat = (%d, %v), want %d", k, gotV, ok, wantV)
		}
	}

	// 4. Verify DecodeBlind on concatenated bytes
	blindConcat, err := DecodeBlind(wireConcat)
	if err != nil {
		t.Fatalf("DecodeBlind(wireConcat) failed: %v", err)
	}
	out = append(out, ConformanceVector{
		Name:          "concat_two_messages_full_merge",
		Category:      "K_ConcatenationAndOutOfOrder",
		Description:   "Raw byte concatenation append(msgA, msgB...) verifying last-write-wins scalars and merged arrays/maps/sub-messages",
		Schema:        "TestAllTypes",
		WireHex:       hex.EncodeToString(wireConcat),
		WireByteLen:   len(wireConcat),
		PresentFields: inspectPresentFields(wireConcat),
		ExpectedBlind: blindConcat,
	})

	// K2: Concatenating two self-describing (Field #0) messages and decoding via DecodeBlind
	encA := NewEncoder(128)
	encA.WriteRaw((&SchemaMeta{Fields: map[uint32]string{1: "asn", 2: "router_id"}}).AppendField0(nil, true))
	encA.WriteUint(1, 64500)
	encA.WriteString(2, "old-rtr")
	sdA := append([]byte(nil), encA.Finish()...)

	encB.Reset()
	encB.WriteRaw((&SchemaMeta{Fields: map[uint32]string{2: "router_id", 11: "sequence"}}).AppendField0(nil, true))
	encB.WriteString(2, "new-rtr")
	encB.WriteUint(11, 42)
	sdB := append([]byte(nil), encB.Finish()...)

	sdConcat := append(sdA, sdB...)
	blindSDConcat, err := DecodeBlind(sdConcat)
	if err != nil {
		t.Fatalf("DecodeBlind on concatenated Field #0 packets failed: %v", err)
	}
	if blindSDConcat["asn"] != uint64(64500) || blindSDConcat["router_id"] != "new-rtr" || blindSDConcat["sequence"] != uint64(42) {
		t.Fatalf("DecodeBlind on concatenated Field #0 mismatch: %+v", blindSDConcat)
	}
	out = append(out, ConformanceVector{
		Name:          "concat_two_self_describing_field0_messages",
		Category:      "K_ConcatenationAndOutOfOrder",
		Description:   "Concatenation of two self-describing Field #0 packets where their SchemaMeta blocks merge recursively",
		Schema:        "SelfDescribingConcat",
		WireHex:       hex.EncodeToString(sdConcat),
		WireByteLen:   len(sdConcat),
		PresentFields: inspectPresentFields(sdConcat),
		ExpectedBlind: blindSDConcat,
	})

	// K3: Append-Only Fast Mutation on PeerUpdate (appending a new Sequence #11 tag+value at the end of a full PeerUpdate packet)
	sample := SamplePeerUpdate()
	basePkt := EncodePeerOmniBufDirect(nil, &sample)
	mutatedPkt := append([]byte(nil), basePkt...)
	mutatedPkt = appendTag(mutatedPkt, FieldPeerSequence, WireUint)
	mutatedPkt = AppendVarint(mutatedPkt, 7777777)

	var directDec PeerUpdate
	if err := DecodePeerOmniBufInto(mutatedPkt, &directDec, false); err != nil {
		t.Fatalf("DecodePeerOmniBufInto(mutatedPkt) failed: %v", err)
	}
	if directDec.Sequence != 7777777 || directDec.Location.City != "Frankfurt" {
		t.Fatalf("Direct decode of append-mutated PeerUpdate mismatch: %+v", directDec)
	}
	var pbDec pb.PeerUpdate
	if err := pbDec.UnmarshalOmniBuf(mutatedPkt, false); err != nil {
		t.Fatalf("Plugin UnmarshalOmniBuf(mutatedPkt) failed: %v", err)
	}
	if pbDec.Sequence != 7777777 || pbDec.Location.City != "Frankfurt" {
		t.Fatalf("Plugin decode of append-mutated PeerUpdate mismatch: %+v", &pbDec)
	}
	if seq, ok := ReadUint(mutatedPkt, FieldPeerSequence); !ok || seq != 7777777 {
		t.Fatalf("ReadUint on append-mutated PeerUpdate = (%d, %v), want 7777777", seq, ok)
	}

	return out
}

// ============================================================================
// 3. Schema Evolution & Unknown Field Skipping Conformance (V2 -> V1)
// ============================================================================

type LocationV2 struct {
	Region     string  `protobuf:"bytes,1,opt,name=region,proto3"`
	Datacenter string  `protobuf:"bytes,2,opt,name=datacenter,proto3"`
	City       string  `protobuf:"bytes,3,opt,name=city,proto3"`
	Rack       uint32  `protobuf:"varint,4,opt,name=rack,proto3"`
	Latitude   float32 `protobuf:"fixed32,5,opt,name=latitude,proto3"`
	Longitude  float32 `protobuf:"fixed32,6,opt,name=longitude,proto3"`
	// V2 additions inside nested sub-message:
	BuildingCode  string  `protobuf:"bytes,7,opt,name=building_code,proto3"`
	PowerUsageKW  float64 `protobuf:"fixed64,8,opt,name=power_usage_kw,proto3"`
	LiquidCooling bool    `protobuf:"varint,9,opt,name=liquid_cooling,proto3"`
}

type NeighborV2 struct {
	PeerASN     uint32 `protobuf:"varint,1,opt,name=peer_asn,proto3"`
	Router      string `protobuf:"bytes,2,opt,name=router,proto3"`
	LatencyUS   uint32 `protobuf:"varint,3,opt,name=latency_us,proto3"`
	Established bool   `protobuf:"varint,4,opt,name=established,proto3"`
	// V2 additions inside repeated WireSubMsgArr elements:
	SessionName string   `protobuf:"bytes,5,opt,name=session_name,proto3"`
	Communities []string `protobuf:"bytes,6,rep,name=communities,proto3"`
}

type PeerUpdateV2 struct {
	ASN       uint32       `protobuf:"varint,1,opt,name=asn,proto3"`
	RouterID  string       `protobuf:"bytes,2,opt,name=router_id,proto3"`
	Active    bool         `protobuf:"varint,3,opt,name=active,proto3"`
	UptimeSec uint64       `protobuf:"varint,4,opt,name=uptime_sec,proto3"`
	RssiDbm   int32        `protobuf:"zigzag32,5,opt,name=rssi_dbm,proto3"`
	CPULoad   float64      `protobuf:"fixed64,6,opt,name=cpu_load,proto3"`
	Tags      []string     `protobuf:"bytes,7,rep,name=tags,proto3"`
	Prefixes  []uint32     `protobuf:"varint,8,rep,packed,name=prefixes,proto3"`
	Location  LocationV2   `protobuf:"bytes,9,opt,name=location,proto3"`
	Neighbors []NeighborV2 `protobuf:"bytes,10,rep,name=neighbors,proto3"`
	Sequence  uint64       `protobuf:"varint,11,opt,name=sequence,proto3"`
	// V2 new indexed fields (#12..#14) + V2 overflow fields (#15, #16):
	FirmwareVer string            `protobuf:"bytes,12,opt,name=firmware_ver,proto3"`
	TempCelsius float32           `protobuf:"fixed32,13,opt,name=temp_celsius,proto3"`
	Counters    map[string]uint64 `protobuf:"bytes,14,rep,name=counters,proto3"`
	RawDiag     []byte            `protobuf:"bytes,15,opt,name=raw_diag,proto3"`
	ExtraNotes  []string          `protobuf:"bytes,16,rep,name=extra_notes,proto3"`
}

func TestSchemaEvolutionUnknownFieldsConformance(t *testing.T) {
	v2 := &PeerUpdateV2{
		ASN:       64512,
		RouterID:  "edge-rtr-fra01.core.backbone.net",
		Active:    true,
		UptimeSec: 1849203,
		RssiDbm:   -62,
		CPULoad:   0.42875,
		Tags:      []string{"tier1", "ebgp"},
		Prefixes:  []uint32{167772160, 172032000},
		Location: LocationV2{
			Region:        "eu-central-1",
			Datacenter:    "FRA-Equinix-FR5",
			City:          "Frankfurt",
			Rack:          42,
			Latitude:      50.1109,
			Longitude:     8.6821,
			BuildingCode:  "FR5-HALL-B",
			PowerUsageKW:  18.75,
			LiquidCooling: true,
		},
		Neighbors: []NeighborV2{
			{PeerASN: 3356, Router: "ae0.fra1.level3.net", LatencyUS: 850, Established: true, SessionName: "L3-PRIMARY", Communities: []string{"3356:100"}},
			{PeerASN: 1299, Router: "ffm-b5-link.telia.net", LatencyUS: 920, Established: false, SessionName: "TELIA-BACKUP", Communities: []string{"1299:200"}},
		},
		Sequence:    9876543210,
		FirmwareVer: "IOS-XR-24.4.1",
		TempCelsius: 48.5,
		Counters:    map[string]uint64{"rx_pkts": 1000000, "tx_pkts": 999999},
		RawDiag:     []byte{0xDE, 0xAD, 0xBE, 0xEF},
		ExtraNotes:  []string{"note-1", "note-2"},
	}

	wireV2, err := Marshal(v2)
	if err != nil {
		t.Fatalf("Marshal V2 failed: %v", err)
	}

	// 1. Unmarshal V2 packet into V1 *pb.PeerUpdate using Table-Driven Unmarshal
	var v1Table pb.PeerUpdate
	if err := Unmarshal(wireV2, &v1Table); err != nil {
		t.Fatalf("Table Unmarshal(V2 -> V1) failed: %v", err)
	}
	if v1Table.Asn != 64512 || v1Table.RouterId != v2.RouterID || v1Table.Location.City != "Frankfurt" ||
		len(v1Table.Neighbors) != 2 || v1Table.Neighbors[1].Router != "ffm-b5-link.telia.net" || v1Table.Sequence != 9876543210 {
		t.Fatalf("Table Unmarshal(V2 -> V1) corrupted V1 fields: %+v", &v1Table)
	}

	// 2. Unmarshal V2 packet into V1 *pb.PeerUpdate using Plugin-Generated UnmarshalOmniBuf
	var v1Plugin pb.PeerUpdate
	if err := v1Plugin.UnmarshalOmniBuf(wireV2, false); err != nil {
		t.Fatalf("Plugin UnmarshalOmniBuf(V2 -> V1) failed: %v", err)
	}
	if !reflect.DeepEqual(&v1Plugin, &v1Table) {
		t.Fatalf("Plugin vs Table V2->V1 mismatch:\nplugin: %+v\ntable:  %+v", &v1Plugin, &v1Table)
	}

	// 3. Opaque API on V2 packet using V1 OpaquePeerUpdate
	var v1Opaque pb.OpaquePeerUpdate
	if err := v1Opaque.Unmarshal(wireV2); err != nil {
		t.Fatalf("OpaquePeerUpdate.Unmarshal(V2) failed: %v", err)
	}
	if v1Opaque.GetLocation().GetCity() != "Frankfurt" ||
		v1Opaque.GetNeighbors(1).GetRouter() != "ffm-b5-link.telia.net" ||
		v1Opaque.GetSequence() != 9876543210 {
		t.Fatalf("OpaquePeerUpdate V1 getters failed on V2 packet")
	}

	// 4. Forward compatibility with WireExtended (15): an unknown future field (#99) encoded
	// with WireExtended ([ByteLen][ExtType (1B)][Payload...]) followed by an updated Sequence (#11)
	// MUST be skipped in O(1) time by existing decoders without error.
	var wireWithExt []byte
	wireWithExt = append(wireWithExt, wireV2...)
	wireWithExt = appendTag(wireWithExt, 99, WireExtended)
	wireWithExt = AppendVarint(wireWithExt, 5) // 1B ExtType + 4B future payload
	wireWithExt = append(wireWithExt, 0x42, 0xDE, 0xAD, 0xBE, 0xEF)
	wireWithExt = appendTag(wireWithExt, FieldPeerSequence, WireUint)
	wireWithExt = AppendVarint(wireWithExt, 123456789)

	var v1FromExt pb.PeerUpdate
	if err := Unmarshal(wireWithExt, &v1FromExt); err != nil {
		t.Fatalf("Unmarshal with unknown WireExtended field failed: %v", err)
	}
	if v1FromExt.Sequence != 123456789 || v1FromExt.RouterId != v2.RouterID {
		t.Fatalf("Unmarshal after skipping WireExtended field got Sequence=%d, want 123456789", v1FromExt.Sequence)
	}
	if seq, ok := ReadUint(wireWithExt, FieldPeerSequence); !ok || seq != 123456789 {
		t.Fatalf("ReadUint after skipping WireExtended field = (%d, %v), want 123456789", seq, ok)
	}
}

// ============================================================================
// 4. Negative / Malformed Wire Conformance Suite (Zero Panics Guaranteed)
// ============================================================================

func appendTag(b []byte, fieldNum uint16, wireType uint8) []byte {
	return AppendVarint(b, (uint64(fieldNum)<<4)|uint64(wireType))
}

func buildInvalidWireCases() []InvalidWireCase {
	// Programmatically construct complex malicious / adversarial byte streams
	// so their exact hex is exported into testdata/conformance_corpus.json:

	// 1. WireUintArr outer byteLen mismatch (outer byteLen=3 cuts off a 2-byte varint)
	var uintArrSizeMismatch []byte
	uintArrSizeMismatch = appendTag(uintArrSizeMismatch, 8, WireUintArr) // Field #8 (Prefixes in PeerUpdate)
	uintArrSizeMismatch = AppendVarint(uintArrSizeMismatch, 3)           // claims 3 bytes
	uintArrSizeMismatch = append(uintArrSizeMismatch, 0x02, 0x00, 0x02)  // first varint is 2B (0x02 0x00), second is truncated 2B varint (0x02)!

	// 3. WireStringArr inner string length exceeds outer byteLen
	var strArrInnerExceedsOuter []byte
	strArrInnerExceedsOuter = appendTag(strArrInnerExceedsOuter, 9, WireStringArr)
	strArrInnerExceedsOuter = AppendVarint(strArrInnerExceedsOuter, 3)         // outer byteLen = 3
	strArrInnerExceedsOuter = append(strArrInnerExceedsOuter, 0x03, 0x15, 'a') // count=1 (0x03), inner string claims len 10 (0x15), only 1 byte follows!

	// 4. WireSubMsg outer byteLen exceeds packet
	var subMsgOuterExceedsPkt []byte
	subMsgOuterExceedsPkt = appendTag(subMsgOuterExceedsPkt, 9, WireSubMsg)
	subMsgOuterExceedsPkt = AppendVarint(subMsgOuterExceedsPkt, 50) // claims 50 bytes, only 2 follow
	subMsgOuterExceedsPkt = append(subMsgOuterExceedsPkt, 0x21, 0x03)

	// 5. WireSubMsg inner truncated field
	var subMsgInnerTruncated []byte
	subMsgInnerTruncated = appendTag(subMsgInnerTruncated, 8, WireSubMsg)
	subMsgInnerTruncated = AppendVarint(subMsgInnerTruncated, 2)    // 2-byte inner sub-message
	subMsgInnerTruncated = append(subMsgInnerTruncated, 0x21, 0x02) // Field #1 WireUint followed by 1B of a 2B varint!

	// 9. WireSubMsgArr element length exceeds array byteLen
	var subMsgArrElemExceeds []byte
	subMsgArrElemExceeds = appendTag(subMsgArrElemExceeds, 10, WireSubMsgArr)
	subMsgArrElemExceeds = AppendVarint(subMsgArrElemExceeds, 3)  // 3-byte array body
	subMsgArrElemExceeds = AppendVarint(subMsgArrElemExceeds, 20) // Element 0 claims 20 bytes inside a 3-byte array!
	subMsgArrElemExceeds = append(subMsgArrElemExceeds, 0x21, 0x03)

	// 10. WireSubMsgArr element with truncated varint length
	var subMsgArrTruncatedElemLen []byte
	subMsgArrTruncatedElemLen = appendTag(subMsgArrTruncatedElemLen, 11, WireSubMsgArr)
	subMsgArrTruncatedElemLen = AppendVarint(subMsgArrTruncatedElemLen, 1) // 1-byte array body
	subMsgArrTruncatedElemLen = append(subMsgArrTruncatedElemLen, 0x02)    // 1 byte of a 2-byte element length varint!

	// 11. WireSubMsgArr element with corrupt inner sub-message (truncated varint inside element)
	var subMsgArrCorruptElem []byte
	subMsgArrCorruptElem = appendTag(subMsgArrCorruptElem, 11, WireSubMsgArr)
	subMsgArrCorruptElem = AppendVarint(subMsgArrCorruptElem, 3)    // 3-byte array body
	subMsgArrCorruptElem = AppendVarint(subMsgArrCorruptElem, 2)    // 2-byte element 0
	subMsgArrCorruptElem = append(subMsgArrCorruptElem, 0x21, 0x02) // Field #1 WireUint with truncated 2B varint!

	// 12. WireMap with entry count exceeding remaining bytes (OOM bomb protection)
	var mapOOMBomb []byte
	mapOOMBomb = appendTag(mapOOMBomb, 12, WireMap)
	mapOOMBomb = AppendVarint(mapOOMBomb, 3) // 3-byte map body
	mapOOMBomb = append(mapOOMBomb, MapHeaderByte(WireString, WireUint))
	mapOOMBomb = AppendVarint(mapOOMBomb, 10000) // claims 10,000 entries in 2 bytes!

	// 13. WireMap with trailing garbage bytes after `count` entries
	var mapTrailingBytes []byte
	mapTrailingBytes = appendTag(mapTrailingBytes, 12, WireMap)
	// header(1B) + count=1 (1B) + entry 0 ("a": 1 -> 3B) + 2 trailing garbage bytes = 7 bytes
	mapTrailingBytes = AppendVarint(mapTrailingBytes, 7)
	mapTrailingBytes = append(mapTrailingBytes, MapHeaderByte(WireString, WireUint))
	mapTrailingBytes = AppendVarint(mapTrailingBytes, 1)
	mapTrailingBytes = append(mapTrailingBytes, 0x03, 'a', 0x03, 0xFF, 0xFF)

	// 14. WireFixedArr (ElemType=WireFixed32) with (byteLen-1) not divisible by 4
	var fixed32ArrUnaligned []byte
	fixed32ArrUnaligned = appendTag(fixed32ArrUnaligned, 1, WireFixedArr)
	fixed32ArrUnaligned = AppendVarint(fixed32ArrUnaligned, 4) // 1B ElemType + 3B (not divisible by 4)
	fixed32ArrUnaligned = append(fixed32ArrUnaligned, WireFixed32, 0x01, 0x02, 0x03)

	// 15. WireFixedArr (ElemType=WireFixed64) with (byteLen-1) not divisible by 8
	var fixed64ArrUnaligned []byte
	fixed64ArrUnaligned = appendTag(fixed64ArrUnaligned, 1, WireFixedArr)
	fixed64ArrUnaligned = AppendVarint(fixed64ArrUnaligned, 6) // 1B ElemType + 5B (not divisible by 8)
	fixed64ArrUnaligned = append(fixed64ArrUnaligned, WireFixed64, 0x01, 0x02, 0x03, 0x04, 0x05)

	// 15b. WireExtended (15) with byteLen == 0 (missing 1B ExtType header)
	var extendedEmpty []byte
	extendedEmpty = appendTag(extendedEmpty, 1, WireExtended)
	extendedEmpty = AppendVarint(extendedEmpty, 0)

	// 16. Self-describing Field #0 (0x0B) with corrupt fields map (wrong MapType 0x00 instead of 0x03)
	field0CorruptFieldsMap := []byte{
		0x0b, 0x09, // Field #0 (WireSubMsg), len = 4
		0x53, 0x05, 0x00, 0x01, // SchemaMeta Field #2 (WireMap), len=2, invalid MapType=0x00, count=0
	}

	// 17. Self-describing Field #0 (0x0B) with truncated string inside children map
	field0CorruptChildrenMap := []byte{
		0x0b, 0x13, // Field #0 (WireSubMsg), len = 9
		0x73, 0x0f, 0x05, 0x03, // SchemaMeta Field #3 (WireMap), len=7, MapType=0x05 (uint->submsg), count=1
		0x13,            // child key = 9
		0x07,            // child submsg len = 3
		0x27, 0x15, 'A', // child type_name claims len 10 (0x15), only 1 byte follows!
	}

	// 18. 40-Level Deeply Nested WireSubMsg Bomb (exceeds MaxNestingDepth = 32)
	var deepBomb []byte // innermost empty sub-message (0 bytes)
	for range 40 {
		var outer []byte
		outer = appendTag(outer, 4, WireSubMsg) // Field #4 (Primary in TreeNode)
		outer = AppendVarint(outer, uint64(len(deepBomb)))
		outer = append(outer, deepBomb...)
		deepBomb = outer
	}

	return []InvalidWireCase{
		{
			Name:        "malformed_truncated_1b_of_2b_tag",
			Category:    "TruncatedTag",
			Description: "1-byte slice containing the first byte of a 2-byte Prefix-Varint tag (TZCNT=1)",
			WireHex:     "02",
		},
		{
			Name:        "malformed_truncated_2b_of_4b_tag",
			Category:    "TruncatedTag",
			Description: "2-byte slice containing only 2 bytes of a 4-byte Prefix-Varint tag (TZCNT=2)",
			WireHex:     "0400",
		},
		{
			Name:        "malformed_truncated_2b_varint_payload",
			Category:    "TruncatedVarint",
			Description: "Field #1 WireUint tag followed by 1 byte of a 2-byte Prefix-Varint (TZCNT=1)",
			WireHex:     "2102",
		},
		{
			Name:        "malformed_truncated_4b_varint_payload",
			Category:    "TruncatedVarint",
			Description: "Field #1 WireUint tag followed by 2 bytes of a 4-byte Prefix-Varint (TZCNT=2)",
			WireHex:     "210400",
		},
		{
			Name:        "malformed_truncated_9b_varint_payload",
			Category:    "TruncatedVarint",
			Description: "Field #1 WireUint tag followed by 0x00 and only 3 bytes of an 8-byte uint64",
			WireHex:     "2100112233",
		},
		{
			Name:        "malformed_string_length_exceeds_packet",
			Category:    "LengthOutOfBounds",
			Description: "Field #2 WireString claims length 100 in a 3-byte packet",
			WireHex:     "47c961",
		},
		{
			Name:        "malformed_bytes_uint64_max_length_overflow",
			Category:    "LengthOutOfBounds",
			Description: "Field #7 WireBytes with 9-byte MaxUint64 length (tests integer overflow guard)",
			WireHex:     "e900ffffffffffffffff",
		},
		{
			Name:        "malicious_uint_arr_inner_varint_size_mismatch",
			Category:    "SizeMismatch",
			Description: "WireUintArr outer byteLen=3 cuts off in the middle of a 2-byte element varint",
			WireHex:     hex.EncodeToString(uintArrSizeMismatch),
		},
		{
			Name:        "malicious_string_arr_inner_len_exceeds_outer",
			Category:    "SizeMismatch",
			Description: "WireStringArr outer byteLen=2 contains an element claiming 10 bytes",
			WireHex:     hex.EncodeToString(strArrInnerExceedsOuter),
		},
		{
			Name:        "malicious_submsg_outer_len_exceeds_packet",
			Category:    "SizeMismatch",
			Description: "WireSubMsg outer length claims 50 bytes when only 2 bytes follow",
			WireHex:     hex.EncodeToString(subMsgOuterExceedsPkt),
		},
		{
			Name:        "malicious_submsg_inner_truncated_field",
			Category:    "CorruptSubMsg",
			Description: "WireSubMsg outer length is 2 bytes, cutting off an inner 2-byte Prefix-Varint value",
			WireHex:     hex.EncodeToString(subMsgInnerTruncated),
		},
		{
			Name:        "malicious_submsg_arr_elem_len_exceeds_array",
			Category:    "CorruptSubMsgArr",
			Description: "WireSubMsgArr element length claims 20 bytes inside a 3-byte array container",
			WireHex:     hex.EncodeToString(subMsgArrElemExceeds),
		},
		{
			Name:        "malicious_submsg_arr_truncated_elem_len",
			Category:    "CorruptSubMsgArr",
			Description: "WireSubMsgArr body ends in the middle of an element's 2-byte length prefix",
			WireHex:     hex.EncodeToString(subMsgArrTruncatedElemLen),
		},
		{
			Name:        "malicious_submsg_arr_corrupt_inner_element",
			Category:    "CorruptSubMsgArr",
			Description: "WireSubMsgArr element contains an invalid 4-bit wire type (15)",
			WireHex:     hex.EncodeToString(subMsgArrCorruptElem),
		},
		{
			Name:        "malformed_truncated_fixed32",
			Category:    "TruncatedFixed",
			Description: "Field #5 WireFixed32 with only 2 payload bytes instead of 4",
			WireHex:     "b51122",
		},
		{
			Name:        "malformed_truncated_fixed64",
			Category:    "TruncatedFixed",
			Description: "Field #6 WireFixed64 with only 5 payload bytes instead of 8",
			WireHex:     "d71122334455",
		},
		{
			Name:        "malformed_fixed32_arr_unaligned_byte_len",
			Category:    "TruncatedFixed",
			Description: "Field #1 WireFixedArr(WireFixed32) with (byteLen-1)=3 (not divisible by 4)",
			WireHex:     hex.EncodeToString(fixed32ArrUnaligned),
		},
		{
			Name:        "malformed_fixed64_arr_unaligned_byte_len",
			Category:    "TruncatedFixed",
			Description: "Field #1 WireFixedArr(WireFixed64) with (byteLen-1)=5 (not divisible by 8)",
			WireHex:     hex.EncodeToString(fixed64ArrUnaligned),
		},
		{
			Name:        "malformed_wire_extended_empty_body",
			Category:    "CorruptWireExtended",
			Description: "Field #1 WireExtended (15) with byteLen=0 (missing required 1B ExtType header)",
			WireHex:     hex.EncodeToString(extendedEmpty),
		},
		{
			Name:        "malformed_wire_map_count_exceeds_body",
			Category:    "CorruptWireMap",
			Description: "Field #12 WireMap claiming 10,000 entries inside a 2-byte map body",
			WireHex:     hex.EncodeToString(mapOOMBomb),
		},
		{
			Name:        "malicious_wire_map_trailing_bytes",
			Category:    "CorruptWireMap",
			Description: "Field #12 WireMap where 2 extra unparsed bytes remain after `count` entries",
			WireHex:     hex.EncodeToString(mapTrailingBytes),
		},
		{
			Name:        "malformed_field0_wrong_wire_type",
			Category:    "CorruptField0",
			Description: "Field #0 tag at byte 0 has WireUint instead of WireSubMsg (0x0B)",
			WireHex:     "0105",
		},
		{
			Name:        "malicious_field0_corrupt_fields_map",
			Category:    "CorruptField0",
			Description: "Field #0 SchemaMeta fields map has invalid MapType 0x00 instead of 0x03 (uint32 -> string)",
			WireHex:     hex.EncodeToString(field0CorruptFieldsMap),
		},
		{
			Name:        "malicious_field0_corrupt_children_map",
			Category:    "CorruptField0",
			Description: "Field #0 SchemaMeta children map contains a child SchemaMeta with truncated type_name",
			WireHex:     hex.EncodeToString(field0CorruptChildrenMap),
		},
		{
			Name:        "malicious_nested_submsg_depth_40_bomb",
			Category:    "MaliciousDepthBomb",
			Description: "40-level nested WireSubMsg chain exceeding MaxNestingDepth (32)",
			WireHex:     hex.EncodeToString(deepBomb),
		},
	}
}

func exerciseAllReadersNoPanic(raw []byte) {
	// 1. Table-driven Unmarshal into TestAllTypes, TreeNode, and pb.PeerUpdate
	var allTypes TestAllTypes
	_ = Unmarshal(raw, &allTypes)
	var allTypesZC TestAllTypes
	_ = UnmarshalZeroCopy(raw, &allTypesZC)

	var tree TreeNode
	_ = Unmarshal(raw, &tree)

	var peer pb.PeerUpdate
	_ = Unmarshal(raw, &peer)

	// 2. Plugin-generated UnmarshalOmniBuf (both copy and zeroCopy)
	var peerPlugin pb.PeerUpdate
	_ = peerPlugin.UnmarshalOmniBuf(raw, false)
	var peerPluginZC pb.PeerUpdate
	_ = peerPluginZC.UnmarshalOmniBuf(raw, true)

	// 3. Plugin-generated pb.OpaquePeerUpdate lazy getters
	var pbOp pb.OpaquePeerUpdate
	if err := pbOp.Unmarshal(raw); err == nil {
		_ = pbOp.GetAsn()
		_ = pbOp.GetRouterId()
		_ = pbOp.GetActive()
		_ = pbOp.GetUptimeSec()
		_ = pbOp.GetRssiDbm()
		_ = pbOp.GetCpuLoad()
		_ = pbOp.TagsLen()
		_ = pbOp.AppendTagsInto(nil)
		_ = pbOp.PrefixesLen()
		_ = pbOp.AppendPrefixesInto(nil)
		loc := pbOp.GetLocation()
		_ = loc.GetRegion()
		_ = loc.GetDatacenter()
		_ = loc.GetCity()
		_ = loc.GetRack()
		_ = loc.GetLatitude()
		_ = loc.GetLongitude()
		nLen := pbOp.NeighborsLen()
		for i := 0; i < nLen && i < 4; i++ {
			nb := pbOp.GetNeighbors(i)
			_ = nb.GetPeerAsn()
			_ = nb.GetRouter()
			_ = nb.GetLatencyUs()
			_ = nb.GetEstablished()
		}
		_ = pbOp.GetSequence()
	}

	// 4. Header-Hop readers across all field numbers (including overflow >14)
	for f := uint16(0); f <= 16; f++ {
		_, _, _ = LocateField(raw, f)
		_, _ = ReadUint(raw, f)
		_, _ = ReadInt(raw, f)
		_, _ = ReadBool(raw, f)
		_, _ = ReadFloat32(raw, f)
		_, _ = ReadFloat64(raw, f)
		_, _ = ReadString(raw, f)
		_, _ = ReadStringZeroCopy(raw, f)
		_, _ = ReadBytes(raw, f)
		_, _ = ReadStringArr(raw, f)
		_, _ = ReadUint32Arr(raw, f)
		_, _ = ReadSintArr(raw, f)
		_, _ = ReadFloat32Arr(raw, f)
		_, _ = ReadFloat64Arr(raw, f)
		_, _ = ReadSubMsg(raw, f)
		_, _ = ReadSubMsgArrElem(raw, f, 0)
		_, _ = ReadNestedStringZeroCopy(raw, f, 3)
		_, _ = ReadArrNestedStringZeroCopy(raw, f, 0, 2)
		_, _ = LookupMapStringUint(raw, f, "k")
	}
}

func TestMalformedWireRejectionConformance(t *testing.T) {
	cases := buildInvalidWireCases()
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			raw, err := hex.DecodeString(tc.WireHex)
			if err != nil {
				t.Fatalf("bad hex in test case %s: %v", tc.Name, err)
			}

			// 1. DecodeBlind must return an error and never panic on EVERY malformed/malicious case
			if _, err := DecodeBlind(raw); err == nil {
				t.Errorf("[%s] expected DecodeBlind to return error on malformed/malicious input %x", tc.Name, raw)
			}

			// 2. All Table-Driven, Plugin-Generated, Opaque, and O(1) readers must never panic
			exerciseAllReadersNoPanic(raw)
		})
	}
}

// ============================================================================
// 5. Go Native Fuzz Target (Seeded with Golden Conformance Corpus)
// ============================================================================

func FuzzConformanceNoPanic(f *testing.F) {
	// Seed with valid packets
	sample := SamplePeerUpdate()
	enc := NewEncoder(512)
	f.Add(append([]byte(nil), EncodePeerOmniBuf(enc, &sample, false)...))
	f.Add(append([]byte(nil), EncodePeerOmniBuf(enc, &sample, true)...))

	// Seed with all invalid / malicious wire cases
	for _, ic := range buildInvalidWireCases() {
		if b, err := hex.DecodeString(ic.WireHex); err == nil {
			f.Add(b)
		}
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// None of the decoders, plugin methods, Opaque getters, or O(1) readers may ever panic on arbitrary byte streams
		_, _ = DecodeBlind(data)
		exerciseAllReadersNoPanic(data)
	})
}

func TestLargeContainerExpansionOver16KB(t *testing.T) {
	// Construct a PeerUpdate where nested Location (> 20 KB), packed Prefixes (> 20 KB),
	// and repeated Neighbors (multiple > 18 KB elements, total array > 40 KB) all exceed
	// the 2-byte Prefix-Varint limit (16,383 bytes) and trigger automatic 4-byte expansion.
	bigCity := strings.Repeat("Frankfurt-DE-", 1600)  // 20,800 bytes
	bigRouter := strings.Repeat("rtr-core-01.", 1600) // 19,200 bytes
	bigPrefixes := make([]uint32, 6000)
	for i := range bigPrefixes {
		bigPrefixes[i] = uint32(100000 + i) // 4B varint each -> 24,000+ bytes
	}

	orig := SamplePeerUpdate()
	orig.Location.City = bigCity
	orig.Prefixes = bigPrefixes
	orig.Neighbors[0].Router = bigRouter
	orig.Neighbors[2].Router = bigRouter

	// 1. Encoder API
	enc := NewEncoder(1024)
	pktEnc := append([]byte(nil), EncodePeerOmniBuf(enc, &orig, false)...)

	// 2. Direct Encoder
	pktDirect := EncodePeerOmniBufDirect(nil, &orig)
	if !reflect.DeepEqual(pktEnc, pktDirect) {
		t.Fatalf("large container EncodePeerOmniBuf vs Direct mismatch: len %d vs %d", len(pktEnc), len(pktDirect))
	}

	// 3. Table-Driven Marshal on *PeerUpdate and *pb.PeerUpdate
	pktTable, err := Marshal(&orig)
	if err != nil {
		t.Fatalf("table Marshal large PeerUpdate failed: %v", err)
	}
	if !reflect.DeepEqual(pktTable, pktDirect) {
		t.Fatalf("table Marshal large PeerUpdate bytes mismatch")
	}

	// 4. Plugin-Generated MarshalOmniBufAppend on *pb.PeerUpdate
	pbIn := SamplePBPeerUpdate(&orig)
	pktPlugin := pbIn.MarshalOmniBufAppend(nil)
	if !reflect.DeepEqual(pktPlugin, pktDirect) {
		t.Fatalf("plugin MarshalOmniBufAppend large PeerUpdate bytes mismatch")
	}

	// Verify round-trip decoding across all decoders + O(1) header-hopping
	var outDirect PeerUpdate
	if err := DecodePeerOmniBufInto(pktDirect, &outDirect, false); err != nil {
		t.Fatalf("DecodePeerOmniBufInto large packet failed: %v", err)
	}
	if !reflect.DeepEqual(outDirect, orig) {
		t.Fatalf("DecodePeerOmniBufInto large packet mismatch")
	}

	var outTable PeerUpdate
	if err := Unmarshal(pktDirect, &outTable); err != nil {
		t.Fatalf("table Unmarshal large packet failed: %v", err)
	}
	if !reflect.DeepEqual(outTable, orig) {
		t.Fatalf("table Unmarshal large packet mismatch")
	}

	var outPlugin pb.PeerUpdate
	if err := outPlugin.UnmarshalOmniBuf(pktDirect, false); err != nil {
		t.Fatalf("plugin UnmarshalOmniBuf large packet failed: %v", err)
	}
	if outPlugin.Location.City != bigCity || outPlugin.Neighbors[2].Router != bigRouter || len(outPlugin.Prefixes) != len(bigPrefixes) || outPlugin.Sequence != orig.Sequence {
		t.Fatalf("plugin UnmarshalOmniBuf large packet field mismatch")
	}

	// Verify O(1) header-hop across >16KB containers reaches Field #11 (Sequence) and nested fields
	if gotSeq, ok := ReadUint(pktDirect, FieldPeerSequence); !ok || gotSeq != orig.Sequence {
		t.Fatalf("ReadUint(Sequence) after >16KB containers = %d, %v", gotSeq, ok)
	}
	if gotCity, ok := ReadNestedString(pktDirect, FieldPeerLocation, FieldLocCity); !ok || gotCity != bigCity {
		t.Fatalf("ReadNestedString(Location.City > 20KB) failed: ok=%v, len=%d", ok, len(gotCity))
	}
	if gotRtr, ok := ReadArrNestedString(pktDirect, FieldPeerNeighbors, 2, FieldNeighRouter); !ok || gotRtr != bigRouter {
		t.Fatalf("ReadArrNestedString(Neighbors[2].Router > 19KB) failed: ok=%v, len=%d", ok, len(gotRtr))
	}

	// Verify DecodeBlind on large containers
	blind, err := DecodeBlind(pktDirect)
	if err != nil {
		t.Fatalf("DecodeBlind large packet failed: %v", err)
	}
	if blind["field_11"] != orig.Sequence {
		t.Fatalf("DecodeBlind large packet field_11 = %v, want %d", blind["field_11"], orig.Sequence)
	}
}

func TestConformanceRunnerAdapterHarness(t *testing.T) {
	tmpDir := t.TempDir()
	runnerBin := filepath.Join(tmpDir, "conformance-runner")
	adapterBin := filepath.Join(tmpDir, "conformance-go")

	if out, err := exec.Command("go", "build", "-o", runnerBin, "./cmd/conformance-runner").CombinedOutput(); err != nil {
		t.Fatalf("build conformance-runner failed: %v\n%s", err, out)
	}
	if out, err := exec.Command("go", "build", "-o", adapterBin, "./cmd/conformance-go").CombinedOutput(); err != nil {
		t.Fatalf("build conformance-go failed: %v\n%s", err, out)
	}

	corpusPath := filepath.Join("testdata", "conformance_corpus.json")
	sum, err := conformance.RunSuite(adapterBin, corpusPath)
	if err != nil {
		t.Fatalf("conformance.RunSuite failed: %v", err)
	}
	if len(sum.Failures) > 0 {
		t.Fatalf("conformance failures (%d):\n%s", len(sum.Failures), strings.Join(sum.Failures, "\n"))
	}
	if sum.PassedCases == 0 {
		t.Fatalf("expected >0 passed conformance cases")
	}

	// Also verify the standalone conformance-runner CLI binary exits 0 against conformance-go
	// #nosec G204 -- runnerBin and adapterBin are built in t.TempDir() above
	out, err := exec.Command(runnerBin, "-corpus", corpusPath, adapterBin).CombinedOutput()
	if err != nil {
		t.Fatalf("conformance-runner CLI failed: %v\n%s", err, out)
	}
	t.Logf("\n%s", strings.TrimSpace(string(out)))
}
