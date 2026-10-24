package omnibuf_test

import (
	"fmt"
	"math"
	"reflect"
	"testing"

	flatbuffers "github.com/google/flatbuffers/go"
	. "github.com/sudorandom/omnibuf"
	"github.com/sudorandom/omnibuf/pb"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

// SamplePBPeerUpdate converts a PeerUpdate into the generated google.golang.org/protobuf struct.
func SamplePBPeerUpdate(p *PeerUpdate) *pb.PeerUpdate {
	neighs := make([]*pb.Neighbor, len(p.Neighbors))
	for i := range p.Neighbors {
		n := &p.Neighbors[i]
		neighs[i] = &pb.Neighbor{
			PeerAsn:     n.PeerASN,
			Router:      n.Router,
			LatencyUs:   n.LatencyUS,
			Established: n.Established,
		}
	}
	return &pb.PeerUpdate{
		Asn:       p.ASN,
		RouterId:  p.RouterID,
		Active:    p.Active,
		UptimeSec: p.UptimeSec,
		RssiDbm:   p.RssiDbm,
		CpuLoad:   p.CPULoad,
		Tags:      append([]string(nil), p.Tags...),
		Prefixes:  append([]uint32(nil), p.Prefixes...),
		Location: &pb.Location{
			Region:     p.Location.Region,
			Datacenter: p.Location.Datacenter,
			City:       p.Location.City,
			Rack:       p.Location.Rack,
			Latitude:   p.Location.Latitude,
			Longitude:  p.Location.Longitude,
		},
		Neighbors: neighs,
		Sequence:  p.Sequence,
	}
}

func MarshalProtoOfficial(msg *pb.PeerUpdate) ([]byte, error) {
	return proto.Marshal(msg)
}

func MarshalAppendProtoOfficial(dst []byte, msg *pb.PeerUpdate) ([]byte, error) {
	return proto.MarshalOptions{}.MarshalAppend(dst[:0], msg)
}

func UnmarshalProtoOfficial(b []byte, out *pb.PeerUpdate) error {
	out.Reset()
	return proto.Unmarshal(b, out)
}

func TestPrefixVarintRoundTrip(t *testing.T) {
	cases := []struct {
		val     uint64
		wantLen int
	}{
		{0, 1},
		{1, 1},
		{127, 1},
		{128, 2},
		{16383, 2},
		{16384, 4},
		{536870911, 4},
		{536870912, 9},
		{math.MaxUint32, 9},
		{math.MaxUint64, 9},
	}

	var buf []byte
	for _, tc := range cases {
		buf = AppendVarint(buf[:0], tc.val)
		if len(buf) != tc.wantLen {
			t.Fatalf("AppendVarint(%d) len = %d, want %d", tc.val, len(buf), tc.wantLen)
		}
		got, n := DecodeVarint(buf)
		if n != tc.wantLen || got != tc.val {
			t.Fatalf("DecodeVarint(%d) = (%d, %d), want (%d, %d)", tc.val, got, n, tc.val, tc.wantLen)
		}
	}

	// Verify ZigZag encoding
	signedCases := []int64{0, -1, 1, -64, 63, -65, -100000, 100000, math.MinInt64, math.MaxInt64}
	for _, s := range signedCases {
		zz := EncodeZigZag(s)
		if s == -1 && zz != 1 {
			t.Fatalf("EncodeZigZag(-1) = %d, want 1", zz)
		}
		if got := DecodeZigZag(zz); got != s {
			t.Fatalf("DecodeZigZag(EncodeZigZag(%d)) = %d", s, got)
		}
	}
}

func TestSparseField0AndBlindDecode(t *testing.T) {
	enc := NewEncoder(128)
	// Write Field #1 ("asn"), Field #3 ("router"), and Field #5 ("active"), omitting Field #2 and #4
	enc.WriteField0Fields(map[uint32]string{1: "asn", 3: "router", 5: "active"})
	enc.WriteUint(1, 64512)
	enc.WriteString(3, "core-rtr-01")
	enc.WriteBool(5, true)
	pkt := enc.Finish()

	// Verify O(1) per-field header-hop lookup works alongside Field #0
	if asn, ok := ReadUint(pkt, 1); !ok || asn != 64512 {
		t.Fatalf("ReadUint(1) = %d, %v", asn, ok)
	}
	if _, _, ok := LocateField(pkt, 2); ok {
		t.Fatalf("LocateField(2) should be false for omitted field")
	}
	if rtr, ok := ReadString(pkt, 3); !ok || rtr != "core-rtr-01" {
		t.Fatalf("ReadString(3) = %q, %v", rtr, ok)
	}
	if act, ok := ReadBool(pkt, 5); !ok || !act {
		t.Fatalf("ReadBool(5) = %v, %v", act, ok)
	}

	// Verify schema-less blind decode
	blind, err := DecodeBlind(pkt)
	if err != nil {
		t.Fatalf("DecodeBlind failed: %v", err)
	}
	if blind["asn"] != uint64(64512) || blind["router"] != "core-rtr-01" || blind["active"] != true {
		t.Fatalf("unexpected blind decode output: %#v", blind)
	}
}

func TestWireMapLookup(t *testing.T) {
	enc := NewEncoder(256)
	m := map[string]uint64{
		"fra01": 100,
		"ams02": 200,
		"lhr03": 300,
		"iad04": 400,
		"nrt05": 500,
	}
	enc.WriteMapStringUint(1, m)
	pkt := enc.Finish()

	for k, want := range m {
		got, ok := LookupMapStringUint(pkt, 1, k)
		if !ok || got != want {
			t.Fatalf("LookupMapStringUint(%q) = (%d, %v), want (%d, true)", k, got, ok, want)
		}
	}
	if _, ok := LookupMapStringUint(pkt, 1, "missing"); ok {
		t.Fatalf("expected missing key to return false")
	}
}

func TestAllCodecsRoundTripAndWireSize(t *testing.T) {
	orig := SamplePeerUpdate()

	// 1. OmniBuf Shared-Schema
	enc := NewEncoder(512)
	omniBytes := append([]byte(nil), EncodePeerOmniBuf(enc, &orig, false)...)
	omniDirectBytes := EncodePeerOmniBufDirect(nil, &orig)
	if !reflect.DeepEqual(omniBytes, omniDirectBytes) {
		t.Fatalf("EncodePeerOmniBufDirect bytes mismatch:\ngot:  %x\nwant: %x", omniDirectBytes, omniBytes)
	}
	var omniDec PeerUpdate
	if err := DecodePeerOmniBufInto(omniBytes, &omniDec, false); err != nil {
		t.Fatalf("DecodePeerOmniBufInto error: %v", err)
	}
	if !reflect.DeepEqual(orig, omniDec) {
		t.Fatalf("OmniBuf mismatch:\ngot:  %#v\nwant: %#v", omniDec, orig)
	}

	// Verify O(1) per-field header-hop lookup on OmniBuf
	if city, ok := ReadNestedString(omniBytes, FieldPeerLocation, FieldLocCity); !ok || city != "Frankfurt" {
		t.Fatalf("ReadNestedString(location.city) = %q, %v", city, ok)
	}
	if rtr, ok := ReadArrNestedString(omniBytes, FieldPeerNeighbors, 2, FieldNeighRouter); !ok || rtr != "be2040.ccr41.fra03.atlas.cogentco.com" {
		t.Fatalf("ReadArrNestedString(neighbors[2].router) = %q, %v", rtr, ok)
	}
	if seq, ok := ReadUint(omniBytes, FieldPeerSequence); !ok || seq != orig.Sequence {
		t.Fatalf("ReadUint(sequence) = %d, %v", seq, ok)
	}

	// 2. OmniBuf Self-Describing (Field #0)
	omniSelfBytes := append([]byte(nil), EncodePeerOmniBuf(enc, &orig, true)...)
	blindMap, err := DecodeBlind(omniSelfBytes)
	if err != nil {
		t.Fatalf("DecodeBlind self-describing error: %v", err)
	}
	if gotRssi, ok := blindMap["rssi_dbm"].(int64); !ok || gotRssi != int64(orig.RssiDbm) {
		t.Fatalf("DecodeBlind rssi_dbm = (%T %v), want int64(%d)", blindMap["rssi_dbm"], blindMap["rssi_dbm"], orig.RssiDbm)
	}
	locMap := blindMap["location"].(map[string]any)
	if locMap["city"] != "Frankfurt" {
		t.Fatalf("DecodeBlind location.city = %v", locMap["city"])
	}

	// 3. Protobuf Official (google.golang.org/protobuf/proto) & Direct protowire
	pbMsg := SamplePBPeerUpdate(&orig)
	pbOfficialBytes, err := MarshalProtoOfficial(pbMsg)
	if err != nil {
		t.Fatalf("MarshalProtoOfficial error: %v", err)
	}
	var pbDec pb.PeerUpdate
	if err := UnmarshalProtoOfficial(pbOfficialBytes, &pbDec); err != nil {
		t.Fatalf("UnmarshalProtoOfficial error: %v", err)
	}
	if pbDec.Location.City != "Frankfurt" || pbDec.Neighbors[2].Router != orig.Neighbors[2].Router || pbDec.Sequence != orig.Sequence {
		t.Fatalf("Protobuf Official round-trip mismatch")
	}

	protoWireBytes := EncodePeerProtoWire(nil, &orig)
	var pwDec PeerUpdate
	if err := DecodePeerProtoWireInto(protoWireBytes, &pwDec, false); err != nil {
		t.Fatalf("DecodePeerProtoWireInto error: %v", err)
	}
	if !reflect.DeepEqual(orig, pwDec) {
		t.Fatalf("ProtoWire mismatch:\ngot:  %#v\nwant: %#v", pwDec, orig)
	}
	if city, ok := LookupProtoWireNestedCity(protoWireBytes, false); !ok || city != "Frankfurt" {
		t.Fatalf("LookupProtoWireNestedCity = %q, %v", city, ok)
	}
	if rtr, ok := LookupProtoWireNeighborRouter(protoWireBytes, 2, false); !ok || rtr != orig.Neighbors[2].Router {
		t.Fatalf("LookupProtoWireNeighborRouter = %q, %v", rtr, ok)
	}

	// 4. FlatBuffers
	fbBuilder := flatbuffers.NewBuilder(512)
	fbBytes := append([]byte(nil), EncodePeerFlatBuffers(fbBuilder, &orig)...)
	var fbDec PeerUpdate
	DecodePeerFlatBuffersInto(fbBytes, &fbDec, false)
	if !reflect.DeepEqual(orig, fbDec) {
		t.Fatalf("FlatBuffers mismatch:\ngot:  %#v\nwant: %#v", fbDec, orig)
	}
	if city, ok := LookupFlatBuffersNestedCity(fbBytes, false); !ok || city != "Frankfurt" {
		t.Fatalf("LookupFlatBuffersNestedCity = %q, %v", city, ok)
	}
	if rtr, ok := LookupFlatBuffersNeighborRouter(fbBytes, 2, false); !ok || rtr != orig.Neighbors[2].Router {
		t.Fatalf("LookupFlatBuffersNeighborRouter = %q, %v", rtr, ok)
	}

	// 5. Cap'n Proto
	capnpBytes, err := EncodePeerCapnp(nil, &orig)
	if err != nil {
		t.Fatalf("EncodePeerCapnp error: %v", err)
	}
	var capnpDec PeerUpdate
	if err := DecodePeerCapnpInto(capnpBytes, &capnpDec, false); err != nil {
		t.Fatalf("DecodePeerCapnpInto error: %v", err)
	}
	if !reflect.DeepEqual(orig, capnpDec) {
		t.Fatalf("Cap'n Proto mismatch:\ngot:  %#v\nwant: %#v", capnpDec, orig)
	}
	if city, ok := LookupCapnpNestedCity(capnpBytes, false); !ok || city != "Frankfurt" {
		t.Fatalf("LookupCapnpNestedCity = %q, %v", city, ok)
	}
	if city, ok := LookupCapnpInPlaceNestedCity(capnpBytes, false); !ok || city != "Frankfurt" {
		t.Fatalf("LookupCapnpInPlaceNestedCity = %q, %v", city, ok)
	}
	if rtr, ok := LookupCapnpNeighborRouter(capnpBytes, 2, false); !ok || rtr != orig.Neighbors[2].Router {
		t.Fatalf("LookupCapnpNeighborRouter = %q, %v", rtr, ok)
	}
	if rtr, ok := LookupCapnpInPlaceNeighborRouter(capnpBytes, 2, false); !ok || rtr != orig.Neighbors[2].Router {
		t.Fatalf("LookupCapnpInPlaceNeighborRouter = %q, %v", rtr, ok)
	}

	// 6. MessagePack
	mpBytes, err := MarshalMsgpack(&orig)
	if err != nil {
		t.Fatalf("MarshalMsgpack error: %v", err)
	}
	var mpDec PeerUpdate
	if err := UnmarshalMsgpack(mpBytes, &mpDec); err != nil {
		t.Fatalf("UnmarshalMsgpack error: %v", err)
	}
	if !reflect.DeepEqual(orig, mpDec) {
		t.Fatalf("Msgpack mismatch:\ngot:  %#v\nwant: %#v", mpDec, orig)
	}
	if city, ok := LookupMsgpackNestedCity(mpBytes); !ok || city != "Frankfurt" {
		t.Fatalf("LookupMsgpackNestedCity = %q, %v", city, ok)
	}

	// 7. JSON
	jsonBytes, err := MarshalJSON(&orig)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}
	var jsonDec PeerUpdate
	if err := UnmarshalJSON(jsonBytes, &jsonDec); err != nil {
		t.Fatalf("UnmarshalJSON error: %v", err)
	}
	if !reflect.DeepEqual(orig, jsonDec) {
		t.Fatalf("JSON mismatch:\ngot:  %#v\nwant: %#v", jsonDec, orig)
	}

	t.Logf("\n=== WIRE SIZE COMPARISON (PeerUpdate Payload) ===")
	t.Logf("  Protobuf (proto.Marshal / protowire): %4d bytes", len(pbOfficialBytes))
	t.Logf("  OmniBuf (Shared-Schema):              %4d bytes (%+d B vs Protobuf)", len(omniBytes), len(omniBytes)-len(pbOfficialBytes))
	t.Logf("  OmniBuf (Self-Describing Field #0):   %4d bytes (includes all field names inline)", len(omniSelfBytes))
	t.Logf("  MessagePack (Self-Describing Map):    %4d bytes", len(mpBytes))
	t.Logf("  FlatBuffers (VTables + Alignment):    %4d bytes", len(fbBytes))
	t.Logf("  Cap'n Proto (64-bit Words + Ptrs):    %4d bytes", len(capnpBytes))
	t.Logf("  Standard JSON (encoding/json):        %4d bytes", len(jsonBytes))
	fmt.Printf("")
}

