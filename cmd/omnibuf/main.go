package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/sudorandom/omnibuf"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "omnibuf: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return errors.New("missing command (expected 'decode' or 'encode')")
	}

	switch args[0] {
	case "decode", "dec", "d":
		return runDecode(args[1:], stdin, stdout, stderr)
	case "encode", "enc", "e":
		return runEncode(args[1:], stdin, stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return nil
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q (expected 'decode' or 'encode')", args[0])
	}
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  omnibuf <command> [flags] [input-file]

Commands:
  decode    Decode OmniBuf wire bytes to JSON (blind or schema-aware)
  encode    Encode JSON to OmniBuf wire bytes using a FileDescriptorSet schema

Decode Examples:
  # Blind decode without a schema (from file or stdin)
  omnibuf decode packet.obuf
  omnibuf decode --hex <<< "2104e00700"

  # Schema-aware decode using a FileDescriptorSet (.json, .pb, or .pb.gz)
  omnibuf decode -s schema.pb.gz -t omnibuf.bench.PeerUpdate packet.obuf

Encode Examples:
  # Encode JSON to OmniBuf binary using a FileDescriptorSet (.json, .pb, or .pb.gz)
  omnibuf encode -s schema.pb.gz -t omnibuf.bench.PeerUpdate input.json -o packet.obuf

  # Encode with Field #0 self-describing inline field names and output hex
  omnibuf encode -s schema.json -t PeerUpdate --field0 --hex input.json
`)
}

func runDecode(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("decode", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		schemaPath string
		msgType    string
		inputPath  string
		outputPath string
		isHex      bool
		compact    bool
	)
	fs.StringVar(&schemaPath, "schema", "", "Optional FileDescriptorSet path (.json, .pb, .pb.gz); omit for blind decode")
	fs.StringVar(&schemaPath, "s", "", "Short alias for --schema")
	fs.StringVar(&msgType, "type", "", "Message type name when --schema is used (auto-selected if schema has 1 message)")
	fs.StringVar(&msgType, "t", "", "Short alias for --type")
	fs.StringVar(&inputPath, "input", "", "Input file path (defaults to positional arg or stdin)")
	fs.StringVar(&inputPath, "i", "", "Short alias for --input")
	fs.StringVar(&outputPath, "output", "", "Output file path (defaults to stdout)")
	fs.StringVar(&outputPath, "o", "", "Short alias for --output")
	fs.BoolVar(&isHex, "hex", false, "Read hex-encoded OmniBuf wire bytes")
	fs.BoolVar(&compact, "compact", false, "Emit compact single-line JSON")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if inputPath == "" && fs.NArg() > 0 {
		inputPath = fs.Arg(0)
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("unexpected extra arguments: %v", fs.Args()[1:])
	}

	rawInput, err := readInput(inputPath, stdin)
	if err != nil {
		return err
	}

	wireBytes := rawInput
	if isHex {
		wireBytes, err = decodeHexBytes(rawInput)
		if err != nil {
			return fmt.Errorf("invalid hex input: %w", err)
		}
	}

	var jsonOut []byte
	if schemaPath == "" {
		if msgType != "" {
			return errors.New("--type requires --schema (-s)")
		}
		blindMap, err := omnibuf.DecodeBlind(wireBytes)
		if err != nil {
			return err
		}
		sanitized := sanitizeForJSON(blindMap)
		if compact {
			jsonOut, err = json.Marshal(sanitized)
		} else {
			jsonOut, err = json.MarshalIndent(sanitized, "", "  ")
		}
		if err != nil {
			return fmt.Errorf("failed to marshal blind-decoded JSON: %w", err)
		}
	} else {
		md, err := loadMessageDescriptor(schemaPath, msgType)
		if err != nil {
			return err
		}
		msg := dynamicpb.NewMessage(md)
		if err := omnibuf.Unmarshal(wireBytes, msg); err != nil {
			return err
		}
		mOpts := protojson.MarshalOptions{
			UseProtoNames: true,
		}
		rawJSON, err := mOpts.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to marshal protobuf JSON: %w", err)
		}
		var normBuf bytes.Buffer
		if compact {
			if err := json.Compact(&normBuf, rawJSON); err != nil {
				return fmt.Errorf("failed to compact JSON: %w", err)
			}
		} else {
			if err := json.Indent(&normBuf, rawJSON, "", "  "); err != nil {
				return fmt.Errorf("failed to indent JSON: %w", err)
			}
		}
		jsonOut = normBuf.Bytes()
	}

	jsonOut = append(jsonOut, '\n')
	return writeOutput(outputPath, stdout, jsonOut)
}

func runEncode(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("encode", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		schemaPath    string
		msgType       string
		inputPath     string
		outputPath    string
		field0        bool
		deterministic bool
		isHex         bool
	)
	fs.StringVar(&schemaPath, "schema", "", "FileDescriptorSet path (.json, .pb, .pb.gz)")
	fs.StringVar(&schemaPath, "s", "", "Short alias for --schema")
	fs.StringVar(&msgType, "type", "", "Message type name (auto-selected if schema has 1 message)")
	fs.StringVar(&msgType, "t", "", "Short alias for --type")
	fs.StringVar(&inputPath, "input", "", "Input JSON file path (defaults to positional arg or stdin)")
	fs.StringVar(&inputPath, "i", "", "Short alias for --input")
	fs.StringVar(&outputPath, "output", "", "Output file path (defaults to stdout)")
	fs.StringVar(&outputPath, "o", "", "Short alias for --output")
	fs.BoolVar(&field0, "field0", false, "Include Field #0 inline field names for self-describing mode")
	fs.BoolVar(&field0, "self-describing", false, "Alias for --field0")
	fs.BoolVar(&deterministic, "deterministic", true, "Sort WireMap keys deterministically for reproducible wire output")
	fs.BoolVar(&isHex, "hex", false, "Write hex-encoded OmniBuf wire bytes")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if schemaPath == "" {
		return errors.New("encode requires --schema (-s) pointing to a FileDescriptorSet (.json, .pb, or .pb.gz)")
	}
	if inputPath == "" && fs.NArg() > 0 {
		inputPath = fs.Arg(0)
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("unexpected extra arguments: %v", fs.Args()[1:])
	}

	md, err := loadMessageDescriptor(schemaPath, msgType)
	if err != nil {
		return err
	}

	jsonBytes, err := readInput(inputPath, stdin)
	if err != nil {
		return err
	}

	msg := dynamicpb.NewMessage(md)
	uOpts := protojson.UnmarshalOptions{
		DiscardUnknown: false,
	}
	if err := uOpts.Unmarshal(jsonBytes, msg); err != nil {
		return fmt.Errorf("failed to parse input JSON for %s: %w", md.FullName(), err)
	}

	wireBytes, err := omnibuf.MarshalOptions{
		Deterministic:  deterministic,
		SelfDescribing: field0,
	}.Marshal(msg)
	if err != nil {
		return err
	}

	if isHex {
		hexOut := []byte(hex.EncodeToString(wireBytes) + "\n")
		return writeOutput(outputPath, stdout, hexOut)
	}
	return writeOutput(outputPath, stdout, wireBytes)
}

// loadFileDescriptorSet loads a *descriptorpb.FileDescriptorSet from a .json, .pb, or .pb.gz file.
// Format is detected from the filename extension (.pb.gz/.gz, .json, .pb) as well as content inspection
// (gzip magic header 0x1f 0x8b or leading '{' for JSON).
func loadFileDescriptorSet(path string) (*descriptorpb.FileDescriptorSet, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read schema %q: %w", path, err)
	}

	lower := strings.ToLower(path)
	isGzip := strings.HasSuffix(lower, ".gz") || (len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b)
	if isGzip {
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("open gzip schema %q: %w", path, err)
		}
		decompressed, err := io.ReadAll(zr)
		_ = zr.Close()
		if err != nil {
			return nil, fmt.Errorf("decompress gzip schema %q: %w", path, err)
		}
		raw = decompressed
		lower = strings.TrimSuffix(lower, ".gz")
	}

	trimmed := bytes.TrimSpace(raw)
	isJSON := strings.HasSuffix(lower, ".json") || (len(trimmed) > 0 && trimmed[0] == '{')

	fds := &descriptorpb.FileDescriptorSet{}
	if isJSON {
		if err := protojson.Unmarshal(trimmed, fds); err != nil {
			// Also support a single FileDescriptorProto JSON object for convenience.
			fdp := &descriptorpb.FileDescriptorProto{}
			if errSingle := protojson.Unmarshal(trimmed, fdp); errSingle == nil && fdp.GetName() != "" {
				fds.File = []*descriptorpb.FileDescriptorProto{fdp}
				return fds, nil
			}
			return nil, fmt.Errorf("unmarshal FileDescriptorSet JSON from %q: %w", path, err)
		}
		if len(fds.File) == 0 {
			return nil, fmt.Errorf("FileDescriptorSet JSON %q contains 0 files", path)
		}
		return fds, nil
	}

	if err := proto.Unmarshal(raw, fds); err != nil {
		return nil, fmt.Errorf("unmarshal FileDescriptorSet binary from %q: %w", path, err)
	}
	if len(fds.File) == 0 {
		return nil, fmt.Errorf("FileDescriptorSet %q contains 0 files", path)
	}
	return fds, nil
}

// loadMessageDescriptor loads the FileDescriptorSet from schemaPath and resolves msgType.
// If msgType is empty and the FileDescriptorSet contains exactly one top-level message, that message is returned.
func loadMessageDescriptor(schemaPath, msgType string) (protoreflect.MessageDescriptor, error) {
	fds, err := loadFileDescriptorSet(schemaPath)
	if err != nil {
		return nil, err
	}

	files, err := protodesc.NewFiles(fds)
	if err != nil {
		return nil, fmt.Errorf("build descriptor registry from %q: %w", schemaPath, err)
	}

	cleanType := strings.TrimPrefix(strings.TrimSpace(msgType), ".")
	if cleanType != "" {
		if desc, err := files.FindDescriptorByName(protoreflect.FullName(cleanType)); err == nil {
			if md, ok := desc.(protoreflect.MessageDescriptor); ok {
				return md, nil
			}
			return nil, fmt.Errorf("%q is not a message descriptor", cleanType)
		}
	}

	// Collect all message descriptors across the FileDescriptorSet for short-name matching or single-message fallback.
	var allMessages []protoreflect.MessageDescriptor
	var topLevelMessages []protoreflect.MessageDescriptor
	var collectNested func(mds protoreflect.MessageDescriptors, top bool)
	collectNested = func(mds protoreflect.MessageDescriptors, top bool) {
		for i := 0; i < mds.Len(); i++ {
			md := mds.Get(i)
			if md.IsMapEntry() {
				continue
			}
			allMessages = append(allMessages, md)
			if top {
				topLevelMessages = append(topLevelMessages, md)
			}
			collectNested(md.Messages(), false)
		}
	}
	files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		collectNested(fd.Messages(), true)
		return true
	})

	if cleanType == "" {
		if len(topLevelMessages) == 1 {
			return topLevelMessages[0], nil
		}
		if len(allMessages) == 1 {
			return allMessages[0], nil
		}
		names := make([]string, 0, len(allMessages))
		for _, md := range allMessages {
			names = append(names, string(md.FullName()))
		}
		return nil, fmt.Errorf("please specify --type (-t); available messages in %q: %s", schemaPath, strings.Join(names, ", "))
	}

	var matches []protoreflect.MessageDescriptor
	suffix := "." + cleanType
	for _, md := range allMessages {
		full := string(md.FullName())
		if string(md.Name()) == cleanType || strings.HasSuffix(full, suffix) {
			matches = append(matches, md)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		names := make([]string, 0, len(matches))
		for _, md := range matches {
			names = append(names, string(md.FullName()))
		}
		return nil, fmt.Errorf("ambiguous message type %q matches multiple messages: %s", msgType, strings.Join(names, ", "))
	}

	names := make([]string, 0, len(allMessages))
	for _, md := range allMessages {
		names = append(names, string(md.FullName()))
	}
	return nil, fmt.Errorf("message type %q not found in %q (available: %s)", msgType, schemaPath, strings.Join(names, ", "))
}

func readInput(path string, stdin io.Reader) ([]byte, error) {
	if path == "" || path == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return data, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", path, err)
	}
	return data, nil
}

func writeOutput(path string, stdout io.Writer, data []byte) error {
	if path == "" || path == "-" {
		_, err := stdout.Write(data)
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}

func decodeHexBytes(raw []byte) ([]byte, error) {
	var clean []byte
	for _, b := range raw {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			clean = append(clean, b)
		}
	}
	if len(clean) >= 2 && clean[0] == '0' && (clean[1] == 'x' || clean[1] == 'X') {
		clean = clean[2:]
	}
	out := make([]byte, hex.DecodedLen(len(clean)))
	n, err := hex.Decode(out, clean)
	if err != nil {
		return nil, err
	}
	return out[:n], nil
}

// sanitizeForJSON recursively converts map[any]any (from non-string WireMap keys) into map[string]any
// and ensures non-UTF-8 strings in WireStringArr (from repeated bytes) are base64-encoded so
// encoding/json.Marshal produces valid UTF-8 JSON for any blind-decoded packet.
func sanitizeForJSON(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, elem := range val {
			out[k] = sanitizeForJSON(elem)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(val))
		for k, elem := range val {
			out[fmt.Sprint(k)] = sanitizeForJSON(elem)
		}
		return out
	case []map[string]any:
		out := make([]any, len(val))
		for i, elem := range val {
			out[i] = sanitizeForJSON(elem)
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
	case string:
		if utf8.ValidString(val) {
			return val
		}
		return "base64:" + base64.StdEncoding.EncodeToString([]byte(val))
	default:
		return v
	}
}
