package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/brokerflow"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/export"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// Broker workflow (BROKER_WORKFLOW; docs/design/broker-workflow.md): a human
// review queue (#881), a discussion with sign-offs and per-case roles
// (#882), and a CBP eRulings-ready filing package with status tracking
// (#879). Everything is stored inside the case document. Nothing is filed
// with CBP from here.

const (
	maxComments      = 500
	maxApprovals     = 100
	maxCollaborators = 20
	maxAttachments   = 20
	maxLetterBytes   = 64 << 10
)

// brokerErr turns a brokerflow error into the API's classes: invalid is 400,
// a refused transition is 409.
func brokerErr(err error) error {
	switch {
	case errors.Is(err, brokerflow.ErrInvalid):
		return fmt.Errorf("%w: %s", domain.ErrInvalid, strings.TrimPrefix(err.Error(), "brokerflow: invalid: "))
	case errors.Is(err, brokerflow.ErrConflict):
		return fmt.Errorf("%w: %s", errBrokerConflict, strings.TrimPrefix(err.Error(), "brokerflow: conflict: "))
	}
	return err
}

var (
	errBrokerConflict = errors.New("conflict")
	sha256Hex         = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
)

// actorOf names who acted, as stored on the case: the signed-in email, else
// the credential.
func actorOf(p principal) string {
	switch {
	case p.Email != "":
		return p.Email
	case p.Admin:
		return "admin"
	case p.HasKey:
		return "apikey:" + p.KeyID
	case p.Owner != "":
		return "visitor"
	}
	return "anonymous"
}

// caseRoleOf is the principal's role on a case for a comment or a sign-off:
// their per-case collaborator role if they have one, else their workspace
// role, else owner.
func caseRoleOf(p principal, c store.Case) string {
	if p.Email != "" {
		for _, cl := range c.Collaborators {
			if cl.Email == p.Email {
				return cl.Role
			}
		}
	}
	if p.roleGoverns(c) {
		if p.Role == "" {
			return string(teams.RoleViewer)
		}
		return string(p.Role)
	}
	if p.Admin {
		return "admin"
	}
	return "owner"
}

func newBrokerID(prefix string) (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b), nil
}

// ---- review queue (#881) ----

type enqueueRequest struct {
	Priority string `json:"priority"`
	Reason   string `json:"reason"`
}

type queueView struct {
	CaseID   string           `json:"case_id"`
	Product  string           `json:"product,omitempty"`
	Route    domain.Route     `json:"route,omitempty"`
	Item     *store.QueueItem `json:"item"`
	Overdue  bool             `json:"overdue"`
	DueInSec int64            `json:"due_in_s"`
}

func queueViewOf(c store.Case, now time.Time) queueView {
	v := queueView{CaseID: c.ID, Route: c.Triage.Route, Item: c.Queue}
	if c.Product != nil {
		v.Product = c.Product.Name
	}
	if c.Queue != nil {
		v.Overdue = brokerflow.Overdue(c.Queue, now)
		if c.Queue.State != brokerflow.StateDecided {
			v.DueInSec = int64(c.Queue.DueAt.Sub(now).Seconds())
		}
	}
	return v
}

// enqueue is PUT /v1/cases/{id}/queue: put the case in the review queue, or
// change its priority and reason while it waits.
func (a *api) enqueue(w http.ResponseWriter, r *http.Request) {
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if c.Pinned {
		a.fail(w, r, fmt.Errorf("%w: a pinned demo case cannot be queued", domain.ErrInvalid))
		return
	}
	var in enqueueRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	p := a.principalOf(r)
	now := time.Now().UTC()
	if _, err := brokerflow.Enqueue(c.Queue, in.Priority, in.Reason, actorOf(p), now); err != nil {
		a.fail(w, r, brokerErr(err))
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: tenantOfCase(c), Object: objectRef("case", c.ID), Detail: "enqueue"}); err != nil {
		a.fail(w, r, err)
		return
	}
	updated, err := a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
		q, err := brokerflow.Enqueue(latest.Queue, in.Priority, in.Reason, actorOf(p), now)
		if err != nil {
			return err
		}
		latest.Queue, latest.UpdatedAt = q, now
		record(r.Context(), latest, store.Event{Kind: "queued", Summary: "queued for review, " + q.Priority + " priority"})
		return nil
	})
	if err != nil {
		a.fail(w, r, brokerErr(err))
		return
	}
	writeJSON(w, http.StatusOK, queueViewOf(updated, now))
}

