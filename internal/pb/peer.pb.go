// Code generated for omnibuf/bench/peer.proto. DO NOT EDIT.
package pb

import (
	proto "google.golang.org/protobuf/proto"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	descriptorpb "google.golang.org/protobuf/types/descriptorpb"
	reflect "reflect"
)

type Location struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Region     string  `protobuf:"bytes,1,opt,name=region,proto3" json:"region,omitempty"`
	Datacenter string  `protobuf:"bytes,2,opt,name=datacenter,proto3" json:"datacenter,omitempty"`
	City       string  `protobuf:"bytes,3,opt,name=city,proto3" json:"city,omitempty"`
	Rack       uint32  `protobuf:"varint,4,opt,name=rack,proto3" json:"rack,omitempty"`
	Latitude   float32 `protobuf:"fixed32,5,opt,name=latitude,proto3" json:"latitude,omitempty"`
	Longitude  float32 `protobuf:"fixed32,6,opt,name=longitude,proto3" json:"longitude,omitempty"`
}

func (x *Location) Reset() {
	*x = Location{}
	if protoimpl.UnsafeEnabled {
		mi := &file_schemas_peer_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Location) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Location) ProtoMessage() {}

func (x *Location) ProtoReflect() protoreflect.Message {
	mi := &file_schemas_peer_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (x *Location) GetCity() string {
	if x != nil {
		return x.City
	}
	return ""
}

type Neighbor struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	PeerAsn     uint32 `protobuf:"varint,1,opt,name=peer_asn,json=peerAsn,proto3" json:"peer_asn,omitempty"`
	Router      string `protobuf:"bytes,2,opt,name=router,proto3" json:"router,omitempty"`
	LatencyUs   uint32 `protobuf:"varint,3,opt,name=latency_us,json=latencyUs,proto3" json:"latency_us,omitempty"`
	Established bool   `protobuf:"varint,4,opt,name=established,proto3" json:"established,omitempty"`
}

func (x *Neighbor) Reset() {
	*x = Neighbor{}
	if protoimpl.UnsafeEnabled {
		mi := &file_schemas_peer_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Neighbor) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Neighbor) ProtoMessage() {}

func (x *Neighbor) ProtoReflect() protoreflect.Message {
	mi := &file_schemas_peer_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (x *Neighbor) GetRouter() string {
	if x != nil {
		return x.Router
	}
	return ""
}

type PeerUpdate struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Asn       uint32      `protobuf:"varint,1,opt,name=asn,proto3" json:"asn,omitempty"`
	RouterId  string      `protobuf:"bytes,2,opt,name=router_id,json=routerId,proto3" json:"router_id,omitempty"`
	Active    bool        `protobuf:"varint,3,opt,name=active,proto3" json:"active,omitempty"`
	UptimeSec uint64      `protobuf:"varint,4,opt,name=uptime_sec,json=uptimeSec,proto3" json:"uptime_sec,omitempty"`
	RssiDbm   int32       `protobuf:"zigzag32,5,opt,name=rssi_dbm,json=rssiDbm,proto3" json:"rssi_dbm,omitempty"`
	CpuLoad   float64     `protobuf:"fixed64,6,opt,name=cpu_load,json=cpuLoad,proto3" json:"cpu_load,omitempty"`
	Tags      []string    `protobuf:"bytes,7,rep,name=tags,proto3" json:"tags,omitempty"`
	Prefixes  []uint32    `protobuf:"varint,8,rep,packed,name=prefixes,proto3" json:"prefixes,omitempty"`
	Location  *Location   `protobuf:"bytes,9,opt,name=location,proto3" json:"location,omitempty"`
	Neighbors []*Neighbor `protobuf:"bytes,10,rep,name=neighbors,proto3" json:"neighbors,omitempty"`
	Sequence  uint64      `protobuf:"varint,11,opt,name=sequence,proto3" json:"sequence,omitempty"`
}

func (x *PeerUpdate) Reset() {
	*x = PeerUpdate{}
	if protoimpl.UnsafeEnabled {
		mi := &file_schemas_peer_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PeerUpdate) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PeerUpdate) ProtoMessage() {}

