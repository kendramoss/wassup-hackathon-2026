# Frontend: Google sign-in and team workspaces (D68)

**Backend status:** done on `feat/finals-teams-auth`. Off by default (`TEAMS=off`); nothing changes until it's on.

**Problem:** cases belong to a browser (D58). A team can't share cases, and a user loses them on another device.

**What the frontend needs to do:**
1. **Sign-in button** (header, right side): "Sign in with Google" using Google Identity Services (`accounts.google.com/gsi/client`), with the same OAuth client ID the backend has as `TEAMS_AUDIENCE` (`NEXT_PUBLIC_GOOGLE_CLIENT_ID`). Signed in: show the email and a "Sign out" link.
2. **Keep the token server-side.** POST the credential (the ID token) to a new route `POST /api/session`, which stores it in an httpOnly cookie `op_user` (`HttpOnly; Secure; SameSite=Lax; Path=/`, `Max-Age` up to the token's `exp`, about 1 hour). `DELETE /api/session` clears it. Don't keep the token in `localStorage`.
3. **Proxy** (`frontend/src/lib/backend.ts`, the `/api/v1` proxy): if the `op_user` cookie is present, send it to the backend as `X-User-Token: <token>`. The proxy builds headers from scratch, so a browser can't inject its own `X-User-Token`. Keep doing that. Keep sending `X-Visitor-ID` as well: a signed-in user still sees the cases they made anonymously in that browser.
4. **Expiry:** Google ID tokens last about an hour. On a backend 401 with `code: "unauthenticated"`, clear the cookie and show "Your sign-in expired. Sign in again." Use GIS auto-select or One Tap to refresh quietly where possible.
5. **Workspace UI** (Cases home, small panel or settings page):
   - `GET /v1/workspaces/mine` → `{email, workspace|null, invites[]}`.
   - No workspace: "Create a team workspace" (name field) → `POST /v1/workspaces {name}`.
   - Pending invitations (`invites`, each `{workspace_id, name, invited_by}`): "alice@acme.com invited you to Acme Imports" with a "Join" button → `POST /v1/workspaces/{workspace_id}/join`. Say that only cases created after joining are shared. 409 means they are already in a workspace.
   - Workspace: name, member list and pending invitations (`invited`, marked "invited"). If `admin`, show "Invite member" (email) → `POST /v1/workspaces/{id}/members {email}`. The invitee joins when they next sign in and accept; nothing is shared before that. Show these errors: 409 "already a member" or "workspace full", 403 "only the admin can invite".
   - A user is in at most one workspace.
6. **History:** show `actor` (email) on history events when present ("Predicted by amy@acme.com").

**Then turn it on:** in `.github/workflows/deploy.yml`, add `TEAMS=on,TEAMS_AUDIENCE=<client id>` to the backend's `--set-env-vars`, and add the frontend origin to the OAuth client's authorised JavaScript origins.

**Acceptance:**
- Alice signs in, creates "Acme", invites Amy. Amy signs in (another browser), sees the invitation, joins, and then sees Alice's cases plus the demos. Before Amy joins, Alice does not see Amy's cases. Bob (signed in, not in Acme) and an anonymous window don't see them, and opening their URL shows not-found.
- A case's history shows who predicted, renamed and exported it.
- Signed out, everything works as today (demos, anonymous cases).
- An expired token shows the sign-in-again note, not a broken page.
