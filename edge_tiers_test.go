package omnibuf_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/dynamicpb"

	. "github.com/sudorandom/omnibuf"
	"github.com/sudorandom/omnibuf/internal/pb"
	. "github.com/sudorandom/omnibuf/wire"
)

// ============================================================================
// Cross-tier decode harness
//
// Every schema-aware decoder tier must agree with every other tier on whether a
// packet is valid and on the decoded value. The tiers exercised here are:
//
//   table/struct     omnibuf.Unmarshal into a plain tagged Go struct
//   table/struct-zc  omnibuf.UnmarshalZeroCopy into a plain tagged Go struct
//   table/pb         omnibuf.Unmarshal into a protoc-gen-go struct (table codec)
//   reflect/dynamic  omnibuf.Unmarshal into a dynamicpb.Message (protoreflect codec)
//   plugin           protoc-gen-omnibuf generated UnmarshalOmniBuf (copying)
//   plugin-zc        protoc-gen-omnibuf generated UnmarshalOmniBuf (zero-copy)
//
// DecodeBlind is checked separately because it has no schema.
// ============================================================================

type tierOutcome struct {
	tier string
	err  error
	// canonical is the deterministic table re-encoding of the decoded value,
	// which is byte-identical across tiers whenever their values are equal.
	canonical []byte
	pbMsg     proto.Message
}

var testAllTypesDesc = (&pb.TestAllTypes{}).ProtoReflect().Descriptor()

func decodeTestAllTypesAllTiers(t *testing.T, raw []byte) []tierOutcome {
	t.Helper()
	var outs []tierOutcome
	reencode := func(v any) []byte {
		b, err := MarshalDeterministic(v)
		if err != nil {
			t.Fatalf("re-encode %T failed: %v", v, err)
		}
		// protoreflect.ValueOfFloat32 widens to float64, quieting signaling
		// NaNs. Ignore only that bit in cross-tier comparisons; retain the
		// sign and payload, and leave the decoded values untouched.
		if value, wt, ok := LocateField(b, 13); ok && wt == WireFloat32 && len(value) >= 4 {
			bits := binary.LittleEndian.Uint32(value)
			if bits&0x7f800000 == 0x7f800000 && bits&0x007fffff != 0 {
				binary.LittleEndian.PutUint32(value, bits|0x00400000)
			}
		}
		return b
	}

	{
		var s TestAllTypes
		err := Unmarshal(cloneBytes(raw), &s)
		o := tierOutcome{tier: "table/struct", err: err}
		if err == nil {
			o.canonical = reencode(&s)
		}
		outs = append(outs, o)
	}
	{
		var s TestAllTypes
		err := UnmarshalZeroCopy(cloneBytes(raw), &s)
		o := tierOutcome{tier: "table/struct-zc", err: err}
		if err == nil {
			o.canonical = reencode(&s)
		}
		outs = append(outs, o)
	}
	{
		m := &pb.TestAllTypes{}
		err := Unmarshal(cloneBytes(raw), m)
		o := tierOutcome{tier: "table/pb", err: err, pbMsg: m}
		if err == nil {
			o.canonical = reencode(m)
		}
		outs = append(outs, o)
	}
	{
		m := dynamicpb.NewMessage(testAllTypesDesc)
		err := Unmarshal(cloneBytes(raw), m)
		o := tierOutcome{tier: "reflect/dynamic", err: err, pbMsg: m}
		if err == nil {
			// dynamicpb has no Go struct, so convert through protobuf for the canonical form.
			cm := &pb.TestAllTypes{}
			pbBytes, perr := proto.Marshal(m)
			if perr != nil {
				t.Fatalf("proto.Marshal(dynamicpb) failed: %v", perr)
			}
			if perr := proto.Unmarshal(pbBytes, cm); perr != nil {
				t.Fatalf("proto.Unmarshal(dynamicpb bytes) failed: %v", perr)
			}
			o.canonical = reencode(cm)
		}
		outs = append(outs, o)
	}
	for _, zc := range []bool{false, true} {
		m := &pb.TestAllTypes{}
		err := m.UnmarshalOmniBuf(cloneBytes(raw), zc)
		name := "plugin"
		if zc {
			name = "plugin-zc"
		}
		o := tierOutcome{tier: name, err: err, pbMsg: m}
		if err == nil {
			o.canonical = reencode(m)
		}
		outs = append(outs, o)
	}
	return outs
}