type transitionRequest struct {
	Action   string `json:"action"`
	Assignee string `json:"assignee"`
	Outcome  string `json:"outcome"`
	Reason   string `json:"reason"`
}

// queueTransition is POST /v1/cases/{id}/queue/transition: assign, start,
// decide (with a required reason) or release a queued review. In a
// workspace only an admin assigns someone else.
func (a *api) queueTransition(w http.ResponseWriter, r *http.Request) {
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in transitionRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	p := a.principalOf(r)
	me := actorOf(p)
	t := brokerflow.Transition{Action: in.Action, Assignee: in.Assignee, Outcome: in.Outcome, Reason: in.Reason, By: me}
	if in.Action == "assign" && p.roleGoverns(c) && p.Role != teams.RoleAdmin && !strings.EqualFold(strings.TrimSpace(in.Assignee), me) {
		a.fail(w, r, errForbidden)
		return
	}
	now := time.Now().UTC()
	if _, err := brokerflow.Apply(c.Queue, t, now); err != nil {
		a.fail(w, r, brokerErr(err))
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: tenantOfCase(c), Object: objectRef("case", c.ID), Detail: in.Action}); err != nil {
		a.fail(w, r, err)
		return
	}
	updated, err := a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
		q, err := brokerflow.Apply(latest.Queue, t, now)
		if err != nil {
			return err
		}
		latest.Queue, latest.UpdatedAt = q, now
		summary := "review " + in.Action
		if in.Action == "decide" {
			summary = "review decided: " + q.Decision.Outcome
		}
		record(r.Context(), latest, store.Event{Kind: "review_" + in.Action, Summary: summary})
		return nil
	})
	if err != nil {
		a.fail(w, r, brokerErr(err))
		return
	}
	writeJSON(w, http.StatusOK, queueViewOf(updated, now))
}

