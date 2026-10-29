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

// A shared schema must not be copied for each array element, including when
// elements supply small local overrides of a large inherited schema.
func TestBlindSharedSchemaAllocation(t *testing.T) {
	for _, local := range []bool{false, true} {
		t.Run(fmt.Sprintf("local=%v", local), func(t *testing.T) {
			var small uint64
			for _, count := range []int{256, 1024} {
				fields := make(map[uint32]string, count)
				for i := 1; i <= count; i++ {
					fields[uint32(i)] = fmt.Sprintf("name_%d", i)
				}
				meta := &ob.SchemaMeta{Children: map[uint32]*ob.SchemaMeta{1: {Fields: fields}}}
				packet := meta.AppendField0(nil, true)
				body := wire.AppendVarint([]byte{wire.WireSubMsg}, uint64(count))
				for i := range count {
					elem := []byte{33, 1} // field 1, uint 0
					if local && i%2 == 0 {
						// Metadata after the value must still rename it, without
						// changing the inherited schema for the next sibling.
						elem = (&ob.SchemaMeta{Fields: map[uint32]string{1: "local"}}).AppendField0(elem, true)
					}
					body = wire.AppendVarint(body, uint64(len(elem)))
					body = append(body, elem...)
				}
				packet = wire.AppendVarint(packet, 1<<4|uint64(wire.WireArray))
				packet = wire.AppendVarint(packet, uint64(len(body)))
				packet = append(packet, body...)
				runtime.GC()
				var before, after runtime.MemStats
				runtime.ReadMemStats(&before)
				out, err := ob.DecodeBlind(packet)
				runtime.ReadMemStats(&after)
				if err != nil {
					t.Fatal(err)
				}
				children := out["field_1"].([]map[string]any)
				if len(children) != count {
					t.Fatal("wrong child count")
				}
				for i, child := range children {
					name := "name_1"
					if local && i%2 == 0 {
						name = "local"
					}
					if value, ok := child[name]; !ok || value != uint64(0) || len(child) != 1 {
						t.Fatalf("child %d: %v", i, child)
					}
				}
				allocated := after.TotalAlloc - before.TotalAlloc
				if count == 256 {
					small = allocated
				} else if allocated > 8<<20 || allocated > small*6 {
					t.Fatalf("schema allocation is not linear: small=%d large=%d", small, allocated)
				}
			}
		})
	}
}

func TestBlindSchemaOverlayNestedMerges(t *testing.T) {
	inherited := &ob.SchemaMeta{Children: map[uint32]*ob.SchemaMeta{1: {
		Fields:   map[uint32]string{1: "nested"},
		Children: map[uint32]*ob.SchemaMeta{1: {Fields: map[uint32]string{1: "original", 2: "retained"}}},
	}}}
	// Two local metadata blocks merge before overlaying the inherited child.
	first := &ob.SchemaMeta{Children: map[uint32]*ob.SchemaMeta{1: {Fields: map[uint32]string{1: "replaced"}}}}
	last := &ob.SchemaMeta{Children: map[uint32]*ob.SchemaMeta{1: {Fields: map[uint32]string{1: ""}}}}
	grandchild := []byte{33, 3, 65, 5} // uint fields 1=1 and 2=2
	child := wire.AppendVarint(nil, 1<<4|uint64(wire.WireSubMsg))
	child = wire.AppendVarint(child, uint64(len(grandchild)))
	child = append(child, grandchild...)
	localChild := first.AppendField0(append([]byte(nil), child...), true)
	localChild = last.AppendField0(localChild, true)
	body := wire.AppendVarint([]byte{wire.WireSubMsg}, 2)
	for _, elem := range [][]byte{localChild, child} {
		body = wire.AppendVarint(body, uint64(len(elem)))
		body = append(body, elem...)
	}
	packet := inherited.AppendField0(nil, true)
	packet = wire.AppendVarint(packet, 1<<4|uint64(wire.WireArray))
	packet = wire.AppendVarint(packet, uint64(len(body)))
	packet = append(packet, body...)
	got, err := ob.DecodeBlind(packet)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"field_1": []map[string]any{
		{"nested": map[string]any{"field_1": uint64(1), "retained": uint64(2)}},
		{"nested": map[string]any{"original": uint64(1), "retained": uint64(2)}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
