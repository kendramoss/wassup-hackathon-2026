# API security and limits

What protects the public API, as built (D137 and the decisions it cites). Defaults are the code's defaults; the environment variable that changes each one is named. Everything here is enforced by the backend unless it says frontend.

## How the API is reached

- The backend runs on Cloud Run and only the frontend's service account may invoke it (Cloud Run IAM). Brokers and browsers reach it through the frontend's `/api/v1/...` proxy (`frontend/src/lib/backend.ts`), which forwards an allowlist of headers: `Content-Type`, `Accept`, `X-API-Key`, `X-Review-Token`, plus `X-Client-IP`, `X-Visitor-ID`, `X-User-Token` it sets itself, and a Google ID token as `Authorization: Bearer` on Cloud Run.
- `REQUIRE_BEARER=on` (default off) makes the backend refuse, with 401, any request other than `/health` that has no `Authorization: Bearer` header. It is defence in depth behind Cloud Run IAM. It is off by default because it has not been run against the deployed service; turn it on after checking that Cloud Run passes the header through to the container.
- CORS: the API sends no CORS headers, on any route. It is for server-to-server calls with `X-API-Key` and for the frontend's own proxy. Calls from a browser page on another site are not supported.

## Who the caller is

| Mode | Header | Notes |
|---|---|---|
| Anonymous visitor | `X-Visitor-ID` (set by the proxy from an httpOnly cookie) | Scopes which cases a visitor sees with `CASE_OWNERSHIP=on`. Not authentication: `STRICT_AUTH=on` refuses it alone for private case operations. |
| Signed-in user | `X-User-Token` (Google ID token, `TEAMS=on`) | Verified on every request: signature, expiry, issuer, audience, verified email (D68). |
| API key | `X-API-Key: op_...` (`API_KEYS=on`) | Only a SHA-256 of the key is stored. Unknown or disabled: 401 `unauthorized`, never a silent fall back to anonymous (D69). |
| Admin | `X-Admin-Token` | Seeding, pinning, the batch watch, issuing keys. Not limited. Never sent from the browser. |
| Broker review link | `X-Review-Token: ubr_...` (on `/v1/review*` only) | Opens one frozen draft and answers it; no identity on any other route (D200, below). |

## Rate limits

Every `/v1` route has a rate class. A test (`TestEveryV1RouteHasALimitClass`) fails if a route is registered without one. Limits are per client: the API key when one is sent, otherwise the address the proxy forwards as `X-Client-IP` (IPv6 grouped by /64), otherwise the connection's address.

| Class | Routes | Per client | Counted |
|---|---|---|---|
| basic | case list, get, rename, delete, economics, export; demos; a ruling by number; duty rates; eval summary; workspaces; the `POST /mcp` request itself | 120 a minute (`RATE_BASIC_PER_MIN`) | on each instance |
| light | predict, answers, ask, extract, extract-media, import, money prefill, HTS suggest, cost estimate, family check, scenarios, watch, BOM version, plan estimate, audit reads | 20 a minute (`RATE_LIGHT_PER_MIN`) | shared on Cloud Run |
| heavy | plan, compare targets | 4 a minute (`RATE_HEAVY_PER_MIN`) | shared on Cloud Run |
| bulk | batch triage | the whole heavy allowance, once a minute | shared on Cloud Run |
| lookup | ruling search (`GET /v1/rulings`); ruling and live duty-rate fetches from outside sources (cached answers are free) | 30 a minute (`RATE_LOOKUP_PER_MIN`) | shared on Cloud Run |
| review | the broker's `GET /v1/review` and `POST /v1/review/decision` | 30 a minute (`RATE_REVIEW_PER_MIN`) | on each instance |
| global | all model-calling requests together, in light units (heavy = 5, batch = 20) | 120 a minute (`RATE_GLOBAL_PER_MIN`) | shared on Cloud Run |

- Shared counting (D70): on Cloud Run, with `RATE_LIMIT_STORE` unset, the model-route limits are counted in one-minute windows in Firestore (`rate_limits`), shared by every instance. Before D137 each instance counted its own, so with three instances a client got about three times its limit. If Firestore fails, the instance falls back to its own buckets and logs a warning at most once a minute. `RATE_LIMIT_STORE=local` restores per-instance counting. The basic class is always counted per instance: a Firestore write per read would cost more than it protects.
- Ended windows are deleted by a Firestore TTL policy on `rate_limits.expires_at` when one exists, and in any case by each instance every 10 minutes (windows that ended more than 5 minutes ago, up to 5,000 per pass), so the collection stays bounded.
- In flight: a client may have at most 4 model-calling requests (light, heavy, bulk) running at once on an instance (`RATE_MAX_IN_FLIGHT`). A slot is released when the request ends, including when a server-sent event stream ends. Past it: 429 `too_many_in_flight`.
- Failed API keys: after 10 invalid keys in a minute from one address (`RATE_AUTH_FAILURES_PER_MIN`, per instance), that address gets 429 `auth_rate_limited` before any key lookup, for any key, until the bucket refills.
- The admin token is never limited.

