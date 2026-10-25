package omnibuf_test

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"

	. "github.com/sudorandom/omnibuf"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

var updateGolden = flag.Bool("update-golden", false, "regenerate golden files in testdata/")

func buildAllWireTypesFDS() *descriptorpb.FileDescriptorSet {
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	rep := descriptorpb.FieldDescriptorProto_LABEL_REPEATED

	subMsg := &descriptorpb.DescriptorProto{
		Name: proto.String("SubItem"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{Name: proto.String("id"), Number: proto.Int32(1), Label: opt.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_UINT32.Enum()},
			{Name: proto.String("label"), Number: proto.Int32(2), Label: opt.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
			{Name: proto.String("active"), Number: proto.Int32(3), Label: opt.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_BOOL.Enum()},
		},
	}

	mapEntry := func(name string, kType, vType descriptorpb.FieldDescriptorProto_Type, vTypeName string) *descriptorpb.DescriptorProto {
		vf := &descriptorpb.FieldDescriptorProto{
			Name:   proto.String("value"),
			Number: proto.Int32(2),
			Label:  opt.Enum(),
			Type:   vType.Enum(),
		}
		if vTypeName != "" {
			vf.TypeName = proto.String(vTypeName)
		}
		return &descriptorpb.DescriptorProto{
			Name:    proto.String(name),
			Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)},
			Field: []*descriptorpb.FieldDescriptorProto{
				{Name: proto.String("key"), Number: proto.Int32(1), Label: opt.Enum(), Type: kType.Enum()},
				vf,
			},
		}
	}

	rootMsg := &descriptorpb.DescriptorProto{
		Name: proto.String("AllWireTypesMsg"),
		NestedType: []*descriptorpb.DescriptorProto{
			mapEntry("MetricsEntry", descriptorpb.FieldDescriptorProto_TYPE_STRING, descriptorpb.FieldDescriptorProto_TYPE_UINT64, ""),
			mapEntry("CodeLabelsEntry", descriptorpb.FieldDescriptorProto_TYPE_UINT64, descriptorpb.FieldDescriptorProto_TYPE_STRING, ""),
			mapEntry("NamedItemsEntry", descriptorpb.FieldDescriptorProto_TYPE_STRING, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, ".omnibuf.golden.SubItem"),
		},
		Field: []*descriptorpb.FieldDescriptorProto{
			{Name: proto.String("u32"), Number: proto.Int32(1), Label: opt.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_UINT32.Enum()},
			{Name: proto.String("s64"), Number: proto.Int32(2), Label: opt.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_SINT64.Enum()},
			{Name: proto.String("flag_true"), Number: proto.Int32(3), Label: opt.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_BOOL.Enum()},
			{Name: proto.String("flag_false"), Number: proto.Int32(4), Label: opt.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_BOOL.Enum()},
			{Name: proto.String("title"), Number: proto.Int32(5), Label: opt.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
			{Name: proto.String("raw_bytes"), Number: proto.Int32(6), Label: opt.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum()},
			{Name: proto.String("f32"), Number: proto.Int32(7), Label: opt.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_FLOAT.Enum()},
			{Name: proto.String("f64"), Number: proto.Int32(8), Label: opt.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_DOUBLE.Enum()},
			{Name: proto.String("primary"), Number: proto.Int32(9), Label: opt.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".omnibuf.golden.SubItem")},
			{Name: proto.String("tags"), Number: proto.Int32(10), Label: rep.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
			{Name: proto.String("u32_arr"), Number: proto.Int32(11), Label: rep.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_UINT32.Enum()},
			{Name: proto.String("s64_arr"), Number: proto.Int32(12), Label: rep.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_SINT64.Enum()},
			{Name: proto.String("f32_arr"), Number: proto.Int32(13), Label: rep.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_FLOAT.Enum()},
			{Name: proto.String("f64_arr"), Number: proto.Int32(14), Label: rep.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_DOUBLE.Enum()},
			{Name: proto.String("items"), Number: proto.Int32(15), Label: rep.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".omnibuf.golden.SubItem")},
			{Name: proto.String("metrics"), Number: proto.Int32(16), Label: rep.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".omnibuf.golden.AllWireTypesMsg.MetricsEntry")},
			{Name: proto.String("code_labels"), Number: proto.Int32(17), Label: rep.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".omnibuf.golden.AllWireTypesMsg.CodeLabelsEntry")},
			{Name: proto.String("named_items"), Number: proto.Int32(18), Label: rep.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".omnibuf.golden.AllWireTypesMsg.NamedItemsEntry")},
		},
	}

	return &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Syntax:      proto.String("proto3"),
				Name:        proto.String("all_wire_types.proto"),
				Package:     proto.String("omnibuf.golden"),
				MessageType: []*descriptorpb.DescriptorProto{subMsg, rootMsg},
			},
		},
	}
}

