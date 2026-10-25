//go:build go1.27 && goexperiment.jsonv2

package omnibuf_test

import (
	jsonv2 "encoding/json/v2"
	"testing"
)

func BenchmarkMarshal_JSONv2(b *testing.B) {
	orig := SamplePeerUpdate()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pkt, err := jsonv2.Marshal(&orig)
		if err != nil || len(pkt) == 0 {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal_JSONv2_SafeAlloc(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt, err := jsonv2.Marshal(&orig)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out PeerUpdate
		if err := jsonv2.Unmarshal(pkt, &out); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal_JSONv2_Reuse(b *testing.B) {
	orig := SamplePeerUpdate()
	pkt, err := jsonv2.Marshal(&orig)
	if err != nil {
		b.Fatal(err)
	}
	var out PeerUpdate
	out.Tags = make([]string, 0, 8)
	out.Prefixes = make([]uint32, 0, 8)
	out.Neighbors = make([]Neighbor, 0, 8)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out.Tags = out.Tags[:0]
		out.Prefixes = out.Prefixes[:0]
		out.Neighbors = out.Neighbors[:0]
		if err := jsonv2.Unmarshal(pkt, &out); err != nil {
			b.Fatal(err)
		}
	}
}
