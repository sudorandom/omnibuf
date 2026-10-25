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
      '2x to 6x faster encoding by writing in one pass instead of measuring nested messages twice.',
      '2x to 8x faster decoding by reading integer byte widths from the first byte instead of looping byte-by-byte.',
      'Explicit 4-bit wire types and optional Field #0 names remove Wire Type 2 ambiguity.',
      'Faster field skipping while keeping raw message concatenation (append(msgA, msgB...)).',
    ],
    oppWinsList: [
      'Supported everywhere when you need wire compatibility with existing third-party Protobuf services.',
      'Roughly 6% smaller wire payloads because Protobuf uses 3-bit tags and 1-byte lengths on small nested messages.',
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
      '40% smaller packets on the wire because there are no VTables or alignment padding.',
      '6x faster encoding with normal top-down writes instead of bottom-up builders.',
      'Faster full message decoding and supports raw byte concatenation.',
      'Works directly with standard .proto schemas and Go structs.',
    ],
    oppWinsList: [
      'Faster single-field random access when reading one field out of a large message via VTable offsets.',
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
      'Serializes a populated 11-field message (scalars, strings, packed int arrays, a 6-field nested sub-message, and 4 repeated sub-messages) into a reused byte buffer or FlatBuffers builder.',
    rows: [
      { name: 'OmniBuf', val: 94, highlight: true, color: '#FF4F18' },
      { name: 'Protobuf', val: 561, color: '#78716C' },
      { name: 'FlatBuffers', val: 562, color: '#78716C' },
      { name: "Cap'n Proto", val: 1016, color: '#A8A29E' },
      { name: 'MessagePack', val: 1552, color: '#A8A29E' },
      { name: 'JSON', val: 1720, color: '#A8A29E' },
    ],
  },
  {
    title: 'Full Message Decode Speed',
    unit: 'ns',
    maxVal: 4000,
    methodology:
      "Deserializes the encoded packet back into a reusable Go struct with zero-copy string slices. For FlatBuffers and Cap'n Proto, all 11 fields and nested structures are read out of the root table into the target struct so every format performs equivalent work.",
    rows: [
      { name: 'OmniBuf', val: 120, highlight: true, color: '#FF4F18' },
      { name: 'FlatBuffers', val: 170, color: '#78716C' },
      { name: "Cap'n Proto", val: 772, color: '#A8A29E' },
      { name: 'Protobuf', val: 960, color: '#78716C' },
      { name: 'MessagePack', val: 2496, color: '#A8A29E' },
      { name: 'JSON', val: 3850, color: '#A8A29E' },
    ],
  },
  {
    title: 'Read a Single Nested Field',
    unit: 'ns',
    maxVal: 1050,
    methodology:
      "Extracts a single string inside a nested sub-message from a raw packet without decoding the rest of the message. OmniBuf skips preceding fields via header hopping, FlatBuffers and unframed Cap'n Proto follow offset pointers directly, framed Cap'n Proto parses the segment frame header first, and Protobuf unmarshals the message.",
    rows: [
      { name: "Cap'n Proto (Unframed Shared Memory)", val: 5, color: '#78716C' },
      { name: 'FlatBuffers', val: 13, color: '#78716C' },
      { name: 'OmniBuf', val: 76, highlight: true, color: '#FF4F18' },
      { name: "Cap'n Proto (Framed Packet)", val: 221, color: '#A8A29E' },
      { name: 'MessagePack', val: 590, color: '#A8A29E' },
      { name: 'Protobuf', val: 983, color: '#78716C' },
    ],
  },
  {
    title: 'Packet Size on the Wire',
    unit: 'B',
    maxVal: 850,
    methodology:
      'Measures the exact serialized byte length of the identical 11-field payload across all formats, including OmniBuf both in standard .proto mode and with Field #0 inline field names enabled.',
    rows: [
      { name: 'Protobuf', val: 341, color: '#78716C' },
      { name: 'OmniBuf', val: 362, highlight: true, color: '#FF4F18' },
      { name: "Cap'n Proto", val: 560, color: '#A8A29E' },
      { name: 'FlatBuffers', val: 600, color: '#A8A29E' },
      { name: 'OmniBuf (With Field #0 Names)', val: 622, highlight: true, color: '#FF4F18' },
      { name: 'MessagePack', val: 656, color: '#A8A29E' },
      { name: 'JSON', val: 803, color: '#A8A29E' },
    ],
  },
];
