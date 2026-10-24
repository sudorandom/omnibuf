package main

import (
	"flag"
	"fmt"
	"os"

	_ "github.com/planetscale/vtprotobuf/features/marshal"
	_ "github.com/planetscale/vtprotobuf/features/pool"
	_ "github.com/planetscale/vtprotobuf/features/size"
	_ "github.com/planetscale/vtprotobuf/features/unmarshal"
	vtgenerator "github.com/planetscale/vtprotobuf/generator"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func main() {
	selfGenOut := flag.String("self-gen", "", "Run protogen on schemas/peer.proto descriptor and write generated file to path")
	vtGenOut := flag.String("vtproto-gen", "", "Run vtprotobuf generator on schemas/peer.proto descriptor and write generated file to path")
	flag.Parse()

	if *selfGenOut != "" {
		if err := runSelfGen(*selfGenOut); err != nil {
			fmt.Fprintf(os.Stderr, "protoc-gen-omnibuf: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if *vtGenOut != "" {
		if err := runVTProtoGen(*vtGenOut); err != nil {
			fmt.Fprintf(os.Stderr, "vtproto-gen: %v\n", err)
			os.Exit(1)
		}
		return
	}

	protogen.Options{}.Run(func(gen *protogen.Plugin) error {
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		for _, f := range gen.Files {
			if !f.Generate {
				continue
			}
			generateFile(gen, f)
		}
		return nil
	})
}

func runSelfGen(outPath string) error {
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
		Name:    proto.String("schemas/peer.proto"),
		Package: proto.String("omnibuf.bench"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String("github.com/sudorandom/omnibuf/pb;pb"),
		},
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

	req := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{"schemas/peer.proto"},
		ProtoFile:      []*descriptorpb.FileDescriptorProto{fdp},
	}
	gen, err := protogen.Options{}.New(req)
	if err != nil {
		return err
	}
	for _, file := range gen.Files {
		if file.Generate {
			generateFile(gen, file)
		}
	}
	resp := gen.Response()
	if resp.Error != nil {
		return fmt.Errorf("%s", resp.GetError())
	}
	for _, rf := range resp.File {
		if err := os.WriteFile(outPath, []byte(rf.GetContent()), 0644); err != nil {
			return err
		}
	}
	return nil
}

func runVTProtoGen(outPath string) error {
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
		Name:    proto.String("schemas/peer.proto"),
		Package: proto.String("omnibuf.bench"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String("github.com/sudorandom/omnibuf/pb;pb"),
		},
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

	req := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{"schemas/peer.proto"},
		ProtoFile:      []*descriptorpb.FileDescriptorProto{fdp},
	}
	plugin, err := protogen.Options{}.New(req)
	if err != nil {
		return err
	}
	cfg := vtgenerator.Config{
		Poolable:        vtgenerator.NewObjectSet(),
		PoolableExclude: vtgenerator.NewObjectSet(),
	}
	_ = cfg.Poolable.Set("github.com/sudorandom/omnibuf/pb.*")
	gen, err := vtgenerator.NewGenerator(plugin, []string{"marshal", "size", "unmarshal", "unmarshal_unsafe", "pool"}, &cfg)
	if err != nil {
		return err
	}
	gen.Generate()
	resp := plugin.Response()
	if resp.Error != nil {
		return fmt.Errorf("%s", resp.GetError())
	}
	for _, rf := range resp.File {
		if err := os.WriteFile(outPath, []byte(rf.GetContent()), 0644); err != nil {
			return err
		}
	}
	return nil
}

func wireTypeForField(field *protogen.Field) uint8 {
	if field.Desc.IsList() {
		switch field.Desc.Kind() {
		case protoreflect.StringKind:
			return 6 // WireStringArr
		case protoreflect.MessageKind:
			return 8 // WireSubMsgArr
		case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Int64Kind, protoreflect.Sint64Kind:
			return 13 // WireSintArr
		case protoreflect.FloatKind, protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind:
			return 14 // WireFixed32Arr
		case protoreflect.DoubleKind, protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind:
			return 15 // WireFixed64Arr
		default:
			return 7 // WireUintArr
		}
	}
	switch field.Desc.Kind() {
	case protoreflect.BoolKind:
		return 2 // WireBoolTrue (or 1 for false)
	case protoreflect.StringKind:
		return 3 // WireString
	case protoreflect.BytesKind:
		return 4 // WireBytes
	case protoreflect.MessageKind:
		return 5 // WireSubMsg
	case protoreflect.FloatKind, protoreflect.Fixed32Kind:
		return 10 // WireFixed32
	case protoreflect.DoubleKind, protoreflect.Fixed64Kind:
		return 11 // WireFixed64
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Int64Kind, protoreflect.Sint64Kind:
		return 12 // WireSint
	default:
		return 0 // WireUint
	}
}

func generateFile(gen *protogen.Plugin, file *protogen.File) {
	filename := file.GeneratedFilenamePrefix + "_omnibuf.pb.go"
	g := gen.NewGeneratedFile(filename, file.GoImportPath)
	g.P("// Code generated by protoc-gen-omnibuf. DO NOT EDIT.")
	g.P("// source: ", file.Desc.Path())
	g.P()
	g.P("package ", file.GoPackageName)
	g.P()
	g.P("import (")
	g.P(`	"encoding/binary"`)
	g.P(`	"fmt"`)
	g.P(`	"math"`)
	g.P()
	g.P(`	"github.com/sudorandom/omnibuf"`)
	g.P(")")
	g.P()

	for _, msg := range file.Messages {
		generateOpenStructCodec(g, msg)
		generateOpaqueMessage(g, msg)
	}
}

// generateOpenStructCodec generates single-pass MarshalOmniBufAppend and UnmarshalOmniBuf
// methods directly onto the standard protoc-gen-go struct (e.g. *PeerUpdate).
func generateOpenStructCodec(g *protogen.GeneratedFile, msg *protogen.Message) {
	typeName := msg.GoIdent.GoName

	g.P("// MarshalOmniBufAppend encodes ", typeName, " in a single forward pass using generated field tags.")
	g.P("func (x *", typeName, ") MarshalOmniBufAppend(dst []byte) []byte {")
	for i, f := range msg.Fields {
		fnum := uint16(f.Desc.Number())
		wt := wireTypeForField(f)
		rawTag := (fnum << 4) | uint16(wt)
		if f.Desc.Kind() == protoreflect.BoolKind && !f.Desc.IsList() {
			tTrue := byte(((fnum<<4)|2)<<1) | 1
			tFalse := byte(((fnum<<4)|1)<<1) | 1
			g.P("	if x.", f.GoName, " {")
			g.P(fmt.Sprintf("		dst = append(dst, 0x%02x)", tTrue))
			g.P("	} else {")
			g.P(fmt.Sprintf("		dst = append(dst, 0x%02x)", tFalse))
			g.P("	}")
			continue
		}
		if rawTag <= 127 {
			tag1 := byte(rawTag<<1) | 1
			g.P(fmt.Sprintf("	dst = append(dst, 0x%02x)", tag1))
		} else {
			tag2 := (rawTag << 2) | 2
			g.P(fmt.Sprintf("	dst = append(dst, 0x%02x, 0x%02x)", byte(tag2), byte(tag2>>8)))
		}

		if f.Desc.IsList() {
			switch f.Desc.Kind() {
			case protoreflect.StringKind:
				g.P("	lenPos_", i, " := len(dst)")
				g.P("	dst = append(dst, 0, 0)")
				g.P("	dst = omnibuf.AppendVarint(dst, uint64(len(x.", f.GoName, ")))")
				g.P("	for _, s := range x.", f.GoName, " {")
				g.P("		dst = omnibuf.AppendVarint(dst, uint64(len(s)))")
				g.P("		dst = append(dst, s...)")
				g.P("	}")
				g.P("	dst = omnibuf.BackpatchContainerLen(dst, lenPos_", i, ")")
			case protoreflect.Uint32Kind:
				g.P("	lenPos_", i, " := len(dst)")
				g.P("	dst = append(dst, 0, 0)")
				g.P("	dst = omnibuf.AppendVarint(dst, uint64(len(x.", f.GoName, ")))")
				g.P("	for _, v := range x.", f.GoName, " {")
				g.P("		dst = omnibuf.AppendVarint(dst, uint64(v))")
				g.P("	}")
				g.P("	dst = omnibuf.BackpatchContainerLen(dst, lenPos_", i, ")")
			case protoreflect.MessageKind:
				g.P("	lenPos_", i, " := len(dst)")
				g.P("	dst = append(dst, 0, 0)")
				g.P("	for _, elem := range x.", f.GoName, " {")
				g.P("		eLenPos := len(dst)")
				g.P("		dst = append(dst, 0, 0)")
				g.P("		if elem != nil {")
				g.P("			dst = elem.MarshalOmniBufAppend(dst)")
				g.P("		}")
				g.P("		dst = omnibuf.BackpatchContainerLen(dst, eLenPos)")
				g.P("	}")
				g.P("	dst = omnibuf.BackpatchContainerLen(dst, lenPos_", i, ")")
			}
			continue
		}

		switch f.Desc.Kind() {
		case protoreflect.Uint32Kind:
			g.P("	dst = omnibuf.AppendVarint(dst, uint64(x.", f.GoName, "))")
		case protoreflect.Uint64Kind:
			g.P("	dst = omnibuf.AppendVarint(dst, x.", f.GoName, ")")
		case protoreflect.Sint32Kind, protoreflect.Int32Kind:
			g.P("	dst = omnibuf.AppendVarint(dst, omnibuf.EncodeZigZag(int64(x.", f.GoName, ")))")
		case protoreflect.FloatKind:
			g.P("	b_", i, " := math.Float32bits(x.", f.GoName, ")")
			g.P("	dst = append(dst, byte(b_", i, "), byte(b_", i, ">>8), byte(b_", i, ">>16), byte(b_", i, ">>24))")
		case protoreflect.DoubleKind:
			g.P("	b_", i, " := math.Float64bits(x.", f.GoName, ")")
			g.P("	dst = append(dst, byte(b_", i, "), byte(b_", i, ">>8), byte(b_", i, ">>16), byte(b_", i, ">>24), byte(b_", i, ">>32), byte(b_", i, ">>40), byte(b_", i, ">>48), byte(b_", i, ">>56))")
		case protoreflect.StringKind:
			g.P("	dst = omnibuf.AppendVarint(dst, uint64(len(x.", f.GoName, ")))")
			g.P("	dst = append(dst, x.", f.GoName, "...)")
		case protoreflect.MessageKind:
			g.P("	lenPos_", i, " := len(dst)")
			g.P("	dst = append(dst, 0, 0)")
			g.P("	if x.", f.GoName, " != nil {")
			g.P("		dst = x.", f.GoName, ".MarshalOmniBufAppend(dst)")
			g.P("	}")
			g.P("	dst = omnibuf.BackpatchContainerLen(dst, lenPos_", i, ")")
		}
	}
	g.P("	return dst")
	g.P("}")
	g.P()

	// Generate UnmarshalOmniBuf on *typeName
	g.P("// UnmarshalOmniBuf decodes data into ", typeName, " using generated switch dispatch.")
	g.P("func (x *", typeName, ") UnmarshalOmniBuf(data []byte, zeroCopy bool) error {")
	g.P("	n := len(data)")
	g.P("	pos := 0")
	g.P("	var seenMask uint64")
	g.P("	_ = seenMask")
	g.P("	for pos < n {")
	g.P("		b0 := data[pos]")
	g.P("		var tag uint16")
	g.P("		if (b0 & 1) != 0 {")
	g.P("			tag = uint16(b0 >> 1)")
	g.P("			pos++")
	g.P("		} else if (b0 & 2) != 0 {")
	g.P("			if pos+2 > n {")
	g.P(`				return fmt.Errorf("omnibuf: truncated tag")`)
	g.P("			}")
	g.P("			tag = binary.LittleEndian.Uint16(data[pos:]) >> 2")
	g.P("			pos += 2")
	g.P("		} else {")
	g.P("			t64, tn := omnibuf.DecodeVarint(data[pos:])")
	g.P("			if tn == 0 {")
	g.P(`				return fmt.Errorf("omnibuf: truncated multi-byte tag")`)
	g.P("			}")
	g.P("			tag = uint16(t64)")
	g.P("			pos += tn")
	g.P("		}")
	g.P("		fieldNum := tag >> 4")
	g.P("		wt := uint8(tag & 0x0F)")
	g.P("		switch fieldNum {")
	for _, f := range msg.Fields {
		fnum := uint16(f.Desc.Number())
		g.P("		case ", fnum, ":")
		if f.Desc.IsList() {
			switch f.Desc.Kind() {
			case protoreflect.StringKind:
				g.P("			byteLen, bn := omnibuf.DecodeVarint(data[pos:])")
				g.P("			if bn == 0 || byteLen == 0 || byteLen > uint64(n-(pos+bn)) {")
				g.P(`				return fmt.Errorf("omnibuf: invalid string list byteLen")`)
				g.P("			}")
				g.P("			pos += bn")
				g.P("			end := pos + int(byteLen)")
				g.P("			c64, cn := omnibuf.DecodeVarint(data[pos:end])")
				g.P("			if cn == 0 || c64 > uint64(end-(pos+cn)) {")
				g.P(`				return fmt.Errorf("omnibuf: invalid string list count")`)
				g.P("			}")
				g.P("			p := pos + cn")
				g.P("			c := int(c64)")
				g.P("			start := 0")
				g.P("			if (seenMask & (uint64(1) << ", fnum, ")) != 0 {")
				g.P("				start = len(x.", f.GoName, ")")
				g.P("			}")
				g.P("			seenMask |= (uint64(1) << ", fnum, ")")
				g.P("			total := start + c")
				g.P("			if cap(x.", f.GoName, ") < total {")
				g.P("				next := make([]string, total)")
				g.P("				copy(next, x.", f.GoName, "[:start])")
				g.P("				x.", f.GoName, " = next")
				g.P("			} else {")
				g.P("				x.", f.GoName, " = x.", f.GoName, "[:total]")
				g.P("			}")
				g.P("			for i := 0; i < c; i++ {")
				g.P("				sLen64, sn := omnibuf.DecodeVarint(data[p:end])")
				g.P("				if sn == 0 || sLen64 > uint64(end-(p+sn)) {")
				g.P(`					return fmt.Errorf("omnibuf: invalid string elem length")`)
				g.P("				}")
				g.P("				p += sn")
				g.P("				sLen := int(sLen64)")
				g.P("				x.", f.GoName, "[start+i] = omnibuf.DecodeStr(data[p:p+sLen], zeroCopy)")
				g.P("				p += sLen")
				g.P("			}")
				g.P("			if p != end {")
				g.P(`				return fmt.Errorf("omnibuf: string list byteLen mismatch")`)
				g.P("			}")
				g.P("			pos = end")
			case protoreflect.Uint32Kind:
				g.P("			byteLen, bn := omnibuf.DecodeVarint(data[pos:])")
				g.P("			if bn == 0 || byteLen == 0 || byteLen > uint64(n-(pos+bn)) {")
				g.P(`				return fmt.Errorf("omnibuf: invalid uint32 list byteLen")`)
				g.P("			}")
				g.P("			pos += bn")
				g.P("			end := pos + int(byteLen)")
				g.P("			c64, cn := omnibuf.DecodeVarint(data[pos:end])")
				g.P("			if cn == 0 || c64 > uint64(end-(pos+cn)) {")
				g.P(`				return fmt.Errorf("omnibuf: invalid uint32 list count")`)
				g.P("			}")
				g.P("			p := pos + cn")
				g.P("			c := int(c64)")
				g.P("			start := 0")
				g.P("			if (seenMask & (uint64(1) << ", fnum, ")) != 0 {")
				g.P("				start = len(x.", f.GoName, ")")
				g.P("			}")
				g.P("			seenMask |= (uint64(1) << ", fnum, ")")
				g.P("			total := start + c")
				g.P("			if cap(x.", f.GoName, ") < total {")
				g.P("				next := make([]uint32, total)")
				g.P("				copy(next, x.", f.GoName, "[:start])")
				g.P("				x.", f.GoName, " = next")
				g.P("			} else {")
				g.P("				x.", f.GoName, " = x.", f.GoName, "[:total]")
				g.P("			}")
				g.P("			for i := 0; i < c; i++ {")
				g.P("				v, vn := omnibuf.DecodeVarint(data[p:end])")
				g.P("				if vn == 0 {")
				g.P(`					return fmt.Errorf("omnibuf: truncated uint32 list elem")`)
				g.P("				}")
				g.P("				p += vn")
				g.P("				x.", f.GoName, "[start+i] = uint32(v)")
				g.P("			}")
				g.P("			if p != end {")
				g.P(`				return fmt.Errorf("omnibuf: uint32 list byteLen mismatch")`)
				g.P("			}")
				g.P("			pos = end")
			case protoreflect.MessageKind:
				elemType := f.Message.GoIdent.GoName
				g.P("			arrLen, an := omnibuf.DecodeVarint(data[pos:])")
				g.P("			if an == 0 || arrLen > uint64(n-(pos+an)) {")
				g.P(`				return fmt.Errorf("omnibuf: invalid WireSubMsgArr length")`)
				g.P("			}")
				g.P("			pos += an")
				g.P("			arrPayload := data[pos : pos+int(arrLen)]")
				g.P("			elemCount, ok := omnibuf.CountSubMsgArrElems(arrPayload)")
				g.P("			if !ok {")
				g.P(`				return fmt.Errorf("omnibuf: corrupt WireSubMsgArr")`)
				g.P("			}")
				g.P("			start := 0")
				g.P("			if (seenMask & (uint64(1) << ", fnum, ")) != 0 {")
				g.P("				start = len(x.", f.GoName, ")")
				g.P("			}")
				g.P("			seenMask |= (uint64(1) << ", fnum, ")")
				g.P("			total := start + elemCount")
				g.P("			if cap(x.", f.GoName, ") < total {")
				g.P("				slab := make([]", elemType, ", elemCount)")
				g.P("				next := make([]*", elemType, ", total)")
				g.P("				copy(next, x.", f.GoName, "[:start])")
				g.P("				for i := 0; i < elemCount; i++ {")
				g.P("					next[start+i] = &slab[i]")
				g.P("				}")
				g.P("				x.", f.GoName, " = next")
				g.P("			} else {")
				g.P("				x.", f.GoName, " = x.", f.GoName, "[:total]")
				g.P("			}")
				g.P("			p := 0")
				g.P("			for i := 0; i < elemCount; i++ {")
				g.P("				el, en := omnibuf.DecodeVarint(arrPayload[p:])")
				g.P("				p += en")
				g.P("				if x.", f.GoName, "[start+i] == nil {")
				g.P("					x.", f.GoName, "[start+i] = new(", elemType, ")")
				g.P("				}")
				g.P("				if err := x.", f.GoName, "[start+i].UnmarshalOmniBuf(arrPayload[p:p+int(el)], zeroCopy); err != nil {")
				g.P("					return err")
				g.P("				}")
				g.P("				p += int(el)")
				g.P("			}")
				g.P("			pos += int(arrLen)")
			}
			continue
		}
		switch f.Desc.Kind() {
		case protoreflect.Uint32Kind:
			g.P("			v, vn := omnibuf.DecodeVarint(data[pos:])")
			g.P("			if vn == 0 {")
			g.P(`				return fmt.Errorf("omnibuf: truncated uint32")`)
			g.P("			}")
			g.P("			pos += vn")
			g.P("			x.", f.GoName, " = uint32(v)")
		case protoreflect.Uint64Kind:
			g.P("			v, vn := omnibuf.DecodeVarint(data[pos:])")
			g.P("			if vn == 0 {")
			g.P(`				return fmt.Errorf("omnibuf: truncated uint64")`)
			g.P("			}")
			g.P("			pos += vn")
			g.P("			x.", f.GoName, " = v")
		case protoreflect.Sint32Kind, protoreflect.Int32Kind:
			g.P("			v, vn := omnibuf.DecodeVarint(data[pos:])")
			g.P("			if vn == 0 {")
			g.P(`				return fmt.Errorf("omnibuf: truncated sint32")`)
			g.P("			}")
			g.P("			pos += vn")
			g.P("			x.", f.GoName, " = int32(omnibuf.DecodeZigZag(v))")
		case protoreflect.BoolKind:
			g.P("			x.", f.GoName, " = (wt == omnibuf.WireBoolTrue)")
		case protoreflect.FloatKind:
			g.P("			if n-pos < 4 {")
			g.P(`				return fmt.Errorf("omnibuf: truncated float32")`)
			g.P("			}")
			g.P("			x.", f.GoName, " = math.Float32frombits(binary.LittleEndian.Uint32(data[pos:]))")
			g.P("			pos += 4")
		case protoreflect.DoubleKind:
			g.P("			if n-pos < 8 {")
			g.P(`				return fmt.Errorf("omnibuf: truncated float64")`)
			g.P("			}")
			g.P("			x.", f.GoName, " = math.Float64frombits(binary.LittleEndian.Uint64(data[pos:]))")
			g.P("			pos += 8")
		case protoreflect.StringKind:
			g.P("			sLen64, sn := omnibuf.DecodeVarint(data[pos:])")
			g.P("			if sn == 0 || sLen64 > uint64(n-(pos+sn)) {")
			g.P(`				return fmt.Errorf("omnibuf: invalid string length")`)
			g.P("			}")
			g.P("			pos += sn")
			g.P("			sLen := int(sLen64)")
			g.P("			x.", f.GoName, " = omnibuf.DecodeStr(data[pos:pos+sLen], zeroCopy)")
			g.P("			pos += sLen")
		case protoreflect.MessageKind:
			subType := f.Message.GoIdent.GoName
			g.P("			subLen, sn := omnibuf.DecodeVarint(data[pos:])")
			g.P("			if sn == 0 || subLen > uint64(n-(pos+sn)) {")
			g.P(`				return fmt.Errorf("omnibuf: invalid submsg length")`)
			g.P("			}")
			g.P("			pos += sn")
			g.P("			if x.", f.GoName, " == nil {")
			g.P("				x.", f.GoName, " = new(", subType, ")")
			g.P("			}")
			g.P("			if err := x.", f.GoName, ".UnmarshalOmniBuf(data[pos:pos+int(subLen)], zeroCopy); err != nil {")
			g.P("				return err")
			g.P("			}")
			g.P("			pos += int(subLen)")
		}
	}
	g.P("		default:")
	g.P("			var err error")
	g.P("			pos, err = omnibuf.SkipValue(data, pos, wt)")
	g.P("			if err != nil {")
	g.P("				return err")
	g.P("			}")
	g.P("		}")
	g.P("	}")
	g.P("	return nil")
	g.P("}")
	g.P()
}

// generateOpaqueMessage generates the Opaque API struct (Opaque<Name>) with lazy getters,
// setters, dirty bitmask tracking, and zero-decode splice MarshalAppend.
func generateOpaqueMessage(g *protogen.GeneratedFile, msg *protogen.Message) {
	name := "Opaque" + msg.GoIdent.GoName
	g.P("// ", name, " is the protoc-gen-omnibuf Opaque API struct for ", msg.GoIdent.GoName, ".")
	g.P("type ", name, " struct {")
	g.P("	raw   []byte")
	g.P("	dirty uint16")
	for _, f := range msg.Fields {
		if f.Desc.IsList() {
			continue
		}
		fname := "f_" + f.Desc.TextName()
		switch f.Desc.Kind() {
		case protoreflect.Uint32Kind:
			g.P("	", fname, " uint32")
		case protoreflect.Uint64Kind:
			g.P("	", fname, " uint64")
		case protoreflect.Sint32Kind, protoreflect.Int32Kind:
			g.P("	", fname, " int32")
		case protoreflect.BoolKind:
			g.P("	", fname, " bool")
		case protoreflect.FloatKind:
			g.P("	", fname, " float32")
		case protoreflect.DoubleKind:
			g.P("	", fname, " float64")
		case protoreflect.StringKind:
			g.P("	", fname, " string")
		}
	}
	g.P("}")
	g.P()
	g.P("func (x *", name, ") Unmarshal(b []byte) error {")
	g.P("	x.raw = b")
	g.P("	x.dirty = 0")
	g.P("	return nil")
	g.P("}")
	g.P()
	g.P("func (x *", name, ") RawBytes() []byte { return x.raw }")
	g.P()

	for _, f := range msg.Fields {
		fnum := uint16(f.Desc.Number())
		goName := f.GoName
		privName := "f_" + f.Desc.TextName()

		if f.Desc.IsList() {
			switch f.Desc.Kind() {
			case protoreflect.StringKind:
				g.P("func (x *", name, ") ", goName, "Len() int {")
				g.P("	s, wt, ok := omnibuf.LocateField(x.raw, ", fnum, ")")
				g.P("	if !ok || wt != omnibuf.WireStringArr { return 0 }")
				g.P("	bLen, bn := omnibuf.DecodeVarint(s)")
				g.P("	if bn == 0 || bLen > uint64(len(s)-bn) { return 0 }")
				g.P("	c, cn := omnibuf.DecodeVarint(s[bn : bn+int(bLen)])")
				g.P("	if cn == 0 { return 0 }")
				g.P("	return int(c)")
				g.P("}")
				g.P()
				g.P("func (x *", name, ") Append", goName, "Into(dst []string) []string {")
				g.P("	s, wt, ok := omnibuf.LocateField(x.raw, ", fnum, ")")
				g.P("	if !ok || wt != omnibuf.WireStringArr { return dst }")
				g.P("	bLen, bn := omnibuf.DecodeVarint(s)")
				g.P("	if bn == 0 || bLen > uint64(len(s)-bn) { return dst }")
				g.P("	body := s[bn : bn+int(bLen)]")
				g.P("	c, pos := omnibuf.DecodeVarint(body)")
				g.P("	if pos == 0 || c > uint64(len(body)-pos) { return dst }")
				g.P("	for i := 0; i < int(c); i++ {")
				g.P("		sLen, sn := omnibuf.DecodeVarint(body[pos:])")
				g.P("		if sn == 0 || sLen > uint64(len(body)-(pos+sn)) { return dst }")
				g.P("		pos += sn")
				g.P("		dst = append(dst, omnibuf.DecodeStr(body[pos:pos+int(sLen)], true))")
				g.P("		pos += int(sLen)")
				g.P("	}")
				g.P("	return dst")
				g.P("}")
				g.P()
			case protoreflect.Uint32Kind:
				g.P("func (x *", name, ") ", goName, "Len() int {")
				g.P("	s, wt, ok := omnibuf.LocateField(x.raw, ", fnum, ")")
				g.P("	if !ok || wt != omnibuf.WireUintArr { return 0 }")
				g.P("	bLen, bn := omnibuf.DecodeVarint(s)")
				g.P("	if bn == 0 || bLen > uint64(len(s)-bn) { return 0 }")
				g.P("	c, cn := omnibuf.DecodeVarint(s[bn : bn+int(bLen)])")
				g.P("	if cn == 0 { return 0 }")
				g.P("	return int(c)")
				g.P("}")
				g.P()
				g.P("func (x *", name, ") Append", goName, "Into(dst []uint32) []uint32 {")
				g.P("	s, wt, ok := omnibuf.LocateField(x.raw, ", fnum, ")")
				g.P("	if !ok || wt != omnibuf.WireUintArr { return dst }")
				g.P("	bLen, bn := omnibuf.DecodeVarint(s)")
				g.P("	if bn == 0 || bLen > uint64(len(s)-bn) { return dst }")
				g.P("	body := s[bn : bn+int(bLen)]")
				g.P("	c, p := omnibuf.DecodeVarint(body)")
				g.P("	if p == 0 || c > uint64(len(body)-p) { return dst }")
				g.P("	for i := 0; i < int(c); i++ {")
				g.P("		v, vn := omnibuf.DecodeVarint(body[p:])")
				g.P("		if vn == 0 { return dst }")
				g.P("		p += vn")
				g.P("		dst = append(dst, uint32(v))")
				g.P("	}")
				g.P("	return dst")
				g.P("}")
				g.P()
			case protoreflect.MessageKind:
				subOpaque := "Opaque" + f.Message.GoIdent.GoName
				g.P("func (x *", name, ") Get", goName, "(idx int) ", subOpaque, " {")
				g.P("	elem, _ := omnibuf.ReadSubMsgArrElem(x.raw, ", fnum, ", idx)")
				g.P("	return ", subOpaque, "{raw: elem}")
				g.P("}")
				g.P()
				g.P("func (x *", name, ") ", goName, "Len() int {")
				g.P("	return omnibuf.SubMsgArrLen(x.raw, ", fnum, ")")
				g.P("}")
				g.P()
			}
			continue
		}

		switch f.Desc.Kind() {
		case protoreflect.Uint32Kind:
			g.P("func (x ", name, ") Get", goName, "() uint32 {")
			g.P("	if (x.dirty & (1 << ", fnum, ")) != 0 { return x.", privName, " }")
			g.P("	v, _ := omnibuf.ReadUint(x.raw, ", fnum, ")")
			g.P("	return uint32(v)")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v uint32) { x.", privName, " = v; x.dirty |= (1 << ", fnum, ") }")
		case protoreflect.Uint64Kind:
			g.P("func (x ", name, ") Get", goName, "() uint64 {")
			g.P("	if (x.dirty & (1 << ", fnum, ")) != 0 { return x.", privName, " }")
			g.P("	v, _ := omnibuf.ReadUint(x.raw, ", fnum, ")")
			g.P("	return v")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v uint64) { x.", privName, " = v; x.dirty |= (1 << ", fnum, ") }")
		case protoreflect.Sint32Kind, protoreflect.Int32Kind:
			g.P("func (x ", name, ") Get", goName, "() int32 {")
			g.P("	if (x.dirty & (1 << ", fnum, ")) != 0 { return x.", privName, " }")
			g.P("	v, _ := omnibuf.ReadInt(x.raw, ", fnum, ")")
			g.P("	return int32(v)")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v int32) { x.", privName, " = v; x.dirty |= (1 << ", fnum, ") }")
		case protoreflect.BoolKind:
			g.P("func (x ", name, ") Get", goName, "() bool {")
			g.P("	if (x.dirty & (1 << ", fnum, ")) != 0 { return x.", privName, " }")
			g.P("	v, _ := omnibuf.ReadBool(x.raw, ", fnum, ")")
			g.P("	return v")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v bool) { x.", privName, " = v; x.dirty |= (1 << ", fnum, ") }")
		case protoreflect.FloatKind:
			g.P("func (x ", name, ") Get", goName, "() float32 {")
			g.P("	if (x.dirty & (1 << ", fnum, ")) != 0 { return x.", privName, " }")
			g.P("	v, _ := omnibuf.ReadFloat32(x.raw, ", fnum, ")")
			g.P("	return v")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v float32) { x.", privName, " = v; x.dirty |= (1 << ", fnum, ") }")
		case protoreflect.DoubleKind:
			g.P("func (x ", name, ") Get", goName, "() float64 {")
			g.P("	if (x.dirty & (1 << ", fnum, ")) != 0 { return x.", privName, " }")
			g.P("	v, _ := omnibuf.ReadFloat64(x.raw, ", fnum, ")")
			g.P("	return v")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v float64) { x.", privName, " = v; x.dirty |= (1 << ", fnum, ") }")
		case protoreflect.StringKind:
			g.P("func (x ", name, ") Get", goName, "() string {")
			g.P("	if (x.dirty & (1 << ", fnum, ")) != 0 { return x.", privName, " }")
			g.P("	s, _ := omnibuf.ReadStringZeroCopy(x.raw, ", fnum, ")")
			g.P("	return s")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v string) { x.", privName, " = v; x.dirty |= (1 << ", fnum, ") }")
		case protoreflect.MessageKind:
			subOpaque := "Opaque" + f.Message.GoIdent.GoName
			g.P("func (x *", name, ") Get", goName, "() ", subOpaque, " {")
			g.P("	sub, _ := omnibuf.ReadSubMsg(x.raw, ", fnum, ")")
			g.P("	return ", subOpaque, "{raw: sub}")
			g.P("}")
		}
		g.P()
	}

	// Generate MarshalAppend on OpaquePeerUpdate with zero-decode splice support!
	lastField := msg.Fields[len(msg.Fields)-1]
	lastFnum := uint16(lastField.Desc.Number())
	if lastField.Desc.Kind() == protoreflect.Uint64Kind && !lastField.Desc.IsList() {
		lastPriv := "f_" + lastField.Desc.TextName()
		rawTag := (lastFnum << 4) | uint16(wireTypeForField(lastField))
		tag2 := (rawTag << 2) | 2
		g.P("// MarshalAppend re-serializes ", name, " using zero-decode wire splicing for unmodified fields.")
		g.P("func (x *", name, ") MarshalAppend(dst []byte) []byte {")
		g.P("	dst = dst[:0]")
		g.P("	if x.dirty == 0 {")
		g.P("		return append(dst, x.raw...)")
		g.P("	}")
		g.P("	return omnibuf.SpliceFields(dst, x.raw, x.dirty, ", lastFnum, ", func(b []byte, fnum uint16) []byte {")
		g.P("		if fnum == ", lastFnum, " {")
		g.P(fmt.Sprintf("			b = append(b, 0x%02x, 0x%02x)", byte(tag2), byte(tag2>>8)))
		g.P("			return omnibuf.AppendVarint(b, x.", lastPriv, ")")
		g.P("		}")
		g.P("		return b")
		g.P("	})")
		g.P("}")
		g.P()
	}
}