func buildAllWireTypesInputJSON() []byte {
	obj := map[string]any{
		"u32":        64512,
		"s64":        -123456789,
		"flag_true":  true,
		"flag_false": false,
		"title":      "omnibuf-golden-v2",
		"raw_bytes":  base64.StdEncoding.EncodeToString([]byte{0xCA, 0xFE, 0xBA, 0xBE}),
		"f32":        2.5,
		"f64":        3.141592653589793,
		"primary": map[string]any{
			"id":     7,
			"label":  "primary-sub",
			"active": true,
		},
		"tags":    []string{"alpha", "beta", "gamma"},
		"u32_arr": []uint32{0, 127, 128, 16384},
		"s64_arr": []int64{-8192, -1, 0, 1, 8192},
		"f32_arr": []float32{-1.5, 0.0, 4.25},
		"f64_arr": []float64{-0.5, 0.25, 1024.125},
		"items": []map[string]any{
			{"id": 1, "label": "item-1", "active": true},
			{"id": 2, "label": "item-2", "active": false},
		},
		"metrics": map[string]uint64{
			"alpha_rx":  100,
			"beta_tx":   2500,
			"delta_ok":  42,
			"gamma_err": 0,
		},
		"code_labels": map[string]string{
			"10":   "ten",
			"200":  "two-hundred",
			"3000": "three-thousand",
		},
		"named_items": map[string]any{
			"first":  map[string]any{"id": 10, "label": "named-first", "active": true},
			"second": map[string]any{"id": 20, "label": "named-second", "active": false},
		},
	}
	b, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		panic(err)
	}
	return append(b, '\n')
}

func formatProtoJSONDeterministic(t *testing.T, m protoreflect.Message) []byte {
	t.Helper()
	raw, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(m.Interface())
	if err != nil {
		t.Fatalf("protojson.Marshal failed: %v", err)
	}
	var out bytes.Buffer
	if err := json.Indent(&out, raw, "", "  "); err != nil {
		t.Fatalf("json.Indent failed: %v", err)
	}
	out.WriteByte('\n')
	return out.Bytes()
}

func sanitizeBlindMapForGoldenJSON(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, elem := range val {
			out[k] = sanitizeBlindMapForGoldenJSON(elem)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(val))
		for k, elem := range val {
			out[fmt.Sprint(k)] = sanitizeBlindMapForGoldenJSON(elem)
		}
		return out
	case []map[string]any:
		out := make([]any, len(val))
		for i, elem := range val {
			out[i] = sanitizeBlindMapForGoldenJSON(elem)
		}
		return out
	case []string:
		out := make([]string, len(val))
		for i, s := range val {
			if utf8.ValidString(s) {
				out[i] = s
			} else {
				out[i] = "base64:" + base64.StdEncoding.EncodeToString([]byte(s))
			}
		}
		return out
	default:
		return v
	}
}

func assertOrUpdateGoldenFile(t *testing.T, path string, got []byte) {
	t.Helper()
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file %s: %v (run `go test . -update-golden` to generate)", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden file mismatch for %s:\ngot (%d bytes):\n%s\nwant (%d bytes):\n%s",
			path, len(got), hex.EncodeToString(got), len(want), hex.EncodeToString(want))
	}
}

