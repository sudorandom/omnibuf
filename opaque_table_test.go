package omnibuf_test

import (
	"reflect"
	"testing"

	. "github.com/sudorandom/omnibuf"
	"github.com/sudorandom/omnibuf/pb"
	"google.golang.org/protobuf/proto"
)

func TestGenericTableMarshalUnmarshal_ExistingPBStruct(t *testing.T) {
	orig := SamplePeerUpdate()
	pbIn := SamplePBPeerUpdate(&orig)

	// 1. Marshal existing *pb.PeerUpdate (standard protoc-gen-go struct) using omnibuf.Marshal!
	pkt, err := Marshal(pbIn)
	if err != nil {
		t.Fatalf("omnibuf.Marshal(*pb.PeerUpdate) failed: %v", err)
	}

	// Verify byte-for-byte identity with hand-written OmniBuf encoder
	wantBytes := EncodePeerOmniBufDirect(nil, &orig)
	if !reflect.DeepEqual(pkt, wantBytes) {
		t.Fatalf("table-driven Marshal(*pb.PeerUpdate) bytes mismatch:\ngot  (%dB): %x\nwant (%dB): %x", len(pkt), pkt, len(wantBytes), wantBytes)
	}

	// 2. Unmarshal back into a fresh *pb.PeerUpdate (standard protoc-gen-go struct)
	var pbOut pb.PeerUpdate
	if err := Unmarshal(pkt, &pbOut); err != nil {
		t.Fatalf("omnibuf.Unmarshal(*pb.PeerUpdate) failed: %v", err)
	}
	if pbOut.Asn != pbIn.Asn ||
		pbOut.RouterId != pbIn.RouterId ||
		pbOut.Active != pbIn.Active ||
		pbOut.UptimeSec != pbIn.UptimeSec ||
		pbOut.RssiDbm != pbIn.RssiDbm ||
		pbOut.CpuLoad != pbIn.CpuLoad ||
		!reflect.DeepEqual(pbOut.Tags, pbIn.Tags) ||
		!reflect.DeepEqual(pbOut.Prefixes, pbIn.Prefixes) ||
		pbOut.Location.City != pbIn.Location.City ||
		len(pbOut.Neighbors) != len(pbIn.Neighbors) ||
		pbOut.Neighbors[2].Router != pbIn.Neighbors[2].Router ||
		pbOut.Sequence != pbIn.Sequence {
		t.Fatalf("pbOut mismatch:\ngot:  %#v\nwant: %#v", &pbOut, pbIn)
	}

	// 3. Also test table-driven Marshal/Unmarshal on value-struct *PeerUpdate
	valPkt, err := Marshal(&orig)
	if err != nil {
		t.Fatalf("omnibuf.Marshal(*PeerUpdate) failed: %v", err)
	}
	if !reflect.DeepEqual(valPkt, wantBytes) {
		t.Fatalf("table-driven Marshal(*PeerUpdate) bytes mismatch")
	}
	var valOut PeerUpdate
	if err := Unmarshal(valPkt, &valOut); err != nil {
		t.Fatalf("omnibuf.Unmarshal(*PeerUpdate) failed: %v", err)
	}
	if !reflect.DeepEqual(valOut, orig) {
		t.Fatalf("valOut mismatch:\ngot:  %#v\nwant: %#v", valOut, orig)
	}
}

