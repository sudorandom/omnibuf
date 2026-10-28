package wire

import (
	"bytes"
	"fmt"
	"testing"
)

func TestSpliceFieldsPreservesAliasedPrefix(t *testing.T) {
	for _, prefixLen := range []int{0, 3, 32} {
		t.Run(fmt.Sprint(prefixLen), func(t *testing.T) {
			prefix := bytes.Repeat([]byte{0xa5}, prefixLen)
			packet := AppendVarint(nil, 1<<4|uint64(WireUint))
			packet = AppendVarint(packet, 42)
			packet = AppendVarint(packet, 2<<4|uint64(WireUint))
			packet = AppendVarint(packet, 99)
			storage := make([]byte, prefixLen+len(packet), prefixLen+len(packet)+512)
			copy(storage, prefix)
			copy(storage[prefixLen:], packet)
			appendDirty := func(dst []byte, fnum uint16) []byte {
				dst = AppendVarint(dst, uint64(fnum)<<4|uint64(WireUint))
				return AppendVarint(dst, 1<<40)
			}
			want := SpliceFields(append([]byte(nil), prefix...), packet, 1<<1, 2, appendDirty)
			got := SpliceFields(storage[:prefixLen], storage[prefixLen:], 1<<1, 2, appendDirty)
			if !bytes.Equal(got, want) {
				t.Fatalf("aliased splice: got %x, want %x", got, want)
			}
			if !bytes.Equal(storage[prefixLen:], packet) {
				t.Fatal("aliased splice modified source packet")
			}
		})
	}
}