### MCP (`POST /mcp`)

- The POST itself takes one token from the caller's basic bucket and carries the RateLimit headers; it has no handler deadline, because a tool call may run a plan search.
- Each tool call re-enters the API in-process and runs one `/v1` route there, with that route's class, the in-flight cap, the spend share, the API-key quota and the failed-key limit. The POST does not take any of these itself, so each applies once per tool call. Tests: `TestMCPCallsCountOncePerToolCall`, `TestMCPPostLimitsAndHeaders`.
- Body cap 12 MB (backend and the frontend `/mcp` proxy): room for 8 MB of media in base64. Over it: 413. The frontend proxy passes `Retry-After` and the RateLimit fields through.
- With `REQUIRE_BEARER=on` the POST needs the bearer; the in-process tool calls are not refused for lacking it (the MCP guard removes `Authorization`).

## Broker review links (D200)

A case owner sends one draft to an outside customs broker as a link; the broker opens it without an account and approves it, requests changes or marks it to file as a binding ruling request. What protects it, as built:

- **Who can make one.** `POST /v1/cases/{id}/reviews` goes through the same `loadCase` as every case route: a case the caller cannot see is the same 404 as a missing case; a workspace viewer gets 403; a pinned demo gets 403 for everyone, the admin token included. Revoke (`DELETE /v1/cases/{id}/reviews/{review}`) has the same rules. The isolation matrix drives all three routes with every principal.
- **The token.** `ubr_` + the case id's 12 hex digits + `_` + 32 random bytes from `crypto/rand` (256 bits), base64url. It is returned once. The server keeps only `SHA-256("unde-broker-review:" + token)`, inside the case document beside the case it opens; the link's public id is the first 16 hex digits of that hash. A lookup reads that one case by id and compares the full hash with `crypto/subtle.ConstantTimeCompare`. The case id in the token only says which document to read; with any other secret the hash matches no link (tested: a token's secret under another case's id is a 404).
- **Transport.** The browser sends the token only in the `X-Review-Token` header to the frontend proxy, which forwards it to the backend. It is never in an API path or query, so neither the proxy's nor the backend's request log can hold it; both log the route pattern only, and the audit trail names the case, never the token or its hash (tested). The page address the broker opens is `/review/<token>` on the frontend: Cloud Run's own platform request log for the frontend records page URLs, outside the application's log policy. That is the residual exposure of a link sent in a URL; expiry and revocation bound it.
- **Expiry and revocation, server side.** `expires_in_days` defaults to 14, at most 30, checked by the server on every read and answer. A revoked link answers 410 `link_revoked`, an expired one 410 `link_expired`. Deleting the case deletes its links.
- **Bad tokens.** A malformed or unknown token is 404 `not_found` and takes from a per-client invalid-credential bucket (the same size as the failed API-key limit, `RATE_AUTH_FAILURES_PER_MIN`, its own bucket, per instance). Past it the client gets 429 `auth_rate_limited` before any lookup. Each refusal, and each use of a revoked or expired link, is an anonymous `auth.denied` event in the `platform` audit tenant.
- **Rate class.** The two broker routes have their own class (`review`, above), separate from the owner's routes. They make no model call.
- **What the broker sees.** A snapshot taken when the link was made: product, the prediction behind the requested position with its calibrated confidence when one applies, the cited rulings, the plan, the draft letter, the reviewer notes and the "not a ruling" disclaimer. No case id, no other link, no owner, history, model trace or reasoning text (tested). A later change to the case does not change what the broker reads.
- **What the broker can change.** Only the link's own record: a decision and a comment. Never the case's analysis, plan, history of analyses or export state (tested). A comment is plain text: control characters other than newline and tab are dropped, at most 2,000 characters, at most 20 answers a link. Both the owner's page and the broker's page render it as text, never as HTML or Markdown.
- **Headers.** The broker routes add `X-Robots-Tag: noindex, nofollow` and `Cache-Control: no-store` to the API's `Referrer-Policy: no-referrer`. The frontend serves `/review/*` with `Referrer-Policy: no-referrer` and `X-Robots-Tag: noindex, nofollow`, and the page's metadata says noindex, nofollow.
- **Audit.** With the trail on: `review.create` (a sensitive write: no intent record, no link), `review.revoke`, `review.decision` (intent and completion) and `review.view` (one event per read) in the case's tenant. The broker is recorded as anonymous. No comment text, product data or token reaches the trail.
- **Storage.** No new collection and no Firestore index: links live in the case document (gzipped JSON), so every read is by document id.
- **Not done.** No broker account or identity check: whoever holds the link is the reviewer, so share it only with the broker. No email or notification. The broker cannot edit the letter. No e-filing.