// listQueue is GET /v1/queue: the review queue of the cases the caller can
// see. state is open (default: not decided), queued, assigned, in_review,
// decided or all; assignee=me keeps the caller's own. Overdue items come
// first, then by due time. suggested lists cases the model sent to review
// that are not queued yet.
func (a *api) listQueue(w http.ResponseWriter, r *http.Request) {
	if a.d.StrictAuth && !a.principalOf(r).Verified() {
		a.fail(w, r, errNeedsVerified)
		return
	}
	state := r.URL.Query().Get("state")
	switch state {
	case "":
		state = "open"
	case "open", "all", brokerflow.StateQueued, brokerflow.StateAssigned, brokerflow.StateInReview, brokerflow.StateDecided:
	default:
		a.fail(w, r, fmt.Errorf("%w: state must be open, all, queued, assigned, in_review or decided", domain.ErrInvalid))
		return
	}
	mine := r.URL.Query().Get("assignee") == "me"
	limit := a.d.CasesListLimit
	if limit <= 0 {
		limit = 200
	}
	cs, err := a.candidateCases(r, limit)
	if err == nil {
		cs, err = a.workspaceCases(r.Context(), cs)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	p := a.principalOf(r)
	me := actorOf(p)
	now := time.Now().UTC()
	items, suggested := []queueView{}, []queueView{}
	for _, c := range cs {
		if IsHealthProbeID(c.ID) || c.Pinned || !a.visible(r, c) {
			continue
		}
		if c.Queue == nil {
			if c.Triage.Route == domain.RouteNeedsReview && len(suggested) < 50 {
				suggested = append(suggested, queueViewOf(c, now))
			}
			continue
		}
		q := c.Queue
		if state == "open" && q.State == brokerflow.StateDecided || state != "open" && state != "all" && q.State != state {
			continue
		}
		if mine && q.Assignee != me {
			continue
		}
		items = append(items, queueViewOf(c, now))
	}
	sort.SliceStable(items, func(i, j int) bool { return brokerflow.Less(items[i].Item, items[j].Item, now) })
	overdue := 0
	for _, it := range items {
		if it.Overdue {
			overdue++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "overdue": overdue, "suggested": suggested, "review_targets_h": reviewTargetsHours()})
}

func reviewTargetsHours() map[string]int {
	out := map[string]int{}
	for _, p := range brokerflow.Priorities() {
		d, _ := brokerflow.ReviewTarget(p)
		out[p] = int(d.Hours())
	}
	return out
}

// ---- collaboration (#882) ----

type collabView struct {
	Collaborators []store.Collaborator `json:"collaborators"`
	Threads       []thread             `json:"threads"`
	Approvals     []store.Approval     `json:"approvals"`
	SignOff       signOff              `json:"sign_off"`
}

type thread struct {
	store.Comment
	Replies []store.Comment `json:"replies"`
}

// signOff is where the case's approvals stand: the latest decision of each
// person counts, and a broker approval (in the case or by review link) is
// what a filing needs.
type signOff struct {
	State          string   `json:"state"` // none | approved | changes_requested | rejected
	BrokerApproved bool     `json:"broker_approved"`
	Approvers      []string `json:"approvers"`
	Blocking       []string `json:"blocking"`
}

func threadsOf(cs []store.Comment) []thread {
	var out []thread
	at := map[string]int{}
	for _, c := range cs {
		if c.ParentID == "" {
			at[c.ID] = len(out)
			out = append(out, thread{Comment: c, Replies: []store.Comment{}})
		}
	}
	for _, c := range cs {
		if c.ParentID != "" {
			if i, ok := at[c.ParentID]; ok {
				out[i].Replies = append(out[i].Replies, c)
			}
		}
	}
	if out == nil {
		out = []thread{}
	}
	return out
}

func signOffOf(c store.Case) signOff {
	latest := map[string]store.Approval{}
	var order []string
	for _, ap := range c.Approvals {
		if _, ok := latest[ap.By]; !ok {
			order = append(order, ap.By)
		}
		latest[ap.By] = ap
	}
	s := signOff{State: "none", Approvers: []string{}, Blocking: []string{}}
	for _, who := range order {
		ap := latest[who]
		switch ap.Decision {
		case "approve":
			s.Approvers = append(s.Approvers, who)
			if ap.Role == "broker" {
				s.BrokerApproved = true
			}
		default:
			s.Blocking = append(s.Blocking, who+": "+ap.Decision)
		}
	}
	// A broker's approval through a review link counts too (D200).
	for _, rv := range c.BrokerReviews {
		if n := len(rv.Decisions); n > 0 {
			switch rv.Decisions[n-1].Decision {
			case decisionApprove, decisionFile:
				s.BrokerApproved = true
				s.Approvers = append(s.Approvers, "broker via review link "+rv.ID)
			case decisionChanges:
				s.Blocking = append(s.Blocking, "broker via review link "+rv.ID+": changes")
			}
		}
	}
	switch {
	case len(s.Blocking) > 0 && slices.ContainsFunc(s.Blocking, func(b string) bool { return strings.HasSuffix(b, ": reject") }):
		s.State = "rejected"
	case len(s.Blocking) > 0:
		s.State = "changes_requested"
	case len(s.Approvers) > 0:
		s.State = "approved"
	}
	return s
}

func collabViewOf(c store.Case) collabView {
	cl := c.Collaborators
	if cl == nil {
		cl = []store.Collaborator{}
	}
	ap := c.Approvals
	if ap == nil {
		ap = []store.Approval{}
	}
	return collabView{Collaborators: cl, Threads: threadsOf(c.Comments), Approvals: ap, SignOff: signOffOf(c)}
}

// getCollab is GET /v1/cases/{id}/collab.
func (a *api) getCollab(w http.ResponseWriter, r *http.Request) {
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, collabViewOf(c))
}

