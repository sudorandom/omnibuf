// Package conformance implements the language-independent OmniBuf conformance
// test protocol (modeled after Protocol Buffers' conformance_test_runner)
// where a runner binary communicates with a language-specific adapter binary
// over length-prefixed stdin/stdout frames.
package conformance

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/sudorandom/omnibuf"
)

// WireFormat identifies the serialization format of payloads in ConformanceRequest and ConformanceResponse.
type WireFormat uint32

const (
	WireFormatUnspecified WireFormat = 0
	WireFormatOmniBuf     WireFormat = 1
	WireFormatJSON        WireFormat = 2
)

// TestTier selects which required implementation tier the adapter binary must exercise.
type TestTier uint32

const (
	TestTierUnspecified     TestTier = 0
	TestTierPlugin          TestTier = 1 // Generated code from protoc-gen-omnibuf-* plugin
	TestTierProtobufTypes   TestTier = 2 // Standard protoc-generated Protobuf types without plugin
	TestTierAnnotatedStruct TestTier = 3 // Plain language structs/classes/objects with annotations or mapping
	TestTierBlind           TestTier = 4 // Schema-less inspection (DecodeBlind / DecodeBlindJSON)
)

func (t TestTier) String() string {
	switch t {
	case TestTierPlugin:
		return "TIER_PLUGIN"
	case TestTierProtobufTypes:
		return "TIER_PROTOBUF_TYPES"
	case TestTierAnnotatedStruct:
		return "TIER_ANNOTATED_STRUCT"
	case TestTierBlind:
		return "TIER_BLIND"
	default:
		return "TIER_UNSPECIFIED"
	}
}

// ConformanceRequest is sent from conformance-runner to the adapter binary over stdin
// prefixed by a 4-byte little-endian uint32 byte length.
type ConformanceRequest struct {
	TestName              string `omnibuf:"1,name=test_name" protobuf:"bytes,1,opt,name=test_name,proto3" json:"test_name"`
	MessageType           string `omnibuf:"2,name=message_type" protobuf:"bytes,2,opt,name=message_type,proto3" json:"message_type"`
	Tier                  uint32 `omnibuf:"3,name=tier" protobuf:"varint,3,opt,name=tier,proto3" json:"tier"`
	RequestedOutputFormat uint32 `omnibuf:"4,name=requested_output_format" protobuf:"varint,4,opt,name=requested_output_format,proto3" json:"requested_output_format"`
	OmniBufPayload        []byte `omnibuf:"5,name=omnibuf_payload" protobuf:"bytes,5,opt,name=omnibuf_payload,proto3" json:"omnibuf_payload"`
	JSONPayload           string `omnibuf:"6,name=json_payload" protobuf:"bytes,6,opt,name=json_payload,proto3" json:"json_payload"`
	SelfDescribing        bool   `omnibuf:"7,name=self_describing" protobuf:"varint,7,opt,name=self_describing,proto3" json:"self_describing"`
	TestOpaqueSplice      bool   `omnibuf:"8,name=test_opaque_splice" protobuf:"varint,8,opt,name=test_opaque_splice,proto3" json:"test_opaque_splice"`
}

// ConformanceResponse is returned by the adapter binary over stdout
// prefixed by a 4-byte little-endian uint32 byte length.
type ConformanceResponse struct {
	ParseError     string `omnibuf:"1,name=parse_error" protobuf:"bytes,1,opt,name=parse_error,proto3" json:"parse_error"`
	SerializeError string `omnibuf:"2,name=serialize_error" protobuf:"bytes,2,opt,name=serialize_error,proto3" json:"serialize_error"`
	RuntimeError   string `omnibuf:"3,name=runtime_error" protobuf:"bytes,3,opt,name=runtime_error,proto3" json:"runtime_error"`
	Skipped        string `omnibuf:"4,name=skipped" protobuf:"bytes,4,opt,name=skipped,proto3" json:"skipped"`
	OmniBufPayload []byte `omnibuf:"5,name=omnibuf_payload" protobuf:"bytes,5,opt,name=omnibuf_payload,proto3" json:"omnibuf_payload"`
	JSONPayload    string `omnibuf:"6,name=json_payload" protobuf:"bytes,6,opt,name=json_payload,proto3" json:"json_payload"`
}

