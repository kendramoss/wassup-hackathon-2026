package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/apikey"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/cross"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// Audit hooks (#284). See docs/design/audit-trail.md.
//
// withAudit wraps the mux. It does three things for a route in auditTable:
//
//   - a handler that has authorized and validated a write calls auditBegin
//     (the intent). For a sensitive action the intent must be stored first: if
//     it cannot be, the handler answers 503 audit_unavailable and changes
//     nothing. Everything else carries on if the intent cannot be stored.
//   - when the handler commits its status (WriteHeader, or the first Write),
//     this wrapper writes the completion or failure BEFORE the status goes out,
//     so a completion that cannot be stored can still be shown to the caller in
//     the X-Audit-Incomplete header. The response is not buffered and the
//     status is never changed: the mutation is not rolled back and is not
//     reported as failed. An event stream commits 200 before the work runs, so
//     it finishes explicitly (auditStreamEnd) and a late failure can only be
//     logged.
//   - a 401 or 403 (and a 404 that hid an existing case) with no intent is a
//     denial, recorded best effort in the REQUESTER'S tenant.
//
// Nothing here is atomic with the stored change: the audit write and the case,
// workspace or key write are separate stores.

const (
	auditIncompleteHeader = "X-Audit-Incomplete"
	auditIncompleteValue  = "completion-not-recorded"
	auditWriteTimeout     = 5 * time.Second
)

var errAuditUnavailable = errors.New("audit trail unavailable")

type auditClass int

const (
	auditNone       auditClass = iota // no authorization decision and no state: nothing is recorded
	auditDenyOnly                     // an allowed read is not audited; a denial is
	auditTwoPhase                     // intent, then completion or failure
	auditSingleOnly                   // one event written by the handler
)

type auditRoute struct {
	Class  auditClass
	Action audit.Action
}

