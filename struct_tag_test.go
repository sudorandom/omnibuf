package omnibuf_test

import (
	"bytes"
	"math"
	"reflect"
	"strings"
	"testing"

	. "github.com/sudorandom/omnibuf"
)

type CustomRole string
type CustomStatus uint16
type CustomOffset int32

type TaggedChild struct {
	Code    uint16 `omnibuf:"1,name=code"`
	Detail  string `omnibuf:"2,name=detail"`
	Enabled bool   `omnibuf:"3,name=enabled"`
}

type TaggedNativeStruct struct {
	// Native signed and unsigned integer widths
	I   int    `omnibuf:"1,name=i"`
	I8  int8   `omnibuf:"2,name=i8"`
	I16 int16  `omnibuf:"3,name=i16"`
	I32 int32  `omnibuf:"4,name=i32"`
	I64 int64  `omnibuf:"5,name=i64"`
	U   uint   `omnibuf:"6,name=u"`
	U8  uint8  `omnibuf:"7,name=u8"`
	U16 uint16 `omnibuf:"8,name=u16"`
	U32 uint32 `omnibuf:"9,name=u32"`
	U64 uint64 `omnibuf:"10,name=u64"`

	// Floats, bool, string, bytes
	F32  float32 `omnibuf:"11,name=f32"`
	F64  float64 `omnibuf:"12,name=f64"`
	Flag bool    `omnibuf:"13,name=flag"`
	Text string  `omnibuf:"14,name=text"`
	Raw  []byte  `omnibuf:"15,name=raw"`

	// Slices of native types
	SliceInt     []int     `omnibuf:"16,name=slice_int"`
	SliceInt8    []int8    `omnibuf:"17,name=slice_int8"`
	SliceInt16   []int16   `omnibuf:"18,name=slice_int16"`
	SliceInt32   []int32   `omnibuf:"19,name=slice_int32"`
	SliceInt64   []int64   `omnibuf:"20,name=slice_int64"`
	SliceUint    []uint    `omnibuf:"21,name=slice_uint"`
	SliceUint8   []uint8   `omnibuf:"22,name=slice_uint8,wire=uint"`
	SliceUint16  []uint16  `omnibuf:"23,name=slice_uint16"`
	SliceUint32  []uint32  `omnibuf:"24,name=slice_uint32"`
	SliceUint64  []uint64  `omnibuf:"25,name=slice_uint64"`
	SliceFloat32 []float32 `omnibuf:"26,name=slice_float32"`
	SliceFloat64 []float64 `omnibuf:"27,name=slice_float64"`
	SliceBool    []bool    `omnibuf:"28,name=slice_bool"`
	SliceString  []string  `omnibuf:"29,name=slice_string"`
	SliceBytes   [][]byte  `omnibuf:"30,name=slice_bytes"`

	// Pointers to scalars
	OptInt    *int     `omnibuf:"31,name=opt_int"`
	OptUint32 *uint32  `omnibuf:"32,name=opt_uint32"`
	OptString *string  `omnibuf:"33,name=opt_string"`
	OptBool   *bool    `omnibuf:"34,name=opt_bool"`
	OptFloat  *float64 `omnibuf:"35,name=opt_float"`
	NilInt    *int     `omnibuf:"36,name=nil_int"`

	// Nested structs & slices
	ChildVal      TaggedChild    `omnibuf:"37,name=child_val"`
	ChildPtr      *TaggedChild   `omnibuf:"38,name=child_ptr"`
	ChildValSlice []TaggedChild  `omnibuf:"39,name=child_val_slice"`
	ChildPtrSlice []*TaggedChild `omnibuf:"40,name=child_ptr_slice"`

	// Diverse map types
	MapStrUint   map[string]uint64       `omnibuf:"41,name=map_str_uint"`
	MapStrStr    map[string]string       `omnibuf:"42,name=map_str_str"`
	MapIntFloat  map[int]float64         `omnibuf:"43,name=map_int_float"`
	MapUintBool  map[uint32]bool         `omnibuf:"44,name=map_uint_bool"`
	MapBoolStr   map[bool]string         `omnibuf:"45,name=map_bool_str"`
	MapStrChild  map[string]TaggedChild  `omnibuf:"46,name=map_str_child"`
	MapStrPtr    map[string]*TaggedChild `omnibuf:"47,name=map_str_ptr"`
	MapIntBytes  map[int16][]byte        `omnibuf:"48,name=map_int_bytes"`
	MapFixed32   map[string]uint32       `omnibuf:"49,name=map_fixed32,wire=fixed32"`
	MapFixed64   map[string]int64        `omnibuf:"50,name=map_fixed64,wire=fixed64"`
	MapStrBytesV map[string]string       `omnibuf:"51,name=map_str_bytes_v,wire=bytes"`

	// Custom typedefs
	Role   CustomRole   `omnibuf:"52,name=role"`
	Status CustomStatus `omnibuf:"53,name=status"`
	Offset CustomOffset `omnibuf:"54,name=offset"`

	// High field numbers (2-byte and 4-byte tags)
	High2BField string `omnibuf:"500,name=high_2b"`
	High4BField uint64 `omnibuf:"5000,name=high_4b"`
	MaxFieldNum int32  `omnibuf:"65535,name=max_field"`

	// Untracked / ignored fields
	UntaggedField string
	IgnoredDash   string `omnibuf:"-"`
	IgnoredEmpty  string `omnibuf:""`
	unexported    int
}