// ============================================================================
// BENCHMARKS: 1. Marshal / Encode
// ============================================================================

func BenchmarkMarshal_OmniBuf_Direct_ReuseBuf(b *testing.B) {
	orig := SamplePeerUpdate()
	buf := make([]byte, 0, 512)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf = EncodePeerOmniBufDirect(buf, &orig)
		if len(buf) == 0 {
			b.Fatal("empty")
		}
	}
}

func BenchmarkMarshal_OmniBuf_ReuseBuf(b *testing.B) {
	orig := SamplePeerUpdate()
	enc := NewEncoder(512)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pkt := EncodePeerOmniBuf(enc, &orig, false)
		if len(pkt) == 0 {
			b.Fatal("empty")
		}
	}
}

func BenchmarkMarshal_OmniBuf_FreshAlloc(b *testing.B) {
	orig := SamplePeerUpdate()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc := NewEncoder(384)
		pkt := EncodePeerOmniBuf(enc, &orig, false)
		if len(pkt) == 0 {
			b.Fatal("empty")
		}
	}
}

func BenchmarkMarshal_OmniBuf_SelfDescribing_ReuseBuf(b *testing.B) {
	orig := SamplePeerUpdate()
	enc := NewEncoder(640)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pkt := EncodePeerOmniBuf(enc, &orig, true)
		if len(pkt) == 0 {
			b.Fatal("empty")
		}
	}
}

