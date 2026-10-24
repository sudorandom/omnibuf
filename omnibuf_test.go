package omnibuf_test

import (
	"fmt"
	"math"
	"reflect"
	"testing"

	flatbuffers "github.com/google/flatbuffers/go"
	. "github.com/sudorandom/omnibuf"
	"github.com/sudorandom/omnibuf/pb"
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
