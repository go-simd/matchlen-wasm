// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"math/rand"
	"testing"

	"github.com/tetratelabs/wazero"
)

const kernelPath = "../matchlen.wasm"

// backends runs a test against both of wazero's execution engines. They are
// different implementations of the same v128 instructions -- one compiles to
// host code on amd64/arm64, the other interprets -- so a kernel that agrees
// with the reference under one and not the other has found a real
// disagreement, in the kernel or in an engine. Running only the default would
// silently test whichever one the runner happens to have.
func backends(t *testing.T, run func(t *testing.T, k *kernel)) {
	t.Helper()
	for _, tc := range []struct {
		name string
		cfg  wazero.RuntimeConfig
	}{
		{"default engine", wazero.NewRuntimeConfig()},
		{"interpreter", wazero.NewRuntimeConfigInterpreter()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k, err := loadKernel(context.Background(), kernelPath, tc.cfg)
			if err != nil {
				t.Fatalf("loading the kernel: %v", err)
			}
			defer k.Close()
			run(t, k)
		})
	}
}

// The named cases: the empty pair, the boundaries either side of one v128
// lane, and a long run that ends one byte before the end.
func TestTheNamedCases(t *testing.T) {
	backends(t, func(t *testing.T, k *kernel) {
		for _, c := range cases {
			got, err := k.matchLen([]byte(c.a), []byte(c.b))
			if err != nil {
				t.Errorf("matchlen(%q, %q): %v", snip(c.a), snip(c.b), err)
				continue
			}
			if got != c.want {
				t.Errorf("matchlen(%q, %q) = %d, want %d", snip(c.a), snip(c.b), got, c.want)
			}
			// The table's own answer has to be the reference's answer, or
			// the table is the thing being tested.
			if want := scalarMatchLen([]byte(c.a), []byte(c.b)); want != c.want {
				t.Errorf("the table says matchlen(%q, %q) is %d; the reference says %d",
					snip(c.a), snip(c.b), c.want, want)
			}
		}
	})
}

// A table of eleven cases cannot cover a kernel that reads sixteen bytes at a
// time: what matters is where the mismatch falls relative to a lane boundary,
// for every length and every offset. So the kernel is compared with the
// reference on inputs that put the mismatch at each position in turn, and then
// on random ones.
func TestAgainstTheReference(t *testing.T) {
	backends(t, func(t *testing.T, k *kernel) {
		// Every mismatch position, for lengths that straddle one, two and
		// three lanes plus a tail.
		for _, n := range []int{0, 1, 15, 16, 17, 31, 32, 33, 47, 48, 63, 64, 65, 129} {
			a := make([]byte, n)
			for i := range a {
				a[i] = byte(i*31 + 7)
			}
			for pos := 0; pos <= n; pos++ {
				b := append([]byte(nil), a...)
				if pos < n {
					b[pos] ^= 0xFF
				}
				got, err := k.matchLen(a, b)
				if err != nil {
					t.Fatalf("len %d, mismatch at %d: %v", n, pos, err)
				}
				if want := scalarMatchLen(a, b); got != want {
					t.Errorf("len %d, mismatch at %d: kernel says %d, reference says %d", n, pos, got, want)
				}
			}
		}

		// …and inputs nobody chose. A fixed seed so a failure can be
		// re-run: a test that cannot be repeated cannot be bisected.
		rng := rand.New(rand.NewSource(20260907))
		for i := 0; i < 2000; i++ {
			n := rng.Intn(600)
			a := make([]byte, n)
			b := make([]byte, n)
			rng.Read(a)
			copy(b, a)
			// Mostly-equal inputs, because that is what a match-extension
			// caller feeds it; a purely random pair almost always differs
			// in the first lane and would exercise nothing else.
			if n > 0 && rng.Intn(4) > 0 {
				b[rng.Intn(n)] ^= byte(1 + rng.Intn(255))
			}
			// Different lengths on a quarter of the runs: the kernel takes
			// the shorter one as its limit.
			if n > 1 && rng.Intn(4) == 0 {
				b = b[:rng.Intn(n)]
			}
			got, err := k.matchLen(a, b)
			if err != nil {
				t.Fatalf("case %d (len %d/%d): %v", i, len(a), len(b), err)
			}
			if want := scalarMatchLen(a, b); got != want {
				t.Fatalf("case %d: kernel says %d, reference says %d, for %d and %d bytes",
					i, got, want, len(a), len(b))
			}
		}
	})
}

// The kernel must stop at the limit it was given, not at the first mismatch it
// can find. The bytes past the limit are made to MATCH, so a kernel that reads
// past the end returns a number bigger than the limit rather than the same one
// by luck.
func TestItStopsAtTheLimit(t *testing.T) {
	backends(t, func(t *testing.T, k *kernel) {
		const n = 64
		a := make([]byte, n)
		b := make([]byte, n)
		for i := range a {
			a[i], b[i] = 0xAB, 0xAB
		}
		for limit := 0; limit <= n; limit++ {
			got, err := k.matchLen(a[:limit], b[:limit])
			if err != nil {
				t.Fatalf("limit %d: %v", limit, err)
			}
			if got != limit {
				t.Errorf("limit %d: kernel says %d", limit, got)
			}
		}
	})
}

// A kernel that cannot be loaded is a failure with a name, not a panic.
func TestLoadingSomethingThatIsNotTheKernel(t *testing.T) {
	ctx := context.Background()
	if _, err := loadKernel(ctx, "does-not-exist.wasm", wazero.NewRuntimeConfig()); err == nil {
		t.Error("loadKernel accepted a path that is not there")
	}
	if _, err := loadKernel(ctx, "env.wat", wazero.NewRuntimeConfig()); err == nil {
		t.Error("loadKernel accepted a file that is not wasm")
	}
}
