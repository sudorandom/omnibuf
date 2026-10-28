// Command conformance-runner executes the self-contained OmniBuf conformance
// test suite against a target language conformance client binary over
// length-prefixed stdin/stdout frames (modeled after Protobuf's
// conformance-test-runner, protovalidate-conformance, and connectconformance).
//
// Usage:
//
//	conformance-runner [flags] <client-binary> [client-args...]
//	conformance-runner ./bin/conformance-go
//	conformance-runner node ./packages/omnibuf-node/dist/bin/conformance-es.js
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/sudorandom/omnibuf/internal/conformance"
)

func main() {
	corpusPath := flag.String("corpus", "", "Optional path to override the built-in embedded conformance_corpus.json")
	jsonOutput := flag.Bool("json", false, "Emit conformance summary as JSON")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] <client-binary> [client-args...]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}

	sum, err := conformance.RunSuiteWithOptions(conformance.RunOptions{
		AdapterBinary: flag.Arg(0),
		AdapterArgs:   flag.Args()[1:],
		CorpusPath:    *corpusPath,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "conformance-runner error: %v\n", err)
		os.Exit(1)
	}

	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(sum)
	} else {
		conformance.FormatSummary(os.Stdout, os.Stderr, sum)
	}

	if len(sum.Failures) > 0 {
		os.Exit(1)
	}
}
