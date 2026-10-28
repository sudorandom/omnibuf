package omnibuf_test

import (
	"math"
	"reflect"
	"testing"

	ob "github.com/sudorandom/omnibuf"
	"github.com/sudorandom/omnibuf/internal/pb"
	"github.com/sudorandom/omnibuf/wire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/structpb"
)

type mergeLeaf struct {
	A    uint32            `omnibuf:"1,omitempty"`
	B    uint32            `omnibuf:"2,omitempty"`
	List []uint32          `omnibuf:"3,omitempty"`
	Map  map[string]uint64 `omnibuf:"4,omitempty"`
}
type mergeChild struct {
	Leaf *mergeLeaf `omnibuf:"1"`
}
type mergeRoot struct {
	Child *mergeChild `omnibuf:"1"`
}

func TestReviewRecursiveMerge(t *testing.T) {
	a := &mergeRoot{&mergeChild{&mergeLeaf{A: 1, List: []uint32{1}, Map: map[string]uint64{"a": 1}}}}
	b := &mergeRoot{&mergeChild{&mergeLeaf{B: 2, List: []uint32{2}, Map: map[string]uint64{"b": 2}}}}
	for _, self := range []bool{false, true} {
		opts := ob.MarshalOptions{SelfDescribing: self}
		pa, err := opts.Marshal(a)
		if err != nil {
			t.Fatal(err)
		}
		pb, err := opts.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		packet := append(pa, pb...)
		var out mergeRoot
		if err := ob.Unmarshal(packet, &out); err != nil {
			t.Fatal(err)
		}
		want := &mergeLeaf{A: 1, B: 2, List: []uint32{1, 2}, Map: map[string]uint64{"a": 1, "b": 2}}
		if !reflect.DeepEqual(out.Child.Leaf, want) {
			t.Fatalf("merge got %+v", out.Child.Leaf)
		}
		blind, err := ob.DecodeBlind(packet)
		if err != nil {
			t.Fatal(err)
		}
		child, leaf, aa, bb, list, mp := "field_1", "field_1", "field_1", "field_2", "field_3", "field_4"
		if self {
			child, leaf, aa, bb, list, mp = "Child", "Leaf", "A", "B", "List", "Map"
		}
		got := blind[child].(map[string]any)[leaf].(map[string]any)
		if got[aa] != uint64(1) || got[bb] != uint64(2) || !reflect.DeepEqual(got[list], []uint64{1, 2}) || !reflect.DeepEqual(got[mp], map[string]uint64{"a": 1, "b": 2}) {
			t.Fatalf("blind merge: %#v", got)
		}
	}
}
func TestReviewOneofPresence(t *testing.T) {
	for _, in := range []*structpb.Value{structpb.NewNumberValue(42), structpb.NewNumberValue(0), structpb.NewStringValue(""), structpb.NewBoolValue(false), structpb.NewNullValue(), structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{structpb.NewNumberValue(1)}})} {
		for _, input := range []any{in, in.ProtoReflect()} {
			packet, err := ob.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			var out structpb.Value
			if err := ob.Unmarshal(packet, &out); err != nil {
				t.Fatal(err)
			}
			if !proto.Equal(in, &out) {
				t.Fatalf("oneof got %v want %v", &out, in)
			}
		}
	}
}
func TestReviewMalformedMetadataAndCounts(t *testing.T) {
	bad := wire.AppendVarint(nil, (1<<4)|uint64(wire.WireMap))
	bad = wire.AppendVarint(bad, 11)
	bad = append(bad, wire.WireUint, wire.WireString)
	bad = wire.AppendVarint(bad, uint64(1)<<63)
	var out struct {
		M map[uint32]string `omnibuf:"1"`
	}
	if err := ob.Unmarshal(bad, &out); err == nil {
		t.Fatal("accepted impossible map count")
	}
	tag0 := byte(((0<<4)|wire.WireSubMsg)<<1) | 1
	tag2 := byte(((2<<4)|wire.WireMap)<<1) | 1
	for _, input := range [][]byte{{tag0, 0x03, 0x00}, append([]byte{tag0, byte((len(bad)+0)<<1) | 1}, append([]byte{tag2}, bad[1:]...)...)} {
		var empty struct{}
		if err := ob.Unmarshal(input, &empty); err == nil {
			t.Fatalf("accepted metadata %x", input)
		}
		if _, err := ob.DecodeBlind(input); err == nil {
			t.Fatalf("blind accepted metadata %x", input)
		}
		var pb structpb.Value
		if err := ob.Unmarshal(input, &pb); err == nil {
			t.Fatalf("protobuf accepted metadata %x", input)
		}
	}
}
func TestReviewLogicalTypes(t *testing.T) {
	type data struct {
		U32     uint32            `omnibuf:"1,wire=fixed32"`
		I32     int32             `omnibuf:"2,wire=fixed32"`
		U64     uint64            `omnibuf:"3,wire=fixed64"`
		I64     int64             `omnibuf:"4,wire=fixed64"`
		Blobs   [][]byte          `omnibuf:"5"`
		Flags   []bool            `omnibuf:"6"`
		Numbers []uint32          `omnibuf:"7,wire=fixed32"`
		Signed  []int64           `omnibuf:"8,wire=fixed64"`
		Map     map[string]uint64 `omnibuf:"9"`
		Narrow  []int64           `omnibuf:"10,wire=fixed32"`
	}
	in := data{42, -42, math.MaxUint64, math.MinInt64, [][]byte{{0xff, 0x00}, {}}, []bool{true, false}, []uint32{42, math.MaxUint32}, []int64{-1, math.MinInt64}, map[string]uint64{"x": 42}, []int64{-1}}
	packet, err := ob.MarshalOptions{SelfDescribing: true}.Marshal(&in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ob.DecodeBlind(packet)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"U32": in.U32, "I32": in.I32, "U64": in.U64, "I64": in.I64, "Blobs": in.Blobs, "Flags": in.Flags, "Numbers": in.Numbers, "Signed": in.Signed, "Map": in.Map, "Narrow": []int32{-1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("logical types got %#v want %#v", got, want)
	}
}

func TestReviewLateSchema(t *testing.T) {
	packet := wire.AppendVarint(nil, 16)
	packet = wire.AppendVarint(packet, 42)
	packet = (&ob.SchemaMeta{Fields: map[uint32]string{1: "answer"}}).AppendField0(packet, true)
	got, err := ob.DecodeBlind(packet)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[string]any{"answer": uint64(42)}) {
		t.Fatalf("late metadata: %#v", got)
	}
}

