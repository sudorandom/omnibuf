package main

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sudorandom/omnibuf"
	"github.com/sudorandom/omnibuf/pb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
)

func createSchemaFiles(t *testing.T) (jsonPath, pbPath, pbGzPath string) {
	t.Helper()
	dir := t.TempDir()

	msg := (&pb.PeerUpdate{}).ProtoReflect()
	fd := msg.Descriptor().ParentFile()
	fdp := protodesc.ToFileDescriptorProto(fd)
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{fdp},
	}

	// 1. .json format
	jsonBytes, err := protojson.MarshalOptions{Indent: "  "}.Marshal(fds)
	if err != nil {
		t.Fatalf("protojson.Marshal(fds) failed: %v", err)
	}
	jsonPath = filepath.Join(dir, "schema.json")
	if err := os.WriteFile(jsonPath, jsonBytes, 0o600); err != nil {
		t.Fatalf("write schema.json: %v", err)
	}

	// 2. .pb format
	pbBytes, err := proto.Marshal(fds)
	if err != nil {
		t.Fatalf("proto.Marshal(fds) failed: %v", err)
	}
	pbPath = filepath.Join(dir, "schema.pb")
	if err := os.WriteFile(pbPath, pbBytes, 0o600); err != nil {
		t.Fatalf("write schema.pb: %v", err)
	}

	// 3. .pb.gz format
	var gzBuf bytes.Buffer
	zw := gzip.NewWriter(&gzBuf)
	if _, err := zw.Write(pbBytes); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	pbGzPath = filepath.Join(dir, "schema.pb.gz")
	if err := os.WriteFile(pbGzPath, gzBuf.Bytes(), 0o600); err != nil {
		t.Fatalf("write schema.pb.gz: %v", err)
	}

	return jsonPath, pbPath, pbGzPath
}

func TestCLIEncodeAndDecode_AllSchemaFormats(t *testing.T) {
	jsonSchema, pbSchema, pbGzSchema := createSchemaFiles(t)
	schemas := []struct {
		name string
		path string
	}{
		{name: "json", path: jsonSchema},
		{name: "pb", path: pbSchema},
		{name: "pb.gz", path: pbGzSchema},
	}

	sample := &pb.PeerUpdate{
		Asn:       64512,
		RouterId:  "edge-rtr-fra-01",
		Active:    true,
		UptimeSec: 864000,
		RssiDbm:   -62,
		CpuLoad:   0.425,
		Tags:      []string{"core", "bgp", "ixp"},
		Prefixes:  []uint32{10, 200, 3000},
		Location:  &pb.Location{Region: "eu-central", Datacenter: "fra1", City: "Frankfurt", Rack: 42, Latitude: 50.1109, Longitude: 8.6821},
		Neighbors: []*pb.Neighbor{{PeerAsn: 65001, Router: "peer-ams-01", LatencyUs: 4200, Established: true}},
		Sequence:  987654321,
	}
	inputJSONBytes, err := protojson.MarshalOptions{UseProtoNames: true, Indent: "  "}.Marshal(sample)
	if err != nil {
		t.Fatalf("protojson.Marshal(sample) failed: %v", err)
	}

	for _, sc := range schemas {
		for _, field0 := range []bool{false, true} {
			mode := "shared"
			if field0 {
				mode = "field0"
			}
			t.Run(sc.name+"_"+mode, func(t *testing.T) {
				dir := t.TempDir()
				obufFile := filepath.Join(dir, "peer.obuf")

				encArgs := []string{"encode", "-s", sc.path, "-t", "omnibuf.bench.PeerUpdate", "-o", obufFile}
				if field0 {
					encArgs = append(encArgs, "--field0")
				}
				var encOut, encErr bytes.Buffer
				if err := run(encArgs, bytes.NewReader(inputJSONBytes), &encOut, &encErr); err != nil {
					t.Fatalf("encode (%s) failed: %v (stderr: %s)", sc.name, err, encErr.String())
				}

				// 1. Schema-aware decode using short type name ("PeerUpdate")
				var decOut, decErr bytes.Buffer
				decArgs := []string{"decode", "-s", sc.path, "-t", "PeerUpdate", obufFile}
				if err := run(decArgs, strings.NewReader(""), &decOut, &decErr); err != nil {
					t.Fatalf("decode (%s) failed: %v (stderr: %s)", sc.name, err, decErr.String())
				}

				var roundTrip pb.PeerUpdate
				if err := protojson.Unmarshal(decOut.Bytes(), &roundTrip); err != nil {
					t.Fatalf("protojson.Unmarshal round-trip failed: %v\nJSON:\n%s", err, decOut.String())
				}
				if !proto.Equal(sample, &roundTrip) {
					t.Fatalf("round-trip mismatch for %s (%s)", sc.name, mode)
				}

				// 2. Blind decode (no schema)
				var blindOut, blindErr bytes.Buffer
				if err := run([]string{"decode", obufFile}, strings.NewReader(""), &blindOut, &blindErr); err != nil {
					t.Fatalf("blind decode failed: %v (stderr: %s)", err, blindErr.String())
				}
				var blindMap map[string]any
				if err := json.Unmarshal(blindOut.Bytes(), &blindMap); err != nil {
					t.Fatalf("blind JSON unmarshal failed: %v\nOutput:\n%s", err, blindOut.String())
				}
				if field0 {
					if blindMap["router_id"] != sample.RouterId {
						t.Fatalf("blind decode router_id = %v, want %q", blindMap["router_id"], sample.RouterId)
					}
				} else {
					if blindMap["field_2"] != sample.RouterId {
						t.Fatalf("blind decode field_2 = %v, want %q", blindMap["field_2"], sample.RouterId)
					}
				}
			})
		}
	}
}