func cloneBytes(b []byte) []byte { return append([]byte(nil), b...) }

// requireAllTiersReject asserts that every schema-aware tier AND DecodeBlind reject raw.
func requireAllTiersReject(t *testing.T, raw []byte) {
	t.Helper()
	for _, o := range decodeTestAllTypesAllTiers(t, raw) {
		if o.err == nil {
			t.Errorf("%s accepted invalid packet %x", o.tier, raw)
		}
	}
	if _, err := DecodeBlind(raw); err == nil {
		t.Errorf("DecodeBlind accepted invalid packet %x", raw)
	}
}

// requireAllTiersAgree asserts every schema-aware tier accepts raw with
// identical decoded values, and returns the canonical re-encoding.
func requireAllTiersAgree(t *testing.T, raw []byte) []byte {
	t.Helper()
	outs := decodeTestAllTypesAllTiers(t, raw)
	var want []byte
	for _, o := range outs {
		if o.err != nil {
			t.Fatalf("%s rejected valid packet %x: %v", o.tier, raw, o.err)
		}
		if want == nil {
			want = o.canonical
			continue
		}
		if !bytes.Equal(o.canonical, want) {
			t.Fatalf("%s decoded a different value:\n got  %x\n want %x (%s)", o.tier, o.canonical, want, outs[0].tier)
		}
	}
	// protobuf-level equality between the proto-backed tiers.
	var ref proto.Message
	for _, o := range outs {
		if o.pbMsg == nil {
			continue
		}
		if ref == nil {
			ref = o.pbMsg
			continue
		}
		if !proto.Equal(ref, o.pbMsg) {
			t.Fatalf("%s proto value differs:\n got  %v\n want %v", o.tier, o.pbMsg, ref)
		}
	}
	return want
}

// requireTiersConsistent asserts that tiers either all reject or all accept
// with identical values (used by differential/fuzz tests).
func requireTiersConsistent(t *testing.T, raw []byte) {
	t.Helper()
	outs := decodeTestAllTypesAllTiers(t, raw)
	accepted, rejected := []string{}, []string{}
	for _, o := range outs {
		if o.err == nil {
			accepted = append(accepted, o.tier)
		} else {
			rejected = append(rejected, fmt.Sprintf("%s(%v)", o.tier, o.err))
		}
	}
	if len(accepted) > 0 && len(rejected) > 0 {
		t.Fatalf("tiers disagree on validity of %x:\n accepted: %v\n rejected: %v", raw, accepted, rejected)
	}
	if len(accepted) == len(outs) {
		requireAllTiersAgree(t, raw)
	}
}

// ============================================================================
// 1. Prefix-Varint first-byte table (all 256 first bytes)
// ============================================================================

func TestEdge_PrefixVarintFirstByteTable(t *testing.T) {
	for b0 := 0; b0 < 256; b0++ {
		buf := []byte{byte(b0), 0, 0, 0, 0, 0, 0, 0, 0}
		var wantN int
		switch {
		case b0&1 != 0:
			wantN = 1
		case b0&2 != 0:
			wantN = 2
		case b0&4 != 0:
			wantN = 4
		case b0 == 0:
			wantN = 9
		default:
			wantN = 0 // reserved: 0x08, 0x10, ..., 0xF8
		}
		if _, n := DecodeVarint(buf); n != wantN {
			t.Errorf("DecodeVarint first byte 0x%02x: n=%d, want %d", b0, n, wantN)
		}
		// Every truncation shorter than the encoded width must be rejected.
		for l := 0; l < wantN; l++ {
			if _, n := DecodeVarint(buf[:l]); n != 0 {
				t.Errorf("DecodeVarint(0x%02x truncated to %d bytes) n=%d, want 0", b0, l, n)
			}
		}
		// SkipValue must agree with DecodeVarint for varint-typed values.
		next, err := SkipValue(buf, 0, WireUint)
		if wantN == 0 {
			if err == nil {
				t.Errorf("SkipValue accepted reserved varint first byte 0x%02x", b0)
			}
		} else if err != nil || next != wantN {
			t.Errorf("SkipValue(0x%02x) = (%d, %v), want (%d, nil)", b0, next, err, wantN)
		}
	}
}

