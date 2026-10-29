package omnibuf_test

import (
	"bytes"
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/dynamicpb"

	. "github.com/sudorandom/omnibuf"
	"github.com/sudorandom/omnibuf/internal/pb"
	. "github.com/sudorandom/omnibuf/wire"
)

// ============================================================================
// 6. UTF-8 validation (spec §10: strings MUST be valid UTF-8)
// ============================================================================

var invalidUTF8Samples = map[string]string{
	"lone_ff":              "\xff",
	"lone_fe":              "\xfe",
	"stray_continuation":   "a\x80b",
	"overlong_nul":         "\xc0\x80",
	"overlong_slash":       "\xe0\x80\xaf",
	"surrogate_high":       "\xed\xa0\x80",
	"surrogate_low":        "\xed\xbf\xbf",
	"above_max_codepoint":  "\xf4\x90\x80\x80",
	"truncated_2byte":      "\xc3",
	"truncated_3byte":      "\xe2\x82",
	"truncated_4byte":      "\xf0\x9f\x98",
	"valid_then_truncated": "hello\xe2\x82",
	"five_byte_form":       "\xf8\x88\x80\x80\x80",
}

var validUTF8Boundaries = []string{
	"", "\x00", "\x7f", "\u0080", "\u07ff", "\u0800", "\ufffd", "\uffff",
	"\U00010000", "\U0010ffff", "🚀👩‍👩‍👧‍👦", "a\x00b", strings.Repeat("é", 200),
}

func TestEdge_InvalidUTF8RejectedOnDecodeAllTiers(t *testing.T) {
	for name, bad := range invalidUTF8Samples {
		builders := map[string]func(e *Encoder){
			"f6_string":   func(e *Encoder) { e.WriteString(6, bad) },
			"f20_string":  func(e *Encoder) { e.WriteString(20, bad) },
			"f9_elem":     func(e *Encoder) { e.WriteStringArr(9, []string{"ok", bad}) },
			"f12_map_key": func(e *Encoder) { e.WriteMapStringUint(12, map[string]uint64{bad: 1}) },
			"f8_nested": func(e *Encoder) {
				e.BeginSubMsg(8)
				e.WriteString(2, bad)
				e.EndSubMsg()
			},
			"f11_elem_nested": func(e *Encoder) {
				e.BeginSubMsgArr(11)
				e.BeginSubMsgElem()
				e.WriteString(2, "fine")
				e.EndSubMsgElem()
				e.BeginSubMsgElem()
				e.WriteString(2, bad)
				e.EndSubMsgElem()
				e.EndSubMsgArr()
			},
			"field0_type_name": func(e *Encoder) { e.WriteRaw((&SchemaMeta{TypeName: bad}).AppendField0(nil, true)) },
			"field0_field_name": func(e *Encoder) {
				e.WriteRaw((&SchemaMeta{Fields: map[uint32]string{1: bad}}).AppendField0(nil, true))
			},
			"field0_child_name": func(e *Encoder) {
				e.WriteRaw((&SchemaMeta{Children: map[uint32]*SchemaMeta{8: {Fields: map[uint32]string{2: bad}}}}).AppendField0(nil, true))
			},
		}
		for pos, build := range builders {
			t.Run(name+"/"+pos, func(t *testing.T) {
				e := NewEncoder(64)
				e.WriteUint(1, 1)
				build(e)
				requireAllTiersReject(t, append([]byte(nil), e.Finish()...))
			})
		}
	}
}

func TestEdge_InvalidUTF8RejectedByHeaderHopReaders(t *testing.T) {
	bad := "\xff\xfe"
	e := NewEncoder(64)
	e.WriteString(6, bad)
	e.WriteStringArr(9, []string{bad})
	e.WriteMapStringUint(12, map[string]uint64{bad: 1, "ok": 2})
	e.BeginSubMsg(8)
	e.WriteString(2, bad)
	e.EndSubMsg()
	raw := e.Finish()
	if _, ok := ReadString(raw, 6); ok {
		t.Error("ReadString accepted invalid UTF-8")
	}
	if _, ok := ReadStringZeroCopy(raw, 6); ok {
		t.Error("ReadStringZeroCopy accepted invalid UTF-8")
	}
	if _, ok := ReadStringBytes(raw, 6); ok {
		t.Error("ReadStringBytes accepted invalid UTF-8")
	}
	if _, ok := ReadStringArr(raw, 9); ok {
		t.Error("ReadStringArr accepted invalid UTF-8")
	}
	if _, ok := ReadMapStringUint(raw, 12, false); ok {
		t.Error("ReadMapStringUint accepted invalid UTF-8 key")
	}
	if _, ok := LookupMapStringUint(raw, 12, "ok"); ok {
		t.Error("LookupMapStringUint accepted map with invalid UTF-8 key")
	}
	if _, ok := ReadNestedString(raw, 8, 2); ok {
		t.Error("ReadNestedString accepted invalid UTF-8")
	}
}