func TestOpaqueAPILazyUnmarshalAndSpliceMarshal(t *testing.T) {
	orig := SamplePeerUpdate()
	pkt := EncodePeerOmniBufDirect(nil, &orig)

	// 1. Lazy Unmarshal + O(1) Getters
	var op OpaquePeerUpdate
	if err := op.Unmarshal(pkt); err != nil {
		t.Fatalf("OpaquePeerUpdate.Unmarshal failed: %v", err)
	}
	if got := op.GetASN(); got != orig.ASN {
		t.Fatalf("GetASN() = %d, want %d", got, orig.ASN)
	}
	if got := op.GetRouterID(); got != orig.RouterID {
		t.Fatalf("GetRouterID() = %q, want %q", got, orig.RouterID)
	}
	if got := op.GetLocation().GetCity(); got != "Frankfurt" {
		t.Fatalf("GetLocation().GetCity() = %q, want Frankfurt", got)
	}
	if got := op.NeighborsLen(); got != 4 {
		t.Fatalf("NeighborsLen() = %d, want 4", got)
	}
	if got := op.GetNeighbors(2).GetRouter(); got != orig.Neighbors[2].Router {
		t.Fatalf("GetNeighbors(2).GetRouter() = %q, want %q", got, orig.Neighbors[2].Router)
	}
	if got := op.GetSequence(); got != orig.Sequence {
		t.Fatalf("GetSequence() = %d, want %d", got, orig.Sequence)
	}

	// 2. Read-Modify-Write Splice: mutate ASN and Sequence without decoding Location or Neighbors!
	op.SetASN(65530)
	op.SetSequence(orig.Sequence + 1)
	splicedPkt := op.MarshalAppend(nil)

	// Verify the spliced packet decodes cleanly and preserves all untouched fields + updated fields
	var decoded PeerUpdate
	if err := DecodePeerOmniBufInto(splicedPkt, &decoded, false); err != nil {
		t.Fatalf("DecodePeerOmniBufInto(splicedPkt) failed: %v", err)
	}
	want := orig
	want.ASN = 65530
	want.Sequence = orig.Sequence + 1
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("spliced packet mismatch:\ngot:  %#v\nwant: %#v", decoded, want)
	}

	// 3. Fresh build via Opaque setters
	var fresh OpaquePeerUpdate
	fresh.PopulateFromPeerUpdate(&orig)
	freshBytes := fresh.MarshalAppend(nil)
	if !reflect.DeepEqual(freshBytes, pkt) {
		t.Fatalf("Opaque fresh MarshalAppend mismatch:\ngot  (%dB): %x\nwant (%dB): %x", len(freshBytes), freshBytes, len(pkt), pkt)
	}
}

func TestPluginGeneratedPBCodecsAndOpaqueAPI(t *testing.T) {
	orig := SamplePeerUpdate()
	pbIn := SamplePBPeerUpdate(&orig)

	// 1. Test plugin-generated (*pb.PeerUpdate).MarshalOmniBufAppend
	pluginBytes := pbIn.MarshalOmniBufAppend(nil)
	wantBytes := EncodePeerOmniBufDirect(nil, &orig)
	if !reflect.DeepEqual(pluginBytes, wantBytes) {
		t.Fatalf("plugin MarshalOmniBufAppend mismatch:\ngot  (%dB): %x\nwant (%dB): %x", len(pluginBytes), pluginBytes, len(wantBytes), wantBytes)
	}

	// 2. Test plugin-generated (*pb.PeerUpdate).UnmarshalOmniBuf
	var pbOut pb.PeerUpdate
	if err := pbOut.UnmarshalOmniBuf(pluginBytes, false); err != nil {
		t.Fatalf("plugin UnmarshalOmniBuf failed: %v", err)
	}
	if pbOut.Asn != pbIn.Asn ||
		pbOut.RouterId != pbIn.RouterId ||
		pbOut.Location.City != "Frankfurt" ||
		pbOut.Neighbors[2].Router != pbIn.Neighbors[2].Router ||
		pbOut.Sequence != pbIn.Sequence {
		t.Fatalf("plugin UnmarshalOmniBuf mismatch: %#v", &pbOut)
	}

	// 3. Test plugin-generated pb.OpaquePeerUpdate lazy getters & splice re-marshal
	var op pb.OpaquePeerUpdate
	if err := op.Unmarshal(pluginBytes); err != nil {
		t.Fatalf("pb.OpaquePeerUpdate.Unmarshal failed: %v", err)
	}
	if got := op.GetLocation().GetCity(); got != "Frankfurt" {
		t.Fatalf("plugin Opaque GetLocation().GetCity() = %q, want Frankfurt", got)
	}
	if got := op.GetNeighbors(2).GetRouter(); got != pbIn.Neighbors[2].Router {
		t.Fatalf("plugin Opaque GetNeighbors(2).GetRouter() = %q", got)
	}
	op.SetSequence(pbIn.Sequence + 1)
	spliced := op.MarshalAppend(nil)
	var pbSpliced pb.PeerUpdate
	if err := pbSpliced.UnmarshalOmniBuf(spliced, false); err != nil {
		t.Fatalf("UnmarshalOmniBuf(spliced) failed: %v", err)
	}
	if pbSpliced.Sequence != pbIn.Sequence+1 || pbSpliced.Location.City != "Frankfurt" {
		t.Fatalf("plugin Opaque spliced packet mismatch: %#v", &pbSpliced)
	}

	// 4. Test vtprotobuf round-trip (MarshalVT / UnmarshalVT / UnmarshalVTUnsafe)
	vtBytes, err := pbIn.MarshalVT()
	if err != nil {
		t.Fatalf("MarshalVT failed: %v", err)
	}
	var vtOut pb.PeerUpdate
	if err := vtOut.UnmarshalVT(vtBytes); err != nil {
		t.Fatalf("UnmarshalVT failed: %v", err)
	}
	if vtOut.Location.City != "Frankfurt" || vtOut.Neighbors[2].Router != pbIn.Neighbors[2].Router || vtOut.Sequence != pbIn.Sequence {
		t.Fatalf("vtprotobuf round-trip mismatch: %#v", &vtOut)
	}
}

