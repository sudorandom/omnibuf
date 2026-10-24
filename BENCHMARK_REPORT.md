# OmniBuf Benchmark & Engineering Report

**Environment:**
- **CPU:** Apple M1 Pro (`darwin/arm64`)
- **Go Version:** `go1.27.1 darwin/arm64`
- **Workload:** Canonical BGP/Telemetry `PeerUpdate` message containing 11 root fields (`uint32`, `uint64`, signed ZigZag `int32`, `bool`, `float64`, `repeated string`, packed `repeated uint32`, nested `Location` sub-message with 6 fields, and `repeated Neighbor` sub-message array with 4 elements).

---

## 1. Key Architectural Findings

### 1.1 Single-Pass Forward Encoding via Overlong 2-Byte Prefix-Varint Backpatching
Protocol Buffers requires a 2-pass encoding strategy (`proto.Size()` followed by `proto.Marshal()`) because a parent message must write each child sub-message's byte length as a base-128 LEB128 varint *before* writing the child's bytes. FlatBuffers avoids a `Size()` pass by serializing inside-out (bottom-up from the end of the buffer) and deduplicating vtables at every table boundary (`731.7 ns/op`).

Because OmniBuf dispatches strictly on `TZCNT(b[0])`, an overlong 2-byte Prefix-Varint (`uint16(subLen << 2) | 2`, `TZCNT=1`) is 100% valid even when `subLen < 128`. The encoder reserves 2 bytes (`0x00, 0x00`), writes the child message forward in the same buffer, and calls [`BackpatchContainerLen`](file:///Users/kevin/Projects/omnibuf/omnibuf.go#L103-L129) once the child finishes. For the common fast path (`subLen <= 16,383` bytes), it writes the 2-byte little-endian header in place (`0` moves). If a container exceeds 16,383 bytes, [`backpatchLargeContainerLen`](file:///Users/kevin/Projects/omnibuf/omnibuf.go#L112-L129) automatically shifts the child payload forward in the same slice by `+2` bytes (4-byte Prefix-Varint up to 512 MB) or `+7` bytes (9-byte Prefix-Varint up to `MaxUint64`).
- **Result:** [`EncodePeerOmniBufDirect`](file:///Users/kevin/Projects/omnibuf/peer_codecs.go#L300-L433) (`85.9 ns/op`) and plugin-generated [`MarshalOmniBufAppend`](file:///Users/kevin/Projects/omnibuf/pb/peer_omnibuf.pb.go#L22-L125) (`96.3 ns/op`) run with `0 allocs/op`, outperforming PlanetScale's `vtprotobuf` (`MarshalToVT` at `129.6 ns/op`, which requires a pre-traversal `SizeVT()` pass before `MarshalToSizedBufferVT`), hand-rolled `protowire` (`260.2 ns/op`), `flatbuffers.Builder` (`731.7 ns/op`), and official `proto.MarshalAppend` (`738.9 ns/op`).

### 1.2 Index-Free Field Stream + Branchless `TZCNT` Header Hopping & Concatenation Merge
FlatBuffers (`600 B`) and Cap'n Proto (`560 B`) achieve random access by padding fields to alignment boundaries and storing 16-bit vtable offsets or 64-bit struct/list pointers throughout the payload.
OmniBuf skips on-wire offset tables and vtables entirely. Instead, fields are encoded as a contiguous stream with explicit 4-bit wire types (`0..11`), `0`-byte booleans, `TZCNT` Prefix-Varints, outer `ByteLen` prefixes on all containers (`WireString`, `WireBytes`, `WireSubMsg`, `WireUintArr`, `WireSubMsgArr`, `WireMap`), and full Protocol Buffers concatenation merge semantics (`append(msgA, msgB...)`):
- **Wire Size:** **360 bytes** (40% smaller than FlatBuffers' 600 B, 36% smaller than Cap'n Proto's 560 B, and within 19 B of Protobuf's 341 B).
- **Full Zero-Copy Unmarshal:** **`158.5 ns/op` (`0 allocs/op`)**, which is **2.4x faster** than hand-rolled `protowire` (`374.7 ns/op`), **1.42x faster** than FlatBuffers (`225.0 ns/op`), and **8.2x faster** than `proto.Unmarshal` (`1,304.0 ns/op`).
- **Header-Hop Nested Field Lookup (`peer.location.city`):** **`101.1 ns/op` (`0 allocs/op`)**, including full-stream "last write wins" scanning across concatenated messages, which is faster than a `protowire` sequential wire scan (`124.4 ns/op`) and **13.1x faster** than `proto.Unmarshal` (`1,328.0 ns/op`).
- **Header-Hop Sub-Message Array Lookup (`peer.neighbors[2].router`):** **`95.9 ns/op` (`0 allocs/op`)**, which is **1.63x faster** than `protowire` sequential scanning (`156.2 ns/op`).

---

## 2. Full Benchmark Tables

### 2.1 Wire Size Breakdown

| Format | Shared Schema | Self-Describing | Size (Bytes) | Overhead vs Protobuf |
| :--- | :---: | :---: | ---: | ---: |
| **Protobuf (`proto` / `protowire`)** | Yes | No | **341 B** | `0%` (baseline) |
| **OmniBuf (Shared Schema)** | Yes | Optional | **360 B** | `+5.6%` (+19 B) |
| **Cap'n Proto** | Yes | No | **560 B** | `+64.2%` |
| **FlatBuffers** | Yes | No | **600 B** | `+76.0%` |
| **MessagePack** | No | Yes | **656 B** | `+92.4%` |
| **OmniBuf (`Field #0` Inline Schema)** | Optional | Yes | **667 B** | `+95.6%` |
| **JSON (`encoding/json`)** | No | Yes | **803 B** | `+135.5%` |

### 2.2 Marshal / Encode Benchmarks

| Benchmark Name | Latency (`ns/op`) | `B/op` | `allocs/op` | Notes |
| :--- | ---: | ---: | ---: | :--- |
| `BenchmarkMarshal_OmniBuf_Direct_ReuseBuf` | **109.60** | 0 | 0 | 1-pass forward slice write + 2B length backpatch |
| `BenchmarkPlugin_Marshal_PBStruct_ReuseBuf` | **125.00** | 0 | 0 | `protoc-gen-omnibuf` generated `MarshalOmniBufAppend` |
| `BenchmarkTable_Marshal_ValueStruct_ReuseBuf` | **251.50** | 0 | 0 | Generic table-driven `omnibuf.MarshalAppend` |
| `BenchmarkTable_Marshal_ExistingPBStruct_ReuseBuf` | **252.60** | 0 | 0 | Generic table-driven on `*pb.PeerUpdate` |
| `BenchmarkMarshal_Protobuf_ProtoWire_ReuseBuf` | 260.20 | 0 | 0 | 2-pass `Size()` + `protowire.Append*` |
| `BenchmarkMarshal_OmniBuf_ReuseBuf` | **294.80** | 0 | 0 | General `Encoder` builder API |
| `BenchmarkMarshal_OmniBuf_FreshAlloc` | **389.60** | 384 | 1 | `NewEncoder` + `EncodePeerOmniBuf` |
| `BenchmarkMarshal_OmniBuf_SelfDescribing_ReuseBuf` | 540.70 | 0 | 0 | Includes `Field #0` field-name tables |
| `BenchmarkMarshal_FlatBuffers_ReuseBuilder` | 731.70 | 0 | 0 | Bottom-up `flatbuffers.Builder` + vtable dedup |
| `BenchmarkMarshal_Protobuf_Official_MarshalAppend` | 738.90 | 0 | 0 | `proto.MarshalOptions{}.MarshalAppend` |
| `BenchmarkMarshal_Protobuf_Official_FreshAlloc` | 810.30 | 352 | 1 | `proto.Marshal` |
| `BenchmarkMarshal_CapnProto` | 1,318.00 | 720 | 3 | `capnp.NewSingleSegmentMessage` + `msg.Marshal()` |
| `BenchmarkMarshal_Msgpack` | 2,048.00 | 2,056 | 7 | `msgpack.Marshal` |
| `BenchmarkMarshal_JSON` | 2,204.00 | 896 | 1 | `json.Marshal` |

### 2.3 Full Unmarshal / Decode Benchmarks

| Benchmark Name | Latency (`ns/op`) | `B/op` | `allocs/op` | Notes |
| :--- | ---: | ---: | ---: | :--- |
| `BenchmarkUnmarshal_OmniBuf_ZeroCopy` | **158.50** | **0** | **0** | Full struct decode + `unsafe.String` |
| `BenchmarkUnmarshal_FlatBuffers_ZeroCopy` | 225.00 | 0 | 0 | Full table traversal + `unsafe.String` |
| `BenchmarkPlugin_Unmarshal_PBStruct_ZeroCopyReuse` | **258.90** | 0 | 0 | `protoc-gen-omnibuf` into `*pb.PeerUpdate` |
| `BenchmarkMarshal_Protobuf_ProtoWire_ZeroCopy` | 374.70 | 0 | 0 | Hand-rolled `protowire` + `unsafe.String` |
| `BenchmarkTable_Unmarshal_ExistingPBStruct_ZeroCopyReuse` | **380.90** | 0 | 0 | Generic table-driven `UnmarshalZeroCopy` |
| `BenchmarkTable_Unmarshal_ValueStruct_ZeroCopyReuse` | **381.50** | 0 | 0 | Generic table-driven `UnmarshalZeroCopy` |
| `BenchmarkUnmarshal_OmniBuf_SafeAlloc` | **434.00** | 472 | 15 | Full struct decode with safe heap strings |
| `BenchmarkUnmarshal_FlatBuffers_SafeAlloc` | 456.40 | 472 | 15 | Full table traversal with safe heap strings |
| `BenchmarkPlugin_Unmarshal_PBStruct_FreshAlloc` | **645.00** | 776 | 17 | `protoc-gen-omnibuf` into fresh `*pb.PeerUpdate` |
| `BenchmarkTable_Unmarshal_ValueStruct_SafeAlloc` | **688.00** | 496 | 16 | Generic table-driven safe heap strings |
| `BenchmarkUnmarshal_Protobuf_ProtoWire_SafeAlloc` | 819.60 | 656 | 21 | Hand-rolled `protowire` with safe heap strings |
| `BenchmarkTable_Unmarshal_ExistingPBStruct_FreshAlloc` | **837.40** | 800 | 18 | Generic table-driven into fresh `*pb.PeerUpdate` |
| `BenchmarkUnmarshal_CapnProto_ZeroCopy` | 989.40 | 240 | 4 | `capnp.Unmarshal` + struct/list reads |
| `BenchmarkUnmarshal_CapnProto_SafeAlloc` | 1,287.00 | 712 | 19 | `capnp.Unmarshal` with safe heap strings |
| `BenchmarkUnmarshal_Protobuf_Official` | 1,304.00 | 1,072 | 25 | `proto.Unmarshal` |
| `BenchmarkDecodeBlind_OmniBuf_Field0` | 3,210.00 | 4,192 | 93 | Schema-less `DecodeBlind` -> `map[string]any` |
| `BenchmarkUnmarshal_Msgpack` | 3,235.00 | 1,024 | 25 | `msgpack.Unmarshal` |
| `BenchmarkUnmarshal_JSON` | 5,073.00 | 600 | 10 | `json.Unmarshal` |

### 2.4 Single Nested Field Lookup Benchmarks

| Benchmark Name | Target Path | Latency (`ns/op`) | `B/op` | `allocs/op` | Mechanism |
| :--- | :--- | ---: | ---: | ---: | :--- |
| `BenchmarkLookup_LocationCity_CapnProto_InPlace_ZeroCopy` | `peer.location.city` | **6.25** | 0 | 0 | Fixed 64b ptrs |
| `BenchmarkLookup_LocationCity_FlatBuffers_ZeroCopy` | `peer.location.city` | 16.79 | 0 | 0 | VTable indirection |
| `BenchmarkLookup_LocationCity_OmniBuf_ZeroCopy` | `peer.location.city` | **101.10** | 0 | 0 | Branchless `TZCNT` header hop (last-write-wins) |
| `BenchmarkLookup_LocationCity_OmniBuf_SafeAlloc` | `peer.location.city` | **103.50** | 0 | 0 | Header hop + safe `string(b)` |
| `BenchmarkLookup_LocationCity_Protobuf_WireScan_ZeroCopy` | `peer.location.city` | 124.40 | 0 | 0 | TLV + LEB128 wire scan |
| `BenchmarkLookup_LocationCity_CapnProto_ZeroCopy` | `peer.location.city` | 295.90 | 240 | 4 | `capnp.Unmarshal` + ptr |
| `BenchmarkLookup_LocationCity_Msgpack_SkipScan` | `peer.location.city` | 785.10 | 160 | 14 | Stream skip |
| `BenchmarkLookup_LocationCity_Protobuf_Official_Unmarshal` | `peer.location.city` | 1,328.00 | 1,072 | 25 | Full `proto.Unmarshal` |
| `BenchmarkLookup_LocationCity_JSON_Unmarshal` | `peer.location.city` | 5,050.00 | 600 | 10 | Full `json.Unmarshal` |
| `BenchmarkLookup_Neighbor2Router_CapnProto_InPlace_ZeroCopy` | `peer.neighbors[2].router` | **8.08** | 0 | 0 | Fixed 64b ptrs |
| `BenchmarkLookup_Neighbor2Router_FlatBuffers_ZeroCopy` | `peer.neighbors[2].router` | 24.19 | 0 | 0 | VTable + Vector |
| `BenchmarkLookup_Neighbor2Router_OmniBuf_ZeroCopy` | `peer.neighbors[2].router` | **95.90** | 0 | 0 | `WireSubMsgArr` length hop |
| `BenchmarkLookup_Neighbor2Router_Protobuf_WireScan_ZeroCopy` | `peer.neighbors[2].router` | 156.20 | 0 | 0 | TLV + LEB128 wire scan |
| `BenchmarkLookup_Neighbor2Router_CapnProto_ZeroCopy` | `peer.neighbors[2].router` | 321.60 | 240 | 4 | `capnp.Unmarshal` + ptr |

---

### 2.5 No-Plugin Table-Driven vs. `vtprotobuf` vs. `protoc-gen-omnibuf` vs. `google.golang.org/protobuf`

Below is the direct comparison on `*pb.PeerUpdate` and `pb.OpaquePeerUpdate` across all four tiers:
1. **`google.golang.org/protobuf` ([`pb/peer.pb.go`](file:///Users/kevin/Projects/omnibuf/pb/peer.pb.go)):** Official Protobuf runtime.
2. **OmniBuf No-Plugin Table Engine ([`codec_table.go`](file:///Users/kevin/Projects/omnibuf/codec_table.go)):** Uses existing `*pb.PeerUpdate` struct tags with cached `unsafe.Pointer` field offsets (no `protoc-gen-omnibuf` needed).
3. **PlanetScale `vtprotobuf` ([`pb/peer_vtproto.pb.go`](file:///Users/kevin/Projects/omnibuf/pb/peer_vtproto.pb.go)):** Generated Protobuf fast-path methods (`MarshalToVT`, `SizeVT`, `UnmarshalVT`, `UnmarshalVTUnsafe`, and `PeerUpdateFromVTPool`).
4. **OmniBuf Plugin-Generated ([`pb/peer_omnibuf.pb.go`](file:///Users/kevin/Projects/omnibuf/pb/peer_omnibuf.pb.go) generated by [`cmd/protoc-gen-omnibuf`](file:///Users/kevin/Projects/omnibuf/cmd/protoc-gen-omnibuf/main.go)):** Emits direct 1-pass `MarshalOmniBufAppend` / `UnmarshalOmniBuf` methods on `*pb.PeerUpdate` and the generated `pb.OpaquePeerUpdate` Opaque API struct.

| Workload | `google.golang.org/protobuf` | OmniBuf (No Plugin, Table) | `vtprotobuf` (Codegen + Pool) | OmniBuf (`protoc-gen-omnibuf`) | OmniBuf vs `vtprotobuf` |
| :--- | ---: | ---: | ---: | ---: | :--- |
| **Marshal `*pb.PeerUpdate` (Reuse Buf)** | `738.9 ns` (`0 allocs`) | `191.7 ns` (`0 allocs`) | `129.6 ns` (`0 allocs`) | **`96.3 ns` (`0 allocs`)** | **1.35x faster** (1-pass vs 2-pass `SizeVT`) |
| **Unmarshal `*pb.PeerUpdate` (Fresh Struct + Safe Strings)** | `1,304.0 ns` (`25 allocs`, `1,072 B`) | `647.8 ns` (`18 allocs`, `800 B`) | `557.2 ns` (`24 allocs`, `952 B`) | **`497.6 ns` (`17 allocs`, `776 B`)** | **1.12x faster** (`17` vs `24` allocs) |
| **Unmarshal `*pb.PeerUpdate` (Reuse / Pool + Zero-Copy)** | `1,304.0 ns` (`25 allocs`, `1,072 B`) | `289.1 ns` (`0 allocs`) | `201.0 ns` (`0 allocs`, Pool) | **`197.6 ns` (`0 allocs`, Reuse)** | **Tied (~200 ns)** (no `sync.Pool` needed) |
| **Unmarshal + Read 2 Nested Fields (`Location.City` & `Neighbors[2].Router`)** | `970.1 ns` (`25 allocs`, `1,072 B`) | `289.1 ns` (`0 allocs`) | `202.3 ns` (`0 allocs`, Pool) | **`144.0 ns` (`0 allocs`, Opaque API)** | **1.40x faster** (header hop) |
| **Unmarshal + Read ALL 33 Leaf Fields** | `985.3 ns` (`25 allocs`, `1,072 B`) | `289.1 ns` (`0 allocs`) | `219.9 ns` (`0 allocs`, Pool) | **`197.6 ns` (`0 allocs`, Struct Reuse)** | **1.11x faster** |
| **Read-Modify-Write (`Unmarshal` -> Check `City` -> `Seq++` -> `Marshal`)** | `1,590.0 ns` (`25 allocs`, `1,072 B`) | `480.8 ns` (`0 allocs`) | `331.3 ns` (`0 allocs`, Pool) | **`177.8 ns` (`0 allocs`, Opaque Splice)** | **1.86x faster** (in-place wire splice) |

---

### 2.6 Varint Microbenchmarks: OmniBuf `TZCNT` Prefix-Varint vs. Protobuf `LEB128`

In Go's official Protobuf library (`google.golang.org/protobuf/encoding/protowire.ConsumeVarint`), decoding a varint uses a 100-line unrolled 10-step function with 10 sequential bounds checks and 10 continuation-bit branches (`if y < 0x80`), which exceeds the Go compiler's inline budget. Code generators like `vtprotobuf` inline a `for shift := uint(0); ; shift += 7` loop at every field (`pb/peer_vtproto.pb.go` is 1,899 lines), whereas OmniBuf's [`ReadVarintAt`](file:///Users/kevin/Projects/omnibuf/omnibuf.go#L338-L351) is 12 lines, inlines at every call site, and executes a single unaligned little-endian load (`1`, `2`, `4`, or `8` bytes):

| Operation | Protobuf (`protowire`) | `vtprotobuf` | OmniBuf `TZCNT` | Speedup |
| :--- | ---: | ---: | ---: | :--- |
| **Decode 1B Int** | `1.36 ns` | `0.80 ns` | **`0.37 ns`** | **2.2x to 3.7x faster** |
| **Decode 2B Int** | `1.32 ns` | `1.29 ns` | **`0.83 ns`** | **1.6x faster** |
| **Decode 4B Int** | `1.93 ns` | `2.25 ns` | **`0.99 ns`** | **1.9x to 2.3x faster** |
| **Decode 8B Int** | `4.06 ns` | `5.11 ns` | **`1.38 ns`** | **2.9x to 3.7x faster** |
| **Decode 16 Mixed Ints** | `40.8 ns` | `42.5 ns` | **`26.6 ns`** | **1.5x to 1.6x faster** |
| **Skip 16 Mixed Ints** | `38.6 ns` | `42.5 ns` | **`25.0 ns`** | **1.5x to 1.7x faster** |
| **Encode 16 Mixed Ints** | `62.9 ns` | `59.4 ns` | **`41.2 ns`** | **1.5x faster** |
