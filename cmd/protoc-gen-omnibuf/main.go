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
	vtGenOut := flag.String("vtproto-gen", "", "Run vtprotobuf generator on omnibuf/bench/peer.proto descriptor and write generated file to path")
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

	var expectedMask uint64
	var fnumList []string
	fnumList = append(fnumList, "0")
	for i, f := range msg.Fields {
		if i < 64 {
			expectedMask |= 1 << i
		}
		fnumList = append(fnumList, fmt.Sprintf("%d", f.Desc.Number()))
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
		emitConstTag(g, "dst", rawTag)

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
			case protoreflect.Uint32Kind:
				g.P("	lenPos_", i, " := len(dst)")
				g.P("	dst = append(dst, 0, 0, wire.WireUint)")
				g.P("	dst = wire.AppendVarint(dst, uint64(len(x.", f.GoName, ")))")
				g.P("	for _, v := range x.", f.GoName, " {")
				g.P("		dst = wire.AppendVarint(dst, uint64(v))")
				g.P("	}")
				emitBackpatchLen(g, "dst", fmt.Sprintf("lenPos_%d", i))
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
		case protoreflect.FloatKind:
			g.P("	b_", i, " := math.Float32bits(x.", f.GoName, ")")
			g.P("	dst = append(dst, byte(b_", i, "), byte(b_", i, ">>8), byte(b_", i, ">>16), byte(b_", i, ">>24))")
		case protoreflect.DoubleKind:
			g.P("	b_", i, " := math.Float64bits(x.", f.GoName, ")")
			g.P("	dst = append(dst, byte(b_", i, "), byte(b_", i, ">>8), byte(b_", i, ">>16), byte(b_", i, ">>24), byte(b_", i, ">>32), byte(b_", i, ">>40), byte(b_", i, ">>48), byte(b_", i, ">>56))")
		case protoreflect.StringKind:
			g.P("	dst = wire.AppendVarint(dst, uint64(len(x.", f.GoName, ")))")
			g.P("	dst = append(dst, x.", f.GoName, "...)")
		case protoreflect.MessageKind:
			g.P("	lenPos_", i, " := len(dst)")
			g.P("	dst = append(dst, 0, 0)")
			g.P("	if x.", f.GoName, " != nil {")
			g.P("		dst = x.", f.GoName, ".MarshalOmniBufAppend(dst)")
			g.P("	}")
			emitBackpatchLen(g, "dst", fmt.Sprintf("lenPos_%d", i))
		}
	}
	g.P("	return dst")
	g.P("}")
	g.P()

	g.P("//go:noinline")
	g.P("func (x *", typeName, ") clearUnseenOmniBuf(seenMask uint64) {")
	for i, f := range msg.Fields {
		if i >= 64 {
			continue
		}
		g.P(fmt.Sprintf("	if (seenMask & (1 << %d)) == 0 {", i))
		if f.Desc.IsList() {
			g.P("		x.", f.GoName, " = x.", f.GoName, "[:0]")
		} else {
			switch f.Desc.Kind() {
			case protoreflect.Uint32Kind, protoreflect.Uint64Kind, protoreflect.Sint32Kind, protoreflect.Int32Kind, protoreflect.FloatKind, protoreflect.DoubleKind:
				g.P("		x.", f.GoName, " = 0")
			case protoreflect.BoolKind:
				g.P("		x.", f.GoName, " = false")
			case protoreflect.StringKind:
				g.P(`		x.`, f.GoName, ` = ""`)
			case protoreflect.MessageKind:
				g.P("		if x.", f.GoName, " != nil {")
				g.P("			x.", f.GoName, ".clearUnseenOmniBuf(0)")
				g.P("		}")
			}
		}
		g.P("	}")
	}
	g.P("}")
	g.P()

	g.P("// UnmarshalOmniBuf decodes data into ", typeName, " using generated switch dispatch.")
	g.P("func (x *", typeName, ") UnmarshalOmniBuf(data []byte, zeroCopy bool) error {")
	if len(msg.Fields) > 64 {
		g.P("	x.clearUnseenOmniBuf(0)")
	}
	g.P("	return x.decodeOmniBuf(data, zeroCopy, 0, false)")
	g.P("}")
	g.P()

	g.P("func (x *", typeName, ") decodeOmniBuf(data []byte, zeroCopy bool, depth int, merge bool) error {")
	g.P("	if depth > omnibuf.MaxNestingDepth {")
	g.P(`		return fmt.Errorf("omnibuf: max nesting depth exceeded")`)
	g.P("	}")
	g.P(fmt.Sprintf("	const expectedMask uint64 = 0x%x", expectedMask))
	g.P("	var seenMask uint64")
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
		if f.Desc.Kind() == protoreflect.BoolKind && !f.Desc.IsList() {
			g.P("		case ", (uint64(fnum)<<4)|uint64(wire.WireBoolTrue), ":")
			g.P("			seenMask |= 1 << ", i)
			g.P("			x.", f.GoName, " = true")
			g.P("		case ", (uint64(fnum)<<4)|uint64(wire.WireBoolFalse), ":")
			g.P("			seenMask |= 1 << ", i)
			g.P("			x.", f.GoName, " = false")
			continue
		}
		g.P("		case ", (uint64(fnum)<<4)|uint64(wt), ":")
		if f.Desc.IsList() {
			g.P("			byteLen, bn := wire.DecodeVarint(data[pos:])")
			g.P("			if bn == 0 || byteLen > uint64(n-(pos+bn)) {")
			g.P(`				return fmt.Errorf("omnibuf: invalid array byteLen")`)
			g.P("			}")
			g.P("			pos += bn")
			g.P("			body := data[pos : pos+int(byteLen)]")
			g.P("			pos += int(byteLen)")
			g.P("			if !merge && (seenMask&(1<<", i, ")) == 0 {")
			g.P("				x.", f.GoName, " = x.", f.GoName, "[:0]")
			g.P("			}")
			g.P("			seenMask |= 1 << ", i)
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
			g.P("			seenMask |= 1 << ", i)
			g.P("			x.", f.GoName, " = uint32(v)")
		case protoreflect.Uint64Kind:
			g.P("			v, vn := wire.DecodeVarint(data[pos:])")
			g.P("			if vn == 0 {")
			g.P(`				return fmt.Errorf("omnibuf: truncated uint64")`)
			g.P("			}")
			g.P("			pos += vn")
			g.P("			seenMask |= 1 << ", i)
			g.P("			x.", f.GoName, " = v")
		case protoreflect.Sint32Kind, protoreflect.Int32Kind:
			g.P("			v, vn := wire.DecodeVarint(data[pos:])")
			g.P("			if vn == 0 {")
			g.P(`				return fmt.Errorf("omnibuf: truncated sint32")`)
			g.P("			}")
			g.P("			pos += vn")
			g.P("			seenMask |= 1 << ", i)
			g.P("			x.", f.GoName, " = int32(wire.DecodeZigZag(v))")
		case protoreflect.FloatKind:
			g.P("			if n-pos < 4 {")
			g.P(`				return fmt.Errorf("omnibuf: invalid float32")`)
			g.P("			}")
			g.P("			seenMask |= 1 << ", i)
			g.P("			x.", f.GoName, " = math.Float32frombits(binary.LittleEndian.Uint32(data[pos:]))")
			g.P("			pos += 4")
		case protoreflect.DoubleKind:
			g.P("			if n-pos < 8 {")
			g.P(`				return fmt.Errorf("omnibuf: invalid float64")`)
			g.P("			}")
			g.P("			seenMask |= 1 << ", i)
			g.P("			x.", f.GoName, " = math.Float64frombits(binary.LittleEndian.Uint64(data[pos:]))")
			g.P("			pos += 8")
		case protoreflect.StringKind:
			g.P("			sLen64, sn := wire.DecodeVarint(data[pos:])")
			g.P("			if sn == 0 || sLen64 > uint64(n-(pos+sn)) {")
			g.P(`				return fmt.Errorf("omnibuf: invalid string length")`)
			g.P("			}")
			g.P("			pos += sn")
			g.P("			sLen := int(sLen64)")
			g.P("			seenMask |= 1 << ", i)
			g.P("			x.", f.GoName, " = wire.DecodeStr(data[pos:pos+sLen], zeroCopy)")
			g.P("			pos += sLen")
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
			g.P("			subMerge := merge || (seenMask&(1<<", i, ")) != 0")
			g.P("			seenMask |= 1 << ", i)
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
	g.P("	if !merge && seenMask != expectedMask {")
	g.P("		x.clearUnseenOmniBuf(seenMask)")
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
				g.P("	return wire.CountedArrLen(x.raw, ", fnum, ", wire.WireString)")
				g.P("}")
				g.P()
				g.P("func (x *", name, ") Append", goName, "Into(dst []string) []string {")
				g.P("	return wire.AppendStringArrZeroCopy(dst, x.raw, ", fnum, ")")
				g.P("}")
				g.P()
			case protoreflect.Uint32Kind:
				g.P("func (x *", name, ") ", goName, "Len() int {")
				g.P("	return wire.CountedArrLen(x.raw, ", fnum, ", wire.WireUint)")
				g.P("}")
				g.P()
				g.P("func (x *", name, ") Append", goName, "Into(dst []uint32) []uint32 {")
				g.P("	return wire.AppendUint32Arr(dst, x.raw, ", fnum, ")")
				g.P("}")
				g.P()
			case protoreflect.MessageKind:
				subOpaque := "Opaque" + f.Message.GoIdent.GoName
				g.P("func (x *", name, ") Get", goName, "(idx int) ", subOpaque, " {")
				g.P("	elem, _ := wire.ReadSubMsgArrElem(x.raw, ", fnum, ", idx)")
				g.P("	return ", subOpaque, "{raw: elem}")
				g.P("}")
				g.P()
				g.P("func (x *", name, ") ", goName, "Len() int {")
				g.P("	return wire.SubMsgArrLen(x.raw, ", fnum, ")")
				g.P("}")
				g.P()
			}
			continue
		}

		switch f.Desc.Kind() {
		case protoreflect.Uint32Kind:
			g.P("func (x ", name, ") Get", goName, "() uint32 {")
			g.P("	if (x.dirty & (1 << ", fnum, ")) != 0 { return x.", privName, " }")
			g.P("	v, _ := wire.ReadUint(x.raw, ", fnum, ")")
			g.P("	return uint32(v)")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v uint32) { x.", privName, " = v; x.dirty |= (1 << ", fnum, ") }")
		case protoreflect.Uint64Kind:
			g.P("func (x ", name, ") Get", goName, "() uint64 {")
			g.P("	if (x.dirty & (1 << ", fnum, ")) != 0 { return x.", privName, " }")
			g.P("	v, _ := wire.ReadUint(x.raw, ", fnum, ")")
			g.P("	return v")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v uint64) { x.", privName, " = v; x.dirty |= (1 << ", fnum, ") }")
		case protoreflect.Sint32Kind, protoreflect.Int32Kind:
			g.P("func (x ", name, ") Get", goName, "() int32 {")
			g.P("	if (x.dirty & (1 << ", fnum, ")) != 0 { return x.", privName, " }")
			g.P("	v, _ := wire.ReadInt(x.raw, ", fnum, ")")
			g.P("	return int32(v)")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v int32) { x.", privName, " = v; x.dirty |= (1 << ", fnum, ") }")
		case protoreflect.BoolKind:
			g.P("func (x ", name, ") Get", goName, "() bool {")
			g.P("	if (x.dirty & (1 << ", fnum, ")) != 0 { return x.", privName, " }")
			g.P("	v, _ := wire.ReadBool(x.raw, ", fnum, ")")
			g.P("	return v")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v bool) { x.", privName, " = v; x.dirty |= (1 << ", fnum, ") }")
		case protoreflect.FloatKind:
			g.P("func (x ", name, ") Get", goName, "() float32 {")
			g.P("	if (x.dirty & (1 << ", fnum, ")) != 0 { return x.", privName, " }")
			g.P("	v, _ := wire.ReadFloat32(x.raw, ", fnum, ")")
			g.P("	return v")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v float32) { x.", privName, " = v; x.dirty |= (1 << ", fnum, ") }")
		case protoreflect.DoubleKind:
			g.P("func (x ", name, ") Get", goName, "() float64 {")
			g.P("	if (x.dirty & (1 << ", fnum, ")) != 0 { return x.", privName, " }")
			g.P("	v, _ := wire.ReadFloat64(x.raw, ", fnum, ")")
			g.P("	return v")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v float64) { x.", privName, " = v; x.dirty |= (1 << ", fnum, ") }")
		case protoreflect.StringKind:
			g.P("func (x ", name, ") Get", goName, "() string {")
			g.P("	if (x.dirty & (1 << ", fnum, ")) != 0 { return x.", privName, " }")
			g.P("	s, _ := wire.ReadStringZeroCopy(x.raw, ", fnum, ")")
			g.P("	return s")
			g.P("}")
			g.P("func (x *", name, ") Set", goName, "(v string) { x.", privName, " = v; x.dirty |= (1 << ", fnum, ") }")
		case protoreflect.MessageKind:
			subOpaque := "Opaque" + f.Message.GoIdent.GoName
			g.P("func (x *", name, ") Get", goName, "() ", subOpaque, " {")
			g.P("	sub, _ := wire.ReadSubMsg(x.raw, ", fnum, ")")
			g.P("	return ", subOpaque, "{raw: sub}")
			g.P("}")
		}
		g.P()
	}

	// Generate MarshalAppend on Opaque struct with full scalar zero-decode splice support!
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
		if f.Desc.IsList() || f.Desc.Kind() == protoreflect.MessageKind {
			continue
		}
		fnum := uint16(f.Desc.Number())
		privName := "f_" + f.Desc.TextName()
		g.P("		case ", fnum, ":")
		if f.Desc.Kind() == protoreflect.BoolKind {
			g.P("if x.", privName, " { return wire.AppendVarint(b, ", (uint64(fnum)<<4)|uint64(wire.WireBoolTrue), ") }")
			g.P("return wire.AppendVarint(b, ", (uint64(fnum)<<4)|uint64(wire.WireBoolFalse), ")")
			continue
		}

		rawTag := (fnum << 4) | uint16(wireTypeForField(f))
		if rawTag <= 127 {
			g.P(fmt.Sprintf("			b = append(b, 0x%02x)", byte(rawTag<<1)|1))
		} else {
			tag2 := (rawTag << 2) | 2
			g.P(fmt.Sprintf("			b = append(b, 0x%02x, 0x%02x)", byte(tag2), byte(tag2>>8)))
		}
		switch f.Desc.Kind() {
		case protoreflect.Uint32Kind:
			g.P("			return wire.AppendVarint(b, uint64(x.", privName, "))")
		case protoreflect.Uint64Kind:
			g.P("			return wire.AppendVarint(b, x.", privName, ")")
		case protoreflect.Sint32Kind, protoreflect.Int32Kind:
			g.P("			return wire.AppendVarint(b, wire.EncodeZigZag(int64(x.", privName, ")))")
		case protoreflect.FloatKind:
			g.P("			v := math.Float32bits(x.", privName, ")")
			g.P("			return append(b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))")
		case protoreflect.DoubleKind:
			g.P("			v := math.Float64bits(x.", privName, ")")
			g.P("			return append(b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24), byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56))")
		case protoreflect.StringKind:
			g.P("			b = wire.AppendVarint(b, uint64(len(x.", privName, ")))")
			g.P("			return append(b, x.", privName, "...)")
		}
	}
	g.P("		}")
	g.P("		return b")
	g.P("	})")
	g.P("}")
	g.P()
}

