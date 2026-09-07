// SPDX-License-Identifier: BSD-3-Clause

// Command wrappercheck is the program the wrapper test RUNS. It is compiled
// for GOOS=wasip1 GOARCH=wasm and executed under wazero, which is the only way
// the //go:wasmimport wrapper in matchlen_wasm.go is ever executed rather than
// merely compiled.
//
// It reads comma-separated pairs of hex-encoded byte strings from stdin, one
// pair per line,
// calls MatchLen on each, and prints the answer. The comparing is done by the
// test on the other side of the boundary: this program decides nothing, so it
// cannot agree with itself.
package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	matchlenwasm "github.com/go-simd/matchlen-wasm"
)

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<20)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		// Comma-separated, not space-separated: an empty input encodes to an
		// empty string, and two of those round a space is a line that trims
		// away to nothing. The empty pair is a case that matters -- it is the
		// one the wrapper answers without crossing the boundary at all.
		hexA, hexB, ok := strings.Cut(line, ",")
		if !ok {
			fmt.Fprintf(os.Stderr, "wrappercheck: %q is not a pair\n", line)
			os.Exit(1)
		}
		a, err := hex.DecodeString(hexA)
		if err != nil {
			fmt.Fprintln(os.Stderr, "wrappercheck:", err)
			os.Exit(1)
		}
		b, err := hex.DecodeString(hexB)
		if err != nil {
			fmt.Fprintln(os.Stderr, "wrappercheck:", err)
			os.Exit(1)
		}
		fmt.Fprintln(out, matchlenwasm.MatchLen(a, b))
	}
	if err := in.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "wrappercheck:", err)
		os.Exit(1)
	}
}