// auditTable is the coverage table: every route registered in server.go, keyed
// by mux pattern. TestEveryRegisteredRouteHasAnAuditRow fails for a route not
// listed here, so a new endpoint cannot ship without a decision.
var auditTable = map[string]auditRoute{
	"POST /v1/cases/predict":                       {auditTwoPhase, audit.CaseCreate},
	"POST /v1/cases/batch":                         {auditTwoPhase, audit.CaseCreate},
	"GET /v1/cases":                                {auditDenyOnly, audit.CaseRead},
	"POST /v1/products/import":                     {auditNone, ""},
	"POST /v1/products/extract":                    {auditNone, ""},
	"POST /v1/products/extract-media":              {auditNone, ""},
	"POST /v1/products/money-prefill":              {auditNone, ""},
	"POST /v1/products/hts-suggest":                {auditNone, ""},
	"POST /v1/products/parts-hts-suggest":          {auditNone, ""},
	"POST /v1/products/cost-estimate":              {auditNone, ""},
	"POST /v1/products/family-check":               {auditNone, ""},
	"POST /v1/cases/{id}/bom-version":              {auditTwoPhase, audit.CaseUpdate},
	"POST /v1/cases/{id}/scenarios":                {auditTwoPhase, audit.CaseUpdate},
	"POST /v1/cases/{id}/compare-targets":          {auditTwoPhase, audit.CaseUpdate},
	"POST /v1/cases/{id}/ask":                      {auditTwoPhase, audit.CaseUpdate},
	"PUT /v1/cases/{id}/economics":                 {auditTwoPhase, audit.CaseUpdate},
	"PUT /v1/cases/{id}/pin":                       {auditTwoPhase, audit.CasePin},
	"POST /v1/cases/{id}/watch":                    {auditTwoPhase, audit.CaseUpdate},
	"POST /v1/watch/run":                           {auditTwoPhase, audit.AdminWatchRun},
	"GET /v1/cases/{id}":                           {auditDenyOnly, audit.CaseRead},
	"DELETE /v1/cases/{id}":                        {auditTwoPhase, audit.CaseDelete},
	"PATCH /v1/cases/{id}":                         {auditTwoPhase, audit.CaseUpdate},
	"POST /v1/cases/{id}/plan":                     {auditTwoPhase, audit.CaseUpdate},
	"POST /v1/cases/{id}/plan/estimate":            {auditDenyOnly, audit.CaseRead},
	"POST /v1/cases/{id}/export":                   {auditTwoPhase, audit.CaseExport},
	"POST /v1/cases/{id}/answers":                  {auditTwoPhase, audit.CaseUpdate},
	"PUT /v1/cases/{id}/origin-facts/{market}":     {auditTwoPhase, audit.CaseUpdate},
	"GET /v1/cases/{id}/evidence-report":           {auditDenyOnly, audit.CaseRead},
	"GET /v1/rulings/{id}":                         {auditNone, ""},
	"GET /v1/rulings":                              {auditNone, ""},
	"POST /mcp":                                    {auditNone, ""}, // each tool call is its own audited /v1 request
	"GET /v1/eval/summary":                         {auditNone, ""},
	"GET /v1/duty-rates":                           {auditNone, ""},
	"POST /v1/rcep/check":                          {auditNone, ""},
	"POST /v1/usmca/check":                         {auditNone, ""},
	"POST /v1/rules/canada-marking":                {auditNone, ""},
	"POST /v1/rules/eu-origin":                     {auditNone, ""},
	"POST /v1/rules/uk-origin":                     {auditNone, ""},
	"GET /v1/demos":                                {auditNone, ""},
	"GET /v1/jurisdictions":                        {auditNone, ""},
	"GET /v1/demos/{id}":                           {auditNone, ""},
	"POST /v1/workspaces":                          {auditTwoPhase, audit.WorkspaceCreate},
	"GET /v1/workspaces/mine":                      {auditNone, ""},
	"POST /v1/workspaces/{id}/members":             {auditTwoPhase, audit.WorkspaceInvite},
	"POST /v1/workspaces/{id}/join":                {auditTwoPhase, audit.WorkspaceJoin},
	"PUT /v1/workspaces/{id}/members/{email}/role": {auditTwoPhase, audit.WorkspaceRoleChange},
	"DELETE /v1/workspaces/{id}/members/{email}":   {auditTwoPhase, audit.WorkspaceMemberRemove},
	"POST /v1/admin/api-keys":                      {auditTwoPhase, audit.APIKeyIssue},
	"GET /v1/admin/api-keys":                       {auditSingleOnly, audit.APIKeyList},
	"DELETE /v1/admin/api-keys/{id}":               {auditTwoPhase, audit.APIKeyDisable},
	"GET /v1/audit":                                {auditSingleOnly, audit.AuditRead},
	"GET /v1/audit/verify":                         {auditSingleOnly, audit.AuditVerify},
	"POST /v1/cases/{id}/reviews":                  {auditTwoPhase, audit.ReviewCreate},
	"GET /v1/cases/{id}/reviews":                   {auditDenyOnly, audit.CaseRead},
	"DELETE /v1/cases/{id}/reviews/{review}":       {auditTwoPhase, audit.ReviewRevoke},
	"GET /v1/review":                               {auditSingleOnly, audit.ReviewView},
	"POST /v1/review/decision":                     {auditTwoPhase, audit.ReviewDecision},
	"GET /health":                                  {auditNone, ""},
}

type auditCtxKey struct{}

// auditOp is one request's audit state.
type auditOp struct {
	a       *api
	req     *http.Request
	pattern string
	route   auditRoute

	intent     *audit.Event // set once the intent is begun (Seq 0: it could not be stored)
	closed     bool         // the completion, denial or single event has been written (or tried)
	streaming  bool         // an event stream committed 200 before the work ran
	incomplete bool         // a completion or single event could not be stored

	object string // a server-confirmed object ref for denials ("" none)
	hidden bool   // a 404 that hid an existing case: a denial
}

func auditOpOf(ctx context.Context) *auditOp {
	op, _ := ctx.Value(auditCtxKey{}).(*auditOp)
	return op
}

// withAudit wraps the mux (inside withUser and withAPIKeys, so the verified
// principal is already in the context).
func (a *api) withAudit(mux *http.ServeMux, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := mux.Handler(r)
		rt, ok := auditTable[pattern]
		if !ok || rt.Class == auditNone {
			next.ServeHTTP(w, r)
			return
		}
		op := &auditOp{a: a, pattern: pattern, route: rt}
		rr := r.WithContext(context.WithValue(r.Context(), auditCtxKey{}, op))
		op.req = rr
		aw := &auditWriter{ResponseWriter: w, op: op}
		defer func() {
			if rec := recover(); rec != nil {
				op.finish(audit.Failed, audit.ReasonInternal, "")
				panic(rec)
			}
		}()
		next.ServeHTTP(aw, rr)
		if !aw.committed {
			aw.commit(http.StatusOK)
		}
		if op.streaming && !op.closed { // a stream that never reported its end
			op.finish(audit.Allowed, audit.ReasonNone, "")
		}
	})
}