func writeOrVerifyFDSFormats(t *testing.T, baseName string, fds *descriptorpb.FileDescriptorSet) {
	t.Helper()
	goldenDir := filepath.Join("testdata", "golden")

	// 1. .fds.json
	rawJSON, err := protojson.Marshal(fds)
	if err != nil {
		t.Fatalf("protojson.Marshal FDS: %v", err)
	}
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, rawJSON, "", "  "); err != nil {
		t.Fatalf("json.Indent FDS: %v", err)
	}
	prettyJSON.WriteByte('\n')
	assertOrUpdateGoldenFile(t, filepath.Join(goldenDir, baseName+".fds.json"), prettyJSON.Bytes())

	// 2. .fds.pb
	pbBytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(fds)
	if err != nil {
		t.Fatalf("proto.Marshal FDS: %v", err)
	}
	assertOrUpdateGoldenFile(t, filepath.Join(goldenDir, baseName+".fds.pb"), pbBytes)

	// 3. .fds.pb.gz (only update if -update-golden or verify decompressed bytes match pbBytes)
	gzPath := filepath.Join(goldenDir, baseName+".fds.pb.gz")
	if *updateGolden {
		var gzBuf bytes.Buffer
		zw := gzip.NewWriter(&gzBuf)
		if _, err := zw.Write(pbBytes); err != nil {
			t.Fatalf("gzip write: %v", err)
		}
		if err := zw.Close(); err != nil {
			t.Fatalf("gzip close: %v", err)
		}
		assertOrUpdateGoldenFile(t, gzPath, gzBuf.Bytes())
	} else {
		gzRaw, err := os.ReadFile(gzPath)
		if err != nil {
			t.Fatalf("read %s: %v", gzPath, err)
		}
		zr, err := gzip.NewReader(bytes.NewReader(gzRaw))
		if err != nil {
			t.Fatalf("gzip reader %s: %v", gzPath, err)
		}
		var uncomp bytes.Buffer
		if _, err := uncomp.ReadFrom(zr); err != nil {
			t.Fatalf("gzip decompress %s: %v", gzPath, err)
		}
		_ = zr.Close()
		if !bytes.Equal(uncomp.Bytes(), pbBytes) {
			t.Fatalf("golden %s decompressed bytes do not match %s.fds.pb", gzPath, baseName)
		}
	}
}