func TestEdge_PrefixVarintBoundariesAndOverlong(t *testing.T) {
	cases := []struct {
		v     uint64
		width int
	}{
		{0, 1}, {1, 1}, {126, 1}, {127, 1},
		{128, 2}, {129, 2}, {16382, 2}, {16383, 2},
		{16384, 4}, {16385, 4}, {1<<29 - 2, 4}, {1<<29 - 1, 4},
		{1 << 29, 9}, {1<<29 + 1, 9}, {1<<32 - 1, 9}, {1 << 32, 9},
		{1<<63 - 1, 9}, {1 << 63, 9}, {^uint64(0) - 1, 9}, {^uint64(0), 9},
	}
	for _, c := range cases {
		enc := AppendVarint(nil, c.v)
		if len(enc) != c.width {
			t.Errorf("AppendVarint(%d) width %d, want %d", c.v, len(enc), c.width)
		}
		v, n := DecodeVarint(enc)
		if v != c.v || n != c.width {
			t.Errorf("DecodeVarint(AppendVarint(%d)) = (%d, %d)", c.v, v, n)
		}
		// Trailing garbage must not be consumed.
		v, n = DecodeVarint(append(enc, 0xAA, 0xBB))
		if v != c.v || n != c.width {
			t.Errorf("DecodeVarint with trailing bytes (%d) = (%d, %d)", c.v, v, n)
		}
	}

	// Overlong (non-minimal) encodings are valid and decode to the same value.
	for _, v := range []uint64{0, 1, 5, 127, 128, 16383} {
		ov2 := []byte{byte(v<<2) | 2, byte(v >> 6)}
		if got, n := DecodeVarint(ov2); got != v || n != 2 {
			t.Errorf("overlong 2B %d -> (%d, %d)", v, got, n)
		}
		u := uint32(v<<3) | 4
		ov4 := []byte{byte(u), byte(u >> 8), byte(u >> 16), byte(u >> 24)}
		if got, n := DecodeVarint(ov4); got != v || n != 4 {
			t.Errorf("overlong 4B %d -> (%d, %d)", v, got, n)
		}
		ov9 := []byte{0, byte(v), byte(v >> 8), 0, 0, 0, 0, 0, 0}
		if got, n := DecodeVarint(ov9); got != v || n != 9 {
			t.Errorf("overlong 9B %d -> (%d, %d)", v, got, n)
		}
	}
}

