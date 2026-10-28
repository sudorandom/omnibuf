package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	_ "github.com/planetscale/vtprotobuf/features/marshal"
	_ "github.com/planetscale/vtprotobuf/features/pool"
	_ "github.com/planetscale/vtprotobuf/features/size"
	_ "github.com/planetscale/vtprotobuf/features/unmarshal"
	vtgenerator "github.com/planetscale/vtprotobuf/generator"
	"github.com/sudorandom/omnibuf/wire"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func main() {
	selfGenOut := flag.String("self-gen", "", "Run protogen on omnibuf/bench/peer.proto descriptor and write generated file to path")
	conformanceGenOut := flag.String("conformance-gen", "", "Run protogen on omnibuf/conformance/conformance.proto descriptor and write generated file to path")
	vtGenOut := flag.String("vtproto-gen", "", "Run vtprotobuf generator on omnibuf/bench/peer.proto descriptor and write generated file to path")
	flag.Parse()

	if *selfGenOut != "" {
		if err := runSelfGen(*selfGenOut); err != nil {
			fmt.Fprintf(os.Stderr, "protoc-gen-omnibuf: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if *conformanceGenOut != "" {
		if err := runConformanceGen(*conformanceGenOut); err != nil {
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

func peerProtoRequest() *pluginpb.CodeGeneratorRequest {
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
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String("github.com/sudorandom/omnibuf/internal/pb;pb"),
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
	return &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{"omnibuf/bench/peer.proto"},
		ProtoFile:      []*descriptorpb.FileDescriptorProto{fdp},
	}
}

func writePluginResponse(resp *pluginpb.CodeGeneratorResponse, outPath string) error {
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

func conformanceProtoRequest() *pluginpb.CodeGeneratorRequest {
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
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String("github.com/sudorandom/omnibuf/internal/pb;pb"),
		},
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
	return &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{"omnibuf/conformance/conformance.proto"},
		ProtoFile:      []*descriptorpb.FileDescriptorProto{fdp},
	}
}

func runSelfGen(outPath string) error {
	gen, err := protogen.Options{}.New(peerProtoRequest())
	if err != nil {
		return err
	}
	for _, file := range gen.Files {
		if file.Generate {
			generateFile(gen, file)
		}
	}
	return writePluginResponse(gen.Response(), outPath)
}

func runConformanceGen(outPath string) error {
	gen, err := protogen.Options{}.New(conformanceProtoRequest())
	if err != nil {
		return err
	}
	for _, file := range gen.Files {
		if file.Generate {
			generateFile(gen, file)
		}
	}
	return writePluginResponse(gen.Response(), outPath)
}

func runVTProtoGen(outPath string) error {
	plugin, err := protogen.Options{}.New(peerProtoRequest())
	if err != nil {
		return err
	}
	cfg := vtgenerator.Config{
		Poolable:        vtgenerator.NewObjectSet(),
		PoolableExclude: vtgenerator.NewObjectSet(),
	}
	_ = cfg.Poolable.Set("github.com/sudorandom/omnibuf/internal/pb.*")
	gen, err := vtgenerator.NewGenerator(plugin, []string{"marshal", "size", "unmarshal", "unmarshal_unsafe", "pool"}, &cfg)
	if err != nil {
		return err
	}
	gen.Generate()
	return writePluginResponse(plugin.Response(), outPath)
}

func wireTypeForField(field *protogen.Field) uint8 {
	if field.Desc.IsMap() {
		return wire.WireMap
	}
	if field.Desc.IsList() {
		return wire.WireArray
	}
	switch field.Desc.Kind() {
	case protoreflect.BoolKind:
		return wire.WireBoolTrue
	case protoreflect.StringKind:
		return wire.WireString
	case protoreflect.BytesKind:
		return wire.WireBytes
	case protoreflect.MessageKind:
		return wire.WireSubMsg
	case protoreflect.FloatKind:
		return wire.WireFloat32
	case protoreflect.DoubleKind:
		return wire.WireFloat64
	case protoreflect.Fixed32Kind:
		return wire.WireFixed32
	case protoreflect.Fixed64Kind:
		return wire.WireFixed64
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.EnumKind:
		return wire.WireSint
	case protoreflect.Sfixed32Kind:
		return wire.WireSfixed32
	case protoreflect.Sfixed64Kind:
		return wire.WireSfixed64
	default:
		return wire.WireUint
	}
}

func generateFile(gen *protogen.Plugin, file *protogen.File) {
	if err := validateGeneratedFile(file); err != nil {
		gen.Error(err)
		return
	}

	filename := file.GeneratedFilenamePrefix + "_wire.pb.go"
	g := gen.NewGeneratedFile(filename, file.GoImportPath)
	g.P("// Code generated by protoc-gen-omnibuf. DO NOT EDIT.")
	g.P("// source: ", file.Desc.Path())
	g.P()
	g.P("package ", file.GoPackageName)
	g.P()
	g.P("import (")
	g.P(`	"encoding/binary"`)
	g.P(`	"fmt"`)
	if generatedFileUsesMath(file) {
		g.P(`	"math"`)
	}
	g.P()
	g.P(`	"github.com/sudorandom/omnibuf"`)
	g.P(`	"github.com/sudorandom/omnibuf/wire"`)
	g.P(")")
	g.P()

	for _, msg := range file.Messages {
		generateOpenStructCodec(g, msg)
		if canGenerateOpaque(msg) {
			generateOpaqueMessage(g, msg)
		}
	}
}

func emitConstTag(g *protogen.GeneratedFile, dstVar string, rawTag uint64) {
	if rawTag <= 127 {
		g.P(fmt.Sprintf("\t%s = append(%s, 0x%02x)", dstVar, dstVar, byte(rawTag<<1)|1))
	} else if rawTag <= 16383 {
		u := uint16(rawTag<<2) | 2
		g.P(fmt.Sprintf("\t%s = append(%s, 0x%02x, 0x%02x)", dstVar, dstVar, byte(u), byte(u>>8)))
	} else {
		g.P("\t", dstVar, " = wire.AppendVarint(", dstVar, ", ", rawTag, ")")
	}
}

func emitBackpatchLen(g *protogen.GeneratedFile, dstVar, lenPosVar string) {
	g.P("\tif d := len(", dstVar, ") - ", lenPosVar, "; d <= 16385 {")
	g.P("\t\tbinary.LittleEndian.PutUint16(", dstVar, "[", lenPosVar, ":], uint16(d<<2-6))")
	g.P("\t} else {")
	g.P("\t\t", dstVar, " = wire.BackpatchContainerLen(", dstVar, ", ", lenPosVar, ")")
	g.P("\t}")
}

// generateOpenStructCodec generates single-pass MarshalOmniBufAppend and UnmarshalOmniBuf
// methods directly onto the standard protoc-gen-go struct (e.g. *PeerUpdate).
func generateOpenStructCodec(g *protogen.GeneratedFile, msg *protogen.Message) {
	typeName := msg.GoIdent.GoName

	numWords := (len(msg.Fields) + 63) / 64
	if numWords == 0 {
		numWords = 1
	}
	expectedMasks := make([]uint64, numWords)
	var fnumList []string
	fnumList = append(fnumList, "0")
	for i, f := range msg.Fields {
		expectedMasks[i/64] |= 1 << (i % 64)
		fnumList = append(fnumList, fmt.Sprintf("%d", f.Desc.Number()))
	}
	maskVar := func(word int) string {
		if word == 0 {
			return "seenMask"
		}
		return fmt.Sprintf("seenMask%d", word)
	}
	expectedVar := func(word int) string {
		if word == 0 {
			return "expectedMask"
		}
		return fmt.Sprintf("expectedMask%d", word)
	}

	g.P("// MarshalOmniBufAppend encodes ", typeName, " in a single forward pass using generated field tags.")
	g.P("func (x *", typeName, ") MarshalOmniBufAppend(dst []byte) []byte {")
	for i, f := range msg.Fields {
		fnum := uint16(f.Desc.Number())
		wt := wireTypeForField(f)
		rawTag := (uint64(fnum) << 4) | uint64(wt)
		if f.Desc.Kind() == protoreflect.BoolKind && !f.Desc.IsList() {
			tTrue := (uint64(fnum) << 4) | uint64(wire.WireBoolTrue)
			tFalse := (uint64(fnum) << 4) | uint64(wire.WireBoolFalse)
			g.P("\tif x.", f.GoName, " {")
			emitConstTag(g, "dst", tTrue)
			g.P("\t} else {")
			emitConstTag(g, "dst", tFalse)
			g.P("\t}")
			continue
		}
		if f.Desc.Kind() == protoreflect.MessageKind && !f.Desc.IsList() && !f.Desc.IsMap() {
			g.P("\tif x.", f.GoName, " != nil {")
			emitConstTag(g, "dst", rawTag)
			g.P("		lenPos_", i, " := len(dst)")
			g.P("		dst = append(dst, 0, 0)")
			g.P("		dst = x.", f.GoName, ".MarshalOmniBufAppend(dst)")
			emitBackpatchLen(g, "dst", fmt.Sprintf("lenPos_%d", i))
			g.P("\t}")
			continue
		}
		emitConstTag(g, "dst", rawTag)

		if f.Desc.IsMap() {
			g.P("	dst = wire.EncodeMapStringUint(dst, x.", f.GoName, ", true)")
			continue
		}

		if f.Desc.IsList() {
			switch f.Desc.Kind() {
			case protoreflect.StringKind:
				g.P("	lenPos_", i, " := len(dst)")
				g.P("	dst = append(dst, 0, 0, wire.WireString)")
				g.P("	dst = wire.AppendVarint(dst, uint64(len(x.", f.GoName, ")))")
				g.P("	for _, s := range x.", f.GoName, " {")
				g.P("		dst = wire.AppendVarint(dst, uint64(len(s)))")
				g.P("		dst = append(dst, s...)")
				g.P("	}")
				emitBackpatchLen(g, "dst", fmt.Sprintf("lenPos_%d", i))
			case protoreflect.BytesKind:
				g.P("	dst = wire.EncodeBytesArr(dst, x.", f.GoName, ")")
			case protoreflect.BoolKind:
				g.P("	dst = wire.EncodeBoolArr(dst, x.", f.GoName, ")")
			case protoreflect.Uint32Kind, protoreflect.Uint64Kind:
				g.P("	lenPos_", i, " := len(dst)")
				g.P("	dst = append(dst, 0, 0, wire.WireUint)")
				g.P("	dst = wire.AppendVarint(dst, uint64(len(x.", f.GoName, ")))")
				g.P("	for _, v := range x.", f.GoName, " {")
				g.P("		dst = wire.AppendVarint(dst, uint64(v))")
				g.P("	}")
				emitBackpatchLen(g, "dst", fmt.Sprintf("lenPos_%d", i))
			case protoreflect.Sint32Kind, protoreflect.Int32Kind, protoreflect.Sint64Kind, protoreflect.Int64Kind:
				g.P("	dst = wire.EncodeSintArr(dst, x.", f.GoName, ")")
			case protoreflect.Fixed32Kind:
				g.P("	dst = wire.EncodeFixed32Arr(dst, x.", f.GoName, ", wire.WireFixed32)")
			case protoreflect.Fixed64Kind:
				g.P("	dst = wire.EncodeFixed64Arr(dst, x.", f.GoName, ", wire.WireFixed64)")
			case protoreflect.Sfixed32Kind:
				g.P("	dst = wire.EncodeFixed32Arr(dst, x.", f.GoName, ", wire.WireSfixed32)")
			case protoreflect.Sfixed64Kind:
				g.P("	dst = wire.EncodeFixed64Arr(dst, x.", f.GoName, ", wire.WireSfixed64)")
			case protoreflect.FloatKind:
				g.P("	dst = wire.EncodeFloat32Arr(dst, x.", f.GoName, ")")
			case protoreflect.DoubleKind:
				g.P("	dst = wire.EncodeFloat64Arr(dst, x.", f.GoName, ")")
			case protoreflect.MessageKind:
				g.P("	lenPos_", i, " := len(dst)")
				g.P("	dst = append(dst, 0, 0, wire.WireSubMsg)")
				g.P("	dst = wire.AppendVarint(dst, uint64(len(x.", f.GoName, ")))")
				g.P("	for _, elem := range x.", f.GoName, " {")
				g.P("		eLenPos := len(dst)")
				g.P("		dst = append(dst, 0, 0)")
				g.P("		if elem != nil {")
				g.P("			dst = elem.MarshalOmniBufAppend(dst)")
				g.P("		}")
				emitBackpatchLen(g, "dst", "eLenPos")
				g.P("	}")
				emitBackpatchLen(g, "dst", fmt.Sprintf("lenPos_%d", i))
			}
			continue
		}

		switch f.Desc.Kind() {
		case protoreflect.Uint32Kind:
			g.P("	dst = wire.AppendVarint(dst, uint64(x.", f.GoName, "))")
		case protoreflect.Uint64Kind:
			g.P("	dst = wire.AppendVarint(dst, x.", f.GoName, ")")
		case protoreflect.Sint32Kind, protoreflect.Int32Kind:
			g.P("	dst = wire.AppendVarint(dst, wire.EncodeZigZag(int64(x.", f.GoName, ")))")
		case protoreflect.Sint64Kind, protoreflect.Int64Kind:
			g.P("	dst = wire.AppendVarint(dst, wire.EncodeZigZag(x.", f.GoName, "))")
		case protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind:
			g.P("	b_", i, " := uint32(x.", f.GoName, ")")
			g.P("	dst = append(dst, byte(b_", i, "), byte(b_", i, ">>8), byte(b_", i, ">>16), byte(b_", i, ">>24))")
		case protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind:
			g.P("	b_", i, " := uint64(x.", f.GoName, ")")
			g.P("	dst = append(dst, byte(b_", i, "), byte(b_", i, ">>8), byte(b_", i, ">>16), byte(b_", i, ">>24), byte(b_", i, ">>32), byte(b_", i, ">>40), byte(b_", i, ">>48), byte(b_", i, ">>56))")
		case protoreflect.FloatKind:
			g.P("	b_", i, " := math.Float32bits(x.", f.GoName, ")")
			g.P("	dst = append(dst, byte(b_", i, "), byte(b_", i, ">>8), byte(b_", i, ">>16), byte(b_", i, ">>24))")
		case protoreflect.DoubleKind:
			g.P("	b_", i, " := math.Float64bits(x.", f.GoName, ")")
			g.P("	dst = append(dst, byte(b_", i, "), byte(b_", i, ">>8), byte(b_", i, ">>16), byte(b_", i, ">>24), byte(b_", i, ">>32), byte(b_", i, ">>40), byte(b_", i, ">>48), byte(b_", i, ">>56))")
		case protoreflect.StringKind, protoreflect.BytesKind:
			g.P("	dst = wire.AppendVarint(dst, uint64(len(x.", f.GoName, ")))")
			g.P("	dst = append(dst, x.", f.GoName, "...)")
		}
	}
	g.P("	return dst")
	g.P("}")
	g.P()

	var clearParams []string
	var clearArgs []string
	var maskCheckConds []string
	for w := 0; w < numWords; w++ {
		mv := maskVar(w)
		ev := expectedVar(w)
		clearParams = append(clearParams, mv+" uint64")
		clearArgs = append(clearArgs, mv)
		maskCheckConds = append(maskCheckConds, mv+" != "+ev)
	}

	g.P("//go:noinline")
	g.P("func (x *", typeName, ") clearUnseenOmniBuf(", strings.Join(clearParams, ", "), ") {")
	for i, f := range msg.Fields {
		mv := maskVar(i / 64)
		bit := i % 64
		g.P(fmt.Sprintf("	if (%s & (1 << %d)) == 0 {", mv, bit))
		if f.Desc.IsMap() {
			g.P("		clear(x.", f.GoName, ")")
		} else if f.Desc.IsList() {
			g.P("		x.", f.GoName, " = x.", f.GoName, "[:0]")
		} else {
			switch f.Desc.Kind() {
			case protoreflect.Uint32Kind, protoreflect.Uint64Kind, protoreflect.Sint32Kind, protoreflect.Int32Kind, protoreflect.Sint64Kind, protoreflect.Int64Kind, protoreflect.Fixed32Kind, protoreflect.Fixed64Kind, protoreflect.Sfixed32Kind, protoreflect.Sfixed64Kind, protoreflect.FloatKind, protoreflect.DoubleKind:
				g.P("		x.", f.GoName, " = 0")
			case protoreflect.BoolKind:
				g.P("		x.", f.GoName, " = false")
			case protoreflect.StringKind:
				g.P(`		x.`, f.GoName, ` = ""`)
			case protoreflect.BytesKind:
				g.P("		x.", f.GoName, " = nil")
			case protoreflect.MessageKind:
				g.P("		x.", f.GoName, " = nil")
			}
		}
		g.P("	}")
	}
	g.P("}")
	g.P()

	g.P("// UnmarshalOmniBuf decodes data into ", typeName, " using generated switch dispatch.")
	g.P("func (x *", typeName, ") UnmarshalOmniBuf(data []byte, zeroCopy bool) error {")
	g.P("	return x.decodeOmniBuf(data, zeroCopy, 0, false)")
	g.P("}")
	g.P()

	g.P("func (x *", typeName, ") decodeOmniBuf(data []byte, zeroCopy bool, depth int, merge bool) error {")
	g.P("	if depth > omnibuf.MaxNestingDepth {")
	g.P(`		return fmt.Errorf("omnibuf: max nesting depth exceeded")`)
	g.P("	}")
	for w := 0; w < numWords; w++ {
		g.P(fmt.Sprintf("	const %s uint64 = 0x%x", expectedVar(w), expectedMasks[w]))
		g.P("	var ", maskVar(w), " uint64")
	}
	g.P("	n := len(data)")
	g.P("	pos := 0")
	g.P("	for pos < n {")
	g.P("		b0 := data[pos]")
	g.P("		var tag uint64")
	g.P("		if (b0 & 1) != 0 {")
	g.P("			tag = uint64(b0 >> 1)")
	g.P("			pos++")
	g.P("		} else if (b0&2) != 0 && pos+2 <= n {")
	g.P("			tag = uint64(binary.LittleEndian.Uint16(data[pos:]) >> 2)")
	g.P("			pos += 2")
	g.P("		} else {")
	g.P("			t64, tn := wire.DecodeVarint(data[pos:])")
	g.P("			if tn == 0 || (t64>>4) > 536870911 {")
	g.P(`				return fmt.Errorf("omnibuf: invalid tag")`)
	g.P("			}")
	g.P("			tag = t64")
	g.P("			pos += tn")
	g.P("		}")
	g.P("		switch tag {")
	g.P("		case uint64(wire.WireSubMsg):")
	g.P("			if _, ok := omnibuf.ReadField0SchemaMeta(data); !ok {")
	g.P(`				return fmt.Errorf("omnibuf: invalid Field #0 SchemaMeta")`)
	g.P("			}")
	g.P("			var err error")
	g.P("			pos, err = wire.SkipValue(data, pos, wire.WireSubMsg)")
	g.P("			if err != nil {")
	g.P("				return err")
	g.P("			}")
	for i, f := range msg.Fields {
		fnum := uint32(f.Desc.Number())
		wt := wireTypeForField(f)
		mv := maskVar(i / 64)
		bit := i % 64
		if f.Desc.Kind() == protoreflect.BoolKind && !f.Desc.IsList() {
			g.P("		case ", (uint64(fnum)<<4)|uint64(wire.WireBoolTrue), ":")
			g.P("			", mv, " |= 1 << ", bit)
			g.P("			x.", f.GoName, " = true")
			g.P("		case ", (uint64(fnum)<<4)|uint64(wire.WireBoolFalse), ":")
			g.P("			", mv, " |= 1 << ", bit)
			g.P("			x.", f.GoName, " = false")
			continue
		}
		g.P("		case ", (uint64(fnum)<<4)|uint64(wt), ":")
		if f.Desc.IsMap() {
			g.P("			byteLen, bn := wire.DecodeVarint(data[pos:])")
			g.P("			if bn == 0 || byteLen > uint64(n-(pos+bn)) {")
			g.P(`				return fmt.Errorf("omnibuf: invalid map byteLen")`)
			g.P("			}")
			g.P("			pos += bn")
			g.P("			body := data[pos : pos+int(byteLen)]")
			g.P("			pos += int(byteLen)")
			g.P("			keyWT, valWT, count, p, ok := wire.ParseMapHeader(body)")
			g.P("			if !ok || keyWT != wire.WireString || valWT != wire.WireUint {")
			g.P(`				return fmt.Errorf("omnibuf: invalid map header")`)
			g.P("			}")
			g.P("			if !merge && (", mv, "&(1<<", bit, ")) == 0 {")
			g.P("				clear(x.", f.GoName, ")")
			g.P("			}")
			g.P("			if x.", f.GoName, " == nil {")
			g.P("				x.", f.GoName, " = make(map[string]uint64, count)")
			g.P("			}")
			g.P("			", mv, " |= 1 << ", bit)
			g.P("			for j := 0; j < count; j++ {")
			g.P("				kLen, kn := wire.DecodeVarint(body[p:])")
			g.P("				if kn == 0 || kLen > uint64(len(body)-(p+kn)) {")
			g.P(`					return fmt.Errorf("omnibuf: invalid map key")`)
			g.P("				}")
			g.P("				p += kn")
			g.P("				k := wire.DecodeStr(body[p:p+int(kLen)], zeroCopy)")
			g.P("				p += int(kLen)")
			g.P("				v, vn := wire.DecodeVarint(body[p:])")
			g.P("				if vn == 0 {")
			g.P(`					return fmt.Errorf("omnibuf: invalid map value")`)
			g.P("				}")
			g.P("				p += vn")
			g.P("				x.", f.GoName, "[k] = v")
			g.P("			}")
			g.P("			if p != len(body) {")
			g.P(`				return fmt.Errorf("omnibuf: map trailing bytes")`)
			g.P("			}")
			continue
		}
		if f.Desc.IsList() {
			g.P("			byteLen, bn := wire.DecodeVarint(data[pos:])")
			g.P("			if bn == 0 || byteLen > uint64(n-(pos+bn)) {")
			g.P(`				return fmt.Errorf("omnibuf: invalid array byteLen")`)
			g.P("			}")
			g.P("			pos += bn")
			g.P("			body := data[pos : pos+int(byteLen)]")
			g.P("			pos += int(byteLen)")
			g.P("			if !merge && (", mv, "&(1<<", bit, ")) == 0 {")
			g.P("				x.", f.GoName, " = x.", f.GoName, "[:0]")
			g.P("			}")
			g.P("			", mv, " |= 1 << ", bit)
			switch f.Desc.Kind() {
			case protoreflect.StringKind:
				g.P("			if len(body) < 2 || body[0] != wire.WireString {")
				g.P(`				return fmt.Errorf("omnibuf: invalid string array")`)
				g.P("			}")
				g.P("			cu, cn := wire.DecodeVarint(body[1:])")
				g.P("			p := 1 + cn")
				g.P("			if cn == 0 || cu > uint64(len(body)-p) {")
				g.P(`				return fmt.Errorf("omnibuf: invalid string array count")`)
				g.P("			}")
				g.P("			count := int(cu)")
				g.P("			start := len(x.", f.GoName, ")")
				g.P("			total := start + count")
				g.P("			if cap(x.", f.GoName, ") < total {")
				g.P("				next := make([]string, total)")
				g.P("				copy(next, x.", f.GoName, ")")
				g.P("				x.", f.GoName, " = next")
				g.P("			} else {")
				g.P("				x.", f.GoName, " = x.", f.GoName, "[:total]")
				g.P("			}")
				g.P("			for i := 0; i < count; i++ {")
				g.P("				l, ln := wire.DecodeVarint(body[p:])")
				g.P("				if ln == 0 || l > uint64(len(body)-(p+ln)) {")
				g.P(`					return fmt.Errorf("omnibuf: invalid string element")`)
				g.P("				}")
				g.P("				p += ln")
				g.P("				x.", f.GoName, "[start+i] = wire.DecodeStr(body[p:p+int(l)], zeroCopy)")
				g.P("				p += int(l)")
				g.P("			}")
				g.P("			if p != len(body) {")
				g.P(`				return fmt.Errorf("omnibuf: string array trailing bytes")`)
				g.P("			}")
			case protoreflect.BytesKind:
				g.P("			var ok bool")
				g.P("			x.", f.GoName, ", ok = wire.AppendBytesArrBody(x.", f.GoName, ", body, zeroCopy)")
				g.P("			if !ok {")
				g.P(`				return fmt.Errorf("omnibuf: invalid bytes array")`)
				g.P("			}")
			case protoreflect.BoolKind:
				g.P("			var ok bool")
				g.P("			x.", f.GoName, ", ok = wire.AppendBoolArrBody(x.", f.GoName, ", body)")
				g.P("			if !ok {")
				g.P(`				return fmt.Errorf("omnibuf: invalid bool array")`)
				g.P("			}")
			case protoreflect.Uint32Kind:
				g.P("			if len(body) < 2 || body[0] != wire.WireUint {")
				g.P(`				return fmt.Errorf("omnibuf: invalid uint32 array")`)
				g.P("			}")
				g.P("			cu, cn := wire.DecodeVarint(body[1:])")
				g.P("			p := 1 + cn")
				g.P("			if cn == 0 || cu > uint64(len(body)-p) {")
				g.P(`				return fmt.Errorf("omnibuf: invalid uint32 array count")`)
				g.P("			}")
				g.P("			count := int(cu)")
				g.P("			start := len(x.", f.GoName, ")")
				g.P("			total := start + count")
				g.P("			if cap(x.", f.GoName, ") < total {")
				g.P("				next := make([]uint32, total)")
				g.P("				copy(next, x.", f.GoName, ")")
				g.P("				x.", f.GoName, " = next")
				g.P("			} else {")
				g.P("				x.", f.GoName, " = x.", f.GoName, "[:total]")
				g.P("			}")
				g.P("			for i := 0; i < count; i++ {")
				g.P("				v, vn := wire.DecodeVarint(body[p:])")
				g.P("				if vn == 0 {")
				g.P(`					return fmt.Errorf("omnibuf: invalid uint32 element")`)
				g.P("				}")
				g.P("				p += vn")
				g.P("				x.", f.GoName, "[start+i] = uint32(v)")
				g.P("			}")
				g.P("			if p != len(body) {")
				g.P(`				return fmt.Errorf("omnibuf: uint32 array trailing bytes")`)
				g.P("			}")
			case protoreflect.Uint64Kind:
				g.P("			var ok bool")
				g.P("			x.", f.GoName, ", ok = wire.AppendUintArrBody(x.", f.GoName, ", body)")
				g.P("			if !ok {")
				g.P(`				return fmt.Errorf("omnibuf: invalid uint64 array")`)
				g.P("			}")
			case protoreflect.Sint32Kind, protoreflect.Int32Kind, protoreflect.Sint64Kind, protoreflect.Int64Kind:
				g.P("			var ok bool")
				g.P("			x.", f.GoName, ", ok = wire.AppendSintArrBody(x.", f.GoName, ", body)")
				g.P("			if !ok {")
				g.P(`				return fmt.Errorf("omnibuf: invalid sint array")`)
				g.P("			}")
			case protoreflect.Fixed32Kind:
				g.P("			if len(body) < 2 || body[0] != wire.WireFixed32 {")
				g.P(`				return fmt.Errorf("omnibuf: invalid fixed32 array")`)
				g.P("			}")
				g.P("			var ok bool")
				g.P("			x.", f.GoName, ", ok = wire.AppendFixed32ArrBody(x.", f.GoName, ", body)")
				g.P("			if !ok {")
				g.P(`				return fmt.Errorf("omnibuf: invalid fixed32 array")`)
				g.P("			}")
			case protoreflect.Sfixed32Kind:
				g.P("			if len(body) < 2 || body[0] != wire.WireSfixed32 {")
				g.P(`				return fmt.Errorf("omnibuf: invalid sfixed32 array")`)
				g.P("			}")
				g.P("			var ok bool")
				g.P("			x.", f.GoName, ", ok = wire.AppendFixed32ArrBody(x.", f.GoName, ", body)")
				g.P("			if !ok {")
				g.P(`				return fmt.Errorf("omnibuf: invalid sfixed32 array")`)
				g.P("			}")
			case protoreflect.Fixed64Kind:
				g.P("			if len(body) < 2 || body[0] != wire.WireFixed64 {")
				g.P(`				return fmt.Errorf("omnibuf: invalid fixed64 array")`)
				g.P("			}")
				g.P("			var ok bool")
				g.P("			x.", f.GoName, ", ok = wire.AppendFixed64ArrBody(x.", f.GoName, ", body)")
				g.P("			if !ok {")
				g.P(`				return fmt.Errorf("omnibuf: invalid fixed64 array")`)
				g.P("			}")
			case protoreflect.Sfixed64Kind:
				g.P("			if len(body) < 2 || body[0] != wire.WireSfixed64 {")
				g.P(`				return fmt.Errorf("omnibuf: invalid sfixed64 array")`)
				g.P("			}")
				g.P("			var ok bool")
				g.P("			x.", f.GoName, ", ok = wire.AppendFixed64ArrBody(x.", f.GoName, ", body)")
				g.P("			if !ok {")
				g.P(`				return fmt.Errorf("omnibuf: invalid sfixed64 array")`)
				g.P("			}")
			case protoreflect.FloatKind:
				g.P("			var ok bool")
				g.P("			x.", f.GoName, ", ok = wire.AppendFloat32ArrBody(x.", f.GoName, ", body)")
				g.P("			if !ok {")
				g.P(`				return fmt.Errorf("omnibuf: invalid float32 array")`)
				g.P("			}")
			case protoreflect.DoubleKind:
				g.P("			var ok bool")
				g.P("			x.", f.GoName, ", ok = wire.AppendFloat64ArrBody(x.", f.GoName, ", body)")
				g.P("			if !ok {")
				g.P(`				return fmt.Errorf("omnibuf: invalid float64 array")`)
				g.P("			}")
			case protoreflect.MessageKind:
				elemType := f.Message.GoIdent.GoName
				g.P("			if depth+1 > omnibuf.MaxNestingDepth || len(body) < 2 || body[0] != wire.WireSubMsg {")
				g.P(`				return fmt.Errorf("omnibuf: invalid submsg array")`)
				g.P("			}")
				g.P("			cu, cn := wire.DecodeVarint(body[1:])")
				g.P("			p := 1 + cn")
				g.P("			if cn == 0 || cu > uint64(len(body)-p) {")
				g.P(`				return fmt.Errorf("omnibuf: invalid submsg array count")`)
				g.P("			}")
				g.P("			count := int(cu)")
				g.P("			start := len(x.", f.GoName, ")")
				g.P("			total := start + count")
				g.P("			if cap(x.", f.GoName, ") < total {")
				g.P("				slab := make([]", elemType, ", count)")
				g.P("				next := make([]*", elemType, ", total)")
				g.P("				copy(next, x.", f.GoName, ")")
				g.P("				for i := 0; i < count; i++ {")
				g.P("					next[start+i] = &slab[i]")
				g.P("				}")
				g.P("				x.", f.GoName, " = next")
				g.P("			} else {")
				g.P("				x.", f.GoName, " = x.", f.GoName, "[:total]")
				g.P("			}")
				g.P("			for i := 0; i < count; i++ {")
				g.P("				el, en := wire.DecodeVarint(body[p:])")
				g.P("				if en == 0 || el > uint64(len(body)-(p+en)) {")
				g.P(`					return fmt.Errorf("omnibuf: invalid submsg elem length")`)
				g.P("				}")
				g.P("				p += en")
				g.P("				if x.", f.GoName, "[start+i] == nil {")
				g.P("					x.", f.GoName, "[start+i] = new(", elemType, ")")
				g.P("				}")
				g.P("				if err := x.", f.GoName, "[start+i].decodeOmniBuf(body[p:p+int(el)], zeroCopy, depth+2, false); err != nil {")
				g.P("					return err")
				g.P("				}")
				g.P("				p += int(el)")
				g.P("			}")
				g.P("			if p != len(body) {")
				g.P(`				return fmt.Errorf("omnibuf: submsg array trailing bytes")`)
				g.P("			}")
			}
			continue
		}
		switch f.Desc.Kind() {
		case protoreflect.Uint32Kind:
			g.P("			v, vn := wire.DecodeVarint(data[pos:])")
			g.P("			if vn == 0 {")
			g.P(`				return fmt.Errorf("omnibuf: truncated uint32")`)
			g.P("			}")
			g.P("			pos += vn")
			g.P("			", mv, " |= 1 << ", bit)
			g.P("			x.", f.GoName, " = uint32(v)")
		case protoreflect.Uint64Kind:
			g.P("			v, vn := wire.DecodeVarint(data[pos:])")
			g.P("			if vn == 0 {")
			g.P(`				return fmt.Errorf("omnibuf: truncated uint64")`)
			g.P("			}")
			g.P("			pos += vn")
			g.P("			", mv, " |= 1 << ", bit)
			g.P("			x.", f.GoName, " = v")
		case protoreflect.Sint32Kind, protoreflect.Int32Kind:
			g.P("			v, vn := wire.DecodeVarint(data[pos:])")
			g.P("			if vn == 0 {")
			g.P(`				return fmt.Errorf("omnibuf: truncated sint32")`)
			g.P("			}")
			g.P("			pos += vn")
			g.P("			", mv, " |= 1 << ", bit)
			g.P("			x.", f.GoName, " = int32(wire.DecodeZigZag(v))")
		case protoreflect.Sint64Kind, protoreflect.Int64Kind:
			g.P("			v, vn := wire.DecodeVarint(data[pos:])")
			g.P("			if vn == 0 {")
			g.P(`				return fmt.Errorf("omnibuf: truncated sint64")`)
			g.P("			}")
			g.P("			pos += vn")
			g.P("			", mv, " |= 1 << ", bit)
			g.P("			x.", f.GoName, " = wire.DecodeZigZag(v)")
		case protoreflect.Fixed32Kind:
			g.P("			if n-pos < 4 {")
			g.P(`				return fmt.Errorf("omnibuf: invalid fixed32")`)
			g.P("			}")
			g.P("			", mv, " |= 1 << ", bit)
			g.P("			x.", f.GoName, " = binary.LittleEndian.Uint32(data[pos:])")
			g.P("			pos += 4")
		case protoreflect.Fixed64Kind:
			g.P("			if n-pos < 8 {")
			g.P(`				return fmt.Errorf("omnibuf: invalid fixed64")`)
			g.P("			}")
			g.P("			", mv, " |= 1 << ", bit)
			g.P("			x.", f.GoName, " = binary.LittleEndian.Uint64(data[pos:])")
			g.P("			pos += 8")
		case protoreflect.Sfixed32Kind:
			g.P("			if n-pos < 4 {")
			g.P(`				return fmt.Errorf("omnibuf: invalid sfixed32")`)
			g.P("			}")
			g.P("			", mv, " |= 1 << ", bit)
			g.P("			x.", f.GoName, " = int32(binary.LittleEndian.Uint32(data[pos:]))")
			g.P("			pos += 4")
		case protoreflect.Sfixed64Kind:
			g.P("			if n-pos < 8 {")
			g.P(`				return fmt.Errorf("omnibuf: invalid sfixed64")`)
			g.P("			}")
			g.P("			", mv, " |= 1 << ", bit)
			g.P("			x.", f.GoName, " = int64(binary.LittleEndian.Uint64(data[pos:]))")
			g.P("			pos += 8")
		case protoreflect.FloatKind:
			g.P("			if n-pos < 4 {")
			g.P(`				return fmt.Errorf("omnibuf: invalid float32")`)
			g.P("			}")
			g.P("			", mv, " |= 1 << ", bit)
			g.P("			x.", f.GoName, " = math.Float32frombits(binary.LittleEndian.Uint32(data[pos:]))")
			g.P("			pos += 4")
		case protoreflect.DoubleKind:
			g.P("			if n-pos < 8 {")
			g.P(`				return fmt.Errorf("omnibuf: invalid float64")`)
			g.P("			}")
			g.P("			", mv, " |= 1 << ", bit)
			g.P("			x.", f.GoName, " = math.Float64frombits(binary.LittleEndian.Uint64(data[pos:]))")
			g.P("			pos += 8")
		case protoreflect.StringKind:
			g.P("			sLen64, sn := wire.DecodeVarint(data[pos:])")
			g.P("			if sn == 0 || sLen64 > uint64(n-(pos+sn)) {")
			g.P(`				return fmt.Errorf("omnibuf: invalid string length")`)
			g.P("			}")
			g.P("			pos += sn")
			g.P("			sLen := int(sLen64)")
			g.P("			", mv, " |= 1 << ", bit)
			g.P("			x.", f.GoName, " = wire.DecodeStr(data[pos:pos+sLen], zeroCopy)")
			g.P("			pos += sLen")
		case protoreflect.BytesKind:
			g.P("			bLen64, bn := wire.DecodeVarint(data[pos:])")
			g.P("			if bn == 0 || bLen64 > uint64(n-(pos+bn)) {")
			g.P(`				return fmt.Errorf("omnibuf: invalid bytes length")`)
			g.P("			}")
			g.P("			pos += bn")
			g.P("			bLen := int(bLen64)")
			g.P("			", mv, " |= 1 << ", bit)
			g.P("			if zeroCopy {")
			g.P("				x.", f.GoName, " = data[pos : pos+bLen]")
			g.P("			} else {")
			g.P("				x.", f.GoName, " = append(x.", f.GoName, "[:0], data[pos:pos+bLen]...)")
			g.P("			}")
			g.P("			pos += bLen")
		case protoreflect.MessageKind:
			subType := f.Message.GoIdent.GoName
			g.P("			subLen, sn := wire.DecodeVarint(data[pos:])")
			g.P("			if sn == 0 || subLen > uint64(n-(pos+sn)) {")
			g.P(`				return fmt.Errorf("omnibuf: invalid submsg length")`)
			g.P("			}")
			g.P("			pos += sn")
			g.P("			if x.", f.GoName, " == nil {")
			g.P("				x.", f.GoName, " = new(", subType, ")")
			g.P("			}")
			g.P("			subMerge := merge || (", mv, "&(1<<", bit, ")) != 0")
			g.P("			", mv, " |= 1 << ", bit)
			g.P("			if err := x.", f.GoName, ".decodeOmniBuf(data[pos:pos+int(subLen)], zeroCopy, depth+1, subMerge); err != nil {")
			g.P("				return err")
			g.P("			}")
			g.P("			pos += int(subLen)")
		}
	}
	g.P("		default:")
	g.P("			fnum := tag >> 4")
	g.P("			wt := uint8(tag & 0x0F)")
	g.P("			switch fnum {")
	g.P("			case ", strings.Join(fnumList, ", "), ":")
	g.P(`				return fmt.Errorf("omnibuf: unexpected wire type %d for field %d", wt, fnum)`)
	g.P("			}")
	g.P("			var err error")
	g.P("			pos, err = wire.SkipValue(data, pos, wt)")
	g.P("			if err != nil {")
	g.P("				return err")
	g.P("			}")
	g.P("		}")
	g.P("	}")
	if numWords == 1 {
		g.P("	if !merge && seenMask != expectedMask {")
	} else {
		g.P("	if !merge && (", strings.Join(maskCheckConds, " || "), ") {")
	}
	g.P("		x.clearUnseenOmniBuf(", strings.Join(clearArgs, ", "), ")")
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
	g.P("	dirty uint64")
	for _, f := range msg.Fields {
		fname := "f_" + f.Desc.TextName()
		if f.Desc.IsMap() {
			g.P("	", fname, " map[string]uint64")
			continue
		}
		if f.Desc.IsList() {
			switch f.Desc.Kind() {
			case protoreflect.StringKind:
				g.P("	", fname, " []string")
			case protoreflect.BytesKind:
				g.P("	", fname, " [][]byte")
			case protoreflect.BoolKind:
				g.P("	", fname, " []bool")
			case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
				g.P("	", fname, " []uint32")
			case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
				g.P("	", fname, " []uint64")
			case protoreflect.Sint32Kind, protoreflect.Int32Kind, protoreflect.Sfixed32Kind:
				g.P("	", fname, " []int32")
			case protoreflect.Sint64Kind, protoreflect.Int64Kind, protoreflect.Sfixed64Kind:
				g.P("	", fname, " []int64")
			case protoreflect.FloatKind:
				g.P("	", fname, " []float32")
			case protoreflect.DoubleKind:
				g.P("	", fname, " []float64")
			case protoreflect.MessageKind:
				g.P("	", fname, " []*", f.Message.GoIdent.GoName)
			}
			continue
		}
		switch f.Desc.Kind() {
		case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
			g.P("	", fname, " uint32")
		case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
			g.P("	", fname, " uint64")
		case protoreflect.Sint32Kind, protoreflect.Int32Kind, protoreflect.Sfixed32Kind:
			g.P("	", fname, " int32")
		case protoreflect.Sint64Kind, protoreflect.Int64Kind, protoreflect.Sfixed64Kind:
			g.P("	", fname, " int64")
		case protoreflect.BoolKind:
			g.P("	", fname, " bool")
		case protoreflect.FloatKind:
			g.P("	", fname, " float32")
		case protoreflect.DoubleKind:
			g.P("	", fname, " float64")
		case protoreflect.StringKind:
			g.P("	", fname, " string")
		case protoreflect.BytesKind:
			g.P("	", fname, " []byte")
		case protoreflect.MessageKind:
			g.P("	", fname, " *", f.Message.GoIdent.GoName)
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
		bitExpr := fmt.Sprintf("(uint64(1) << %d)", fnum)

		if f.Desc.IsMap() {
			g.P("func (x *", name, ") Lookup", goName, "(key string) (uint64, bool) {")
			g.P("	if (x.dirty & ", bitExpr, ") != 0 { v, ok := x.", privName, "[key]; return v, ok }")
			g.P("	return wire.LookupMapStringUint(x.raw, ", fnum, ", key)")
			g.P("}")
			g.P("func (x *", name, ") Get", goName, "() map[string]uint64 {")
			g.P("	if (x.dirty & ", bitExpr, ") != 0 { return x.", privName, " }")
			g.P("	m, _ := wire.ReadMapStringUint(x.raw, ", fnum, ", false)")
			g.P("	return m")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v map[string]uint64) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
			g.P()
			continue
		}

		if f.Desc.IsList() {
			switch f.Desc.Kind() {
			case protoreflect.StringKind:
				g.P("func (x *", name, ") ", goName, "Len() int {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return len(x.", privName, ") }")
				g.P("	return wire.CountedArrLen(x.raw, ", fnum, ", wire.WireString)")
				g.P("}")
				g.P()
				g.P("func (x *", name, ") Append", goName, "Into(dst []string) []string {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return append(dst, x.", privName, "...) }")
				g.P("	return wire.AppendStringArrZeroCopy(dst, x.raw, ", fnum, ")")
				g.P("}")
				g.P("func (x *", name, ") Set", goName, "(v []string) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
				g.P()
			case protoreflect.BytesKind:
				g.P("func (x *", name, ") ", goName, "Len() int {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return len(x.", privName, ") }")
				g.P("	return wire.CountedArrLen(x.raw, ", fnum, ", wire.WireBytes)")
				g.P("}")
				g.P("func (x *", name, ") Append", goName, "Into(dst [][]byte) [][]byte {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return append(dst, x.", privName, "...) }")
				g.P("	return wire.AppendBytesArr(dst, x.raw, ", fnum, ", false)")
				g.P("}")
				g.P("func (x *", name, ") Set", goName, "(v [][]byte) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
				g.P()
			case protoreflect.BoolKind:
				g.P("func (x *", name, ") ", goName, "Len() int {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return len(x.", privName, ") }")
				g.P("	return wire.CountedArrLen(x.raw, ", fnum, ", wire.WireBoolFalse)")
				g.P("}")
				g.P("func (x *", name, ") Append", goName, "Into(dst []bool) []bool {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return append(dst, x.", privName, "...) }")
				g.P("	return wire.AppendBoolArr(dst, x.raw, ", fnum, ")")
				g.P("}")
				g.P("func (x *", name, ") Set", goName, "(v []bool) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
				g.P()
			case protoreflect.Uint32Kind:
				g.P("func (x *", name, ") ", goName, "Len() int {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return len(x.", privName, ") }")
				g.P("	return wire.CountedArrLen(x.raw, ", fnum, ", wire.WireUint)")
				g.P("}")
				g.P()
				g.P("func (x *", name, ") Append", goName, "Into(dst []uint32) []uint32 {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return append(dst, x.", privName, "...) }")
				g.P("	return wire.AppendUint32Arr(dst, x.raw, ", fnum, ")")
				g.P("}")
				g.P("func (x *", name, ") Set", goName, "(v []uint32) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
				g.P()
			case protoreflect.Uint64Kind:
				g.P("func (x *", name, ") ", goName, "Len() int {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return len(x.", privName, ") }")
				g.P("	return wire.CountedArrLen(x.raw, ", fnum, ", wire.WireUint)")
				g.P("}")
				g.P("func (x *", name, ") Append", goName, "Into(dst []uint64) []uint64 {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return append(dst, x.", privName, "...) }")
				g.P("	return wire.AppendUint64Arr(dst, x.raw, ", fnum, ")")
				g.P("}")
				g.P("func (x *", name, ") Set", goName, "(v []uint64) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
				g.P()
			case protoreflect.Sint32Kind, protoreflect.Int32Kind:
				g.P("func (x *", name, ") ", goName, "Len() int {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return len(x.", privName, ") }")
				g.P("	return wire.CountedArrLen(x.raw, ", fnum, ", wire.WireSint)")
				g.P("}")
				g.P("func (x *", name, ") Append", goName, "Into(dst []int32) []int32 {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return append(dst, x.", privName, "...) }")
				g.P("	return wire.AppendSint32Arr(dst, x.raw, ", fnum, ")")
				g.P("}")
				g.P("func (x *", name, ") Set", goName, "(v []int32) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
				g.P()
			case protoreflect.Sint64Kind, protoreflect.Int64Kind:
				g.P("func (x *", name, ") ", goName, "Len() int {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return len(x.", privName, ") }")
				g.P("	return wire.CountedArrLen(x.raw, ", fnum, ", wire.WireSint)")
				g.P("}")
				g.P("func (x *", name, ") Append", goName, "Into(dst []int64) []int64 {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return append(dst, x.", privName, "...) }")
				g.P("	return wire.AppendSint64Arr(dst, x.raw, ", fnum, ")")
				g.P("}")
				g.P("func (x *", name, ") Set", goName, "(v []int64) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
				g.P()
			case protoreflect.Fixed32Kind:
				g.P("func (x *", name, ") ", goName, "Len() int {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return len(x.", privName, ") }")
				g.P("	return wire.CountedArrLen(x.raw, ", fnum, ", wire.WireFixed32)")
				g.P("}")
				g.P("func (x *", name, ") Append", goName, "Into(dst []uint32) []uint32 {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return append(dst, x.", privName, "...) }")
				g.P("	return wire.AppendFixed32Arr(dst, x.raw, ", fnum, ")")
				g.P("}")
				g.P("func (x *", name, ") Set", goName, "(v []uint32) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
				g.P()
			case protoreflect.Fixed64Kind:
				g.P("func (x *", name, ") ", goName, "Len() int {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return len(x.", privName, ") }")
				g.P("	return wire.CountedArrLen(x.raw, ", fnum, ", wire.WireFixed64)")
				g.P("}")
				g.P("func (x *", name, ") Append", goName, "Into(dst []uint64) []uint64 {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return append(dst, x.", privName, "...) }")
				g.P("	return wire.AppendFixed64Arr(dst, x.raw, ", fnum, ")")
				g.P("}")
				g.P("func (x *", name, ") Set", goName, "(v []uint64) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
				g.P()
			case protoreflect.Sfixed32Kind:
				g.P("func (x *", name, ") ", goName, "Len() int {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return len(x.", privName, ") }")
				g.P("	return wire.CountedArrLen(x.raw, ", fnum, ", wire.WireSfixed32)")
				g.P("}")
				g.P("func (x *", name, ") Append", goName, "Into(dst []int32) []int32 {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return append(dst, x.", privName, "...) }")
				g.P("	return wire.AppendSfixed32Arr(dst, x.raw, ", fnum, ")")
				g.P("}")
				g.P("func (x *", name, ") Set", goName, "(v []int32) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
				g.P()
			case protoreflect.Sfixed64Kind:
				g.P("func (x *", name, ") ", goName, "Len() int {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return len(x.", privName, ") }")
				g.P("	return wire.CountedArrLen(x.raw, ", fnum, ", wire.WireSfixed64)")
				g.P("}")
				g.P("func (x *", name, ") Append", goName, "Into(dst []int64) []int64 {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return append(dst, x.", privName, "...) }")
				g.P("	return wire.AppendSfixed64Arr(dst, x.raw, ", fnum, ")")
				g.P("}")
				g.P("func (x *", name, ") Set", goName, "(v []int64) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
				g.P()
			case protoreflect.FloatKind:
				g.P("func (x *", name, ") ", goName, "Len() int {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return len(x.", privName, ") }")
				g.P("	return wire.CountedArrLen(x.raw, ", fnum, ", wire.WireFloat32)")
				g.P("}")
				g.P("func (x *", name, ") Append", goName, "Into(dst []float32) []float32 {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return append(dst, x.", privName, "...) }")
				g.P("	return wire.AppendFloat32Arr(dst, x.raw, ", fnum, ")")
				g.P("}")
				g.P("func (x *", name, ") Set", goName, "(v []float32) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
				g.P()
			case protoreflect.DoubleKind:
				g.P("func (x *", name, ") ", goName, "Len() int {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return len(x.", privName, ") }")
				g.P("	return wire.CountedArrLen(x.raw, ", fnum, ", wire.WireFloat64)")
				g.P("}")
				g.P("func (x *", name, ") Append", goName, "Into(dst []float64) []float64 {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return append(dst, x.", privName, "...) }")
				g.P("	return wire.AppendFloat64Arr(dst, x.raw, ", fnum, ")")
				g.P("}")
				g.P("func (x *", name, ") Set", goName, "(v []float64) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
				g.P()
			case protoreflect.MessageKind:
				elemType := f.Message.GoIdent.GoName
				subOpaque := "Opaque" + elemType
				g.P("func (x *", name, ") Get", goName, "(idx int) ", subOpaque, " {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 {")
				g.P("		if idx >= 0 && idx < len(x.", privName, ") && x.", privName, "[idx] != nil {")
				g.P("			return ", subOpaque, "{raw: x.", privName, "[idx].MarshalOmniBufAppend(nil)}")
				g.P("		}")
				g.P("		return ", subOpaque, "{}")
				g.P("	}")
				g.P("	elem, _ := wire.ReadSubMsgArrElem(x.raw, ", fnum, ", idx)")
				g.P("	return ", subOpaque, "{raw: elem}")
				g.P("}")
				g.P()
				g.P("func (x *", name, ") ", goName, "Len() int {")
				g.P("	if (x.dirty & ", bitExpr, ") != 0 { return len(x.", privName, ") }")
				g.P("	return wire.SubMsgArrLen(x.raw, ", fnum, ")")
				g.P("}")
				g.P("func (x *", name, ") Set", goName, "(v []*", elemType, ") { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
				g.P()
			}
			continue
		}

		switch f.Desc.Kind() {
		case protoreflect.Uint32Kind:
			g.P("func (x ", name, ") Get", goName, "() uint32 {")
			g.P("	if (x.dirty & ", bitExpr, ") != 0 { return x.", privName, " }")
			g.P("	v, _ := wire.ReadUint(x.raw, ", fnum, ")")
			g.P("	return uint32(v)")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v uint32) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
		case protoreflect.Uint64Kind:
			g.P("func (x ", name, ") Get", goName, "() uint64 {")
			g.P("	if (x.dirty & ", bitExpr, ") != 0 { return x.", privName, " }")
			g.P("	v, _ := wire.ReadUint(x.raw, ", fnum, ")")
			g.P("	return v")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v uint64) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
		case protoreflect.Sint32Kind, protoreflect.Int32Kind:
			g.P("func (x ", name, ") Get", goName, "() int32 {")
			g.P("	if (x.dirty & ", bitExpr, ") != 0 { return x.", privName, " }")
			g.P("	v, _ := wire.ReadInt(x.raw, ", fnum, ")")
			g.P("	return int32(v)")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v int32) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
		case protoreflect.Sint64Kind, protoreflect.Int64Kind:
			g.P("func (x ", name, ") Get", goName, "() int64 {")
			g.P("	if (x.dirty & ", bitExpr, ") != 0 { return x.", privName, " }")
			g.P("	v, _ := wire.ReadInt(x.raw, ", fnum, ")")
			g.P("	return v")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v int64) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
		case protoreflect.Fixed32Kind:
			g.P("func (x ", name, ") Get", goName, "() uint32 {")
			g.P("	if (x.dirty & ", bitExpr, ") != 0 { return x.", privName, " }")
			g.P("	v, _ := wire.ReadFixed32(x.raw, ", fnum, ")")
			g.P("	return v")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v uint32) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
		case protoreflect.Fixed64Kind:
			g.P("func (x ", name, ") Get", goName, "() uint64 {")
			g.P("	if (x.dirty & ", bitExpr, ") != 0 { return x.", privName, " }")
			g.P("	v, _ := wire.ReadFixed64(x.raw, ", fnum, ")")
			g.P("	return v")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v uint64) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
		case protoreflect.Sfixed32Kind:
			g.P("func (x ", name, ") Get", goName, "() int32 {")
			g.P("	if (x.dirty & ", bitExpr, ") != 0 { return x.", privName, " }")
			g.P("	v, _ := wire.ReadSfixed32(x.raw, ", fnum, ")")
			g.P("	return v")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v int32) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
		case protoreflect.Sfixed64Kind:
			g.P("func (x ", name, ") Get", goName, "() int64 {")
			g.P("	if (x.dirty & ", bitExpr, ") != 0 { return x.", privName, " }")
			g.P("	v, _ := wire.ReadSfixed64(x.raw, ", fnum, ")")
			g.P("	return v")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v int64) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
		case protoreflect.BoolKind:
			g.P("func (x ", name, ") Get", goName, "() bool {")
			g.P("	if (x.dirty & ", bitExpr, ") != 0 { return x.", privName, " }")
			g.P("	v, _ := wire.ReadBool(x.raw, ", fnum, ")")
			g.P("	return v")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v bool) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
		case protoreflect.FloatKind:
			g.P("func (x ", name, ") Get", goName, "() float32 {")
			g.P("	if (x.dirty & ", bitExpr, ") != 0 { return x.", privName, " }")
			g.P("	v, _ := wire.ReadFloat32(x.raw, ", fnum, ")")
			g.P("	return v")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v float32) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
		case protoreflect.DoubleKind:
			g.P("func (x ", name, ") Get", goName, "() float64 {")
			g.P("	if (x.dirty & ", bitExpr, ") != 0 { return x.", privName, " }")
			g.P("	v, _ := wire.ReadFloat64(x.raw, ", fnum, ")")
			g.P("	return v")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v float64) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
		case protoreflect.StringKind:
			g.P("func (x ", name, ") Get", goName, "() string {")
			g.P("	if (x.dirty & ", bitExpr, ") != 0 { return x.", privName, " }")
			g.P("	s, _ := wire.ReadStringZeroCopy(x.raw, ", fnum, ")")
			g.P("	return s")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v string) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
		case protoreflect.BytesKind:
			g.P("func (x ", name, ") Get", goName, "() []byte {")
			g.P("	if (x.dirty & ", bitExpr, ") != 0 { return x.", privName, " }")
			g.P("	b, _ := wire.ReadBytes(x.raw, ", fnum, ")")
			g.P("	return b")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v []byte) { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
		case protoreflect.MessageKind:
			elemType := f.Message.GoIdent.GoName
			subOpaque := "Opaque" + elemType
			g.P("func (x *", name, ") Get", goName, "() ", subOpaque, " {")
			g.P("	if (x.dirty & ", bitExpr, ") != 0 {")
			g.P("		if x.", privName, " != nil {")
			g.P("			return ", subOpaque, "{raw: x.", privName, ".MarshalOmniBufAppend(nil)}")
			g.P("		}")
			g.P("		return ", subOpaque, "{}")
			g.P("	}")
			g.P("	sub, _ := wire.ReadSubMsg(x.raw, ", fnum, ")")
			g.P("	return ", subOpaque, "{raw: sub}")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v *", elemType, ") { x.", privName, " = v; x.dirty |= ", bitExpr, " }")
		}
		g.P()
	}

	// Generate MarshalAppend on Opaque struct with full zero-decode splice support across all field types!
	maxFnum := uint16(msg.Fields[len(msg.Fields)-1].Desc.Number())
	g.P("// MarshalAppend re-serializes ", name, " using zero-decode wire splicing for unmodified fields.")
	g.P("func (x *", name, ") MarshalAppend(dst []byte) []byte {")
	g.P("	dst = dst[:0]")
	g.P("	if x.dirty == 0 {")
	g.P("		return append(dst, x.raw...)")
	g.P("	}")
	g.P("	return wire.SpliceFields(dst, x.raw, x.dirty, ", maxFnum, ", func(b []byte, fnum uint16) []byte {")
	g.P("		switch fnum {")
	for _, f := range msg.Fields {
		fnum := uint16(f.Desc.Number())
		privName := "f_" + f.Desc.TextName()
		g.P("		case ", fnum, ":")
		if f.Desc.Kind() == protoreflect.BoolKind && !f.Desc.IsList() {
			g.P("			if x.", privName, " { return wire.AppendVarint(b, ", (uint64(fnum)<<4)|uint64(wire.WireBoolTrue), ") }")
			g.P("			return wire.AppendVarint(b, ", (uint64(fnum)<<4)|uint64(wire.WireBoolFalse), ")")
			continue
		}
		if f.Desc.Kind() == protoreflect.MessageKind && !f.Desc.IsList() && !f.Desc.IsMap() {
			g.P("			if x.", privName, " == nil { return b }")
		}

		rawTag := (uint64(fnum) << 4) | uint64(wireTypeForField(f))
		emitConstTag(g, "b", rawTag)

		if f.Desc.IsMap() {
			g.P("			return wire.EncodeMapStringUint(b, x.", privName, ", true)")
			continue
		}
		if f.Desc.IsList() {
			switch f.Desc.Kind() {
			case protoreflect.StringKind:
				g.P("			return wire.EncodeStringArr(b, x.", privName, ")")
			case protoreflect.BytesKind:
				g.P("			return wire.EncodeBytesArr(b, x.", privName, ")")
			case protoreflect.BoolKind:
				g.P("			return wire.EncodeBoolArr(b, x.", privName, ")")
			case protoreflect.Uint32Kind, protoreflect.Uint64Kind:
				g.P("			return wire.EncodeUintArr(b, x.", privName, ")")
			case protoreflect.Sint32Kind, protoreflect.Int32Kind, protoreflect.Sint64Kind, protoreflect.Int64Kind:
				g.P("			return wire.EncodeSintArr(b, x.", privName, ")")
			case protoreflect.Fixed32Kind:
				g.P("			return wire.EncodeFixed32Arr(b, x.", privName, ", wire.WireFixed32)")
			case protoreflect.Fixed64Kind:
				g.P("			return wire.EncodeFixed64Arr(b, x.", privName, ", wire.WireFixed64)")
			case protoreflect.Sfixed32Kind:
				g.P("			return wire.EncodeFixed32Arr(b, x.", privName, ", wire.WireSfixed32)")
			case protoreflect.Sfixed64Kind:
				g.P("			return wire.EncodeFixed64Arr(b, x.", privName, ", wire.WireSfixed64)")
			case protoreflect.FloatKind:
				g.P("			return wire.EncodeFloat32Arr(b, x.", privName, ")")
			case protoreflect.DoubleKind:
				g.P("			return wire.EncodeFloat64Arr(b, x.", privName, ")")
			case protoreflect.MessageKind:
				g.P("			lenPos := len(b)")
				g.P("			b = append(b, 0, 0, wire.WireSubMsg)")
				g.P("			b = wire.AppendVarint(b, uint64(len(x.", privName, ")))")
				g.P("			for _, elem := range x.", privName, " {")
				g.P("				eLenPos := len(b)")
				g.P("				b = append(b, 0, 0)")
				g.P("				if elem != nil {")
				g.P("					b = elem.MarshalOmniBufAppend(b)")
				g.P("				}")
				g.P("				b = wire.BackpatchContainerLen(b, eLenPos)")
				g.P("			}")
				g.P("			return wire.BackpatchContainerLen(b, lenPos)")
			}
			continue
		}

		switch f.Desc.Kind() {
		case protoreflect.Uint32Kind:
			g.P("			return wire.AppendVarint(b, uint64(x.", privName, "))")
		case protoreflect.Uint64Kind:
			g.P("			return wire.AppendVarint(b, x.", privName, ")")
		case protoreflect.Sint32Kind, protoreflect.Int32Kind:
			g.P("			return wire.AppendVarint(b, wire.EncodeZigZag(int64(x.", privName, ")))")
		case protoreflect.Sint64Kind, protoreflect.Int64Kind:
			g.P("			return wire.AppendVarint(b, wire.EncodeZigZag(x.", privName, "))")
		case protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind:
			g.P("			v := uint32(x.", privName, ")")
			g.P("			return append(b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))")
		case protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind:
			g.P("			v := uint64(x.", privName, ")")
			g.P("			return append(b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24), byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56))")
		case protoreflect.FloatKind:
			g.P("			v := math.Float32bits(x.", privName, ")")
			g.P("			return append(b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))")
		case protoreflect.DoubleKind:
			g.P("			v := math.Float64bits(x.", privName, ")")
			g.P("			return append(b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24), byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56))")
		case protoreflect.StringKind, protoreflect.BytesKind:
			g.P("			b = wire.AppendVarint(b, uint64(len(x.", privName, ")))")
			g.P("			return append(b, x.", privName, "...)")
		case protoreflect.MessageKind:
			g.P("			lenPos := len(b)")
			g.P("			b = append(b, 0, 0)")
			g.P("			if x.", privName, " != nil {")
			g.P("				b = x.", privName, ".MarshalOmniBufAppend(b)")
			g.P("			}")
			g.P("			return wire.BackpatchContainerLen(b, lenPos)")
		}
	}
	g.P("		}")
	g.P("		return b")
	g.P("	})")
	g.P("}")
	g.P()
}

// The optimized encoder supports this explicit subset. Reject other
// schemas at generation time; the descriptor-aware runtime supports more types.
func validateGeneratedFile(file *protogen.File) error {
	for _, msg := range file.Messages {
		for _, nm := range msg.Messages {
			if !nm.Desc.IsMapEntry() {
				return fmt.Errorf("omnibuf: generated codecs do not yet support nested declarations in %s; use omnibuf.Marshal", msg.Desc.FullName())
			}
		}
		for _, f := range msg.Fields {
			if f.Desc.Number() > 65535 || f.Desc.HasOptionalKeyword() || f.Oneof != nil || (f.Desc.HasPresence() && f.Desc.Kind() != protoreflect.MessageKind) {
				return fmt.Errorf("omnibuf: generated codec unsupported field %s; use descriptor-aware omnibuf.Marshal/Unmarshal", f.Desc.FullName())
			}
			if f.Desc.IsMap() {
				if f.Desc.MapKey().Kind() == protoreflect.StringKind && f.Desc.MapValue().Kind() == protoreflect.Uint64Kind {
					continue
				}
				return fmt.Errorf("omnibuf: generated codec unsupported map field %s; use descriptor-aware omnibuf.Marshal/Unmarshal", f.Desc.FullName())
			}
			if f.Message != nil && f.Message.GoIdent.GoImportPath != msg.GoIdent.GoImportPath {
				return fmt.Errorf("omnibuf: generated codec cannot call external message %s; use omnibuf.Marshal", f.Desc.FullName())
			}
			if f.Desc.IsList() {
				switch f.Desc.Kind() {
				case protoreflect.StringKind, protoreflect.BytesKind, protoreflect.BoolKind, protoreflect.Uint32Kind, protoreflect.Uint64Kind, protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Fixed32Kind, protoreflect.Fixed64Kind, protoreflect.Sfixed32Kind, protoreflect.Sfixed64Kind, protoreflect.FloatKind, protoreflect.DoubleKind, protoreflect.MessageKind:
					continue
				}
			} else {
				switch f.Desc.Kind() {
				case protoreflect.StringKind, protoreflect.BytesKind, protoreflect.Uint32Kind, protoreflect.Uint64Kind, protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Fixed32Kind, protoreflect.Fixed64Kind, protoreflect.Sfixed32Kind, protoreflect.Sfixed64Kind, protoreflect.BoolKind, protoreflect.FloatKind, protoreflect.DoubleKind, protoreflect.MessageKind:
					continue
				}
			}
			return fmt.Errorf("omnibuf: generated codec unsupported field %s (%s); use omnibuf.Marshal", f.Desc.FullName(), f.Desc.Kind())
		}
	}
	return nil
}
func canGenerateOpaque(msg *protogen.Message) bool {
	if len(msg.Fields) == 0 {
		return false
	}
	for _, f := range msg.Fields {
		if f.Desc.Number() > 63 {
			return false
		}
	}
	return true
}

func generatedFileUsesMath(file *protogen.File) bool {
	for _, m := range file.Messages {
		for _, f := range m.Fields {
			if f.Desc.Kind() == protoreflect.FloatKind || f.Desc.Kind() == protoreflect.DoubleKind {
				return true
			}
		}
	}
	return false
}
