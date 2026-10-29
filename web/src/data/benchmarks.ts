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
      'These radar charts compare five tradeoffs across our benchmark suite. Encode Speed and Decode Speed reflect how fast a message converts to and from native structs, with generated OmniBuf leading encoding while FlatBuffers decodes faster in this workload. Memory Efficiency tracks heap allocations during serialization, where buffer reuse and zeroCopy strings keep allocations near zero. Wire Size measures encoded byte length (within 7% of Protobuf and 14% of raw Avro), while Field Lookup measures reading one field from raw bytes, where hopping by byte length is much faster than scanning varints but still walks preceding field headers linearly unlike FlatBuffers offset tables.',
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
      'Protobuf is about 7% smaller on the wire and supported across almost every language in production. OmniBuf keeps the same .proto files and generated structs while writing messages in a single pass, encoding up to 6x faster and decoding roughly 3x to 7x faster in Go, and letting you inspect raw packets or use plain structs without a code generator.',
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
      'MessagePack is handy when you need to serialize arbitrary free-form maps with no struct definition at all. Because it is a binary version of JSON, it still repeats string field names on every object in an array. OmniBuf is 44% smaller (and still 4% smaller even with field names turned on, since array child names are written once) and faster in the Go encode and decode benchmarks below.',
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
      'JSON is the only format on this entire page that a human can actually read in curl or browser devtools with no extra tooling. When you want binary speed with the same workflow, OmniBuf uses struct tags in Go and object definitions in TypeScript just like JSON, while cutting packet size in half and reducing encode and decode time in Go (and running 2x faster in JS).',
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
      'Avro is a staple in Kafka Schema Registry and Spark pipelines. Raw Avro leaves out all field numbers and byte lengths on the wire (14% smaller than OmniBuf), which means you cannot read or skip a single field without the exact writer schema in memory, whereas Avro container files (OCF) embed the full JSON schema and are 4x larger. The Go benchmarks below compare encoding, decoding, and single-field reads for both formats.',
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