type commentRequest struct {
	Body     string `json:"body"`
	ParentID string `json:"parent_id"`
}

// appendComment adds a comment to latest, checking the thread and the cap.
func appendComment(latest *store.Case, cm store.Comment) error {
	if len(latest.Comments) >= maxComments {
		return fmt.Errorf("%w: this case has the most comments it can hold (%d)", errBrokerConflict, maxComments)
	}
	if cm.ParentID != "" {
		i := slices.IndexFunc(latest.Comments, func(x store.Comment) bool { return x.ID == cm.ParentID })
		if i < 0 {
			return fmt.Errorf("%w: parent_id names no comment on this case", domain.ErrInvalid)
		}
		if p := latest.Comments[i].ParentID; p != "" {
			cm.ParentID = p // a reply to a reply joins the thread
		}
	}
	latest.Comments = append(slices.Clone(latest.Comments), cm)
	return nil
}

// addComment is POST /v1/cases/{id}/comments: anyone who can see the case
// may take part in its discussion, viewers included.
func (a *api) addComment(w http.ResponseWriter, r *http.Request) {
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if c.Pinned {
		a.fail(w, r, fmt.Errorf("%w: comments are not kept on a pinned demo case", domain.ErrInvalid))
		return
	}
	var in commentRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	body, err := brokerflow.CleanText(in.Body, "body", true)
	if err != nil {
		a.fail(w, r, brokerErr(err))
		return
	}
	p := a.principalOf(r)
	id, err := newBrokerID("cm_")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	cm := store.Comment{ID: id, ParentID: strings.TrimSpace(in.ParentID), Author: actorOf(p), Role: caseRoleOf(p, c), Body: body, At: now}
	if err := a.auditBegin(r, auditStep{Tenant: tenantOfCase(c), Object: objectRef("case", c.ID)}); err != nil {
		a.fail(w, r, err)
		return
	}
	updated, err := a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
		if err := appendComment(latest, cm); err != nil {
			return err
		}
		latest.UpdatedAt = now
		record(r.Context(), latest, store.Event{Kind: "commented", Summary: "comment by " + cm.Role})
		return nil
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, collabViewOf(updated))
}

type approvalRequest struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// addApproval is POST /v1/cases/{id}/approvals: a sign-off on the ruling
// request (approve, changes or reject; a reason is required unless it is an
// approval). The role recorded is the person's case role.
func (a *api) addApproval(w http.ResponseWriter, r *http.Request) {
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if c.Pinned {
		a.fail(w, r, fmt.Errorf("%w: a pinned demo case cannot be signed off", domain.ErrInvalid))
		return
	}
	var in approvalRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if !brokerflow.ValidOutcome(in.Decision) {
		a.fail(w, r, fmt.Errorf("%w: decision must be approve, changes or reject", domain.ErrInvalid))
		return
	}
	reason, err := brokerflow.CleanText(in.Reason, "reason", in.Decision != "approve")
	if err != nil {
		a.fail(w, r, brokerErr(err))
		return
	}
	p := a.principalOf(r)
	now := time.Now().UTC()
	ap := store.Approval{Decision: in.Decision, Reason: reason, By: actorOf(p), Role: caseRoleOf(p, c), At: now}
	if err := a.auditBegin(r, auditStep{Tenant: tenantOfCase(c), Object: objectRef("case", c.ID), Detail: in.Decision}); err != nil {
		a.fail(w, r, err)
		return
	}
	updated, err := a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
		if len(latest.Approvals) >= maxApprovals {
			return fmt.Errorf("%w: this case has the most sign-offs it can hold (%d)", errBrokerConflict, maxApprovals)
		}
		ap.Role = caseRoleOf(p, *latest)
		latest.Approvals = append(slices.Clone(latest.Approvals), ap)
		latest.UpdatedAt = now
		record(r.Context(), latest, store.Event{Kind: "signed_off", Summary: ap.Role + " " + decisionWordsOf(in.Decision)})
		return nil
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, collabViewOf(updated))
}

