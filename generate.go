// Regenerate matchlen.wat programmatically from the go-asmgen wasm
// generator. The kernel source of truth is the pinned generator in
// go-asmgen/asmgen — running `go generate ./...` at the repo root pins
// the WAT to whatever exact byte-string that pinned version emits, and
// drives away any manual drift. The companion CI
// (.github/workflows/wasm-drift.yml) runs the same command and fails if
// the checked-in matchlen.wat is stale, so contributors cannot silently
// diverge.
//
// The generator prints to stdout; sh redirects into matchlen.wat. This
// file is deliberately package-scoped without a build tag so `go generate`
// picks it up on every host (the //go:build wasm on matchlen_wasm.go
// would exclude it during a non-wasm generate).
//
// Migrated from the standalone github.com/go-asmgen/wasm module (pin was
// @v0.1.0) to the folded-in path in go-asmgen/asmgen@v0.6.0. Both paths
// resolve to byte-identical generator output; the standalone module's
// tags remain immutable, so this is a rebase for clarity, not a
// behavioural change.

//go:generate sh -c "go run github.com/go-asmgen/asmgen/examples/wasm/matchlen@v0.6.0 > matchlen.wat"

package matchlenwasm
