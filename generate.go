// Regenerate matchlen.wat programmatically from go-asmgen/wasm.
//
// The kernel source of truth is now the go-asmgen/wasm/matchlen generator —
// running `go generate ./...` at the repo root pins the WAT to whatever
// exact byte-string that pinned generator version emits, and drives away
// any manual drift. The companion CI (.github/workflows/wasm-drift.yml)
// runs the same command and fails if the checked-in matchlen.wat is
// stale, so contributors cannot silently diverge.
//
// The generator prints to stdout; sh redirects into matchlen.wat. This
// file is deliberately package-scoped without a build tag so `go generate`
// picks it up on every host (the //go:build wasm on matchlen_wasm.go
// would exclude it during a non-wasm generate).

//go:generate sh -c "go run github.com/go-asmgen/wasm/matchlen@v0.1.0 > matchlen.wat"

package matchlenwasm
