export interface FormatStat {
  label: string;
  score: number;
}

export interface FormatProfile {
  key: string;
  name: string;
  verdict?: string;
  omniWinsList?: string[];
  oppWinsList?: string[];
  stats: FormatStat[];
}

export interface BenchmarkRow {
  name: string;
  val: number;
  highlight?: boolean;
  color: string;
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
      { label: 'Wire Size', score: 96 },
      { label: 'Encode Speed', score: 99 },
      { label: 'Decode & Skip', score: 94 },
      { label: 'Memory Efficiency', score: 99 },
      { label: 'Simplicity', score: 97 },
    ],
  },
  protobuf: {
    key: 'protobuf',
    name: 'Protobuf',
    verdict:
      'OmniBuf stays within ~6% of Protobuf wire size while encoding up to 6x faster, decoding up to 8x faster, and making every wire type inspectable without a schema.',
    omniWinsList: [
      '2x to 6x faster encoding with single-pass writes instead of measuring nested messages twice.',
      '2x to 8x faster decoding with single-load Prefix-Varints.',
      'Explicit 4-bit wire types and optional Field #0 names remove Wire Type 2 ambiguity.',
      'Faster field skipping while keeping raw message concatenation (append(msgA, msgB...)).',
    ],
    oppWinsList: [
      'Supported everywhere when you need wire compatibility with existing third-party Protobuf services.',
      'Roughly 6% smaller wire payloads because Protobuf uses 3-bit tags and 1-byte lengths on small nested messages.',
    ],
    stats: [
      { label: 'Wire Size', score: 100 },
      { label: 'Encode Speed', score: 48 },
      { label: 'Decode & Skip', score: 38 },
      { label: 'Memory Efficiency', score: 62 },
      { label: 'Simplicity', score: 88 },
    ],
  },
  flatbuffers: {
    key: 'flatbuffers',
    name: 'FlatBuffers',
    verdict:
      'OmniBuf is 40% smaller on the wire, 6x faster to encode, and faster on full message decodes while using standard .proto schemas instead of bottom-up builders.',
    omniWinsList: [
      '40% smaller packets on the wire because there are no VTables or alignment padding.',
      '6x faster encoding with normal top-down writes instead of bottom-up builders.',
      'Faster full message decoding and supports raw byte concatenation.',
      'Works directly with standard .proto schemas and Go structs.',
    ],
    oppWinsList: [
      'Faster single-field random access when reading one field out of a large message via VTable offsets.',
    ],
    stats: [
      { label: 'Wire Size', score: 56 },
      { label: 'Encode Speed', score: 35 },
      { label: 'Decode & Skip', score: 96 },
      { label: 'Memory Efficiency', score: 92 },
      { label: 'Simplicity', score: 42 },
    ],
  },
  capnproto: {
    key: 'capnproto',
    name: "Cap'n Proto",
    verdict:
      'On framed network packets, OmniBuf is 36% smaller, 10x faster to encode, and over 6x faster to decode without segment pointer bounds checking.',
    omniWinsList: [
      '36% smaller network packets without 64-bit pointers or 8-byte word padding.',
      '10x faster encoding with zero heap allocations.',
      'Over 6x faster full decoding and nearly 3x faster single-field reads on framed network packets.',
      'Uses standard .proto schemas and supports raw message concatenation.',
    ],
    oppWinsList: [
      'Includes a built-in RPC system with promise pipelining.',
      'Fixed-offset struct layout jumps directly to fields in shared memory without scanning preceding field headers.',
    ],
    stats: [
      { label: 'Wire Size', score: 60 },
      { label: 'Encode Speed', score: 25 },
      { label: 'Decode & Skip', score: 82 },
      { label: 'Memory Efficiency', score: 65 },
      { label: 'Simplicity', score: 45 },
    ],
  },
  msgpack: {
    key: 'msgpack',
    name: 'MessagePack',
    verdict:
      'OmniBuf is 45% smaller than MessagePack in standard mode, and still smaller even with Field #0 inline field names enabled while encoding over 6x faster.',
    omniWinsList: [
      '45% smaller packets in standard .proto mode, and 5% smaller even with Field #0 inline names enabled.',
      '16x faster encoding (or over 6x faster with Field #0 inline field names included).',
      'Over 20x faster full decoding and nearly 8x faster single-field skipping.',
      'Stores nested child field names once in Field #0 instead of repeating key strings inside every array element.',
    ],
    oppWinsList: [
      'Good fit for schemaless maps with arbitrary keys when you do not have a .proto schema.',
    ],
    stats: [
      { label: 'Wire Size', score: 52 },
      { label: 'Encode Speed', score: 20 },
      { label: 'Decode & Skip', score: 22 },
      { label: 'Memory Efficiency', score: 25 },
      { label: 'Simplicity', score: 90 },
    ],
  },
  json: {
    key: 'json',
    name: 'JSON',
    verdict:
      'OmniBuf is less than half the size of JSON and 18x to 32x faster, while still letting you use standard ProtoJSON for plain-text debugging from the same .proto file.',
    omniWinsList: [
      '55% smaller on the wire (and 23% smaller even when Field #0 field names are embedded).',
      '18x faster encoding and 32x faster decoding.',
      '50x faster single-field extraction without scanning text brackets or quotes.',
      'Compatible with official ProtoJSON so you can still accept or emit plain text JSON from the same .proto schema.',
    ],
    oppWinsList: [
      'Readable as plain text in browser devtools and curl when no CLI tool is installed.',
    ],
    stats: [
      { label: 'Wire Size', score: 42 },
      { label: 'Encode Speed', score: 15 },
      { label: 'Decode & Skip', score: 10 },
      { label: 'Memory Efficiency', score: 30 },
      { label: 'Simplicity', score: 98 },
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
      'Serializes a populated 11-field PeerUpdate struct (with a nested Location message and 4 repeated Neighbor messages) into a reused byte buffer or FlatBuffers builder to measure encoding throughput without allocator noise.',
    rows: [
      { name: 'OmniBuf', val: 94, highlight: true, color: '#FF4F18' },
      { name: 'Protobuf', val: 561, color: '#94A3B8' },
      { name: 'FlatBuffers', val: 562, color: '#94A3B8' },
      { name: "Cap'n Proto", val: 1016, color: '#CBD5E1' },
      { name: 'MessagePack', val: 1552, color: '#CBD5E1' },
      { name: 'JSON', val: 1720, color: '#CBD5E1' },
    ],
  },
  {
    title: 'Full Message Decode Speed',
    unit: 'ns',
    maxVal: 4000,
    methodology:
      "Deserializes the encoded PeerUpdate packet back into a reusable Go struct with zero-copy string slices. For FlatBuffers and Cap'n Proto, all 11 fields and nested structures are read out of the root table into the target struct so every format performs equivalent work.",
    rows: [
      { name: 'OmniBuf', val: 120, highlight: true, color: '#FF4F18' },
      { name: 'FlatBuffers', val: 170, color: '#94A3B8' },
      { name: "Cap'n Proto", val: 772, color: '#CBD5E1' },
      { name: 'Protobuf', val: 960, color: '#94A3B8' },
      { name: 'MessagePack', val: 2496, color: '#CBD5E1' },
      { name: 'JSON', val: 3850, color: '#CBD5E1' },
    ],
  },
  {
    title: 'Read a Single Nested Field (peer.location.city)',
    unit: 'ns',
    maxVal: 1050,
    methodology:
      "Extracts a single nested string (location.city) from a raw PeerUpdate packet without decoding the rest of the message. OmniBuf skips preceding fields via header hopping, FlatBuffers and unframed Cap'n Proto follow offset pointers directly, framed Cap'n Proto parses the segment frame header first, and Protobuf unmarshals the message.",
    rows: [
      { name: "Cap'n Proto (Unframed Shared Memory)", val: 5, color: '#94A3B8' },
      { name: 'FlatBuffers', val: 13, color: '#94A3B8' },
      { name: 'OmniBuf', val: 76, highlight: true, color: '#FF4F18' },
      { name: "Cap'n Proto (Framed Packet)", val: 221, color: '#CBD5E1' },
      { name: 'MessagePack', val: 590, color: '#CBD5E1' },
      { name: 'Protobuf', val: 983, color: '#94A3B8' },
    ],
  },
  {
    title: 'Packet Size on the Wire',
    unit: 'B',
    maxVal: 850,
    methodology:
      'Measures the exact serialized byte length of the identical 11-field PeerUpdate payload across all formats, including OmniBuf both in standard .proto mode and with Field #0 inline field names enabled.',
    rows: [
      { name: 'Protobuf', val: 341, color: '#94A3B8' },
      { name: 'OmniBuf', val: 362, highlight: true, color: '#FF4F18' },
      { name: "Cap'n Proto", val: 560, color: '#CBD5E1' },
      { name: 'FlatBuffers', val: 600, color: '#CBD5E1' },
      { name: 'OmniBuf (With Field #0 Names)', val: 622, highlight: true, color: '#FF4F18' },
      { name: 'MessagePack', val: 656, color: '#CBD5E1' },
      { name: 'JSON', val: 803, color: '#CBD5E1' },
    ],
  },
];
