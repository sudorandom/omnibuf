package omnibuf_test

import (
	"bytes"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"

	ob "github.com/sudorandom/omnibuf"
	"github.com/sudorandom/omnibuf/internal/pb"
	"github.com/sudorandom/omnibuf/wire"
)

// A small, valid packet must not cause quadratic cumulative allocation when
// repeated fields are fragmented into individual elements.
func TestFragmentedArrayAllocation(t *testing.T) {
	const count = 2048
	sample := &pb.TestAllTypes{F9StringArr: []string{"x"}, F10Uint32Arr: []uint32{7}, F11SubMsgArr: []*pb.AllTypesSubMsg{{Id: 42}}}
	packet := bytes.Repeat(sample.MarshalOmniBufAppend(nil), count)
	type child struct {
		ID uint32 `omnibuf:"1"`
	}
	type values struct {
		Strings  []string `omnibuf:"9"`
		Numbers  []uint32 `omnibuf:"10"`
		Children []child  `omnibuf:"11"`
	}
	for _, tier := range []string{"table-values", "table-pointers", "generated", "wire", "table-wire-overrides"} {
		t.Run(tier, func(t *testing.T) {
			decode := func() {
				t.Helper()
				var ss []string
				var ns []uint32
				switch tier {
				case "table-values":
					var out values
					if err := ob.UnmarshalZeroCopy(packet, &out); err != nil {
						t.Fatal(err)
					}
					ss, ns = out.Strings, out.Numbers
					if len(out.Children) != count {
						t.Fatal("wrong child count")
					}
					for _, v := range out.Children {
						if v.ID != 42 {
							t.Fatal("corrupt child")
						}
					}
				case "table-pointers", "generated":
					var out pb.TestAllTypes
					var err error
					if tier == "generated" {
						err = out.UnmarshalOmniBuf(packet, true)
					} else {
						err = ob.UnmarshalZeroCopy(packet, &out)
					}
					if err != nil {
						t.Fatal(err)
					}
					ss, ns = out.F9StringArr, out.F10Uint32Arr
					if len(out.F11SubMsgArr) != count {
						t.Fatal("wrong child count")
					}
					for _, v := range out.F11SubMsgArr {
						if v == nil || v.Id != 42 {
							t.Fatal("corrupt child")
						}
					}
				case "table-wire-overrides":
					type overrides struct {
						Strings []string `omnibuf:"9,wire=bytes"`
						Numbers []uint32 `omnibuf:"10"`
					}
					segment, err := ob.Marshal(&overrides{Strings: []string{"x"}, Numbers: []uint32{7}})
					if err != nil {
						t.Fatal(err)
					}
					input := bytes.Repeat(segment, count)
					var out overrides
					if err := ob.UnmarshalZeroCopy(input, &out); err != nil {
						t.Fatal(err)
					}
					ss, ns = out.Strings, out.Numbers
				case "wire":
					var ok bool
					ss, ok = wire.ReadStringArr(packet, 9)
					if !ok {
						t.Fatal("strings rejected")
					}
					ns, ok = wire.ReadUint32Arr(packet, 10)
					if !ok {
						t.Fatal("integers rejected")
					}
				}
				if len(ss) != count || len(ns) != count {
					t.Fatal("wrong array length")
				}
				for i := range ss {
					if ss[i] != "x" || ns[i] != 7 {
						t.Fatal("corrupt element")
					}
				}
			}
			decode() // Warm codec caches before measuring.
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			decode()
			runtime.ReadMemStats(&after)
			// Generous allowance for linear output and race instrumentation. The old
			// exact-sized growth allocates tens of MiB for this input.
			if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 2<<20 {
				t.Fatalf("fragmented decode allocated %d bytes for %d wire bytes", allocated, len(packet))
			}
		})
	}
}

func TestLargeDeterministicMap(t *testing.T) {
	for _, count := range []int{16, 17, 4096} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			values := make(map[string]uint64, count)
			prefix := strings.Repeat("prefix-", 32)
			for i := range count {
				values[fmt.Sprintf("%s%08d", prefix, i)] = uint64(i)
			}
			type message struct {
				Values map[string]uint64 `omnibuf:"12"`
			}
			want := wire.NewEncoder(0)
			want.WriteMapStringUintDeterministic(12, values)
			packet := want.Finish()
			got, ok := wire.ReadMapStringUint(packet, 12, false)
			if !ok || !reflect.DeepEqual(got, values) {
				t.Fatal("map round trip failed")
			}
			// Both independent encoders must preserve identical sorted wire output.
			for range 3 {
				encoded, err := ob.MarshalOptions{Deterministic: true}.Marshal(&message{values})
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(encoded, packet) {
					t.Fatal("table deterministic map differs")
				}
			}
		})
	}
}