// MaxFrameBytes caps a single conformance IPC frame at 16 MiB.
const MaxFrameBytes = 16 * 1024 * 1024

// WriteFrame encodes msg as an OmniBuf packet and writes it to w with a 4-byte
// little-endian uint32 length prefix (matching Protobuf's conformance framing).
func WriteFrame(w io.Writer, msg any) error {
	payload, err := omnibuf.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal frame: %w", err)
	}
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(payload)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

// ReadFrame reads a 4-byte little-endian uint32 length prefix and decodes the
// OmniBuf payload into dst. Returns io.EOF if the stream ends cleanly at a frame boundary.
func ReadFrame(r io.Reader, dst any) error {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return err
	}
	size := binary.LittleEndian.Uint32(lenBuf[:])
	if size > MaxFrameBytes {
		return fmt.Errorf("conformance frame size %d exceeds limit %d", size, MaxFrameBytes)
	}
	payload := make([]byte, size)
	if size > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			if errors.Is(err, io.EOF) {
				return io.ErrUnexpectedEOF
			}
			return err
		}
	}
	return omnibuf.Unmarshal(payload, dst)
}

// ============================================================================
// Canonical Conformance Test Message Types (Annotated Structs)
// ============================================================================

type AllTypesSubMsg struct {
	ID    uint32 `omnibuf:"1,name=id" protobuf:"varint,1,opt,name=id,proto3" json:"id"`
	Label string `omnibuf:"2,name=label" protobuf:"bytes,2,opt,name=label,proto3" json:"label"`
	Flag  bool   `omnibuf:"3,name=flag" protobuf:"varint,3,opt,name=flag,proto3" json:"flag"`
	Score int64  `omnibuf:"4,name=score,wire=sint" protobuf:"zigzag64,4,opt,name=score,proto3" json:"score"`
}

type TestAllTypes struct {
	F1Uint32       uint32            `omnibuf:"1,name=f1_uint32" protobuf:"varint,1,opt,name=f1_uint32,proto3" json:"f1_uint32"`
	F2Uint64       uint64            `omnibuf:"2,name=f2_uint64" protobuf:"varint,2,opt,name=f2_uint64,proto3" json:"f2_uint64"`
	F3Sint32       int32             `omnibuf:"3,name=f3_sint32,wire=sint" protobuf:"zigzag32,3,opt,name=f3_sint32,proto3" json:"f3_sint32"`
	F4Sint64       int64             `omnibuf:"4,name=f4_sint64,wire=sint" protobuf:"zigzag64,4,opt,name=f4_sint64,proto3" json:"f4_sint64"`
	F5Bool         bool              `omnibuf:"5,name=f5_bool" protobuf:"varint,5,opt,name=f5_bool,proto3" json:"f5_bool"`
	F6String       string            `omnibuf:"6,name=f6_string" protobuf:"bytes,6,opt,name=f6_string,proto3" json:"f6_string"`
	F7Bytes        []byte            `omnibuf:"7,name=f7_bytes" protobuf:"bytes,7,opt,name=f7_bytes,proto3" json:"f7_bytes"`
	F8SubMsg       *AllTypesSubMsg   `omnibuf:"8,name=f8_sub_msg" protobuf:"bytes,8,opt,name=f8_sub_msg,proto3" json:"f8_sub_msg"`
	F9StringArr    []string          `omnibuf:"9,name=f9_string_arr" protobuf:"bytes,9,rep,name=f9_string_arr,proto3" json:"f9_string_arr"`
	F10Uint32Arr   []uint32          `omnibuf:"10,name=f10_uint32_arr" protobuf:"varint,10,rep,packed,name=f10_uint32_arr,proto3" json:"f10_uint32_arr"`
	F11SubMsgArr   []AllTypesSubMsg  `omnibuf:"11,name=f11_sub_msg_arr" protobuf:"bytes,11,rep,name=f11_sub_msg_arr,proto3" json:"f11_sub_msg_arr"`
	F12MapStrUint  map[string]uint64 `omnibuf:"12,name=f12_map_str_uint" protobuf:"bytes,12,rep,name=f12_map_str_uint,proto3" json:"f12_map_str_uint"`
	F13Float32     float32           `omnibuf:"13,name=f13_float32" protobuf:"fixed32,13,opt,name=f13_float32,proto3" json:"f13_float32"`
	F14Float64     float64           `omnibuf:"14,name=f14_float64" protobuf:"fixed64,14,opt,name=f14_float64,proto3" json:"f14_float64"`
	F15Fixed32     uint32            `omnibuf:"15,name=f15_fixed32,wire=fixed32" protobuf:"fixed32,15,opt,name=f15_fixed32,proto3" json:"f15_fixed32"`
	F16Fixed64     uint64            `omnibuf:"16,name=f16_fixed64,wire=fixed64" protobuf:"fixed64,16,opt,name=f16_fixed64,proto3" json:"f16_fixed64"`
	F20OverflowStr string            `omnibuf:"20,name=f20_overflow_str" protobuf:"bytes,20,opt,name=f20_overflow_str,proto3" json:"f20_overflow_str"`
}