func (x *PeerUpdate) ProtoReflect() protoreflect.Message {
	mi := &file_schemas_peer_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (x *PeerUpdate) GetLocation() *Location {
	if x != nil {
		return x.Location
	}
	return nil
}

func (x *PeerUpdate) GetNeighbors() []*Neighbor {
	if x != nil {
		return x.Neighbors
	}
	return nil
}

var File_schemas_peer_proto protoreflect.FileDescriptor

var file_schemas_peer_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_schemas_peer_proto_goTypes = []any{
	(*Location)(nil),   // 0: omnibuf.bench.Location
	(*Neighbor)(nil),   // 1: omnibuf.bench.Neighbor
	(*PeerUpdate)(nil), // 2: omnibuf.bench.PeerUpdate
}
var file_schemas_peer_proto_depIdxs = []int32{
	0, // 0: omnibuf.bench.PeerUpdate.location:type_name -> omnibuf.bench.Location
	1, // 1: omnibuf.bench.PeerUpdate.neighbors:type_name -> omnibuf.bench.Neighbor
	2, // [2:2] is the sub-list for method output_type
	2, // [2:2] is the sub-list for method input_type
	2, // [2:2] is the sub-list for extension type_name
	2, // [2:2] is the sub-list for extension extendee
	0, // [0:2] is the sub-list for field type_name
}

func init() { file_schemas_peer_proto_init() }
func file_schemas_peer_proto_init() {
	if File_schemas_peer_proto != nil {
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
		Name:    proto.String("omnibuf/bench/peer.proto"),
		Package: proto.String("omnibuf.bench"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("Location"),
				Field: []*descriptorpb.FieldDescriptorProto{
					f("region", 1, opt, descriptorpb.FieldDescriptorProto_TYPE_STRING, ""),
					f("datacenter", 2, opt, descriptorpb.FieldDescriptorProto_TYPE_STRING, ""),
					f("city", 3, opt, descriptorpb.FieldDescriptorProto_TYPE_STRING, ""),
					f("rack", 4, opt, descriptorpb.FieldDescriptorProto_TYPE_UINT32, ""),
					f("latitude", 5, opt, descriptorpb.FieldDescriptorProto_TYPE_FLOAT, ""),
					f("longitude", 6, opt, descriptorpb.FieldDescriptorProto_TYPE_FLOAT, ""),
				},
			},
			{
				Name: proto.String("Neighbor"),
				Field: []*descriptorpb.FieldDescriptorProto{
					f("peer_asn", 1, opt, descriptorpb.FieldDescriptorProto_TYPE_UINT32, ""),
					f("router", 2, opt, descriptorpb.FieldDescriptorProto_TYPE_STRING, ""),
					f("latency_us", 3, opt, descriptorpb.FieldDescriptorProto_TYPE_UINT32, ""),
					f("established", 4, opt, descriptorpb.FieldDescriptorProto_TYPE_BOOL, ""),
				},
			},
			{
				Name: proto.String("PeerUpdate"),
				Field: []*descriptorpb.FieldDescriptorProto{
					f("asn", 1, opt, descriptorpb.FieldDescriptorProto_TYPE_UINT32, ""),
					f("router_id", 2, opt, descriptorpb.FieldDescriptorProto_TYPE_STRING, ""),
					f("active", 3, opt, descriptorpb.FieldDescriptorProto_TYPE_BOOL, ""),
					f("uptime_sec", 4, opt, descriptorpb.FieldDescriptorProto_TYPE_UINT64, ""),
					f("rssi_dbm", 5, opt, descriptorpb.FieldDescriptorProto_TYPE_SINT32, ""),
					f("cpu_load", 6, opt, descriptorpb.FieldDescriptorProto_TYPE_DOUBLE, ""),
					f("tags", 7, rep, descriptorpb.FieldDescriptorProto_TYPE_STRING, ""),
					f("prefixes", 8, rep, descriptorpb.FieldDescriptorProto_TYPE_UINT32, ""),
					f("location", 9, opt, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, ".omnibuf.bench.Location"),
					f("neighbors", 10, rep, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, ".omnibuf.bench.Neighbor"),
					f("sequence", 11, opt, descriptorpb.FieldDescriptorProto_TYPE_UINT64, ""),
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
		GoTypes:           file_schemas_peer_proto_goTypes,
		DependencyIndexes: file_schemas_peer_proto_depIdxs,
		MessageInfos:      file_schemas_peer_proto_msgTypes,
	}.Build()
	File_schemas_peer_proto = out.File
	file_schemas_peer_proto_goTypes = nil
	file_schemas_peer_proto_depIdxs = nil
}
