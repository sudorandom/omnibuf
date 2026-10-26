export interface FormatStat {
  label: string;
  score: number;
}

export interface FormatProfile {
  key: string;
  name: string;
  url?: string;
  verdict?: string;
  omniWinsList?: string[];
  oppWinsList?: string[];
  stats: FormatStat[];
}

export interface BenchmarkRow {
  name: string;
  val: number;
  highlight?: boolean;
  baseline?: boolean;
}

export interface BenchmarkCategory {
  title: string;
  unit: string;
  maxVal: number;
  methodology: string;
  rows: BenchmarkRow[];
}

export const formatProfiles: Record<string, FormatProfile> = {
  omnibuf: {
    key: 'omnibuf',
    name: 'OmniBuf',
    stats: [
      { label: 'Wire Size', score: 94 },
      { label: 'Encode Speed', score: 100 },
      { label: 'Decode Speed', score: 100 },
      { label: 'Field Lookup', score: 78 },
      { label: 'Memory Efficiency', score: 98 },
    ],
  },
  protobuf: {
    key: 'protobuf',
    name: 'Protobuf',
    url: 'https://protobuf.dev/',
    verdict:
      'On our populated 11-field PeerUpdate message, OmniBuf is 7% larger than Protobuf (366 B vs 341 B). Against standard google.golang.org/protobuf, OmniBuf encodes 5.9x faster with generated code (or 2.3x faster on existing .pb.go structs) and decodes 2.3x to 7.3x faster depending on whether you allocate fresh structs or reuse buffers.',
    omniWinsList: [
      'Single-pass encoding avoids walking the message tree first to compute nested byte lengths (123 ns with protoc-gen-omnibuf or 300 ns with struct tags, vs 726 ns for standard Go Protobuf).',
      'Prefix-Varints decode in one width lookup instead of a byte-by-byte continuation-bit loop.',
      'Upfront array element counts let fresh-allocation decode allocate one slice slab instead of growing slices element by element (556 ns and 17 allocs vs 1,280 ns and 25 allocs for standard Go Protobuf).',
      '4-bit wire types and 1-byte collection headers let you inspect raw packets without the .proto file (and optionally include Field #0 field names).',
    ],
    oppWinsList: [
      'Supported across almost every language and RPC stack in production today.',
      'Smaller on the wire for many payloads, especially sparse messages (where Protobuf elides default zero values) and payloads with integers between 2^29 and 2^56 (where Prefix-Varints jump from 4 bytes to 9 bytes).',
      'Does not have to shift payload bytes when a nested container exceeds 16 KB.',
    ],
    stats: [
      { label: 'Wire Size', score: 100 },
      { label: 'Encode Speed', score: 45 },
      { label: 'Decode Speed', score: 42 },
      { label: 'Field Lookup', score: 72 },
      { label: 'Memory Efficiency', score: 65 },
    ],
  },
  flatbuffers: {
    key: 'flatbuffers',
    name: 'FlatBuffers',
    url: 'https://flatbuffers.dev/',
    verdict:
      'OmniBuf is 40% smaller on the wire and up to 6x faster when you are working with normal Go structs or TypeScript objects instead of FlatBuffers builder APIs.',
    omniWinsList: [
      'No VTables or alignment padding, so packets are 40% smaller on the wire.',
      'Encoding from a normal Go struct is up to 6x faster than building a FlatBuffers table.',
      'Unpacking a full message into a native struct is faster, and you can merge packets with append(msgA, msgB...).',
      'Uses standard .proto files or plain structs instead of a separate .fbs schema.',
    ],
    oppWinsList: [
      'Reading a single field out of a large message is faster (13 ns) because VTable offsets point straight at the field.',
      'Fixed-size scalars can be mutated in place inside an existing buffer.',
    ],
    stats: [
      { label: 'Wire Size', score: 57 },
      { label: 'Encode Speed', score: 60 },
      { label: 'Decode Speed', score: 90 },
      { label: 'Field Lookup', score: 98 },
      { label: 'Memory Efficiency', score: 98 },
    ],
  },
  capnproto: {
    key: 'capnproto',
    name: "Cap'n Proto",
    url: 'https://capnproto.org/',
    verdict:
      "OmniBuf packets are 36% smaller than Cap'n Proto messages and much faster when converting to and from normal language structs.",
    omniWinsList: [
      'No 64-bit struct/list pointers or 8-byte word padding, which cuts packet size by 36%.',
      'Much faster when marshaling and unmarshaling normal Go structs or TypeScript objects.',
      'Works with .proto schemas or struct tags and supports raw byte concatenation.',
    ],
    oppWinsList: [
      'Fixed-offset struct layout jumps directly to fields by offset.',
      "Can read and write in place inside a segment arena if you use Cap'n Proto's generated pointer types throughout your code.",
      'Includes its own RPC protocol with promise pipelining.',
    ],
    stats: [
      { label: 'Wire Size', score: 61 },
      { label: 'Encode Speed', score: 56 },
      { label: 'Decode Speed', score: 68 },
      { label: 'Field Lookup', score: 95 },
      { label: 'Memory Efficiency', score: 72 },
    ],
  },
  msgpack: {
    key: 'msgpack',
    name: 'MessagePack',
    url: 'https://msgpack.org/',
    verdict:
      'OmniBuf is 45% smaller than MessagePack (and still 5% smaller even if you turn on Field #0 names) and encodes 7x to 16x faster.',
    omniWinsList: [
      '45% smaller in normal mode, and 5% smaller even when Field #0 field names are included.',
      '16x faster to encode with generated code, or ~7x faster on plain structs with struct tags.',
      'Over 20x faster to decode with generated code, and 4x to 6x faster into tagged structs.',
      'Nested child field names are stored once in Field #0 instead of repeated inside every array element.',
    ],
    oppWinsList: [
      'Can serialize arbitrary map[string]any data when you do not have a struct or field numbers.',
      'Has mature libraries in almost every language today.',
    ],
    stats: [
      { label: 'Wire Size', score: 52 },
      { label: 'Encode Speed', score: 15 },
      { label: 'Decode Speed', score: 18 },
      { label: 'Field Lookup', score: 42 },
      { label: 'Memory Efficiency', score: 25 },
    ],
  },
  json: {
    key: 'json',
    name: 'JSON',
    url: 'https://www.json.org/',
    verdict:
      'OmniBuf is less than half the size of JSON and 18x to 32x faster with generated code (or 5x to 11x faster on plain structs).',
    omniWinsList: [
      '55% smaller on the wire, and still 23% smaller with Field #0 field names turned on.',
      'Encodes 18x faster with generated code, and over 7x faster on plain structs than both encoding/json and encoding/json/v2.',
      'Decodes 23x to 32x faster with generated code, and 5x to 11x faster on plain structs.',
      'Reads a single nested field out of a packet 50x faster since it does not have to scan quotes and braces.',
    ],
    oppWinsList: ['Readable as plain text in curl and browser devtools with no extra tools.'],
    stats: [
      { label: 'Wire Size', score: 42 },
      { label: 'Encode Speed', score: 12 },
      { label: 'Decode Speed', score: 10 },
      { label: 'Field Lookup', score: 10 },
      { label: 'Memory Efficiency', score: 38 },
    ],
  },
};

