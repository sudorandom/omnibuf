package wire

import "testing"

func TestFragmentedArrayGrowth(t *testing.T) {
	var dst []uint32
	allocations := testing.AllocsPerRun(1, func() {
		dst = nil
		for i := range 8192 {
			var start int
			dst, start = growSlice(dst, 1)
			dst[start] = uint32(i)
		}
	})
	if allocations > 16 {
		t.Fatalf("fragmented array required %.0f allocations", allocations)
	}
	for i, value := range dst {
		if value != uint32(i) {
			t.Fatal("growth corrupted existing elements")
		}
	}
	// Ordinary single-container decodes keep exact allocation and reuse.
	fresh, start := growSlice[uint32](nil, 127)
	if start != 0 || len(fresh) != 127 || cap(fresh) != 127 {
		t.Fatal("fresh array overallocated")
	}
	reused, _ := growSlice(fresh[:0], 127)
	if &fresh[0] != &reused[0] {
		t.Fatal("existing capacity was not reused")
	}
	maxInt := int(^uint(0) >> 1)
	if got := GrowCapacity(maxInt/2+1, maxInt); got != maxInt {
		t.Fatal("capacity overflow")
	}
}
