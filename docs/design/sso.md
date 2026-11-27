# Single sign-on: OIDC and SAML (#868)

Status: backend done, off by default (`SSO=on`). The sign-in page that sends the browser to the identity provider and posts the answer back is frontend work and is not built yet.

## 1. What it does

A workspace admin connects the organisation's identity provider (IdP) for one or more email domains. A person whose email is on such a domain signs in through the IdP instead of Google. The first sign-in adds them to the connection's workspace with the connection's default role (viewer unless the admin sets another).

Two protocols:

| Protocol | Library | What the admin enters |
|---|---|---|
| OpenID Connect | `coreos/go-oidc` v3, `golang.org/x/oauth2` | issuer URL, client ID, client secret |
| SAML 2.0 | `russellhaering/gosaml2`, `goxmldsig` | IdP SSO URL, IdP entity ID, one or more signing certificates (PEM) |

## 2. Flow

The frontend proxy forwards `/api/v1/*` server to server and follows redirects, so the flow is JSON, not browser redirects through the backend.

1. `POST /v1/sso/start {email}` returns `{protocol, url}`. The browser goes to `url`.
2. The IdP returns the browser to the site: `SSO_SITE_URL/sso/oidc/callback` (OIDC) or `SSO_SITE_URL/sso/saml/acs` (SAML HTTP-POST).
3. That page posts what it got to `POST /v1/sso/oidc/callback {state, code}` or `POST /v1/sso/saml/acs` (form or JSON `{saml_response, relay_state}`).
4. The answer is `{token, email, workspace_id, role, provisioned, expires_at}`. The frontend sends `token` as `X-User-Token`, the same header as a Google ID token.

`GET /v1/sso/saml/metadata` is the service provider metadata to give the IdP.

## 3. Connections

Admin routes, workspace admins only (another workspace gets 404, a non-admin 403):

| Route | Does |
|---|---|
| `GET /v1/workspaces/{id}/sso` | list connections |
| `POST /v1/workspaces/{id}/sso` | create (always starts disabled; at most 5) |
| `PUT /v1/workspaces/{id}/sso/{conn}` | replace settings; an empty client secret keeps the stored one |
| `POST /v1/workspaces/{id}/sso/{conn}/verify` | check the DNS records; `{enable: true}` enables it once every domain is verified |
| `DELETE /v1/workspaces/{id}/sso/{conn}` | delete; its sessions stop working at once |

Create, update and delete are sensitive audit actions (`sso.connection_*`); a sign-in is `sso.login` (detail `provisioned` for a new member); refusals are recorded as denials.

`enforced: true` refuses Google sign-in for the connection's domains, so the IdP is the only way in.

## 4. Safety properties

Each has a test in `backend/internal/sso` or `backend/internal/httpapi/sso_test.go`.

- **Domain ownership.** A domain signs nobody in until a DNS TXT record `_undelabs-verify.<domain>` holds the connection's token. Public mail domains (gmail.com and others) cannot be added. Two connections clash only when both have verified the same domain, so an unverified claim cannot block the real owner.
- **Tenant binding.** A session is good only while the connection is enabled, the domain is still verified on it, and the user is still a member of its workspace. A person who is a member of another workspace is refused (`403 sso_other_workspace`), so one workspace's IdP cannot open another workspace's account.
- **OIDC.** Issuer, audience, expiry, signature, nonce and `email_verified` are checked; the code exchange uses PKCE (S256); state is single use and expires after ten minutes.
- **SAML.** The response or assertion must be signed by a configured certificate. Exactly one assertion; issuer, audience, recipient, destination and the time window are checked. `InResponseTo` must match a request we sent, so IdP-initiated logins are refused. The relay state is single use and each assertion ID is accepted once (replay).
- **SSRF.** OIDC discovery and token calls go through a dialer that refuses loopback, private, link-local and CGNAT addresses.
- **Secrets.** OIDC client secrets are sealed with AES-GCM under a key derived from `SSO_SESSION_KEY` and never returned (`has_client_secret` only). Session tokens are HMAC-SHA256 signed with a separate derived key.
- **Errors.** Callers see `401 sso_failed` with no detail; the reason class goes to the log.

## 5. Configuration

| Variable | Meaning |
|---|---|
| `SSO` | `on` registers the routes. Needs `TEAMS=on`. |
| `SSO_SESSION_KEY` | at least 32 bytes; signs sessions and seals client secrets. Changing it ends every SSO session and makes stored client secrets unreadable. |
| `SSO_SITE_URL` | the https origin the IdP returns the browser to |
| `SSO_SESSION_TTL` | session length, 5m to 24h, default 8h |

Connections and the one-time state live in Firestore (`sso_connections`, `sso_onetime`) when cases do, otherwise in memory.

## 6. Not done

- The frontend sign-in page, callback pages and the admin screen.
- Group-to-role mapping from the IdP (the default role applies; SCIM, #869, manages roles).
- Single logout and IdP-initiated SAML.
- Expired rows in `sso_onetime` are not swept; a Firestore TTL policy on `expires_at` would do it (an operator setting).
