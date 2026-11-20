# Data handling: logs, errors and case data (#285)

Status: describes the `internal/logsafe` helper added by the first #285 change. The helper is NOT yet wired into the server, the model wrapper, the HTTP middleware or any call site; those changes follow in later, separately reviewed units. Nothing here is a certification, a compliance claim, a retention guarantee or a statement about what a cloud or model provider does.

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
