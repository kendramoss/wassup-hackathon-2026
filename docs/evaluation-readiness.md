# Evaluation readiness: eval-only gateway client and budget guard

This describes `backend/cmd/readiness-eval`, `backend/internal/readiness/budget` and `backend/internal/readiness/gateway`. They exist so a held-out evaluation (#275) and a manual smoke test (#276) can call a LiteLLM gateway under a hard shared spend cap. They are not part of the server: `LLM_PROVIDER=litellm` is still rejected by `internal/config` (D49), the adapter is never registered as a provider, and a test checks that the server build does not link these packages.

## What results from this tool mean

Results are labelled **LiteLLM/gateway-backed**. They are not native Vertex validation and cannot close the native-Vertex staging criterion of #276. Model identity is the gateway alias plus the resolved model id the gateway reports, checked against a verified allowlist.

## The evaluated task

The proposed #275 task is a **precedent-verifier check on CBP before/after natural experiments**: given the machine-extracted facts of an earlier ruling A and a later ruling B, does the existing verifier say the origin changes? It does not exercise move enumeration, ranking or economics, so it is not planner validation. Planner enumeration, ranking and cost estimates remain untested unless a separate estimand is defined and run. Origin accuracy on new single-origin rulings is a separate, secondary estimate. The protocol (eligibility, gold, leakage controls, hashes, reporting tiers) is kept with the evidence ledger and is reviewed independently before it is frozen. Public ruling retrieval was denied (HTTP 403) when attempted; the number of eligible rulings is unknown, not zero.

## Budget guard in short

- One raw HTTP attempt is one reservation, and all attempts (retries, tool turns, smoke) share the $20 cap.
- Money is integer nano-USD. Prices come only from a pricing-proof file: exact alias, the maximum of list, gateway-reported and worst-tier prices, verified recently, valid past the end of the run, with the resolved model ids listed. An unknown, stale or missing price refuses the call.
- Every request needs a non-zero output cap, and the proof must attest that the cap covers hidden reasoning. Billed output is the largest figure any usage field implies. Usage above the reserved bound, an unexpected resolved model, or a successful response with no usage is a sticky halt.
- The reservation is written and fsynced before the request is built into a network call. Failures, timeouts, redirects and any ambiguous outcome keep the full reservation. A crash leaves an open reservation that the next start charges in full and halts on until a person reconciles it against the gateway and runs `budget ack-halt`.
- The gateway transport needs a one-shot permit that only the guard can create (the permit has no exported constructor or fields, so a zero or foreign permit fails before the wire). Redirects are never followed, an environment proxy is refused (not cleared), only one HTTPS host and path is allowed, and HTTP/1.1 is used with keep-alives off. `http.DefaultTransport` is also replaced with a denying transport; that is a mitigation against accidental stdlib use, not a sandbox, so an OS-level egress allowlist to the single gateway host is still required before a live run.
- Known-unsent failures (encoding, URL allowlist, permit) cost nothing and do not count toward the failure streak. Anything ambiguous (dial, TLS, write, read, timeout) is charged in full. Ctrl-C cancels the run context and the in-flight attempt is settled (charged in full) before exit, so an interrupt needs no `ack-halt`; a hard kill is a crash and does.
- Sticky halts that need `budget ack-halt` after reconciliation: provider usage above the reserved bound, an unexpected resolved model, usage absent on a success, a crash with an open reservation, a tampered or conflicting shared file, ledger write failure, budget exhaustion (this process only), and three consecutive failures (this process only; `budget ack-failures --operator-approved --note ...` clears only the streak). An oversize request is refused for that request alone and is not a halt.
- A real call needs the frozen config to bind `pricing_proof_sha256` and `guard_commit` (checked at startup against the proof bytes and the binary's clean VCS revision) and a `run_ends_by` that the price proof covers.
- Concurrency is 1 (a semaphore in process and an exclusive lock across processes). The ledger must be on a local, non-synced disk.
- The attempts ledger is authoritative; `SHAREDBUDGET.json` is cross-checked and kept in step. External spend and external reservations are monotonic: a higher value in the file is counted and remembered in the ledger (also across restarts); a lower value stops the run and is never overwritten. Lowering `external_reserved_usd` is refused unless an audited reconciliation (not exposed on the command line) shows the spent amount rose by at least as much. Other payers of the same key (for example a CI smoke) are not visible to this guard unless a sub-allocation is written to the file's `external_reserved_usd`.
- The ledger hash chain detects accidents and edits, not an adversary who can rewrite the whole file.
- Real calls need `--allow-model-calls`, `calls_authorized: true` in the shared file, a pricing proof, and `LITELLM_BASE_URL` / `LITELLM_API_KEY` in the environment of that one process. The key is never read from files, printed, logged or stored.

## Commands

`readiness-eval budget status|ack-halt`, `readiness-eval smoke --dry-run`, `readiness-eval eval --dry-run --fixtures F --config C --out O`. The dry runs use a fake model and a temporary ledger. Real evaluation is intentionally not available until the protocol is frozen and reviewed.