// Overlong varints are valid in every position and all tiers must accept them
// with the same value as the minimal encoding.
func TestEdge_OverlongVarintsAcceptedEverywhere(t *testing.T) {
	ov9 := func(v uint64) []byte {
		b := []byte{0}
		for i := 0; i < 8; i++ {
			b = append(b, byte(v>>(8*i)))
		}
		return b
	}
	ov2 := func(v uint64) []byte { u := uint16(v<<2) | 2; return []byte{byte(u), byte(u >> 8)} }

	var pkt []byte
	pkt = append(pkt, ov2((1<<4)|uint64(WireUint))...) // overlong tag for F1
	pkt = append(pkt, ov9(42)...)                      // overlong value
	pkt = append(pkt, ov9((3<<4)|uint64(WireSint))...) // 9-byte tag for F3
	pkt = append(pkt, ov2(EncodeZigZag(-7))...)
	pkt = appendTag(pkt, 6, WireString)
	pkt = append(pkt, ov9(3)...) // overlong string length
	pkt = append(pkt, "abc"...)
	pkt = appendTag(pkt, 10, WireArray)
	pkt = append(pkt, ov9(1+9+2+9)...) // overlong array byteLen
	pkt = append(pkt, WireUint)
	pkt = append(pkt, ov9(2)...) // overlong count
	pkt = append(pkt, ov2(1)...)
	pkt = append(pkt, ov9(2)...)
	pkt = appendTag(pkt, 12, WireMap)
	body := []byte{WireString, WireUint}
	body = append(body, ov2(1)...)
	body = append(body, ov9(1)...)
	body = append(body, 'k')
	body = append(body, ov2(9)...)
	pkt = append(pkt, ov9(uint64(len(body)))...)
	pkt = append(pkt, body...)

	got := requireAllTiersAgree(t, pkt)
	want, err := MarshalDeterministic(&TestAllTypes{
		F1Uint32: 42, F3Sint32: -7, F6String: "abc",
		F10Uint32Arr: []uint32{1, 2}, F12MapStrUint: map[string]uint64{"k": 9},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("overlong packet decoded to %x, want %x", got, want)
	}
	if _, err := DecodeBlind(pkt); err != nil {
		t.Fatalf("DecodeBlind rejected overlong varints: %v", err)
	}
	if v, ok := ReadUint(pkt, 1); !ok || v != 42 {
		t.Fatalf("ReadUint over overlong encodings = (%d, %v)", v, ok)
	}
	if s, ok := ReadString(pkt, 6); !ok || s != "abc" {
		t.Fatalf("ReadString over overlong encodings = (%q, %v)", s, ok)
	}
}

// ============================================================================
// 2. Reserved varint first bytes in every structural position
// ============================================================================

func reservedVarint(rb byte) []byte { return []byte{rb, 0, 0, 0, 0, 0, 0, 0, 0} }

type reservedPosition struct {
	name     string
	topLevel bool // reserved byte lies in a field header/length the header-hopper must parse
	build    func(rb byte) []byte
}

func reservedPositions() []reservedPosition {
	tagged := func(f uint16, wt uint8, rest ...[]byte) []byte {
		b := appendTag(nil, f, wt)
		for _, r := range rest {
			b = append(b, r...)
		}
		return b
	}
	pad := []byte("abcdefghij")
	return []reservedPosition{
		{"tag", true, func(rb byte) []byte { return reservedVarint(rb) }},
		{"uint_value", true, func(rb byte) []byte { return tagged(1, WireUint, reservedVarint(rb)) }},
		{"sint_value", true, func(rb byte) []byte { return tagged(3, WireSint, reservedVarint(rb)) }},
		{"string_len", true, func(rb byte) []byte { return tagged(6, WireString, reservedVarint(rb), pad) }},
		{"bytes_len", true, func(rb byte) []byte { return tagged(7, WireBytes, reservedVarint(rb), pad) }},
		{"submsg_len", true, func(rb byte) []byte { return tagged(8, WireSubMsg, reservedVarint(rb), pad) }},
		{"array_len", true, func(rb byte) []byte { return tagged(10, WireArray, reservedVarint(rb), pad) }},
		{"array_count", true, func(rb byte) []byte {
			return tagged(10, WireArray, AppendVarint(nil, 10), []byte{WireUint}, reservedVarint(rb))
		}},
		{"map_len", true, func(rb byte) []byte { return tagged(12, WireMap, reservedVarint(rb), pad) }},
		{"map_count", true, func(rb byte) []byte {
			return tagged(12, WireMap, AppendVarint(nil, 11), []byte{WireString, WireUint}, reservedVarint(rb))
		}},
		{"overflow_field_string_len", true, func(rb byte) []byte { return tagged(20, WireString, reservedVarint(rb), pad) }},
		{"unknown_uint_value", true, func(rb byte) []byte { return tagged(99, WireUint, reservedVarint(rb)) }},
		{"unknown_bytes_len", true, func(rb byte) []byte { return tagged(99, WireBytes, reservedVarint(rb), pad) }},
		{"unknown_array_count", true, func(rb byte) []byte {
			return tagged(99, WireArray, AppendVarint(nil, 10), []byte{WireUint}, reservedVarint(rb))
		}},
		{"unknown_map_count", true, func(rb byte) []byte {
			return tagged(99, WireMap, AppendVarint(nil, 11), []byte{WireUint, WireUint}, reservedVarint(rb))
		}},
		{"field0_len", true, func(rb byte) []byte {
			return append([]byte{byte(((0<<4)|WireSubMsg)<<1) | 1}, append(reservedVarint(rb), pad...)...)
		}},
		// Positions inside container bodies (header-hoppers need not parse these).
		{"uint_array_element", false, func(rb byte) []byte {
			return tagged(10, WireArray, AppendVarint(nil, 11), []byte{WireUint, 0x03}, reservedVarint(rb))
		}},
		{"string_array_element_len", false, func(rb byte) []byte {
			return tagged(9, WireArray, AppendVarint(nil, 11), []byte{WireString, 0x03}, reservedVarint(rb))
		}},
		{"submsg_array_element_len", false, func(rb byte) []byte {
			return tagged(11, WireArray, AppendVarint(nil, 11), []byte{WireSubMsg, 0x03}, reservedVarint(rb))
		}},
		{"map_key_len", false, func(rb byte) []byte {
			return tagged(12, WireMap, AppendVarint(nil, 12), []byte{WireString, WireUint, 0x03}, reservedVarint(rb))
		}},
		{"map_value", false, func(rb byte) []byte {
			return tagged(12, WireMap, AppendVarint(nil, 14), []byte{WireString, WireUint, 0x03, 0x03, 'k'}, reservedVarint(rb))
		}},
		{"nested_submsg_tag", false, func(rb byte) []byte {
			return tagged(8, WireSubMsg, AppendVarint(nil, 9), reservedVarint(rb))
		}},
		{"nested_submsg_value", false, func(rb byte) []byte {
			return tagged(8, WireSubMsg, AppendVarint(nil, 10), appendTag(nil, 1, WireUint), reservedVarint(rb))
		}},
		{"submsg_array_element_field", false, func(rb byte) []byte {
			elem := append(appendTag(nil, 1, WireUint), reservedVarint(rb)...)
			body := append([]byte{WireSubMsg, 0x03}, AppendVarint(nil, uint64(len(elem)))...)
			body = append(body, elem...)
			return tagged(11, WireArray, AppendVarint(nil, uint64(len(body))), body)
		}},
	}
}

func TestEdge_ReservedVarintFirstBytesRejectedEverywhere(t *testing.T) {
	trailer := appendTag(nil, 2, WireUint)
	trailer = AppendVarint(trailer, 7)
	for _, pos := range reservedPositions() {
		for rb := 0x08; rb < 0x100; rb += 0x08 {
			raw := append(pos.build(byte(rb)), trailer...)
			t.Run(fmt.Sprintf("%s/0x%02x", pos.name, rb), func(t *testing.T) {
				requireAllTiersReject(t, raw)
				if pos.topLevel {
					if _, ok := ReadUint(raw, 2); ok {
						t.Errorf("ReadUint hopped across reserved varint in %s", pos.name)
					}
					if _, _, ok := LocateField(raw, 2); ok {
						t.Errorf("LocateField hopped across reserved varint in %s", pos.name)
					}
					if _, ok := ReadSubMsg(raw, 2); ok {
						t.Errorf("ReadSubMsg hopped across reserved varint in %s", pos.name)
					}
				}
				exerciseAllReadersNoPanic(raw)
			})
		}
	}
}

// The same positions with 0x00 (the valid 9-byte marker) must decode.
func TestEdge_NineByteMarkerAcceptedInSamePositions(t *testing.T) {
	// value 0 in 9-byte form is a valid overlong zero
	raw := appendTag(nil, 1, WireUint)
	raw = append(raw, reservedVarint(0)...)
	raw = appendTag(raw, 99, WireUint)
	raw = append(raw, reservedVarint(0)...)
	raw = appendTag(raw, 2, WireUint)
	raw = AppendVarint(raw, 7)
	requireAllTiersAgree(t, raw)
	if v, ok := ReadUint(raw, 2); !ok || v != 7 {
		t.Fatalf("ReadUint(2) = (%d, %v)", v, ok)
	}
}

// ============================================================================
// 3. Field #0 placement and validation in every tier
// ============================================================================

func TestEdge_Field0ValidationAllTiers(t *testing.T) {
	valid := (&SchemaMeta{TypeName: "T", Fields: map[uint32]string{1: "a"}}).AppendField0(nil, true)
	f1 := AppendVarint(appendTag(nil, 1, WireUint), 5)

	t.Run("valid_first_middle_last_and_repeated", func(t *testing.T) {
		for _, raw := range [][]byte{
			concat(valid, f1),
			concat(f1, valid),
			concat(f1, valid, f1),
			concat(valid, valid, f1, valid),
		} {
			requireAllTiersAgree(t, raw)
			if _, err := DecodeBlind(raw); err != nil {
				t.Fatalf("DecodeBlind(%x): %v", raw, err)
			}
		}
	})

	for wt := uint8(0); wt < 16; wt++ {
		if wt == WireSubMsg {
			continue
		}
		wt := wt
		t.Run(fmt.Sprintf("wrong_wire_type_%d_mid_packet", wt), func(t *testing.T) {
			raw := concat(f1, []byte{byte(wt<<1) | 1}, AppendVarint(nil, 1), []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
			requireAllTiersReject(t, raw)
		})
	}

	corrupt := []struct {
		name string
		body []byte
	}{
		{"truncated_type_name", []byte{byte(((1<<4)|WireString)<<1 | 1), 0x15, 'A'}},
		{"type_name_wrong_wire_type", []byte{byte(((1<<4)|WireUint)<<1 | 1), 0x03}},
		{"fields_map_wrong_value_type", []byte{byte(((2<<4)|WireMap)<<1 | 1), 0x07, WireUint, WireUint, 0x01}},
		{"fields_map_zero_field_number", []byte{byte(((2<<4)|WireMap)<<1 | 1), 0x0D, WireUint, WireString, 0x03, 0x01, 0x03, 'x'}},
		{"fields_map_trailing_bytes", []byte{byte(((2<<4)|WireMap)<<1 | 1), 0x0F, WireUint, WireString, 0x03, 0x03, 0x03, 'x', 0xFF}},
		{"nested_field0", []byte{byte(((0<<4)|WireSubMsg)<<1 | 1), 0x01}},
		{"invalid_utf8_type_name", []byte{byte(((1<<4)|WireString)<<1 | 1), 0x03, 0xFF}},
		{"invalid_utf8_field_name", []byte{byte(((2<<4)|WireMap)<<1 | 1), 0x0D, WireUint, WireString, 0x03, 0x03, 0x03, 0xFF}},
	}
	for _, c := range corrupt {
		meta := append([]byte{byte(((0<<4)|WireSubMsg)<<1) | 1}, AppendVarint(nil, uint64(len(c.body)))...)
		meta = append(meta, c.body...)
		for _, layout := range []struct {
			name string
			raw  []byte
		}{
			{"first", concat(meta, f1)},
			{"last", concat(f1, meta)},
			{"after_valid", concat(valid, f1, meta)},
		} {
			t.Run(c.name+"/"+layout.name, func(t *testing.T) {
				requireAllTiersReject(t, layout.raw)
			})
		}
		// Field #0 inside a nested sub-message must be validated too.
		t.Run(c.name+"/inside_submsg", func(t *testing.T) {
			raw := appendTag(nil, 8, WireSubMsg)
			raw = AppendVarint(raw, uint64(len(meta)))
			raw = append(raw, meta...)
			requireAllTiersReject(t, raw)
		})
	}
}

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// ============================================================================
// 4. Container headers are validated even when the field is skipped
// ============================================================================

func TestEdge_ContainerHeadersValidatedWhenSkipping(t *testing.T) {
	trailer := AppendVarint(appendTag(nil, 2, WireUint), 7)
	arr := func(f uint16, body ...byte) []byte {
		return concat(appendTag(nil, f, WireArray), AppendVarint(nil, uint64(len(body))), body)
	}
	mp := func(f uint16, body ...byte) []byte {
		return concat(appendTag(nil, f, WireMap), AppendVarint(nil, uint64(len(body))), body)
	}
	cases := map[string][]byte{
		"array_elem_bool_true":        arr(99, WireBoolTrue, 0x01),
		"array_elem_16":               arr(99, 16, 0x01),
		"array_elem_0xff":             arr(99, 0xFF, 0x01),
		"array_body_1_byte":           arr(99, WireUint),
		"array_empty_body":            arr(99),
		"array_count_exceeds_body":    arr(99, WireUint, 0x07, 0x03),
		"array_fixed32_misaligned":    arr(99, WireFixed32, 0x03, 1, 2, 3),
		"array_fixed64_count_wrong":   arr(99, WireFixed64, 0x05, 1, 2, 3, 4, 5, 6, 7, 8),
		"array_bool_count_wrong":      arr(99, WireBoolFalse, 0x05, 0x01),
		"map_key_float32":             mp(99, WireFloat32, WireUint, 0x01),
		"map_key_float64":             mp(99, WireFloat64, WireUint, 0x01),
		"map_key_bytes":               mp(99, WireBytes, WireUint, 0x01),
		"map_key_submsg":              mp(99, WireSubMsg, WireUint, 0x01),
		"map_key_array":               mp(99, WireArray, WireUint, 0x01),
		"map_key_map":                 mp(99, WireMap, WireUint, 0x01),
		"map_key_bool_true":           mp(99, WireBoolTrue, WireUint, 0x01),
		"map_key_extended":            mp(99, WireExtended, WireUint, 0x01),
		"map_key_16":                  mp(99, 16, WireUint, 0x01),
		"map_val_bool_true":           mp(99, WireUint, WireBoolTrue, 0x01),
		"map_val_16":                  mp(99, WireUint, 16, 0x01),
		"map_body_2_bytes":            mp(99, WireUint, WireUint),
		"map_empty_body":              mp(99),
		"map_count_exceeds_half_body": mp(99, WireUint, WireUint, 0x05, 0x03),
		"extended_empty_body":         concat(appendTag(nil, 99, WireExtended), AppendVarint(nil, 0)),
		"known_array_elem_bool_true":  arr(10, WireBoolTrue, 0x01),
		"known_map_key_float":         mp(12, WireFloat32, WireUint, 0x01),
	}
	for name, raw := range cases {
		raw := concat(raw, trailer)
		t.Run(name, func(t *testing.T) {
			requireAllTiersReject(t, raw)
			if _, ok := ReadUint(raw, 2); ok {
				t.Errorf("ReadUint hopped across invalid container header")
			}
			if _, _, ok := LocateField(raw, 2); ok {
				t.Errorf("LocateField hopped across invalid container header")
			}
			if _, ok := ReadSubMsg(raw, 2); ok {
				t.Errorf("ReadSubMsg hopped across invalid container header")
			}
		})
	}
}

// Valid empty containers of every legal element / key / value type are
// accepted and skipped by every tier.
func TestEdge_EmptyContainersOfEveryTypeAccepted(t *testing.T) {
	trailer := AppendVarint(appendTag(nil, 2, WireUint), 7)
	for elem := uint8(0); elem < 16; elem++ {
		if elem == WireBoolTrue {
			continue
		}
		raw := concat(appendTag(nil, 99, WireArray), AppendVarint(nil, 2), []byte{elem, 0x01}, trailer)
		requireAllTiersAgree(t, raw)
		if _, err := DecodeBlind(raw); err != nil && elem != WireExtended {
			t.Errorf("DecodeBlind rejected empty array of elem %d: %v", elem, err)
		}
		if v, ok := ReadUint(raw, 2); !ok || v != 7 {
			t.Errorf("ReadUint across empty array elem %d = (%d, %v)", elem, v, ok)
		}
	}
	for _, k := range []uint8{WireUint, WireSint, WireBoolFalse, WireFixed32, WireFixed64, WireSfixed32, WireSfixed64, WireString} {
		for v := uint8(0); v < 16; v++ {
			if v == WireBoolTrue {
				continue
			}
			raw := concat(appendTag(nil, 99, WireMap), AppendVarint(nil, 3), []byte{k, v, 0x01}, trailer)
			requireAllTiersAgree(t, raw)
			if got, ok := ReadUint(raw, 2); !ok || got != 7 {
				t.Errorf("ReadUint across empty map<%d,%d> = (%d, %v)", k, v, got, ok)
			}
		}
	}
}

// ============================================================================
// 5. Unknown fields of every wire type, including high field numbers
// ============================================================================

func TestEdge_UnknownFieldsOfEveryWireTypeSkippedByAllTiers(t *testing.T) {
	payloads := map[uint8][]byte{
		WireUint:      AppendVarint(nil, 1<<40),
		WireSint:      AppendVarint(nil, EncodeZigZag(-1<<40)),
		WireBoolFalse: nil,
		WireBoolTrue:  nil,
		WireFixed32:   {1, 2, 3, 4},
		WireFixed64:   {1, 2, 3, 4, 5, 6, 7, 8},
		WireSfixed32:  {1, 2, 3, 4},
		WireSfixed64:  {1, 2, 3, 4, 5, 6, 7, 8},
		WireFloat32:   {0, 0, 0x80, 0x3f},
		WireFloat64:   {0, 0, 0, 0, 0, 0, 0xf0, 0x3f},
		WireString:    concat(AppendVarint(nil, 3), []byte("abc")),
		WireBytes:     concat(AppendVarint(nil, 3), []byte{0xff, 0xfe, 0xfd}),
		WireSubMsg:    concat(AppendVarint(nil, 2), AppendVarint(appendTag(nil, 1, WireUint), 9)),
		WireArray:     concat(AppendVarint(nil, 4), []byte{WireUint, 0x05, 0x03, 0x05}),
		WireMap:       concat(AppendVarint(nil, 5), []byte{WireUint, WireUint, 0x03, 0x03, 0x05}),
		WireExtended:  concat(AppendVarint(nil, 3), []byte{0x42, 0xde, 0xad}),
	}
	trailer := AppendVarint(appendTag(nil, 1, WireUint), 77)
	for _, fnum := range []uint64{17, 18, 19, 21, 63, 64, 65, 1023, 1024, 65535, 65536, 65537, 1 << 20, 536870911} {
		for wt := uint8(0); wt < 16; wt++ {
			raw := AppendVarint(nil, fnum<<4|uint64(wt))
			raw = append(raw, payloads[wt]...)
			raw = append(raw, trailer...)
			got := requireAllTiersAgree(t, raw)
			want, _ := MarshalDeterministic(&TestAllTypes{F1Uint32: 77})
			if !bytes.Equal(got, want) {
				t.Errorf("unknown field %d wt %d altered decode: %x", fnum, wt, got)
			}
			if v, ok := ReadUint(raw, 1); !ok || v != 77 {
				t.Errorf("ReadUint after unknown field %d wt %d = (%d, %v)", fnum, wt, v, ok)
			}
		}
	}
	// Field number one past the 29-bit maximum is rejected everywhere.
	raw := AppendVarint(nil, uint64(536870912)<<4|uint64(WireUint))
	raw = append(raw, 0x03)
	requireAllTiersReject(t, raw)
}

// A known field arriving with a different wire type is skipped (Protobuf
// semantics) consistently by every schema-aware tier.
func TestEdge_KnownFieldWithWrongWireTypeSkippedConsistently(t *testing.T) {
	type alt struct {
		field   uint16
		wt      uint8
		payload []byte
	}
	alts := []alt{
		{1, WireString, concat(AppendVarint(nil, 1), []byte("x"))},
		{1, WireSint, AppendVarint(nil, 3)},
		{1, WireFixed32, []byte{1, 0, 0, 0}},
		{3, WireUint, AppendVarint(nil, 3)},
		{5, WireUint, AppendVarint(nil, 1)},
		{6, WireBytes, concat(AppendVarint(nil, 1), []byte{0xff})},
		{7, WireString, concat(AppendVarint(nil, 1), []byte("y"))},
		{8, WireBytes, concat(AppendVarint(nil, 0))},
		{9, WireString, concat(AppendVarint(nil, 1), []byte("z"))},
		{10, WireUint, AppendVarint(nil, 1)},
		{11, WireSubMsg, AppendVarint(nil, 0)},
		{12, WireArray, concat(AppendVarint(nil, 2), []byte{WireUint, 0x01})},
		{13, WireFloat64, make([]byte, 8)},
		{14, WireFloat32, make([]byte, 4)},
		{15, WireFixed64, make([]byte, 8)},
		{16, WireFixed32, make([]byte, 4)},
		{20, WireBytes, concat(AppendVarint(nil, 1), []byte{0xff})},
	}
	base := &TestAllTypes{F1Uint32: 11, F3Sint32: -2, F5Bool: true, F6String: "keep", F7Bytes: []byte{1},
		F8SubMsg: &AllTypesSubMsg{ID: 1}, F9StringArr: []string{"a"}, F10Uint32Arr: []uint32{1},
		F11SubMsgArr: []AllTypesSubMsg{{ID: 2}}, F12MapStrUint: map[string]uint64{"k": 1},
		F13Float32: 1.5, F14Float64: 2.5, F15Fixed32: 3, F16Fixed64: 4, F20OverflowStr: "o"}
	baseWire, err := MarshalDeterministic(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range alts {
		raw := concat(baseWire, appendTag(nil, a.field, a.wt), a.payload)
		t.Run(fmt.Sprintf("f%d_wt%d", a.field, a.wt), func(t *testing.T) {
			got := requireAllTiersAgree(t, raw)
			if !bytes.Equal(got, baseWire) {
				t.Fatalf("wrong-wire-type field altered decoded value:\n got  %x\n want %x", got, baseWire)
			}
		})
	}
}
