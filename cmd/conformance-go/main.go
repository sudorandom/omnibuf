// Command conformance-go is the Go implementation's adapter binary for the
// OmniBuf conformance test runner. It reads length-prefixed ConformanceRequest
// messages from stdin and writes length-prefixed ConformanceResponse messages
// to stdout until EOF.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/sudorandom/omnibuf"
	"github.com/sudorandom/omnibuf/internal/conformance"
	"github.com/sudorandom/omnibuf/internal/pb"
)

func main() {
	in := bufio.NewReaderSize(os.Stdin, 64*1024)
	out := bufio.NewWriterSize(os.Stdout, 64*1024)

	for {
		var req conformance.ConformanceRequest
		err := conformance.ReadFrame(in, &req)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "conformance-go: read frame error: %v\n", err)
			os.Exit(1)
		}

		resp := handleRequest(&req)
		if err := conformance.WriteFrame(out, resp); err != nil {
			fmt.Fprintf(os.Stderr, "conformance-go: write frame error: %v\n", err)
			os.Exit(1)
		}
		if err := out.Flush(); err != nil {
			fmt.Fprintf(os.Stderr, "conformance-go: flush error: %v\n", err)
			os.Exit(1)
		}
	}
}

func handleRequest(req *conformance.ConformanceRequest) (resp *conformance.ConformanceResponse) {
	defer func() {
		if r := recover(); r != nil {
			resp = &conformance.ConformanceResponse{
				RuntimeError: fmt.Sprintf("panic: %v", r),
			}
		}
	}()

	tier := conformance.TestTier(req.Tier)
	switch tier {
	case conformance.TestTierBlind:
		return handleBlind(req)
	case conformance.TestTierPlugin:
		return handlePluginTier(req)
	case conformance.TestTierProtobufTypes:
		return handleProtobufTypesTier(req)
	case conformance.TestTierAnnotatedStruct:
		return handleAnnotatedStructTier(req)
	default:
		return &conformance.ConformanceResponse{
			RuntimeError: fmt.Sprintf("unsupported tier: %d", req.Tier),
		}
	}
}

func handleBlind(req *conformance.ConformanceRequest) *conformance.ConformanceResponse {
	m, err := omnibuf.DecodeBlind(req.OmniBufPayload)
	if err != nil {
		return &conformance.ConformanceResponse{
			ParseError: err.Error(),
		}
	}
	if conformance.WireFormat(req.RequestedOutputFormat) == conformance.WireFormatJSON {
		jb, err := json.Marshal(m)
		if err != nil {
			// Handle NaN/Inf maps gracefully when JSON isn't representable
			return &conformance.ConformanceResponse{
				JSONPayload: "{}",
			}
		}
		return &conformance.ConformanceResponse{
			JSONPayload: string(jb),
		}
	}
	return &conformance.ConformanceResponse{}
}