// ============================================================================
// BENCHMARKS: Table-Driven Struct Marshal/Unmarshal & Opaque API Deferred/Splice
// ============================================================================

// 0. Plugin-Generated Benchmarks (pb/peer_omnibuf.pb.go generated by cmd/protoc-gen-omnibuf)

func BenchmarkPlugin_Marshal_PBStruct_ReuseBuf(b *testing.B) {
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)
	buf := make([]byte, 0, 512)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf = pbMsg.MarshalOmniBufAppend(buf[:0])
		if len(buf) == 0 {
			b.Fatal("empty")
		}
	}
}

func BenchmarkPlugin_Unmarshal_PBStruct_ZeroCopyReuse(b *testing.B) {
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)
	pkt := pbMsg.MarshalOmniBufAppend(nil)
	out := SamplePBPeerUpdate(&orig)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := out.UnmarshalOmniBuf(pkt, true); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPlugin_Unmarshal_PBStruct_FreshAlloc(b *testing.B) {
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)
	pkt := pbMsg.MarshalOmniBufAppend(nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out pb.PeerUpdate
		if err := out.UnmarshalOmniBuf(pkt, false); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPlugin_Opaque_UnmarshalAndRead2Fields(b *testing.B) {
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)
	pkt := pbMsg.MarshalOmniBufAppend(nil)
	var msg pb.OpaquePeerUpdate
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = msg.Unmarshal(pkt)
		city := msg.GetLocation().GetCity()
		rtr := msg.GetNeighbors(2).GetRouter()
		if len(city) != 9 || len(rtr) == 0 {
			b.Fatal("bad read")
		}
	}
}

func BenchmarkPlugin_Opaque_UnmarshalAndReadALLFields(b *testing.B) {
	// Calls Unmarshal on pb.OpaquePeerUpdate and then invokes getters for EVERY single field
	// (all 11 root fields, all 4 Tags, all 6 Prefixes, all 6 Location fields, and all 4 Neighbors x 4 fields = 33 leaf values!)
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)
	pkt := pbMsg.MarshalOmniBufAppend(nil)
	var msg pb.OpaquePeerUpdate
	tagsBuf := make([]string, 0, 8)
	prefsBuf := make([]uint32, 0, 8)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = msg.Unmarshal(pkt)
		asn := msg.GetAsn()
		rtrID := msg.GetRouterId()
		act := msg.GetActive()
		up := msg.GetUptimeSec()
		rssi := msg.GetRssiDbm()
		cpu := msg.GetCpuLoad()
		tagsBuf = msg.AppendTagsInto(tagsBuf[:0])
		prefsBuf = msg.AppendPrefixesInto(prefsBuf[:0])
		loc := msg.GetLocation()
		reg := loc.GetRegion()
		dc := loc.GetDatacenter()
		city := loc.GetCity()
		rack := loc.GetRack()
		lat := loc.GetLatitude()
		lon := loc.GetLongitude()
		nLen := msg.NeighborsLen()
		var lastRtr string
		for j := 0; j < nLen; j++ {
			nb := msg.GetNeighbors(j)
			_ = nb.GetPeerAsn()
			lastRtr = nb.GetRouter()
			_ = nb.GetLatencyUs()
			_ = nb.GetEstablished()
		}
		seq := msg.GetSequence()
		if asn == 0 || len(rtrID) == 0 || !act || up == 0 || rssi == 0 || cpu == 0 ||
			len(tagsBuf) != 4 || len(prefsBuf) != 6 || len(reg) == 0 || len(dc) == 0 ||
			len(city) != 9 || rack == 0 || lat == 0 || lon == 0 || len(lastRtr) == 0 || seq == 0 {
			b.Fatal("bad read")
		}
	}
}