func BenchmarkMarshal_Protobuf_ProtoWire_ReuseBuf(b *testing.B) {
	orig := SamplePeerUpdate()
	buf := make([]byte, 0, 512)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf = EncodePeerProtoWire(buf, &orig)
		if len(buf) == 0 {
			b.Fatal("empty")
		}
	}
}

func BenchmarkMarshal_Protobuf_Official_MarshalAppend(b *testing.B) {
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)
	buf := make([]byte, 0, 512)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		buf, err = MarshalAppendProtoOfficial(buf, pbMsg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshal_Protobuf_Official_FreshAlloc(b *testing.B) {
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pkt, err := MarshalProtoOfficial(pbMsg)
		if err != nil || len(pkt) == 0 {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshal_FlatBuffers_ReuseBuilder(b *testing.B) {
	orig := SamplePeerUpdate()
	builder := flatbuffers.NewBuilder(1024)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pkt := EncodePeerFlatBuffers(builder, &orig)
		if len(pkt) == 0 {
			b.Fatal("empty")
		}
	}
}

func BenchmarkMarshal_CapnProto(b *testing.B) {
	orig := SamplePeerUpdate()
	scratch := make([]byte, 0, 1024)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pkt, err := EncodePeerCapnp(scratch, &orig)
		if err != nil || len(pkt) == 0 {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshal_Msgpack(b *testing.B) {
	orig := SamplePeerUpdate()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pkt, err := MarshalMsgpack(&orig)
		if err != nil || len(pkt) == 0 {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshal_JSON(b *testing.B) {
	orig := SamplePeerUpdate()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pkt, err := MarshalJSON(&orig)
		if err != nil || len(pkt) == 0 {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// BENCHMARKS: 2. Full Unmarshal / Decode
// ============================================================================

func BenchmarkUnmarshal_OmniBuf_ZeroCopy(b *testing.B) {
	orig := SamplePeerUpdate()
	enc := NewEncoder(512)
	pkt := append([]byte(nil), EncodePeerOmniBuf(enc, &orig, false)...)
	var out PeerUpdate
	out.Tags = make([]string, 0, 8)
	out.Prefixes = make([]uint32, 0, 8)
	out.Neighbors = make([]Neighbor, 0, 8)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := DecodePeerOmniBufInto(pkt, &out, true); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal_OmniBuf_SafeAlloc(b *testing.B) {
	orig := SamplePeerUpdate()
	enc := NewEncoder(512)
	pkt := append([]byte(nil), EncodePeerOmniBuf(enc, &orig, false)...)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out PeerUpdate
		if err := DecodePeerOmniBufInto(pkt, &out, false); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal_Protobuf_ProtoWire_ZeroCopy(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt := EncodePeerProtoWire(nil, &orig)
	var out PeerUpdate
	out.Tags = make([]string, 0, 8)
	out.Prefixes = make([]uint32, 0, 8)
	out.Neighbors = make([]Neighbor, 0, 8)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := DecodePeerProtoWireInto(pkt, &out, true); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal_Protobuf_ProtoWire_SafeAlloc(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt := EncodePeerProtoWire(nil, &orig)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out PeerUpdate
		if err := DecodePeerProtoWireInto(pkt, &out, false); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal_Protobuf_Official(b *testing.B) {
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)
	pkt, _ := MarshalProtoOfficial(pbMsg)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out pb.PeerUpdate
		if err := UnmarshalProtoOfficial(pkt, &out); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal_FlatBuffers_ZeroCopy(b *testing.B) {
	orig := SamplePeerUpdate()
	builder := flatbuffers.NewBuilder(1024)
	pkt := append([]byte(nil), EncodePeerFlatBuffers(builder, &orig)...)
	var out PeerUpdate
	out.Tags = make([]string, 0, 8)
	out.Prefixes = make([]uint32, 0, 8)
	out.Neighbors = make([]Neighbor, 0, 8)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DecodePeerFlatBuffersInto(pkt, &out, true)
	}
}

func BenchmarkUnmarshal_FlatBuffers_SafeAlloc(b *testing.B) {
	orig := SamplePeerUpdate()
	builder := flatbuffers.NewBuilder(1024)
	pkt := append([]byte(nil), EncodePeerFlatBuffers(builder, &orig)...)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out PeerUpdate
		DecodePeerFlatBuffersInto(pkt, &out, false)
	}
}

func BenchmarkUnmarshal_CapnProto_ZeroCopy(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt, _ := EncodePeerCapnp(nil, &orig)
	var out PeerUpdate
	out.Tags = make([]string, 0, 8)
	out.Prefixes = make([]uint32, 0, 8)
	out.Neighbors = make([]Neighbor, 0, 8)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := DecodePeerCapnpInto(pkt, &out, true); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal_CapnProto_SafeAlloc(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt, _ := EncodePeerCapnp(nil, &orig)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out PeerUpdate
		if err := DecodePeerCapnpInto(pkt, &out, false); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal_Msgpack(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt, _ := MarshalMsgpack(&orig)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out PeerUpdate
		if err := UnmarshalMsgpack(pkt, &out); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal_JSON(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt, _ := MarshalJSON(&orig)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out PeerUpdate
		if err := UnmarshalJSON(pkt, &out); err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// BENCHMARKS: 3. Single Nested Field Lookup (peer.location.city & peer.neighbors[2].router)
// ============================================================================

func BenchmarkLookup_LocationCity_OmniBuf_ZeroCopy(b *testing.B) {
	orig := SamplePeerUpdate()
	enc := NewEncoder(512)
	pkt := append([]byte(nil), EncodePeerOmniBuf(enc, &orig, false)...)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, ok := ReadNestedStringZeroCopy(pkt, FieldPeerLocation, FieldLocCity)
		if !ok || len(s) != 9 {
			b.Fatal("lookup failed")
		}
	}
}

func BenchmarkLookup_LocationCity_OmniBuf_SafeAlloc(b *testing.B) {
	orig := SamplePeerUpdate()
	enc := NewEncoder(512)
	pkt := append([]byte(nil), EncodePeerOmniBuf(enc, &orig, false)...)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, ok := ReadNestedString(pkt, FieldPeerLocation, FieldLocCity)
		if !ok || len(s) != 9 {
			b.Fatal("lookup failed")
		}
	}
}

func BenchmarkLookup_LocationCity_FlatBuffers_ZeroCopy(b *testing.B) {
	orig := SamplePeerUpdate()
	builder := flatbuffers.NewBuilder(1024)
	pkt := append([]byte(nil), EncodePeerFlatBuffers(builder, &orig)...)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, ok := LookupFlatBuffersNestedCity(pkt, true)
		if !ok || len(s) != 9 {
			b.Fatal("lookup failed")
		}
	}
}

func BenchmarkLookup_LocationCity_CapnProto_InPlace_ZeroCopy(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt, _ := EncodePeerCapnp(nil, &orig)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, ok := LookupCapnpInPlaceNestedCity(pkt, true)
		if !ok || len(s) != 9 {
			b.Fatal("lookup failed")
		}
	}
}

func BenchmarkLookup_LocationCity_CapnProto_ZeroCopy(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt, _ := EncodePeerCapnp(nil, &orig)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, ok := LookupCapnpNestedCity(pkt, true)
		if !ok || len(s) != 9 {
			b.Fatal("lookup failed")
		}
	}
}

func BenchmarkLookup_LocationCity_Protobuf_WireScan_ZeroCopy(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt := EncodePeerProtoWire(nil, &orig)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, ok := LookupProtoWireNestedCity(pkt, true)
		if !ok || len(s) != 9 {
			b.Fatal("lookup failed")
		}
	}
}

func BenchmarkLookup_LocationCity_Protobuf_Official_Unmarshal(b *testing.B) {
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)
	pkt, _ := MarshalProtoOfficial(pbMsg)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out pb.PeerUpdate
		if err := UnmarshalProtoOfficial(pkt, &out); err != nil || len(out.Location.City) != 9 {
			b.Fatal("lookup failed")
		}
	}
}

func BenchmarkLookup_LocationCity_Msgpack_SkipScan(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt, _ := MarshalMsgpack(&orig)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, ok := LookupMsgpackNestedCity(pkt)
		if !ok || len(s) != 9 {
			b.Fatal("lookup failed")
		}
	}
}

func BenchmarkLookup_LocationCity_JSON_Unmarshal(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt, _ := MarshalJSON(&orig)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out PeerUpdate
		if err := UnmarshalJSON(pkt, &out); err != nil || len(out.Location.City) != 9 {
			b.Fatal("lookup failed")
		}
	}
}

// Indexed repeated sub-message lookup: peer.neighbors[2].router

func BenchmarkLookup_Neighbor2Router_OmniBuf_ZeroCopy(b *testing.B) {
	orig := SamplePeerUpdate()
	enc := NewEncoder(512)
	pkt := append([]byte(nil), EncodePeerOmniBuf(enc, &orig, false)...)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, ok := ReadArrNestedStringZeroCopy(pkt, FieldPeerNeighbors, 2, FieldNeighRouter)
		if !ok || len(s) == 0 {
			b.Fatal("lookup failed")
		}
	}
}

func BenchmarkLookup_Neighbor2Router_FlatBuffers_ZeroCopy(b *testing.B) {
	orig := SamplePeerUpdate()
	builder := flatbuffers.NewBuilder(1024)
	pkt := append([]byte(nil), EncodePeerFlatBuffers(builder, &orig)...)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, ok := LookupFlatBuffersNeighborRouter(pkt, 2, true)
		if !ok || len(s) == 0 {
			b.Fatal("lookup failed")
		}
	}
}

func BenchmarkLookup_Neighbor2Router_CapnProto_InPlace_ZeroCopy(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt, _ := EncodePeerCapnp(nil, &orig)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, ok := LookupCapnpInPlaceNeighborRouter(pkt, 2, true)
		if !ok || len(s) == 0 {
			b.Fatal("lookup failed")
		}
	}
}

func BenchmarkLookup_Neighbor2Router_CapnProto_ZeroCopy(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt, _ := EncodePeerCapnp(nil, &orig)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, ok := LookupCapnpNeighborRouter(pkt, 2, true)
		if !ok || len(s) == 0 {
			b.Fatal("lookup failed")
		}
	}
}