func TestCLIHexAndBlindDecodeNonStringMapKeys(t *testing.T) {
	jsonSchema, _, _ := createSchemaFiles(t)

	// Encode with --hex to stdout, then blind decode with --hex from stdin
	inputJSON := `{"asn": 64512, "router_id": "edge-fra-01", "active": true, "rssi_dbm": -62}`
	var hexOut, stderr bytes.Buffer
	if err := run([]string{"encode", "-s", jsonSchema, "-t", "PeerUpdate", "--field0", "--hex"}, strings.NewReader(inputJSON), &hexOut, &stderr); err != nil {
		t.Fatalf("encode --hex failed: %v", err)
	}

	var blindOut bytes.Buffer
	if err := run([]string{"decode", "--hex", "--compact"}, bytes.NewReader(hexOut.Bytes()), &blindOut, &stderr); err != nil {
		t.Fatalf("decode --hex --compact failed: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(blindOut.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if decoded["router_id"] != "edge-fra-01" || decoded["active"] != true || decoded["rssi_dbm"] != float64(-62) {
		t.Fatalf("unexpected blind decoded map: %v", decoded)
	}

	// Test blind decode of a WireMap with uint64 keys (which produces map[any]any in DecodeBlind)
	var wire []byte
	wire = omnibuf.AppendVarint(wire, (1<<4)|uint64(omnibuf.WireMap))
	lenPos := len(wire)
	wire = append(wire, 0, 0)
	wire = append(wire, omnibuf.MapHeaderByte(omnibuf.WireUint, omnibuf.WireString))
	wire = omnibuf.AppendVarint(wire, 1)
	wire = omnibuf.AppendVarint(wire, 42)
	wire = omnibuf.AppendVarint(wire, 2)
	wire = append(wire, "ok"...)
	wire = omnibuf.BackpatchContainerLen(wire, lenPos)

	wireHex := hex.EncodeToString(wire)
	blindOut.Reset()
	if err := run([]string{"decode", "--hex"}, strings.NewReader(wireHex), &blindOut, &stderr); err != nil {
		t.Fatalf("blind decode of uint64-keyed WireMap failed: %v", err)
	}
	var mapDecoded map[string]any
	if err := json.Unmarshal(blindOut.Bytes(), &mapDecoded); err != nil {
		t.Fatalf("json.Unmarshal uint64 map failed: %v", err)
	}
	f1, _ := mapDecoded["field_1"].(map[string]any)
	if f1["42"] != "ok" {
		t.Fatalf("expected field_1[\"42\"] == \"ok\", got %v", mapDecoded)
	}
}

func TestCLIErrorCases(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(nil, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Fatal("expected error for missing command")
	}
	if err := run([]string{"unknown"}, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Fatal("expected error for unknown command")
	}
	if err := run([]string{"encode"}, strings.NewReader("{}"), &stdout, &stderr); err == nil {
		t.Fatal("expected error for encode without --schema")
	}
	if err := run([]string{"decode", "-t", "PeerUpdate"}, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Fatal("expected error when --type is passed without --schema")
	}
	if err := run([]string{"--help"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("expected --help to succeed, got %v", err)
	}
}

func TestCLIAgainstCheckedInGoldenFiles(t *testing.T) {
	goldenDir := filepath.Join("..", "..", "testdata", "golden")
	cases := []struct {
		baseName   string
		schemaBase string
		msgType    string
	}{
		{baseName: "peer_update", schemaBase: "peer", msgType: "omnibuf.bench.PeerUpdate"},
		{baseName: "all_wire_types", schemaBase: "all_wire_types", msgType: "omnibuf.golden.AllWireTypesMsg"},
	}

	for _, tc := range cases {
		for _, ext := range []string{".fds.json", ".fds.pb", ".fds.pb.gz"} {
			t.Run(tc.baseName+ext, func(t *testing.T) {
				schemaFile := filepath.Join(goldenDir, tc.schemaBase+ext)
				inputJSONFile := filepath.Join(goldenDir, tc.baseName+".input.json")
				sharedObufFile := filepath.Join(goldenDir, tc.baseName+".shared.obuf")
				field0ObufFile := filepath.Join(goldenDir, tc.baseName+".field0.obuf")
				field0HexFile := filepath.Join(goldenDir, tc.baseName+".field0.obuf.hex")

				wantSharedObuf, err := os.ReadFile(sharedObufFile)
				if err != nil {
					t.Fatalf("read %s: %v", sharedObufFile, err)
				}
				wantField0Obuf, err := os.ReadFile(field0ObufFile)
				if err != nil {
					t.Fatalf("read %s: %v", field0ObufFile, err)
				}
				wantField0Hex, err := os.ReadFile(field0HexFile)
				if err != nil {
					t.Fatalf("read %s: %v", field0HexFile, err)
				}
				wantBlindSharedJSON, err := os.ReadFile(filepath.Join(goldenDir, tc.baseName+".blind.shared.json"))
				if err != nil {
					t.Fatalf("read blind.shared.json: %v", err)
				}
				wantBlindField0JSON, err := os.ReadFile(filepath.Join(goldenDir, tc.baseName+".blind.field0.json"))
				if err != nil {
					t.Fatalf("read blind.field0.json: %v", err)
				}
				wantSchemaJSON, err := os.ReadFile(filepath.Join(goldenDir, tc.baseName+".schema.json"))
				if err != nil {
					t.Fatalf("read schema.json: %v", err)
				}

				// 1. CLI encode (shared, --deterministic) -> must match golden .shared.obuf byte-for-byte
				var stdout, stderr bytes.Buffer
				if err := run([]string{"encode", "--deterministic", "-s", schemaFile, "-t", tc.msgType, inputJSONFile}, strings.NewReader(""), &stdout, &stderr); err != nil {
					t.Fatalf("encode shared failed: %v (%s)", err, stderr.String())
				}
				if !bytes.Equal(stdout.Bytes(), wantSharedObuf) {
					t.Fatalf("CLI encode shared output differs from golden %s", sharedObufFile)
				}

				// 2. CLI encode (--field0 --deterministic) -> must match golden .field0.obuf byte-for-byte
				stdout.Reset()
				if err := run([]string{"encode", "--deterministic", "-s", schemaFile, "-t", tc.msgType, "--field0", inputJSONFile}, strings.NewReader(""), &stdout, &stderr); err != nil {
					t.Fatalf("encode field0 failed: %v (%s)", err, stderr.String())
				}
				if !bytes.Equal(stdout.Bytes(), wantField0Obuf) {
					t.Fatalf("CLI encode --field0 output differs from golden %s", field0ObufFile)
				}

				// 3. CLI encode (--field0 --deterministic --hex) -> must match golden .field0.obuf.hex byte-for-byte
				stdout.Reset()
				if err := run([]string{"encode", "--deterministic", "-s", schemaFile, "-t", tc.msgType, "--field0", "--hex", inputJSONFile}, strings.NewReader(""), &stdout, &stderr); err != nil {
					t.Fatalf("encode field0 hex failed: %v (%s)", err, stderr.String())
				}
				if !bytes.Equal(stdout.Bytes(), wantField0Hex) {
					t.Fatalf("CLI encode --field0 --hex output differs from golden %s", field0HexFile)
				}

				// 4. CLI blind decode (shared) -> must match golden .blind.shared.json
				stdout.Reset()
				if err := run([]string{"decode", sharedObufFile}, strings.NewReader(""), &stdout, &stderr); err != nil {
					t.Fatalf("blind decode shared failed: %v (%s)", err, stderr.String())
				}
				if !bytes.Equal(stdout.Bytes(), wantBlindSharedJSON) {
					t.Fatalf("CLI blind decode shared differs from golden .blind.shared.json")
				}

				// 5. CLI blind decode (field0) -> must match golden .blind.field0.json
				stdout.Reset()
				if err := run([]string{"decode", field0ObufFile}, strings.NewReader(""), &stdout, &stderr); err != nil {
					t.Fatalf("blind decode field0 failed: %v (%s)", err, stderr.String())
				}
				if !bytes.Equal(stdout.Bytes(), wantBlindField0JSON) {
					t.Fatalf("CLI blind decode field0 differs from golden .blind.field0.json")
				}

				// 6. CLI schema-aware decode -> must match golden .schema.json
				stdout.Reset()
				if err := run([]string{"decode", "-s", schemaFile, "-t", tc.msgType, sharedObufFile}, strings.NewReader(""), &stdout, &stderr); err != nil {
					t.Fatalf("schema decode failed: %v (%s)", err, stderr.String())
				}
				if !bytes.Equal(stdout.Bytes(), wantSchemaJSON) {
					t.Fatalf("CLI schema decode differs from golden .schema.json")
				}
			})
		}
	}
}
