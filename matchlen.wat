;; matchlen — WebAssembly SIMD (v128) kernel for the LZ-family common-prefix
;; primitive. Compiled with wat2wasm (WABT) into matchlen.wasm and imported by
;; the Go host via //go:wasmimport (see matchlen_wasm.go).
;;
;; API surface:
;;   (func $matchlen16 (param $ptrA i32) (param $ptrB i32) (param $limit i32) (result i32))
;;   → returns the number of leading bytes in [ptrA..ptrA+limit) that match
;;     the corresponding bytes at [ptrB..ptrB+limit).
;;
;; Algorithm: process the input in 16-byte chunks using v128.load + i8x16.eq +
;; v128.any_true. On the first chunk with a mismatch, we bail out to a byte
;; loop that finds the exact first-mismatch offset (the v128.any_true test
;; can't tell us WHICH byte differs; a follow-up would use i8x16.bitmask +
;; i32.ctz on browsers but the Go wasm ABI can't easily surface bitmask, so
;; we fall back to byte comparison on the mismatch chunk).
;;
;; Memory model: the module imports memory from the Go host. The host's Go
;; slice headers are (ptr, len, cap) triples; we pass ptr + effective limit.
;; Both slices must be in the same linear memory (Go's default: the module
;; exports linear memory 0).

(module
  ;; Import memory from the Go host. The Go wasm runtime shares a single
  ;; linear memory across the module and the JS host; importing it lets the
  ;; kernel read the byte slices the Go caller allocated.
  (import "env" "memory" (memory 0))

  ;; matchlen16 — SIMD-accelerated common-prefix length.
  (func $matchlen16 (export "matchlen16")
        (param $ptrA i32) (param $ptrB i32) (param $limit i32)
        (result i32)

    (local $i i32)                  ;; running byte offset into the chunk
    (local $matched i32)            ;; running matched count (final return)
    (local $chunk i32)              ;; 16-byte-aligned chunk boundary
    (local $eq v128)                ;; per-byte equality mask

    (local.set $matched (i32.const 0))
    (local.set $i (i32.const 0))

    ;; Process 16-byte chunks. Load one v128 from each slice, XOR-equality
    ;; per-byte, then v128.any_true to detect any mismatch in the chunk.
    ;;
    ;;   for (; i + 16 <= limit; i += 16) {
    ;;     v128 a = *(ptrA + i);
    ;;     v128 b = *(ptrB + i);
    ;;     v128 eq = a == b (i8x16.eq);
    ;;     if (any lane of eq is 0) { break; }
    ;;     matched += 16;
    ;;   }
    (block $chunk_done
      (loop $chunk_loop
        ;; if i + 16 > limit, exit
        (br_if $chunk_done
          (i32.gt_s
            (i32.add (local.get $i) (i32.const 16))
            (local.get $limit)))

        ;; eq = i8x16.eq( v128.load(A + i), v128.load(B + i) )
        (local.set $eq
          (i8x16.eq
            (v128.load (i32.add (local.get $ptrA) (local.get $i)))
            (v128.load (i32.add (local.get $ptrB) (local.get $i)))))

        ;; if NOT all_true(eq) — at least one mismatch in this chunk — break
        ;; out of the fast loop and let the byte tail find the exact offset.
        (br_if $chunk_done
          (i32.eqz (i8x16.all_true (local.get $eq))))

        ;; Whole 16-byte chunk matched. Advance.
        (local.set $matched (i32.add (local.get $matched) (i32.const 16)))
        (local.set $i (i32.add (local.get $i) (i32.const 16)))
        (br $chunk_loop)))

    ;; Byte tail: find the exact first-mismatch offset from where the fast
    ;; loop bailed out. Covers both (a) the sub-16 remainder after the last
    ;; matching chunk and (b) the specific mismatching byte inside a chunk
    ;; the fast loop broke on.
    (block $byte_done
      (loop $byte_loop
        ;; if matched >= limit, done
        (br_if $byte_done
          (i32.ge_s (local.get $matched) (local.get $limit)))

        ;; if bytes differ, done
        (br_if $byte_done
          (i32.ne
            (i32.load8_u (i32.add (local.get $ptrA) (local.get $matched)))
            (i32.load8_u (i32.add (local.get $ptrB) (local.get $matched)))))

        (local.set $matched (i32.add (local.get $matched) (i32.const 1)))
        (br $byte_loop)))

    (local.get $matched)))
