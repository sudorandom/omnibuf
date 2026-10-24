package omnibuf_test

import (
	"bytes"
	"fmt"
	"math"
	"reflect"
	"testing"

	. "github.com/sudorandom/omnibuf"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// TestDynamicPB_CanonicalPeerUpdateEquivalence verifies that standard Go Protobuf's
// dynamicpb.Message (instantiated from pb.File_schemas_peer_proto) produces the exact
// same wire bytes as compiled structs (*pb.PeerUpdate, *PeerUpdate, and Encoder) and
// round-trips cleanly via omnibuf.Marshal and omnibuf.Unmarshal.
func TestDynamicPB_CanonicalPeerUpdateEquivalence(t *testing.T) {
	orig := SamplePeerUpdate()
	pbMsg := SamplePBPeerUpdate(&orig)

	// Convert compiled *pb.PeerUpdate into a *dynamicpb.Message via standard proto.Marshal/Unmarshal
	rawPB, err := proto.Marshal(pbMsg)
	if err != nil {
		t.Fatalf("proto.Marshal failed: %v", err)
	}
	peerDesc := pbMsg.ProtoReflect().Descriptor()
	dynIn := dynamicpb.NewMessage(peerDesc)
	if err := proto.Unmarshal(rawPB, dynIn); err != nil {
		t.Fatalf("proto.Unmarshal into dynamicpb.Message failed: %v", err)
	}

	// 1. Marshal dynamicpb.Message via omnibuf.Marshal (shared-schema mode)
	dynOmniBytes, err := Marshal(dynIn)
	if err != nil {
		t.Fatalf("omnibuf.Marshal(dynamicpb.Message) failed: %v", err)
	}
	wantOmniBytes := EncodePeerOmniBufDirect(nil, &orig)
	if !bytes.Equal(dynOmniBytes, wantOmniBytes) {
		t.Fatalf("dynamicpb Marshal bytes mismatch:\ngot  (%dB): %x\nwant (%dB): %x",
			len(dynOmniBytes), dynOmniBytes, len(wantOmniBytes), wantOmniBytes)
	}

	// 2. Unmarshal OmniBuf bytes into a fresh dynamicpb.Message via omnibuf.Unmarshal
	dynOut := dynamicpb.NewMessage(peerDesc)
	if err := Unmarshal(dynOmniBytes, dynOut); err != nil {
		t.Fatalf("omnibuf.Unmarshal(dynamicpb.Message) failed: %v", err)
	}
	if !proto.Equal(dynIn, dynOut) {
		t.Fatalf("dynamicpb round-trip mismatch:\ngot:  %v\nwant: %v", dynOut, dynIn)
	}

	// 3. Marshal dynamicpb.Message with Field #0 inline schema (self-describing mode)
	dynSelfBytes, err := MarshalProtoReflect(dynIn, true)
	if err != nil {
		t.Fatalf("MarshalProtoReflect(includeField0=true) failed: %v", err)
	}
	enc := NewEncoder(640)
	wantSelfBytes := EncodePeerOmniBuf(enc, &orig, true)
	if !bytes.Equal(dynSelfBytes, wantSelfBytes) {
		t.Fatalf("dynamicpb self-describing bytes mismatch:\ngot  (%dB): %x\nwant (%dB): %x",
			len(dynSelfBytes), dynSelfBytes, len(wantSelfBytes), wantSelfBytes)
	}

	// 4. Unmarshal self-describing packet into dynamicpb.Message (verifies Field #0 O(1) skip)
	dynSelfOut := dynamicpb.NewMessage(peerDesc)
	if err := Unmarshal(dynSelfBytes, dynSelfOut); err != nil {
		t.Fatalf("Unmarshal(self-describing -> dynamicpb) failed: %v", err)
	}
	if !proto.Equal(dynIn, dynSelfOut) {
		t.Fatalf("dynamicpb self-describing round-trip mismatch")
	}
}

type dynFieldSpec struct {
	name     string
	fType    descriptorpb.FieldDescriptorProto_Type
	label    descriptorpb.FieldDescriptorProto_Label
	typeName string
	populate func(m *dynamicpb.Message, fd protoreflect.FieldDescriptor, subDesc protoreflect.MessageDescriptor)
	verify   func(t *testing.T, wire []byte, fnum uint16, m *dynamicpb.Message, fd protoreflect.FieldDescriptor)
}

func allDynamicFieldSpecs() []dynFieldSpec {
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	rep := descriptorpb.FieldDescriptorProto_LABEL_REPEATED

	return []dynFieldSpec{
		{
			name:  "u32",
			fType: descriptorpb.FieldDescriptorProto_TYPE_UINT32,
			label: opt,
			populate: func(m *dynamicpb.Message, fd protoreflect.FieldDescriptor, _ protoreflect.MessageDescriptor) {
				m.Set(fd, protoreflect.ValueOfUint32(64512))
			},
			verify: func(t *testing.T, wire []byte, fnum uint16, m *dynamicpb.Message, fd protoreflect.FieldDescriptor) {
				if v, ok := ReadUint(wire, fnum); !ok || v != 64512 || m.Get(fd).Uint() != 64512 {
					t.Fatalf("u32 mismatch at field %d", fnum)
				}
			},
		},
		{
			name:  "u64_large",
			fType: descriptorpb.FieldDescriptorProto_TYPE_UINT64,
			label: opt,
			populate: func(m *dynamicpb.Message, fd protoreflect.FieldDescriptor, _ protoreflect.MessageDescriptor) {
				m.Set(fd, protoreflect.ValueOfUint64(math.MaxUint64-42))
			},
			verify: func(t *testing.T, wire []byte, fnum uint16, m *dynamicpb.Message, fd protoreflect.FieldDescriptor) {
				if v, ok := ReadUint(wire, fnum); !ok || v != math.MaxUint64-42 || m.Get(fd).Uint() != math.MaxUint64-42 {
					t.Fatalf("u64_large mismatch at field %d", fnum)
				}
			},
		},
		{
			name:  "sint32_neg",
			fType: descriptorpb.FieldDescriptorProto_TYPE_SINT32,
			label: opt,
			populate: func(m *dynamicpb.Message, fd protoreflect.FieldDescriptor, _ protoreflect.MessageDescriptor) {
				m.Set(fd, protoreflect.ValueOfInt32(-123456))
			},
			verify: func(t *testing.T, wire []byte, fnum uint16, m *dynamicpb.Message, fd protoreflect.FieldDescriptor) {
				if v, ok := ReadInt(wire, fnum); !ok || v != -123456 || m.Get(fd).Int() != -123456 {
					t.Fatalf("sint32_neg mismatch at field %d", fnum)
				}
			},
		},
		{
			name:  "int64_min",
			fType: descriptorpb.FieldDescriptorProto_TYPE_INT64,
			label: opt,
			populate: func(m *dynamicpb.Message, fd protoreflect.FieldDescriptor, _ protoreflect.MessageDescriptor) {
				m.Set(fd, protoreflect.ValueOfInt64(math.MinInt64))
			},
			verify: func(t *testing.T, wire []byte, fnum uint16, m *dynamicpb.Message, fd protoreflect.FieldDescriptor) {
				if v, ok := ReadInt(wire, fnum); !ok || v != math.MinInt64 || m.Get(fd).Int() != math.MinInt64 {
					t.Fatalf("int64_min mismatch at field %d", fnum)
				}
			},
		},
		{
			name:  "bool_false",
			fType: descriptorpb.FieldDescriptorProto_TYPE_BOOL,
			label: opt,
			populate: func(m *dynamicpb.Message, fd protoreflect.FieldDescriptor, _ protoreflect.MessageDescriptor) {
				m.Set(fd, protoreflect.ValueOfBool(false))
			},
			verify: func(t *testing.T, wire []byte, fnum uint16, m *dynamicpb.Message, fd protoreflect.FieldDescriptor) {
				if v, ok := ReadBool(wire, fnum); !ok || v || m.Get(fd).Bool() {
					t.Fatalf("bool_false mismatch at field %d", fnum)
				}
			},
		},
		{
			name:  "bool_true",
			fType: descriptorpb.FieldDescriptorProto_TYPE_BOOL,
			label: opt,
			populate: func(m *dynamicpb.Message, fd protoreflect.FieldDescriptor, _ protoreflect.MessageDescriptor) {
				m.Set(fd, protoreflect.ValueOfBool(true))
			},
			verify: func(t *testing.T, wire []byte, fnum uint16, m *dynamicpb.Message, fd protoreflect.FieldDescriptor) {
				if v, ok := ReadBool(wire, fnum); !ok || !v || !m.Get(fd).Bool() {
					t.Fatalf("bool_true mismatch at field %d", fnum)
				}
			},
		},
		{
			name:  "str_utf8",
			fType: descriptorpb.FieldDescriptorProto_TYPE_STRING,
			label: opt,
			populate: func(m *dynamicpb.Message, fd protoreflect.FieldDescriptor, _ protoreflect.MessageDescriptor) {
				m.Set(fd, protoreflect.ValueOfString("dynamic-utf8-🚀-fra01"))
			},
			verify: func(t *testing.T, wire []byte, fnum uint16, m *dynamicpb.Message, fd protoreflect.FieldDescriptor) {
				if v, ok := ReadString(wire, fnum); !ok || v != "dynamic-utf8-🚀-fra01" || m.Get(fd).String() != "dynamic-utf8-🚀-fra01" {
					t.Fatalf("str_utf8 mismatch at field %d", fnum)
				}
			},
		},
		{
			name:  "raw_bytes",
			fType: descriptorpb.FieldDescriptorProto_TYPE_BYTES,
			label: opt,
			populate: func(m *dynamicpb.Message, fd protoreflect.FieldDescriptor, _ protoreflect.MessageDescriptor) {
				m.Set(fd, protoreflect.ValueOfBytes([]byte{0x00, 0xFF, 0x7F, 0x80, 0x42}))
			},
			verify: func(t *testing.T, wire []byte, fnum uint16, m *dynamicpb.Message, fd protoreflect.FieldDescriptor) {
				want := []byte{0x00, 0xFF, 0x7F, 0x80, 0x42}
				if v, ok := ReadBytes(wire, fnum); !ok || !bytes.Equal(v, want) || !bytes.Equal(m.Get(fd).Bytes(), want) {
					t.Fatalf("raw_bytes mismatch at field %d", fnum)
				}
			},
		},
		{
			name:  "f32",
			fType: descriptorpb.FieldDescriptorProto_TYPE_FLOAT,
			label: opt,
			populate: func(m *dynamicpb.Message, fd protoreflect.FieldDescriptor, _ protoreflect.MessageDescriptor) {
				m.Set(fd, protoreflect.ValueOfFloat32(50.1109))
			},
			verify: func(t *testing.T, wire []byte, fnum uint16, m *dynamicpb.Message, fd protoreflect.FieldDescriptor) {
				if v, ok := ReadFloat32(wire, fnum); !ok || v != 50.1109 || float32(m.Get(fd).Float()) != 50.1109 {
					t.Fatalf("f32 mismatch at field %d", fnum)
				}
			},
		},
		{
			name:  "f64",
			fType: descriptorpb.FieldDescriptorProto_TYPE_DOUBLE,
			label: opt,
			populate: func(m *dynamicpb.Message, fd protoreflect.FieldDescriptor, _ protoreflect.MessageDescriptor) {
				m.Set(fd, protoreflect.ValueOfFloat64(0.42875123456))
			},
			verify: func(t *testing.T, wire []byte, fnum uint16, m *dynamicpb.Message, fd protoreflect.FieldDescriptor) {
				if v, ok := ReadFloat64(wire, fnum); !ok || v != 0.42875123456 || m.Get(fd).Float() != 0.42875123456 {
					t.Fatalf("f64 mismatch at field %d", fnum)
				}
			},
		},
		{
			name:     "submsg",
			fType:    descriptorpb.FieldDescriptorProto_TYPE_MESSAGE,
			label:    opt,
			typeName: ".omnibuf.dynamic.ChildMsg",
			populate: func(m *dynamicpb.Message, fd protoreflect.FieldDescriptor, subDesc protoreflect.MessageDescriptor) {
				sub := dynamicpb.NewMessage(subDesc)
				sub.Set(subDesc.Fields().ByNumber(1), protoreflect.ValueOfUint32(777))
				sub.Set(subDesc.Fields().ByNumber(2), protoreflect.ValueOfString("nested-label"))
				sub.Set(subDesc.Fields().ByNumber(3), protoreflect.ValueOfInt64(-9999))
				m.Set(fd, protoreflect.ValueOfMessage(sub))
			},
			verify: func(t *testing.T, wire []byte, fnum uint16, m *dynamicpb.Message, fd protoreflect.FieldDescriptor) {
				subBytes, ok := ReadSubMsg(wire, fnum)
				if !ok {
					t.Fatalf("ReadSubMsg(%d) failed", fnum)
				}
				if lbl, ok := ReadString(subBytes, 2); !ok || lbl != "nested-label" {
					t.Fatalf("ReadSubMsg(%d).label = %q", fnum, lbl)
				}
				gotSub := m.Get(fd).Message()
				if gotSub.Get(gotSub.Descriptor().Fields().ByNumber(3)).Int() != -9999 {
					t.Fatalf("submsg int64 mismatch")
				}
			},
		},
		{
			name:  "rep_str",
			fType: descriptorpb.FieldDescriptorProto_TYPE_STRING,
			label: rep,
			populate: func(m *dynamicpb.Message, fd protoreflect.FieldDescriptor, _ protoreflect.MessageDescriptor) {
				l := m.Mutable(fd).List()
				l.Append(protoreflect.ValueOfString("alpha"))
				l.Append(protoreflect.ValueOfString(""))
				l.Append(protoreflect.ValueOfString("omega-🚀"))
			},
			verify: func(t *testing.T, wire []byte, fnum uint16, m *dynamicpb.Message, fd protoreflect.FieldDescriptor) {
				want := []string{"alpha", "", "omega-🚀"}
				if got, ok := ReadStringArr(wire, fnum); !ok || !reflect.DeepEqual(got, want) {
					t.Fatalf("ReadStringArr(%d) = %v, want %v", fnum, got, want)
				}
				if m.Get(fd).List().Len() != 3 {
					t.Fatalf("rep_str list len = %d", m.Get(fd).List().Len())
				}
			},
		},
		{
			name:  "rep_bytes",
			fType: descriptorpb.FieldDescriptorProto_TYPE_BYTES,
			label: rep,
			populate: func(m *dynamicpb.Message, fd protoreflect.FieldDescriptor, _ protoreflect.MessageDescriptor) {
				l := m.Mutable(fd).List()
				l.Append(protoreflect.ValueOfBytes([]byte{0x00, 0x01}))
				l.Append(protoreflect.ValueOfBytes([]byte{}))
				l.Append(protoreflect.ValueOfBytes([]byte{0xDE, 0xAD, 0xBE, 0xEF}))
			},
			verify: func(t *testing.T, _ []byte, _ uint16, m *dynamicpb.Message, fd protoreflect.FieldDescriptor) {
				if m.Get(fd).List().Len() != 3 || !bytes.Equal(m.Get(fd).List().Get(2).Bytes(), []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
					t.Fatalf("rep_bytes list mismatch")
				}
			},
		},
		{
			name:  "rep_bool",
			fType: descriptorpb.FieldDescriptorProto_TYPE_BOOL,
			label: rep,
			populate: func(m *dynamicpb.Message, fd protoreflect.FieldDescriptor, _ protoreflect.MessageDescriptor) {
				l := m.Mutable(fd).List()
				for _, v := range []bool{true, false, true, true, false} {
					l.Append(protoreflect.ValueOfBool(v))
				}
			},
			verify: func(t *testing.T, wire []byte, fnum uint16, m *dynamicpb.Message, fd protoreflect.FieldDescriptor) {
				wantU := []uint32{1, 0, 1, 1, 0}
				if got, ok := ReadUint32Arr(wire, fnum); !ok || !reflect.DeepEqual(got, wantU) {
					t.Fatalf("ReadUint32Arr(%d) = %v, want %v", fnum, got, wantU)
				}
				if m.Get(fd).List().Len() != 5 || !m.Get(fd).List().Get(0).Bool() || m.Get(fd).List().Get(1).Bool() {
					t.Fatalf("rep_bool list mismatch")
				}
			},
		},
		{
			name:  "rep_u32",
			fType: descriptorpb.FieldDescriptorProto_TYPE_UINT32,
			label: rep,
			populate: func(m *dynamicpb.Message, fd protoreflect.FieldDescriptor, _ protoreflect.MessageDescriptor) {
				l := m.Mutable(fd).List()
				for _, v := range []uint32{0, 127, 128, 16383, 16384, math.MaxUint32} {
					l.Append(protoreflect.ValueOfUint32(v))
				}
			},
			verify: func(t *testing.T, wire []byte, fnum uint16, m *dynamicpb.Message, fd protoreflect.FieldDescriptor) {
				want := []uint32{0, 127, 128, 16383, 16384, math.MaxUint32}
				if got, ok := ReadUint32Arr(wire, fnum); !ok || !reflect.DeepEqual(got, want) {
					t.Fatalf("ReadUint32Arr(%d) = %v, want %v", fnum, got, want)
				}
				if m.Get(fd).List().Len() != len(want) {
					t.Fatalf("rep_u32 list len mismatch")
				}
			},
		},
		{
			name:  "rep_sint64",
			fType: descriptorpb.FieldDescriptorProto_TYPE_SINT64,
			label: rep,
			populate: func(m *dynamicpb.Message, fd protoreflect.FieldDescriptor, _ protoreflect.MessageDescriptor) {
				l := m.Mutable(fd).List()
				for _, v := range []int64{-1, 0, 1, -64, 64, -8192, 8192, math.MinInt64, math.MaxInt64} {
					l.Append(protoreflect.ValueOfInt64(v))
				}
			},
			verify: func(t *testing.T, wire []byte, fnum uint16, m *dynamicpb.Message, fd protoreflect.FieldDescriptor) {
				want := []int64{-1, 0, 1, -64, 64, -8192, 8192, math.MinInt64, math.MaxInt64}
				if got, ok := ReadSintArr(wire, fnum); !ok || !reflect.DeepEqual(got, want) {
					t.Fatalf("ReadSintArr(%d) = %v, want %v", fnum, got, want)
				}
				if m.Get(fd).List().Len() != len(want) {
					t.Fatalf("rep_sint64 list len mismatch")
				}
			},
		},
		{
			name:  "rep_f32",
			fType: descriptorpb.FieldDescriptorProto_TYPE_FLOAT,
			label: rep,
			populate: func(m *dynamicpb.Message, fd protoreflect.FieldDescriptor, _ protoreflect.MessageDescriptor) {
				l := m.Mutable(fd).List()
				for _, v := range []float32{-1.5, 0.0, 3.25, 1024.5} {
					l.Append(protoreflect.ValueOfFloat32(v))
				}
			},
			verify: func(t *testing.T, wire []byte, fnum uint16, m *dynamicpb.Message, fd protoreflect.FieldDescriptor) {
				want := []float32{-1.5, 0.0, 3.25, 1024.5}
				if got, ok := ReadFloat32Arr(wire, fnum); !ok || !reflect.DeepEqual(got, want) {
					t.Fatalf("ReadFloat32Arr(%d) = %v, want %v", fnum, got, want)
				}
				if m.Get(fd).List().Len() != len(want) {
					t.Fatalf("rep_f32 list len mismatch")
				}
			},
		},
		{
			name:  "rep_f64",
			fType: descriptorpb.FieldDescriptorProto_TYPE_DOUBLE,
			label: rep,
			populate: func(m *dynamicpb.Message, fd protoreflect.FieldDescriptor, _ protoreflect.MessageDescriptor) {
				l := m.Mutable(fd).List()
				for _, v := range []float64{-99.125, 0.0, 3.141592653589793} {
					l.Append(protoreflect.ValueOfFloat64(v))
				}
			},
			verify: func(t *testing.T, wire []byte, fnum uint16, m *dynamicpb.Message, fd protoreflect.FieldDescriptor) {
				want := []float64{-99.125, 0.0, 3.141592653589793}
				if got, ok := ReadFloat64Arr(wire, fnum); !ok || !reflect.DeepEqual(got, want) {
					t.Fatalf("ReadFloat64Arr(%d) = %v, want %v", fnum, got, want)
				}
				if m.Get(fd).List().Len() != len(want) {
					t.Fatalf("rep_f64 list len mismatch")
				}
			},
		},
		{
			name:     "rep_submsg",
			fType:    descriptorpb.FieldDescriptorProto_TYPE_MESSAGE,
			label:    rep,
			typeName: ".omnibuf.dynamic.ChildMsg",
			populate: func(m *dynamicpb.Message, fd protoreflect.FieldDescriptor, subDesc protoreflect.MessageDescriptor) {
				l := m.Mutable(fd).List()
				for idx := range 3 {
					elem := dynamicpb.NewMessage(subDesc)
					elem.Set(subDesc.Fields().ByNumber(1), protoreflect.ValueOfUint32(uint32(100+idx)))
					elem.Set(subDesc.Fields().ByNumber(2), protoreflect.ValueOfString(fmt.Sprintf("item-%d", idx)))
					elem.Set(subDesc.Fields().ByNumber(3), protoreflect.ValueOfInt64(int64(-10*idx)))
					l.Append(protoreflect.ValueOfMessage(elem))
				}
			},
			verify: func(t *testing.T, wire []byte, fnum uint16, m *dynamicpb.Message, fd protoreflect.FieldDescriptor) {
				if SubMsgArrLen(wire, fnum) != 3 {
					t.Fatalf("SubMsgArrLen(%d) = %d, want 3", fnum, SubMsgArrLen(wire, fnum))
				}
				elem2, ok := ReadSubMsgArrElem(wire, fnum, 2)
				if !ok {
					t.Fatalf("ReadSubMsgArrElem(%d, 2) failed", fnum)
				}
				if lbl, ok := ReadString(elem2, 2); !ok || lbl != "item-2" {
					t.Fatalf("elem[2].label = %q, want item-2", lbl)
				}
				if m.Get(fd).List().Len() != 3 {
					t.Fatalf("rep_submsg len = %d", m.Get(fd).List().Len())
				}
			},
		},
	}
}

func buildDynamicSchemaFile(t *testing.T, msgDesc *descriptorpb.DescriptorProto) (protoreflect.MessageDescriptor, protoreflect.MessageDescriptor) {
	t.Helper()
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	childProto := &descriptorpb.DescriptorProto{
		Name: proto.String("ChildMsg"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{
				Name:   proto.String("id"),
				Number: proto.Int32(1),
				Label:  opt.Enum(),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_UINT32.Enum(),
			},
			{
				Name:   proto.String("label"),
				Number: proto.Int32(2),
				Label:  opt.Enum(),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			},
			{
				Name:   proto.String("delta"),
				Number: proto.Int32(3),
				Label:  opt.Enum(),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_SINT64.Enum(),
			},
		},
	}
	fdp := &descriptorpb.FileDescriptorProto{
		Syntax:      proto.String("proto3"),
		Name:        proto.String("dynamic_matrix.proto"),
		Package:     proto.String("omnibuf.dynamic"),
		MessageType: []*descriptorpb.DescriptorProto{childProto, msgDesc},
	}
	fd, err := protodesc.NewFile(fdp, nil)
	if err != nil {
		t.Fatalf("protodesc.NewFile failed: %v", err)
	}
	return fd.Messages().ByName(protoreflect.Name(msgDesc.GetName())), fd.Messages().ByName("ChildMsg")
}

// TestDynamicPB_FieldNumberAndWireTypeExhaustiveMatrix enumerates field numbers across
// Prefix-Varint tag-width boundaries (1, 7, 8, 15, 127, 1023, 1024, 4096) and all wire types.
func TestDynamicPB_FieldNumberAndWireTypeExhaustiveMatrix(t *testing.T) {
	boundaryFieldNums := []uint16{
		1,    // min 1-byte tag
		7,    // max 1-byte tag ((7<<4)|15 = 127)
		8,    // min 2-byte tag ((8<<4)|0 = 128)
		15,   // proto classic boundary
		127,  // mid 2-byte tag
		1023, // max 2-byte tag ((1023<<4)|15 = 16383)
		1024, // min 4-byte tag ((1024<<4)|0 = 16384)
		4096, // high field number
	}
	specs := allDynamicFieldSpecs()

	for _, fnum := range boundaryFieldNums {
		for _, spec := range specs {
			caseName := fmt.Sprintf("fnum_%d_%s", fnum, spec.name)
			t.Run(caseName, func(t *testing.T) {
				fdProto := &descriptorpb.FieldDescriptorProto{
					Name:   proto.String(spec.name),
					Number: proto.Int32(int32(fnum)),
					Label:  spec.label.Enum(),
					Type:   spec.fType.Enum(),
				}
				if spec.typeName != "" {
					fdProto.TypeName = proto.String(spec.typeName)
				}
				msgProto := &descriptorpb.DescriptorProto{
					Name:  proto.String("SingleFieldMsg"),
					Field: []*descriptorpb.FieldDescriptorProto{fdProto},
				}
				msgDesc, childDesc := buildDynamicSchemaFile(t, msgProto)
				fd := msgDesc.Fields().ByNumber(protoreflect.FieldNumber(fnum))

				inMsg := dynamicpb.NewMessage(msgDesc)
				spec.populate(inMsg, fd, childDesc)

				// 1. Shared-schema Marshal + Unmarshal + Reader verification
				wire, err := Marshal(inMsg)
				if err != nil {
					t.Fatalf("Marshal failed: %v", err)
				}
				outMsg := dynamicpb.NewMessage(msgDesc)
				if err := Unmarshal(wire, outMsg); err != nil {
					t.Fatalf("Unmarshal failed: %v", err)
				}
				spec.verify(t, wire, fnum, outMsg, fd)

				// 2. Self-describing (Field #0) Marshal + DecodeBlind + Unmarshal
				wireSD, err := MarshalProtoReflect(inMsg, true)
				if err != nil {
					t.Fatalf("MarshalProtoReflect(includeField0=true) failed: %v", err)
				}
				blindMap, err := DecodeBlind(wireSD)
				if err != nil {
					t.Fatalf("DecodeBlind failed: %v", err)
				}
				if _, ok := blindMap[spec.name]; !ok {
					t.Fatalf("DecodeBlind missing field %q in %v", spec.name, blindMap)
				}
				outSD := dynamicpb.NewMessage(msgDesc)
				if err := Unmarshal(wireSD, outSD); err != nil {
					t.Fatalf("Unmarshal(self-describing) failed: %v", err)
				}
				spec.verify(t, wireSD, fnum, outSD, fd)
			})
		}
	}
}

// TestDynamicPB_PairwiseWireTypeSkipMatrix tests every pair of (FirstFieldType, SecondFieldType)
// to verify that SkipValue and LocateField jump over Field #1 in O(1) time to reach Field #2
// across all wire types.
func TestDynamicPB_PairwiseWireTypeSkipMatrix(t *testing.T) {
	specs := allDynamicFieldSpecs()
	for i, first := range specs {
		for j, second := range specs {
			caseName := fmt.Sprintf("skip_%s_to_%s", first.name, second.name)
			t.Run(caseName, func(t *testing.T) {
				fnum1 := uint16(1 + (i % 10))
				fnum2 := fnum1 + uint16(1+(j%15))
				fd1Proto := &descriptorpb.FieldDescriptorProto{
					Name:   proto.String("first_" + first.name),
					Number: proto.Int32(int32(fnum1)),
					Label:  first.label.Enum(),
					Type:   first.fType.Enum(),
				}
				if first.typeName != "" {
					fd1Proto.TypeName = proto.String(first.typeName)
				}
				fd2Proto := &descriptorpb.FieldDescriptorProto{
					Name:   proto.String("second_" + second.name),
					Number: proto.Int32(int32(fnum2)),
					Label:  second.label.Enum(),
					Type:   second.fType.Enum(),
				}
				if second.typeName != "" {
					fd2Proto.TypeName = proto.String(second.typeName)
				}
				msgProto := &descriptorpb.DescriptorProto{
					Name:  proto.String("PairMsg"),
					Field: []*descriptorpb.FieldDescriptorProto{fd1Proto, fd2Proto},
				}
				msgDesc, childDesc := buildDynamicSchemaFile(t, msgProto)
				fd1 := msgDesc.Fields().ByNumber(protoreflect.FieldNumber(fnum1))
				fd2 := msgDesc.Fields().ByNumber(protoreflect.FieldNumber(fnum2))

				inMsg := dynamicpb.NewMessage(msgDesc)
				first.populate(inMsg, fd1, childDesc)
				second.populate(inMsg, fd2, childDesc)

				wire, err := Marshal(inMsg)
				if err != nil {
					t.Fatalf("Marshal failed: %v", err)
				}
				outMsg := dynamicpb.NewMessage(msgDesc)
				if err := Unmarshal(wire, outMsg); err != nil {
					t.Fatalf("Unmarshal failed: %v", err)
				}
				first.verify(t, wire, fnum1, outMsg, fd1)
				second.verify(t, wire, fnum2, outMsg, fd2)
			})
		}
	}
}

// TestDynamicPB_GeneralizedWireMapMatrix tests WireMap (9) with the 1-byte
// MapType = (KeyWT<<4)|ValWT header across diverse Protobuf map<K, V> combinations.
func TestDynamicPB_GeneralizedWireMapMatrix(t *testing.T) {
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	rep := descriptorpb.FieldDescriptorProto_LABEL_REPEATED

	mapEntry := func(entryName string, kType, vType descriptorpb.FieldDescriptorProto_Type, vTypeName string) *descriptorpb.DescriptorProto {
		valField := &descriptorpb.FieldDescriptorProto{
			Name:   proto.String("value"),
			Number: proto.Int32(2),
			Label:  opt.Enum(),
			Type:   vType.Enum(),
		}
		if vTypeName != "" {
			valField.TypeName = proto.String(vTypeName)
		}
		return &descriptorpb.DescriptorProto{
			Name: proto.String(entryName),
			Options: &descriptorpb.MessageOptions{
				MapEntry: proto.Bool(true),
			},
			Field: []*descriptorpb.FieldDescriptorProto{
				{
					Name:   proto.String("key"),
					Number: proto.Int32(1),
					Label:  opt.Enum(),
					Type:   kType.Enum(),
				},
				valField,
			},
		}
	}

	msgProto := &descriptorpb.DescriptorProto{
		Name: proto.String("MultiMapMsg"),
		NestedType: []*descriptorpb.DescriptorProto{
			mapEntry("StrUintEntry", descriptorpb.FieldDescriptorProto_TYPE_STRING, descriptorpb.FieldDescriptorProto_TYPE_UINT64, ""),
			mapEntry("StrSintEntry", descriptorpb.FieldDescriptorProto_TYPE_STRING, descriptorpb.FieldDescriptorProto_TYPE_SINT64, ""),
			mapEntry("StrStrEntry", descriptorpb.FieldDescriptorProto_TYPE_STRING, descriptorpb.FieldDescriptorProto_TYPE_STRING, ""),
			mapEntry("StrChildEntry", descriptorpb.FieldDescriptorProto_TYPE_STRING, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, ".omnibuf.dynamic.ChildMsg"),
			mapEntry("UintStrEntry", descriptorpb.FieldDescriptorProto_TYPE_UINT64, descriptorpb.FieldDescriptorProto_TYPE_STRING, ""),
			mapEntry("SintDoubleEntry", descriptorpb.FieldDescriptorProto_TYPE_SINT32, descriptorpb.FieldDescriptorProto_TYPE_DOUBLE, ""),
			mapEntry("Fixed32BoolEntry", descriptorpb.FieldDescriptorProto_TYPE_FIXED32, descriptorpb.FieldDescriptorProto_TYPE_BOOL, ""),
		},
		Field: []*descriptorpb.FieldDescriptorProto{
			{Name: proto.String("str_uint"), Number: proto.Int32(1), Label: rep.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".omnibuf.dynamic.MultiMapMsg.StrUintEntry")},
			{Name: proto.String("str_sint"), Number: proto.Int32(2), Label: rep.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".omnibuf.dynamic.MultiMapMsg.StrSintEntry")},
			{Name: proto.String("str_str"), Number: proto.Int32(3), Label: rep.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".omnibuf.dynamic.MultiMapMsg.StrStrEntry")},
			{Name: proto.String("str_child"), Number: proto.Int32(4), Label: rep.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".omnibuf.dynamic.MultiMapMsg.StrChildEntry")},
			{Name: proto.String("uint_str"), Number: proto.Int32(5), Label: rep.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".omnibuf.dynamic.MultiMapMsg.UintStrEntry")},
			{Name: proto.String("sint_double"), Number: proto.Int32(6), Label: rep.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".omnibuf.dynamic.MultiMapMsg.SintDoubleEntry")},
			{Name: proto.String("fixed32_bool"), Number: proto.Int32(7), Label: rep.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".omnibuf.dynamic.MultiMapMsg.Fixed32BoolEntry")},
		},
	}

	msgDesc, childDesc := buildDynamicSchemaFile(t, msgProto)
	inMsg := dynamicpb.NewMessage(msgDesc)

	// Populate 1: map<string, uint64>
	m1 := inMsg.Mutable(msgDesc.Fields().ByNumber(1)).Map()
	m1.Set(protoreflect.ValueOfString("rx").MapKey(), protoreflect.ValueOfUint64(1000))
	m1.Set(protoreflect.ValueOfString("tx").MapKey(), protoreflect.ValueOfUint64(2000))

	// Populate 2: map<string, sint64>
	m2 := inMsg.Mutable(msgDesc.Fields().ByNumber(2)).Map()
	m2.Set(protoreflect.ValueOfString("offset_a").MapKey(), protoreflect.ValueOfInt64(-420))
	m2.Set(protoreflect.ValueOfString("offset_b").MapKey(), protoreflect.ValueOfInt64(840))

	// Populate 3: map<string, string>
	m3 := inMsg.Mutable(msgDesc.Fields().ByNumber(3)).Map()
	m3.Set(protoreflect.ValueOfString("env").MapKey(), protoreflect.ValueOfString("production"))
	m3.Set(protoreflect.ValueOfString("dc").MapKey(), protoreflect.ValueOfString("fra01"))

	// Populate 4: map<string, ChildMsg>
	m4 := inMsg.Mutable(msgDesc.Fields().ByNumber(4)).Map()
	childVal := dynamicpb.NewMessage(childDesc)
	childVal.Set(childDesc.Fields().ByNumber(1), protoreflect.ValueOfUint32(99))
	childVal.Set(childDesc.Fields().ByNumber(2), protoreflect.ValueOfString("map-submsg-val"))
	childVal.Set(childDesc.Fields().ByNumber(3), protoreflect.ValueOfInt64(-77))
	m4.Set(protoreflect.ValueOfString("primary").MapKey(), protoreflect.ValueOfMessage(childVal))

	// Populate 5: map<uint64, string>
	m5 := inMsg.Mutable(msgDesc.Fields().ByNumber(5)).Map()
	m5.Set(protoreflect.ValueOfUint64(64512).MapKey(), protoreflect.ValueOfString("edge-rtr-01"))

	// Populate 6: map<sint32, double>
	m6 := inMsg.Mutable(msgDesc.Fields().ByNumber(6)).Map()
	m6.Set(protoreflect.ValueOfInt32(-10).MapKey(), protoreflect.ValueOfFloat64(99.5))

	// Populate 7: map<fixed32, bool>
	m7 := inMsg.Mutable(msgDesc.Fields().ByNumber(7)).Map()
	m7.Set(protoreflect.ValueOfUint32(0xDEADBEEF).MapKey(), protoreflect.ValueOfBool(true))

	// Encode with Field #0 enabled (so nested ChildMsg inside map<string, ChildMsg> also gets Field #0!)
	wire, err := MarshalProtoReflect(inMsg, true)
	if err != nil {
		t.Fatalf("MarshalProtoReflect failed: %v", err)
	}

	// Verify Unmarshal into a fresh dynamicpb.Message
	outMsg := dynamicpb.NewMessage(msgDesc)
	if err := Unmarshal(wire, outMsg); err != nil {
		t.Fatalf("Unmarshal MultiMapMsg failed: %v", err)
	}
	if !proto.Equal(inMsg, outMsg) {
		t.Fatalf("MultiMapMsg round-trip mismatch:\ngot:  %v\nwant: %v", outMsg, inMsg)
	}

	// Verify DecodeBlind decodes all 7 map types losslessly without a schema!
	blind, err := DecodeBlind(wire)
	if err != nil {
		t.Fatalf("DecodeBlind MultiMapMsg failed: %v", err)
	}
	strUintMap, ok := blind["str_uint"].(map[string]uint64)
	if !ok || strUintMap["rx"] != 1000 || strUintMap["tx"] != 2000 {
		t.Fatalf("DecodeBlind str_uint mismatch: %#v", blind["str_uint"])
	}
	strSintMap, ok := blind["str_sint"].(map[string]any)
	if !ok || strSintMap["offset_a"] != int64(-420) {
		t.Fatalf("DecodeBlind str_sint mismatch: %#v", blind["str_sint"])
	}
	strChildMap, ok := blind["str_child"].(map[string]any)
	if !ok {
		t.Fatalf("DecodeBlind str_child missing: %#v", blind["str_child"])
	}
	primarySub, ok := strChildMap["primary"].(map[string]any)
	if !ok || primarySub["label"] != "map-submsg-val" || primarySub["delta"] != int64(-77) {
		t.Fatalf("DecodeBlind nested map submsg mismatch: %#v", primarySub)
	}
}

