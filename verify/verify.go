// Command verify runs the matchlen.wasm kernel in a wazero embedded runtime
// against a table of test cases and prints PASS/FAIL. This is the end-to-end
// smoke test that the kernel does the right thing on real wasm execution — it
// does NOT require `//go:wasmimport` (which needs GOOS=wasip1/js) and can
// therefore run on the developer's native machine as a sanity check.
//
// Run with:
//
//	cd verify && go run . ../matchlen.wasm
//
// Requires:  github.com/tetratelabs/wazero. Not part of the module build —
// dev-time smoke test only.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/tetratelabs/wazero"
)

// envWasm is the trivial companion module (env.wat compiled) that just
// exports a 32-page (2 MiB) linear memory named "memory". The matchlen
// kernel imports (env, memory), so instantiating this as "env" first
// satisfies that import. 32 pages is enough for two 1 MiB inputs at
// offsets 0 and 0x100000 side by side. In production (Go host via
// //go:wasmimport) the Go wasm runtime provides its own linear memory;
// this env module is a test-only stand-in.
var envWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // magic + version
	0x05, 0x03, 0x01, 0x00, 0x20, // Memory section: 1 memory, min 32 pages
	0x07, 0x0a, 0x01, 0x06, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x02, 0x00, // Export "memory"
}

func main() {
	kernelPath := "matchlen.wasm"
	if len(os.Args) >= 2 {
		kernelPath = os.Args[1]
	}

	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	// Instantiate the env module (just exports memory) as "env" so the
	// kernel's (import "env" "memory") resolves. wazero picks up the module
	// name from the wasm's "module name" section — but our env module has
	// no name section. Provide it via InstantiateWithConfig.
	envMod, err := r.InstantiateWithConfig(ctx, envWasm,
		wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		die("instantiate env: %v", err)
	}
	mem := envMod.Memory()
	if mem == nil {
		die("env module has no memory")
	}

	// Read and instantiate the SIMD kernel.
	wasmBytes, err := os.ReadFile(kernelPath)
	if err != nil {
		die("read %s: %v", kernelPath, err)
	}
	kern, err := r.Instantiate(ctx, wasmBytes)
	if err != nil {
		die("instantiate kernel: %v", err)
	}
	fn := kern.ExportedFunction("matchlen16")
	if fn == nil {
		die("kernel does not export matchlen16")
	}

	// Test cases: (a, b, expected match length).
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"x", "x", 1},
		{"x", "y", 0},
		{"hello", "hello", 5},
		{"hello", "help!", 3},
		{"abcdefghijklmnop", "abcdefghijklmnop", 16},
		{"abcdefghijklmnop", "abcdefghijklmnoq", 15},
		{"abcdefghijklmnopq", "abcdefghijklmnopq", 17},
		{"abcdefghijklmnopqrstuvwxyz012345", "abcdefghijklmnopqrstuvwxyz012345", 32},
		{"abcdefghijklmnopqrstuvwxyz012345", "abcdefghijklmnopqrstuvwxyz01234!", 31},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaab", 51},
	}

	fail := 0
	for i, c := range cases {
		aOff := uint32(0)
		bOff := uint32(4096)
		mem.Write(aOff, []byte(c.a))
		mem.Write(bOff, []byte(c.b))
		limit := uint32(len(c.a))
		if uint32(len(c.b)) < limit {
			limit = uint32(len(c.b))
		}
		results, err := fn.Call(ctx, uint64(aOff), uint64(bOff), uint64(limit))
		if err != nil {
			fmt.Printf("[%d] ERR   %v\n", i, err)
			fail++
			continue
		}
		got := int(uint32(results[0]))
		status := "OK  "
		if got != c.want {
			status = "FAIL"
			fail++
		}
		fmt.Printf("[%d] %s matchlen(%q, %q, %d) = %d, want %d\n",
			i, status, snip(c.a), snip(c.b), limit, got, c.want)
	}
	if fail > 0 {
		fmt.Fprintf(os.Stderr, "\n%d case(s) failed\n", fail)
		os.Exit(1)
	}
	fmt.Println("\nAll cases passed — the wasm-SIMD kernel works end-to-end.")
}

func snip(s string) string {
	if len(s) > 20 {
		return s[:17] + "..."
	}
	return s
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "verify: "+format+"\n", args...)
	os.Exit(2)
}