// Only DecodeBlind validates the contents of unknown fields (it has no schema
// and decodes everything); schema-aware decoders skip unknown fields by length.
func TestEdge_InvalidUTF8InUnknownFieldsRejectedByBlind(t *testing.T) {
	cases := map[string][]byte{}
	{
		e := NewEncoder(32)
		e.WriteString(99, "\xff")
		cases["unknown_string"] = append([]byte(nil), e.Finish()...)
	}
	{
		e := NewEncoder(32)
		e.WriteStringArr(99, []string{"\xff"})
		cases["unknown_string_array"] = append([]byte(nil), e.Finish()...)
	}
	{
		body := []byte{WireUint, WireString, 0x03, 0x03, 0x03, 0xff}
		cases["unknown_map_string_value"] = concat(appendTag(nil, 99, WireMap), AppendVarint(nil, uint64(len(body))), body)
	}
	{
		body := []byte{WireString, WireSint, 0x03, 0x03, 0xff, 0x03}
		cases["unknown_map_string_key"] = concat(appendTag(nil, 99, WireMap), AppendVarint(nil, uint64(len(body))), body)
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeBlind(raw); err == nil {
				t.Fatalf("DecodeBlind accepted invalid UTF-8 in %s", name)
			}
			requireAllTiersAgree(t, raw) // schema tiers skip without inspecting
		})
	}
}

func TestEdge_ValidUTF8BoundariesRoundTripAllTiers(t *testing.T) {
	for i, s := range validUTF8Boundaries {
		msg := &TestAllTypes{
			F6String:       s,
			F9StringArr:    []string{s, "", s},
			F12MapStrUint:  map[string]uint64{s: uint64(i)},
			F8SubMsg:       &AllTypesSubMsg{Label: s},
			F11SubMsgArr:   []AllTypesSubMsg{{Label: s}},
			F20OverflowStr: s,
		}
		raw, err := MarshalDeterministic(msg)
		if err != nil {
			t.Fatalf("Marshal(%q): %v", s, err)
		}
		if got := requireAllTiersAgree(t, raw); !bytes.Equal(got, raw) {
			t.Fatalf("round-trip of %q changed bytes", s)
		}
		if _, err := DecodeBlind(raw); err != nil {
			t.Fatalf("DecodeBlind(%q): %v", s, err)
		}
		if got, ok := ReadString(raw, 6); !ok || got != s {
			t.Fatalf("ReadString(%q) = (%q, %v)", s, got, ok)
		}
	}
	// Arbitrary binary in WireBytes is never UTF-8 checked.
	raw, err := MarshalDeterministic(&TestAllTypes{F7Bytes: []byte{0xff, 0xfe, 0xc0, 0x80}})
	if err != nil {
		t.Fatal(err)
	}
	requireAllTiersAgree(t, raw)
	if _, err := DecodeBlind(raw); err != nil {
		t.Fatalf("DecodeBlind rejected binary WireBytes: %v", err)
	}
}

func TestEdge_InvalidUTF8RejectedOnMarshal(t *testing.T) {
	bad := "\xff"
	structCases := map[string]*TestAllTypes{
		"f6":         {F6String: bad},
		"f20":        {F20OverflowStr: bad},
		"f9":         {F9StringArr: []string{"ok", bad}},
		"f12_key":    {F12MapStrUint: map[string]uint64{bad: 1}},
		"f8_nested":  {F8SubMsg: &AllTypesSubMsg{Label: bad}},
		"f11_nested": {F11SubMsgArr: []AllTypesSubMsg{{}, {Label: bad}}},
	}
	for name, m := range structCases {
		if _, err := Marshal(m); err == nil {
			t.Errorf("table Marshal(struct %s) accepted invalid UTF-8", name)
		}
		if _, err := MarshalDeterministic(m); err == nil {
			t.Errorf("table MarshalDeterministic(struct %s) accepted invalid UTF-8", name)
		}
		if _, err := (MarshalOptions{SelfDescribing: true}).Marshal(m); err == nil {
			t.Errorf("self-describing Marshal(struct %s) accepted invalid UTF-8", name)
		}
	}
	pbCases := map[string]*pb.TestAllTypes{
		"f6":         {F6String: bad},
		"f9":         {F9StringArr: []string{bad}},
		"f12_key":    {F12MapStrUint: map[string]uint64{bad: 1}},
		"f8_nested":  {F8SubMsg: &pb.AllTypesSubMsg{Label: bad}},
		"f11_nested": {F11SubMsgArr: []*pb.AllTypesSubMsg{{Label: bad}}},
	}
	for name, m := range pbCases {
		if _, err := Marshal(m); err == nil {
			t.Errorf("table Marshal(pb %s) accepted invalid UTF-8", name)
		}
		dyn := dynamicpb.NewMessage(testAllTypesDesc)
		proto.Merge(dyn, m)
		if _, err := Marshal(dyn); err == nil {
			t.Errorf("reflect Marshal(dynamicpb %s) accepted invalid UTF-8", name)
		}
	}
}

type utf8OverrideMsg struct {
	BinStr    string   `omnibuf:"1,wire=bytes"`
	TextBytes []byte   `omnibuf:"2,wire=string"`
	BinStrs   []string `omnibuf:"3,wire=bytes"`
	TextBlobs [][]byte `omnibuf:"4,wire=string"`
}

