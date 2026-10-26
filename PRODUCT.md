# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Backend and systems engineers evaluating binary serialization formats, reading the wire specification, or trying the Go implementation (`protoc-gen-omnibuf` and the `omnibuf` runtime package). Secondary audiences include technical decision-makers comparing serialization trade-offs across Protobuf, FlatBuffers, Cap'n Proto, MessagePack, and JSON, as well as contributors interested in implementing OmniBuf in additional languages.

## Product Purpose

OmniBuf (`omnibuf.com`) is a binary wire format and code generator for `.proto` schemas. It exists to keep everything Protobuf got right at the schema layer (explicit field IDs, forward/backward schema evolution, compact binary payloads, and raw byte concatenation) while fixing the two biggest drawbacks of Protobuf's 2001 wire format: byte-by-byte LEB128 varint decoding and wire types that cannot be decoded or inspected without a schema.

Success on the web surface means an engineer can immediately understand how the wire format works at the byte level, verify its trade-offs against existing serializers with reproducible benchmarks, and adopt or implement the specification.

## Positioning

Unlike FlatBuffers or Cap'n Proto (which require custom IDLs, arena builders, and larger pointer/vtable overhead) and unlike MessagePack or JSON (which repeat string keys or lack `.proto` schema evolution), OmniBuf compiles standard `.proto` files into a self-describing binary wire format that encodes and decodes 6x to 8x faster than standard Go Protobuf (`google.golang.org/protobuf`) and ~45% faster than `vtprotobuf` while generating roughly half the code.

Its core wire-format mechanisms are:
- **trailing-zero Prefix-Varints (`TZCNT`):** The first byte reveals the exact byte width (`1`, `2`, `4`, or `9` bytes), enabling single-instruction little-endian loads and stores instead of byte-by-byte continuation-bit loops.
- **16 self-describing wire types (4-bit nibble):** Every tag encodes the exact data type (`WireUint`, `WireSint`, `WireBoolFalse`, `WireBoolTrue`, `WireFixed32`, `WireFixed64`, `WireSfixed32`, `WireSfixed64`, `WireFloat32`, `WireFloat64`, `WireString`, `WireBytes`, `WireSubMsg`, `WireArray`, `WireMap`, `WireExtended`), allowing any payload to be inspected or converted to JSON without the `.proto` file.
- **Contiguous primitive arrays and length-prefixed containers:** Eliminates repeated per-element tags and varint loops on slices, allowing any container to be skipped in one jump (`pos += byteLen`).
- **Single-pass forward encoding:** Fixed 2-byte length patch slots avoid Protobuf's two-pass `Size()` calculation.
- **Opaque field lookup and splicing (`LocateField`, `SpliceFields`):** Jump over unrelated fields in `O(1)` per field using self-describing byte lengths to read, drop, or replace individual fields without decoding the message.

## Operating Context

- Evaluated alongside standard `.proto` toolchains (`protoc`, `buf`), Go benchmarks (`go test -bench`), and existing wire formats (`google.golang.org/protobuf`, `vtprotobuf`, FlatBuffers, Cap'n Proto, MessagePack, and `encoding/json/v2`).
- The web site (`web/`) is an Astro + Tailwind CSS static site deployed to Cloudflare Pages with five primary surfaces: Overview (`/`), How It Works (`/how-it-works`), Benchmarks (`/benchmarks`), Wire Specification (`/spec`), and Go Language Guide (`/go`).

## Capabilities and Constraints

- **Language support:** The wire specification is language-agnostic, but only the Go runtime and `protoc-gen-omnibuf` plugin are implemented today. Always state this candidly.
- **Benchmark presentation:** High-level callouts, cards, and prose must use relative whole-number multipliers and percentages (such as `6x faster`, `8x faster`, `40% smaller`, `50% less code`) rather than fragile exact `ns/op`, byte counts, or line counts. Detailed benchmark tables and methodology notes remain grounded in reproducible repository benchmarks.
- **Honest trade-offs:** Every format comparison must acknowledge where competing formats win (for example, Protobuf's ubiquitous multi-language ecosystem, FlatBuffers/Cap'n Proto's zero-unpack `O(1)` random access into large buffers, MessagePack's schema-free dynamic maps, and JSON's native browser/human readability).

## Brand Commitments

- **Voice and tone:** Direct, technical, and opinionated engineering prose. Explain what the bytes and CPU instructions actually do.
- **Copy bans:** Never use marketing fluff, AI-sounding filler phrases, or em-dashes anywhere in copy or documentation.

## Evidence on Hand

- Reproducible Go benchmark suite in [`omnibuf_bench_test.go`](file:///Users/kevin/Projects/omnibuf/omnibuf_bench_test.go) and [`opaque_table_bench_test.go`](file:///Users/kevin/Projects/omnibuf/opaque_table_bench_test.go), summarized in [`web/src/data/benchmarks.ts`](file:///Users/kevin/Projects/omnibuf/web/src/data/benchmarks.ts).
- Complete wire-format implementation in [`omnibuf.go`](file:///Users/kevin/Projects/omnibuf/omnibuf.go), [`array.go`](file:///Users/kevin/Projects/omnibuf/array.go), [`opaque.go`](file:///Users/kevin/Projects/omnibuf/opaque.go), and code generator in [`cmd/protoc-gen-omnibuf/main.go`](file:///Users/kevin/Projects/omnibuf/cmd/protoc-gen-omnibuf/main.go).
- Wire specification in [`web/src/pages/spec.astro`](file:///Users/kevin/Projects/omnibuf/web/src/pages/spec.astro).
- **Absences not to fabricate:** No external customer logos, third-party production case studies, or multi-language SDK claims exist today; never invent them.

## Product Principles

1. **Show the bytes, not buzzwords:** Every performance or ergonomic claim should trace directly to a concrete wire-format layout or codegen decision.
2. **Relative headlines, reproducible receipts:** Lead with durable multipliers and percentages in headlines and prose, backed by reproducible benchmark suites in the repository.
3. **Credibility through candor:** Be upfront about what OmniBuf does not do (it is not a zero-unpack pointer graph like FlatBuffers, and it currently ships only for Go).
4. **Drop-in schema ergonomics:** Preserve the `.proto` authoring workflow so adopting OmniBuf never requires learning a new IDL or rewriting schema definitions.