func BenchmarkLookup_Neighbor2Router_Protobuf_WireScan_ZeroCopy(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt := EncodePeerProtoWire(nil, &orig)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, ok := LookupProtoWireNeighborRouter(pkt, 2, true)
		if !ok || len(s) == 0 {
			b.Fatal("lookup failed")
		}
	}
}

// Schema-less Blind Decode (OmniBuf Field #0 vs JSON vs Msgpack map[string]any)

func BenchmarkDecodeBlind_OmniBuf_Field0(b *testing.B) {
	orig := SamplePeerUpdate()
	enc := NewEncoder(640)
	pkt := append([]byte(nil), EncodePeerOmniBuf(enc, &orig, true)...)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m, err := DecodeBlind(pkt)
		if err != nil || len(m) == 0 {
			b.Fatal(err)
		}
	}
}

var sampleVarintValues = [16]uint64{
	// 4x 1-byte values (< 128)
	1, 17, 42, 120,
	// 4x 2-byte values (128 .. 16,383)
	300, 1420, 8192, 15000,
	// 4x 4-byte values (16,384 .. 536,870,911)
	64512, 1240500, 34210000, 480000000,
	// 4x 64-bit values (> 536,870,911: timestamps, sequence IDs, trace IDs)
	1711324800123456789, 987654321012345678, 1844674407370955160, 1407374883553289999,
}

