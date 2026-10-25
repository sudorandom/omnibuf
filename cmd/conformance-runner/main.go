// Command conformance-runner executes the language-independent OmniBuf
// conformance test suite against a target language adapter binary over
// length-prefixed stdin/stdout frames (modeled after Protobuf's
// conformance-test-runner).
//
// Usage:
//
//	conformance-runner [-corpus testdata/conformance_corpus.json] ./bin/conformance-go
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sudorandom/omnibuf/conformance"
)

func main() {
	corpusPath := flag.String("corpus", "testdata/conformance_corpus.json", "Path to conformance_corpus.json")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] <adapter-binary>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	adapterBinary := flag.Arg(0)

	sum, err := conformance.RunSuite(adapterBinary, *corpusPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "conformance-runner error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("OmniBuf Conformance Suite (%s)\n", sum.AdapterPath)
	fmt.Printf("  %-24s %4d passed\n", conformance.TestTierPlugin.String()+":", sum.ByTier[conformance.TestTierPlugin])
	fmt.Printf("  %-24s %4d passed\n", conformance.TestTierProtobufTypes.String()+":", sum.ByTier[conformance.TestTierProtobufTypes])
	fmt.Printf("  %-24s %4d passed\n", conformance.TestTierAnnotatedStruct.String()+":", sum.ByTier[conformance.TestTierAnnotatedStruct])
	fmt.Printf("  %-24s %4d passed\n", conformance.TestTierBlind.String()+":", sum.ByTier[conformance.TestTierBlind])
	fmt.Printf("  Total: %d passed, %d skipped, %d failed (out of %d cases)\n",
		sum.PassedCases, sum.Skipped, len(sum.Failures), sum.TotalCases)

	if len(sum.Failures) > 0 {
		fmt.Fprintln(os.Stderr, "\nCONFORMANCE FAILURES:")
		for _, f := range sum.Failures {
			fmt.Fprintf(os.Stderr, "  - %s\n", f)
		}
		os.Exit(1)
	}
}
