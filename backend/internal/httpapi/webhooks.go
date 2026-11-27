package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"fmt"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/metering"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/webhooks"
)

// Webhooks (#915, D936, docs/api/openapi.yaml). Off unless Deps.Webhooks is
// set (WEBHOOKS=on, which needs TEAMS=on). A workspace admin registers HTTPS
// endpoints; the server posts the events each endpoint subscribed to, signed
// with HMAC-SHA256 (see the webhooks package), retries failures on a backoff
// schedule and keeps a delivery log per endpoint. Events carry ids and
// statuses only, never case text, comments or letters.

// webhookView is an endpoint as the admin sees it. Secret is set only in the
// answers to create and rotate: it is shown once.
type webhookView struct {
	ID                      string     `json:"id"`
	URL                     string     `json:"url"`
	Events                  []string   `json:"events"`
	Description             string     `json:"description"`
	Enabled                 bool       `json:"enabled"`
	SecretHint              string     `json:"secret_hint"`
	CreatedBy               string     `json:"created_by"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
	SecretRotatedAt         *time.Time `json:"secret_rotated_at,omitempty"`
	PreviousSecretExpiresAt *time.Time `json:"previous_secret_expires_at,omitempty"`
	Secret                  string     `json:"secret,omitempty"`
}

func (a *api) webhookView(e webhooks.Endpoint, withSecret bool) webhookView {
	v := webhookView{ID: e.ID, URL: e.URL, Events: e.Events, Description: e.Description, Enabled: e.Enabled, SecretHint: e.SecretHint(),
		CreatedBy: e.CreatedBy, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt}
	if v.Events == nil {
		v.Events = []string{}
	}
	if !e.RotatedAt.IsZero() {
		v.SecretRotatedAt = &e.RotatedAt
	}
	if e.PrevSecret != "" && time.Now().Before(e.PrevUntil) {
		v.PreviousSecretExpiresAt = &e.PrevUntil
	}
	if withSecret {
		v.Secret = e.Secret
	}
	return v
}

type deliveryView struct {
	ID          string             `json:"id"`
	Event       string             `json:"event"`
	EventID     string             `json:"event_id"`
	Test        bool               `json:"test"`
	Status      string             `json:"status"`
	Attempts    []webhooks.Attempt `json:"attempts"`
	NextAttempt *time.Time         `json:"next_attempt_at,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
}

func deliveryViewOf(d webhooks.Delivery) deliveryView {
	v := deliveryView{ID: d.ID, Event: d.Event, EventID: d.EventID, Test: d.Test, Status: d.Status, Attempts: d.Attempts, CreatedAt: d.CreatedAt}
	if v.Attempts == nil {
		v.Attempts = []webhooks.Attempt{}
	}
	if !d.NextAttempt.IsZero() {
		v.NextAttempt = &d.NextAttempt
	}
	return v
}

// emit sends a workspace event to its subscribed endpoints; it does nothing
// with webhooks off or for a case outside a workspace, and never fails the
// request that caused the event.
func (a *api) emit(ctx context.Context, workspace, typ string, data map[string]any) {
	if a.d.Webhooks != nil && workspace != "" {
		a.d.Webhooks.Emit(ctx, workspace, typ, data)
	}
}

// wireBudgetAlerts has the usage meter tell the workspace's endpoints when
// its month passes the budget's alert share or the budget itself.
func (a *api) wireBudgetAlerts() {
	if a.d.Webhooks == nil || a.d.Metering == nil {
		return
	}
	a.d.Metering.OnCross(func(ctx context.Context, c metering.Crossing) {
		a.emit(ctx, c.Workspace, webhooks.EventBudgetAlert, map[string]any{
			"month": c.Month, "level": c.Level, "spent_usd": c.SpentUSD, "budget_usd": c.Budget.MonthlyUSD,
			"alert_percent": alertPercent(c.Budget), "hard_cap": c.Budget.HardCap,
		})
	})
}

func alertPercent(b metering.Budget) int {
	if b.AlertPercent == 0 {
		return metering.DefaultAlertPercent
	}
	return b.AlertPercent
}

// webhookEndpoint loads the endpoint {wid} of the admin's own workspace;
// another workspace's is a 404 that names nothing.
func (a *api) webhookEndpoint(w http.ResponseWriter, r *http.Request, id identity) (webhooks.Endpoint, bool) {
	e, err := a.d.Webhooks.Store().GetEndpoint(r.Context(), r.PathValue("wid"))
	if err == nil && e.Workspace != id.workspace {
		err = webhooks.ErrNotFound
	}
	if err != nil {
		a.fail(w, r, err)
		return webhooks.Endpoint{}, false
	}
	return e, true
}

func (a *api) listWebhooks(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ssoAdmin(w, r)
	if !ok {
		return
	}
	eps, err := a.d.Webhooks.Store().ListEndpoints(r.Context(), id.workspace)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out := make([]webhookView, 0, len(eps))
	for _, e := range eps {
		out = append(out, a.webhookView(e, false))
	}
	writeJSON(w, http.StatusOK, map[string]any{"endpoints": out, "available_events": webhooks.Events, "max_endpoints": webhooks.MaxEndpoints})
}