type TreeNode struct {
	ID       uint32            `omnibuf:"1,name=id" protobuf:"varint,1,opt,name=id,proto3" json:"id"`
	Name     string            `omnibuf:"2,name=name" protobuf:"bytes,2,opt,name=name,proto3" json:"name"`
	Weight   int64             `omnibuf:"3,name=weight,wire=sint" protobuf:"zigzag64,3,opt,name=weight,proto3" json:"weight"`
	Primary  *TreeNode         `omnibuf:"4,name=primary" protobuf:"bytes,4,opt,name=primary,proto3" json:"primary"`
	Children []TreeNode        `omnibuf:"5,name=children" protobuf:"bytes,5,rep,name=children,proto3" json:"children"`
	Branches []*TreeNode       `omnibuf:"6,name=branches" protobuf:"bytes,6,rep,name=branches,proto3" json:"branches"`
	Labels   map[string]uint64 `omnibuf:"7,name=labels" protobuf:"bytes,7,rep,name=labels,proto3" json:"labels"`
}

type NestedNIC struct {
	IfIndex uint32   `omnibuf:"1,name=if_index" protobuf:"varint,1,opt,name=if_index,proto3"`
	MAC     string   `omnibuf:"2,name=mac" protobuf:"bytes,2,opt,name=mac,proto3"`
	IPs     []uint32 `omnibuf:"3,name=ips" protobuf:"varint,3,rep,packed,name=ips,proto3"`
}

type NestedServer struct {
	ServerID uint32      `omnibuf:"1,name=server_id" protobuf:"varint,1,opt,name=server_id,proto3"`
	Hostname string      `omnibuf:"2,name=hostname" protobuf:"bytes,2,opt,name=hostname,proto3"`
	NICs     []NestedNIC `omnibuf:"3,name=nics" protobuf:"bytes,3,rep,name=nics,proto3"`
}

type NestedRack struct {
	RackID  uint32         `omnibuf:"1,name=rack_id" protobuf:"varint,1,opt,name=rack_id,proto3"`
	Zone    string         `omnibuf:"2,name=zone" protobuf:"bytes,2,opt,name=zone,proto3"`
	Servers []NestedServer `omnibuf:"3,name=servers" protobuf:"bytes,3,rep,name=servers,proto3"`
}

type NestedCluster struct {
	ClusterName string       `omnibuf:"1,name=cluster_name" protobuf:"bytes,1,opt,name=cluster_name,proto3"`
	Racks       []NestedRack `omnibuf:"2,name=racks" protobuf:"bytes,2,rep,name=racks,proto3"`
	Epoch       uint64       `omnibuf:"3,name=epoch" protobuf:"varint,3,opt,name=epoch,proto3"`
}

// ============================================================================
// Conformance Corpus & Runner Harness
// ============================================================================

