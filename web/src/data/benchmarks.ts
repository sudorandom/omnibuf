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
      'OmniBuf stays within ~6% of Protobuf wire size while encoding up to 6x faster, decoding up to 8x faster, and making every wire type inspectable without a schema.',
    omniWinsList: [
      'OmniBuf encodes 2x to 6x faster because it writes in a single pass instead of measuring nested messages twice.',
      'OmniBuf decodes 2x to 8x faster because it reads integer byte widths from the first byte instead of looping byte-by-byte.',
      'Explicit 4-bit wire types and optional Field #0 names remove Wire Type 2 ambiguity.',
      'OmniBuf skips fields in one step while keeping raw message concatenation (append(msgA, msgB...)).',
    ],
    oppWinsList: [
      'Protobuf is supported everywhere when you need wire compatibility with existing third-party services.',
      'Protobuf payloads are roughly 6% smaller because Protobuf uses 3-bit tags and 1-byte lengths on small nested messages.',
    ],
    stats: [
      { label: 'Wire Size', score: 100 },
      { label: 'Encode Speed', score: 46 },
      { label: 'Decode Speed', score: 40 },
      { label: 'Field Lookup', score: 32 },
      { label: 'Memory Efficiency', score: 64 },
    ],
  },
  flatbuffers: {
    key: 'flatbuffers',
    name: 'FlatBuffers',
    url: 'https://flatbuffers.dev/',
    verdict:
      'OmniBuf is 40% smaller on the wire, 6x faster to encode, and faster on full message decodes while using standard .proto schemas instead of bottom-up builders.',
    omniWinsList: [
      'OmniBuf packets are 40% smaller on the wire because there are no VTables or alignment padding.',
      'OmniBuf encodes 6x faster with normal top-down writes instead of bottom-up builders.',
      'OmniBuf decodes full messages faster and supports raw byte concatenation.',
      'OmniBuf works directly with standard .proto schemas and plain structs.',
    ],
    oppWinsList: [
      'FlatBuffers has faster single-field random access when reading one field out of a large message via VTable offsets.',
    ],
    stats: [
      { label: 'Wire Size', score: 57 },
      { label: 'Encode Speed', score: 46 },
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
      'On framed network packets, OmniBuf is 36% smaller, 10x faster to encode, and over 6x faster to decode without segment pointer bounds checking.',
    omniWinsList: [
      'OmniBuf network packets are 36% smaller because they do not carry 64-bit pointers or 8-byte word padding.',
      'OmniBuf encodes 10x faster with zero heap allocations.',
      'OmniBuf is over 6x faster on full decoding and nearly 3x faster on single-field reads over framed network packets.',
      'OmniBuf uses standard .proto schemas and supports raw message concatenation.',
    ],
    oppWinsList: [
      "Cap'n Proto includes a built-in RPC system with promise pipelining.",
      "Cap'n Proto's fixed-offset struct layout jumps directly to fields in shared memory without scanning preceding field headers.",
    ],
    stats: [
      { label: 'Wire Size', score: 61 },
      { label: 'Encode Speed', score: 28 },
      { label: 'Decode Speed', score: 46 },
      { label: 'Field Lookup', score: 85 },
      { label: 'Memory Efficiency', score: 58 },
    ],
  },
  msgpack: {
    key: 'msgpack',
    name: 'MessagePack',
    url: 'https://msgpack.org/',
    verdict:
      'OmniBuf is 45% smaller than MessagePack (and still smaller even with Field #0 inline field names enabled), encoding 16x faster with generated code or ~7x faster using plain struct tags without a .proto file.',
    omniWinsList: [
      'OmniBuf packets are 45% smaller in standard mode, and 5% smaller even with Field #0 inline names enabled.',
      'OmniBuf encodes 16x faster with generated code, and ~7x faster on plain structs using omnibuf struct tags.',
      'OmniBuf decodes full messages over 20x faster with generated code, and 4x to 6x faster when decoding directly into tagged structs.',
      'OmniBuf stores nested child field names once in Field #0 instead of repeating key strings inside every array element.',
    ],
    oppWinsList: [
      'MessagePack can encode untyped dynamic maps (such as map[string]any) when you do not have a struct or assigned field numbers.',
      'MessagePack has broad multi-language library support when interoperating with languages beyond Go and TypeScript/JavaScript today.',
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
      'OmniBuf is less than half the size of JSON and 18x to 32x faster with generated code (or 5x to 11x faster using plain struct tags against both encoding/json and encoding/json/v2), while still supporting standard ProtoJSON.',
    omniWinsList: [
      'OmniBuf is 55% smaller on the wire, and 23% smaller even when Field #0 field names are embedded.',
      'OmniBuf encodes 18x faster with generated code, and over 7x faster on plain structs with omnibuf tags than both encoding/json and encoding/json/v2.',
      'OmniBuf decodes 23x to 32x faster with generated code, and 5x to 11x faster on plain structs than both encoding/json and encoding/json/v2.',
      'OmniBuf extracts a single nested field 50x faster without scanning text brackets or quotes, while remaining compatible with official ProtoJSON.',
    ],
    oppWinsList: [
      'JSON is readable as plain text in browser devtools and curl when no CLI tool is installed.',
    ],
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
    maxVal: 1800,
    methodology:
      'This benchmark measures how long it takes to serialize an 11-field message into a reused byte buffer. OmniBuf performs extremely well with encoding because it writes forward in a single pass, reserves 2-byte length slots instead of pre-calculating sub-message sizes, and encodes integers without byte-by-byte varint loops.',
    rows: [
      { name: 'OmniBuf (Generated)', val: 94, highlight: true, baseline: true },
      { name: 'OmniBuf (Go Struct Tags)', val: 232, highlight: true },
      { name: 'Protobuf', val: 561 },
      { name: 'FlatBuffers', val: 562 },
      { name: "Cap'n Proto", val: 1016 },
      { name: 'MessagePack', val: 1590 },
      { name: 'JSON (encoding/json)', val: 1705 },
      { name: 'JSON (encoding/json/v2)', val: 1753 },
    ],
  },
  {
    title: 'Full Message Decode Speed',
    unit: 'ns',
    maxVal: 4000,
    methodology:
      'This benchmark measures how long it takes to decode the full 11-field message back into a struct. OmniBuf performs extremely well with decoding because the trailing zeros of the first byte reveal the exact integer width for a single-step read, booleans require zero value bytes, and arrays are decoded from a single length-prefixed block.',
    rows: [
      { name: 'OmniBuf (Generated)', val: 120, highlight: true, baseline: true },
      { name: 'FlatBuffers', val: 170 },
      { name: 'OmniBuf (Go Struct Tags)', val: 343, highlight: true },
      { name: "Cap'n Proto", val: 772 },
      { name: 'Protobuf', val: 960 },
      { name: 'MessagePack', val: 2185 },
      { name: 'JSON (encoding/json/v2)', val: 2778 },
      { name: 'JSON (encoding/json)', val: 3891 },
    ],
  },
  {
    title: 'Read a Single Nested Field',
    unit: 'ns',
    maxVal: 1050,
    methodology:
      "This benchmark is about extracting a string from a nested message without decoding the rest of the message. Although OmniBuf isn't optimized for single-field random access like pointer-table formats (FlatBuffers and Cap'n Proto), it still performs quite well by hopping over preceding field headers in a single addition per field.",
    rows: [
      { name: "Cap'n Proto (Unframed Shared Memory)", val: 5 },
      { name: 'FlatBuffers', val: 13 },
      { name: 'OmniBuf', val: 76, highlight: true, baseline: true },
      { name: "Cap'n Proto (Framed Packet)", val: 221 },
      { name: 'MessagePack', val: 590 },
      { name: 'Protobuf', val: 983 },
    ],
  },
  {
    title: 'Packet Size on the Wire',
    unit: 'B',
    maxVal: 850,
    methodology:
      "This benchmark compares the encoded byte size of the same 11-field message across formats. OmniBuf uses only slightly more bytes on the wire than Protobuf (~6% on this message) to make every wire type and container length self-describing, while staying much smaller than FlatBuffers, Cap'n Proto, MessagePack, and JSON.",
    rows: [
      { name: 'Protobuf', val: 341 },
      { name: 'OmniBuf', val: 362, highlight: true, baseline: true },
      { name: "Cap'n Proto", val: 560 },
      { name: 'FlatBuffers', val: 600 },
      { name: 'OmniBuf (With Field #0 Names)', val: 622, highlight: true },
      { name: 'MessagePack', val: 656 },
      { name: 'JSON', val: 803 },
    ],
  },
];