func BenchmarkVarint_Decode_1B_OmniBuf_TZCNT(b *testing.B) {
	buf := AppendVarint(nil, 42)
	b.ResetTimer()
	var sink uint64
	for i := 0; i < b.N; i++ {
		v, _ := ReadVarintAt(buf, 0)
		sink += v
	}
	if sink == 0 {
		b.Fatal("unreachable")
	}
}

func BenchmarkVarint_Decode_1B_Protobuf_Protowire(b *testing.B) {
	buf := protowire.AppendVarint(nil, 42)
	b.ResetTimer()
	var sink uint64
	for i := 0; i < b.N; i++ {
		v, _ := protowire.ConsumeVarint(buf)
		sink += v
	}
	if sink == 0 {
		b.Fatal("unreachable")
	}
}

func BenchmarkVarint_Decode_1B_VTProtobuf_Loop(b *testing.B) {
	buf := protowire.AppendVarint(nil, 42)
	b.ResetTimer()
	var sink uint64
	for i := 0; i < b.N; i++ {
		var v uint64
		pos := 0
		for shift := uint(0); ; shift += 7 {
			b0 := buf[pos]
			pos++
			v |= uint64(b0&0x7f) << shift
			if b0 < 0x80 {
				break
			}
		}
		sink += v
	}
	if sink == 0 {
		b.Fatal("unreachable")
	}
}