## Quotas and spend caps

- API key quota: each key has a request quota per UTC day, set when it is issued. Past it: 429 `quota_exceeded` until 00:00 UTC. If the shared usage counter is unavailable, the request is served (fail open) and logged, and the key is counted on that instance instead, so it still stops at its quota there.
- Daily model budget: past `DAILY_SPEND_CAP_USD` of estimated model spend in a UTC day, new model-calling requests get 503 `budget_reached` (D59). Saved cases and reads keep working.
- Per-client share: one client (API key or address) may spend at most `DAILY_SPEND_CLIENT_SHARE` of that budget in a UTC day, default 0.1 (a tenth). Past it: 429 `client_budget_reached` until 00:00 UTC. Model calls are charged to the client that made the request. The admin token is exempt. With no daily budget set, there is no share either.

## Size limits

| What | Limit |
|---|---|
| JSON request body | 256 KB |
| BOM or portfolio upload (CSV or .xlsx) | 5 MB |
| BOM photos or scans (`extract-media`) | up to 4 files, 8 MB together |
| Any body through the frontend `/api` proxy | 9 MB (room for the 8 MB of media plus multipart overhead) |
| One MCP request (`POST /mcp`, backend and the frontend `/mcp` proxy) | 12 MB (8 MB of media in base64 plus the JSON around it) |
| Rows read from a CSV or a sheet | 1,000 (a BOM is capped at 500 components) |
| Columns read | 64; extra columns are ignored |
| One cell | 2,000 characters |
| .xlsx unzip | 32 MB total, 16 MB of XML |
| Products in one batch | 20 |
| Request headers | 64 KB |

## Timeouts

- Reading request headers: 10 s. Reading a whole request, body included: 30 s. Idle keep-alive connections: closed after 120 s.
- There is no server-wide write timeout: predict and plan stream server-sent events for longer than any fixed limit. Model calls have their own timeouts (`LLM_TIMEOUT`, 30 s an attempt), and the basic routes run under a 30 s deadline.
- Cloud Run ends any request at its own request timeout.

## Response headers

- Every limited response: `RateLimit-Limit`, `RateLimit-Remaining`, `RateLimit-Reset` (IETF draft RateLimit header fields; seconds until the bucket is full or the window ends). The proxy passes them through.
- Every 429 and 503 from a limit: `Retry-After` in seconds, repeated as `details.retry_after_s`.
- Every backend response: `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`; every `/v1` response: `Cache-Control: no-store` (server-sent events use `no-cache`); the broker review routes also `X-Robots-Tag: noindex, nofollow`. The proxy marks its JSON responses `private, no-store`.
- Every response: `X-Request-ID`, the request reference used in logs and the audit trail (never the caller's own value, D135).
- Frontend pages and `/api` responses: `Strict-Transport-Security: max-age=31536000; includeSubDomains`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin` (`no-referrer` on `/review/*`), `X-Frame-Options: DENY`, `Permissions-Policy: camera=(self), microphone=(), geolocation=()`, and a Content Security Policy sent as `Content-Security-Policy-Report-Only` (violations are reported in the browser console, not blocked; see D137 for why).

## What is logged

Logs follow `docs/design/data-handling.md` (D135): a closed allowlist of keys and messages. A request log line has the request reference, method, matched route pattern, status and latency; never the body, path, query, headers, keys or addresses. Rate-limit counters in Firestore hold a hash of the client key, never the address. Per-client spend totals are stored under a hash of the client key. API keys and broker review tokens are stored as hashes. The audit trail, when enabled, is described in `openapi.yaml` (#284).

## Reporting a vulnerability

Use GitHub's private vulnerability reporting on this repository (Security tab, "Report a vulnerability"). If that option is not shown, open an issue that says only that you have a security report and asks for a private contact. Do not post details in a public issue.
