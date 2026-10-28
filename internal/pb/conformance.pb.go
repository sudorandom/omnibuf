// Code generated for omnibuf/conformance/conformance.proto. DO NOT EDIT.
package pb

import (
	reflect "reflect"

	proto "google.golang.org/protobuf/proto"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	descriptorpb "google.golang.org/protobuf/types/descriptorpb"
)

type AllTypesSubMsg struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id    uint32 `protobuf:"varint,1,opt,name=id,proto3" json:"id,omitempty"`
	Label string `protobuf:"bytes,2,opt,name=label,proto3" json:"label,omitempty"`
	Flag  bool   `protobuf:"varint,3,opt,name=flag,proto3" json:"flag,omitempty"`
	Score int64  `protobuf:"zigzag64,4,opt,name=score,proto3" json:"score,omitempty"`
}

func (x *AllTypesSubMsg) Reset() {
	*x = AllTypesSubMsg{}
	if protoimpl.UnsafeEnabled {
		mi := &file_conformance_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *AllTypesSubMsg) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AllTypesSubMsg) ProtoMessage() {}

func (x *AllTypesSubMsg) ProtoReflect() protoreflect.Message {
	mi := &file_conformance_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

type TestAllTypes struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	F1Uint32       uint32            `protobuf:"varint,1,opt,name=f1_uint32,json=f1Uint32,proto3" json:"f1_uint32,omitempty"`
	F2Uint64       uint64            `protobuf:"varint,2,opt,name=f2_uint64,json=f2Uint64,proto3" json:"f2_uint64,omitempty"`
	F3Sint32       int32             `protobuf:"zigzag32,3,opt,name=f3_sint32,json=f3Sint32,proto3" json:"f3_sint32,omitempty"`
	F4Sint64       int64             `protobuf:"zigzag64,4,opt,name=f4_sint64,json=f4Sint64,proto3" json:"f4_sint64,omitempty"`
	F5Bool         bool              `protobuf:"varint,5,opt,name=f5_bool,json=f5Bool,proto3" json:"f5_bool,omitempty"`
	F6String       string            `protobuf:"bytes,6,opt,name=f6_string,json=f6String,proto3" json:"f6_string,omitempty"`
	F7Bytes        []byte            `protobuf:"bytes,7,opt,name=f7_bytes,json=f7Bytes,proto3" json:"f7_bytes,omitempty"`
	F8SubMsg       *AllTypesSubMsg   `protobuf:"bytes,8,opt,name=f8_sub_msg,json=f8SubMsg,proto3" json:"f8_sub_msg,omitempty"`
	F9StringArr    []string          `protobuf:"bytes,9,rep,name=f9_string_arr,json=f9StringArr,proto3" json:"f9_string_arr,omitempty"`
	F10Uint32Arr   []uint32          `protobuf:"varint,10,rep,packed,name=f10_uint32_arr,json=f10Uint32Arr,proto3" json:"f10_uint32_arr,omitempty"`
	F11SubMsgArr   []*AllTypesSubMsg `protobuf:"bytes,11,rep,name=f11_sub_msg_arr,json=f11SubMsgArr,proto3" json:"f11_sub_msg_arr,omitempty"`
	F12MapStrUint  map[string]uint64 `protobuf:"bytes,12,rep,name=f12_map_str_uint,json=f12MapStrUint,proto3" json:"f12_map_str_uint,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"varint,2,opt,name=value,proto3"`
	F13Float32     float32           `protobuf:"fixed32,13,opt,name=f13_float32,json=f13Float32,proto3" json:"f13_float32,omitempty"`
	F14Float64     float64           `protobuf:"fixed64,14,opt,name=f14_float64,json=f14Float64,proto3" json:"f14_float64,omitempty"`
	F15Fixed32     uint32            `protobuf:"fixed32,15,opt,name=f15_fixed32,json=f15Fixed32,proto3" json:"f15_fixed32,omitempty"`
	F16Fixed64     uint64            `protobuf:"fixed64,16,opt,name=f16_fixed64,json=f16Fixed64,proto3" json:"f16_fixed64,omitempty"`
	F20OverflowStr string            `protobuf:"bytes,20,opt,name=f20_overflow_str,json=f20OverflowStr,proto3" json:"f20_overflow_str,omitempty"`
}

func (x *TestAllTypes) Reset() {
	*x = TestAllTypes{}
	if protoimpl.UnsafeEnabled {
		mi := &file_conformance_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *TestAllTypes) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TestAllTypes) ProtoMessage() {}