func TestEdge_WireOverridesControlUTF8Validation(t *testing.T) {
	// wire=bytes on strings carries arbitrary binary (no UTF-8 requirement).
	in := utf8OverrideMsg{BinStr: "\xff\x00", BinStrs: []string{"\xfe", "ok"}}
	raw, err := Marshal(&in)
	if err != nil {
		t.Fatalf("Marshal with wire=bytes strings: %v", err)
	}
	// []string with wire=bytes must use WireBytes as the array element type.
	body, wt, ok := LocateField(raw, 3)
	if !ok || wt != WireArray {
		t.Fatalf("field 3 not an array: %v %v", wt, ok)
	}
	if _, ln := DecodeVarint(body); body[ln] != WireBytes {
		t.Fatalf("[]string wire=bytes encoded element type %d, want WireBytes", body[ln])
	}
	var out utf8OverrideMsg
	if err := Unmarshal(raw, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.BinStr != in.BinStr || !reflect.DeepEqual(out.BinStrs, in.BinStrs) {
		t.Fatalf("binary strings did not round-trip: %+v", out)
	}
	if _, err := DecodeBlind(raw); err != nil {
		t.Fatalf("DecodeBlind: %v", err)
	}

	// wire=string on []byte / [][]byte requires valid UTF-8 both ways.
	if _, err := Marshal(&utf8OverrideMsg{TextBytes: []byte{0xff}}); err == nil {
		t.Error("Marshal accepted invalid UTF-8 in []byte wire=string")
	}
	if _, err := Marshal(&utf8OverrideMsg{TextBlobs: [][]byte{[]byte("ok"), {0xff}}}); err == nil {
		t.Error("Marshal accepted invalid UTF-8 in [][]byte wire=string")
	}
	good := utf8OverrideMsg{TextBytes: []byte("héllo"), TextBlobs: [][]byte{[]byte("a"), []byte("🚀")}}
	raw, err = Marshal(&good)
	if err != nil {
		t.Fatal(err)
	}
	body, _, _ = LocateField(raw, 4)
	if _, ln := DecodeVarint(body); body[ln] != WireString {
		t.Fatalf("[][]byte wire=string encoded element type %d, want WireString", body[ln])
	}
	var back utf8OverrideMsg
	if err := Unmarshal(raw, &back); err != nil || !bytes.Equal(back.TextBytes, good.TextBytes) || !reflect.DeepEqual(back.TextBlobs, good.TextBlobs) {
		t.Fatalf("wire=string bytes round-trip: %+v, %v", back, err)
	}
	e := NewEncoder(16)
	e.WriteString(2, "\xff")
	if err := Unmarshal(e.Finish(), &back); err == nil {
		t.Error("Unmarshal accepted invalid UTF-8 into []byte wire=string")
	}
}

// ============================================================================
// 7. Reuse / state bleed: decoding replaces the destination in every tier
// ============================================================================

func fullTestAllTypes() *TestAllTypes {
	return &TestAllTypes{
		F1Uint32: 1, F2Uint64: 2, F3Sint32: -3, F4Sint64: -4, F5Bool: true,
		F6String: "six", F7Bytes: []byte{7}, F8SubMsg: &AllTypesSubMsg{ID: 8, Label: "eight", Flag: true, Score: -8},
		F9StringArr: []string{"a", "b", "c"}, F10Uint32Arr: []uint32{1, 2, 3},
		F11SubMsgArr: []AllTypesSubMsg{
			{ID: 1, Label: "x", Flag: true, Score: 1},
			{ID: 2, Label: "y", Flag: true, Score: 2},
			{ID: 3, Label: "z", Flag: true, Score: 3},
		},
		F12MapStrUint: map[string]uint64{"stale": 1, "keep?": 2},
		F13Float32:    13, F14Float64: 14, F15Fixed32: 15, F16Fixed64: 16, F20OverflowStr: "twenty",
	}
}

func TestEdge_ReuseDoesNotBleedStateAcrossDecodes(t *testing.T) {
	fullWire, err := MarshalDeterministic(fullTestAllTypes())
	if err != nil {
		t.Fatal(err)
	}
	// Second packet: sparse, with a partial sub-message and fewer, partial array elements.
	e := NewEncoder(64)
	e.WriteUint(1, 100)
	e.BeginSubMsg(8)
	e.WriteUint(1, 80)
	e.EndSubMsg()
	e.BeginSubMsgArr(11)
	for _, id := range []uint64{10, 20} {
		e.BeginSubMsgElem()
		e.WriteUint(1, id)
		e.EndSubMsgElem()
	}
	e.EndSubMsgArr()
	e.WriteMapStringUintDeterministic(12, map[string]uint64{"new": 9})
	sparse := append([]byte(nil), e.Finish()...)
	want := &TestAllTypes{
		F1Uint32: 100, F8SubMsg: &AllTypesSubMsg{ID: 80},
		F11SubMsgArr:  []AllTypesSubMsg{{ID: 10}, {ID: 20}},
		F12MapStrUint: map[string]uint64{"new": 9},
	}
	wantWire, _ := MarshalDeterministic(want)

	t.Run("table/struct", func(t *testing.T) {
		for _, zc := range []bool{false, true} {
			var dst TestAllTypes
			dec := Unmarshal
			if zc {
				dec = UnmarshalZeroCopy
			}
			if err := dec(fullWire, &dst); err != nil {
				t.Fatal(err)
			}
			if err := dec(sparse, &dst); err != nil {
				t.Fatal(err)
			}
			got, _ := MarshalDeterministic(&dst)
			if !bytes.Equal(got, wantWire) {
				t.Fatalf("zc=%v stale state after reuse: %+v", zc, dst)
			}
			// Empty packet clears everything.
			if err := dec(nil, &dst); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(normalizeAllTypes(&dst), normalizeAllTypes(&TestAllTypes{})) {
				t.Fatalf("zc=%v empty packet left state behind: %+v", zc, dst)
			}
		}
	})

	pbTiers := map[string]func(raw []byte, m proto.Message) error{
		"table/pb": func(raw []byte, m proto.Message) error { return Unmarshal(raw, m) },
		"plugin":   func(raw []byte, m proto.Message) error { return m.(*pb.TestAllTypes).UnmarshalOmniBuf(raw, false) },
		"plugin-zc": func(raw []byte, m proto.Message) error {
			return m.(*pb.TestAllTypes).UnmarshalOmniBuf(raw, true)
		},
		"reflect/dynamic": nil,
	}
	wantPB := &pb.TestAllTypes{}
	if err := wantPB.UnmarshalOmniBuf(wantWire, false); err != nil {
		t.Fatal(err)
	}
	for name, dec := range pbTiers {
		t.Run(name, func(t *testing.T) {
			var dst proto.Message = &pb.TestAllTypes{}
			if dec == nil {
				dst = dynamicpb.NewMessage(testAllTypesDesc)
				dec = func(raw []byte, m proto.Message) error { return Unmarshal(raw, m) }
			}
			if err := dec(fullWire, dst); err != nil {
				t.Fatal(err)
			}
			if err := dec(sparse, dst); err != nil {
				t.Fatal(err)
			}
			if !proto.Equal(dst, wantPB) && !protoEqualAcrossTypes(t, dst, wantPB) {
				t.Fatalf("stale state after reuse:\n got  %v\n want %v", dst, wantPB)
			}
			if err := dec(nil, dst); err != nil {
				t.Fatal(err)
			}
			if !protoEqualAcrossTypes(t, dst, &pb.TestAllTypes{}) {
				t.Fatalf("empty packet left state behind: %v", dst)
			}
		})
	}
}

func protoEqualAcrossTypes(t *testing.T, a, b proto.Message) bool {
	t.Helper()
	ab, err := proto.MarshalOptions{Deterministic: true}.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	bb, err := proto.MarshalOptions{Deterministic: true}.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Equal(ab, bb)
}

func normalizeAllTypes(m *TestAllTypes) []byte {
	b, _ := MarshalDeterministic(m)
	return b
}

// Struct-valued fields and value-slice element reuse (PeerUpdate.Location is a
// struct value; Neighbors is []Neighbor whose backing array gets reused).
func TestEdge_ReuseClearsStructValuesAndReusedSliceElements(t *testing.T) {
	full := SamplePeerUpdate()
	fullWire, err := Marshal(&full)
	if err != nil {
		t.Fatal(err)
	}
	sparse := PeerUpdate{ASN: 9, Neighbors: []Neighbor{{PeerASN: 1}}}
	sparseWire, err := Marshal(&sparse)
	if err != nil {
		t.Fatal(err)
	}
	var dst PeerUpdate
	if err := Unmarshal(fullWire, &dst); err != nil {
		t.Fatal(err)
	}
	tagsCap := cap(dst.Tags)
	if err := Unmarshal(sparseWire, &dst); err != nil {
		t.Fatal(err)
	}
	if dst.Location != (Location{}) {
		t.Fatalf("struct-value Location kept stale fields: %+v", dst.Location)
	}
	if len(dst.Neighbors) != 1 || dst.Neighbors[0] != (Neighbor{PeerASN: 1}) {
		t.Fatalf("reused Neighbors element kept stale fields: %+v", dst.Neighbors)
	}
	if dst.RouterID != "" || dst.Active || dst.Sequence != 0 || len(dst.Tags) != 0 || len(dst.Prefixes) != 0 {
		t.Fatalf("absent scalars/slices kept stale values: %+v", dst)
	}
	if cap(dst.Tags) != tagsCap {
		t.Fatalf("absent slice lost its capacity (%d -> %d); reuse should keep it", tagsCap, cap(dst.Tags))
	}
	// Generated PeerUpdate behaves identically.
	var pbDst pb.PeerUpdate
	if err := pbDst.UnmarshalOmniBuf(fullWire, false); err != nil {
		t.Fatal(err)
	}
	if err := pbDst.UnmarshalOmniBuf(sparseWire, false); err != nil {
		t.Fatal(err)
	}
	if pbDst.Location.GetCity() != "" || len(pbDst.Neighbors) != 1 || pbDst.Neighbors[0].Router != "" {
		t.Fatalf("plugin reuse kept stale state: %v", &pbDst)
	}
}

// Recursive pointer slices ([]*TreeNode) reuse previously allocated elements.
func TestEdge_ReuseClearsRecursivePointerSliceElements(t *testing.T) {
	a := &TreeNode{Branches: []*TreeNode{
		{ID: 1, Name: "one", Labels: map[string]uint64{"x": 1}, Primary: &TreeNode{ID: 11}},
		{ID: 2, Name: "two", Children: []TreeNode{{ID: 21}}},
	}}
	b := &TreeNode{Branches: []*TreeNode{{ID: 5}}}
	aw, _ := Marshal(a)
	bw, _ := Marshal(b)
	var dst TreeNode
	if err := Unmarshal(aw, &dst); err != nil {
		t.Fatal(err)
	}
	if err := Unmarshal(bw, &dst); err != nil {
		t.Fatal(err)
	}
	got := dst.Branches[0]
	if len(dst.Branches) != 1 || got.ID != 5 || got.Name != "" || len(got.Labels) != 0 || got.Primary != nil || len(got.Children) != 0 {
		t.Fatalf("reused *TreeNode element kept stale state: %+v", got)
	}
}

// Merging within a single packet (repeated sub-message occurrences) must
// still merge, while a fresh decode must still replace.
func TestEdge_MergeWithinPacketVersusReplaceAcrossDecodes(t *testing.T) {
	e := NewEncoder(64)
	e.BeginSubMsg(8)
	e.WriteString(2, "from-first")
	e.EndSubMsg()
	e.BeginSubMsg(8)
	e.WriteUint(1, 42)
	e.EndSubMsg()
	raw := append([]byte(nil), e.Finish()...)
	got := requireAllTiersAgree(t, raw)
	want, _ := MarshalDeterministic(&TestAllTypes{F8SubMsg: &AllTypesSubMsg{ID: 42, Label: "from-first"}})
	if !bytes.Equal(got, want) {
		t.Fatalf("repeated sub-message did not merge: %x", got)
	}
}

// Regression: generated safe-mode bytes decode used append(x.F[:0], ...),
// which wrote into a packet buffer aliased by an earlier zero-copy decode.
func TestEdge_PluginSafeDecodeNeverWritesIntoPreviousZeroCopyBuffer(t *testing.T) {
	pkt1 := (&pb.TestAllTypes{F7Bytes: []byte("AAAAAAAA"), F6String: "tail"}).MarshalOmniBufAppend(nil)
	snapshot := append([]byte(nil), pkt1...)
	var m pb.TestAllTypes
	if err := m.UnmarshalOmniBuf(pkt1, true); err != nil {
		t.Fatal(err)
	}
	pkt2 := (&pb.TestAllTypes{F7Bytes: []byte("BBBBBBBBBBBB")}).MarshalOmniBufAppend(nil)
	if err := m.UnmarshalOmniBuf(pkt2, false); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pkt1, snapshot) {
		t.Fatalf("safe decode scribbled over previous zero-copy packet:\n now  %x\n was  %x", pkt1, snapshot)
	}
	if string(m.F7Bytes) != "BBBBBBBBBBBB" {
		t.Fatalf("F7Bytes = %q", m.F7Bytes)
	}
}

