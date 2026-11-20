package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// GET /v1/audit and GET /v1/audit/verify (#284).
//
// Who may read a tenant's trail:
//   - the admin token, for the tenant named in ?tenant= (required);
//   - a signed-in user whose CURRENT stored role in their workspace is admin,
//     for that workspace's tenant. The tenant is never taken from the request
//     for them: a ?tenant= from anyone but the admin token is a 400.
//
// Everyone else is refused the same way: 401 under STRICT_AUTH when they are
// not verified at all, else 403 (viewer, editor, a user without a workspace,
// an API key, a visitor, nobody). A personal tenant has no tenant admin, so
// only the admin token reads it.

// auditTenant resolves the tenant the caller may read, or writes the refusal.
func (a *api) auditTenant(w http.ResponseWriter, r *http.Request) (string, bool) {
	p := a.principalOf(r)
	switch {
	case p.Admin:
		t := r.URL.Query().Get("tenant")
		if !audit.ValidTenant(t) {
			a.fail(w, r, fmt.Errorf("%w: tenant is required and must be a tenant id", domain.ErrInvalid))
			return "", false
		}
		return t, true
	case a.d.StrictAuth && !p.Verified():
		a.fail(w, r, errNeedsVerified)
	case p.signedIn() && p.Workspace != "" && p.Role == teams.RoleAdmin:
		if r.URL.Query().Has("tenant") {
			a.fail(w, r, fmt.Errorf("%w: tenant cannot be set; your workspace is implied", domain.ErrInvalid))
			return "", false
		}
		return workspaceTenant(p.Workspace), true
	default:
		writeError(w, http.StatusForbidden, "forbidden", "only a workspace admin, or the admin token, can read the audit trail", nil)
	}
	return "", false
}

func allowedQuery(r *http.Request, names ...string) error {
	ok := map[string]bool{}
	for _, n := range names {
		ok[n] = true
	}
	for k := range r.URL.Query() {
		if !ok[k] {
			return fmt.Errorf("%w: unknown parameter %q", domain.ErrInvalid, k)
		}
	}
	return nil
}

func (a *api) auditListQuery(r *http.Request) (audit.ListQuery, error) {
	q := r.URL.Query()
	if err := allowedQuery(r, "tenant", "limit", "cursor", "order", "action", "outcome", "phase", "actor", "object", "from", "to"); err != nil {
		return audit.ListQuery{}, err
	}
	bad := func(format string, args ...any) (audit.ListQuery, error) {
		return audit.ListQuery{}, fmt.Errorf("%w: %s", domain.ErrInvalid, fmt.Sprintf(format, args...))
	}
	out := audit.ListQuery{Cursor: q.Get("cursor"), Desc: true}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			return bad("limit must be 1 to 200")
		}
		out.Limit = n
	}
	switch q.Get("order") {
	case "", "desc":
	case "asc":
		out.Desc = false
	default:
		return bad("order must be asc or desc")
	}
	if v := q.Get("action"); v != "" {
		if !audit.Action(v).Known() {
			return bad("unknown action")
		}
		out.Filter.Action = audit.Action(v)
	}
	switch v := audit.Outcome(q.Get("outcome")); v {
	case "", audit.Allowed, audit.Denied, audit.Failed:
		out.Filter.Outcome = v
	default:
		return bad("outcome must be allowed, denied or failed")
	}
	switch v := audit.Phase(q.Get("phase")); v {
	case "", audit.PhaseIntent, audit.PhaseCompletion, audit.PhaseSingle:
		out.Filter.Phase = v
	default:
		return bad("phase must be intent, completion or single")
	}
	if v := q.Get("actor"); v != "" {
		if !audit.ValidActor(v) {
			return bad("actor must be a pseudonym as shown in events")
		}
		out.Filter.Actor = v
	}
	if v := q.Get("object"); v != "" {
		if !audit.ValidObject(v) {
			return bad("object must be an object reference as shown in events")
		}
		out.Filter.Object = v
	}
	for name, dst := range map[string]*time.Time{"from": &out.Filter.From, "to": &out.Filter.To} {
		if v := q.Get(name); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return bad("%s must be an RFC 3339 time", name)
			}
			*dst = t.UTC()
		}
	}
	return out, nil
}

// auditList is GET /v1/audit.
func (a *api) auditList(w http.ResponseWriter, r *http.Request) {
	tenant, ok := a.auditTenant(w, r)
	if !ok {
		return
	}
	q, err := a.auditListQuery(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	order := "desc"
	if !q.Desc {
		order = "asc"
	}
	page, err := a.d.Audit.List(r.Context(), tenant, q)
	switch {
	case errors.Is(err, audit.ErrBadQuery):
		a.fail(w, r, fmt.Errorf("%w: the cursor or a parameter is not valid for this listing", domain.ErrInvalid))
		return
	case err != nil:
		a.fail(w, r, err)
		return
	}
	if page.Events == nil {
		page.Events = []audit.Event{}
	}
	out := map[string]any{"tenant": tenant, "retention_days": int(a.d.Audit.Retention().Hours() / 24), "order": order, "events": page.Events}
	if page.NextCursor != "" {
		out["next_cursor"] = page.NextCursor
	}
	// The read of a trail is itself recorded (first page, a tenant with events),
	// in the tenant that was read.
	if q.Cursor == "" && len(page.Events) > 0 {
		a.auditSingle(r, auditStep{Tenant: tenant})
	}
	writeJSON(w, http.StatusOK, out)
}

// auditVerify is GET /v1/audit/verify.
func (a *api) auditVerify(w http.ResponseWriter, r *http.Request) {
	tenant, ok := a.auditTenant(w, r)
	if !ok {
		return
	}
	if err := allowedQuery(r, "tenant", "cursor"); err != nil {
		a.fail(w, r, err)
		return
	}
	cursor := r.URL.Query().Get("cursor")
	res, err := a.d.Audit.Verify(r.Context(), tenant, cursor)
	switch {
	case errors.Is(err, audit.ErrBadQuery):
		a.fail(w, r, fmt.Errorf("%w: the cursor is not valid for this verification", domain.ErrInvalid))
		return
	case err != nil:
		a.fail(w, r, err)
		return
	}
	if cursor == "" && (res.Checked > 0 || res.Checkpoint != nil) {
		a.auditSingle(r, auditStep{Tenant: tenant})
	}
	writeJSON(w, http.StatusOK, res)
}
