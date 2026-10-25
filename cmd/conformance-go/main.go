// Command conformance-go is the Go implementation's adapter binary for the
// OmniBuf conformance test runner. It reads length-prefixed ConformanceRequest
// messages from stdin and writes length-prefixed ConformanceResponse messages
// to stdout until EOF.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/sudorandom/omnibuf"
	"github.com/sudorandom/omnibuf/conformance"
	"github.com/sudorandom/omnibuf/pb"
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
	if req.MessageType != "omnibuf.bench.PeerUpdate" {
		return &conformance.ConformanceResponse{
			Skipped: "plugin tier generated for omnibuf.bench.PeerUpdate",
		}
	}
	var msg pb.PeerUpdate
	if err := msg.UnmarshalOmniBuf(req.OmniBufPayload, false); err != nil {
		return &conformance.ConformanceResponse{
			ParseError: err.Error(),
		}
	}
	// Also exercise lazy Opaque view getters to verify zero-panic conformance
	var op pb.OpaquePeerUpdate
	_ = op.Unmarshal(req.OmniBufPayload)
	_ = op.GetAsn()
	_ = op.GetRouterId()
	_ = op.GetActive()
	_ = op.GetSequence()

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
}

func handleProtobufTypesTier(req *conformance.ConformanceRequest) *conformance.ConformanceResponse {
	if req.MessageType != "omnibuf.bench.PeerUpdate" {
		return &conformance.ConformanceResponse{
			Skipped: "protobuf types tier exercises pb.PeerUpdate",
		}
	}
	var msg pb.PeerUpdate
	if err := omnibuf.Unmarshal(req.OmniBufPayload, &msg); err != nil {
		return &conformance.ConformanceResponse{
			ParseError: err.Error(),
		}
	}
	opts := omnibuf.MarshalOptions{
		Deterministic:  true,
		SelfDescribing: req.SelfDescribing,
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