// ============================================================================
// 8. DecodeBlind: wire-shape consistency and name collisions
// ============================================================================

func TestEdge_BlindRejectsShapeChangesWithinMessage(t *testing.T) {
	enc := func(f func(e *Encoder)) []byte {
		e := NewEncoder(64)
		f(e)
		return append([]byte(nil), e.Finish()...)
	}
	bad := map[string][]byte{
		"uint_then_string":    enc(func(e *Encoder) { e.WriteUint(1, 1); e.WriteString(1, "x") }),
		"submsg_then_uint":    enc(func(e *Encoder) { e.BeginSubMsg(1); e.EndSubMsg(); e.WriteUint(1, 1) }),
		"uint_then_submsg":    enc(func(e *Encoder) { e.WriteUint(1, 1); e.BeginSubMsg(1); e.EndSubMsg() }),
		"uint_then_sint":      enc(func(e *Encoder) { e.WriteUint(1, 1); e.WriteInt(1, 1) }),
		"fixed32_then_float":  enc(func(e *Encoder) { e.WriteFixed32(1, 1); e.WriteFloat32(1, 1) }),
		"bool_then_uint":      enc(func(e *Encoder) { e.WriteBool(1, true); e.WriteUint(1, 1) }),
		"array_uint_then_f64": enc(func(e *Encoder) { e.WriteUint32Arr(1, []uint32{1}); e.WriteFloat64Arr(1, []float64{1}) }),
		"array_uint_then_sint": enc(func(e *Encoder) {
			e.WriteUint32Arr(1, []uint32{1})
			e.WriteSintArr(1, []int64{1})
		}),
		"map_value_type_change": func() []byte {
			b := enc(func(e *Encoder) { e.WriteMapStringUint(1, map[string]uint64{"a": 1}) })
			body := []byte{WireString, WireString, 0x03, 0x03, 'b', 0x03, 'c'}
			return concat(b, appendTag(nil, 1, WireMap), AppendVarint(nil, uint64(len(body))), body)
		}(),
		"map_key_type_change": func() []byte {
			b1 := []byte{WireUint, WireString, 0x03, 0x03, 0x03, 'a'}
			b2 := []byte{WireSint, WireString, 0x03, 0x03, 0x03, 'a'}
			return concat(appendTag(nil, 1, WireMap), AppendVarint(nil, uint64(len(b1))), b1,
				appendTag(nil, 1, WireMap), AppendVarint(nil, uint64(len(b2))), b2)
		}(),
		"array_then_map": enc(func(e *Encoder) {
			e.WriteUint32Arr(1, []uint32{1})
			e.WriteMapStringUint(1, map[string]uint64{"a": 1})
		}),
		"name_collision_synthetic": enc(func(e *Encoder) {
			e.WriteRaw((&SchemaMeta{Fields: map[uint32]string{2: "field_1"}}).AppendField0(nil, true))
			e.WriteUint(1, 1)
			e.WriteUint(2, 2)
		}),
		"name_collision_schema": enc(func(e *Encoder) {
			e.WriteRaw((&SchemaMeta{Fields: map[uint32]string{1: "dup", 2: "dup"}}).AppendField0(nil, true))
			e.WriteUint(1, 1)
			e.WriteUint(2, 2)
		}),
		"nested_shape_change": enc(func(e *Encoder) {
			e.BeginSubMsg(3)
			e.WriteUint(1, 1)
			e.EndSubMsg()
			e.BeginSubMsg(3)
			e.WriteString(1, "x")
			e.EndSubMsg()
		}),
	}
	for name, raw := range bad {
		t.Run(name, func(t *testing.T) {
			if m, err := DecodeBlind(raw); err == nil {
				t.Fatalf("DecodeBlind accepted %s: %v", name, m)
			}
		})
	}

	ok := map[string]struct {
		raw  []byte
		want map[string]any
	}{
		"bool_false_then_true": {enc(func(e *Encoder) { e.WriteBool(1, false); e.WriteBool(1, true) }), map[string]any{"field_1": true}},
		"bool_true_then_false": {enc(func(e *Encoder) { e.WriteBool(1, true); e.WriteBool(1, false) }), map[string]any{"field_1": false}},
		"uint_last_wins":       {enc(func(e *Encoder) { e.WriteUint(1, 1); e.WriteUint(1, 2) }), map[string]any{"field_1": uint64(2)}},
		"arrays_concat": {enc(func(e *Encoder) { e.WriteUint32Arr(1, []uint32{1}); e.WriteUint32Arr(1, []uint32{2, 3}) }),
			map[string]any{"field_1": []uint64{1, 2, 3}}},
		"maps_merge_last_key_wins": {enc(func(e *Encoder) {
			e.WriteMapStringUint(1, map[string]uint64{"a": 1, "b": 1})
			e.WriteMapStringUint(1, map[string]uint64{"b": 2})
		}), map[string]any{"field_1": map[string]uint64{"a": 1, "b": 2}}},
		"same_name_same_field_from_two_schemas": {enc(func(e *Encoder) {
			e.WriteRaw((&SchemaMeta{Fields: map[uint32]string{1: "x"}}).AppendField0(nil, true))
			e.WriteUint(1, 1)
			e.WriteRaw((&SchemaMeta{Fields: map[uint32]string{1: "x"}}).AppendField0(nil, true))
			e.WriteUint(1, 2)
		}), map[string]any{"x": uint64(2)}},
	}
	for name, c := range ok {
		t.Run(name, func(t *testing.T) {
			got, err := DecodeBlind(c.raw)
			if err != nil {
				t.Fatalf("DecodeBlind: %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %#v, want %#v", got, c.want)
			}
		})
	}
}

