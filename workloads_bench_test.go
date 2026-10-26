package omnibuf_test

import (
	"strings"
	"testing"

	ob "github.com/sudorandom/omnibuf"
	"github.com/sudorandom/omnibuf/internal/pb"
	"google.golang.org/protobuf/proto"
)

// Exercise integer-width cliffs and container expansion, with matching ownership
// contracts. These cases supplement the small PeerUpdate showcase, not a claim
// about every application's workload.
func BenchmarkRepresentativeWorkloads(b *testing.B) {
	for _, scenario := range []string{"small", "sparse", "counter_29bit_boundary", "timestamps", "nested_over_16KiB", "large_repeated"} {
		sample := SamplePeerUpdate()
		switch scenario {
		case "small":
			sample.UptimeSec = 1
			sample.Sequence = 127
			sample.Prefixes = []uint32{1, 2, 3}
		case "sparse":
			sample = PeerUpdate{
				ASN:      64512,
				RouterID: "edge-rtr-01",
				Active:   true,
			}
		case "counter_29bit_boundary":
			sample.UptimeSec = 1 << 29
			sample.Sequence = (1 << 29) - 1
			sample.Prefixes = []uint32{(1 << 29) - 1, 1 << 29, 1 << 30}
		case "timestamps":
			sample.UptimeSec = 1700000000
			sample.Sequence = 1700000000000
		case "nested_over_16KiB":
			sample.Location.City = strings.Repeat("x", 20<<10)
		case "large_repeated":
			sample.Prefixes = make([]uint32, 10000)
			for i := range sample.Prefixes {
				sample.Prefixes[i] = uint32(i * 100000)
			}
		}
		msg := SamplePBPeerUpdate(&sample)
		omni := msg.MarshalOmniBufAppend(nil)
		protobuf, err := proto.Marshal(msg)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(scenario, func(b *testing.B) {
			b.Run("encode_omnibuf_reuse", func(b *testing.B) {
				buf := make([]byte, 0, len(omni)+32)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					buf = msg.MarshalOmniBufAppend(buf[:0])
				}
				b.ReportMetric(float64(len(buf)), "wire-B/op")
			})
			b.Run("encode_vtprotobuf_reuse", func(b *testing.B) {
				buf := make([]byte, len(protobuf))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := msg.MarshalToVT(buf); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(float64(len(protobuf)), "wire-B/op")
			})
			b.Run("decode_omnibuf_owned_fresh", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var out pb.PeerUpdate
					if err := out.UnmarshalOmniBuf(omni, false); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("decode_omnibuf_table_owned_fresh", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var out pb.PeerUpdate
					if err := ob.Unmarshal(omni, &out); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("decode_vtprotobuf_owned_fresh", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var out pb.PeerUpdate
					if err := out.UnmarshalVT(protobuf); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
