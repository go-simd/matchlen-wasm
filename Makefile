# Build matchlen.wasm from matchlen.wat using WABT's wat2wasm.
# matchlen.wat itself is regenerated from the go-asmgen/wasm/matchlen
# emitter (pinned in generate.go) — do NOT edit matchlen.wat by hand.
#
# Requires:
#   go        (any modern release)
#   wat2wasm  (WABT, https://github.com/WebAssembly/wabt)
#     darwin: brew install wabt
#     debian: apt install wabt
#     fedora: dnf install wabt

.PHONY: gen
gen:
	# Regenerate matchlen.wat from the pinned generator version. The
	# CI wasm-drift job enforces that this stays in sync with what is
	# checked in.
	go generate ./...

matchlen.wat: generate.go
	$(MAKE) gen

matchlen.wasm: matchlen.wat
	# WABT 1.0.41+ enables the SIMD proposal by default. If you use an
	# older release, add `--enable-simd`.
	wat2wasm $< -o $@

.PHONY: clean
clean:
	rm -f matchlen.wasm

.PHONY: verify
verify: matchlen.wasm
	# Round-trip check: decompile back to .wat and compare structural equality.
	wasm2wat matchlen.wasm | diff -q - <(wat2wasm --enable-simd matchlen.wat -o - | wasm2wat -) || \
		(echo "round-trip mismatch"; exit 1)
	@echo "matchlen.wasm verified round-trip clean"

.PHONY: e2e
e2e: matchlen.wasm
	# Wazero-driven functional cross-check against a Go reference.
	cd verify && go run . ../matchlen.wasm
