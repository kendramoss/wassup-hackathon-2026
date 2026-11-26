package httpapi

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/metering"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// Workspace metering and budgets (#886, USAGE_METERING). With Deps.Metering
// set and teams on, every model-calling request of a signed-in workspace
// member is charged to the workspace: each prediction, each plan search and
// the model spend of every call made for it, per UTC month. A workspace admin
// may set a monthly budget; a budget with a hard cap refuses model-calling
// requests once the month's spend reaches it (402 workspace_budget_reached)
// and a plan search whose estimate (D87) would pass it. Requests without a
// workspace (anonymous, API keys, the admin token) are not metered here; the
// daily spend cap (D59) still applies to them.

// meterAdd charges d to the request's workspace; a no-op with metering off.
func (a *api) meterAdd(ctx context.Context, d metering.Delta) {
	if a.d.Metering != nil {
		a.d.Metering.Add(ctx, d)
	}
}

// meterGate runs in limit: it puts the caller's workspace in the request
// context so model spend is charged to it, and refuses the request when the
// workspace's hard cap is reached. A store error fails open (logged): the
// daily spend cap still bounds spend.
func (a *api) meterGate(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	if a.d.Metering == nil {
		return r, true
	}
	ws := identityOf(r.Context()).workspace
	if ws == "" {
		return r, true
	}
	st, err := a.d.Metering.Status(r.Context(), ws, a.d.Metering.Month())
	if err != nil {
		a.d.Logger.WarnContext(r.Context(), "workspace usage unavailable; not enforcing the budget", "request_id", requestRef(r.Context()), "err", logsafe.Err(err))
	} else if st.Blocked {
		a.workspaceBudgetReached(w, st, 0)
		return r, false
	}
	return r.WithContext(metering.WithWorkspace(r.Context(), ws)), true
}

// untilNextMonth is the time to the start of the next UTC month.
func untilNextMonth(now time.Time) time.Duration {
	now = now.UTC()
	return time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC).Sub(now)
}

// workspaceBudgetReached writes 402: the workspace's hard cap is reached, or
// estimateUSD (a plan search's estimate) would pass it.
func (a *api) workspaceBudgetReached(w http.ResponseWriter, st metering.Status, estimateUSD float64) {
	secs := int(math.Ceil(untilNextMonth(time.Now()).Seconds()))
	details := map[string]any{"month": st.Totals.Month, "spent_usd": st.Totals.ModelUSD, "retry_after_s": secs}
	if st.Budget != nil {
		details["budget_usd"] = st.Budget.MonthlyUSD
	}
	msg := "your workspace's monthly model budget is used up; a workspace admin can raise it, or new analyses open next month (UTC)"
	if estimateUSD > 0 && !st.Blocked {
		details["estimate_usd"] = estimateUSD
		msg = "this plan search could cost more than your workspace's remaining monthly budget; a workspace admin can raise it"
	}
	w.Header().Set("Retry-After", fmt.Sprint(secs))
	writeError(w, http.StatusPaymentRequired, "workspace_budget_reached", msg, details)
}

// planBudgetCheck refuses a plan search whose estimated worst-case cost would
// pass the workspace's hard cap. Without an estimator, metering, a workspace
// or a hard cap it allows the search.
func (a *api) planBudgetCheck(w http.ResponseWriter, r *http.Request, req planner.Request) bool {
	ws := metering.WorkspaceFrom(r.Context())
	if a.d.Metering == nil || ws == "" {
		return true
	}
	est, ok := a.d.Planner.(Estimator)
	if !ok {
		return true
	}
	st, err := a.d.Metering.Status(r.Context(), ws, a.d.Metering.Month())
	if err != nil || st.Budget == nil || !st.Budget.HardCap {
		return true
	}
	e, err := est.Estimate(r.Context(), req)
	if err != nil {
		return true
	}
	if st.WouldExceed(e.CostUSD.Max) {
		a.workspaceBudgetReached(w, st, e.CostUSD.Max)
		return false
	}
	return true
}