// ============================================================================
// 9. Nesting limit counts messages, arrays AND maps (spec §10)
// ============================================================================

func treeChain(depth int, leaf *TreeNode) *TreeNode {
	n := leaf
	for i := 0; i < depth; i++ {
		n = &TreeNode{ID: uint32(i + 1), Primary: n}
	}
	return n
}

func TestEdge_NestingLimitCountsArraysAndMaps(t *testing.T) {
	// Note: the table encoder emits (empty) Children/Branches/Labels containers
	// for every TreeNode, so a node at depth d always has containers at d+1.
	cases := []struct {
		name   string
		leaf   *TreeNode
		depth  int // depth of leaf message
		accept bool
	}{
		{"leaf_containers_at_32", &TreeNode{ID: 99}, 31, true},
		{"leaf_containers_at_33", &TreeNode{ID: 99}, 32, false},
		{"message_at_33", &TreeNode{ID: 99}, 33, false},
		{"map_at_32", &TreeNode{Labels: map[string]uint64{"a": 1}}, 31, true},
		{"map_at_33", &TreeNode{Labels: map[string]uint64{"a": 1}}, 32, false},
		// array at d+1, element message at d+2, element's containers at d+3
		{"array_elem_containers_at_32", &TreeNode{Children: []TreeNode{{ID: 1}}}, 29, true},
		{"array_elem_containers_at_33", &TreeNode{Children: []TreeNode{{ID: 1}}}, 30, false},
		{"ptr_array_elem_containers_at_33", &TreeNode{Branches: []*TreeNode{{ID: 1}}}, 30, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw, err := Marshal(treeChain(c.depth, c.leaf))
			if err != nil {
				t.Fatal(err)
			}
			var dst TreeNode
			tableErr := Unmarshal(raw, &dst)
			_, blindErr := DecodeBlind(raw)
			if c.accept {
				if tableErr != nil || blindErr != nil {
					t.Fatalf("expected accept: table=%v blind=%v", tableErr, blindErr)
				}
			} else if tableErr == nil || blindErr == nil {
				t.Fatalf("expected reject: table=%v blind=%v", tableErr, blindErr)
			}
		})
	}
}

