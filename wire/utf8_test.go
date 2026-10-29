package wire

import (
	"bytes"
	"math/rand/v2"
	"testing"
	"unicode/utf8"
)

func TestValidUTF8MatchesStandardLibrary(t *testing.T) {
	check := func(b []byte) {
		t.Helper()
		if got, want := ValidUTF8(b), utf8.Valid(b); got != want {
			t.Fatalf("ValidUTF8(%x) = %v, want %v", b, got, want)
		}
	}
	check(nil)
	// Exercise word-load boundaries, overlap, and every possible non-ASCII byte
	// at every position, including the final byte.
	for n := 0; n <= 80; n++ {
		b := bytes.Repeat([]byte{'a'}, n)
		check(b)
		for i := range b {
			for c := 128; c <= 255; c++ {
				b[i] = byte(c)
				check(b)
			}
			b[i] = 'a'
		}
		for _, suffix := range []string{"é", "世界", "🚀", "\xc0\x80", "\xed\xa0\x80", "\xf4\x90\x80\x80", "\xe2\x82"} {
			check(append(bytes.Clone(b), suffix...))
			check(append([]byte(suffix), b...))
		}
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for range 2000 {
		b := make([]byte, rng.IntN(4096))
		for i := range b {
			b[i] = byte(rng.Uint32())
		}
		check(b)
	}
}

func FuzzValidUTF8(f *testing.F) {
	for _, seed := range []string{"", "ASCII", "eu-central-1", "世界🚀", "\xff", "\xc0\x80", "aaaaaaaaaaaaaaa\xe2\x82"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		if got, want := ValidUTF8(b), utf8.Valid(b); got != want {
			t.Fatalf("ValidUTF8(%x) = %v, want %v", b, got, want)
		}
	})
}