// Go timings: 2026-10-28, median of 8 × 200ms samples, CPU=1.
// Raw output and commands: testdata/benchmarks.txt.
export const benchmarkCategories: BenchmarkCategory[] = [
  {
    title: 'Full Message Encode Speed',
    unit: 'ns',
    maxVal: 2800,
    methodology:
      "Time to serialize our 11-field Go struct into a byte slice in a single pass (FlatBuffers and Cap'n Proto times include copying fields into their buffer builders).",
    rows: [
      { name: 'OmniBuf (Generated)', val: 123, highlight: true, baseline: true },
      { name: 'OmniBuf (Go Struct Tags)', val: 350, highlight: true },
      { name: 'Avro (Raw Binary)', val: 640 },
      { name: 'Protobuf', val: 721 },
      { name: 'FlatBuffers', val: 722 },
      { name: "Cap'n Proto", val: 1467 },
      { name: 'MessagePack', val: 2219 },
      { name: 'JSON (encoding/json/v2)', val: 2734 },
      { name: 'JSON (encoding/json)', val: 2739 },
    ],
  },
  {
    title: 'Full Message Decode Speed',
    unit: 'ns',
    maxVal: 4600,
    methodology:
      "Time to unpack all 11 fields. OmniBuf, FlatBuffers, Cap'n Proto, Avro, MessagePack, and JSON reuse the destination; standard Protobuf allocates a fresh message. Zero-copy strings are used where supported. For zero-unpack reads on FlatBuffers or Cap'n Proto buffers, see the single-field benchmark below.",
    rows: [
      { name: 'OmniBuf (Generated)', val: 229, highlight: true, baseline: true },
      { name: 'FlatBuffers', val: 253 },
      { name: 'OmniBuf (Go Struct Tags)', val: 452, highlight: true },
      { name: 'Avro (Raw Binary)', val: 670 },
      { name: "Cap'n Proto", val: 1105 },
      { name: 'Protobuf', val: 1521 },
      { name: 'MessagePack', val: 3146 },
      { name: 'JSON (encoding/json/v2)', val: 3492 },
      { name: 'JSON (encoding/json)', val: 4451 },
    ],
  },
  {
    title: 'Read a Single Nested Field (peer.location.city)',
    unit: 'ns',
    maxVal: 1600,
    methodology:
      "Time to read peer.location.city without unpacking the rest of the message. Cap'n Proto and FlatBuffers jump by offset; OmniBuf hops over earlier fields by byte length.",
    rows: [
      { name: "Cap'n Proto", val: 6 },
      { name: 'FlatBuffers', val: 35 },
      { name: 'OmniBuf', val: 117, highlight: true, baseline: true },
      { name: 'Avro (Raw Binary)', val: 677 },
      { name: 'MessagePack', val: 914 },
      { name: 'Protobuf', val: 1535 },
    ],
  },
  {
    title: 'Selective Single-Field Mutation (peer.sequence)',
    unit: 'ns',
    maxVal: 7600,
    methodology:
      'Checking peer.location.city, bumping the 64-bit sequence counter, and writing the updated packet. OmniBuf splices the field into the packet; the other implementations decode and re-encode the message.',
    rows: [
      { name: 'OmniBuf (Opaque Splice)', val: 255, highlight: true, baseline: true },
      { name: 'FlatBuffers', val: 991 },
      { name: 'Avro (Raw Binary)', val: 1345 },
      { name: 'Protobuf', val: 2299 },
      { name: "Cap'n Proto", val: 2733 },
      { name: 'MessagePack', val: 5635 },
      { name: 'JSON (encoding/json)', val: 7376 },
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

// TypeScript results imported from omnibuf-es/testdata/benchmarks.json (Node.js v24.22.1).
export const esBenchmarkCategories: EsBenchmarkCategory[] = [
  {
    title: 'Full Message Encode Speed',
    unit: 'ns',
    maxVal: 5000,
    methodology:
      'Serializing the 11-field PeerUpdate message (nested Location, 4 Neighbor messages, packed uint32 prefixes, string tags, and 64-bit counters) into a Uint8Array.',
    rows: [
      {
        name: 'OmniBuf',
        val: 920,
        ops: '1,086,721 ops/s',
        ratio: '1.00x (base)',
        sizeBytes: 366,
        highlight: true,
        baseline: true,
      },
      {
        name: 'protobufjs',
        val: 1282,
        ops: '780,291 ops/s',
        ratio: '1.39x slower',
        sizeBytes: 350,
      },
      {
        name: 'JSON',
        val: 1819,
        ops: '549,711 ops/s',
        ratio: '1.98x slower',
        sizeBytes: 791,
      },
      {
        name: '@bufbuild/protobuf (protobuf-es)',
        val: 2311,
        ops: '432,651 ops/s',
        ratio: '2.51x slower',
        sizeBytes: 350,
      },
      {
        name: '@msgpack/msgpack',
        val: 3166,
        ops: '315,890 ops/s',
        ratio: '3.44x slower',
        sizeBytes: 622,
      },
      {
        name: 'google-protobuf',
        val: 4763,
        ops: '209,930 ops/s',
        ratio: '5.18x slower',
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
        val: 846,
        ops: '1,181,895 ops/s',
        ratio: '1.00x (base)',
        highlight: true,
        baseline: true,
      },
      {
        name: 'protobufjs',
        val: 1341,
        ops: '745,980 ops/s',
        ratio: '1.59x slower',
      },
      {
        name: 'JSON',
        val: 1859,
        ops: '537,815 ops/s',
        ratio: '2.20x slower',
      },
      {
        name: 'google-protobuf',
        val: 2587,
        ops: '386,486 ops/s',
        ratio: '3.06x slower',
      },
      {
        name: '@msgpack/msgpack',
        val: 2992,
        ops: '334,193 ops/s',
        ratio: '3.54x slower',
      },
      {
        name: '@bufbuild/protobuf (protobuf-es)',
        val: 3290,
        ops: '303,968 ops/s',
        ratio: '3.89x slower',
      },
    ],
  },
  {
    title: 'Read a Single Nested Field (peer.location.city)',
    unit: 'ns',
    maxVal: 5000,
    methodology:
      'Reading peer.location.city without unpacking the rest of the message. Other JS codecs must decode the full message first; OmniBuf hops over earlier field headers by byte length.',
    rows: [
      {
        name: 'OmniBuf',
        val: 317,
        ops: '3,156,949 ops/s',
        ratio: '1.00x (base)',
        highlight: true,
        baseline: true,
      },
      {
        name: 'JSON',
        val: 2300,
        ops: '434,812 ops/s',
        ratio: '7.26x slower',
      },
      {
        name: 'protobufjs',
        val: 2651,
        ops: '377,153 ops/s',
        ratio: '8.36x slower',
      },
      {
        name: '@msgpack/msgpack',
        val: 2843,
        ops: '351,690 ops/s',
        ratio: '8.97x slower',
      },
      {
        name: 'google-protobuf',
        val: 2920,
        ops: '342,473 ops/s',
        ratio: '9.21x slower',
      },
      {
        name: '@bufbuild/protobuf (protobuf-es)',
        val: 4574,
        ops: '218,637 ops/s',
        ratio: '14.43x slower',
      },
    ],
  },
  {
    title: 'Selective Single-Field Mutation (peer.sequence)',
    unit: 'ns',
    maxVal: 8000,
    methodology:
      'Bumping the 64-bit sequence counter on an in-flight packet and returning the updated Uint8Array without unpacking the other 10 fields.',
    rows: [
      {
        name: 'OmniBuf',
        val: 442,
        ops: '2,260,498 ops/s',
        ratio: '1.00x (base)',
        highlight: true,
        baseline: true,
      },
      {
        name: 'protobufjs',
        val: 2980,
        ops: '335,539 ops/s',
        ratio: '6.74x slower',
      },
      {
        name: 'JSON',
        val: 3666,
        ops: '272,758 ops/s',
        ratio: '8.29x slower',
      },
      {
        name: '@bufbuild/protobuf (protobuf-es)',
        val: 5634,
        ops: '177,508 ops/s',
        ratio: '12.75x slower',
      },
      {
        name: '@msgpack/msgpack',
        val: 6212,
        ops: '160,974 ops/s',
        ratio: '14.05x slower',
      },
      {
        name: 'google-protobuf',
        val: 7459,
        ops: '134,071 ops/s',
        ratio: '16.88x slower',
      },
    ],
  },
];
