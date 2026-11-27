# SCIM provisioning (#869)

Status: backend done, off by default (`SCIM=on`, needs `SSO=on`). The admin screen is built (D961); the proxy mapping for identity providers is frontend work and is not built yet.

## 1. What it does

The organisation's identity provider (Okta, Entra ID, Google Workspace) adds, changes and removes workspace members through SCIM 2.0 (RFC 7643, RFC 7644), Users only. Groups are not supported; a member's role comes from `roles[0].value` or the token's default role.

## 2. The token

A workspace admin issues one token per workspace:

| Route | Does |
|---|---|
| `GET /v1/workspaces/{id}/scim` | status: configured, last four characters, domains, default role, allow_admin, base URL |
| `POST /v1/workspaces/{id}/scim/token` | issue, replacing any previous token; the token is in this answer only |
| `DELETE /v1/workspaces/{id}/scim/token` | revoke |

The token is `scim_` plus 43 characters (256 random bits), stored only as a SHA-256 hash. Each domain it manages must be verified on one of the workspace's SSO connections (`409 scim_unverified_domain` otherwise). `default_role` is viewer or editor.

## 3. SCIM routes

Base URL: `SSO_SITE_URL/api/v1/scim/v2`. The token goes in `X-SCIM-Token`, or as `Authorization: Bearer scim_...` on a direct call.

`GET /ServiceProviderConfig`, `/ResourceTypes`, `/Schemas`; `GET` and `POST /Users`; `GET`, `PUT`, `PATCH` and `DELETE /Users/{id}`. The only filter is `userName eq "email"`. PATCH changes `active` and `roles`; `active: false` removes the member. User ids are opaque and differ per workspace.

## 4. Safety properties

Each has a test in `backend/internal/scim` or `backend/internal/httpapi/scim_test.go`.

- **Domains.** A token sees and changes only members of its workspace whose email is on its domains, which are SSO-verified; anyone else is invisible (404, absent from lists) and cannot be added (403). So a workspace cannot pull in another company's employees.
- **Other workspaces.** Adding someone already in another workspace is a 409 that does not name it.
- **Admins.** Without `allow_admin` a token cannot grant, change or remove the admin role (403). With it, the last admin still cannot be demoted or removed (409).
- **Bad tokens.** A malformed token is refused without a lookup; too many wrong tokens from one client get 429 with Retry-After; a replaced or revoked token stops working at once.
- **Audit.** Issue, revoke, create, update and delete fail closed (`scim.*`, sensitive). Provider actions are recorded as the system actor in the workspace's tenant, the member as a pseudonym; a wrong token is a denial with nothing it claimed.

## 5. Not done

- The frontend proxy forwards only some headers: for identity providers to reach SCIM through the site, it must map `Authorization: Bearer scim_...` to `X-SCIM-Token` on `/api/v1/scim/` (as it does for MCP keys) and pass `application/scim+json` through. Until then SCIM works only against the backend directly.
- Groups and group-to-role mapping; `externalId` and names are not stored.