// ============================================================================
// 10. Sub-message array element access validates the whole array
// ============================================================================

func TestEdge_SubMsgArrayAccessorsRejectTrailingBytes(t *testing.T) {
	elem := AppendVarint(appendTag(nil, 1, WireUint), 5)
	body := []byte{WireSubMsg, 0x03}
	body = append(body, AppendVarint(nil, uint64(len(elem)))...)
	body = append(body, elem...)
	good := concat(appendTag(nil, 11, WireArray), AppendVarint(nil, uint64(len(body))), body)
	if e, ok := ReadSubMsgArrElem(good, 11, 0); !ok || !bytes.Equal(e, elem) {
		t.Fatalf("ReadSubMsgArrElem(good) = %x, %v", e, ok)
	}
	if n := SubMsgArrLen(good, 11); n != 1 {
		t.Fatalf("SubMsgArrLen(good) = %d", n)
	}
	badBody := append(append([]byte(nil), body...), 0xEE) // trailing byte after declared count
	bad := concat(appendTag(nil, 11, WireArray), AppendVarint(nil, uint64(len(badBody))), badBody)
	if _, ok := ReadElem(badBody, 0); ok {
		t.Error("ReadElem accepted array with trailing bytes")
	}
	if _, ok := ReadSubMsgArrElem(bad, 11, 0); ok {
		t.Error("ReadSubMsgArrElem accepted array with trailing bytes")
	}
	if n := SubMsgArrLen(bad, 11); n != 0 {
		t.Errorf("SubMsgArrLen(bad) = %d, want 0", n)
	}
	requireAllTiersReject(t, bad)
	for _, idx := range []int{-1, 1, 1 << 30} {
		if _, ok := ReadSubMsgArrElem(good, 11, idx); ok {
			t.Errorf("ReadSubMsgArrElem(index %d) succeeded", idx)
		}
	}
}

// ============================================================================
// 11. Duplicate map keys, empty maps, and scalar extremes agree across tiers
// ============================================================================

