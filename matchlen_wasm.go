// Package matchlenwasm is the wasm-SIMD (v128) kernel for the
// go-simd/matchlen common-prefix primitive. The kernel lives in a small
// companion WebAssembly module (matchlen.wasm, compiled from matchlen.wat
// via wat2wasm) and is called from Go via //go:wasmimport.
//
// matchlen.wat is programmatically emitted by go-asmgen/wasm/matchlen —
// see generate.go for the pinned generator version. The regeneration is
// enforced in CI by .github/workflows/wasm-drift.yml, so the checked-in
// .wat / .wasm cannot silently diverge from the generator output.
//
// This is the seventh target for go-simd — after the 6 native architectures
// (amd64/arm64/riscv64/loong64/ppc64le/s390x) it adds js/wasm + wasip1/wasm.
// The Go compiler does not emit wasm-SIMD (v128) from Go code today; this
// scaffolding is the practical work-around for pure-Go SIMD on wasm.
//
// Deployment model
//
// The consumer instantiates matchlen.wasm at boot in the wasm HOST (a JS
// runtime, a WASI runtime like wazero, a wasmbox compositor…) and exposes
// the exported "matchlen16" function under the import name "env.matchlen16".
// The Go module then calls it via //go:wasmimport, passing linear-memory
// offsets to the input byte slices.
//
// See the README for the host-side JS/wazero example.
//
// Overhead vs SIMD gain
//
// Every wasmimport call crosses the wasm module boundary — cheap in modern
// V8/SpiderMonkey (~50ns per call) and cheaper still in wazero. The v128
// matchlen loop lands at ~4 bytes/cycle (i8x16.eq + all_true retire in one
// cycle each on modern browser engines), so the crossover point vs a Go
// scalar 8-byte-word loop is around 16 bytes of input — anything bigger
// starts to win. LZ4 match extension routinely runs into kilobyte-sized
// tails, so this kernel is a real gain in that consumer.

//go:build wasm

package matchlenwasm

// matchlen16 is the wasm-SIMD kernel imported from the companion module.
// The caller (MatchLen below) passes byte-slice base pointers as linear
// memory offsets. The Go wasm ABI translates a []byte header's data pointer
// into a wasm linear memory offset (i32) on this call, and the wasm module's
// (import "env" "memory") declaration shares Go's linear memory — so the
// module reads the slice bytes directly without a copy.
//
//go:wasmimport env matchlen16
func matchlen16(ptrA, ptrB uint32, limit uint32) uint32

// MatchLen returns the number of leading bytes that a and b share. Drop-in
// compatible with github.com/go-simd/matchlen.MatchLen — same result, same
// tolerance for pathological inputs.
//
// Sub-16-byte inputs bail out to the byte-tail loop inside the wasm kernel
// (still cheap because the whole call is one boundary crossing); the SIMD
// speed-up only shows up past 16 B.
func MatchLen(a, b []byte) int {
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	if limit == 0 {
		return 0
	}
	// Cast the slice headers' data pointers to wasm linear-memory offsets.
	// The Go wasm compiler represents Go pointers as i32 offsets into the
	// module's linear memory, so passing &a[0] here goes across the
	// wasmimport ABI cleanly. Empty-slice case (limit==0) is guarded above.
	return int(matchlen16(uint32(uintptr(_pin(a))), uint32(uintptr(_pin(b))), uint32(limit)))
}

// _pin returns the address of the underlying byte-slice buffer as an
// unsafe.Pointer, in the form the wasm import wants. Kept in a helper so
// the unsafe scope stays visible.
//
// The Go wasm runtime pins Go's linear memory for the duration of a
// wasmimport call (no GC compaction across the boundary), so passing a raw
// pointer for one call is safe. Longer-lived cross-module views would need
// syscall/js pinning or a memory-copy staging buffer.
//
//go:noinline
func _pin(b []byte) *byte {
	if len(b) == 0 {
		// Return a stable non-nil pointer for empty slices so the wasm side
		// can still do bounds checks. The kernel handles limit==0 with an
		// immediate return.
		var sentinel [1]byte
		return &sentinel[0]
	}
	return &b[0]
}