export const opponentKeys = ['protobuf', 'flatbuffers', 'capnproto', 'msgpack', 'json'] as const;

export const benchmarkCategories: BenchmarkCategory[] = [
  {
    title: 'Full Message Encode Speed',
    unit: 'ns',
    maxVal: 2200,
    methodology:
      "This benchmark measures serializing a native Go PeerUpdate struct into a wire buffer. OmniBuf writes forward in a single pass with 2-byte length slots and branchless Prefix-Varints. Note that FlatBuffers and Cap'n Proto are designed for in-place buffer construction, so their times here reflect copying from a separate Go struct.",
    rows: [
      { name: 'OmniBuf (Generated)', val: 123, highlight: true, baseline: true },
      { name: 'OmniBuf (Go Struct Tags)', val: 300, highlight: true },
      { name: 'FlatBuffers', val: 722 },
      { name: 'Protobuf', val: 726 },
      { name: "Cap'n Proto", val: 1308 },
      { name: 'MessagePack', val: 2025 },
      { name: 'JSON (encoding/json)', val: 2167 },
      { name: 'JSON (encoding/json/v2)', val: 2177 },
    ],
  },
  {
    title: 'Full Message Decode Speed',
    unit: 'ns',
    maxVal: 4500,
    methodology:
      "This benchmark measures unpacking all 11 fields of a packet into a reused Go struct with borrowed strings (or 556 ns when allocating a fresh struct and copying strings on every call). When FlatBuffers and Cap'n Proto are read directly in place without unpacking into a separate struct, their decode step is effectively zero (see the single-field lookup benchmark below).",
    rows: [
      { name: 'OmniBuf (Generated)', val: 176, highlight: true, baseline: true },
      { name: 'FlatBuffers', val: 222 },
      { name: 'OmniBuf (Go Struct Tags)', val: 403, highlight: true },
      { name: "Cap'n Proto", val: 972 },
      { name: 'Protobuf', val: 1280 },
      { name: 'MessagePack', val: 2743 },
      { name: 'JSON (encoding/json/v2)', val: 3519 },
      { name: 'JSON (encoding/json)', val: 4456 },
    ],
  },
  {
    title: 'Read a Single Nested Field',
    unit: 'ns',
    maxVal: 1300,
    methodology:
      "This benchmark is about extracting a string from a nested message without decoding the rest of the message. Although OmniBuf isn't optimized for single-field random access like pointer-table formats (FlatBuffers and Cap'n Proto), it still performs well by hopping over preceding field headers in a single addition per field.",
    rows: [
      { name: "Cap'n Proto", val: 6 },
      { name: 'FlatBuffers', val: 16 },
      { name: 'OmniBuf', val: 103, highlight: true, baseline: true },
      { name: 'MessagePack', val: 766 },
      { name: 'Protobuf', val: 1270 },
    ],
  },
  {
    title: 'Packet Size on the Wire',
    unit: 'B',
    maxVal: 850,
    methodology:
      "This benchmark compares the encoded byte size of the same 11-field message across formats. OmniBuf uses 25 more bytes on the wire than Protobuf (~7% on this message) to make every wire type and container length self-describing, while staying much smaller than FlatBuffers, Cap'n Proto, MessagePack, and JSON.",
    rows: [
      { name: 'Protobuf', val: 341 },
      { name: 'OmniBuf', val: 366, highlight: true, baseline: true },
      { name: "Cap'n Proto", val: 560 },
      { name: 'FlatBuffers', val: 600 },
      { name: 'OmniBuf (With Field #0 Names)', val: 630, highlight: true },
      { name: 'MessagePack', val: 656 },
      { name: 'JSON', val: 803 },
    ],
  },
];