type ConformanceVector struct {
	Name          string         `json:"name"`
	Category      string         `json:"category"`
	Description   string         `json:"description"`
	Schema        string         `json:"schema"`
	WireHex       string         `json:"wire_hex"`
	WireByteLen   int            `json:"wire_byte_len"`
	PresentFields []uint16       `json:"present_fields"`
	ExpectedBlind map[string]any `json:"expected_blind,omitempty"`
}

type InvalidWireCase struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	WireHex     string `json:"wire_hex"`
}

type ConformanceCorpusFile struct {
	Version       string              `json:"version"`
	Specification string              `json:"specification"`
	ValidVectors  []ConformanceVector `json:"valid_vectors"`
	InvalidCases  []InvalidWireCase   `json:"invalid_cases"`
}

// Summary reports the outcome of running the conformance suite against an adapter binary.
type Summary struct {
	AdapterPath string
	TotalCases  int
	PassedCases int
	Skipped     int
	Failures    []string
	ByTier      map[TestTier]int
}

// RunSuite spawns adapterBinary and executes the full conformance suite over stdin/stdout pipes.
func RunSuite(adapterBinary, corpusPath string) (*Summary, error) {
	corpusBytes, err := os.ReadFile(filepath.Clean(corpusPath))
	if err != nil {
		return nil, fmt.Errorf("read corpus %s: %w", corpusPath, err)
	}
	var corpus ConformanceCorpusFile
	if err := json.Unmarshal(corpusBytes, &corpus); err != nil {
		return nil, fmt.Errorf("unmarshal corpus %s: %w", corpusPath, err)
	}

	// #nosec G204 -- adapterBinary is the explicit conformance testee binary path provided by the test runner caller
	cmd := exec.Command(adapterBinary)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open stdout pipe: %w", err)
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start adapter %s: %w", adapterBinary, err)
	}

	sum := &Summary{
		AdapterPath: adapterBinary,
		ByTier:      make(map[TestTier]int),
	}

	sendReq := func(req *ConformanceRequest) (*ConformanceResponse, error) {
		if err := WriteFrame(stdin, req); err != nil {
			return nil, fmt.Errorf("write request %q (%s): %w (stderr: %s)", req.TestName, TestTier(req.Tier), err, stderrBuf.String())
		}
		var resp ConformanceResponse
		if err := ReadFrame(stdout, &resp); err != nil {
			return nil, fmt.Errorf("read response %q (%s): %w (stderr: %s)", req.TestName, TestTier(req.Tier), err, stderrBuf.String())
		}
		return &resp, nil
	}

	recordResult := func(tier TestTier, name string, failMsg string, skipped string) {
		sum.TotalCases++
		if skipped != "" {
			sum.Skipped++
			return
		}
		if failMsg != "" {
			sum.Failures = append(sum.Failures, fmt.Sprintf("[%s] %s: %s", tier, name, failMsg))
			return
		}
		sum.PassedCases++
		sum.ByTier[tier]++
	}

	// 1. Execute all Valid Vectors across supported tiers
	for _, vec := range corpus.ValidVectors {
		rawWire, err := hex.DecodeString(vec.WireHex)
		if err != nil {
			_ = stdin.Close()
			_ = cmd.Wait()
			return nil, fmt.Errorf("invalid hex in vector %s: %w", vec.Name, err)
		}

		// 1A. Schema-less Blind Inspection Tier (all valid vectors)
		{
			req := &ConformanceRequest{
				TestName:              vec.Name + "/Blind",
				MessageType:           vec.Schema,
				Tier:                  uint32(TestTierBlind),
				RequestedOutputFormat: uint32(WireFormatJSON),
				OmniBufPayload:        rawWire,
			}
			resp, err := sendReq(req)
			if err != nil {
				_ = stdin.Close()
				_ = cmd.Wait()
				return nil, err
			}
			if resp.ParseError != "" || resp.RuntimeError != "" || resp.SerializeError != "" {
				recordResult(TestTierBlind, vec.Name, fmt.Sprintf("unexpected error: parse=%q runtime=%q serialize=%q", resp.ParseError, resp.RuntimeError, resp.SerializeError), resp.Skipped)
			} else if vec.ExpectedBlind != nil {
				var gotBlind map[string]any
				if err := json.Unmarshal([]byte(resp.JSONPayload), &gotBlind); err != nil {
					recordResult(TestTierBlind, vec.Name, fmt.Sprintf("invalid JSON response %q: %v", resp.JSONPayload, err), "")
				} else if !jsonMapsEqual(gotBlind, vec.ExpectedBlind) {
					recordResult(TestTierBlind, vec.Name, fmt.Sprintf("DecodeBlind JSON mismatch:\ngot:  %v\nwant: %v", gotBlind, vec.ExpectedBlind), "")
				} else {
					recordResult(TestTierBlind, vec.Name, "", resp.Skipped)
				}
			} else {
				recordResult(TestTierBlind, vec.Name, "", resp.Skipped)
			}
		}

		// Determine which schema tiers apply to this vector's Schema
		tiers := tiersForSchema(vec.Schema)
		for _, tier := range tiers {
			req := &ConformanceRequest{
				TestName:              vec.Name + "/" + tier.String(),
				MessageType:           canonicalMessageType(vec.Schema),
				Tier:                  uint32(tier),
				RequestedOutputFormat: uint32(WireFormatOmniBuf),
				OmniBufPayload:        rawWire,
			}
			resp, err := sendReq(req)
			if err != nil {
				_ = stdin.Close()
				_ = cmd.Wait()
				return nil, err
			}
			if resp.Skipped != "" {
				recordResult(tier, vec.Name, "", resp.Skipped)
				continue
			}
			if resp.ParseError != "" || resp.SerializeError != "" || resp.RuntimeError != "" {
				recordResult(tier, vec.Name, fmt.Sprintf("unexpected error: parse=%q serialize=%q runtime=%q", resp.ParseError, resp.SerializeError, resp.RuntimeError), "")
				continue
			}
			if err := verifyRoundTripEquivalence(vec.Schema, rawWire, resp.OmniBufPayload); err != nil {
				recordResult(tier, vec.Name, err.Error(), "")
				continue
			}
			recordResult(tier, vec.Name, "", "")

			// For PeerUpdate, also test SelfDescribing (Field #0) output generation across all 3 tiers
			if vec.Schema == "PeerUpdate" {
				reqSD := &ConformanceRequest{
					TestName:              vec.Name + "/" + tier.String() + "/SelfDescribing",
					MessageType:           canonicalMessageType(vec.Schema),
					Tier:                  uint32(tier),
					RequestedOutputFormat: uint32(WireFormatOmniBuf),
					OmniBufPayload:        rawWire,
					SelfDescribing:        true,
				}
				respSD, err := sendReq(reqSD)
				if err != nil {
					_ = stdin.Close()
					_ = cmd.Wait()
					return nil, err
				}
				if respSD.ParseError != "" || respSD.SerializeError != "" || respSD.RuntimeError != "" {
					recordResult(tier, vec.Name+"/SelfDescribing", fmt.Sprintf("error: parse=%q serialize=%q", respSD.ParseError, respSD.SerializeError), respSD.Skipped)
				} else if meta, ok := omnibuf.ReadField0SchemaMeta(respSD.OmniBufPayload); !ok || meta.Fields[2] != "router_id" {
					recordResult(tier, vec.Name+"/SelfDescribing", "missing or invalid Field #0 SchemaMeta in self_describing output", respSD.Skipped)
				} else {
					recordResult(tier, vec.Name+"/SelfDescribing", "", respSD.Skipped)
				}

				if tier == TestTierPlugin {
					reqSplice := &ConformanceRequest{
						TestName:              vec.Name + "/" + tier.String() + "/OpaqueSpliceGrow",
						MessageType:           canonicalMessageType(vec.Schema),
						Tier:                  uint32(tier),
						RequestedOutputFormat: uint32(WireFormatOmniBuf),
						OmniBufPayload:        rawWire,
						TestOpaqueSplice:      true,
					}
					respSplice, err := sendReq(reqSplice)
					if err != nil {
						_ = stdin.Close()
						_ = cmd.Wait()
						return nil, err
					}
					if respSplice.ParseError != "" || respSplice.SerializeError != "" || respSplice.RuntimeError != "" {
						recordResult(tier, vec.Name+"/OpaqueSpliceGrow", fmt.Sprintf("error: parse=%q serialize=%q runtime=%q", respSplice.ParseError, respSplice.SerializeError, respSplice.RuntimeError), respSplice.Skipped)
					} else if err := verifyOpaqueSpliceGrowEquivalence(rawWire, respSplice.OmniBufPayload); err != nil {
						recordResult(tier, vec.Name+"/OpaqueSpliceGrow", err.Error(), respSplice.Skipped)
					} else {
						recordResult(tier, vec.Name+"/OpaqueSpliceGrow", "", respSplice.Skipped)
					}
				}
			}
		}
	}

	// 2. Execute all Invalid / Malformed Wire Cases across all 4 tiers
	for _, ic := range corpus.InvalidCases {
		rawWire, err := hex.DecodeString(ic.WireHex)
		if err != nil {
			_ = stdin.Close()
			_ = cmd.Wait()
			return nil, fmt.Errorf("invalid hex in invalid_case %s: %w", ic.Name, err)
		}

		// 2A. Blind tier MUST reject every malformed wire case with ParseError != ""
		{
			req := &ConformanceRequest{
				TestName:              "Invalid/" + ic.Name + "/Blind",
				MessageType:           "omnibuf.conformance.TestAllTypes",
				Tier:                  uint32(TestTierBlind),
				RequestedOutputFormat: uint32(WireFormatJSON),
				OmniBufPayload:        rawWire,
			}
			resp, err := sendReq(req)
			if err != nil {
				_ = stdin.Close()
				_ = cmd.Wait()
				return nil, err
			}
			if resp.ParseError == "" {
				recordResult(TestTierBlind, "Invalid/"+ic.Name, "expected parse_error on malformed wire input, but succeeded", "")
			} else {
				recordResult(TestTierBlind, "Invalid/"+ic.Name, "", "")
			}
		}

		// 2B. Schema-aware tiers (AnnotatedStruct, ProtobufTypes, Plugin) MUST never crash/panic on malformed bytes
		for _, tier := range []TestTier{TestTierAnnotatedStruct, TestTierProtobufTypes, TestTierPlugin} {
			req := &ConformanceRequest{
				TestName:              "Invalid/" + ic.Name + "/" + tier.String(),
				MessageType:           "omnibuf.bench.PeerUpdate",
				Tier:                  uint32(tier),
				RequestedOutputFormat: uint32(WireFormatOmniBuf),
				OmniBufPayload:        rawWire,
			}
			resp, err := sendReq(req)
			if err != nil {
				_ = stdin.Close()
				_ = cmd.Wait()
				return nil, err
			}
			if resp.RuntimeError != "" {
				recordResult(tier, "Invalid/"+ic.Name, fmt.Sprintf("unexpected runtime_error: %s", resp.RuntimeError), "")
			} else {
				recordResult(tier, "Invalid/"+ic.Name, "", resp.Skipped)
			}
		}
	}

	if err := stdin.Close(); err != nil {
		return nil, fmt.Errorf("close adapter stdin: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("adapter exited with error: %w (stderr: %s)", err, stderrBuf.String())
	}

	return sum, nil
}