func handlePluginTier(req *conformance.ConformanceRequest) *conformance.ConformanceResponse {
	switch req.MessageType {
	case "omnibuf.bench.PeerUpdate":
		var msg pb.PeerUpdate
		if err := msg.UnmarshalOmniBuf(req.OmniBufPayload, false); err != nil {
			return &conformance.ConformanceResponse{
				ParseError: err.Error(),
			}
		}
		// Exercise lazy Opaque view getters to verify zero-panic conformance
		var op pb.OpaquePeerUpdate
		_ = op.Unmarshal(req.OmniBufPayload)
		_ = op.GetAsn()
		_ = op.GetRouterId()
		_ = op.GetActive()
		_ = op.GetUptimeSec()
		_ = op.GetRssiDbm()
		_ = op.GetCpuLoad()
		_ = op.TagsLen()
		_ = op.PrefixesLen()
		_ = op.GetLocation().GetCity()
		_ = op.NeighborsLen()
		_ = op.GetSequence()

		if req.TestOpaqueSplice {
			op.SetAsn(1_000_000)
			op.SetRouterId(strings.Repeat("edge-rtr-fra01-", 20))
			op.SetActive(!op.GetActive())
			op.SetUptimeSec(math.MaxUint64)
			op.SetRssiDbm(-1_000_000)
			op.SetCpuLoad(99.875)
			op.SetTags([]string{"tag-grow-a", "tag-grow-b", strings.Repeat("long-tag-", 20)})
			op.SetPrefixes([]uint32{100000, 200000, 300000, 400000, 500000, 600000})
			op.SetLocation(&pb.Location{
				Region:     "eu-central-grown",
				Datacenter: strings.Repeat("fra-dc-", 25),
				City:       "Berlin",
				Rack:       999999,
				Latitude:   52.52,
				Longitude:  13.405,
			})
			op.SetSequence(op.GetSequence() + 1)
			return &conformance.ConformanceResponse{
				OmniBufPayload: op.MarshalAppend(nil),
			}
		}
		if req.TestOpaqueSpliceShrink {
			op.SetAsn(1)
			op.SetRouterId("r")
			op.SetActive(false)
			op.SetUptimeSec(5)
			op.SetRssiDbm(-1)
			op.SetCpuLoad(0.5)
			op.SetTags([]string{"t"})
			op.SetPrefixes([]uint32{1})
			op.SetLocation(&pb.Location{
				Region:     "r",
				Datacenter: "d",
				City:       "c",
				Rack:       1,
				Latitude:   1.0,
				Longitude:  2.0,
			})
			op.SetSequence(2)
			return &conformance.ConformanceResponse{
				OmniBufPayload: op.MarshalAppend(nil),
			}
		}

		if req.SelfDescribing {
			out, err := (omnibuf.MarshalOptions{Deterministic: true, SelfDescribing: true}).Marshal(&msg)
			if err != nil {
				return &conformance.ConformanceResponse{SerializeError: err.Error()}
			}
			return &conformance.ConformanceResponse{OmniBufPayload: out}
		}
		dst := msg.MarshalOmniBufAppend(nil)
		return &conformance.ConformanceResponse{
			OmniBufPayload: dst,
		}

	case "omnibuf.conformance.TestAllTypes":
		var msg pb.TestAllTypes
		if err := msg.UnmarshalOmniBuf(req.OmniBufPayload, false); err != nil {
			return &conformance.ConformanceResponse{
				ParseError: err.Error(),
			}
		}
		var op pb.OpaqueTestAllTypes
		_ = op.Unmarshal(req.OmniBufPayload)
		_ = op.GetF1Uint32()
		_ = op.GetF2Uint64()
		_ = op.GetF3Sint32()
		_ = op.GetF4Sint64()
		_ = op.GetF5Bool()
		_ = op.GetF6String()
		_ = op.GetF7Bytes()
		_ = op.GetF8SubMsg().GetLabel()
		_ = op.F9StringArrLen()
		_ = op.F10Uint32ArrLen()
		_ = op.F11SubMsgArrLen()
		_, _ = op.LookupF12MapStrUint("k")
		_ = op.GetF13Float32()
		_ = op.GetF14Float64()
		_ = op.GetF15Fixed32()
		_ = op.GetF16Fixed64()
		_ = op.GetF20OverflowStr()

		if req.TestOpaqueSplice {
			op.SetF1Uint32(1_000_000)
			op.SetF2Uint64(math.MaxUint64)
			op.SetF3Sint32(-1_000_000)
			op.SetF4Sint64(math.MinInt64)
			op.SetF5Bool(!op.GetF5Bool())
			op.SetF6String(strings.Repeat("grown-str-", 30))
			op.SetF7Bytes(bytes.Repeat([]byte{0xab, 0xcd}, 130))
			op.SetF8SubMsg(&pb.AllTypesSubMsg{
				Id:    888888,
				Label: strings.Repeat("sub-grown-", 20),
				Flag:  true,
				Score: -999999999,
			})
			op.SetF9StringArr([]string{"arr-grow-1", "arr-grow-2", strings.Repeat("arr-grow-3-", 15)})
			op.SetF10Uint32Arr([]uint32{100000, 200000, 300000, 400000, 500000})
			op.SetF11SubMsgArr([]*pb.AllTypesSubMsg{
				{Id: 100001, Label: strings.Repeat("elem0-", 25), Flag: true, Score: -500000},
				{Id: 100002, Label: "elem1-grown", Flag: false, Score: 500000},
			})
			op.SetF12MapStrUint(map[string]uint64{
				"k1":          1000000,
				"k2":          math.MaxUint64,
				"k3_long_key": 424242,
			})
			op.SetF13Float32(-1234.5)
			op.SetF14Float64(3.141592653589793)
			op.SetF15Fixed32(0xdeadbeef)
			op.SetF16Fixed64(0xcafebabedeadbeef)
			op.SetF20OverflowStr(strings.Repeat("overflow20-", 20))
			return &conformance.ConformanceResponse{
				OmniBufPayload: op.MarshalAppend(nil),
			}
		}
		if req.TestOpaqueSpliceShrink {
			op.SetF1Uint32(1)
			op.SetF2Uint64(2)
			op.SetF3Sint32(-1)
			op.SetF4Sint64(-2)
			op.SetF5Bool(false)
			op.SetF6String("s")
			op.SetF7Bytes([]byte{0x01})
			op.SetF8SubMsg(&pb.AllTypesSubMsg{Id: 1, Label: "x", Flag: false, Score: -1})
			op.SetF9StringArr([]string{"a"})
			op.SetF10Uint32Arr([]uint32{7})
			op.SetF11SubMsgArr([]*pb.AllTypesSubMsg{
				{Id: 1, Label: "e", Flag: true, Score: 1},
			})
			op.SetF12MapStrUint(map[string]uint64{"k": 1})
			op.SetF13Float32(0.5)
			op.SetF14Float64(-0.25)
			op.SetF15Fixed32(7)
			op.SetF16Fixed64(9)
			op.SetF20OverflowStr("z")
			return &conformance.ConformanceResponse{
				OmniBufPayload: op.MarshalAppend(nil),
			}
		}

		dst := msg.MarshalOmniBufAppend(nil)
		return &conformance.ConformanceResponse{
			OmniBufPayload: dst,
		}

	default:
		return &conformance.ConformanceResponse{
			Skipped: "plugin tier generated for PeerUpdate and TestAllTypes",
		}
	}
}

