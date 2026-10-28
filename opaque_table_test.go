package omnibuf_test

import (
	"math"
	"reflect"
	"strings"
	"testing"

	. "github.com/sudorandom/omnibuf"
	"github.com/sudorandom/omnibuf/internal/pb"
	"github.com/sudorandom/omnibuf/wire"
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
	var op pb.OpaquePeerUpdate
	if err := op.Unmarshal(pkt); err != nil {
		t.Fatalf("OpaquePeerUpdate.Unmarshal failed: %v", err)
	}
	if got := op.GetAsn(); got != orig.ASN {
		t.Fatalf("GetAsn() = %d, want %d", got, orig.ASN)
	}
	if got := op.GetRouterId(); got != orig.RouterID {
		t.Fatalf("GetRouterId() = %q, want %q", got, orig.RouterID)
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
	op.SetAsn(65530)
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

func TestOpaqueSpliceFieldSizeGrowAndShrink(t *testing.T) {
	orig := SamplePeerUpdate()
	orig.ASN = 7 // 1-byte varint originally
	pkt := EncodePeerOmniBufDirect(nil, &orig)

	// 1. Grow fields in the middle of the message so their encoded lengths expand:
	//    - Field #1 (ASN): 1-byte varint (7) -> 4-byte varint (1_000_000)
	//    - Field #2 (RouterID): 19-byte string (1B len prefix) -> 300-byte string (2B len prefix)
	//    - Field #4 (UptimeSec): 4-byte varint (864000) -> 9-byte varint (math.MaxUint64)
	longRouter := strings.Repeat("edge-rtr-fra01-", 20) // 300 bytes
	var op pb.OpaquePeerUpdate
	if err := op.Unmarshal(pkt); err != nil {
		t.Fatalf("OpaquePeerUpdate.Unmarshal failed: %v", err)
	}
	op.SetAsn(1_000_000)
	op.SetRouterId(longRouter)
	op.SetUptimeSec(math.MaxUint64)
	grownPkt := op.MarshalAppend(nil)

	if len(grownPkt) <= len(pkt) {
		t.Fatalf("expected grownPkt (%dB) > pkt (%dB)", len(grownPkt), len(pkt))
	}

	var grownDec PeerUpdate
	if err := DecodePeerOmniBufInto(grownPkt, &grownDec, false); err != nil {
		t.Fatalf("DecodePeerOmniBufInto(grownPkt) failed: %v", err)
	}
	wantGrown := orig
	wantGrown.ASN = 1_000_000
	wantGrown.RouterID = longRouter
	wantGrown.UptimeSec = math.MaxUint64
	if !reflect.DeepEqual(grownDec, wantGrown) {
		t.Fatalf("grownPkt mismatch:\ngot:  %#v\nwant: %#v", grownDec, wantGrown)
	}

	// Verify O(1) header-hop getters on the re-serialized grown packet still locate shifted fields
	var opGrown pb.OpaquePeerUpdate
	_ = opGrown.Unmarshal(grownPkt)
	if got := opGrown.GetLocation().GetCity(); got != "Frankfurt" {
		t.Fatalf("after grow: GetLocation().GetCity() = %q, want Frankfurt", got)
	}
	if got := opGrown.GetNeighbors(3).GetRouter(); got != orig.Neighbors[3].Router {
		t.Fatalf("after grow: GetNeighbors(3).GetRouter() = %q, want %q", got, orig.Neighbors[3].Router)
	}
	if got := opGrown.GetSequence(); got != orig.Sequence {
		t.Fatalf("after grow: GetSequence() = %d, want %d", got, orig.Sequence)
	}

	// 2. Shrink those same middle fields back down:
	//    - Field #1 (ASN): 4-byte varint -> 1-byte varint (1)
	//    - Field #2 (RouterID): 300-byte string -> 1-byte string ("r")
	//    - Field #4 (UptimeSec): 9-byte varint -> 1-byte varint (5)
	opGrown.SetAsn(1)
	opGrown.SetRouterId("r")
	opGrown.SetUptimeSec(5)
	shrunkPkt := opGrown.MarshalAppend(nil)

	if len(shrunkPkt) >= len(pkt) {
		t.Fatalf("expected shrunkPkt (%dB) < pkt (%dB)", len(shrunkPkt), len(pkt))
	}

	var shrunkDec PeerUpdate
	if err := DecodePeerOmniBufInto(shrunkPkt, &shrunkDec, false); err != nil {
		t.Fatalf("DecodePeerOmniBufInto(shrunkPkt) failed: %v", err)
	}
	wantShrunk := orig
	wantShrunk.ASN = 1
	wantShrunk.RouterID = "r"
	wantShrunk.UptimeSec = 5
	if !reflect.DeepEqual(shrunkDec, wantShrunk) {
		t.Fatalf("shrunkPkt mismatch:\ngot:  %#v\nwant: %#v", shrunkDec, wantShrunk)
	}

	// 3. Verify splicing into an aliased destination slice (pkt = op.MarshalAppend(pkt[:0]))
	//    when a middle field grows does not corrupt subsequent fields in raw.
	aliasedBuf := make([]byte, len(pkt), len(pkt)+512)
	copy(aliasedBuf, pkt)
	var opAlias pb.OpaquePeerUpdate
	_ = opAlias.Unmarshal(aliasedBuf)
	opAlias.SetRouterId(longRouter)
	aliasedOut := opAlias.MarshalAppend(aliasedBuf[:0])

	var aliasedDec PeerUpdate
	if err := DecodePeerOmniBufInto(aliasedOut, &aliasedDec, false); err != nil {
		t.Fatalf("DecodePeerOmniBufInto(aliasedOut) failed: %v", err)
	}
	if aliasedDec.RouterID != longRouter || aliasedDec.Location.City != "Frankfurt" || aliasedDec.Sequence != orig.Sequence {
		t.Fatalf("aliasedOut mismatch: %#v", aliasedDec)
	}

	// 4. Verify splicing preserves Field #0 (SchemaMeta) and unknown fields (e.g. Field #99 and Field #2000)
	//    when an earlier field increases in size.
	enc := wire.NewEncoder(512)
	enc.WriteRaw(EncodePeerOmniBuf(enc, &orig, true))
	withUnknown := append([]byte(nil), enc.Bytes()...)
	withUnknown = wire.AppendVarint(withUnknown, (99<<4)|uint64(wire.WireUint))
	withUnknown = wire.AppendVarint(withUnknown, 424242)
	withUnknown = wire.AppendVarint(withUnknown, (2000<<4)|uint64(wire.WireString))
	withUnknown = wire.AppendVarint(withUnknown, 5)
	withUnknown = append(withUnknown, "hello"...)

	var opUnknown pb.OpaquePeerUpdate
	_ = opUnknown.Unmarshal(withUnknown)
	opUnknown.SetRouterId(longRouter)
	splicedUnknown := opUnknown.MarshalAppend(nil)

	if u99, ok := wire.ReadUint(splicedUnknown, 99); !ok || u99 != 424242 {
		t.Fatalf("unknown field #99 lost or corrupted after grow splice: %d, %v", u99, ok)
	}
	if s2000, ok := wire.ReadString(splicedUnknown, 2000); !ok || s2000 != "hello" {
		t.Fatalf("unknown field #2000 lost or corrupted after grow splice: %q, %v", s2000, ok)
	}
	if meta, ok := ReadField0SchemaMeta(splicedUnknown); !ok || meta.Fields[2] != "router_id" {
		t.Fatalf("Field #0 SchemaMeta lost after grow splice: %#v, %v", meta, ok)
	}
}