func tiersForSchema(schema string) []TestTier {
	switch schema {
	case "PeerUpdate":
		return []TestTier{TestTierPlugin, TestTierProtobufTypes, TestTierAnnotatedStruct}
	case "TestAllTypes", "EmptyMessage", "SparseField14", "SelfDescribingSparse":
		return []TestTier{TestTierAnnotatedStruct}
	case "TreeNode", "NestedCluster":
		return []TestTier{TestTierAnnotatedStruct}
	default:
		return nil
	}
}

func canonicalMessageType(schema string) string {
	switch schema {
	case "PeerUpdate":
		return "omnibuf.bench.PeerUpdate"
	case "TreeNode":
		return "omnibuf.conformance.TreeNode"
	case "NestedCluster":
		return "omnibuf.conformance.NestedCluster"
	default:
		return "omnibuf.conformance.TestAllTypes"
	}
}

// Location is the canonical nested location struct for omnibuf.bench.PeerUpdate conformance tests.
type Location struct {
	Region     string  `omnibuf:"1,name=region" json:"region"`
	Datacenter string  `omnibuf:"2,name=datacenter" json:"datacenter"`
	City       string  `omnibuf:"3,name=city" json:"city"`
	Rack       uint32  `omnibuf:"4,name=rack" json:"rack"`
	Latitude   float32 `omnibuf:"5,name=latitude" json:"latitude"`
	Longitude  float32 `omnibuf:"6,name=longitude" json:"longitude"`
}