type webhookRequest struct {
	URL         *string   `json:"url"`
	Events      *[]string `json:"events"`
	Description *string   `json:"description"`
	Enabled     *bool     `json:"enabled"`
}

func (a *api) createWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ssoAdmin(w, r)
	if !ok {
		return
	}
	var in webhookRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if in.URL == nil || in.Events == nil {
		a.fail(w, r, fmt.Errorf("%w: url and events are required", webhooks.ErrInvalid))
		return
	}
	svc := a.d.Webhooks
	u, err := svc.CheckURL(r.Context(), *in.URL)
	var events []string
	if err == nil {
		events, err = webhooks.ValidateEvents(*in.Events)
	}
	desc := ""
	if err == nil && in.Description != nil {
		desc, err = webhooks.ValidateDescription(*in.Description)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	enabled := in.Enabled == nil || *in.Enabled
	existing, err := svc.Store().ListEndpoints(r.Context(), id.workspace)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if len(existing) >= webhooks.MaxEndpoints {
		a.fail(w, r, webhooks.ErrFull)
		return
	}
	now := time.Now().UTC()
	e := webhooks.Endpoint{ID: webhooks.NewID("wh_"), Workspace: id.workspace, URL: u, Events: events, Description: desc, Enabled: enabled,
		Secret: webhooks.NewSecret(), CreatedBy: id.email, CreatedAt: now, UpdatedAt: now}
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(id.workspace), Object: objectRef("webhook", e.ID)}); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := svc.Store().PutEndpoint(r.Context(), e); err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"endpoint": a.webhookView(e, true)})
}

func (a *api) updateWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ssoAdmin(w, r)
	if !ok {
		return
	}
	e, ok := a.webhookEndpoint(w, r, id)
	if !ok {
		return
	}
	var in webhookRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if in.URL == nil && in.Events == nil && in.Description == nil && in.Enabled == nil {
		a.fail(w, r, fmt.Errorf("%w: give at least one of url, events, description or enabled", webhooks.ErrInvalid))
		return
	}
	var err error
	if in.URL != nil {
		e.URL, err = a.d.Webhooks.CheckURL(r.Context(), *in.URL)
	}
	if err == nil && in.Events != nil {
		e.Events, err = webhooks.ValidateEvents(*in.Events)
	}
	if err == nil && in.Description != nil {
		e.Description, err = webhooks.ValidateDescription(*in.Description)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if in.Enabled != nil {
		e.Enabled = *in.Enabled
	}
	e.UpdatedAt = time.Now().UTC()
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(id.workspace), Object: objectRef("webhook", e.ID)}); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.d.Webhooks.Store().PutEndpoint(r.Context(), e); err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"endpoint": a.webhookView(e, false)})
}

func (a *api) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ssoAdmin(w, r)
	if !ok {
		return
	}
	e, ok := a.webhookEndpoint(w, r, id)
	if !ok {
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(id.workspace), Object: objectRef("webhook", e.ID)}); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.d.Webhooks.Store().DeleteEndpoint(r.Context(), e.ID); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// rotateWebhook gives the endpoint a new secret, shown once. The old secret
// signs beside it for a day, so a receiver can switch without a gap.
func (a *api) rotateWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ssoAdmin(w, r)
	if !ok {
		return
	}
	e, ok := a.webhookEndpoint(w, r, id)
	if !ok {
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(id.workspace), Object: objectRef("webhook", e.ID)}); err != nil {
		a.fail(w, r, err)
		return
	}
	e, err := a.d.Webhooks.Rotate(r.Context(), e.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"endpoint": a.webhookView(e, true)})
}

// testWebhook sends one test event now, to a disabled endpoint too, and
// answers with how the receiver took it. The ping is in the delivery log and
// is not retried.
func (a *api) testWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ssoAdmin(w, r)
	if !ok {
		return
	}
	e, ok := a.webhookEndpoint(w, r, id)
	if !ok {
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(id.workspace), Object: objectRef("webhook", e.ID)}); err != nil {
		a.fail(w, r, err)
		return
	}
	d, err := a.d.Webhooks.Ping(r.Context(), e)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"delivery": deliveryViewOf(d)})
}

func (a *api) listWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ssoAdmin(w, r)
	if !ok {
		return
	}
	e, ok := a.webhookEndpoint(w, r, id)
	if !ok {
		return
	}
	limit := 50
	if err := allowedQuery(r, "limit"); err != nil {
		a.fail(w, r, err)
		return
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > webhooks.MaxDeliveriesKept {
			a.fail(w, r, fmt.Errorf("%w: limit must be 1 to %d", domain.ErrInvalid, webhooks.MaxDeliveriesKept))
			return
		}
		limit = n
	}
	ds, err := a.d.Webhooks.Store().ListDeliveries(r.Context(), e.ID, limit)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out := make([]deliveryView, 0, len(ds))
	for _, d := range ds {
		out = append(out, deliveryViewOf(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{"deliveries": out})
}

// sweepWebhooks sends the retries now due (admin token; for a scheduler).
func (a *api) sweepWebhooks(w http.ResponseWriter, r *http.Request) {
	if err := a.auditBegin(r, auditStep{Tenant: "platform"}); err != nil {
		a.fail(w, r, err)
		return
	}
	n, err := a.d.Webhooks.Sweep(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"attempted": n})
}