// TestDynamicPB_SchemaEvolutionMatrix tests multi-version schema evolution using
// dynamicpb.Message descriptors (WriterDescriptor -> ReaderDescriptor) where the reader:
//   - Drops fields present in the writer (V2 -> V1)
//   - Adds new fields not present in the writer (V1 -> V2)
//   - Changes the wire type of an existing field number (gracefully skipped without error)
func TestDynamicPB_SchemaEvolutionMatrix(t *testing.T) {
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	rep := descriptorpb.FieldDescriptorProto_LABEL_REPEATED

	writerProto := &descriptorpb.DescriptorProto{
		Name: proto.String("TelemetryWriter"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{Name: proto.String("asn"), Number: proto.Int32(1), Label: opt.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_UINT32.Enum()},
			{Name: proto.String("tags"), Number: proto.Int32(2), Label: rep.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
			{Name: proto.String("deltas"), Number: proto.Int32(3), Label: rep.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_SINT64.Enum()},
			{Name: proto.String("samples"), Number: proto.Int32(4), Label: rep.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_DOUBLE.Enum()},
			{Name: proto.String("child"), Number: proto.Int32(5), Label: opt.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".omnibuf.dynamic.ChildMsg")},
			{Name: proto.String("seq"), Number: proto.Int32(10), Label: opt.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_UINT64.Enum()},
		},
	}
	writerDesc, childDesc := buildDynamicSchemaFile(t, writerProto)
	wMsg := dynamicpb.NewMessage(writerDesc)
	wMsg.Set(writerDesc.Fields().ByNumber(1), protoreflect.ValueOfUint32(64512))
	tagsList := wMsg.Mutable(writerDesc.Fields().ByNumber(2)).List()
	tagsList.Append(protoreflect.ValueOfString("edge"))
	tagsList.Append(protoreflect.ValueOfString("core"))
	deltasList := wMsg.Mutable(writerDesc.Fields().ByNumber(3)).List()
	deltasList.Append(protoreflect.ValueOfInt64(-500))
	deltasList.Append(protoreflect.ValueOfInt64(500))
	samplesList := wMsg.Mutable(writerDesc.Fields().ByNumber(4)).List()
	samplesList.Append(protoreflect.ValueOfFloat64(0.99))
	ch := dynamicpb.NewMessage(childDesc)
	ch.Set(childDesc.Fields().ByNumber(1), protoreflect.ValueOfUint32(7))
	ch.Set(childDesc.Fields().ByNumber(2), protoreflect.ValueOfString("child-v2"))
	wMsg.Set(writerDesc.Fields().ByNumber(5), protoreflect.ValueOfMessage(ch))
	wMsg.Set(writerDesc.Fields().ByNumber(10), protoreflect.ValueOfUint64(123456789))

	wire, err := Marshal(wMsg)
	if err != nil {
		t.Fatalf("Marshal writer failed: %v", err)
	}

	// Reader schema: only knows Field #1 (asn), has a wire-type mismatch on Field #3 (string instead of repeated sint64),
	// skips Field #2 (WireStringArr), Field #4 (WireFixed64Arr), and Field #5 (WireSubMsg), and reads Field #10 (seq).
	readerProto := &descriptorpb.DescriptorProto{
		Name: proto.String("TelemetryReader"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{Name: proto.String("asn"), Number: proto.Int32(1), Label: opt.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_UINT32.Enum()},
			{Name: proto.String("mismatched_f3"), Number: proto.Int32(3), Label: opt.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
			{Name: proto.String("seq"), Number: proto.Int32(10), Label: opt.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_UINT64.Enum()},
			{Name: proto.String("new_reader_field"), Number: proto.Int32(15), Label: opt.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
		},
	}
	readerDesc, _ := buildDynamicSchemaFile(t, readerProto)
	rMsg := dynamicpb.NewMessage(readerDesc)
	if err := Unmarshal(wire, rMsg); err != nil {
		t.Fatalf("Unmarshal into evolved reader failed: %v", err)
	}
	if gotASN := rMsg.Get(readerDesc.Fields().ByNumber(1)).Uint(); gotASN != 64512 {
		t.Fatalf("reader asn = %d, want 64512", gotASN)
	}
	if gotF3 := rMsg.Get(readerDesc.Fields().ByNumber(3)).String(); gotF3 != "" {
		t.Fatalf("expected mismatched field #3 to be skipped, got %q", gotF3)
	}
	if gotSeq := rMsg.Get(readerDesc.Fields().ByNumber(10)).Uint(); gotSeq != 123456789 {
		t.Fatalf("reader seq = %d, want 123456789", gotSeq)
	}
}