func decisionWordsOf(d string) string {
	switch d {
	case "approve":
		return "approved"
	case "reject":
		return "rejected"
	}
	return "asked for changes"
}

type collaboratorRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"` // broker | reviewer | "" to remove
}

// putCollaborator is PUT /v1/cases/{id}/collaborators: give a person a role
// on this case (broker or reviewer), or remove it with an empty role. A role
// names how their comments and sign-offs count; it never lets someone see a
// case they otherwise could not.
func (a *api) putCollaborator(w http.ResponseWriter, r *http.Request) {
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if c.Pinned {
		a.fail(w, r, fmt.Errorf("%w: a pinned demo case has no collaborators", domain.ErrInvalid))
		return
	}
	var in collaboratorRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" || !strings.Contains(email, "@") || len(email) > 254 || strings.ContainsAny(email, " \t\n") {
		a.fail(w, r, fmt.Errorf("%w: email must be an email address", domain.ErrInvalid))
		return
	}
	role := strings.ToLower(strings.TrimSpace(in.Role))
	detail := role
	switch role {
	case "broker", "reviewer":
	case "":
		detail = "remove"
	default:
		a.fail(w, r, fmt.Errorf("%w: role must be broker, reviewer or empty to remove", domain.ErrInvalid))
		return
	}
	p := a.principalOf(r)
	now := time.Now().UTC()
	if err := a.auditBegin(r, auditStep{Tenant: tenantOfCase(c), Object: objectRef("case", c.ID), Detail: detail, TargetID: email}); err != nil {
		a.fail(w, r, err)
		return
	}
	updated, err := a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
		cl := slices.DeleteFunc(slices.Clone(latest.Collaborators), func(x store.Collaborator) bool { return x.Email == email })
		if role != "" {
			if len(cl) >= maxCollaborators {
				return fmt.Errorf("%w: this case has the most collaborators it can hold (%d)", errBrokerConflict, maxCollaborators)
			}
			cl = append(cl, store.Collaborator{Email: email, Role: role, AddedBy: actorOf(p), AddedAt: now})
		}
		latest.Collaborators, latest.UpdatedAt = cl, now
		record(r.Context(), latest, store.Event{Kind: "collaborator", Summary: "case role " + detail})
		return nil
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, collabViewOf(updated))
}

// reviewComments is GET /v1/review/comments: the discussion as the broker
// holding a review link sees it. Like the link's packet, nothing names the
// case or the people beyond their roles.
func (a *api) reviewComments(w http.ResponseWriter, r *http.Request) {
	reviewHeaders(w)
	c, rv, ok := a.reviewByToken(w, r)
	if !ok {
		return
	}
	if err := reviewOpen(rv, time.Now().UTC()); err != nil {
		a.reviewGone(w, err)
		return
	}
	a.auditSingle(r, auditStep{Tenant: tenantOfCase(c), Object: objectRef("case", c.ID)})
	writeJSON(w, http.StatusOK, map[string]any{"threads": brokerThreads(c.Comments)})
}

// brokerThreads drops author identities: an external broker sees roles.
func brokerThreads(cs []store.Comment) []thread {
	anon := make([]store.Comment, len(cs))
	for i, c := range cs {
		c.Author = c.Role
		anon[i] = c
	}
	return threadsOf(anon)
}