func TestEdge_MapDuplicateKeysAndEmptyCollections(t *testing.T) {
	body := []byte{WireString, WireUint, 0x07}
	for _, kv := range []struct {
		k string
		v byte
	}{{"a", 1}, {"b", 2}, {"a", 3}} {
		body = append(body, 0x03, kv.k[0], kv.v<<1|1)
	}
	raw := concat(appendTag(nil, 12, WireMap), AppendVarint(nil, uint64(len(body))), body)
	got := requireAllTiersAgree(t, raw)
	want, _ := MarshalDeterministic(&TestAllTypes{F12MapStrUint: map[string]uint64{"a": 3, "b": 2}})
	if !bytes.Equal(got, want) {
		t.Fatalf("duplicate map key: last entry must win")
	}
	if v, ok := LookupMapStringUint(raw, 12, "a"); !ok || v != 3 {
		t.Fatalf("LookupMapStringUint last-wins = (%d, %v)", v, ok)
	}

	empty := &TestAllTypes{F9StringArr: []string{}, F10Uint32Arr: []uint32{}, F11SubMsgArr: []AllTypesSubMsg{}, F12MapStrUint: map[string]uint64{}, F8SubMsg: &AllTypesSubMsg{}}
	raw, err := MarshalDeterministic(empty)
	if err != nil {
		t.Fatal(err)
	}
	requireAllTiersAgree(t, raw)
	if _, err := DecodeBlind(raw); err != nil {
		t.Fatal(err)
	}
}

func TestEdge_ScalarExtremesRoundTripAllTiers(t *testing.T) {
	const maxU32, maxU64 = ^uint32(0), ^uint64(0)
	for _, m := range []*TestAllTypes{
		{F1Uint32: maxU32, F2Uint64: maxU64, F3Sint32: -1 << 31, F4Sint64: -1 << 63, F15Fixed32: maxU32, F16Fixed64: maxU64},
		{F3Sint32: 1<<31 - 1, F4Sint64: 1<<63 - 1},
		{F1Uint32: 1 << 29, F2Uint64: 1<<29 - 1},
		{F13Float32: math.Float32frombits(0x7fc00001), F14Float64: math.Float64frombits(0x7ff8000000000001)}, // NaN payloads
		{F13Float32: math.Float32frombits(0x80000000), F14Float64: math.Float64frombits(0x8000000000000000)}, // -0
		{F13Float32: math.Float32frombits(0x00000001), F14Float64: math.Float64frombits(1)},                  // denormals
	} {
		raw, err := MarshalDeterministic(m)
		if err != nil {
			t.Fatal(err)
		}
		if got := requireAllTiersAgree(t, raw); !bytes.Equal(got, raw) {
			t.Fatalf("scalar extremes changed on round trip:\n got  %x\n want %x", got, raw)
		}
	}
}

// ============================================================================
// 12. Differential mutation testing: tiers never disagree
// ============================================================================

func randomTestAllTypes(r *rand.Rand) *TestAllTypes {
	str := func() string {
		alphabet := []string{"", "a", "é", "🚀", "\x00", "z"}
		var b strings.Builder
		for i := r.Intn(4); i > 0; i-- {
			b.WriteString(alphabet[r.Intn(len(alphabet))])
		}
		return b.String()
	}
	m := &TestAllTypes{
		F1Uint32: r.Uint32() >> uint(r.Intn(32)), F2Uint64: r.Uint64() >> uint(r.Intn(64)),
		F3Sint32: int32(r.Uint32()) >> uint(r.Intn(32)), F4Sint64: int64(r.Uint64()) >> uint(r.Intn(64)),
		F5Bool: r.Intn(2) == 0, F6String: str(), F13Float32: r.Float32(), F14Float64: r.NormFloat64(),
		F15Fixed32: r.Uint32(), F16Fixed64: r.Uint64(), F20OverflowStr: str(),
	}
	if r.Intn(2) == 0 {
		m.F7Bytes = []byte(str())
	}
	if r.Intn(2) == 0 {
		m.F8SubMsg = &AllTypesSubMsg{ID: r.Uint32(), Label: str(), Flag: r.Intn(2) == 0, Score: r.Int63() - r.Int63()}
	}
	for i := r.Intn(4); i > 0; i-- {
		m.F9StringArr = append(m.F9StringArr, str())
		m.F10Uint32Arr = append(m.F10Uint32Arr, r.Uint32()>>uint(r.Intn(32)))
		m.F11SubMsgArr = append(m.F11SubMsgArr, AllTypesSubMsg{ID: uint32(i), Label: str()})
	}
	if n := r.Intn(4); n > 0 {
		m.F12MapStrUint = map[string]uint64{}
		for i := 0; i < n; i++ {
			m.F12MapStrUint[str()] = r.Uint64() >> uint(r.Intn(64))
		}
	}
	return m
}

func TestEdge_DifferentialRandomMessagesAndConcatenation(t *testing.T) {
	r := rand.New(rand.NewSource(0x0b1b0f))
	for i := 0; i < 300; i++ {
		a, b := randomTestAllTypes(r), randomTestAllTypes(r)
		aw, err := MarshalDeterministic(a)
		if err != nil {
			t.Fatal(err)
		}
		bw, err := MarshalDeterministic(b)
		if err != nil {
			t.Fatal(err)
		}
		// table Marshal of the struct and of the equivalent pb message must match the plugin encoder.
		var viaPlugin pb.TestAllTypes
		if err := viaPlugin.UnmarshalOmniBuf(aw, false); err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if pw := viaPlugin.MarshalOmniBufAppend(nil); !bytes.Equal(normalize(t, pw), aw) {
			t.Fatalf("iteration %d: plugin re-encode differs", i)
		}
		if got := requireAllTiersAgree(t, aw); !bytes.Equal(got, aw) {
			t.Fatalf("iteration %d: round trip changed bytes", i)
		}
		requireAllTiersAgree(t, concat(aw, bw))
		if _, err := DecodeBlind(concat(aw, bw)); err != nil {
			t.Fatalf("iteration %d: DecodeBlind(concat): %v", i, err)
		}
	}
}

