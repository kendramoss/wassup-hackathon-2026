# API security and limits

What protects the public API, as built (D137 and the decisions it cites). Defaults are the code's defaults; the environment variable that changes each one is named. Everything here is enforced by the backend unless it says frontend.

## How the API is reached

- The backend runs on Cloud Run and only the frontend's service account may invoke it (Cloud Run IAM). Brokers and browsers reach it through the frontend's `/api/v1/...` proxy (`frontend/src/lib/backend.ts`), which forwards an allowlist of headers: `Content-Type`, `Accept`, `X-API-Key`, `X-Review-Token`, plus `X-Client-IP`, `X-Visitor-ID`, `X-User-Token` it sets itself, and a Google ID token as `Authorization: Bearer` on Cloud Run.
- `REQUIRE_BEARER=on` (default off) makes the backend refuse, with 401, any request other than `/health` that has no `Authorization: Bearer` header. It is defence in depth behind Cloud Run IAM. It is **on in production**: Cloud Run header pass-through to the container was verified end to end before the flag was enabled (23 Nov 2026). The code default stays off so local UI work needs no token.
- CORS: the API sends no CORS headers, on any route. It is for server-to-server calls with `X-API-Key` and for the frontend's own proxy. Calls from a browser page on another site are not supported.

## As built vs as deployed (23 Nov 2026)