// postReviewComment is POST /v1/review/comments: the broker holding a review
// link takes part in the case's discussion.
func (a *api) postReviewComment(w http.ResponseWriter, r *http.Request) {
	reviewHeaders(w)
	c, rv, ok := a.reviewByToken(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC()
	if err := reviewOpen(rv, now); err != nil {
		a.reviewGone(w, err)
		return
	}
	var in commentRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	body, err := brokerflow.CleanText(in.Body, "body", true)
	if err != nil {
		a.fail(w, r, brokerErr(err))
		return
	}
	id, err := newBrokerID("cm_")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	cm := store.Comment{ID: id, ParentID: strings.TrimSpace(in.ParentID), Author: "external broker (" + rv.ID + ")", Role: "external_broker", Body: body, At: now}
	if err := a.auditBegin(r, auditStep{Tenant: tenantOfCase(c), Object: objectRef("case", c.ID), Detail: "external_broker"}); err != nil {
		a.fail(w, r, err)
		return
	}
	updated, err := a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
		if latest.Pinned {
			return errReviewPinned
		}
		i := indexOfReview(latest.BrokerReviews, rv.ID)
		if i < 0 {
			return errReviewMissing
		}
		if err := reviewOpen(latest.BrokerReviews[i], now); err != nil {
			return err
		}
		if err := appendComment(latest, cm); err != nil {
			return err
		}
		latest.Record(store.Event{Kind: "commented", Summary: "comment by the broker on " + latest.BrokerReviews[i].Draft.Label, At: now})
		return nil
	})
	switch {
	case errors.Is(err, errReviewRevoked), errors.Is(err, errReviewExpired), errors.Is(err, errReviewPinned):
		a.reviewGone(w, err)
		return
	case errors.Is(err, errReviewMissing):
		writeError(w, http.StatusNotFound, "not_found", "this review link is not valid", nil)
		return
	case err != nil:
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"threads": brokerThreads(updated.Comments)})
}

// ---- filing package (#879) ----

type filingRequest struct {
	Submitter store.FilingSubmitter `json:"submitter"`
	// Letter, when given, is the broker's edited letter; otherwise the
	// 19 CFR 177.2 letter is generated from the case and filled in.
	Letter      string             `json:"letter"`
	Attachments []store.FilingFile `json:"attachments"`
	// The plan the letter argues for, as for export.
	PlanIndex *int   `json:"plan_index"`
	PlanID    string `json:"plan_id"`
	MoveKey   string `json:"move_key"`
	Target    string `json:"target"`
}

type filingView struct {
	*store.Filing
	// ReadyToFile needs every required element and a broker's approval.
	ReadyToFile  bool     `json:"ready_to_file"`
	SignOff      signOff  `json:"sign_off"`
	NextStatuses []string `json:"next_statuses"`
	Blocking     []string `json:"blocking"`
}

func filingViewOf(c store.Case) filingView {
	so := signOffOf(c)
	v := filingView{Filing: c.Filing, SignOff: so, NextStatuses: []string{}, Blocking: []string{}}
	if c.Filing == nil {
		v.Blocking = append(v.Blocking, "no filing package prepared")
		return v
	}
	v.NextStatuses = brokerflow.NextStatuses(c.Filing.Status)
	for _, e := range c.Filing.Elements {
		if e.Required && e.Status != "present" {
			v.Blocking = append(v.Blocking, e.Label+" ("+e.Citation+")")
		}
	}
	if !so.BrokerApproved {
		v.Blocking = append(v.Blocking, "no broker approval")
	}
	if so.State == "changes_requested" || so.State == "rejected" {
		v.Blocking = append(v.Blocking, "a sign-off asks for changes or rejects the request")
	}
	v.ReadyToFile = len(v.Blocking) == 0
	return v
}

var attachmentName = func(s string) bool {
	if s == "" || len(s) > 120 || strings.ContainsAny(s, "/\\\x00") || strings.HasPrefix(s, ".") {
		return false
	}
	return true
}

