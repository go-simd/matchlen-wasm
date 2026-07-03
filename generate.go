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
// The wasm surface used to live in a standalone module; it is now
// folded into go-asmgen/asmgen as a peer package. This pin points at
// go-asmgen/asmgen@v0.6.0 (the first tag that includes the fold).

//go:generate sh -c "go run github.com/go-asmgen/asmgen/examples/wasm/matchlen@v0.6.0 > matchlen.wat"

package matchlenwasm
