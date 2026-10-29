package main

import (
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	const output = `# example/wire
wire.go:71:6: can inline DecodeVarint with cost 80 as: func([]byte) (uint64, int)
# example
omnibuf.go:73:6: cannot inline decodeVarint: function too complex: cost 82 exceeds budget 80
`
	for _, tc := range []struct {
		name      string
		functions []string
		want      string
	}{
		{"inlineable", []string{"example/wire.DecodeVarint"}, ""},
		{"over budget", []string{"example.decodeVarint"}, "cost 82 exceeds budget 80"},
		{"missing", []string{"example.Missing"}, "no inlining decision"},
		{"wrong package", []string{"other.DecodeVarint"}, "no inlining decision"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failures := check(output, tc.functions)
			if tc.want == "" {
				if len(failures) != 0 {
					t.Fatal(failures)
				}
				return
			}
			if len(failures) != 1 || !strings.Contains(failures[0], tc.want) {
				t.Fatalf("got %v, want %q", failures, tc.want)
			}
		})
	}
}
