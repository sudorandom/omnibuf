export interface FormatStat {
  label: string;
  score: number;
}

export interface FormatProfile {
  key: string;
  name: string;
  url?: string;
  verdict?: string;
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

export interface EsBenchmarkRow {
  name: string;
  val: number;
  ops: string;
  ratio: string;
  sizeBytes?: number;
  highlight?: boolean;
  baseline?: boolean;
}

export interface EsBenchmarkCategory {
  title: string;
  unit: string;
  maxVal: number;
  methodology: string;
  rows: EsBenchmarkRow[];
}

export const formatProfiles: Record<string, FormatProfile> = {
  omnibuf: {
    key: 'omnibuf',
    name: 'OmniBuf',
    verdict:
      'These radar charts compare five tradeoffs across our benchmark suite. Encode Speed and Decode Speed reflect how fast a message converts to and from native structs, where single-pass writing and TZCNT Prefix-Varints put OmniBuf at the top. Memory Efficiency tracks heap allocations during serialization, where buffer reuse and zeroCopy strings keep allocations near zero. Wire Size measures encoded byte length (within 7% of Protobuf and 14% of raw Avro), while Field Lookup measures reading one field from raw bytes, where hopping by byte length is much faster than scanning varints but still walks preceding field headers linearly unlike FlatBuffers offset tables.',
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
      'Protobuf is about 7% smaller on the wire and supported across almost every language in production. OmniBuf keeps the same .proto files and generated structs while writing messages in a single pass, encoding up to 6x faster and decoding 2x to 7x faster in Go, and letting you inspect raw packets or use plain structs without a code generator.',
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
      'FlatBuffers is built around reading fields straight out of a byte buffer by offset, so it is great when you only need one or two fields from a huge payload without unpacking. If your code works with normal Go structs or TypeScript objects, OmniBuf is 39% smaller on the wire, up to 6x faster, and does not require a separate .fbs schema or builder API.',
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
      "Like FlatBuffers, Cap'n Proto is designed to be used through its own generated pointer types at fixed offsets, and includes an RPC system with promise pipelining. When converting to and from standard language structs, OmniBuf is 35% smaller on the wire and several times faster, while working with normal .proto files or struct tags.",
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
      'MessagePack is handy when you need to serialize arbitrary free-form maps with no struct definition at all. Because it is a binary version of JSON, it still repeats string field names on every object in an array. OmniBuf is 44% smaller (and still 4% smaller even with field names turned on, since array child names are written once) and 7x to 16x faster in Go.',
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
      'JSON is the only format on this entire page that a human can actually read in curl or browser devtools with no extra tooling. When you want binary speed with the same workflow, OmniBuf uses struct tags in Go and object definitions in TypeScript just like JSON, while cutting packet size in half and running 7x to 25x faster in Go (and 2x faster in JS).',
    stats: [
      { label: 'Wire Size', score: 42 },
      { label: 'Encode Speed', score: 12 },
      { label: 'Decode Speed', score: 10 },
      { label: 'Field Lookup', score: 10 },
      { label: 'Memory Efficiency', score: 38 },
    ],
  },
  avro: {
    key: 'avro',
    name: 'Apache Avro',
    url: 'https://avro.apache.org/',
    verdict:
      'Avro is a staple in Kafka Schema Registry and Spark pipelines. Raw Avro leaves out all field numbers and byte lengths on the wire (14% smaller than OmniBuf), which means you cannot read or skip a single field without the exact writer schema in memory, whereas Avro container files (OCF) embed the full JSON schema and are 4x larger. In Go, OmniBuf encodes 4x faster, decodes 3x faster, and reads a single field 6x faster.',
    stats: [
      { label: 'Wire Size', score: 98 },
      { label: 'Encode Speed', score: 58 },
      { label: 'Decode Speed', score: 66 },
      { label: 'Field Lookup', score: 48 },
      { label: 'Memory Efficiency', score: 78 },
    ],
  },
  parquet: {
    key: 'parquet',
    name: 'Apache Parquet',
    url: 'https://parquet.apache.org/',
    verdict:
      'Parquet is left out of the single-message benchmarks because it is a columnar file format built for analytical batch scans over thousands or millions of rows rather than individual RPC or pub/sub packets. Even a 1-row Parquet file has to write column chunks, page headers, and a multi-kilobyte Thrift schema and statistics footer for every leaf column. Use Parquet when archiving batches of events for DuckDB or Spark, and OmniBuf when sending individual messages over the wire.',
    stats: [
      { label: 'Wire Size', score: 15 },
      { label: 'Encode Speed', score: 8 },
      { label: 'Decode Speed', score: 10 },
      { label: 'Field Lookup', score: 20 },
      { label: 'Memory Efficiency', score: 20 },
    ],
  },
};

export const opponentKeys = [
  'protobuf',
  'flatbuffers',
  'capnproto',
  'avro',
  'parquet',
  'msgpack',
  'json',
] as const;

export const benchmarkCategories: BenchmarkCategory[] = [
  {
    title: 'Full Message Encode Speed',
    unit: 'ns',
    maxVal: 2200,
    methodology:
      "Time to serialize our 11-field Go struct into a byte slice. OmniBuf writes the message in a single pass. Note that FlatBuffers and Cap'n Proto are built around their own buffer builders rather than normal Go structs, so their times here include copying the fields out of a Go struct.",
    rows: [
      { name: 'OmniBuf (Generated)', val: 123, highlight: true, baseline: true },
      { name: 'OmniBuf (Go Struct Tags)', val: 300, highlight: true },
      { name: 'Avro (Raw Binary)', val: 546 },
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
      "Time to unpack all 11 fields of a packet into a reused Go struct (when allocating a fresh struct and copying every string on each call, OmniBuf still runs more than 2x faster than Protobuf). If you read FlatBuffers or Cap'n Proto buffers directly in place without unpacking into a Go struct, see the single-field benchmark below.",
    rows: [
      { name: 'OmniBuf (Generated)', val: 176, highlight: true, baseline: true },
      { name: 'FlatBuffers', val: 222 },
      { name: 'OmniBuf (Go Struct Tags)', val: 403, highlight: true },
      { name: 'Avro (Raw Binary)', val: 540 },
      { name: "Cap'n Proto", val: 972 },
      { name: 'Protobuf', val: 1280 },
      { name: 'MessagePack', val: 2743 },
      { name: 'JSON (encoding/json/v2)', val: 3519 },
      { name: 'JSON (encoding/json)', val: 4456 },
    ],
  },
  {
    title: 'Read a Single Nested Field (peer.location.city)',
    unit: 'ns',
    maxVal: 1300,
    methodology:
      "Time to read a single string (peer.location.city) out of a packet without unpacking the rest of the message. Pointer-based formats (Cap'n Proto and FlatBuffers) can jump straight to an offset, while OmniBuf steps over earlier fields by adding their byte lengths.",
    rows: [
      { name: "Cap'n Proto", val: 6 },
      { name: 'FlatBuffers', val: 16 },
      { name: 'OmniBuf', val: 103, highlight: true, baseline: true },
      { name: 'Avro (Raw Binary)', val: 649 },
      { name: 'MessagePack', val: 766 },
      { name: 'Protobuf', val: 1270 },
    ],
  },
  {
    title: 'Selective Single-Field Mutation (Update peer.sequence & Re-Serialize)',
    unit: 'ns',
    maxVal: 7000,
    methodology:
      'Checking peer.location.city, bumping the 64-bit sequence counter on an in-flight packet, and writing the updated packet bytes. OmniBuf splices the updated field directly onto the raw byte slice without unpacking or re-encoding the other 10 fields.',
    rows: [
      { name: 'OmniBuf (Opaque Splice)', val: 199, highlight: true, baseline: true },
      { name: 'FlatBuffers', val: 958 },
      { name: 'Avro (Raw Binary)', val: 1131 },
      { name: 'Protobuf', val: 2027 },
      { name: "Cap'n Proto", val: 2441 },
      { name: 'MessagePack', val: 5051 },
      { name: 'JSON (encoding/json)', val: 6846 },
    ],
  },
  {
    title: 'Packet Size on the Wire',
    unit: 'B',
    maxVal: 1400,
    methodology:
      'Encoded byte size for the same 11-field message. Raw Avro leaves out all field tags and byte lengths (so you need the exact writer schema on hand to read it), while Avro OCF includes the JSON schema in the file header.',
    rows: [
      { name: 'Avro (Raw, No Field Tags)', val: 313 },
      { name: 'Protobuf', val: 341 },
      { name: 'OmniBuf', val: 366, highlight: true, baseline: true },
      { name: "Cap'n Proto", val: 560 },
      { name: 'FlatBuffers', val: 600 },
      { name: 'OmniBuf (With Field #0 Names)', val: 630, highlight: true },
      { name: 'MessagePack', val: 656 },
      { name: 'JSON', val: 803 },
      { name: 'Avro OCF (With Schema Header)', val: 1359 },
    ],
  },
];

export const esBenchmarkCategories: EsBenchmarkCategory[] = [
  {
    title: 'Full Message Encode Speed',
    unit: 'ns',
    maxVal: 6000,
    methodology:
      'Serializing the 11-field PeerUpdate message (nested Location, 4 Neighbor messages, packed uint32 prefixes, string tags, and 64-bit counters) into a Uint8Array.',
    rows: [
      {
        name: 'OmniBuf',
        val: 1169,
        ops: '855,686 ops/s',
        ratio: '1.00x (base)',
        sizeBytes: 366,
        highlight: true,
        baseline: true,
      },
      {
        name: 'protobufjs',
        val: 1653,
        ops: '604,858 ops/s',
        ratio: '1.41x slower',
        sizeBytes: 350,
      },
      {
        name: 'JSON',
        val: 2320,
        ops: '430,946 ops/s',
        ratio: '1.99x slower',
        sizeBytes: 791,
      },
      {
        name: '@bufbuild/protobuf (protobuf-es)',
        val: 2706,
        ops: '369,601 ops/s',
        ratio: '2.32x slower',
        sizeBytes: 350,
      },
      {
        name: '@msgpack/msgpack',
        val: 4042,
        ops: '247,391 ops/s',
        ratio: '3.46x slower',
        sizeBytes: 622,
      },
      {
        name: 'google-protobuf',
        val: 5863,
        ops: '170,553 ops/s',
        ratio: '5.02x slower',
        sizeBytes: 350,
      },
    ],
  },
  {
    title: 'Full Message Decode Speed',
    unit: 'ns',
    maxVal: 4000,
    methodology: 'Decoding the 11-field PeerUpdate packet back into a typed JavaScript object.',
    rows: [
      {
        name: 'OmniBuf',
        val: 1090,
        ops: '917,408 ops/s',
        ratio: '1.00x (base)',
        highlight: true,
        baseline: true,
      },
      {
        name: 'protobufjs',
        val: 1537,
        ops: '650,771 ops/s',
        ratio: '1.41x slower',
      },
      {
        name: 'JSON',
        val: 2335,
        ops: '428,243 ops/s',
        ratio: '2.14x slower',
      },
      {
        name: 'google-protobuf',
        val: 3390,
        ops: '295,025 ops/s',
        ratio: '3.11x slower',
      },
      {
        name: '@msgpack/msgpack',
        val: 3599,
        ops: '277,843 ops/s',
        ratio: '3.30x slower',
      },
      {
        name: '@bufbuild/protobuf (protobuf-es)',
        val: 3780,
        ops: '264,582 ops/s',
        ratio: '3.47x slower',
      },
    ],
  },
  {
    title: 'Read a Single Nested Field (peer.location.city)',
    unit: 'ns',
    maxVal: 4000,
    methodology:
      'Time to read peer.location.city from a packet without unpacking the rest of the message. The other JS libraries have to decode the full message first, while OmniBuf steps over the earlier field headers by adding their byte lengths.',
    rows: [
      {
        name: 'OmniBuf',
        val: 389,
        ops: '2,568,985 ops/s',
        ratio: '1.00x (base)',
        highlight: true,
        baseline: true,
      },
      {
        name: 'protobufjs',
        val: 1533,
        ops: '652,421 ops/s',
        ratio: '3.94x slower',
      },
      {
        name: 'JSON',
        val: 2348,
        ops: '425,901 ops/s',
        ratio: '6.03x slower',
      },
      {
        name: 'google-protobuf',
        val: 3385,
        ops: '295,379 ops/s',
        ratio: '8.70x slower',
      },
      {
        name: '@msgpack/msgpack',
        val: 3591,
        ops: '278,440 ops/s',
        ratio: '9.23x slower',
      },
      {
        name: '@bufbuild/protobuf (protobuf-es)',
        val: 3762,
        ops: '265,825 ops/s',
        ratio: '9.66x slower',
      },
    ],
  },
  {
    title: 'Selective Single-Field Mutation (Update peer.sequence & Re-Serialize)',
    unit: 'ns',
    maxVal: 10000,
    methodology:
      'Bumping the 64-bit sequence counter on an in-flight packet and returning the updated Uint8Array. OmniBuf splices the new field directly onto the raw bytes without touching the other 10 fields.',
    rows: [
      {
        name: 'OmniBuf',
        val: 533,
        ops: '1,875,474 ops/s',
        ratio: '1.00x (base)',
        highlight: true,
        baseline: true,
      },
      {
        name: 'protobufjs',
        val: 3734,
        ops: '267,791 ops/s',
        ratio: '7.00x slower',
      },
      {
        name: 'JSON',
        val: 4773,
        ops: '209,496 ops/s',
        ratio: '8.95x slower',
      },
      {
        name: '@bufbuild/protobuf (protobuf-es)',
        val: 6952,
        ops: '143,845 ops/s',
        ratio: '13.04x slower',
      },
      {
        name: '@msgpack/msgpack',
        val: 7706,
        ops: '129,761 ops/s',
        ratio: '14.45x slower',
      },
      {
        name: 'google-protobuf',
        val: 9492,
        ops: '105,351 ops/s',
        ratio: '17.80x slower',
      },
    ],
  },
];