// prepareFiling is POST /v1/cases/{id}/filing: build the eRulings package
// from the case's 19 CFR 177.2 letter (or the broker's edit of it) and the
// submitter's answers, check the required elements and store it. A package
// can be prepared again until it is submitted.
func (a *api) prepareFiling(w http.ResponseWriter, r *http.Request) {
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.requireMarket(c, ""); err != nil {
		a.fail(w, r, err)
		return
	}
	if c.Pinned {
		a.fail(w, r, fmt.Errorf("%w: a pinned demo case cannot be filed; start a new case", domain.ErrInvalid))
		return
	}
	if c.Prediction.Origin == "" {
		a.fail(w, r, fmt.Errorf("%w: predict the origin before preparing a filing", domain.ErrInvalid))
		return
	}
	if c.Filing != nil && c.Filing.Status != brokerflow.FilingPrepared && c.Filing.Status != brokerflow.FilingWithdrawn {
		a.fail(w, r, fmt.Errorf("%w: the request is already %s; withdraw it before preparing another", errBrokerConflict, c.Filing.Status))
		return
	}
	var in filingRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	sub, err := brokerflow.CleanSubmitter(in.Submitter)
	if err != nil {
		a.fail(w, r, brokerErr(err))
		return
	}
	if len(in.Attachments) > maxAttachments {
		a.fail(w, r, fmt.Errorf("%w: at most %d attachments", domain.ErrInvalid, maxAttachments))
		return
	}
	var atts []store.FilingFile
	for _, f := range in.Attachments {
		desc, err := brokerflow.CleanText(f.Description, "attachment description", false)
		if err != nil || !attachmentName(f.Name) || (f.SHA256 != "" && !sha256Hex.MatchString(f.SHA256)) || f.Bytes < 0 {
			a.fail(w, r, fmt.Errorf("%w: each attachment needs a plain file name (no path), and sha256 as 64 hex characters if given", domain.ErrInvalid))
			return
		}
		atts = append(atts, store.FilingFile{Name: f.Name, Kind: "to_attach", Description: desc, SHA256: strings.ToLower(f.SHA256), Bytes: f.Bytes})
	}
	letter := in.Letter
	if strings.TrimSpace(letter) == "" {
		d, err := a.buildDraft(r, c, exportRequest{PlanIndex: in.PlanIndex, PlanID: in.PlanID, MoveKey: in.MoveKey, Target: in.Target})
		if err != nil {
			a.fail(w, r, err)
			return
		}
		letter = export.CFR177(d.ex)
	} else if len(letter) > maxLetterBytes || strings.ContainsRune(letter, 0) {
		a.fail(w, r, fmt.Errorf("%w: letter must be text under %d KB", domain.ErrInvalid, maxLetterBytes>>10))
		return
	}
	letter = brokerflow.Fill(letter, sub)
	p := a.principalOf(r)
	now := time.Now().UTC()
	f := &store.Filing{Status: brokerflow.FilingPrepared, PreparedAt: now, PreparedBy: actorOf(p), Submitter: sub, Letter: letter, Attachments: atts}
	f.Elements = brokerflow.Check(letter)
	f.Ready = brokerflow.Ready(f.Elements)
	f.History = []store.FilingStep{{Status: brokerflow.FilingPrepared, By: f.PreparedBy, At: now}}
	_, files, err := brokerflow.Build(c.ID, f, c.Approvals, c.Product)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	f.PackageSHA256 = brokerflow.Hash(files)
	if err := a.auditBegin(r, auditStep{Tenant: tenantOfCase(c), Object: objectRef("case", c.ID)}); err != nil {
		a.fail(w, r, err)
		return
	}
	updated, err := a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
		if latest.Filing != nil && latest.Filing.Status != brokerflow.FilingPrepared && latest.Filing.Status != brokerflow.FilingWithdrawn {
			return fmt.Errorf("%w: the request is already %s", errBrokerConflict, latest.Filing.Status)
		}
		latest.Filing, latest.UpdatedAt = f, now
		ready := "not ready"
		if f.Ready {
			ready = "all required elements present"
		}
		record(r.Context(), latest, store.Event{Kind: "filing_prepared", Summary: "eRulings filing package prepared, " + ready})
		return nil
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, filingViewOf(updated))
}

