# Data handling: logs, errors and case data (#285)

Status: describes the `internal/logsafe` helper and its wiring (#285). It is wired into `cmd/server` (root logger and ErrorLog), `internal/httpapi` (request log, health, API keys, dispatch, cases, teams, scenarios, products, hts, duty, limit, audit), `internal/llm` (call log), `internal/planner` (rejection reason) and `frontend/src/lib/backend.ts` (proxy log). Nothing here is a certification, a compliance claim, a retention guarantee or a statement about what a cloud or model provider does.

## Two regimes, never mixed

1. Logs, errors, debug output, metrics, audit and history summaries are minimised: a closed allowlist decides what may appear, everything else is dropped or masked.
2. Authorized case data, model input and requested exports stay exactly as the user supplied and the legal reasoning needs them: origin countries, HTS codes, process facts, costs, supplier names and so on are never redacted, masked, hashed or reordered. They are protected by authorization, tenant isolation, no logging, retention limits and deletion, not by rewriting them. `logsafe` must not be imported by code on those paths.

`logsafe` is not a PII detector. It does not scan free text. A user who must not share something with a service provider should not enter it (the privacy page says so).

## What `internal/logsafe` does

- Imports only the Go standard library, so any package may import it without a cycle. It knows no application type: an error describes itself with optional `LogClass() string` and `LogStatus() int` methods, which the helper validates against a closed set (`Classify`). Unknown errors are `unclassified`; `Error()` is never called.
- Wraps any `slog.Handler` (`NewHandler`, construct once; re-wrapping with a different policy is an error, never silently ignored). Unknown attribute keys and group names are dropped and only a constant count (`masked_attrs`) is kept. A known key with a value that fails its rule keeps the key and gets the constant `[masked]`. `LogValuer`, `Stringer`, struct and map values are not called or formatted unless they are the helper's own error value.
- The record message must be in a closed known-source set, otherwise it is `[message masked]`. A source guard test is additional proof, not the gate.
- Fails closed: a panic or failure while sanitising replaces the record with a constant `log record dropped` record.
- Identifiers: the `request_id`, `case` and `key_id` keys accept only the opaque `Ref` type, never a string, because the shape of an id (16 hex, `h_` plus 16 hex) cannot prove it was not planted by a client. `RequestRef` normalises the request id once at the top boundary: any client-supplied value becomes `h_` plus 16 hex of its SHA-256 (the form #284 stores); a server-minted id is kept as is and not hashed again. `MintedRef` wraps an id the server itself created. The one normalised value is what the log, the response header and the audit should all use.
- Value sets are closed: `finish_reason`, `regime`, `provider` and `method` are `OneOf` over the real sets. Model ids, tool names and route patterns have no default; they are registered at wiring (`AllowModels`, `AllowTools`, `AllowRoutes`) and masked until then. The shape helpers `Enum`, `ID`, `ModelID` and `ToolName` exist but no default key uses them.
- `ErrorLog` bridges `net/http`'s error log: each line becomes one constant record of class `stdlog`; the original text is never printed.
- No key, secret, environment variable or audit setting is read.

## Known limits

Closed sets and the opaque `Ref` are the only default string rules; shape patterns are not treated as a privacy proof. A key registered with a closed set can only carry those exact values. Shipping the helper changes no existing log line until it is wired.

## What is wired

Root logger, ErrorLog bridge and `slog.Default()` (cmd/server); request log, request id, matched route, audit request id, every `Logger` call in `internal/httpapi`; `llm.WithLogging`; planner rejection reason; health deep; NEW ask/rename history lines; the audit log lines (`audit write failed`, startup audit lines) with their closed fields kept; the Next proxy console lines.

Uploaded images and scanned PDFs (`POST /v1/products/extract-media`, `docs/MULTIMODAL.md`) are model input under rule 2: sent unchanged to the model, never logged. They are read into memory only and not stored or cached; the handler zeroes its buffers when the request ends (the encoded copy the model client sends is left to the garbage collector). `llm.Message.Parts` is never serialised. The route and the model call log under the same allowlist as every other route; `internal/httpapi/media_test.go` checks that no log line holds the bytes, the file names or the parts read.

## What is not wired, known limits, residual risk

1. Offline CLIs (`cmd/eval`, `cmd/demo`, `cmd/corpus`, `cmd/apikey`, `cmd/readiness-eval`) print to a terminal with their own loggers and `fmt.Fprint(os.Stderr, ...)`; not wired and not covered by the guard. They process public/synthetic eval input only, by protocol.
2. Fatal startup errors log the CLASS only (`unclassified` for most). The cause text (a path, a Firestore message) is intentionally not in the structured log; operators read it by running the binary in a terminal. The `credentialHint` fixed text still goes to stderr.
3. The net/http panic value and stack are no longer logged (constant `stdlog` record). Diagnosability cost accepted per contract; a panicking handler also emits no `request` line (unchanged from BASE).
4. `MintedRef` remains a caller assertion. It is confined to `logattrs.go` and the five call arguments are pinned by a test, but a future edit that passes a request-derived id to `caseAttr`/`keyAttr` would only be caught by that test, not by the type system.
5. The request reference is an UNKEYED SHA-256 (16 hex). It is log hygiene, not anonymisation; a low-entropy request id can be guessed. No claim of secrecy.
6. A provider-reported model version that is not `<configured id>-<1..8 digits>` is masked (intended operational loss). Tool names are the declared tools; a test pins `LogToolNames` to the declared tools. Health check names (`case_store`, `model`) and audit actions are closed sets in `httpapi.LogPolicy`; a new route, model, tool, key or message must be registered or it shows as `[masked]`/`masked_attrs` (the suite-wide `TestMain` and the startup test fail on that). New external or future callers that add unregistered attributes get masked defaults; I did not add a permissive shape fallback.
7. Existing stored history lines, rejection reasons, case content and caches are not rewritten or purged. `Event.Actor` keeps the raw email (authorized, shown to the case's workspace). Resetting a case clears its chat but not the earlier lines of its history. Deleting a case removes the case and its embedded history and chat. The separately stored content-keyed prediction, precedent-check and advice caches derived from its facts, and its supplier documents, are removed too when `CACHE_PURGE=on` (D953): each entry is linked to the cases that used it and deleted when no case does. Entries written before that flag was on, or for no case, are not linked and stay; so does a copy an entry shares with a case that still exists. Copies held outside the application (any backups, provider-side records) are not under the application's control and are not verified here.
8. The unit tests of the Go layer cover the real handler, mux, middleware, audit service (memory store) and a real `http.Server` over a loopback listener. Not verified: Cloud Logging, the deployed ingress or CDN, Firestore, a live provider, a browser. The frontend test drives the real proxy functions with a stubbed `fetch`; no browser or Next runtime log capture.
9. `X-Audit-Incomplete`, audit read routes, audit retention and the optional/OFF-without-key behaviour are untouched. The log policy reads no key and no audit state; audit on/off gives identical logging (tested for the startup lines and the request path).

## Operator notes

- A startup failure logs `fatal` with `err.class` only. To read the cause of a failed start, run the binary in a terminal or inspect the platform's revision status; the text is intentionally not in the structured log.
- Unknown or unregistered log attributes appear as `masked_attrs` counts and values as `[masked]`. New log keys, messages, routes, models or tools must be registered in `httpapi.LogPolicy` / `cmd/server/logging.go`; `TestMain` in `internal/httpapi` and `TestStartupLinesAreKeptUnderTheProductionPolicy` fail when a site is not registered.
- The audit trail is optional and independent: with `AUDIT=off` or no `AUDIT_ACTOR_KEY` the logging policy is identical; with audit on, the audit event carries the same request reference as the log line.
