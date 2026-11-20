# Workspace roles, strict auth and anonymous cases (#283)

Scope: the backend only. Roles live inside the existing single-workspace model (D68); there is no multi-workspace membership, no case reassignment and no frontend role UI yet.

## 1. What exists (inventory)

| Layer | Where | What it proves |
| --- | --- | --- |
| Sign-in token | `teams.Google.Verify` behind `withUser` (`X-User-Token`) | Google ID token: signature, expiry, audience, issuer, verified email. Invalid or oversized: 401, never an anonymous fallback. |
| API key | `withAPIKeys` (`X-API-Key`) | A broker key. Unknown or disabled: 401. |
| Admin token | `X-Admin-Token`, constant-time compare | The operator. |
| Visitor ID | `X-Visitor-ID` | A bearer secret kept in a cookie by the frontend proxy. It is not authentication (see `case-isolation.md`). |
| Principal | `principalOf` (one place) | Kind (admin, user, apikey, visitor, none), owner hash, workspace, and now the role. `Verified()` is true only for a validated user token, an API key or the admin token. |
| Case access | `loadCase` (the only case read path) and `principal.canSee` | Existence-hiding 404, pinned demos readable by all and read-only for non-admins. |

## 2. Roles

`viewer`, `editor`, `admin`, one per member of a workspace. A user is in at most one workspace.

- The role is stored on the workspace record (`roles`: email to role) and read from the store on every request for a verified signed-in user. It is never taken from a header, query, body field or token claim; `X-Role`, `X-Workspace`, `X-Tenant` and similar are ignored (tests).
- Legacy: a workspace written before roles has no `roles` field. Its creator is admin and every other member is an editor, derived on read. A stored role always wins over that rule, so a demoted creator is not admin again. New workspaces store the creator as admin and every joiner as editor explicitly, so a creator who left and is invited back returns as an editor.
- Invited emails have no role until they join.
- API keys and the admin token have no role and never inherit one. The admin token keeps its existing all-access behaviour, also when a user token rides along.

### Permission table (`rolePerms` in `roles.go`; tested cell by cell)

Applies to cases that carry the caller's own workspace ID.

| Action | viewer | editor | admin |
| --- | --- | --- | --- |
| List cases, read a case (with its history), plan estimate (reads caches only) | yes | yes | yes |
| Export with `"preview": true` (writes nothing) | yes | yes | yes |
| Export without preview (sets `exported_at`, records an `exported` event) | 403 | yes | yes |
| Create (predict, batch, streamed predict) | 403 | yes | yes |
| Rename, delete, economics, answers, scenarios, compare-targets, ask, plan (JSON and SSE), watch, bom-version | 403 | yes | yes |
| Invite, change a role, remove a member | 403 | 403 | yes |
| Pin a case, run the watch for all, API key admin | admin token only (unchanged) | | |

Notes:
- Export: only a JSON boolean `preview: true`, parsed by the same typed decoder as the rest of the body, is the read branch. A missing or false flag is the recording branch. A viewer gets 403 and nothing is written. There is no GET download route.
- Ask and watch append history events, so they are edits.
- A case the user may see but may not change is 403 `forbidden`. A case they may not see is 404, identical to a missing case.
- Pinned demos follow the demo rules for everyone (readable, never written by non-admins); a viewer has the same demo behaviour as an anonymous visitor.
- Stateless helpers (`/v1/products/*`, rulings, eval summary, duty rates, demos, health) touch no case and have no role.

### Whose case is it

A case created by a workspace member is stamped with the workspace and belongs to the workspace: the role governs access for everybody in it, the author included (a demoted author is a viewer on it). A case made before the user joined, or by their browser while anonymous, stays personal: owner behaviour, whatever the role. A member who is removed or leaves loses every case stamped with that workspace, including ones they made; their personal cases remain theirs.

### Role management (admin only)

- `PUT /v1/workspaces/{id}/members/{email}/role` and `DELETE /v1/workspaces/{id}/members/{email}` (admin removes anyone; a member removes themselves). Request and response shapes: the API contract in the ledger.
- A workspace that is not the caller's is 404, the same as an unknown one.
- The last admin cannot be demoted, removed or leave (409). The creator can be demoted when another admin exists.
- The admin check and the last-admin check run inside the store's atomic step (a mutex in memory; a Firestore transaction on the stored document), on the stored record rather than on the request context, so two admins demoting each other cannot both succeed.
- Not supported: cancelling a pending invitation, a role at invite time, several workspaces per user, moving cases between workspaces.

## 3. STRICT_AUTH

`STRICT_AUTH=on` (default off; off is byte-for-byte the previous behaviour). When on, private case operations need a verified principal: create (predict, batch, stream), `GET /v1/cases`, and every case route on a case that is not a pinned demo. A request with only a visitor ID, or no credentials, gets 401 `unauthenticated`, and gets the same 401 for a missing, hidden or its own case, so the answer reveals nothing. Pinned demos, `/v1/demos*` and the other public endpoints stay open. An invalid user token or unknown API key is 401 in either mode.

Strict mode is not a claim that visitor IDs authenticate anyone. It only stops treating one as enough. If it is on while neither Teams nor API keys are, only the admin token can reach private routes (the server logs a warning).

## 4. Anonymous case migration: safe procedure (documented, not automated)

There is no automatic claiming, reassignment or guessing of ownership, and owner hashes and salts are unchanged. Anonymous cases are stored under the hash of the browser's visitor ID. A signed-in user already sees the anonymous cases of the browser they are signed in from (they are read through `X-Visitor-ID`), so nothing needs to move for them to keep working.

Moving a case under a user or into a workspace is not supported yet. If it is built, it must be explicit and user initiated: the signed-in user, from the same browser, names the case and confirms; the server checks both the verified user and the visitor hash; the change is recorded as a history event; nothing is inferred from content. Under `STRICT_AUTH=on`, anonymous cases become unreachable by visitor ID alone: a user signed in from the same browser still reaches them.

## 5. Limits

- Everything above is exercised against the in-memory stores. The Firestore adapter (`store/workspaces.go`) implements the same operations in transactions, but it was not run here: no emulator. An emulator test is included and skipped unless `FIRESTORE_EMULATOR_HOST` is set. Atomicity under real Firestore contention is unverified.
- Deployed `CASE_OWNERSHIP`, `TEAMS` and `STRICT_AUTH` values are unverified.
- Nothing in the frontend uses roles yet. Needed: role display, role selector and remove/leave for admins, disabled write actions and 403 handling for viewers, export-preview-only affordance, a sign-in-required state when strict is on.