// Neighbor is the canonical repeated neighbor struct for omnibuf.bench.PeerUpdate conformance tests.
type Neighbor struct {
	PeerASN     uint32 `omnibuf:"1,name=peer_asn" json:"peer_asn"`
	Router      string `omnibuf:"2,name=router" json:"router"`
	LatencyUS   uint32 `omnibuf:"3,name=latency_us" json:"latency_us"`
	Established bool   `omnibuf:"4,name=established" json:"established"`
}

// PeerUpdate is the canonical plain annotated struct for omnibuf.bench.PeerUpdate conformance tests.
type PeerUpdate struct {
	ASN       uint32     `omnibuf:"1,name=asn" json:"asn"`
	RouterID  string     `omnibuf:"2,name=router_id" json:"router_id"`
	Active    bool       `omnibuf:"3,name=active" json:"active"`
	UptimeSec uint64     `omnibuf:"4,name=uptime_sec" json:"uptime_sec"`
	RssiDbm   int32      `omnibuf:"5,name=rssi_dbm" json:"rssi_dbm"`
	CPULoad   float64    `omnibuf:"6,name=cpu_load" json:"cpu_load"`
	Tags      []string   `omnibuf:"7,name=tags" json:"tags"`
	Prefixes  []uint32   `omnibuf:"8,name=prefixes" json:"prefixes"`
	Location  Location   `omnibuf:"9,name=location" json:"location"`
	Neighbors []Neighbor `omnibuf:"10,name=neighbors" json:"neighbors"`
	Sequence  uint64     `omnibuf:"11,name=sequence" json:"sequence"`
}

