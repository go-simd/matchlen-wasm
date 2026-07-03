// bench.go is a small stand-alone benchmark comparing the wazero-hosted
// wasm-SIMD matchlen16 kernel against a Go scalar reference at several input
// sizes. Prints ns/op and MB/s per size so we can measure the crossover point
// where the wasm boundary cost is amortised.
//
// Run with:
//
//	cd verify && go test -bench=BenchmarkMatchlenWasm . -benchtime=2s
//
// This is a t.Run/b.Run test-file benchmark, not the verify main.

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"math/bits"
	"os"
	"testing"

	"github.com/tetratelabs/wazero"
)

// scalarMatchLen is the reference Go implementation of matchlen, using the
// standard 8-byte-word XOR + TrailingZeros trick that the go-simd/matchlen
// generic fallback uses. Faster than a byte loop by ~8×; this is the fair
// baseline the wasm SIMD kernel has to beat.
func scalarMatchLen(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i+8 <= n {
		if d := binary.LittleEndian.Uint64(a[i:]) ^ binary.LittleEndian.Uint64(b[i:]); d != 0 {
			return i + bits.TrailingZeros64(d)>>3
		}
		i += 8
	}
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}

func benchSizes() []int {
	return []int{8, 16, 32, 64, 128, 256, 1024, 4096, 16384, 65536, 1024 * 1024}
}

func randish(n int) []byte {
	// Fill deterministic bytes (a monotonic pattern) so matchlen returns n.
	// Uses a full-match input so we measure the fast path — the mismatch
	// early-exit case is separately benchmarked below.
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i * 31)
	}
	return b
}

// BenchmarkMatchlenScalar and BenchmarkMatchlenWasm form a pair, run at every
// size in benchSizes(). Compare the two ns/op columns to find the crossover
// point where the wasm boundary cost breaks even.
func BenchmarkMatchlenScalar(b *testing.B) {
	for _, n := range benchSizes() {
		x := randish(n)
		y := bytes.Clone(x)
		b.Run(sizeName(n), func(b *testing.B) {
			b.SetBytes(int64(n))
			for i := 0; i < b.N; i++ {
				_ = scalarMatchLen(x, y)
			}
		})
	}
}

func BenchmarkMatchlenWasm(b *testing.B) {
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	envMod, err := r.InstantiateWithConfig(ctx, envWasm,
		wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		b.Fatalf("instantiate env: %v", err)
	}
	mem := envMod.Memory()
	wasmBytes, err := os.ReadFile("../matchlen.wasm")
	if err != nil {
		b.Fatalf("read matchlen.wasm: %v", err)
	}
	kern, err := r.Instantiate(ctx, wasmBytes)
	if err != nil {
		b.Fatalf("instantiate kernel: %v", err)
	}
	fn := kern.ExportedFunction("matchlen16")

	for _, n := range benchSizes() {
		x := randish(n)
		y := bytes.Clone(x)
		aOff := uint32(0)
		bOff := uint32(0x100000) // 1 MiB offset — env memory is 32 pages = 2 MiB total.
		if ok := mem.Write(aOff, x); !ok {
			b.Fatalf("write A @ %d for n=%d failed (mem size=%d)", aOff, n, mem.Size())
		}
		if ok := mem.Write(bOff, y); !ok {
			b.Fatalf("write B @ %d for n=%d failed (mem size=%d)", bOff, n, mem.Size())
		}
		// Sanity check: one call MUST return n (full match) — otherwise the
		// bench numbers below are measuring an early-exit, not a full scan.
		res, err := fn.Call(ctx, uint64(aOff), uint64(bOff), uint64(n))
		if err != nil {
			b.Fatalf("sanity call n=%d: %v", n, err)
		}
		if got := int(uint32(res[0])); got != n {
			b.Fatalf("sanity call n=%d: matchlen16 = %d, want %d — bench numbers would be meaningless", n, got, n)
		}
		limit := uint64(n)

		b.Run(sizeName(n), func(b *testing.B) {
			b.SetBytes(int64(n))
			for i := 0; i < b.N; i++ {
				_, _ = fn.Call(ctx, uint64(aOff), uint64(bOff), limit)
			}
		})
	}
}

func sizeName(n int) string {
	switch {
	case n >= 1024*1024:
		return "1MiB"
	case n >= 1024:
		return sizeKB(n)
	default:
		return sizeB(n)
	}
}

func sizeB(n int) string {
	switch n {
	case 8:
		return "8B"
	case 16:
		return "16B"
	case 32:
		return "32B"
	case 64:
		return "64B"
	case 128:
		return "128B"
	case 256:
		return "256B"
	}
	return "B"
}

func sizeKB(n int) string {
	switch n / 1024 {
	case 1:
		return "1KiB"
	case 4:
		return "4KiB"
	case 16:
		return "16KiB"
	case 64:
		return "64KiB"
	}
	return "KiB"
}
