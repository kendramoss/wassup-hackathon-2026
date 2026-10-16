# Frontend: send a visitor ID so each visitor sees only their own cases (D58)

**Backend status:** done on `main` once `fix/origin-proof-hardening` merges. It's off by default (`CASE_OWNERSHIP=off`).

**Problem:** anyone visiting the demo can list, open, rename and delete every other visitor's cases. A judge sees what another judge typed.

**What the frontend needs to do** (in `frontend/src/lib/backend.ts`, the `/api/v1` proxy):
1. **Cookie:** on each proxied request, read the cookie `op_visitor`. If it's missing, create one: 32 random bytes, base64url-encoded (only `A-Z a-z 0-9 _ -`, 16–128 characters). Set it `HttpOnly; Secure; SameSite=Lax; Path=/; Max-Age=31536000` on the response.
2. **Header:** send it to the backend as `X-Visitor-ID: <value>`. The proxy already builds its headers from scratch, so a browser can't inject its own `X-Visitor-ID`. Keep it that way.
3. **Never** expose the ID to client-side JavaScript, and never put it in a URL.
4. **UI:** Cases home shows the pinned demos plus the visitor's own cases. Remove any copy that implies a shared list. Another visitor's case link now returns 404; show the normal "case not found" state.

**Then turn it on:** add `CASE_OWNERSHIP=on` to the backend's `--set-env-vars` in `.github/workflows/deploy.yml`.

**Acceptance:**
- Two browsers (or one normal and one private window) each create a case. Neither sees the other's case on Cases home, and opening the other's case URL shows not-found. Both see the 5 pinned demos.
- Deleting your own case works.
- The demo click path (Cases → heat gun → … → Draft) is unchanged.

**Note:** existing unpinned cases have no owner, so once ownership is on only the admin sees them. That's acceptable; they're test cases.