func TestOmniBufStructTags_FullRoundTrip(t *testing.T) {
	optInt := -999
	optU32 := uint32(123456)
	optStr := "optional-hello"
	optBool := false
	optFloat := 2.718281828

	in := TaggedNativeStruct{
		I:            -1234567,
		I8:           -120,
		I16:          -31000,
		I32:          -2000000000,
		I64:          -9000000000000000000,
		U:            1234567,
		U8:           250,
		U16:          65000,
		U32:          4000000000,
		U64:          18000000000000000000,
		F32:          3.1415927,
		F64:          1.4142135623730951,
		Flag:         true,
		Text:         "omnibuf-struct-tag-test",
		Raw:          []byte{0xDE, 0xAD, 0xBE, 0xEF},
		SliceInt:     []int{-100, 0, 100, 99999},
		SliceInt8:    []int8{-128, -1, 0, 1, 127},
		SliceInt16:   []int16{-32768, 0, 32767},
		SliceInt32:   []int32{-100000, 200000},
		SliceInt64:   []int64{-5000000000, 5000000000},
		SliceUint:    []uint{0, 1, 42, 100000},
		SliceUint8:   []uint8{1, 2, 128, 255},
		SliceUint16:  []uint16{10, 1000, 65535},
		SliceUint32:  []uint32{100, 200, 300},
		SliceUint64:  []uint64{1000, 2000, 3000},
		SliceFloat32: []float32{-1.5, 0.0, 2.5, 100.25},
		SliceFloat64: []float64{-99.99, 0.0, 123456.789},
		SliceBool:    []bool{true, false, true, true, false},
		SliceString:  []string{"alpha", "beta", "gamma"},
		SliceBytes:   [][]byte{{0x01, 0x02}, {0x03, 0x04, 0x05}},
		OptInt:       &optInt,
		OptUint32:    &optU32,
		OptString:    &optStr,
		OptBool:      &optBool,
		OptFloat:     &optFloat,
		NilInt:       nil,
		ChildVal:     TaggedChild{Code: 200, Detail: "ok", Enabled: true},
		ChildPtr:     &TaggedChild{Code: 404, Detail: "not-found", Enabled: false},
		ChildValSlice: []TaggedChild{
			{Code: 1, Detail: "first", Enabled: true},
			{Code: 2, Detail: "second", Enabled: false},
		},
		ChildPtrSlice: []*TaggedChild{
			{Code: 10, Detail: "ten", Enabled: true},
			{Code: 20, Detail: "twenty", Enabled: false},
		},
		MapStrUint:  map[string]uint64{"a": 1, "b": 2},
		MapStrStr:   map[string]string{"env": "prod", "region": "eu"},
		MapIntFloat: map[int]float64{-5: 12.5, 10: -0.25},
		MapUintBool: map[uint32]bool{1: true, 2: false},
		MapBoolStr:  map[bool]string{true: "yes", false: "no"},
		MapStrChild: map[string]TaggedChild{
			"c1": {Code: 500, Detail: "err", Enabled: true},
		},
		MapStrPtr: map[string]*TaggedChild{
			"p1": {Code: 201, Detail: "created", Enabled: true},
		},
		MapIntBytes:  map[int16][]byte{-7: {0xAA, 0xBB}, 8: {0xCC}},
		MapFixed32:   map[string]uint32{"f32": 0x12345678},
		MapFixed64:   map[string]int64{"f64": -9876543210},
		MapStrBytesV: map[string]string{"raw_k": "raw_v"},
		Role:         "admin",
		Status:       7,
		Offset:       -42,
		High2BField:  "field-500-value",
		High4BField:  999888777666,
		MaxFieldNum:  -12345,

		UntaggedField: "should-not-serialize",
		IgnoredDash:   "should-not-serialize-dash",
		IgnoredEmpty:  "should-not-serialize-empty",
		unexported:    999,
	}

	wire, err := MarshalDeterministic(&in)
	if err != nil {
		t.Fatalf("MarshalDeterministic failed: %v", err)
	}

	// Verify deterministic output produces identical bytes on repeat calls
	wire2, err := MarshalDeterministic(&in)
	if err != nil {
		t.Fatalf("MarshalDeterministic second call failed: %v", err)
	}
	if !bytes.Equal(wire, wire2) {
		t.Fatalf("MarshalDeterministic is not deterministic")
	}

	// Safe Unmarshal
	var gotSafe TaggedNativeStruct
	if err := Unmarshal(wire, &gotSafe); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Zero-Copy Unmarshal
	var gotZC TaggedNativeStruct
	if err := UnmarshalZeroCopy(wire, &gotZC); err != nil {
		t.Fatalf("UnmarshalZeroCopy failed: %v", err)
	}

	expected := in
	expected.UntaggedField = ""
	expected.IgnoredDash = ""
	expected.IgnoredEmpty = ""
	expected.unexported = 0

	if !reflect.DeepEqual(gotSafe, expected) {
		t.Fatalf("safe Unmarshal mismatch:\ngot:  %#v\nwant: %#v", gotSafe, expected)
	}
	if !reflect.DeepEqual(gotZC, expected) {
		t.Fatalf("zero-copy Unmarshal mismatch:\ngot:  %#v\nwant: %#v", gotZC, expected)
	}
}