func (x *TestAllTypes) ProtoReflect() protoreflect.Message {
	mi := &file_conformance_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

var File_conformance_proto protoreflect.FileDescriptor

var file_conformance_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_conformance_proto_goTypes = []any{
	(*AllTypesSubMsg)(nil), // 0: omnibuf.conformance.AllTypesSubMsg
	(*TestAllTypes)(nil),   // 1: omnibuf.conformance.TestAllTypes
	nil,                    // 2: omnibuf.conformance.TestAllTypes.F12MapStrUintEntry
}
var file_conformance_proto_depIdxs = []int32{
	0, // 0: omnibuf.conformance.TestAllTypes.f8_sub_msg:type_name -> omnibuf.conformance.AllTypesSubMsg
	0, // 1: omnibuf.conformance.TestAllTypes.f11_sub_msg_arr:type_name -> omnibuf.conformance.AllTypesSubMsg
	2, // 2: omnibuf.conformance.TestAllTypes.f12_map_str_uint:type_name -> omnibuf.conformance.TestAllTypes.F12MapStrUintEntry
	3, // [3:3] is the sub-list for method output_type
	3, // [3:3] is the sub-list for method input_type
	3, // [3:3] is the sub-list for extension type_name
	3, // [3:3] is the sub-list for extension extendee
	0, // [0:3] is the sub-list for field type_name
}

func init() { file_conformance_proto_init() }
func file_conformance_proto_init() {
	if File_conformance_proto != nil {
		return
	}
	f := func(name string, num int32, label descriptorpb.FieldDescriptorProto_Label, typ descriptorpb.FieldDescriptorProto_Type, typeName string) *descriptorpb.FieldDescriptorProto {
		fd := &descriptorpb.FieldDescriptorProto{
			Name:   proto.String(name),
			Number: proto.Int32(num),
			Label:  label.Enum(),
			Type:   typ.Enum(),
		}
		if typeName != "" {
			fd.TypeName = proto.String(typeName)
		}
		return fd
	}
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	rep := descriptorpb.FieldDescriptorProto_LABEL_REPEATED
	fdp := &descriptorpb.FileDescriptorProto{
		Syntax:  proto.String("proto3"),
		Name:    proto.String("omnibuf/conformance/conformance.proto"),
		Package: proto.String("omnibuf.conformance"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("AllTypesSubMsg"),
				Field: []*descriptorpb.FieldDescriptorProto{
					f("id", 1, opt, descriptorpb.FieldDescriptorProto_TYPE_UINT32, ""),
					f("label", 2, opt, descriptorpb.FieldDescriptorProto_TYPE_STRING, ""),
					f("flag", 3, opt, descriptorpb.FieldDescriptorProto_TYPE_BOOL, ""),
					f("score", 4, opt, descriptorpb.FieldDescriptorProto_TYPE_SINT64, ""),
				},
			},
			{
				Name: proto.String("TestAllTypes"),
				Field: []*descriptorpb.FieldDescriptorProto{
					f("f1_uint32", 1, opt, descriptorpb.FieldDescriptorProto_TYPE_UINT32, ""),
					f("f2_uint64", 2, opt, descriptorpb.FieldDescriptorProto_TYPE_UINT64, ""),
					f("f3_sint32", 3, opt, descriptorpb.FieldDescriptorProto_TYPE_SINT32, ""),
					f("f4_sint64", 4, opt, descriptorpb.FieldDescriptorProto_TYPE_SINT64, ""),
					f("f5_bool", 5, opt, descriptorpb.FieldDescriptorProto_TYPE_BOOL, ""),
					f("f6_string", 6, opt, descriptorpb.FieldDescriptorProto_TYPE_STRING, ""),
					f("f7_bytes", 7, opt, descriptorpb.FieldDescriptorProto_TYPE_BYTES, ""),
					f("f8_sub_msg", 8, opt, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, ".omnibuf.conformance.AllTypesSubMsg"),
					f("f9_string_arr", 9, rep, descriptorpb.FieldDescriptorProto_TYPE_STRING, ""),
					f("f10_uint32_arr", 10, rep, descriptorpb.FieldDescriptorProto_TYPE_UINT32, ""),
					f("f11_sub_msg_arr", 11, rep, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, ".omnibuf.conformance.AllTypesSubMsg"),
					f("f12_map_str_uint", 12, rep, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, ".omnibuf.conformance.TestAllTypes.F12MapStrUintEntry"),
					f("f13_float32", 13, opt, descriptorpb.FieldDescriptorProto_TYPE_FLOAT, ""),
					f("f14_float64", 14, opt, descriptorpb.FieldDescriptorProto_TYPE_DOUBLE, ""),
					f("f15_fixed32", 15, opt, descriptorpb.FieldDescriptorProto_TYPE_FIXED32, ""),
					f("f16_fixed64", 16, opt, descriptorpb.FieldDescriptorProto_TYPE_FIXED64, ""),
					f("f20_overflow_str", 20, opt, descriptorpb.FieldDescriptorProto_TYPE_STRING, ""),
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("F12MapStrUintEntry"),
						Options: &descriptorpb.MessageOptions{
							MapEntry: proto.Bool(true),
						},
						Field: []*descriptorpb.FieldDescriptorProto{
							f("key", 1, opt, descriptorpb.FieldDescriptorProto_TYPE_STRING, ""),
							f("value", 2, opt, descriptorpb.FieldDescriptorProto_TYPE_UINT64, ""),
						},
					},
				},
			},
		},
	}
	rawDesc, err := proto.Marshal(fdp)
	if err != nil {
		panic(err)
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: rawDesc,
			NumEnums:      0,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_conformance_proto_goTypes,
		DependencyIndexes: file_conformance_proto_depIdxs,
		MessageInfos:      file_conformance_proto_msgTypes,
	}.Build()
	File_conformance_proto = out.File
	file_conformance_proto_goTypes = nil
	file_conformance_proto_depIdxs = nil
}