func BenchmarkProtobufOfficial_UnmarshalAndReadALLFields(b *testing.B) {
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)
	pkt, _ := MarshalProtoOfficial(pbMsg)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var msg pb.PeerUpdate
		if err := proto.Unmarshal(pkt, &msg); err != nil {
			b.Fatal(err)
		}
		asn := msg.Asn
		rtrID := msg.RouterId
		act := msg.Active
		up := msg.UptimeSec
		rssi := msg.RssiDbm
		cpu := msg.CpuLoad
		tags := msg.Tags
		prefs := msg.Prefixes
		loc := msg.GetLocation()
		reg := loc.Region
		dc := loc.Datacenter
		city := loc.City
		rack := loc.Rack
		lat := loc.Latitude
		lon := loc.Longitude
		neighs := msg.GetNeighbors()
		var lastRtr string
		for j := 0; j < len(neighs); j++ {
			nb := neighs[j]
			_ = nb.PeerAsn
			lastRtr = nb.Router
			_ = nb.LatencyUs
			_ = nb.Established
		}
		seq := msg.Sequence
		if asn == 0 || len(rtrID) == 0 || !act || up == 0 || rssi == 0 || cpu == 0 ||
			len(tags) != 4 || len(prefs) != 6 || len(reg) == 0 || len(dc) == 0 ||
			len(city) != 9 || rack == 0 || lat == 0 || lon == 0 || len(lastRtr) == 0 || seq == 0 {
			b.Fatal("bad read")
		}
	}
}

func BenchmarkPlugin_Opaque_ReadModifyWrite_Splice(b *testing.B) {
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)
	pkt := pbMsg.MarshalOmniBufAppend(nil)
	outBuf := make([]byte, 0, 512)
	var msg pb.OpaquePeerUpdate
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = msg.Unmarshal(pkt)
		if msg.GetLocation().GetCity() == "Frankfurt" {
			msg.SetSequence(msg.GetSequence() + 1)
		}
		outBuf = msg.MarshalAppend(outBuf)
		if len(outBuf) == 0 {
			b.Fatal("empty")
		}
	}
}

// 1. Generic Table-Driven Marshal/Unmarshal on Existing *pb.PeerUpdate and *PeerUpdate

func BenchmarkTable_Marshal_ExistingPBStruct_ReuseBuf(b *testing.B) {
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)
	buf := make([]byte, 0, 512)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		buf, err = MarshalAppend(buf, pbMsg)
		if err != nil || len(buf) == 0 {
			b.Fatal(err)
		}
	}
}

func BenchmarkTable_Marshal_ValueStruct_ReuseBuf(b *testing.B) {
	orig := SamplePeerUpdate()
	buf := make([]byte, 0, 512)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		buf, err = MarshalAppend(buf, &orig)
		if err != nil || len(buf) == 0 {
			b.Fatal(err)
		}
	}
}