func TestReviewFullProtobufFieldRange(t *testing.T) {
	fd, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{Name: proto.String("wide.proto"), Syntax: proto.String("proto3"), MessageType: []*descriptorpb.DescriptorProto{{Name: proto.String("Wide"), Field: []*descriptorpb.FieldDescriptorProto{{Name: proto.String("value"), Number: proto.Int32(536870911), Type: descriptorpb.FieldDescriptorProto_TYPE_FIXED64.Enum()}}}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	md := fd.Messages().Get(0)
	field := md.Fields().Get(0)
	in := dynamicpb.NewMessage(md)
	in.Set(field, protoreflect.ValueOfUint64(math.MaxUint64))
	packet, err := ob.MarshalOptions{SelfDescribing: true}.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	out := dynamicpb.NewMessage(md)
	if err := ob.Unmarshal(packet, out); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(in, out) {
		t.Fatalf("high field roundtrip: %v", out)
	}
	blind, err := ob.DecodeBlind(packet)
	if err != nil {
		t.Fatal(err)
	}
	if blind["value"] != uint64(math.MaxUint64) {
		t.Fatalf("blind high field: %#v", blind)
	}
	if meta, ok := ob.ReadField0SchemaMeta(packet); !ok || meta.Fields[536870911] != "value" {
		t.Fatalf("ReadField0SchemaMeta failed on packet with high field number: ok=%v meta=%+v", ok, meta)
	}
	// Verify field 65537 (0x10001) does not alias field 1 in table codec or wire.LocateField
	aliasPkt := append([]byte(nil), packet...)
	aliasPkt = wire.AppendVarint(aliasPkt, (uint64(1)<<4)|uint64(wire.WireUint))
	aliasPkt = wire.AppendVarint(aliasPkt, 77)
	aliasPkt = wire.AppendVarint(aliasPkt, (uint64(65537)<<4)|uint64(wire.WireUint))
	aliasPkt = wire.AppendVarint(aliasPkt, 999)
	var tableOut struct {
		A uint32 `omnibuf:"1"`
	}
	if err := ob.Unmarshal(aliasPkt, &tableOut); err != nil {
		t.Fatalf("table Unmarshal failed skipping high field numbers: %v", err)
	}
	if tableOut.A != 77 {
		t.Fatalf("field 65537 aliased field 1 in table Unmarshal: got %d, want 77", tableOut.A)
	}
	if gotU, ok := wire.ReadUint(aliasPkt, 1); !ok || gotU != 77 {
		t.Fatalf("field 65537 aliased field 1 in wire.LocateField: got (%d, %v), want (77, true)", gotU, ok)
	}
	var genOut pb.TestAllTypes
	if err := genOut.UnmarshalOmniBuf(aliasPkt, false); err != nil {
		t.Fatalf("generated UnmarshalOmniBuf failed on self-describing packet with high unknown fields: %v", err)
	}
	if genOut.F1Uint32 != 77 {
		t.Fatalf("generated UnmarshalOmniBuf got F1Uint32=%d, want 77", genOut.F1Uint32)
	}
}

func TestReviewMergedSubMsgAndReusePresence(t *testing.T) {
	// 1. Concatenated packets with partial sub-messages must merge in wire.ReadSubMsg and Opaque getters
	encA := wire.NewEncoder(64)
	encA.BeginSubMsg(8)
	encA.WriteUint(1, 10)
	encA.WriteString(2, "from-A")
	encA.WriteBool(3, true)
	encA.EndSubMsg()

	encB := wire.NewEncoder(64)
	encB.BeginSubMsg(8)
	encB.WriteUint(1, 20)
	encB.WriteInt(4, -555)
	encB.EndSubMsg()

	concat := append(append([]byte(nil), encA.Bytes()...), encB.Bytes()...)
	var op pb.OpaqueTestAllTypes
	if err := op.Unmarshal(concat); err != nil {
		t.Fatal(err)
	}
	subOp := op.GetF8SubMsg()
	if subOp.GetId() != 20 || subOp.GetLabel() != "from-A" || !subOp.GetFlag() || subOp.GetScore() != -555 {
		t.Fatalf("Opaque merged sub-message mismatch: id=%d label=%q flag=%v score=%d",
			subOp.GetId(), subOp.GetLabel(), subOp.GetFlag(), subOp.GetScore())
	}

	// 2. Reusing a generated struct when a singular sub-message is absent must reset pointer to nil
	reused := pb.TestAllTypes{
		F1Uint32: 42,
		F8SubMsg: &pb.AllTypesSubMsg{Id: 9, Label: "stale"},
	}
	if err := reused.UnmarshalOmniBuf(nil, false); err != nil {
		t.Fatal(err)
	}
	if reused.F8SubMsg != nil {
		t.Fatalf("expected reused.F8SubMsg to be nil when absent, got %+v", reused.F8SubMsg)
	}

	// 3. wire.AppendFixed*Arr / AppendSfixed*Arr must reject mismatched element wire types
	floatArrPkt := wire.AppendVarint(nil, (uint64(1)<<4)|uint64(wire.WireArray))
	floatArrPkt = wire.EncodeFloat32Arr(floatArrPkt, []float32{1.5, 2.5})
	if got := wire.AppendFixed32Arr(nil, floatArrPkt, 1); len(got) != 0 {
		t.Fatalf("AppendFixed32Arr accepted WireFloat32 array: %v", got)
	}
	if got := wire.AppendSfixed32Arr(nil, floatArrPkt, 1); len(got) != 0 {
		t.Fatalf("AppendSfixed32Arr accepted WireFloat32 array: %v", got)
	}
	float64ArrPkt := wire.AppendVarint(nil, (uint64(1)<<4)|uint64(wire.WireArray))
	float64ArrPkt = wire.EncodeFloat64Arr(float64ArrPkt, []float64{1.5, 2.5})
	if got := wire.AppendFixed64Arr(nil, float64ArrPkt, 1); len(got) != 0 {
		t.Fatalf("AppendFixed64Arr accepted WireFloat64 array: %v", got)
	}
	if got := wire.AppendSfixed64Arr(nil, float64ArrPkt, 1); len(got) != 0 {
		t.Fatalf("AppendSfixed64Arr accepted WireFloat64 array: %v", got)
	}
}
func TestReviewProtoFixedMaps(t *testing.T) {
	fd, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{Name: proto.String("map.proto"), Syntax: proto.String("proto3"), MessageType: []*descriptorpb.DescriptorProto{{Name: proto.String("Maps"), NestedType: []*descriptorpb.DescriptorProto{{Name: proto.String("ValuesEntry"), Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)}, Field: []*descriptorpb.FieldDescriptorProto{
		{Name: proto.String("key"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_SFIXED32.Enum()},
		{Name: proto.String("value"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_FIXED64.Enum()},
	}}}, Field: []*descriptorpb.FieldDescriptorProto{{Name: proto.String("values"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(), TypeName: proto.String(".Maps.ValuesEntry")}}}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	md := fd.Messages().Get(0)
	in := dynamicpb.NewMessage(md)
	in.Mutable(md.Fields().Get(0)).Map().Set(protoreflect.ValueOfInt32(-1).MapKey(), protoreflect.ValueOfUint64(math.MaxUint64))
	packet, err := ob.MarshalOptions{SelfDescribing: true}.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	blind, err := ob.DecodeBlind(packet)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"values": map[string]any{"-1": uint64(math.MaxUint64)}}
	if !reflect.DeepEqual(blind, want) {
		t.Fatalf("map got %#v", blind)
	}
}
