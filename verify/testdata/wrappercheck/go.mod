// A module of its own, under testdata so the go tool leaves it alone: it is
// compiled by the wrapper test, for wasm, not by a build of this repository.
module wrappercheck

go 1.26.4

require github.com/go-simd/matchlen-wasm v0.0.0

replace github.com/go-simd/matchlen-wasm => ../../..