func BenchmarkTable_Unmarshal_ExistingPBStruct_ZeroCopyReuse(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt := EncodePeerOmniBufDirect(nil, &orig)
	out := SamplePBPeerUpdate(&orig)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := UnmarshalZeroCopy(pkt, out); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTable_Unmarshal_ExistingPBStruct_FreshAlloc(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt := EncodePeerOmniBufDirect(nil, &orig)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out pb.PeerUpdate
		if err := Unmarshal(pkt, &out); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTable_Unmarshal_ValueStruct_ZeroCopyReuse(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt := EncodePeerOmniBufDirect(nil, &orig)
	var out PeerUpdate
	out.Tags = make([]string, 0, 8)
	out.Prefixes = make([]uint32, 0, 8)
	out.Neighbors = make([]Neighbor, 0, 8)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := UnmarshalZeroCopy(pkt, &out); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTable_Unmarshal_ValueStruct_SafeAlloc(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt := EncodePeerOmniBufDirect(nil, &orig)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out PeerUpdate
		if err := Unmarshal(pkt, &out); err != nil {
			b.Fatal(err)
		}
	}
}

// 2. Opaque API Benchmarks: Lazy Unmarshal, On-Demand Getter, and Read-Modify-Write Splicing

func BenchmarkOpaque_UnmarshalAndRead2Fields(b *testing.B) {
	// Simulates a service unmarshaling into an OpaquePeerUpdate and reading
	// peer.GetLocation().GetCity() and peer.GetNeighbors(2).GetRouter()
	orig := SamplePeerUpdate()
	pkt := EncodePeerOmniBufDirect(nil, &orig)
	var msg OpaquePeerUpdate
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = msg.Unmarshal(pkt)
		city := msg.GetLocation().GetCity()
		rtr := msg.GetNeighbors(2).GetRouter()
		if len(city) != 9 || len(rtr) == 0 {
			b.Fatal("bad read")
		}
	}
}

func BenchmarkProtobufOfficial_UnmarshalAndRead2Fields(b *testing.B) {
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)
	pkt, _ := MarshalProtoOfficial(pbMsg)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var msg pb.PeerUpdate
		if err := proto.Unmarshal(pkt, &msg); err != nil {
			b.Fatal(err)
		}
		city := msg.GetLocation().GetCity()
		rtr := msg.GetNeighbors()[2].GetRouter()
		if len(city) != 9 || len(rtr) == 0 {
			b.Fatal("bad read")
		}
	}
}

func BenchmarkOpaque_ReadModifyWrite_Splice(b *testing.B) {
	// Simulates a telemetry router/proxy:
	//   1. Unmarshal packet
	//   2. Check msg.GetLocation().GetCity()
	//   3. Increment msg.SetSequence(msg.GetSequence() + 1)
	//   4. Re-marshal to outgoing buffer (splicing untouched fields without decoding!)
	orig := SamplePeerUpdate()
	pkt := EncodePeerOmniBufDirect(nil, &orig)
	outBuf := make([]byte, 0, 512)
	var msg OpaquePeerUpdate
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = msg.Unmarshal(pkt)
		if msg.GetLocation().GetCity() == "Frankfurt" {
			msg.SetSequence(msg.GetSequence() + 1)
		}
		outBuf = msg.MarshalAppend(outBuf)
		if len(outBuf) == 0 {
			b.Fatal("empty")
		}
	}
}

func BenchmarkProtobufOfficial_ReadModifyWrite(b *testing.B) {
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)
	pkt, _ := MarshalProtoOfficial(pbMsg)
	outBuf := make([]byte, 0, 512)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var msg pb.PeerUpdate
		if err := proto.Unmarshal(pkt, &msg); err != nil {
			b.Fatal(err)
		}
		if msg.GetLocation().GetCity() == "Frankfurt" {
			msg.Sequence = msg.Sequence + 1
		}
		var err error
		outBuf, err = proto.MarshalOptions{}.MarshalAppend(outBuf[:0], &msg)
		if err != nil || len(outBuf) == 0 {
			b.Fatal(err)
		}
	}
}

