// Command verify runs the matchlen.wasm kernel in a wazero embedded runtime
// against the named cases and prints PASS/FAIL. It is the human-facing form of
// what kernel_test.go checks: the same loader, the same table, the same
// reference — so a person can look at the answers one by one without reading a
// test's output.
//
// It does NOT require //go:wasmimport (which needs GOOS=wasip1/js) and can
// therefore run on the developer's native machine.
//
// Run with:
//
//	cd verify && go run . ../matchlen.wasm
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/tetratelabs/wazero"
)

func main() {
	kernelPath := "matchlen.wasm"
	if len(os.Args) >= 2 {
		kernelPath = os.Args[1]
	}
	ctx := context.Background()
	k, err := loadKernel(ctx, kernelPath, wazero.NewRuntimeConfig())
	if err != nil {
		die("%v", err)
	}
	defer k.Close()

	fail := 0
	for i, c := range cases {
		got, err := k.matchLen([]byte(c.a), []byte(c.b))
		if err != nil {
			fmt.Printf("[%d] ERR   %v\n", i, err)
			fail++
			continue
		}
		status := "OK  "
		if got != c.want {
			status = "FAIL"
			fail++
		}
		fmt.Printf("[%d] %s matchlen(%q, %q) = %d, want %d\n",
			i, status, snip(c.a), snip(c.b), got, c.want)
	}
	if fail > 0 {
		fmt.Fprintf(os.Stderr, "\n%d case(s) failed\n", fail)
		os.Exit(1)
	}
	fmt.Printf("\nall %d cases passed\n", len(cases))
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "verify: "+format+"\n", args...)
	os.Exit(1)
}

// snip keeps a long input readable in the printed line.
func snip(s string) string {
	const max = 24
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
