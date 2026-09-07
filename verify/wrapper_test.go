// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// The //go:wasmimport wrapper in matchlen_wasm.go compiles for wasm and
// nothing else, so every other check in this repository can only BUILD it. Yet
// it is where the pointers come from: it turns two Go byte slices into linear
// memory offsets and a limit, and if it gets that wrong the kernel reads the
// wrong bytes and no amount of testing the kernel would say so.
//
// So this compiles a wasip1 program that calls MatchLen, runs it under wazero,
// and answers its env.matchlen16 import from the HOST -- reading the bytes at
// the offsets the wrapper passed, out of the Go module's own memory, and
// returning the reference answer for them.
//
// That is what makes it a test of the wrapper rather than of the kernel: the
// host trusts the offsets, so wrong offsets mean wrong bytes and a wrong
// answer. The kernel is checked separately, against the same reference, in
// kernel_test.go.
func TestTheWasmimportWrapperRuns(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the wrapper has to be compiled for wasm, and there is no go here")
	}
	wasm := buildWrapperCheck(t)

	// The pairs, and what the reference says about them. Sizes either side of
	// a v128 lane, an empty pair (the wrapper's own special case, where there
	// is no first element to take the address of), and a pair of different
	// lengths (the wrapper computes the limit, not the kernel).
	type pair struct{ a, b []byte }
	pairs := []pair{
		{nil, nil},
		{[]byte("x"), []byte("x")},
		{[]byte("x"), []byte("y")},
		{[]byte("hello"), []byte("help!")},
		{bytes.Repeat([]byte("ab"), 8), bytes.Repeat([]byte("ab"), 8)},
		{bytes.Repeat([]byte("ab"), 8), append(bytes.Repeat([]byte("ab"), 7), "az"...)},
		{[]byte("abcdefghijklmnopq"), []byte("abcdefghijklmnopq")},
		{[]byte("the same start, then not"), []byte("the same start, THEN NOT")},
		{bytes.Repeat([]byte{7}, 300), bytes.Repeat([]byte{7}, 300)},
		{bytes.Repeat([]byte{7}, 300), bytes.Repeat([]byte{7}, 40)},
		{bytes.Repeat([]byte{7}, 40), bytes.Repeat([]byte{7}, 300)},
	}
	rng := rand.New(rand.NewSource(20260907))
	for i := 0; i < 60; i++ {
		n := rng.Intn(200)
		a := make([]byte, n)
		rng.Read(a)
		b := append([]byte(nil), a...)
		if n > 0 && rng.Intn(3) > 0 {
			b[rng.Intn(n)] ^= byte(1 + rng.Intn(255))
		}
		pairs = append(pairs, pair{a, b})
	}

	var in bytes.Buffer
	for _, p := range pairs {
		fmt.Fprintf(&in, "%s,%s\n", hex.EncodeToString(p.a), hex.EncodeToString(p.b))
	}

	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	// env.matchlen16, answered by the host: read what the wrapper pointed at,
	// out of the calling module's memory, and say what the reference says.
	var calls int
	_, err := r.NewHostModuleBuilder("env").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, ptrA, ptrB, limit uint32) uint32 {
			calls++
			a, okA := m.Memory().Read(ptrA, limit)
			b, okB := m.Memory().Read(ptrB, limit)
			if !okA || !okB {
				t.Errorf("the wrapper pointed outside the module's memory: a=%#x b=%#x limit=%d", ptrA, ptrB, limit)
				return 0
			}
			return uint32(scalarMatchLen(a, b))
		}).
		Export("matchlen16").
		Instantiate(ctx)
	if err != nil {
		t.Fatalf("host module: %v", err)
	}

	var out, errOut bytes.Buffer
	cfg := wazero.NewModuleConfig().WithStdin(&in).WithStdout(&out).WithStderr(&errOut).WithArgs("wrappercheck")
	if _, err := r.InstantiateWithConfig(ctx, wasm, cfg); err != nil {
		t.Fatalf("running the wasip1 program: %v\n%s", err, errOut.String())
	}
	if errOut.Len() > 0 {
		t.Errorf("the program complained: %s", errOut.String())
	}

	lines := strings.Fields(out.String())
	if len(lines) != len(pairs) {
		t.Fatalf("got %d answers for %d pairs\n%s", len(lines), len(pairs), out.String())
	}
	for i, p := range pairs {
		got, err := strconv.Atoi(lines[i])
		if err != nil {
			t.Fatalf("answer %d is %q: %v", i, lines[i], err)
		}
		if want := scalarMatchLen(p.a, p.b); got != want {
			t.Errorf("pair %d (%d and %d bytes): the wrapper produced %d, the reference says %d",
				i, len(p.a), len(p.b), got, want)
		}
	}

	// A pair with nothing to compare must NOT cross the boundary: the wrapper
	// answers 0 itself, which is the only reason taking the address of a first
	// element is safe for all the others. The count is derived rather than
	// written down -- the random pairs contain empty ones too.
	crossings := 0
	for _, p := range pairs {
		if len(p.a) > 0 && len(p.b) > 0 {
			crossings++
		}
	}
	if calls != crossings {
		t.Errorf("the kernel was called %d times; %d of the %d pairs have something to compare", calls, crossings, len(pairs))
	}
}

// buildWrapperCheck compiles testdata/wrappercheck for wasip1. It is a module
// of its own, with a replace pointing back here, so it compiles the wrapper in
// THIS working tree rather than a published one.
func buildWrapperCheck(t *testing.T) []byte {
	t.Helper()
	out := filepath.Join(t.TempDir(), "wrappercheck.wasm")
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = "testdata/wrappercheck"
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0", "GOWORK=off")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the wasip1 program: %v\n%s", err, b)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