// auditWriter commits the audit state when the handler commits its status.
type auditWriter struct {
	http.ResponseWriter
	op        *auditOp
	committed bool
}

func (w *auditWriter) commit(code int) {
	if w.committed {
		return
	}
	w.committed = true
	w.op.commit(w.ResponseWriter.Header(), code)
}

func (w *auditWriter) WriteHeader(code int) {
	w.commit(code)
	w.ResponseWriter.WriteHeader(code)
}

func (w *auditWriter) Write(b []byte) (int, error) {
	w.commit(http.StatusOK)
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach Flush on the real writer (SSE).
func (w *auditWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (op *auditOp) commit(h http.Header, code int) {
	switch {
	case op.closed:
	case op.intent != nil:
		if strings.HasPrefix(h.Get("Content-Type"), "text/event-stream") {
			op.streaming = true // the stream reports its own end
			break
		}
		if code < 400 {
			op.finish(audit.Allowed, audit.ReasonNone, op.object)
		} else {
			op.finish(audit.Failed, audit.ReasonForStatus(code), op.object)
		}
	default:
		op.deny(code)
	}
	if op.incomplete {
		h.Set(auditIncompleteHeader, auditIncompleteValue)
	}
}

// auditCtx is a context that survives the client going away: an operation
// that ran must be recorded even if the caller hung up.
func auditCtx(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), auditWriteTimeout)
}

// logFail logs that an audit write failed. It records the request, the action
// and an error CLASS only: never the error text, which may carry store or
// document details.
func (op *auditOp) logFail(what string, seq uint64, err error) {
	op.a.d.Logger.ErrorContext(op.req.Context(), "audit write failed", "what", what, "request_id", requestRef(op.req.Context()),
		"action", string(op.route.Action), "route", op.pattern, "intent_seq", seq, "class", auditErrClass(err))
}

func auditErrClass(err error) string {
	switch {
	case errors.Is(err, audit.ErrInvalid):
		return "invalid_event"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "timeout"
	}
	return "store"
}

// finish writes the completion that closes the begun intent. A failure to
// store it marks the response incomplete; the mutation stands.
func (op *auditOp) finish(outcome audit.Outcome, reason audit.Reason, object string) {
	if op.closed || op.intent == nil {
		return
	}
	op.closed = true
	ctx, cancel := auditCtx(op.req.Context())
	defer cancel()
	if _, err := op.a.d.Audit.Finish(ctx, *op.intent, outcome, reason, object); err != nil {
		op.incomplete = true
		op.logFail("completion", op.intent.Seq, err)
	}
}

// deny records a refusal (best effort, flood-limited) when code is one.
func (op *auditOp) deny(code int) {
	var reason audit.Reason
	switch {
	case code == http.StatusUnauthorized:
		reason = audit.ReasonUnauthenticated
	case code == http.StatusForbidden:
		reason = audit.ReasonForbidden
	case code == http.StatusNotFound && op.hidden:
		reason = audit.ReasonHidden
	default:
		return
	}
	op.closed = true
	if op.route.Action == "" {
		return
	}
	a := op.a
	p := a.principalOf(op.req)
	kind, id := a.auditActor(p)
	d := audit.Draft{Event: audit.Event{Tenant: a.requesterTenant(p), ActorKind: kind, Action: op.route.Action, Phase: audit.PhaseSingle,
		Outcome: audit.Denied, Reason: reason, Object: op.denialObject(), Route: op.pattern}, ActorID: id, RequestRef: requestRef(op.req.Context())}
	ctx, cancel := auditCtx(op.req.Context())
	defer cancel()
	if _, err := a.d.Audit.Deny(ctx, d); err != nil {
		op.logFail("denial", 0, err)
	}
}