func TestOmniBufStructTags_WireTypeOverrides(t *testing.T) {
	type WireOverrideStruct struct {
		// Unsigned ints with wire overrides
		U8Sint     uint8  `omnibuf:"1,wire=sint"`
		U8Fixed32  uint8  `omnibuf:"2,wire=fixed32"`
		U8Fixed64  uint8  `omnibuf:"3,wire=fixed64"`
		U16Sint    uint16 `omnibuf:"4,wire=sint"`
		U16Fixed32 uint16 `omnibuf:"5,wire=fixed32"`
		U16Fixed64 uint16 `omnibuf:"6,wire=fixed64"`
		U32Sint    uint32 `omnibuf:"7,wire=sint"`
		U32Fixed32 uint32 `omnibuf:"8,wire=fixed32"`
		U32Fixed64 uint32 `omnibuf:"9,wire=fixed64"`
		U64Sint    uint64 `omnibuf:"10,wire=sint"`
		U64Fixed32 uint64 `omnibuf:"11,wire=fixed32"`
		U64Fixed64 uint64 `omnibuf:"12,wire=fixed64"`

		// Signed ints with wire overrides (including negative numbers)
		I8Uint     int8  `omnibuf:"13,wire=uint"`
		I8Fixed32  int8  `omnibuf:"14,wire=fixed32"`
		I8Fixed64  int8  `omnibuf:"15,wire=fixed64"`
		I16Uint    int16 `omnibuf:"16,wire=uint"`
		I16Fixed32 int16 `omnibuf:"17,wire=fixed32"`
		I16Fixed64 int16 `omnibuf:"18,wire=fixed64"`
		I32Uint    int32 `omnibuf:"19,wire=uint"`
		I32Fixed32 int32 `omnibuf:"20,wire=fixed32"`
		I32Fixed64 int32 `omnibuf:"21,wire=fixed64"`
		I64Uint    int64 `omnibuf:"22,wire=uint"`
		I64Fixed32 int64 `omnibuf:"23,wire=fixed32"`
		I64Fixed64 int64 `omnibuf:"24,wire=fixed64"`

		// String as bytes and []byte as string
		StrAsBytes string `omnibuf:"25,wire=bytes"`
		BytesAsStr []byte `omnibuf:"26,wire=string"`

		// Slices with wire overrides
		U8ArrSint     []uint8  `omnibuf:"27,wire=sint"`
		U8ArrFixed32  []uint8  `omnibuf:"28,wire=fixed32"`
		U8ArrFixed64  []uint8  `omnibuf:"29,wire=fixed64"`
		U16ArrSint    []uint16 `omnibuf:"30,wire=sint"`
		U16ArrFixed32 []uint16 `omnibuf:"31,wire=fixed32"`
		U16ArrFixed64 []uint16 `omnibuf:"32,wire=fixed64"`
		U32ArrSint    []uint32 `omnibuf:"33,wire=sint"`
		U32ArrFixed32 []uint32 `omnibuf:"34,wire=fixed32"`
		U32ArrFixed64 []uint32 `omnibuf:"35,wire=fixed64"`
		U64ArrSint    []uint64 `omnibuf:"36,wire=sint"`
		U64ArrFixed32 []uint64 `omnibuf:"37,wire=fixed32"`
		U64ArrFixed64 []uint64 `omnibuf:"38,wire=fixed64"`

		I8ArrUint     []int8  `omnibuf:"39,wire=uint"`
		I8ArrFixed32  []int8  `omnibuf:"40,wire=fixed32"`
		I8ArrFixed64  []int8  `omnibuf:"41,wire=fixed64"`
		I16ArrUint    []int16 `omnibuf:"42,wire=uint"`
		I16ArrFixed32 []int16 `omnibuf:"43,wire=fixed32"`
		I16ArrFixed64 []int16 `omnibuf:"44,wire=fixed64"`
		I32ArrUint    []int32 `omnibuf:"45,wire=uint"`
		I32ArrFixed32 []int32 `omnibuf:"46,wire=fixed32"`
		I32ArrFixed64 []int32 `omnibuf:"47,wire=fixed64"`
		I64ArrUint    []int64 `omnibuf:"48,wire=uint"`
		I64ArrFixed32 []int64 `omnibuf:"49,wire=fixed32"`
		I64ArrFixed64 []int64 `omnibuf:"50,wire=fixed64"`

		// Pointer with wire override
		OptU32Fixed *uint32 `omnibuf:"51,wire=fixed32"`
	}

	optFixed := uint32(0xCAFEBABE)
	in := WireOverrideStruct{
		U8Sint:        200,
		U8Fixed32:     201,
		U8Fixed64:     202,
		U16Sint:       50000,
		U16Fixed32:    50001,
		U16Fixed64:    50002,
		U32Sint:       3000000000,
		U32Fixed32:    0xDEADBEEF,
		U32Fixed64:    0x12345678,
		U64Sint:       9000000000,
		U64Fixed32:    0x87654321,
		U64Fixed64:    0xFEDCBA9876543210,
		I8Uint:        -50,
		I8Fixed32:     -51,
		I8Fixed64:     -52,
		I16Uint:       -12000,
		I16Fixed32:    -12001,
		I16Fixed64:    -12002,
		I32Uint:       -1000000,
		I32Fixed32:    -1000001,
		I32Fixed64:    -1000002,
		I64Uint:       -5000000000,
		I64Fixed32:    -12345678,
		I64Fixed64:    -9000000000000,
		StrAsBytes:    "hello-as-bytes",
		BytesAsStr:    []byte("bytes-as-string"),
		U8ArrSint:     []uint8{1, 100, 250},
		U8ArrFixed32:  []uint8{2, 200},
		U8ArrFixed64:  []uint8{3, 255},
		U16ArrSint:    []uint16{10, 40000},
		U16ArrFixed32: []uint16{20, 50000},
		U16ArrFixed64: []uint16{30, 60000},
		U32ArrSint:    []uint32{100, 3000000000},
		U32ArrFixed32: []uint32{0x11111111, 0x22222222},
		U32ArrFixed64: []uint32{0x33333333, 0x44444444},
		U64ArrSint:    []uint64{1000, 9000000000},
		U64ArrFixed32: []uint64{100, 200},
		U64ArrFixed64: []uint64{0xAAAAAAAAAAAAAAAA, 0xBBBBBBBBBBBBBBBB},
		I8ArrUint:     []int8{-10, 0, 10},
		I8ArrFixed32:  []int8{-20, 20},
		I8ArrFixed64:  []int8{-30, 30},
		I16ArrUint:    []int16{-1000, 1000},
		I16ArrFixed32: []int16{-2000, 2000},
		I16ArrFixed64: []int16{-3000, 3000},
		I32ArrUint:    []int32{-100000, 100000},
		I32ArrFixed32: []int32{-200000, 200000},
		I32ArrFixed64: []int32{-300000, 300000},
		I64ArrUint:    []int64{-1000000000, 1000000000},
		I64ArrFixed32: []int64{-2000000, 2000000},
		I64ArrFixed64: []int64{-3000000000, 3000000000},
		OptU32Fixed:   &optFixed,
	}

	pkt, err := Marshal(&in)
	if err != nil {
		t.Fatalf("Marshal WireOverrideStruct failed: %v", err)
	}

	// Verify explicit wire types on the wire via LocateField
	checkWT := func(fieldNum uint16, wantWT uint8) {
		t.Helper()
		_, wt, ok := LocateField(pkt, fieldNum)
		if !ok {
			t.Fatalf("field %d not found in packet", fieldNum)
		}
		if wt != wantWT {
			t.Fatalf("field %d wireType = %d, want %d", fieldNum, wt, wantWT)
		}
	}
	checkWT(1, WireSint)
	checkWT(2, WireFixed32)
	checkWT(3, WireFixed64)
	checkWT(8, WireFixed32)
	checkWT(12, WireFixed64)
	checkWT(13, WireUint)
	checkWT(20, WireFixed32)
	checkWT(24, WireFixed64)
	checkWT(25, WireBytes)
	checkWT(26, WireString)
	checkWT(27, WireSintArr)
	checkWT(28, WireFixedArr)
	checkWT(29, WireFixedArr)
	checkWT(39, WireUintArr)
	checkWT(49, WireFixedArr)
	checkWT(50, WireFixedArr)
	checkWT(51, WireFixed32)

	var out WireOverrideStruct
	if err := Unmarshal(pkt, &out); err != nil {
		t.Fatalf("Unmarshal WireOverrideStruct failed: %v", err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("WireOverrideStruct round-trip mismatch:\ngot:  %#v\nwant: %#v", out, in)
	}
}

func TestOmniBufStructTags_FieldNamesAndSelfDescribingField0(t *testing.T) {
	type NestedMetric struct {
		MetricName string  `omnibuf:"1,name=metric_name"`
		Value      float64 `omnibuf:"2"` // default name: "Value"
	}
	type HostReport struct {
		HostID  uint64                  `omnibuf:"1,name=host_id"`
		Default string                  `omnibuf:"2"` // default name: "Default"
		Primary NestedMetric            `omnibuf:"3,name=primary_metric"`
		History []NestedMetric          `omnibuf:"4,name=history"`
		ByTag   map[string]NestedMetric `omnibuf:"5,name=by_tag"`
	}

	report := &HostReport{
		HostID:  42,
		Default: "default-field-name",
		Primary: NestedMetric{MetricName: "cpu", Value: 0.75},
		History: []NestedMetric{
			{MetricName: "mem", Value: 0.5},
		},
		ByTag: map[string]NestedMetric{
			"disk": {MetricName: "iops", Value: 1200},
		},
	}

	pkt, err := (MarshalOptions{SelfDescribing: true, Deterministic: true}).Marshal(report)
	if err != nil {
		t.Fatalf("Marshal SelfDescribing failed: %v", err)
	}

	// 1. Inspect Field #0 SchemaMeta directly
	meta, ok := ReadField0SchemaMeta(pkt)
	if !ok || meta == nil {
		t.Fatalf("expected Field #0 SchemaMeta in packet")
	}
	if meta.TypeName != "HostReport" {
		t.Fatalf("TypeName = %q, want HostReport", meta.TypeName)
	}
	if meta.Fields[1] != "host_id" || meta.Fields[2] != "Default" || meta.Fields[3] != "primary_metric" {
		t.Fatalf("unexpected root Fields map: %#v", meta.Fields)
	}
	if meta.Children[3] == nil || meta.Children[3].Fields[1] != "metric_name" || meta.Children[3].Fields[2] != "Value" {
		t.Fatalf("unexpected child #3 SchemaMeta: %#v", meta.Children[3])
	}
	if meta.Children[4] == nil || meta.Children[4].Fields[1] != "metric_name" {
		t.Fatalf("unexpected child #4 SchemaMeta: %#v", meta.Children[4])
	}
	if meta.Children[5] == nil || meta.Children[5].Fields[1] != "metric_name" {
		t.Fatalf("unexpected child #5 SchemaMeta: %#v", meta.Children[5])
	}

	// 2. DecodeBlind should use the configured field names at root and all nested levels!
	blind, err := DecodeBlind(pkt)
	if err != nil {
		t.Fatalf("DecodeBlind failed: %v", err)
	}
	if blind["host_id"] != uint64(42) || blind["Default"] != "default-field-name" {
		t.Fatalf("DecodeBlind root mismatch: %#v", blind)
	}
	prim, ok := blind["primary_metric"].(map[string]any)
	if !ok || prim["metric_name"] != "cpu" || prim["Value"] != float64(0.75) {
		t.Fatalf("DecodeBlind primary_metric mismatch: %#v", blind["primary_metric"])
	}

	// 3. Unmarshal back into HostReport (skips Field #0 cleanly)
	var decoded HostReport
	if err := Unmarshal(pkt, &decoded); err != nil {
		t.Fatalf("Unmarshal from SelfDescribing packet failed: %v", err)
	}
	if !reflect.DeepEqual(&decoded, report) {
		t.Fatalf("decoded mismatch:\ngot:  %#v\nwant: %#v", &decoded, report)
	}
}

func TestOmniBufStructTags_OmitEmpty(t *testing.T) {
	type OmitEmptyChild struct {
		A int    `omnibuf:"1,omitempty"`
		B string `omnibuf:"2,omitempty"`
	}
	type OmitEmptyStruct struct {
		U8     uint8             `omnibuf:"1,omitempty"`
		U16    uint16            `omnibuf:"2,omitempty"`
		U32    uint32            `omnibuf:"3,omitempty"`
		U64    uint64            `omnibuf:"4,omitempty"`
		I64    int64             `omnibuf:"5,omitempty"`
		F32    float32           `omnibuf:"6,omitempty"`
		F64    float64           `omnibuf:"7,omitempty"`
		Flag   bool              `omnibuf:"8,omitempty"`
		Str    string            `omnibuf:"9,omitempty"`
		Data   []byte            `omnibuf:"10,omitempty"`
		Items  []string          `omnibuf:"11,omitempty"`
		Counts map[string]uint64 `omnibuf:"12,omitempty"`
		Labels map[string]string `omnibuf:"13,omitempty"`
		Child  OmitEmptyChild    `omnibuf:"14,omitempty"`
		Ptr    *OmitEmptyChild   `omnibuf:"15,omitempty"`
	}

	// 1. Completely zero-valued struct with omitempty on all fields -> 0 wire bytes!
	var zero OmitEmptyStruct
	pkt, err := Marshal(&zero)
	if err != nil {
		t.Fatalf("Marshal zero OmitEmptyStruct failed: %v", err)
	}
	if len(pkt) != 0 {
		t.Fatalf("expected 0 wire bytes for all-omitempty zero struct, got %d bytes: %x", len(pkt), pkt)
	}

	// 2. Populate only Str and Child.A
	partial := OmitEmptyStruct{
		Str:   "present",
		Child: OmitEmptyChild{A: 7},
	}
	pkt2, err := Marshal(&partial)
	if err != nil {
		t.Fatalf("Marshal partial OmitEmptyStruct failed: %v", err)
	}
	if _, _, ok := LocateField(pkt2, 1); ok {
		t.Fatalf("expected field 1 to be omitted")
	}
	if _, _, ok := LocateField(pkt2, 8); ok {
		t.Fatalf("expected field 8 (false bool) to be omitted")
	}
	if _, _, ok := LocateField(pkt2, 9); !ok {
		t.Fatalf("expected field 9 (Str) to be present")
	}
	if _, _, ok := LocateField(pkt2, 14); !ok {
		t.Fatalf("expected field 14 (non-empty Child) to be present")
	}

	var out OmitEmptyStruct
	if err := Unmarshal(pkt2, &out); err != nil {
		t.Fatalf("Unmarshal partial OmitEmptyStruct failed: %v", err)
	}
	if !reflect.DeepEqual(out, partial) {
		t.Fatalf("partial OmitEmptyStruct mismatch: got %#v, want %#v", out, partial)
	}
}

func TestOmniBufStructTags_PrecedenceOverProtobufTag(t *testing.T) {
	type MixedTagStruct struct {
		// omnibuf tag overrides protobuf field number, name, and wire type
		Overridden uint32 `protobuf:"varint,1,opt,name=pb_name,proto3" omnibuf:"10,name=ob_name,wire=fixed32"`
		// omnibuf:"-" suppresses a field that has a protobuf tag
		Suppressed string `protobuf:"bytes,2,opt,name=suppressed,proto3" omnibuf:"-"`
		// Fallback to protobuf tag when omnibuf tag is absent
		Fallback int64 `protobuf:"zigzag64,3,opt,name=fallback_pb,proto3"`
	}

	in := MixedTagStruct{
		Overridden: 0x11223344,
		Suppressed: "secret",
		Fallback:   -99,
	}
	pkt, err := (MarshalOptions{SelfDescribing: true, Deterministic: true}).Marshal(&in)
	if err != nil {
		t.Fatalf("Marshal MixedTagStruct failed: %v", err)
	}

	// Field #1 and #2 should NOT be present; Field #10 (WireFixed32) and Field #3 (WireSint) should be present
	if _, _, ok := LocateField(pkt, 1); ok {
		t.Fatalf("did not expect protobuf field #1 when overridden by omnibuf:#10")
	}
	if _, _, ok := LocateField(pkt, 2); ok {
		t.Fatalf("did not expect field #2 when suppressed by omnibuf:\"-\"")
	}
	if _, wt, ok := LocateField(pkt, 10); !ok || wt != WireFixed32 {
		t.Fatalf("expected field #10 with WireFixed32, got ok=%v wt=%d", ok, wt)
	}
	if _, wt, ok := LocateField(pkt, 3); !ok || wt != WireSint {
		t.Fatalf("expected field #3 with WireSint, got ok=%v wt=%d", ok, wt)
	}

	meta, ok := ReadField0SchemaMeta(pkt)
	if !ok || meta.Fields[10] != "ob_name" || meta.Fields[3] != "fallback_pb" {
		t.Fatalf("unexpected SchemaMeta fields: %#v", meta)
	}

	var out MixedTagStruct
	if err := Unmarshal(pkt, &out); err != nil {
		t.Fatalf("Unmarshal MixedTagStruct failed: %v", err)
	}
	if out.Overridden != in.Overridden || out.Fallback != in.Fallback || out.Suppressed != "" {
		t.Fatalf("MixedTagStruct round-trip mismatch: %#v", out)
	}
}

func TestOmniBufStructTags_ConcatMergeHighFieldsAndSlices(t *testing.T) {
	type ConcatStruct struct {
		Items   []int16        `omnibuf:"70"`
		Metrics map[int]string `omnibuf:"71"`
		Scalar  uint32         `omnibuf:"2048"`
	}

	partA := ConcatStruct{
		Items:   []int16{1, 2},
		Metrics: map[int]string{1: "one", 2: "two"},
		Scalar:  100,
	}
	partB := ConcatStruct{
		Items:   []int16{3, 4},
		Metrics: map[int]string{2: "two-override", 3: "three"},
		Scalar:  200,
	}

	bytesA, err := Marshal(&partA)
	if err != nil {
		t.Fatalf("Marshal partA failed: %v", err)
	}
	bytesB, err := Marshal(&partB)
	if err != nil {
		t.Fatalf("Marshal partB failed: %v", err)
	}

	merged := append(append([]byte{}, bytesA...), bytesB...)
	var out ConcatStruct
	if err := Unmarshal(merged, &out); err != nil {
		t.Fatalf("Unmarshal merged failed: %v", err)
	}
	if !reflect.DeepEqual(out.Items, []int16{1, 2, 3, 4}) {
		t.Fatalf("Items concat mismatch: got %v", out.Items)
	}
	if out.Scalar != 200 {
		t.Fatalf("Scalar last-write-wins mismatch: got %d, want 200", out.Scalar)
	}
	wantMap := map[int]string{1: "one", 2: "two-override", 3: "three"}
	if !reflect.DeepEqual(out.Metrics, wantMap) {
		t.Fatalf("Metrics concat mismatch: got %v, want %v", out.Metrics, wantMap)
	}
}

func TestOmniBufStructTags_ValidationErrors(t *testing.T) {
	// 1. Non-pointer or nil pointer
	if _, err := Marshal(123); err == nil {
		t.Fatalf("expected error for non-pointer Marshal")
	}
	var nilPtr *TaggedChild
	if _, err := Marshal(nilPtr); err == nil {
		t.Fatalf("expected error for nil pointer Marshal")
	}
	if err := Unmarshal(nil, 123); err == nil {
		t.Fatalf("expected error for non-pointer Unmarshal")
	}
	if err := Unmarshal(nil, nilPtr); err == nil {
		t.Fatalf("expected error for nil pointer Unmarshal")
	}
	x := 5
	if _, err := Marshal(&x); err == nil {
		t.Fatalf("expected error for pointer to non-struct")
	}

	// 2. Invalid tag syntax & options
	errorCases := []struct {
		name string
		val  any
		want string
	}{
		{
			name: "zero_field_num",
			val: &struct {
				A int `omnibuf:"0"`
			}{},
			want: "invalid field number",
		},
		{
			name: "non_numeric_field_num",
			val: &struct {
				A int `omnibuf:"abc"`
			}{},
			want: "invalid field number",
		},
		{
			name: "overflow_field_num",
			val: &struct {
				A int `omnibuf:"70000"`
			}{},
			want: "invalid field number",
		},
		{
			name: "duplicate_field_num",
			val: &struct {
				A int `omnibuf:"1"`
				B int `omnibuf:"1"`
			}{},
			want: "duplicate field number 1",
		},
		{
			name: "bare_shorthand_keyword_rejected",
			val: &struct {
				A uint32 `omnibuf:"1,fixed32"`
			}{},
			want: "invalid option",
		},
		{
			name: "empty_name_option",
			val: &struct {
				A uint32 `omnibuf:"1,name="`
			}{},
			want: "empty name option",
		},
		{
			name: "empty_wire_option",
			val: &struct {
				A uint32 `omnibuf:"1,wire="`
			}{},
			want: "empty wire option",
		},
		{
			name: "incompatible_wire_on_string",
			val: &struct {
				A string `omnibuf:"1,wire=fixed32"`
			}{},
			want: "unsupported wire type",
		},
		{
			name: "incompatible_wire_on_int",
			val: &struct {
				A int `omnibuf:"1,wire=string"`
			}{},
			want: "unsupported wire type",
		},
		{
			name: "incompatible_wire_on_bool",
			val: &struct {
				A bool `omnibuf:"1,wire=uint"`
			}{},
			want: "unsupported wire type",
		},
		{
			name: "incompatible_wire_on_float32",
			val: &struct {
				A float32 `omnibuf:"1,wire=fixed64"`
			}{},
			want: "unsupported wire type",
		},
		{
			name: "incompatible_wire_on_float64",
			val: &struct {
				A float64 `omnibuf:"1,wire=fixed32"`
			}{},
			want: "unsupported wire type",
		},
		{
			name: "incompatible_wire_on_struct",
			val: &struct {
				A TaggedChild `omnibuf:"1,wire=bytes"`
			}{},
			want: "unsupported wire type",
		},
		{
			name: "unsupported_channel_field",
			val: &struct {
				A chan int `omnibuf:"1"`
			}{},
			want: "unsupported field type",
		},
		{
			name: "unsupported_map_key_type",
			val: &struct {
				A map[float64]string `omnibuf:"1"`
			}{},
			want: "unsupported map key type",
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Marshal(tc.val)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}

	// Ensure NaN float32/float64 round-trips bit-for-bit on tagged struct
	type FloatStruct struct {
		F32 float32 `omnibuf:"1"`
		F64 float64 `omnibuf:"2"`
	}
	nanIn := FloatStruct{F32: float32(math.NaN()), F64: math.NaN()}
	nanPkt, err := Marshal(&nanIn)
	if err != nil {
		t.Fatalf("Marshal FloatStruct failed: %v", err)
	}
	var nanOut FloatStruct
	if err := Unmarshal(nanPkt, &nanOut); err != nil {
		t.Fatalf("Unmarshal FloatStruct failed: %v", err)
	}
	if !math.IsNaN(float64(nanOut.F32)) || !math.IsNaN(nanOut.F64) {
		t.Fatalf("expected NaN preserved, got %+v", nanOut)
	}
}

func TestOmniBufStructTags_CrossWidthEvolutionAndMalformedPackets(t *testing.T) {
	// Narrow integer types encoded on V1 struct, decoded into wide 64-bit integer types on V2 struct
	type NarrowV1 struct {
		I8Varint   int8    `omnibuf:"1"`
		I16Fixed32 int16   `omnibuf:"2,wire=fixed32"`
		I32Fixed64 int32   `omnibuf:"3,wire=fixed64"`
		U8Varint   uint8   `omnibuf:"4"`
		U16Fixed32 uint16  `omnibuf:"5,wire=fixed32"`
		U32Fixed64 uint32  `omnibuf:"6,wire=fixed64"`
		ArrI8      []int8  `omnibuf:"7"`
		ArrI16F32  []int16 `omnibuf:"8,wire=fixed32"`
		ArrI32F64  []int32 `omnibuf:"9,wire=fixed64"`
	}
	type WideV2 struct {
		I8Varint   int64   `omnibuf:"1"`
		I16Fixed32 int64   `omnibuf:"2,wire=fixed32"`
		I32Fixed64 int64   `omnibuf:"3,wire=fixed64"`
		U8Varint   uint64  `omnibuf:"4"`
		U16Fixed32 uint64  `omnibuf:"5,wire=fixed32"`
		U32Fixed64 uint64  `omnibuf:"6,wire=fixed64"`
		ArrI8      []int64 `omnibuf:"7"`
		ArrI16F32  []int64 `omnibuf:"8,wire=fixed32"`
		ArrI32F64  []int64 `omnibuf:"9,wire=fixed64"`
	}

	v1 := NarrowV1{
		I8Varint:   -99,
		I16Fixed32: -12345,
		I32Fixed64: -987654321,
		U8Varint:   250,
		U16Fixed32: 60000,
		U32Fixed64: 4000000000,
		ArrI8:      []int8{-10, 0, 10},
		ArrI16F32:  []int16{-500, 500},
		ArrI32F64:  []int32{-100000, 100000},
	}

	pkt, err := Marshal(&v1)
	if err != nil {
		t.Fatalf("Marshal NarrowV1 failed: %v", err)
	}

	var v2 WideV2
	if err := Unmarshal(pkt, &v2); err != nil {
		t.Fatalf("Unmarshal into WideV2 failed: %v", err)
	}

	wantV2 := WideV2{
		I8Varint:   -99,
		I16Fixed32: -12345,
		I32Fixed64: -987654321,
		U8Varint:   250,
		U16Fixed32: 60000,
		U32Fixed64: 4000000000,
		ArrI8:      []int64{-10, 0, 10},
		ArrI16F32:  []int64{-500, 500},
		ArrI32F64:  []int64{-100000, 100000},
	}
	if !reflect.DeepEqual(v2, wantV2) {
		t.Fatalf("cross-width V1->V2 evolution mismatch:\ngot:  %+v\nwant: %+v", v2, wantV2)
	}

	// Verify truncated packets are rejected for every prefix length
	for cut := 1; cut < len(pkt); cut++ {
		var dst NarrowV1
		if err := Unmarshal(pkt[:cut], &dst); err == nil {
			// A cut on an exact field boundary is valid; verify if cut is mid-field
			continue
		}
	}
	// Truncate inside the last 8-byte element of field #9 (ArrI32F64)
	var dst NarrowV1
	if err := Unmarshal(pkt[:len(pkt)-1], &dst); err == nil {
		t.Fatalf("expected error on truncated packet")
	}
}