func BenchmarkVarint_Decode_2B_OmniBuf_TZCNT(b *testing.B) {
	buf := AppendVarint(nil, 8192)
	b.ResetTimer()
	var sink uint64
	for i := 0; i < b.N; i++ {
		v, _ := ReadVarintAt(buf, 0)
		sink += v
	}
	if sink == 0 {
		b.Fatal("unreachable")
	}
}

func BenchmarkVarint_Decode_2B_Protobuf_Protowire(b *testing.B) {
	buf := protowire.AppendVarint(nil, 8192)
	b.ResetTimer()
	var sink uint64
	for i := 0; i < b.N; i++ {
		v, _ := protowire.ConsumeVarint(buf)
		sink += v
	}
	if sink == 0 {
		b.Fatal("unreachable")
	}
}

func BenchmarkVarint_Decode_2B_VTProtobuf_Loop(b *testing.B) {
	buf := protowire.AppendVarint(nil, 8192)
	b.ResetTimer()
	var sink uint64
	for i := 0; i < b.N; i++ {
		var v uint64
		pos := 0
		for shift := uint(0); ; shift += 7 {
			b0 := buf[pos]
			pos++
			v |= uint64(b0&0x7f) << shift
			if b0 < 0x80 {
				break
			}
		}
		sink += v
	}
	if sink == 0 {
		b.Fatal("unreachable")
	}
}