func handleProtobufTypesTier(req *conformance.ConformanceRequest) *conformance.ConformanceResponse {
	opts := omnibuf.MarshalOptions{
		Deterministic:  true,
		SelfDescribing: req.SelfDescribing,
	}
	switch req.MessageType {
	case "omnibuf.bench.PeerUpdate":
		var msg pb.PeerUpdate
		if err := omnibuf.Unmarshal(req.OmniBufPayload, &msg); err != nil {
			return &conformance.ConformanceResponse{
				ParseError: err.Error(),
			}
		}
		out, err := opts.Marshal(&msg)
		if err != nil {
			return &conformance.ConformanceResponse{
				SerializeError: err.Error(),
			}
		}
		return &conformance.ConformanceResponse{
			OmniBufPayload: out,
		}
	case "omnibuf.conformance.TestAllTypes":
		var msg pb.TestAllTypes
		if err := omnibuf.Unmarshal(req.OmniBufPayload, &msg); err != nil {
			return &conformance.ConformanceResponse{
				ParseError: err.Error(),
			}
		}
		out, err := opts.Marshal(&msg)
		if err != nil {
			return &conformance.ConformanceResponse{
				SerializeError: err.Error(),
			}
		}
		return &conformance.ConformanceResponse{
			OmniBufPayload: out,
		}
	default:
		return &conformance.ConformanceResponse{
			Skipped: "protobuf types tier exercises pb.PeerUpdate and pb.TestAllTypes",
		}
	}
}

func handleAnnotatedStructTier(req *conformance.ConformanceRequest) *conformance.ConformanceResponse {
	opts := omnibuf.MarshalOptions{
		Deterministic:  true,
		SelfDescribing: req.SelfDescribing,
	}
	switch req.MessageType {
	case "omnibuf.bench.PeerUpdate":
		var msg conformance.PeerUpdate
		if err := omnibuf.Unmarshal(req.OmniBufPayload, &msg); err != nil {
			return &conformance.ConformanceResponse{ParseError: err.Error()}
		}
		out, err := opts.Marshal(&msg)
		if err != nil {
			return &conformance.ConformanceResponse{SerializeError: err.Error()}
		}
		return &conformance.ConformanceResponse{OmniBufPayload: out}

	case "omnibuf.conformance.TreeNode":
		var msg conformance.TreeNode
		if err := omnibuf.Unmarshal(req.OmniBufPayload, &msg); err != nil {
			return &conformance.ConformanceResponse{ParseError: err.Error()}
		}
		out, err := opts.Marshal(&msg)
		if err != nil {
			return &conformance.ConformanceResponse{SerializeError: err.Error()}
		}
		return &conformance.ConformanceResponse{OmniBufPayload: out}

	case "omnibuf.conformance.NestedCluster":
		var msg conformance.NestedCluster
		if err := omnibuf.Unmarshal(req.OmniBufPayload, &msg); err != nil {
			return &conformance.ConformanceResponse{ParseError: err.Error()}
		}
		out, err := opts.Marshal(&msg)
		if err != nil {
			return &conformance.ConformanceResponse{SerializeError: err.Error()}
		}
		return &conformance.ConformanceResponse{OmniBufPayload: out}

	default:
		var msg conformance.TestAllTypes
		if err := omnibuf.Unmarshal(req.OmniBufPayload, &msg); err != nil {
			return &conformance.ConformanceResponse{ParseError: err.Error()}
		}
		out, err := opts.Marshal(&msg)
		if err != nil {
			return &conformance.ConformanceResponse{SerializeError: err.Error()}
		}
		return &conformance.ConformanceResponse{OmniBufPayload: out}
	}
}