// getFiling is GET /v1/cases/{id}/filing: the package's status (no format),
// or the package as json, zip (letter PDF and Markdown, exhibits, package
// JSON) or pdf (the letter).
func (a *api) getFiling(w http.ResponseWriter, r *http.Request) {
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	format := r.URL.Query().Get("format")
	if format == "" {
		writeJSON(w, http.StatusOK, filingViewOf(c))
		return
	}
	if format != "json" && format != "zip" && format != "pdf" {
		a.fail(w, r, fmt.Errorf("%w: format must be json, zip or pdf", domain.ErrInvalid))
		return
	}
	if c.Filing == nil {
		a.fail(w, r, fmt.Errorf("%w: no filing package is prepared for this case", store.ErrNotFound))
		return
	}
	pkg, files, err := brokerflow.Build(c.ID, c.Filing, c.Approvals, c.Product)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.auditSingle(r, auditStep{Tenant: tenantOfCase(c), Object: objectRef("case", c.ID), Detail: format, Action: audit.FilingDownload})
	name := "ruling-request-" + c.ID
	switch format {
	case "json":
		v := filingViewOf(c)
		pkg.Ready = v.ReadyToFile
		writeJSON(w, http.StatusOK, map[string]any{"package": pkg, "ready_to_file": v.ReadyToFile, "blocking": v.Blocking})
	case "pdf":
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.pdf"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(files["letter.pdf"])
	case "zip":
		z, err := brokerflow.Zip(files, c.Filing.PreparedAt)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.zip"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(z)
	}
}

type filingStatusRequest struct {
	Status    string `json:"status"`
	Reference string `json:"reference"`
	Note      string `json:"note"`
}

// filingStatus is POST /v1/cases/{id}/filing/status: record what happened
// after the package left (submitted with the eRulings reference,
// acknowledged, ruling_issued with the ruling number, withdrawn). Moving to
// submitted needs a package ready to file.
func (a *api) filingStatus(w http.ResponseWriter, r *http.Request) {
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in filingStatusRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if c.Filing == nil {
		a.fail(w, r, fmt.Errorf("%w: prepare a filing package first", errBrokerConflict))
		return
	}
	ref, err := brokerflow.CleanText(in.Reference, "reference", in.Status == brokerflow.FilingSubmitted || in.Status == brokerflow.FilingRuling)
	if err == nil && len(ref) > 64 {
		err = fmt.Errorf("%w: reference is longer than 64 characters", brokerflow.ErrInvalid)
	}
	if err != nil {
		a.fail(w, r, brokerErr(err))
		return
	}
	note, err := brokerflow.CleanText(in.Note, "note", false)
	if err != nil {
		a.fail(w, r, brokerErr(err))
		return
	}
	check := func(cur store.Case) error {
		if !brokerflow.CanMove(cur.Filing.Status, in.Status) {
			return fmt.Errorf("%w: a %s request cannot become %q (next: %s)", errBrokerConflict, cur.Filing.Status, in.Status, strings.Join(brokerflow.NextStatuses(cur.Filing.Status), ", "))
		}
		if in.Status == brokerflow.FilingSubmitted {
			if v := filingViewOf(cur); !v.ReadyToFile {
				return fmt.Errorf("%w: the package is not ready to file: %s", errBrokerConflict, strings.Join(v.Blocking, "; "))
			}
		}
		return nil
	}
	if err := check(c); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: tenantOfCase(c), Object: objectRef("case", c.ID), Detail: in.Status}); err != nil {
		a.fail(w, r, err)
		return
	}
	p := a.principalOf(r)
	now := time.Now().UTC()
	updated, err := a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
		if latest.Filing == nil {
			return fmt.Errorf("%w: the filing package is gone", errBrokerConflict)
		}
		if err := check(*latest); err != nil {
			return err
		}
		f := *latest.Filing
		f.History = append(slices.Clone(f.History), store.FilingStep{Status: in.Status, By: actorOf(p), At: now, Reference: ref, Note: note})
		f.Status = in.Status
		switch in.Status {
		case brokerflow.FilingSubmitted:
			f.CBPReference = ref
		case brokerflow.FilingRuling:
			f.RulingNumber = ref
		}
		latest.Filing, latest.UpdatedAt = &f, now
		record(r.Context(), latest, store.Event{Kind: "filing_" + in.Status, Summary: "ruling request " + strings.ReplaceAll(in.Status, "_", " ")})
		return nil
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, filingViewOf(updated))
}