func normalize(t *testing.T, raw []byte) []byte {
	t.Helper()
	var m TestAllTypes
	if err := Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	b, err := MarshalDeterministic(&m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mutate(r *rand.Rand, in []byte) []byte {
	b := append([]byte(nil), in...)
	switch r.Intn(6) {
	case 0: // flip a byte
		if len(b) > 0 {
			b[r.Intn(len(b))] ^= byte(1 << uint(r.Intn(8)))
		}
	case 1: // set a byte to a reserved varint marker
		if len(b) > 0 {
			b[r.Intn(len(b))] = byte(8 * (1 + r.Intn(31)))
		}
	case 2: // truncate
		if len(b) > 0 {
			b = b[:r.Intn(len(b))]
		}
	case 3: // insert random byte
		p := r.Intn(len(b) + 1)
		b = append(b[:p], append([]byte{byte(r.Intn(256))}, b[p:]...)...)
	case 4: // delete a byte
		if len(b) > 0 {
			p := r.Intn(len(b))
			b = append(b[:p], b[p+1:]...)
		}
	case 5: // inject invalid UTF-8
		if len(b) > 0 {
			b[r.Intn(len(b))] = 0xff
		}
	}
	return b
}

func TestEdge_DifferentialMutationsTiersNeverDisagree(t *testing.T) {
	r := rand.New(rand.NewSource(0xdecaf))
	n := 4000
	if testing.Short() {
		n = 500
	}
	for i := 0; i < n; i++ {
		seed, err := MarshalDeterministic(randomTestAllTypes(r))
		if err != nil {
			t.Fatal(err)
		}
		raw := seed
		for k := r.Intn(3) + 1; k > 0; k-- {
			raw = mutate(r, raw)
		}
		t.Run(fmt.Sprintf("m%d", i), func(t *testing.T) {
			requireTiersConsistent(t, raw)
			exerciseAllReadersNoPanic(raw)
		})
	}
}

// FuzzDifferentialTiers is the open-ended version of the mutation test:
//
//	go test -run=^$ -fuzz=FuzzDifferentialTiers .
func FuzzDifferentialTiers(f *testing.F) {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 16; i++ {
		seed, _ := MarshalDeterministic(randomTestAllTypes(r))
		f.Add(seed)
	}
	for _, ic := range buildInvalidWireCases() {
		f.Add([]byte(ic.WireHex))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1024 {
			return
		}
		requireTiersConsistent(t, data)
	})
}

// Reflection widens float32 through float64; signaling NaNs may become quiet.
// The struct and generated tiers must still preserve their exact wire payload.
func TestEdge_SignalingNaNBits(t *testing.T) {
	for _, bits := range []uint32{0x7f800001, 0x7fa83030, 0xffa83030, 0x7fc00001} {
		raw, err := MarshalDeterministic(&TestAllTypes{F13Float32: math.Float32frombits(bits)})
		if err != nil {
			t.Fatal(err)
		}
		requireAllTiersAgree(t, raw)
		var plain TestAllTypes
		if err := Unmarshal(raw, &plain); err != nil {
			t.Fatal(err)
		}
		if got := math.Float32bits(plain.F13Float32); got != bits {
			t.Fatalf("table: got %x, want %x", got, bits)
		}
		for _, zeroCopy := range []bool{false, true} {
			var generated pb.TestAllTypes
			if err := generated.UnmarshalOmniBuf(raw, zeroCopy); err != nil {
				t.Fatal(err)
			}
			if got := math.Float32bits(generated.F13Float32); got != bits {
				t.Fatalf("generated: got %x, want %x", got, bits)
			}
		}
	}
}

func TestEdge_TableReuseAcrossSeenMaskBoundary(t *testing.T) {
	for _, count := range []int{0, 1, 63, 64, 65, 70} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			fields := make([]reflect.StructField, count)
			for i := range fields {
				fields[i] = reflect.StructField{Name: fmt.Sprintf("F%d", i+1), Type: reflect.TypeFor[uint64](), Tag: reflect.StructTag(fmt.Sprintf(`omnibuf:"%d"`, i+1))}
			}
			typ := reflect.StructOf(fields)
			input, output := reflect.New(typ), reflect.New(typ)
			for i := range count {
				input.Elem().Field(i).SetUint(uint64(i + 1))
			}
			raw, err := Marshal(input.Interface())
			if err != nil {
				t.Fatal(err)
			}
			if err := Unmarshal(raw, output.Interface()); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(input.Interface(), output.Interface()) {
				t.Fatal("full decode changed fields")
			}
			if count != 0 {
				enc := NewEncoder(16)
				enc.WriteUint(uint16(count), 99)
				if err := Unmarshal(enc.Finish(), output.Interface()); err != nil {
					t.Fatal(err)
				}
				for i := range count {
					want := uint64(0)
					if i == count-1 {
						want = 99
					}
					if got := output.Elem().Field(i).Uint(); got != want {
						t.Fatalf("field %d: got %d, want %d", i+1, got, want)
					}
				}
			}
			if err := Unmarshal(nil, output.Interface()); err != nil {
				t.Fatal(err)
			}
			if !output.Elem().IsZero() {
				t.Fatal("empty decode retained stale fields")
			}
		})
	}
}
