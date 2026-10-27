package omnibuf_test

import (
	"testing"

	flatbuffers "github.com/google/flatbuffers/go"
	. "github.com/sudorandom/omnibuf"
	"github.com/sudorandom/omnibuf/internal/pb"
	. "github.com/sudorandom/omnibuf/wire"
	"google.golang.org/protobuf/encoding/protowire"
)

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

func BenchmarkMarshal_Avro(b *testing.B) {
	orig := SamplePeerUpdate()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pkt, err := MarshalAvro(&orig)
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

func BenchmarkUnmarshal_Msgpack_Reuse(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt, _ := MarshalMsgpack(&orig)
	out := SamplePeerUpdate()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
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

func BenchmarkUnmarshal_JSON_Reuse(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt, _ := MarshalJSON(&orig)
	out := SamplePeerUpdate()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := UnmarshalJSON(pkt, &out); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal_Avro(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt, _ := MarshalAvro(&orig)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out PeerUpdate
		if err := UnmarshalAvro(pkt, &out); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal_Avro_Reuse(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt, _ := MarshalAvro(&orig)
	out := SamplePeerUpdate()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := UnmarshalAvro(pkt, &out); err != nil {
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

func BenchmarkLookup_LocationCity_Avro_SkipScan(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt, _ := MarshalAvro(&orig)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, ok := LookupAvroNestedCity(pkt)
		if !ok || len(s) != 9 {
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
		for range 16 {
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
		for range 16 {
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
		for range 16 {
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
		for range 16 {
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
		for range 16 {
			_, n := protowire.ConsumeVarint(buf[pos:])
			pos += n
		}
		sink += pos
	}
	if sink == 0 {
		b.Fatal("unreachable")
	}
}
