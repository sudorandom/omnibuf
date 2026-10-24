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
      'OmniBuf stays within a few bytes of Protobuf while encoding up to 6x faster, decoding up to 8x faster, supporting raw message concatenation, and making every wire type inspectable without a schema.',
    omniWinsList: [
      'Over 2x to 6x faster encoding with single-pass writes instead of measuring nested objects twice.',
      'Over 2x to 8x faster full decoding with branchless Prefix-Varints.',
      'Self-describing 4-bit wire types and optional Field #0 names eliminate Wire Type 2 ambiguity.',
      'Faster field skipping while preserving raw message concatenation (append(msgA, msgB...)).',
    ],
    oppWinsList: [
      'Built into existing Protobuf ecosystems and third-party services where you cannot change the wire format.',
      'Roughly 6% smaller wire payloads (341 B vs. 362 B in our benchmark) because Protobuf uses 3-bit tags and 1-byte lengths on small nested messages.',
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
      'OmniBuf is 40% smaller on the wire, over 6x faster to encode, and faster on full message decodes while using standard .proto schemas instead of bottom-up builders.',
    omniWinsList: [
      '40% smaller packets on the wire because there are no VTables or alignment padding.',
      'Over 6x faster encoding with normal top-down writes instead of bottom-up builders.',
      'Faster full message decoding and supports raw byte concatenation.',
      'Works directly with standard .proto schemas.',
    ],
    oppWinsList: [
      'Faster single-field random access when reading one field out of a very large message via VTable offsets.',
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
      'On framed network packets, OmniBuf is 36% smaller, 12x faster to encode, and over 6x faster to decode without multi-segment pointer bounds checking.',
    omniWinsList: [
      '36% smaller network packets without 64-bit pointer fat or 8-byte word padding.',
      '12x faster encoding with zero heap allocations.',
      'Over 6x faster full decoding and nearly 3x faster single-field reads on framed network packets.',
      'Uses standard .proto schemas and supports raw message concatenation.',
    ],
    oppWinsList: [
      'Includes a built-in RPC system with promise pipelining (chaining dependent RPC calls in a single network round-trip).',
      'Fixed-offset struct layout allows jumping straight to fields in very large messages without scanning preceding field headers.',
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
      'OmniBuf is nearly half the size of MessagePack in shared-schema mode, and smaller even with Field #0 inline field names enabled while encoding over 6x faster.',
    omniWinsList: [
      '45% smaller packets in standard .proto mode, and 5% smaller even with Field #0 inline names enabled (622 B vs. 656 B).',
      '16x faster encoding (or over 6x faster with Field #0 inline field names included).',
      'Over 20x faster full decoding and over 8x faster single-field skipping.',
      'Hoists nested child schemas into Field #0 once per message instead of repeating key strings inside every array element.',
    ],
    oppWinsList: [
      'Supports arbitrary schema-free maps with heterogeneous keys when no .proto structure exists.',
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
      'OmniBuf is less than half the size of JSON and 17x to 32x faster, while still letting you use official ProtoJSON for human-readable debugging from the same .proto file.',
    omniWinsList: [
      '55% smaller on the wire (and 23% smaller even when Field #0 field names are embedded).',
      '17x faster encoding and 32x faster decoding.',
      '55x faster single-field extraction without parsing text brackets or quotes.',
      'Compatible with official ProtoJSON so you can still accept or emit plain text JSON from the same .proto schema.',
    ],
    oppWinsList: [
      'Readable as plain text in browser devtools and curl when no decoder tool is available.',
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
    title: 'Full Message Decode Speed',
    unit: 'ns',
    maxVal: 4000,
    rows: [
      { name: 'OmniBuf', val: 120, highlight: true, color: '#D4FF33' },
      { name: 'FlatBuffers', val: 170, color: '#94A3B8' },
      { name: "Cap'n Proto", val: 772, color: '#CBD5E1' },
      { name: 'Protobuf', val: 960, color: '#FF4F18' },
      { name: 'MessagePack', val: 2496, color: '#E2E8F0' },
      { name: 'JSON', val: 3850, color: '#E2E8F0' },
    ],
  },
  {
    title: 'Full Message Encode Speed',
    unit: 'ns',
    maxVal: 1800,
    rows: [
      { name: 'OmniBuf', val: 94, highlight: true, color: '#D4FF33' },
      { name: 'Protobuf', val: 561, color: '#FF4F18' },
      { name: 'FlatBuffers', val: 562, color: '#94A3B8' },
      { name: "Cap'n Proto", val: 1016, color: '#CBD5E1' },
      { name: 'MessagePack', val: 1552, color: '#E2E8F0' },
      { name: 'JSON', val: 1720, color: '#E2E8F0' },
    ],
  },
  {
    title: 'Read a Single Nested Field (peer.location.city)',
    unit: 'ns',
    maxVal: 1050,
    rows: [
      { name: "Cap'n Proto (Unframed Shared Memory)", val: 5, color: '#94A3B8' },
      { name: 'FlatBuffers', val: 13, color: '#94A3B8' },
      { name: 'OmniBuf', val: 76, highlight: true, color: '#D4FF33' },
      { name: "Cap'n Proto (Framed Packet)", val: 221, color: '#CBD5E1' },
      { name: 'MessagePack', val: 590, color: '#E2E8F0' },
      { name: 'Protobuf', val: 983, color: '#FF4F18' },
    ],
  },
  {
    title: 'Packet Size on the Wire',
    unit: 'B',
    maxVal: 850,
    rows: [
      { name: 'Protobuf', val: 341, color: '#94A3B8' },
      { name: 'OmniBuf', val: 362, highlight: true, color: '#D4FF33' },
      { name: "Cap'n Proto", val: 560, color: '#CBD5E1' },
      { name: 'FlatBuffers', val: 600, color: '#CBD5E1' },
      { name: 'OmniBuf (With Field #0 Names)', val: 622, highlight: true, color: '#D4FF33' },
      { name: 'MessagePack', val: 656, color: '#E2E8F0' },
      { name: 'JSON', val: 803, color: '#FF4F18' },
    ],
  },
];