func verifyGoldenCase(t *testing.T, baseName string, md protoreflect.MessageDescriptor, inputJSON []byte) {
	t.Helper()
	goldenDir := filepath.Join("testdata", "golden")

	// 1. Input JSON golden file
	assertOrUpdateGoldenFile(t, filepath.Join(goldenDir, baseName+".input.json"), inputJSON)

	// Parse input JSON into dynamicpb.Message
	msg := dynamicpb.NewMessage(md)
	if err := protojson.Unmarshal(inputJSON, msg); err != nil {
		t.Fatalf("[%s] protojson.Unmarshal input failed: %v", baseName, err)
	}

	// 2. Deterministic Shared-Schema Wire Encoding (.shared.obuf & .shared.obuf.hex)
	wireShared, err := MarshalOptions{Deterministic: true, SelfDescribing: false}.Marshal(msg)
	if err != nil {
		t.Fatalf("[%s] Marshal(shared) failed: %v", baseName, err)
	}
	// Verify 25 consecutive runs produce byte-for-byte identical output
	for i := range 25 {
		again, err := MarshalOptions{Deterministic: true, SelfDescribing: false}.Marshal(msg)
		if err != nil || !bytes.Equal(again, wireShared) {
			t.Fatalf("[%s] non-deterministic shared encoding on iteration %d", baseName, i)
		}
	}
	assertOrUpdateGoldenFile(t, filepath.Join(goldenDir, baseName+".shared.obuf"), wireShared)
	assertOrUpdateGoldenFile(t, filepath.Join(goldenDir, baseName+".shared.obuf.hex"), []byte(hex.EncodeToString(wireShared)+"\n"))

	// 3. Deterministic Self-Describing Field #0 Wire Encoding (.field0.obuf & .field0.obuf.hex)
	wireField0, err := MarshalOptions{Deterministic: true, SelfDescribing: true}.Marshal(msg)
	if err != nil {
		t.Fatalf("[%s] Marshal(field0) failed: %v", baseName, err)
	}
	for i := range 25 {
		again, err := MarshalOptions{Deterministic: true, SelfDescribing: true}.Marshal(msg)
		if err != nil || !bytes.Equal(again, wireField0) {
			t.Fatalf("[%s] non-deterministic field0 encoding on iteration %d", baseName, i)
		}
	}
	assertOrUpdateGoldenFile(t, filepath.Join(goldenDir, baseName+".field0.obuf"), wireField0)
	assertOrUpdateGoldenFile(t, filepath.Join(goldenDir, baseName+".field0.obuf.hex"), []byte(hex.EncodeToString(wireField0)+"\n"))

	// 4. Blind Decode Golden JSON (.blind.shared.json & .blind.field0.json)
	blindShared, err := DecodeBlind(wireShared)
	if err != nil {
		t.Fatalf("[%s] DecodeBlind(shared) failed: %v", baseName, err)
	}
	blindSharedJSON, err := json.MarshalIndent(sanitizeBlindMapForGoldenJSON(blindShared), "", "  ")
	if err != nil {
		t.Fatalf("[%s] json.MarshalIndent(blindShared) failed: %v", baseName, err)
	}
	assertOrUpdateGoldenFile(t, filepath.Join(goldenDir, baseName+".blind.shared.json"), append(blindSharedJSON, '\n'))

	blindField0, err := DecodeBlind(wireField0)
	if err != nil {
		t.Fatalf("[%s] DecodeBlind(field0) failed: %v", baseName, err)
	}
	blindField0JSON, err := json.MarshalIndent(sanitizeBlindMapForGoldenJSON(blindField0), "", "  ")
	if err != nil {
		t.Fatalf("[%s] json.MarshalIndent(blindField0) failed: %v", baseName, err)
	}
	assertOrUpdateGoldenFile(t, filepath.Join(goldenDir, baseName+".blind.field0.json"), append(blindField0JSON, '\n'))

	// 5. Schema-Aware Decode Golden JSON (.schema.json)
	decodedShared := dynamicpb.NewMessage(md)
	if err := Unmarshal(wireShared, decodedShared); err != nil {
		t.Fatalf("[%s] Unmarshal(shared) failed: %v", baseName, err)
	}
	decodedField0 := dynamicpb.NewMessage(md)
	if err := Unmarshal(wireField0, decodedField0); err != nil {
		t.Fatalf("[%s] Unmarshal(field0) failed: %v", baseName, err)
	}
	if !proto.Equal(decodedShared, decodedField0) {
		t.Fatalf("[%s] decodedShared != decodedField0", baseName)
	}
	schemaJSON := formatProtoJSONDeterministic(t, decodedShared)
	assertOrUpdateGoldenFile(t, filepath.Join(goldenDir, baseName+".schema.json"), schemaJSON)
}

func TestGoldenWireFiles(t *testing.T) {
	// Case 1: Canonical PeerUpdate schema (proto/omnibuf/bench/peer.proto)
	rawPeer := SamplePeerUpdate()
	peerSample := SamplePBPeerUpdate(&rawPeer)
	peerFD := peerSample.ProtoReflect().Descriptor().ParentFile()
	peerFDS := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{protodesc.ToFileDescriptorProto(peerFD)},
	}
	writeOrVerifyFDSFormats(t, "peer", peerFDS)
	peerInputJSON := formatProtoJSONDeterministic(t, peerSample.ProtoReflect())
	verifyGoldenCase(t, "peer_update", peerSample.ProtoReflect().Descriptor(), peerInputJSON)

	// Case 2: AllWireTypesMsg schema (all 16 wire types 0..15 + maps)
	allFDS := buildAllWireTypesFDS()
	writeOrVerifyFDSFormats(t, "all_wire_types", allFDS)
	files, err := protodesc.NewFiles(allFDS)
	if err != nil {
		t.Fatalf("protodesc.NewFiles(allFDS) failed: %v", err)
	}
	desc, err := files.FindDescriptorByName("omnibuf.golden.AllWireTypesMsg")
	if err != nil {
		t.Fatalf("FindDescriptorByName failed: %v", err)
	}
	allMD := desc.(protoreflect.MessageDescriptor)
	verifyGoldenCase(t, "all_wire_types", allMD, buildAllWireTypesInputJSON())
}
