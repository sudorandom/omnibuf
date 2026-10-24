# OmniBuf

[![CI](https://github.com/sudorandom/omnibuf/actions/workflows/ci.yml/badge.svg)](https://github.com/sudorandom/omnibuf/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/sudorandom/omnibuf.svg)](https://pkg.go.dev/github.com/sudorandom/omnibuf)
[![Go Report Card](https://goreportcard.com/badge/github.com/sudorandom/omnibuf)](https://goreportcard.com/report/github.com/sudorandom/omnibuf)
[![Website](https://img.shields.io/badge/website-omnibuf.com-111111?style=flat&labelColor=111111&color=D4FF33)](https://omnibuf.com)

**[Website (omnibuf.com)](https://omnibuf.com)** · **[How It Works](https://omnibuf.com/how-it-works)** · **[Benchmarks](https://omnibuf.com/benchmarks)** · **[Wire Format Specification](https://omnibuf.com/spec)**

**OmniBuf** is a binary wire encoding replacement for Protocol Buffers (`.proto` schemas) designed around four goals:

1. **Stay compatible with `.proto` file semantics and types** (existing `.proto` schemas, `protoc-gen-go` structs, and `dynamicpb` messages work directly).
2. **Faster encoding and decoding on real hardware** via branchless `TZCNT` Prefix-Varints and single-pass 2-byte length backpatching (~85% faster marshal and ~88% faster zero-copy unmarshal than `google.golang.org/protobuf`, and ~58% faster than hand-optimized `protowire` / `vtprotobuf`).
3. **Schema-less value decoding (`DecodeBlind`)** via explicit 4-bit wire types (`0..15`), with optional inline field names via **Field `#0`**.
4. **Simple, index-free wire layout** with outer container byte lengths and full Protocol Buffers concatenation merge semantics (`append(msgA, msgB...)`), enabling branchless header-hop field lookup (`LocateField`) and zero-decode wire splicing without vtables or footer offset tables.

> **Language Support:** OmniBuf is currently a **Go-only** implementation (`github.com/sudorandom/omnibuf`). Because the wire format is intentionally simple (4 integer widths and 16 wire types, with no vtables or pointer graphs), porting takes only a few hundred lines of code. If there is interest, support for more languages will be added!

---

## Go Library Usage

OmniBuf works directly with your existing `protoc-gen-go` generated `.pb.go` structs (`proto.Message` / `protoreflect.Message`) without requiring a custom `protoc` plugin:

```go
package main

import (
	"fmt"

	"github.com/sudorandom/omnibuf"
	"github.com/sudorandom/omnibuf/pb"
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