func verifyOpaqueSpliceGrowEquivalence(inputWire, outputWire []byte) error {
	var want, got PeerUpdate
	if err := omnibuf.Unmarshal(inputWire, &want); err != nil {
		return fmt.Errorf("runner decode input PeerUpdate: %w", err)
	}
	want.ASN = 1_000_000
	want.RouterID = strings.Repeat("edge-rtr-fra01-", 20)
	want.UptimeSec = math.MaxUint64
	want.Sequence++

	if err := omnibuf.Unmarshal(outputWire, &got); err != nil {
		return fmt.Errorf("runner decode spliced PeerUpdate: %w", err)
	}
	if !reflect.DeepEqual(want, got) {
		return fmt.Errorf("PeerUpdate OpaqueSpliceGrow mismatch:\ngot:  %+v\nwant: %+v", got, want)
	}
	return nil
}

func verifyRoundTripEquivalence(schema string, inputWire, outputWire []byte) error {
	switch schema {
	case "PeerUpdate":
		var want, got PeerUpdate
		if err := omnibuf.Unmarshal(inputWire, &want); err != nil {
			return fmt.Errorf("runner decode input PeerUpdate: %w", err)
		}
		if err := omnibuf.Unmarshal(outputWire, &got); err != nil {
			return fmt.Errorf("runner decode output PeerUpdate: %w", err)
		}
		if !reflect.DeepEqual(want, got) {
			return fmt.Errorf("PeerUpdate round-trip mismatch:\ngot:  %+v\nwant: %+v", got, want)
		}
		return nil
	case "TreeNode":
		var want, got TreeNode
		if err := omnibuf.Unmarshal(inputWire, &want); err != nil {
			return fmt.Errorf("runner decode input TreeNode: %w", err)
		}
		if err := omnibuf.Unmarshal(outputWire, &got); err != nil {
			return fmt.Errorf("runner decode output TreeNode: %w", err)
		}
		if !reflect.DeepEqual(want, got) {
			return fmt.Errorf("TreeNode round-trip mismatch:\ngot:  %+v\nwant: %+v", got, want)
		}
		return nil
	case "NestedCluster":
		var want, got NestedCluster
		if err := omnibuf.Unmarshal(inputWire, &want); err != nil {
			return fmt.Errorf("runner decode input NestedCluster: %w", err)
		}
		if err := omnibuf.Unmarshal(outputWire, &got); err != nil {
			return fmt.Errorf("runner decode output NestedCluster: %w", err)
		}
		if !reflect.DeepEqual(want, got) {
			return fmt.Errorf("NestedCluster round-trip mismatch:\ngot:  %+v\nwant: %+v", got, want)
		}
		return nil
	default:
		var want, got TestAllTypes
		if err := omnibuf.Unmarshal(inputWire, &want); err != nil {
			return fmt.Errorf("runner decode input TestAllTypes: %w", err)
		}
		if err := omnibuf.Unmarshal(outputWire, &got); err != nil {
			return fmt.Errorf("runner decode output TestAllTypes: %w", err)
		}
		if !testAllTypesEqual(&want, &got) {
			return fmt.Errorf("TestAllTypes round-trip mismatch:\ngot:  %+v\nwant: %+v", got, want)
		}
		return nil
	}
}

