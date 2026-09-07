// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/bits"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// envWasm is the trivial companion module (env.wat compiled) that just exports
// a 32-page (2 MiB) linear memory named "memory". The matchlen kernel imports
// (env, memory), so instantiating this as "env" first satisfies that import.
// 32 pages is enough for two 1 MiB inputs at offsets 0 and 0x100000 side by
// side. In production (a Go host via //go:wasmimport) the Go wasm runtime
// provides its own linear memory; this env module is a stand-in for it.
var envWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // magic + version
	0x05, 0x03, 0x01, 0x00, 0x20, // Memory section: 1 memory, min 32 pages
	0x07, 0x0a, 0x01, 0x06, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x02, 0x00, // Export "memory"
}

// The two inputs sit far enough apart in the stand-in memory that a kernel
// reading past its limit runs into the other one rather than into itself,
// which is what makes an over-read show up as a wrong answer.
const (
	offA = uint32(0)
	offB = uint32(1 << 20)
)

// A kernel is matchlen.wasm loaded into a wasm runtime, with the memory it
// reads its inputs from.
type kernel struct {
	ctx context.Context
	fn  api.Function
	mem api.Memory
	rt  wazero.Runtime
}

// loadKernel instantiates the kernel at path. cfg chooses the runtime: the
// compiler backend where the host has one, or the interpreter — and the two
// are worth running separately, because a SIMD kernel that agrees with the
// reference under one and not the other is a bug in something.
func loadKernel(ctx context.Context, path string, cfg wazero.RuntimeConfig) (*kernel, error) {
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	envMod, err := r.InstantiateWithConfig(ctx, envWasm,
		wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		r.Close(ctx)
		return nil, fmt.Errorf("instantiate env: %w", err)
	}
	mem := envMod.Memory()
	if mem == nil {
		r.Close(ctx)
		return nil, fmt.Errorf("the env module has no memory")
	}
	wasmBytes, err := os.ReadFile(path)
	if err != nil {
		r.Close(ctx)
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	mod, err := r.Instantiate(ctx, wasmBytes)
	if err != nil {
		r.Close(ctx)
		return nil, fmt.Errorf("instantiate kernel: %w", err)
	}
	fn := mod.ExportedFunction("matchlen16")
	if fn == nil {
		r.Close(ctx)
		return nil, fmt.Errorf("the kernel does not export matchlen16")
	}
	return &kernel{ctx: ctx, fn: fn, mem: mem, rt: r}, nil
}

func (k *kernel) Close() { k.rt.Close(k.ctx) }

// matchLen writes the two inputs into the module's memory and calls the
// kernel, exactly as a host would.
func (k *kernel) matchLen(a, b []byte) (int, error) {
	if !k.mem.Write(offA, a) || !k.mem.Write(offB, b) {
		return 0, fmt.Errorf("inputs of %d and %d bytes do not fit the stand-in memory", len(a), len(b))
	}
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	res, err := k.fn.Call(k.ctx, uint64(offA), uint64(offB), uint64(limit))
	if err != nil {
		return 0, err
	}
	return int(uint32(res[0])), nil
}

// scalarMatchLen is the reference implementation: the 8-byte-word XOR +
// TrailingZeros trick the go-simd/matchlen generic fallback uses. It is both
// the benchmark's baseline and the answer the kernel has to agree with.
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

// cases are the inputs worth naming: the empty pair, the boundaries either
// side of one v128 lane, and a long run that ends one byte before the end.
var cases = []struct {
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