func BenchmarkVarint_Decode_4B_OmniBuf_TZCNT(b *testing.B) {
	buf := AppendVarint(nil, 34210000)
	b.ResetTimer()
	var sink uint64
	for i := 0; i < b.N; i++ {
		v, _ := ReadVarintAt(buf, 0)
		sink += v
	}
	if sink == 0 {
		b.Fatal("unreachable")
	}
}

func BenchmarkVarint_Decode_4B_Protobuf_Protowire(b *testing.B) {
	buf := protowire.AppendVarint(nil, 34210000)
	b.ResetTimer()
	var sink uint64
	for i := 0; i < b.N; i++ {
		v, _ := protowire.ConsumeVarint(buf)
		sink += v
	}
	if sink == 0 {
		b.Fatal("unreachable")
	}
}

func BenchmarkVarint_Decode_4B_VTProtobuf_Loop(b *testing.B) {
	buf := protowire.AppendVarint(nil, 34210000)
	b.ResetTimer()
	var sink uint64
	for i := 0; i < b.N; i++ {
		var v uint64
		pos := 0
		for shift := uint(0); ; shift += 7 {
			b0 := buf[pos]
			pos++
			v |= uint64(b0&0x7f) << shift
			if b0 < 0x80 {
				break
			}
		}
		sink += v
	}
	if sink == 0 {
		b.Fatal("unreachable")
	}
}

func BenchmarkVarint_Decode_64b_OmniBuf_TZCNT(b *testing.B) {
	buf := AppendVarint(nil, 1711324800123456789)
	b.ResetTimer()
	var sink uint64
	for i := 0; i < b.N; i++ {
		v, _ := ReadVarintAt(buf, 0)
		sink += v
	}
	if sink == 0 {
		b.Fatal("unreachable")
	}
}