// denialObject is the object of a denial: the server-confirmed ref when the
// handler resolved one, else a keyed opaque hash of the id in the path (text
// the requester chose).
func (op *auditOp) denialObject() string {
	if op.object != "" {
		return op.object
	}
	kind := ""
	switch {
	case strings.Contains(op.pattern, "/v1/cases/{id}"):
		kind = "case"
	case strings.Contains(op.pattern, "/v1/workspaces/{id}"):
		kind = "workspace"
	case strings.Contains(op.pattern, "/v1/admin/api-keys/{id}"):
		kind = "apikey"
	default:
		return ""
	}
	return kind + ":" + op.a.d.Audit.Pseudonyms().Opaque(kind, op.req.PathValue("id"))
}

// auditActor classifies the caller. Only a verified signed-in user or API key
// has a pseudonym; a bare visitor id (spoofable) is anonymous.
func (a *api) auditActor(p principal) (audit.ActorKind, string) {
	switch {
	case p.Admin:
		return audit.ActorAdmin, ""
	case p.signedIn():
		return audit.ActorUser, p.Email
	case p.HasKey:
		return audit.ActorAPIKey, p.KeyID
	}
	return audit.ActorAnonymous, ""
}

// requesterTenant is where a refusal of this caller is recorded: their
// workspace, else their own personal tenant (verified user or API key), else
// the shared platform tenant. An unverified visitor id can be made up, so it
// never gets a tenant of its own.
func (a *api) requesterTenant(p principal) string {
	var t string
	switch {
	case p.Admin:
		return "platform"
	case p.signedIn() && p.Workspace != "":
		t = "ws:" + p.Workspace
	case p.signedIn():
		t = "own:" + p.UserOwner
	case p.HasKey:
		t = "own:" + p.Owner
	default:
		return "platform"
	}
	if !audit.ValidTenant(t) {
		return "platform"
	}
	return t
}

// tenantOfCase is the tenant a case's events belong to: its workspace, else
// its owner, else the platform.
func tenantOfCase(c store.Case) string {
	t := "platform"
	switch {
	case c.Workspace != "":
		t = "ws:" + c.Workspace
	case c.Owner != "":
		t = "own:" + c.Owner
	}
	if !audit.ValidTenant(t) {
		return "platform"
	}
	return t
}

func workspaceTenant(id string) string {
	if t := "ws:" + id; audit.ValidTenant(t) {
		return t
	}
	return "platform"
}

func objectRef(kind, id string) string {
	ref, _ := audit.ObjectRef(kind, id)
	return ref
}

// auditStep describes the operation about to run.
type auditStep struct {
	Tenant   string
	Object   string
	Detail   string
	Action   audit.Action // the route's when empty
	TargetID string       // a member's email: stored only as a pseudonym
}

func (a *api) auditDraft(op *auditOp, st auditStep, phase audit.Phase) audit.Draft {
	p := a.principalOf(op.req)
	kind, id := a.auditActor(p)
	act := st.Action
	if act == "" {
		act = op.route.Action
	}
	tenant := st.Tenant
	if tenant == "" {
		tenant = "platform"
	}
	return audit.Draft{Event: audit.Event{Tenant: tenant, ActorKind: kind, Action: act, Phase: phase, Outcome: audit.Allowed, Detail: st.Detail,
		Object: st.Object, Route: op.pattern}, ActorID: id, TargetID: st.TargetID, RequestRef: requestRef(op.req.Context())}
}

// auditBegin records the intent of a write the caller has authorized and
// validated, before it changes anything. For a sensitive action it returns
// errAuditUnavailable when the intent cannot be stored and the caller must
// stop and change nothing. For any other action the write carries on and the
// completion is recorded without a reference to an intent. With audit off, or
// off the audited routes, it does nothing.
func (a *api) auditBegin(r *http.Request, st auditStep) error {
	op := auditOpOf(r.Context())
	if op == nil || op.intent != nil {
		return nil
	}
	d := a.auditDraft(op, st, audit.PhaseIntent)
	ctx, cancel := auditCtx(r.Context())
	defer cancel()
	e, err := a.d.Audit.Record(ctx, d)
	if err != nil {
		op.logFail("intent", 0, err)
		if d.Action.Sensitive() {
			return errAuditUnavailable
		}
		// Keep the fields the completion needs; Seq 0 means "no intent stored".
		kind, id := a.auditActor(a.principalOf(r))
		lost := d.Event
		lost.Actor = ""
		if kind == audit.ActorUser || kind == audit.ActorAPIKey {
			lost.Actor = a.d.Audit.Pseudonyms().Actor(kind, id)
		}
		lost.RequestID = d.RequestRef.String() // the one normalised id, never re-read from the header
		e = lost
	}
	op.intent = &e
	return nil
}

