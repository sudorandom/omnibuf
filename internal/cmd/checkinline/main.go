// Command checkinline guards compiler inlining decisions for performance-critical
// functions. Run from the repository root, normally through just go-lint.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var required = []string{
	"github.com/sudorandom/omnibuf.decodeVarint",
	"github.com/sudorandom/omnibuf/wire.DecodeVarint",
}

func main() {
	// Ask the compiler, rather than trying to reproduce its evolving cost model.
	// Explicitly enable normal optimizations even if GOFLAGS contains debug flags.
	cmd := exec.Command("go", "build", "-pgo=off", "-gcflags=-m=2", ".", "./wire", "./internal/pb")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "checkinline: compiler failed: %v\n%s", err, output)
		os.Exit(1)
	}
	failures := check(string(output), required)
	for _, failure := range failures {
		fmt.Fprintln(os.Stderr, failure)
	}
	if len(failures) != 0 {
		os.Exit(1)
	}
	fmt.Printf("checkinline: %d hot functions remain inlineable\n", len(required))
}

func check(output string, functions []string) []string {
	inlineable := map[string]bool{}
	reasons := map[string]string{}
	var pkg string
	for line := range strings.SplitSeq(output, "\n") {
		if strings.HasPrefix(line, "# ") {
			pkg = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			continue
		}
		if _, diagnostic, ok := strings.Cut(line, ": can inline "); ok {
			name, _, _ := strings.Cut(diagnostic, " ")
			inlineable[pkg+"."+name] = true
		}
		if _, diagnostic, ok := strings.Cut(line, ": cannot inline "); ok {
			name, reason, _ := strings.Cut(diagnostic, ": ")
			reasons[pkg+"."+name] = reason
		}
	}
	var failures []string
	for _, name := range functions {
		if inlineable[name] {
			continue
		}
		reason := reasons[name]
		if reason == "" {
			reason = "compiler emitted no inlining decision (renamed function or changed diagnostic format?)"
		}
		failures = append(failures, fmt.Sprintf("checkinline: %s must remain inlineable: %s", name, reason))
	}
	return failures
}
