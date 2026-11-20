# Case isolation and the request principal

Status: describes the backend as of the #273 change. Behavior for existing
cases is unchanged; this document records how isolation works, how far it
can be trusted, and where it stops.

## One principal per request

`backend/internal/httpapi/principal.go` resolves who a request acts as, in one
place (`principalOf`). Ownership checks, case loading, case creation and
listing all read that principal; nothing else looks at `X-Visitor-ID`, the
admin token, the API key context or the signed-in user.

| Part of the principal | Source | Trust |
|---|---|---|
| Admin | `X-Admin-Token`, constant-time compare with the configured token | verified |
| User (email, user owner hash, workspace) | set by the `X-User-Token` middleware after the Google ID token is verified | verified |
| API key | set by the `X-API-Key` middleware after the key is authenticated | verified |
| Visitor | a syntactically valid `X-Visitor-ID`, hashed | not verified: a bearer secret |

Any other header (`X-Tenant`, `X-Workspace`, `X-Role`, `X-Owner`,
`X-User-Email`, and so on) is never read, so forging it grants nothing.
A token or key that fails verification never reaches the principal: the
middleware answers 401 on every `/v1` route (a bad `X-User-Token` is 401 on
every path while teams are on).

`principal.Verified()` is true for admin, user and API key, and false for a
visitor ID alone. Features that must not accept a bare visitor ID should ask
for it, instead of re-reading headers.

Salts and prefixes that are part of stored data are unchanged
(`origin-proof-visitor:`, `origin-proof-user:` with `u_`,
`origin-proof-apikey:` with `key_`).

### Extending it (roles)

Add the role to `principal`, derived only from verified membership data, and
add a method beside `canSee`. Call sites use `principalOf` and the methods, so
they do not change.

## Visibility rules

`principal.canSee(case, ownership)` is the single rule, behind `loadCase` (so
every case route answers 404 for a case the caller may not see, the same
answer as for a case that does not exist) and behind the list.

- Pinned demos: visible to everyone, read-only for everyone but the admin.
- Admin: everything.
- Signed-in user: the user's own cases, the user's workspace cases, and the
  anonymous cases of the same browser's visitor ID.
- Cases of a signed-in user or a workspace are never visible to anonymous
  callers, with ownership on or off.
- API-key cases are visible to that key only (and the admin), with ownership
  on or off. A key sees its own cases and the demos only.
- Anonymous visitors, `CASE_OWNERSHIP=on`: own cases and demos.
- Anonymous visitors, `CASE_OWNERSHIP=off` (the default): every anonymous
  case, meant as a shared public demo mode.

The list reads only what the caller can see (pinned demos plus the caller's
own owners) and applies the cap of 200 afterwards, so other visitors' cases
cannot occupy or leak through the page. In the ownership-off public mode the
list is, by design, the demos plus the 200 newest anonymous cases.

Cases created with no usable identity (no `X-Visitor-ID`, or a malformed one)
have no owner. With ownership on, nobody but the admin can open them. They
become visible to every anonymous caller if ownership is later turned off.

## What a client-held `X-Visitor-ID` cannot give you

The ID is a random value in an HttpOnly cookie that the frontend proxy turns
into a header. It is a bearer secret, not an account.

- Whoever holds the ID is that visitor: a stolen or leaked cookie, a shared
  browser profile, or a copied header gives full access to that visitor's
  cases. The backend cannot tell.
- There is no revocation or rotation, and the cookie lives for a year. Clearing
  cookies abandons the cases (they remain stored, unreachable by their owner).
- It does not follow a person across devices or browsers.
- Anyone who can call the backend directly (not through the proxy) can send
  any ID. The values are 16-128 characters, so guessing another visitor's ID
  is not practical, but the ID is the only secret.
- The proxy forwards a fixed set of request headers and sets `X-Visitor-ID`
  from the cookie; it does not forward `X-Admin-Token`.
- Sending a stored owner hash as the visitor ID does not make the sender that
  owner: the ID is hashed again.

Accounts (a verified user token) or API keys are the stronger identities. A
future strict mode should require one of those, or the admin token, for private
case operations, and not accept a valid visitor ID alone; that work is tracked
with the roles change, not here.

## Migrating anonymous cases (no automatic claiming)

Anonymous cases are never moved to an account automatically: a visitor ID
cannot prove who a person is. The safe path is manual and explicit:

1. Keep ownership as it is; do not rewrite `owner` in bulk.
2. A person who wants a case in their workspace signs in on the same browser
   (their anonymous cases stay visible to them there) and re-creates or
   exports the case under their account.
3. An operator who must move a specific case does so one case at a time, after
   confirming with the requester out of band, with the admin token and an
   audited script, never by pattern.
4. Cases created before ownership existed have no owner and are visible only
   to the admin once ownership is on; they are triaged by hand.

## Tests

`httpapi/isolation_matrix_test.go` classifies every route registered in
`server.go`; a new route that is not classified fails the suite. Case-scoped
routes are driven with every principal (stranger, malformed ID, two visitors,
two API keys, signed-in users with and without a workspace, admin) against
every kind of case, with ownership on and off, checking 404 existence hiding
(identical to the answer for a case that does not exist), that a refused
request changes nothing, read-only pinned demos, 401 for bad credentials,
forged identity headers, and the list filter before the default cap.
`httpapi/principal_test.go` fixes the principal, and compares the visibility
rule with a frozen copy of the previous rule over every combination.