func testAllTypesEqual(a, b *TestAllTypes) bool {
	if a.F1Uint32 != b.F1Uint32 ||
		a.F2Uint64 != b.F2Uint64 ||
		a.F3Sint32 != b.F3Sint32 ||
		a.F4Sint64 != b.F4Sint64 ||
		a.F5Bool != b.F5Bool ||
		a.F6String != b.F6String ||
		!bytes.Equal(a.F7Bytes, b.F7Bytes) ||
		mathFloat32Bits(a.F13Float32) != mathFloat32Bits(b.F13Float32) ||
		mathFloat64Bits(a.F14Float64) != mathFloat64Bits(b.F14Float64) ||
		a.F15Fixed32 != b.F15Fixed32 ||
		a.F16Fixed64 != b.F16Fixed64 ||
		a.F20OverflowStr != b.F20OverflowStr {
		return false
	}
	if !reflect.DeepEqual(a.F8SubMsg, b.F8SubMsg) {
		return false
	}
	if len(a.F9StringArr) != len(b.F9StringArr) || (len(a.F9StringArr) > 0 && !reflect.DeepEqual(a.F9StringArr, b.F9StringArr)) {
		return false
	}
	if len(a.F10Uint32Arr) != len(b.F10Uint32Arr) || (len(a.F10Uint32Arr) > 0 && !reflect.DeepEqual(a.F10Uint32Arr, b.F10Uint32Arr)) {
		return false
	}
	if len(a.F11SubMsgArr) != len(b.F11SubMsgArr) || (len(a.F11SubMsgArr) > 0 && !reflect.DeepEqual(a.F11SubMsgArr, b.F11SubMsgArr)) {
		return false
	}
	if len(a.F12MapStrUint) != len(b.F12MapStrUint) || (len(a.F12MapStrUint) > 0 && !reflect.DeepEqual(a.F12MapStrUint, b.F12MapStrUint)) {
		return false
	}
	return true
}

func mathFloat32Bits(f float32) uint32 {
	if math.IsNaN(float64(f)) {
		return 0x7fc00000
	}
	return math.Float32bits(f)
}

func mathFloat64Bits(f float64) uint64 {
	if math.IsNaN(f) {
		return 0x7ff8000000000000
	}
	return math.Float64bits(f)
}

func jsonMapsEqual(a, b map[string]any) bool {
	ja, err1 := json.Marshal(a)
	jb, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		return false
	}
	var na, nb any
	if err := json.Unmarshal(ja, &na); err != nil {
		return false
	}
	if err := json.Unmarshal(jb, &nb); err != nil {
		return false
	}
	return reflect.DeepEqual(na, nb)
}