// The optimized encoder currently supports this explicit subset. Reject other
// schemas at generation time; the descriptor-aware runtime supports more types.
func validateGeneratedFile(file *protogen.File) error {
	for _, msg := range file.Messages {
		if len(msg.Messages) != 0 {
			return fmt.Errorf("omnibuf: generated codecs do not yet support nested declarations in %s; use omnibuf.Marshal", msg.Desc.FullName())
		}
		for _, f := range msg.Fields {
			if f.Desc.Number() > 65535 || f.Desc.IsMap() || f.Desc.HasOptionalKeyword() || f.Oneof != nil || (f.Desc.HasPresence() && f.Desc.Kind() != protoreflect.MessageKind) {
				return fmt.Errorf("omnibuf: generated codec unsupported field %s; use descriptor-aware omnibuf.Marshal/Unmarshal", f.Desc.FullName())
			}
			if f.Message != nil && f.Message.GoIdent.GoImportPath != msg.GoIdent.GoImportPath {
				return fmt.Errorf("omnibuf: generated codec cannot call external message %s; use omnibuf.Marshal", f.Desc.FullName())
			}
			if f.Desc.IsList() {
				switch f.Desc.Kind() {
				case protoreflect.StringKind, protoreflect.Uint32Kind, protoreflect.MessageKind:
					continue
				}
			} else {
				switch f.Desc.Kind() {
				case protoreflect.StringKind, protoreflect.Uint32Kind, protoreflect.Uint64Kind, protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.BoolKind, protoreflect.FloatKind, protoreflect.DoubleKind, protoreflect.MessageKind:
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
		if f.Desc.Number() > 15 {
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