func BenchmarkVarint_Decode_64b_Protobuf_Protowire(b *testing.B) {
	buf := protowire.AppendVarint(nil, 1711324800123456789)
	b.ResetTimer()
	var sink uint64
	for i := 0; i < b.N; i++ {
		v, _ := protowire.ConsumeVarint(buf)
		sink += v
	}
	if sink == 0 {
		b.Fatal("unreachable")
	}
}

func BenchmarkVarint_Decode_64b_VTProtobuf_Loop(b *testing.B) {
	buf := protowire.AppendVarint(nil, 1711324800123456789)
	b.ResetTimer()
	var sink uint64
	for i := 0; i < b.N; i++ {
		var v uint64
		pos := 0
		for shift := uint(0); ; shift += 7 {
			b0 := buf[pos]
			pos++
			v |= uint64(b0&0x7f) << shift
			if b0 < 0x80 {
				break
			}
		}
		sink += v
	}
	if sink == 0 {
		b.Fatal("unreachable")
	}
}

func BenchmarkVarint_Decode_16Mixed_OmniBuf_TZCNT(b *testing.B) {
	var buf []byte
	for _, v := range sampleVarintValues {
		buf = AppendVarint(buf, v)
	}
	b.ResetTimer()
	var sink uint64
	for i := 0; i < b.N; i++ {
		pos := 0
		for j := 0; j < 16; j++ {
			var v uint64
			v, pos = ReadVarintAt(buf, pos)
			sink += v
		}
	}
	if sink == 0 {
		b.Fatal("unreachable")
	}
}

func BenchmarkVarint_Decode_16Mixed_Protobuf_Protowire(b *testing.B) {
	var buf []byte
	for _, v := range sampleVarintValues {
		buf = protowire.AppendVarint(buf, v)
	}
	b.ResetTimer()
	var sink uint64
	for i := 0; i < b.N; i++ {
		pos := 0
		for j := 0; j < 16; j++ {
			v, n := protowire.ConsumeVarint(buf[pos:])
			pos += n
			sink += v
		}
	}
	if sink == 0 {
		b.Fatal("unreachable")
	}
}

func BenchmarkVarint_Decode_16Mixed_VTProtobuf_InlinedLoop(b *testing.B) {
	var buf []byte
	for _, v := range sampleVarintValues {
		buf = protowire.AppendVarint(buf, v)
	}
	b.ResetTimer()
	var sink uint64
	for i := 0; i < b.N; i++ {
		pos := 0
		for j := 0; j < 16; j++ {
			var v uint64
			for shift := uint(0); ; shift += 7 {
				b0 := buf[pos]
				pos++
				v |= uint64(b0&0x7f) << shift
				if b0 < 0x80 {
					break
				}
			}
			sink += v
		}
	}
	if sink == 0 {
		b.Fatal("unreachable")
	}
}

func BenchmarkVarint_Encode_16Mixed_OmniBuf_TZCNT(b *testing.B) {
	buf := make([]byte, 0, 128)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf = buf[:0]
		for _, v := range sampleVarintValues {
			buf = AppendVarint(buf, v)
		}
	}
	if len(buf) == 0 {
		b.Fatal("unreachable")
	}
}

func BenchmarkVarint_Encode_16Mixed_Protobuf_Protowire(b *testing.B) {
	buf := make([]byte, 0, 128)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf = buf[:0]
		for _, v := range sampleVarintValues {
			buf = protowire.AppendVarint(buf, v)
		}
	}
	if len(buf) == 0 {
		b.Fatal("unreachable")
	}
}

func BenchmarkVarint_Skip_16Mixed_OmniBuf_TZCNT(b *testing.B) {
	var buf []byte
	for _, v := range sampleVarintValues {
		buf = AppendVarint(buf, v)
	}
	b.ResetTimer()
	var sink int
	for i := 0; i < b.N; i++ {
		pos := 0
		for j := 0; j < 16; j++ {
			b0 := buf[pos]
			if (b0 & 1) != 0 {
				pos += 1
			} else if (b0 & 2) != 0 {
				pos += 2
			} else if (b0 & 4) != 0 {
				pos += 4
			} else {
				pos += 9
			}
		}
		sink += pos
	}
	if sink == 0 {
		b.Fatal("unreachable")
	}
}

func BenchmarkVarint_Skip_16Mixed_Protobuf_Protowire(b *testing.B) {
	var buf []byte
	for _, v := range sampleVarintValues {
		buf = protowire.AppendVarint(buf, v)
	}
	b.ResetTimer()
	var sink int
	for i := 0; i < b.N; i++ {
		pos := 0
		for j := 0; j < 16; j++ {
			_, n := protowire.ConsumeVarint(buf[pos:])
			pos += n
		}
		sink += pos
	}
	if sink == 0 {
		b.Fatal("unreachable")
	}
}
