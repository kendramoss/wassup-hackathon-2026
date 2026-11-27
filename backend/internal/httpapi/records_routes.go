package httpapi

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retention"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// Records routes: audit export (#871) and workspace retention (#875). Both
// are off unless configured (AUDIT_EXPORT=on, RETENTION=on).

// auditExport is GET /v1/audit/export: the tenant's retained trail as JSONL
// (header, events, summary; checkable offline with cmd/auditverify) or CSV.
// Same readers as GET /v1/audit.
func (a *api) auditExport(w http.ResponseWriter, r *http.Request) {
	tenant, ok := a.auditTenant(w, r)
	if !ok {
		return
	}
	if err := allowedQuery(r, "tenant", "format", "from", "to"); err != nil {
		a.fail(w, r, err)
		return
	}
	q := r.URL.Query()
	format := q.Get("format")
	if format == "" {
		format = "jsonl"
	}
	if format != "jsonl" && format != "csv" {
		a.fail(w, r, fmt.Errorf("%w: format must be jsonl or csv", domain.ErrInvalid))
		return
	}
	h, err := a.d.Audit.ExportSnapshot(r.Context(), tenant)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	for name, dst := range map[string]**time.Time{"from": &h.From, "to": &h.To} {
		if v := q.Get(name); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				a.fail(w, r, fmt.Errorf("%w: %s must be an RFC 3339 time", domain.ErrInvalid, name))
				return
			}
			t = t.UTC()
			*dst = &t
		}
	}
	// The export is recorded before it is streamed, in the tenant exported.
	a.auditSingle(r, auditStep{Tenant: tenant, Detail: format})
	name := fmt.Sprintf("audit-%s-%s.%s", strings.NewReplacer(":", "-").Replace(tenant), h.ExportedAt.Format("20060102T150405Z"), format)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("X-Audit-Head-Seq", strconv.FormatUint(h.Head.Seq, 10))
	w.Header().Set("X-Audit-Head-Hash", h.Head.Hash)
	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		cw := csv.NewWriter(w)
		cols := []string{"tenant", "seq", "time", "actor_kind", "actor", "target", "action", "phase", "outcome", "reason", "detail", "object", "route", "request_id", "ref", "suppressed", "prev_hash", "hash"}
		_ = cw.Write(cols)
		_, _, err := a.d.Audit.ExportEvents(r.Context(), h, 0, func(e audit.Event) error {
			return cw.Write([]string{e.Tenant, strconv.FormatUint(e.Seq, 10), e.Time.Format(time.RFC3339Nano), string(e.ActorKind), e.Actor,
				e.Target, string(e.Action), string(e.Phase), string(e.Outcome), string(e.Reason), e.Detail, e.Object, e.Route, e.RequestID,
				strconv.FormatUint(e.Ref, 10), strconv.Itoa(e.Suppressed), e.PrevHash, e.Hash})
		})
		cw.Flush()
		if err != nil {
			a.d.Logger.Warn("audit export stopped", "class", "store")
		}
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	x := audit.NewExportWriter(w)
	if err := x.Header(h); err != nil {
		return
	}
	_, complete, err := a.d.Audit.ExportEvents(r.Context(), h, 0, x.Event)
	if err != nil {
		// Headers are sent; end without a summary so the file fails offline
		// verification instead of looking whole.
		_ = x.Flush()
		a.d.Logger.Warn("audit export stopped", "class", "store")
		return
	}
	_, _ = x.Summary(complete)
}

type retentionRequest struct {
	CaseDays *int `json:"case_days"`
}

func retentionOut(p retention.Policy) map[string]any {
	out := map[string]any{"workspace": p.Workspace, "case_days": p.CaseDays,
		"limits": map[string]int{"min_case_days": retention.MinCaseDays, "max_case_days": retention.MaxCaseDays}}
	if !p.UpdatedAt.IsZero() {
		out["updated_at"] = p.UpdatedAt
	}
	return out
}

// retentionWorkspace checks the caller is a member of the workspace in the
// path, and an admin when admin is true.
func (a *api) retentionWorkspace(w http.ResponseWriter, r *http.Request, admin bool) (string, bool) {
	id, ok := requireUser(w, r)
	if !ok {
		return "", false
	}
	wsID := r.PathValue("id")
	if wsID != id.workspace {
		auditHidden(r)
		a.workspaceError(w, r, teams.ErrNotFound, "")
		return "", false
	}
	auditConfirm(r, "workspace", wsID)
	if admin && id.role != teams.RoleAdmin {
		a.workspaceError(w, r, teams.ErrForbidden, "")
		return "", false
	}
	return wsID, true
}

// getRetention is GET /v1/workspaces/{id}/retention (any member).
func (a *api) getRetention(w http.ResponseWriter, r *http.Request) {
	wsID, ok := a.retentionWorkspace(w, r, false)
	if !ok {
		return
	}
	p, _, err := a.d.Retention.Policy(r.Context(), wsID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	p.Workspace = wsID
	writeJSON(w, http.StatusOK, retentionOut(p))
}

// putRetention is PUT /v1/workspaces/{id}/retention (admin): case_days 0
// keeps cases forever; otherwise the purge run deletes cases not updated for
// that many days.
func (a *api) putRetention(w http.ResponseWriter, r *http.Request) {
	wsID, ok := a.retentionWorkspace(w, r, true)
	if !ok {
		return
	}
	var in retentionRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if in.CaseDays == nil {
		a.fail(w, r, fmt.Errorf("%w: case_days is required", domain.ErrInvalid))
		return
	}
	p := retention.Policy{Workspace: wsID, CaseDays: *in.CaseDays, UpdatedAt: time.Now().UTC()}
	if err := p.Validate(); err != nil {
		a.fail(w, r, fmt.Errorf("%w: %s", domain.ErrInvalid, strings.TrimPrefix(err.Error(), "retention: invalid: ")))
		return
	}
	detail := "keep"
	if p.CaseDays > 0 {
		detail = "expire"
	}
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(wsID), Object: objectRef("workspace", wsID), Detail: detail}); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.d.Retention.SetPolicy(r.Context(), p); err != nil {
		if errors.Is(err, retention.ErrInvalid) {
			err = fmt.Errorf("%w: %s", domain.ErrInvalid, err)
		}
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, retentionOut(p))
}

// listTombstones is GET /v1/workspaces/{id}/tombstones (admin): the
// workspace's hard-deleted cases, newest first. No case content.
func (a *api) listTombstones(w http.ResponseWriter, r *http.Request) {
	wsID, ok := a.retentionWorkspace(w, r, true)
	if !ok {
		return
	}
	if err := allowedQuery(r, "limit"); err != nil {
		a.fail(w, r, err)
		return
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			a.fail(w, r, fmt.Errorf("%w: limit must be 1 to 500", domain.ErrInvalid))
			return
		}
		limit = n
	}
	ts, err := a.d.Retention.Tombstones(r.Context(), wsID, limit)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if ts == nil {
		ts = []retention.Tombstone{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspace": wsID, "tombstones": ts})
}