// estimateBudget is the budget block a plan estimate carries: what the
// workspace has left and whether the estimate fits; nil without metering, a
// workspace or a budget.
func (a *api) estimateBudget(r *http.Request, e planner.Estimate) map[string]any {
	ws := identityOf(r.Context()).workspace
	if a.d.Metering == nil || ws == "" {
		return nil
	}
	st, err := a.d.Metering.Status(r.Context(), ws, a.d.Metering.Month())
	if err != nil || st.Budget == nil {
		return nil
	}
	return map[string]any{
		"month": st.Totals.Month, "budget_usd": st.Budget.MonthlyUSD, "spent_usd": st.Totals.ModelUSD,
		"remaining_usd": *st.RemainingUSD, "hard_cap": st.Budget.HardCap,
		"fits": e.CostUSD.Max <= *st.RemainingUSD,
	}
}

// workspaceMember checks that the signed-in caller is in workspace {id}; a
// workspace they are not in is a 404, so its existence does not leak.
func (a *api) workspaceMember(w http.ResponseWriter, r *http.Request) (identity, bool) {
	id, ok := requireUser(w, r)
	if !ok {
		return identity{}, false
	}
	if wsID := r.PathValue("id"); wsID == "" || wsID != id.workspace {
		auditHidden(r)
		a.workspaceError(w, r, teams.ErrNotFound, "")
		return identity{}, false
	}
	auditConfirm(r, "workspace", id.workspace)
	return id, true
}

// workspaceUsage is GET /v1/workspaces/{id}/usage?month=YYYY-MM (default:
// this month): the month's predictions, plan searches and model spend, and
// the budget. Any member may read it.
func (a *api) workspaceUsage(w http.ResponseWriter, r *http.Request) {
	id, ok := a.workspaceMember(w, r)
	if !ok {
		return
	}
	month := a.d.Metering.Month()
	if q := r.URL.Query().Get("month"); q != "" {
		m, err := metering.ParseMonth(q)
		if err != nil {
			a.fail(w, r, fmt.Errorf("%w: %v", domain.ErrInvalid, err))
			return
		}
		month = m
	}
	st, err := a.d.Metering.Status(r.Context(), id.workspace, month)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspace_id": id.workspace, "usage": st})
}

type budgetRequest struct {
	MonthlyUSD   float64 `json:"monthly_usd"`
	HardCap      bool    `json:"hard_cap"`
	AlertPercent int     `json:"alert_percent"`
}

// putWorkspaceBudget is PUT /v1/workspaces/{id}/budget: a workspace admin
// sets the monthly budget. DELETE removes it (use is still counted).
func (a *api) putWorkspaceBudget(w http.ResponseWriter, r *http.Request) {
	id, ok := a.workspaceMember(w, r)
	if !ok {
		return
	}
	var in budgetRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	b := metering.Budget{MonthlyUSD: in.MonthlyUSD, HardCap: in.HardCap, AlertPercent: in.AlertPercent}
	if err := b.Validate(); err != nil {
		a.fail(w, r, fmt.Errorf("%w: %v", domain.ErrInvalid, err))
		return
	}
	if id.role != teams.RoleAdmin {
		a.workspaceError(w, r, teams.ErrForbidden, "")
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(id.workspace), Object: objectRef("workspace", id.workspace), Detail: "set", Action: audit.WorkspaceBudget}); err != nil {
		a.fail(w, r, err)
		return
	}
	saved, err := a.d.Metering.SetBudget(r.Context(), id.workspace, id.email, b)
	if err != nil {
		if errors.Is(err, metering.ErrInvalid) {
			err = fmt.Errorf("%w: %v", domain.ErrInvalid, err)
		}
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspace_id": id.workspace, "budget": saved})
}

func (a *api) deleteWorkspaceBudget(w http.ResponseWriter, r *http.Request) {
	id, ok := a.workspaceMember(w, r)
	if !ok {
		return
	}
	if id.role != teams.RoleAdmin {
		a.workspaceError(w, r, teams.ErrForbidden, "")
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(id.workspace), Object: objectRef("workspace", id.workspace), Detail: "clear", Action: audit.WorkspaceBudget}); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.d.Metering.DeleteBudget(r.Context(), id.workspace); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