func BenchmarkProtobufProtoWire_ReadModifyWrite(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt := EncodePeerProtoWire(nil, &orig)
	outBuf := make([]byte, 0, 512)
	var msg PeerUpdate
	msg.Tags = make([]string, 0, 8)
	msg.Prefixes = make([]uint32, 0, 8)
	msg.Neighbors = make([]Neighbor, 0, 8)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := DecodePeerProtoWireInto(pkt, &msg, true); err != nil {
			b.Fatal(err)
		}
		if msg.Location.City == "Frankfurt" {
			msg.Sequence = msg.Sequence + 1
		}
		outBuf = EncodePeerProtoWire(outBuf, &msg)
		if len(outBuf) == 0 {
			b.Fatal("empty")
		}
	}
}

// ============================================================================
// 3. PlanetScale vtprotobuf Benchmarks (protoc-gen-go-vtproto on *pb.PeerUpdate)
// ============================================================================

func BenchmarkVTProto_MarshalToVT_ReuseBuf(b *testing.B) {
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)
	buf := make([]byte, 512)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n, err := pbMsg.MarshalToVT(buf)
		if err != nil || n == 0 {
			b.Fatal(err)
		}
	}
}

func BenchmarkVTProto_MarshalToSizedBufferVT_ReuseBuf(b *testing.B) {
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)
	buf := make([]byte, 512)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n, err := pbMsg.MarshalToSizedBufferVT(buf)
		if err != nil || n == 0 {
			b.Fatal(err)
		}
	}
}

func BenchmarkVTProto_MarshalVT_FreshAlloc(b *testing.B) {
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pkt, err := pbMsg.MarshalVT()
		if err != nil || len(pkt) == 0 {
			b.Fatal(err)
		}
	}
}

func BenchmarkVTProto_UnmarshalVT_FreshAlloc(b *testing.B) {
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)
	pkt, _ := pbMsg.MarshalVT()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out pb.PeerUpdate
		if err := out.UnmarshalVT(pkt); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkVTProto_UnmarshalVTUnsafe_FreshAlloc(b *testing.B) {
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)
	pkt, _ := pbMsg.MarshalVT()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out pb.PeerUpdate
		if err := out.UnmarshalVTUnsafe(pkt); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkVTProto_UnmarshalVTUnsafe_PoolReuse(b *testing.B) {
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)
	pkt, _ := pbMsg.MarshalVT()
	out := pb.PeerUpdateFromVTPool()
	_ = out.UnmarshalVTUnsafe(pkt)
	out.ReturnToVTPool()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := pb.PeerUpdateFromVTPool()
		if err := msg.UnmarshalVTUnsafe(pkt); err != nil {
			b.Fatal(err)
		}
		msg.ReturnToVTPool()
	}
}

func BenchmarkVTProto_UnmarshalAndRead2Fields_PoolReuse(b *testing.B) {
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)
	pkt, _ := pbMsg.MarshalVT()
	warm := pb.PeerUpdateFromVTPool()
	_ = warm.UnmarshalVTUnsafe(pkt)
	warm.ReturnToVTPool()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := pb.PeerUpdateFromVTPool()
		if err := msg.UnmarshalVTUnsafe(pkt); err != nil {
			b.Fatal(err)
		}
		city := msg.GetLocation().GetCity()
		rtr := msg.GetNeighbors()[2].GetRouter()
		if len(city) != 9 || len(rtr) == 0 {
			b.Fatal("bad read")
		}
		msg.ReturnToVTPool()
	}
}

func BenchmarkVTProto_ReadModifyWrite_PoolReuse(b *testing.B) {
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)
	pkt, _ := pbMsg.MarshalVT()
	outBuf := make([]byte, 512)
	warm := pb.PeerUpdateFromVTPool()
	_ = warm.UnmarshalVTUnsafe(pkt)
	warm.ReturnToVTPool()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := pb.PeerUpdateFromVTPool()
		if err := msg.UnmarshalVTUnsafe(pkt); err != nil {
			b.Fatal(err)
		}
		if msg.GetLocation().GetCity() == "Frankfurt" {
			msg.Sequence = msg.Sequence + 1
		}
		n, err := msg.MarshalToVT(outBuf)
		if err != nil || n == 0 {
			b.Fatal(err)
		}
		msg.ReturnToVTPool()
	}
}
