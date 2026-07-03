# Build matchlen.wasm from matchlen.wat using WABT's wat2wasm.
#
# Requires:  wat2wasm (WABT, https://github.com/WebAssembly/wabt)
#   darwin:   brew install wabt
#   debian:   apt install wabt
#   fedora:   dnf install wabt

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
