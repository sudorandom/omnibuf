package omnibuf_test

import (
	"reflect"
	"testing"

	. "github.com/sudorandom/omnibuf"
	"github.com/sudorandom/omnibuf/pb"
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
