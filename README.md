# OmniBuf

[![CI](https://github.com/sudorandom/omnibuf/actions/workflows/ci.yml/badge.svg)](https://github.com/sudorandom/omnibuf/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/sudorandom/omnibuf.svg)](https://pkg.go.dev/github.com/sudorandom/omnibuf)
[![Website](https://img.shields.io/badge/website-omnibuf.com-111111?style=flat&labelColor=111111&color=D4FF33)](https://omnibuf.com)

**[Website (omnibuf.com)](https://omnibuf.com)** · **[How It Works](https://omnibuf.com/how-it-works)** · **[Benchmarks](https://omnibuf.com/benchmarks)** · **[Wire Format Specification](https://omnibuf.com/spec)**

**OmniBuf** is a binary wire format for Protocol Buffers (`.proto` schemas) built around four ideas:

1. **Keep `.proto` schemas and Go structs:** Existing `.proto` files, `protoc-gen-go` structs, and `dynamicpb` messages work without changes.
2. **Fix Protobuf's CPU bottlenecks:** Prefix-Varints (`TZCNT`) decode integers in a single load instead of a byte-by-byte continuation loop, and 2-byte length backpatching encodes messages in a single forward pass instead of walking the struct tree twice (~6x faster encode and ~8x faster zero-copy decode than `google.golang.org/protobuf`).
3. **Inspect packets without a schema (`DecodeBlind`):** Explicit 4-bit wire types (`0..15`) replace Protobuf's overloaded `Wire Type 2`, and optional **Field `#0`** metadata attaches field names on the wire when needed.
4. **Stay a flat `[Tag][Value]` stream:** No vtables or pointer graphs. Containers carry outer byte lengths for single-step skipping (`LocateField`), and concatenating two packets (`append(msgA, msgB...)`) merges them just like Protobuf.

> **Language Support:** OmniBuf is currently a **Go-only** implementation (`github.com/sudorandom/omnibuf`). Because the wire format is a flat `[Tag][Value]` stream with 4 integer widths and 16 wire types (no vtables or relative pointer graphs), it is straightforward to port. If there is interest, support for more languages will be added!

---

## Go Library Usage

OmniBuf works directly with plain Go structs using `omnibuf:"..."` struct tags as well as existing `protoc-gen-go` generated `.pb.go` structs (`proto.Message` / `protoreflect.Message`):

### 1. Plain Go Structs (`omnibuf` Struct Tags)

Tag exported struct fields with a field number (`1..65535`) and optional key-value settings (`name=<name>`, `wire=<type>`, `omitempty`, or `-` to ignore):

```go
package main

import (
	"fmt"

	"github.com/sudorandom/omnibuf"
)

type PeerEvent struct {
	ASN       uint32            `omnibuf:"1,name=asn"`
	RouterID  string            `omnibuf:"2,name=router_id"`
	Active    bool              `omnibuf:"3,name=active"`
	LatencyUS uint32            `omnibuf:"4,name=latency_us,wire=fixed32,omitempty"`
	Tags      []string          `omnibuf:"5,name=tags,omitempty"`
	Counters  map[string]uint64 `omnibuf:"6,name=counters,omitempty"`
	Internal  string            `omnibuf:"-"`
}

func main() {
	in := &PeerEvent{
		ASN:       64512,
		RouterID:  "edge-rtr-fra-01",
		Active:    true,
		LatencyUS: 850,
		Tags:      []string{"tier1", "ebgp"},
	}

	// Marshal (or use MarshalOptions{SelfDescribing: true} to embed Field #0 names)
	wire, err := omnibuf.MarshalOptions{SelfDescribing: true}.Marshal(in)
	if err != nil {
		panic(err)
	}

	var out PeerEvent
	if err := omnibuf.Unmarshal(wire, &out); err != nil {
		panic(err)
	}

	// Inspect without the Go struct definition (uses Field #0 names when present)
	blind, err := omnibuf.DecodeBlind(wire)
	if err != nil {
		panic(err)
	}
	fmt.Println(blind)
}
```

#### Struct Tag Reference (`omnibuf:"..."`)

| Tag Option | Example | Description |
| :--- | :--- | :--- |
| `<fieldNum>` | `omnibuf:"1"` | Field number (`1..65535`). Required for serialized fields. |
| `name=<name>` | `omnibuf:"2,name=router_id"` | Custom field name emitted in `Field #0` (`SchemaMeta`) when `SelfDescribing: true` is enabled (defaults to the Go field name). |
| `wire=<type>` | `omnibuf:"4,wire=fixed32"` | Overrides the inferred wire encoding (`uint`, `sint`, `fixed32`, `fixed64`, `string`, `bytes`). |
| `omitempty` | `omnibuf:"5,omitempty"` | Skips encoding the field when it holds its Go zero value or an empty slice/map. |
| `-` | `omnibuf:"-"` | Ignores the field during encoding and decoding (even if a `protobuf` tag is present). |

### 2. Existing `.pb.go` Structs (`protoc-gen-go`)

```go
package main

import (
	"fmt"

	"example.com/yourproject/pb"
	"github.com/sudorandom/omnibuf"
)

func main() {
	msg := &pb.PeerUpdate{
		Asn:      64512,
		RouterId: "edge-rtr-fra-01",
		Active:   true,
	}

	// Marshal any .pb.go struct or protoreflect.Message into OmniBuf wire bytes
	wire, err := omnibuf.Marshal(msg)
	if err != nil {
		panic(err)
	}

	// Unmarshal back into a struct
	var out pb.PeerUpdate
	if err := omnibuf.Unmarshal(wire, &out); err != nil {
		panic(err)
	}

	// Or inspect values without a schema (DecodeBlind)
	blind, err := omnibuf.DecodeBlind(wire)
	if err != nil {
		panic(err)
	}
	fmt.Println(blind)
}
```

---

## CLI Usage (`omnibuf`)

Build or install the CLI tool:

```bash
go install github.com/sudorandom/omnibuf/cmd/omnibuf@latest
# or locally via just:
just build-cli
```

```bash
# Blind decode any OmniBuf packet to JSON without a schema
omnibuf decode testdata/golden/peer_update.field0.obuf

# Decode using a FileDescriptorSet (.json, .pb, or .pb.gz)
omnibuf decode -s testdata/golden/peer.fds.pb.gz -t PeerUpdate testdata/golden/peer_update.shared.obuf

# Encode JSON to OmniBuf wire bytes using a FileDescriptorSet (.json, .pb, or .pb.gz)
omnibuf encode -s testdata/golden/peer.fds.pb.gz -t PeerUpdate testdata/golden/peer_update.input.json -o out.obuf

# Encode with Field #0 inline field names and output hex
omnibuf encode -s testdata/golden/peer.fds.json -t PeerUpdate --field0 --hex testdata/golden/peer_update.input.json
```

---

## Development Quick Start

```bash
# Install tool dependencies (buf, go, golangci-lint, just, node, pnpm) via mise
mise install

# Format and lint Go, Protobuf schemas (buf), and frontend
just fmt
just lint

# Run all unit, conformance, golden, and dynamicpb matrix tests
just test

# Run the fuzzer
just fuzz 10s

# Run the complete benchmark suite
just bench

# Start the Astro website dev server (or build into web/dist)
just web-dev
just web-build
```