The code defaults are permissive so local UI work works without credentials. "As deployed" is read from `.github/workflows/deploy.yml` and checked against the live public paths; the backend itself is IAM-private (`/health` returns 403 from Google Frontend). This matrix is the answer to "is isolation/auth on?" — do not claim more than it says (#420).

| Switch | Code default | Deployed value | Live evidence / why |
|---|---|---|---|
| Cloud Run IAM on backend | n/a (platform) | only the frontend service account may invoke | direct `https://backend-88903435351.us-central1.run.app/health` → **403**; public proxy is the only path |
| `CASE_OWNERSHIP` | off | **on** | `curl https://undelabs.dev/api/v1/cases` shows the six pinned demos and no visitor cases |
| `TEAMS` / `TEAMS_AUDIENCE` | off / empty | **on** / `88903435351-…apps.googleusercontent.com` | Google ID tokens verified on every request |
| `API_KEYS` | off | **on** | unknown/disabled keys get 401, never anonymous fallback |
| `DAILY_SPEND_CAP_USD` | 0 (uncapped) | **25** | new model calls stop with 503 `budget_reached` past the cap |
| `PARTS_HTS_SUGGEST` | **on** | on (code default, not set by the deploy) | `POST /v1/products/parts-hts-suggest` calls the router model once per request; light rate class, in-flight cap, daily spend cap and the client's share apply (D380). Answers are cached in memory for a day, only for the same caller (workspace, else user, API key, visitor ID or client address) and the same part, origin, product name and description; one caller's answer is never served to another (D410, #577). `off` removes the route |
| `ADMIN_TOKEN` | empty | **set from Secret Manager** | needed to pin demos and issue keys; never sent from the browser |
| `STRICT_AUTH` | off | **on** | `curl https://undelabs.dev/api/v1/cases` with no credentials → **401**; pinned demos and `/v1/demos*` stay open (200). Private case operations need a verified user, API key or admin token |
| `REQUIRE_BEARER` | off | **on** | Cloud Run passes the frontend's `Authorization: Bearer` to the container: `curl https://undelabs.dev/api/v1/demos` → **200** after the flag was enabled. Defence in depth behind IAM, not the primary boundary |
| `USMCA_RULES_EXT` | off | **on** (27 Nov, #910) | USMCA rules check uses the GN 11(o) rows, de minimis and the parts route (D930, D932) |
| `ATIGA_CHECK` | off | **on** (27 Nov, #910) | `POST /v1/atiga/check` and the `check_atiga_preference` tool are served; no accuracy figure (D927) |
| `BROKER_WORKFLOW` | off | **on** (27 Nov, #910) | review queue, case discussion, sign-offs and the eRulings filing package are served (D925) |
| `ERP_IMPORT` | off | **on** (27 Nov, #910) | `POST /v1/products/erp-import` is served (D926) |
| `USAGE_METERING` | off | **on** (27 Nov, #910) | per-workspace monthly usage is counted; no budget is set until a workspace admin sets one (D926) |
| `WEBHOOKS` | off | off (needs `TEAMS`) | workspace admins can register signed https callbacks for workspace events; targets that are private, loopback or link-local are refused at registration and at delivery (D936) |
| `ADMIN_CONSOLE` | off | **on** (27 Nov, #910) | `GET /v1/workspaces/{id}/admin` answers workspace admins only (D935) |
| `RETENTION` | off | **on** (27 Nov, #910) | admins can set `case_days`; nothing is purged until an operator runs `cmd/retention` (D924) |
| `AUDIT_EXPORT` | off | **on** (27 Nov, #910) | `GET /v1/audit/export` answers verified admins (D924) |
| `STATUS_PAGE` | off | **on** (27 Nov, #910) | `GET /v1/status` is served (D928) |
| `AUDIT_ACTOR_KEY` / `AUDIT` | empty / `auto` | **set from Secret Manager / auto → audit on** | `curl https://undelabs.dev/api/v1/audit` and `/audit/verify` now answer (**401** without a verified admin, not 404). It is evidence, not a certification or immutable log |
| `RATE_LIMIT_STORE` | local, auto-firestore on Cloud Run with a project | **Firestore (defaulted)** | model-route buckets are shared by all three instances; `X-Client-IP` is set by the proxy, so a client cannot choose its own bucket through the public path (#421) |

When `K_SERVICE` is set and any of `STRICT_AUTH`, `REQUIRE_BEARER` or `AUDIT_ACTOR_KEY` is missing, the server logs a `production config` warning at startup naming the gap. It is a warning, not a boot failure, so a local or staging boot still works; none of the three flags is missing on undelabs.dev (D730).

**Built and enabled in production.** As of 23 Nov 2026 the audit trail, required sign-in (`STRICT_AUTH`) and the bearer check (`REQUIRE_BEARER`) are all **on** on undelabs.dev. They were rolled out in three separate deploys, each verified before the next: (1) `AUDIT_ACTOR_KEY` from Secret Manager, (2) `STRICT_AUTH=on`, (3) `REQUIRE_BEARER=on` only after Cloud Run header pass-through was confirmed. The public demo opens through the pinned demo cases and `/v1/demos*`; anonymous visitors cannot reach private case operations. This was an owner and infrastructure change, outside feature PRs ([ROADMAP.md](../ROADMAP.md#owner-and-infrastructure-steps-to-switch-these-on), D730). "On" is now the correct word for all three in production; their code defaults remain permissive for local work.

## Who the caller is

| Mode | Header | Notes |
|---|---|---|
| Anonymous visitor | `X-Visitor-ID` (set by the proxy from an httpOnly cookie) | Scopes which cases a visitor sees with `CASE_OWNERSHIP=on`. Not authentication: `STRICT_AUTH=on` refuses it alone for private case operations. |
| Signed-in user | `X-User-Token` (Google ID token, `TEAMS=on`) | Verified on every request: signature, expiry, issuer, audience, verified email (D68). |
| API key | `X-API-Key: op_...` (`API_KEYS=on`) | Admin-issued or self-service (created at `/settings` or `POST /v1/api-keys` by a verified sign-in, #788). Only a SHA-256 of the key is stored. Unknown or disabled: 401 `unauthorized`, never a silent fall back to anonymous (D69). |
| Admin | `X-Admin-Token` | Seeding, pinning, the batch watch, issuing keys. Not limited. Never sent from the browser. |
| Broker review link | `X-Review-Token: ubr_...` (on `/v1/review*` only) | Opens one frozen draft and answers it; no identity on any other route (D200, below). |

## Rate limits

Every `/v1` route has a rate class. A test (`TestEveryV1RouteHasALimitClass`) fails if a route is registered without one. Limits are per client: the API key when one is sent, otherwise the connection's `RemoteAddr` (IPv6 grouped by /64). The header `X-Client-IP` is used **only** when the connection came from a CIDR in `TRUSTED_PROXY_CIDRS`; with no ranges configured the header is ignored, so a direct caller cannot choose its own bucket (#421). The public frontend proxy sets that header from the rightmost `X-Forwarded-For`, so deploying must either configure the proxy's source range deliberately or accept that proxied callers share the connection bucket. A bad CIDR is a startup error. After every deploy, run `scripts/check-backend-private.sh` (or call it from CI): it fails if the backend IAM policy grants `allUsers` or if a bare internet request to `/health` is not refused with 403. That check is part of the safety claim, not decoration.

| Class | Routes | Per client | Counted |
|---|---|---|---|
| basic | case list, get, rename, delete, economics, export; demos; a ruling by number; duty rates; eval summary; workspaces; the `POST /mcp` request itself | 120 a minute (`RATE_BASIC_PER_MIN`) | on each instance |
| light | predict, answers, ask, extract, extract-media, import, money prefill, HTS suggest, cost estimate, family check, scenarios, watch, BOM version, plan estimate, audit reads | 20 a minute (`RATE_LIGHT_PER_MIN`) | shared on Cloud Run |
| heavy | plan, compare targets | 4 a minute (`RATE_HEAVY_PER_MIN`) | shared on Cloud Run |
| bulk | batch triage | the whole heavy allowance, once a minute | shared on Cloud Run |
| lookup | ruling search (`GET /v1/rulings`); ruling and live duty-rate fetches from outside sources (cached answers are free) | 30 a minute (`RATE_LOOKUP_PER_MIN`) | shared on Cloud Run |
| review | the broker's `GET /v1/review`, `POST /v1/review/decision` and, with `BROKER_WORKFLOW=on`, `GET` and `POST /v1/review/comments` | 30 a minute (`RATE_REVIEW_PER_MIN`) | on each instance |
| global | all model-calling requests together, in light units (heavy = 5, batch = 20) | 120 a minute (`RATE_GLOBAL_PER_MIN`) | shared on Cloud Run |

- Shared counting (D70): on Cloud Run, with `RATE_LIMIT_STORE` unset, the model-route limits are counted in one-minute windows in Firestore (`rate_limits`), shared by every instance. Before D137 each instance counted its own, so with three instances a client got about three times its limit. If Firestore fails, the instance falls back to its own buckets and logs a warning at most once a minute. `RATE_LIMIT_STORE=local` restores per-instance counting. The basic class is always counted per instance: a Firestore write per read would cost more than it protects.
- Ended windows are deleted by a Firestore TTL policy on `rate_limits.expires_at` when one exists, and in any case by each instance every 10 minutes (windows that ended more than 5 minutes ago, up to 5,000 per pass), so the collection stays bounded.
- In flight: a client may have at most 4 model-calling requests (light, heavy, bulk) running at once on an instance (`RATE_MAX_IN_FLIGHT`). A slot is released when the request ends, including when a server-sent event stream ends. A JSON plan search that keeps running after its caller left (D402) keeps that caller's slot until the search ends (D411, #581). Past it: 429 `too_many_in_flight`.
- Failed API keys: after 10 invalid keys in a minute from one address (`RATE_AUTH_FAILURES_PER_MIN`, per instance), that address gets 429 `auth_rate_limited` before any key lookup, for any key, until the bucket refills. A lookup slot is reserved under the same lock as the check, and each lookup in flight holds one of the bucket's tokens until it finishes: a lookup is admitted only while the tokens left, less the lookups in flight, are at least one. So no arrival pattern, simultaneous or a few milliseconds apart, lets more than the limit's invalid keys from one address reach the key store in a minute on an instance (#386, #505). A valid key (or a store error) gives its slot back without spending a token. An invalid key always spends one, into debt if the bucket is already empty, so a late failure lengthens the wait.
- The admin token is never limited.
- Duplicate plan searches (D402): a JSON `POST /v1/cases/{id}/plan` (and so the MCP `plan_production_moves`) whose case and inputs match a search still running on the instance waits for it and gets the same answer instead of starting a second, model-heavy search. This covers a client that times out and retries (an MCP client's 300 s read timeout, #574). The search keeps running for 2 minutes after its last waiting caller leaves, then stops if nobody rejoined. An optional `Idempotency-Key` header (MCP: `idempotency_key`) also replays a finished search's answer for 15 minutes (at most 256 kept per instance); the same key with other inputs is 409 `conflict`. A joining request still takes a heavy token and an in-flight slot while it waits, and its model cost stays with the caller that started the search. The search itself holds the starting caller's slot until it ends, so a dropped request cannot free a slot while its search runs on (D411). The table is in memory, so a retry that reaches another instance starts its own search; streamed requests (the website) are not joined.

### MCP (`POST /mcp`)

- The POST itself takes one token from the caller's basic bucket and carries the RateLimit headers; it has no handler deadline, because a tool call may run a plan search.
- Each tool call re-enters the API in-process and runs one `/v1` route there, with that route's class, the in-flight cap, the spend share, the API-key quota and the failed-key limit. The POST does not take any of these itself, so each applies once per tool call. Tests: `TestMCPCallsCountOncePerToolCall`, `TestMCPPostLimitsAndHeaders`.
- Body cap 12 MB (backend and the frontend `/mcp` proxy): room for 8 MB of media in base64. Over it: 413. The frontend proxy passes `Retry-After` and the RateLimit fields through.
- Who a tool call acts as (#385): the API key the MCP guard forwards, or anonymous with the caller's `case_access_key`. The admin token is refused with 403; a sign-in token (`X-User-Token`) is ignored (`withUser` skips `/mcp`, #481); the visitor ID, cookies and `X-Review-Token` are dropped. Defence in depth (D280): each tool call's `/v1` request is built on a context without any of the `POST /mcp` request's values (`mcpserver.isolated`), so a user, key or audit state the middleware put there cannot reach it. Tests: `TestMCPToolCallsDoNotInheritTheOuterPrincipal`, `TestMCPIgnoresABrokerReviewToken`, `TestIsolationMatrixOverMCP`, `mcpserver.TestToolRequestsDoNotInheritContextValues`.
- With `REQUIRE_BEARER=on` the POST needs the bearer; the in-process tool calls are not refused for lacking it (the MCP guard removes `Authorization`). The MCP guard fills in `X-Client-IP` from `RemoteAddr` when absent, and resolves the client address with the same rule as `clientKey`: a caller-supplied value is honoured **only** when the source address is in `TRUSTED_PROXY_CIDRS` (#421, #479). The resolved value, never the raw header from an untrusted caller, is the `RemoteAddr` of every in-process tool request, so changing the header cannot pick a new light or heavy bucket.

## Broker review links (D200)

A case owner sends one draft to an outside customs broker as a link; the broker opens it without an account and approves it, requests changes or marks it to file as a binding ruling request. What protects it, as built:

- **Who can make one.** `POST /v1/cases/{id}/reviews` goes through the same `loadCase` as every case route: a case the caller cannot see is the same 404 as a missing case; a workspace viewer gets 403; a pinned demo gets 403 for everyone, the admin token included. Revoke (`DELETE /v1/cases/{id}/reviews/{review}`) has the same rules. The isolation matrix drives all three routes with every principal.
- **The token.** `ubr_` + the case id's 12 hex digits + `_` + 32 random bytes from `crypto/rand` (256 bits), base64url. It is returned once. The server keeps only `SHA-256("unde-broker-review:" + token)`, inside the case document beside the case it opens; the link's public id is the first 16 hex digits of that hash. A lookup reads that one case by id and compares the full hash with `crypto/subtle.ConstantTimeCompare`. The case id in the token only says which document to read; with any other secret the hash matches no link (tested: a token's secret under another case's id is a 404).
- **Transport.** The link is `/review#<token>`: the token is the URL fragment, which browsers never send to a server, so it is in no request log (the frontend's, Cloud Run's platform log or any proxy's) and in no `Referer` header. Tested: the generated link has `/review` as its path and no query. The page reads `location.hash` once, removes it from the address bar and history with `history.replaceState`, and keeps the token in memory only (no storage); a reload or a bookmark shows "open the review link again". The browser then sends the token only in the `X-Review-Token` header to the frontend proxy, which forwards it to the backend. It is never in an API path or query, so neither the proxy's nor the backend's request log can hold it; both log the route pattern only, and the audit trail names the case, never the token or its hash (tested). A token in a path is never accepted: `/review/<anything>` is a plain 404 with no redirect, and the segment is never read. What remains: the link itself, in the message the owner sends and in the broker's browser history before the page clears it; expiry and revocation bound that.
- **Expiry and revocation, server side.** `expires_in_days` defaults to 14, at most 30, checked by the server on every read and answer. A revoked link answers 410 `link_revoked`, an expired one 410 `link_expired`. Deleting the case deletes its links.
- **Bad tokens.** A malformed or unknown token is 404 `not_found` and takes from a per-client invalid-credential bucket (the same size as the failed API-key limit, `RATE_AUTH_FAILURES_PER_MIN`, its own bucket, per instance). Past it the client gets 429 `auth_rate_limited` before any lookup. The check reserves a lookup slot under the bucket's lock, as for API keys (#489), so concurrent bad tokens from one client cannot all reach the case store (D302). Each refusal, and each use of a revoked or expired link, is an anonymous `auth.denied` event in the `platform` audit tenant.
- **Rate class.** The two broker routes have their own class (`review`, above), separate from the owner's routes. They make no model call.
- **What the broker sees.** A snapshot taken when the link was made: product, the prediction behind the requested position with its calibrated confidence when one applies, the cited rulings, the plan, the draft letter, the reviewer notes and the "not a ruling" disclaimer. No case id, no other link, no owner, history, model trace or reasoning text (tested). A later change to the case does not change what the broker reads.
- **What the broker can change.** Only the link's own record: a decision and a comment. Never the case's analysis, plan, history of analyses or export state (tested). A comment is plain text: control characters other than newline and tab are dropped, at most 2,000 characters, at most 20 answers a link. Both the owner's page and the broker's page render it as text, never as HTML or Markdown.
- **Headers.** The broker routes add `X-Robots-Tag: noindex, nofollow` and `Cache-Control: no-store` to the API's `Referrer-Policy: no-referrer`. The frontend serves `/review` and `/review/*` with `Referrer-Policy: no-referrer` and `X-Robots-Tag: noindex, nofollow`, and the page's metadata says noindex, nofollow.
- **Audit.** With the trail on: `review.create` (a sensitive write: no intent record, no link), `review.revoke`, `review.decision` (intent and completion) and `review.view` (one event per read) in the case's tenant. The broker is recorded as anonymous. No comment text, product data or token reaches the trail.
- **Storage.** No new collection and no Firestore index: links live in the case document (gzipped JSON), so every read is by document id.
- **Discussion (BROKER_WORKFLOW=on).** The broker holding an open link can read the case's discussion and add comments (`/v1/review/comments`), under the same token checks, rate class and limits. The broker sees authors by role only, never their emails; their comments are stored as "external broker" with the link id. Comment text is plain text under the same rules as a decision comment, and no comment text reaches the audit trail (`review.view` for a read, `case.comment` with detail `external_broker` for a post).
- **Not done.** No broker account or identity check: whoever holds the link is the reviewer, so share it only with the broker. No email or notification. The broker cannot edit the letter through the link (an editor can file an edited letter, docs/design/broker-workflow.md). Nothing is submitted to CBP from here.

## Quotas and spend caps

- API key quota: each key has a request quota per UTC day, set when it is issued. Past it: 429 `quota_exceeded` until 00:00 UTC. If the shared usage counter is unavailable, the request is served (fail open) and logged, and the key is counted on that instance instead, so it still stops at its quota there.
- Self-service keys (#788): a verified sign-in creates a named key through `POST /v1/api-keys` with a quota **fixed at 200** requests per UTC day; a `daily_quota` or `owner` in the body is refused. The internal owner is read from the sign-in, never sent on the wire, and scopes `GET /v1/api-keys` and `DELETE /v1/api-keys/{id}` to that account. Another account's id, an admin/legacy key and an unknown id answer 404, so ids cannot be probed. The secret is shown once on creation; only its hash is stored. Issue, list and revoke are recorded as `apikey.issue`, `apikey.list` and `apikey.disable` in the requester's tenant, with a pseudonymous user actor and no owner or hash in the trail.
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
- Frontend pages and `/api` responses: `Strict-Transport-Security: max-age=31536000; includeSubDomains`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin` (`no-referrer` on `/review` and `/review/*`), `X-Frame-Options: DENY`, `Permissions-Policy: camera=(self), microphone=(), geolocation=()`, and a Content Security Policy sent as `Content-Security-Policy-Report-Only` (violations are reported in the browser console, not blocked; see D137 for why). It stays Report-Only after D220: enforcing it needs a real Google sign-in tried against it, which can't be done locally. The `X-Powered-By` header is off (`poweredByHeader: false`, D220).

## What is logged

Logs follow `docs/design/data-handling.md` (D135): a closed allowlist of keys and messages. A request log line has the request reference, method, matched route pattern, status and latency; never the body, path, query, headers, keys or addresses. Rate-limit counters in Firestore hold a hash of the client key, never the address. Per-client spend totals are stored under a hash of the client key. API keys and broker review tokens are stored as hashes. The audit trail, when enabled, is described in `openapi.yaml` (#284).

## Reporting a vulnerability

Use GitHub's private vulnerability reporting on this repository (Security tab, "Report a vulnerability"). If that option is not shown, open an issue that says only that you have a security report and asks for a private contact. Do not post details in a public issue.
