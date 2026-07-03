;; env.wat — a trivial companion module that just exports a linear memory
;; under the name "memory". The matchlen kernel imports (env, memory), so
;; instantiating this module as "env" satisfies that import in a wazero-
;; embedded test.
;;
;; In production (Go host via //go:wasmimport) the memory the kernel sees is
;; Go's own linear memory; this env module is a test-only stand-in.
(module
  (memory (export "memory") 1))