// auditCase begins the audit of a write to case c. A pinned public demo that
// the caller cannot change is not audited: nothing private is involved and
// nothing is saved.
func (a *api) auditCase(r *http.Request, c store.Case, detail string) error {
	if a.demoReadOnly(r, c) {
		return nil
	}
	return a.auditBegin(r, auditStep{Tenant: tenantOfCase(c), Object: objectRef("case", c.ID), Detail: detail})
}

// auditSingle writes one allowed event (a read the trail records: an API-key
// listing, an audit read, a preview export). Best effort: a failure marks the
// response incomplete.
func (a *api) auditSingle(r *http.Request, st auditStep) {
	op := auditOpOf(r.Context())
	if op == nil || op.closed {
		return
	}
	op.closed = true
	d := a.auditDraft(op, st, audit.PhaseSingle)
	ctx, cancel := auditCtx(r.Context())
	defer cancel()
	if _, err := a.d.Audit.Record(ctx, d); err != nil {
		op.incomplete = true
		op.logFail("single", 0, err)
	}
}

// auditStreamEnd closes the audit of an event-stream response, before its
// final event is sent. err is the operation's outcome. The stream's headers
// are already out, so a completion that cannot be stored is logged only, and
// the intent stays unresolved.
func (a *api) auditStreamEnd(r *http.Request, err error) {
	op := auditOpOf(r.Context())
	if op == nil {
		return
	}
	if err != nil {
		op.finish(audit.Failed, auditReason(err), "")
		return
	}
	op.finish(audit.Allowed, audit.ReasonNone, "")
}

// auditReason maps an error to the closed reason set, without logging.
func auditReason(err error) audit.Reason {
	var apiErr *llm.APIError
	switch {
	case errors.Is(err, domain.ErrInvalid), errors.Is(err, planner.ErrUnknownFamily), errors.Is(err, apikey.ErrBadRequest):
		return audit.ReasonInvalidInput
	case errors.Is(err, errNeedsVerified):
		return audit.ReasonUnauthenticated
	case errors.Is(err, errForbidden):
		return audit.ReasonForbidden
	case errors.Is(err, errSuperseded):
		return audit.ReasonSuperseded
	case errors.Is(err, store.ErrNotFound), errors.Is(err, cross.ErrNotFound), errors.Is(err, apikey.ErrNotFound):
		return audit.ReasonNotFound
	case errors.Is(err, llm.ErrBadResponse), errors.Is(err, llm.ErrUnavailable), errors.As(err, &apiErr), errors.Is(err, context.DeadlineExceeded):
		return audit.ReasonUpstream
	}
	return audit.ReasonInternal
}

// auditHidden marks the request's 404 as hiding an existing case.
func auditHidden(r *http.Request) {
	if op := auditOpOf(r.Context()); op != nil {
		op.hidden = true
	}
}

// auditConfirm records an object the server resolved (a case the caller may
// see), so a later refusal names it instead of an opaque hash.
func auditConfirm(r *http.Request, kind, id string) {
	if op := auditOpOf(r.Context()); op != nil {
		op.object = objectRef(kind, id)
	}
}

// auditAuthDenied records a refused credential from the auth middlewares,
// which run before any principal exists. Nothing the caller claimed (a token's
// email, a key's id) is recorded: the actor is anonymous and the tenant is the
// shared platform one, through the flood limiter.
func (a *api) auditAuthDenied(r *http.Request) {
	if a.d.Audit == nil {
		return
	}
	d := audit.Draft{Event: audit.Event{Tenant: "platform", ActorKind: audit.ActorAnonymous, Action: audit.AuthDenied, Phase: audit.PhaseSingle,
		Outcome: audit.Denied, Reason: audit.ReasonInvalidCredential}, RequestRef: requestRef(r.Context())}
	ctx, cancel := auditCtx(r.Context())
	defer cancel()
	if _, err := a.d.Audit.Deny(ctx, d); err != nil {
		a.d.Logger.ErrorContext(r.Context(), "audit write failed", "what", "auth denial", "request_id", requestRef(r.Context()), "class", auditErrClass(err))
	}
}
