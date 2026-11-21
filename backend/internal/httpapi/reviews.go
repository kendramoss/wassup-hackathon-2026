package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// Broker review links (D200). The case owner makes a link for one draft (the
// current setup or a plan); a customs broker opens it without an account,
// reads a frozen snapshot of the draft and answers approve, request changes
// or file as a binding ruling request. See docs/api/SECURITY.md.
//
// The token is "ubr_" + the case id's 12 hex digits + "_" + 32 random bytes
// (base64url): 256 bits of secret. Only its SHA-256 is stored, inside the case
// document, next to the case it opens, so a lookup is one read of that case by
// id and a constant-time comparison of the hash: no new collection, no query,
// no index. The case id in the token only says which document to read; it
// grants nothing on its own.
//
// The token travels in the X-Review-Token header, never in an API path or
// query, and the review page's address carries it in the fragment
// (/review#<token>), never in its path or query, so it is in no request log;
// the logs and the audit trail name the route pattern and the case only.

const (
	reviewTokenHeader = "X-Review-Token"
	// reviewDefaultDays and reviewMaxDays bound a link's life.
	reviewDefaultDays = 14
	reviewMaxDays     = 30
	// maxActiveReviews caps the open links per case; maxStoredReviews caps
	// the links kept on the case (the oldest closed ones are dropped first).
	maxActiveReviews = 5
	maxStoredReviews = 20
	// maxReviewDecisions caps the broker's answers on one link.
	maxReviewDecisions = 20
	// MaxReviewComment caps a broker comment, in characters.
	MaxReviewComment = 2000
)

// Review statuses, as the owner and the broker see them.
const (
	reviewPending   = "pending"
	reviewViewed    = "viewed"
	reviewApproved  = "approved"
	reviewChanges   = "changes_requested"
	reviewFileReq   = "file_requested"
	reviewRevoked   = "revoked"
	reviewExpired   = "expired"
	decisionApprove = "approve"
	decisionChanges = "changes"
	decisionFile    = "file_request"
)

var (
	reviewTokenPattern = regexp.MustCompile(`^ubr_([0-9a-f]{12})_([A-Za-z0-9_-]{43})$`)
	reviewIDPattern    = regexp.MustCompile(`^rv_[0-9a-f]{16}$`)

	errReviewRevoked = errors.New("review link revoked")
	errReviewExpired = errors.New("review link expired")
	errReviewMissing = errors.New("no such review link")
	errReviewFull    = errors.New("review link limit reached")
)

// newReviewToken mints a token for caseID ("case_<12 hex>").
func newReviewToken(caseID string) (string, error) {
	hexID, ok := strings.CutPrefix(caseID, "case_")
	if !ok || len(hexID) != 12 {
		return "", fmt.Errorf("review token: unexpected case id shape")
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "ubr_" + hexID + "_" + base64.RawURLEncoding.EncodeToString(b), nil
}

// reviewPath is the review page's address for tok: the token rides in the
// URL fragment, which browsers never send to a server, so it cannot reach a
// request log (the frontend's, Cloud Run's or a proxy's) or a Referer
// header. The page reads it client-side and drops it from the address bar.
func reviewPath(tok string) string { return "/review#" + tok }

// reviewTokenHash is the stored form of a token.
func reviewTokenHash(tok string) string {
	h := sha256.Sum256([]byte("unde-broker-review:" + tok))
	return hex.EncodeToString(h[:])
}

// reviewIDOf is the public id of the link a hash belongs to.
func reviewIDOf(hash string) string { return "rv_" + hash[:16] }

// parseReviewToken returns the case id a well-formed token names.
func parseReviewToken(tok string) (string, bool) {
	m := reviewTokenPattern.FindStringSubmatch(tok)
	if m == nil {
		return "", false
	}
	return "case_" + m[1], true
}

// reviewStatus is a link's status at now.
func reviewStatus(rv store.BrokerReview, now time.Time) string {
	switch {
	case rv.RevokedAt != nil:
		return reviewRevoked
	case !now.Before(rv.ExpiresAt):
		return reviewExpired
	case len(rv.Decisions) > 0:
		switch rv.Decisions[len(rv.Decisions)-1].Decision {
		case decisionApprove:
			return reviewApproved
		case decisionChanges:
			return reviewChanges
		case decisionFile:
			return reviewFileReq
		}
	case rv.FirstViewedAt != nil:
		return reviewViewed
	}
	return reviewPending
}

func reviewActive(rv store.BrokerReview, now time.Time) bool {
	return rv.RevokedAt == nil && now.Before(rv.ExpiresAt)
}

// reviewSummary is what the owner (and, without the view time, the broker)
// sees of a link: never the token hash, never the draft text.
type reviewSummary struct {
	ID            string                 `json:"id"`
	Status        string                 `json:"status"`
	Subject       string                 `json:"subject"`
	DraftLabel    string                 `json:"draft_label"`
	Format        string                 `json:"format"`
	CreatedAt     time.Time              `json:"created_at"`
	ExpiresAt     time.Time              `json:"expires_at"`
	RevokedAt     *time.Time             `json:"revoked_at,omitempty"`
	FirstViewedAt *time.Time             `json:"first_viewed_at,omitempty"`
	Decisions     []store.ReviewDecision `json:"decisions"`
}

func summarizeReview(rv store.BrokerReview, now time.Time) reviewSummary {
	d := rv.Decisions
	if d == nil {
		d = []store.ReviewDecision{}
	}
	return reviewSummary{ID: rv.ID, Status: reviewStatus(rv, now), Subject: rv.Draft.Subject, DraftLabel: rv.Draft.Label, Format: rv.Draft.Format,
		CreatedAt: rv.CreatedAt, ExpiresAt: rv.ExpiresAt, RevokedAt: rv.RevokedAt, FirstViewedAt: rv.FirstViewedAt, Decisions: d}
}

// reviewsField is the case's links for the owner, newest first.
func reviewsField(rvs []store.BrokerReview, now time.Time) []reviewSummary {
	out := make([]reviewSummary, 0, len(rvs))
	for i := len(rvs) - 1; i >= 0; i-- {
		out = append(out, summarizeReview(rvs[i], now))
	}
	return out
}

// ---- owner routes ----

type createReviewRequest struct {
	PlanIndex     *int   `json:"plan_index"`
	MoveKey       string `json:"move_key"`
	Target        string `json:"target"`
	Format        string `json:"format"`
	ExpiresInDays *int   `json:"expires_in_days"`
}

// createReview is POST /v1/cases/{id}/reviews: a new link for one draft.
// The token is returned once and never stored.
func (a *api) createReview(w http.ResponseWriter, r *http.Request) {
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if c.Pinned { // a public demo is not shared out, even by the admin token
		writeError(w, http.StatusForbidden, "forbidden", "this is a pinned demo case; start a new case to send a draft for review", nil)
		return
	}
	var in createReviewRequest
	if r.ContentLength != 0 {
		if err := decode(w, r, &in); err != nil {
			a.fail(w, r, err)
			return
		}
	}
	days := reviewDefaultDays
	if in.ExpiresInDays != nil {
		days = *in.ExpiresInDays
	}
	if days < 1 || days > reviewMaxDays {
		a.fail(w, r, fmt.Errorf("%w: expires_in_days must be 1 to %d", domain.ErrInvalid, reviewMaxDays))
		return
	}
	if c.Prediction.Origin == "" {
		a.fail(w, r, fmt.Errorf("%w: predict the origin before sending a draft for review", domain.ErrInvalid))
		return
	}
	d, err := a.buildDraft(r, c, exportRequest{PlanIndex: in.PlanIndex, MoveKey: in.MoveKey, Target: in.Target, Format: in.Format})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	if activeReviews(c.BrokerReviews, now) >= maxActiveReviews {
		a.reviewFull(w)
		return
	}
	draft := a.reviewDraft(c, d)
	if err := a.auditBegin(r, auditStep{Tenant: tenantOfCase(c), Object: objectRef("case", c.ID), Detail: draft.Subject}); err != nil {
		a.fail(w, r, err)
		return
	}
	tok, err := newReviewToken(c.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	hash := reviewTokenHash(tok)
	rv := store.BrokerReview{ID: reviewIDOf(hash), TokenHash: hash, CreatedAt: now, ExpiresAt: now.Add(time.Duration(days) * 24 * time.Hour), Draft: draft}
	_, err = a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
		if activeReviews(latest.BrokerReviews, now) >= maxActiveReviews {
			return errReviewFull
		}
		latest.BrokerReviews = pruneReviews(latest.BrokerReviews, now)
		if len(latest.BrokerReviews) >= maxStoredReviews {
			return errReviewFull
		}
		latest.BrokerReviews = append(latest.BrokerReviews, rv)
		record(r.Context(), latest, store.Event{Kind: "review_link_created", Summary: fmt.Sprintf("broker review link for %s, expires %s", draft.Label, rv.ExpiresAt.Format(time.DateOnly))})
		return nil
	})
	if errors.Is(err, errReviewFull) {
		a.reviewFull(w)
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"review": summarizeReview(rv, now), "token": tok, "review_path": reviewPath(tok)})
}

func (a *api) reviewFull(w http.ResponseWriter) {
	writeError(w, http.StatusConflict, "conflict",
		fmt.Sprintf("this case has the most broker review links it can hold (%d open, %d kept); revoke one first", maxActiveReviews, maxStoredReviews), nil)
}

func activeReviews(rvs []store.BrokerReview, now time.Time) int {
	n := 0
	for _, rv := range rvs {
		if reviewActive(rv, now) {
			n++
		}
	}
	return n
}

// pruneReviews drops the oldest closed links while the case holds the most
// it keeps. Open links are never dropped.
func pruneReviews(rvs []store.BrokerReview, now time.Time) []store.BrokerReview {
	out := append([]store.BrokerReview(nil), rvs...)
	for len(out) >= maxStoredReviews {
		i := -1
		for j, rv := range out {
			if !reviewActive(rv, now) {
				i = j
				break
			}
		}
		if i < 0 {
			break
		}
		out = append(out[:i], out[i+1:]...)
	}
	return out
}

// reviewDraft is the frozen snapshot the broker will read.
func (a *api) reviewDraft(c store.Case, d draftResult) store.ReviewDraft {
	out := store.ReviewDraft{Subject: "setup", Label: "Current setup", Format: d.format, ProductName: d.ex.ProductName, Product: d.ex.Product,
		Prediction: a.fillCalibration(c.Prediction), Content: d.content, Notes: d.notes}
	if out.ProductName == "" {
		out.ProductName = firstWords(c.Facts, 8)
	}
	if p := d.chosen; p != nil {
		out.Subject, out.Label = "plan", d.ex.PlanSummary
		rp := &store.ReviewPlan{Description: d.ex.PlanSummary, PredictedOrigin: p.Prediction.Origin, BelowBar: d.ex.BelowBar != nil}
		if p.Verdict != nil {
			conf := p.Verdict.Confidence
			rp.Supporting, rp.Counter, rp.CheckConfidence = p.Verdict.SupportingRuling, p.Verdict.CounterRuling, &conf
		}
		out.Plan = rp
	}
	// The prediction the broker weighs is the one behind the requested
	// position: the plan's when a plan is drafted.
	if d.chosen != nil {
		out.Prediction = a.fillCalibration(d.chosen.Prediction)
	}
	// Model traces and the reasoning text stay in Unde.
	out.Prediction.Trace, out.Prediction.Reasoning = domain.Trace{}, ""
	return out
}

// listReviews is GET /v1/cases/{id}/reviews.
func (a *api) listReviews(w http.ResponseWriter, r *http.Request) {
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"case_id": c.ID, "reviews": a.ownerReviews(r, c)})
}

// revokeReview is DELETE /v1/cases/{id}/reviews/{review}: the link stops
// working at once (410). The record stays so the owner keeps the history.
func (a *api) revokeReview(w http.ResponseWriter, r *http.Request) {
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if a.refuseDemoEdit(w, r, c) {
		return
	}
	id := r.PathValue("review")
	if !reviewIDPattern.MatchString(id) || indexOfReview(c.BrokerReviews, id) < 0 {
		writeError(w, http.StatusNotFound, "review_not_found", "this case has no broker review link with that id", nil)
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: tenantOfCase(c), Object: objectRef("case", c.ID)}); err != nil {
		a.fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	var out store.BrokerReview
	_, err = a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
		i := indexOfReview(latest.BrokerReviews, id)
		if i < 0 {
			return errReviewMissing
		}
		rv := &latest.BrokerReviews[i]
		if rv.RevokedAt == nil { // revoking twice keeps the first time
			rv.RevokedAt = &now
			record(r.Context(), latest, store.Event{Kind: "review_link_revoked", Summary: "broker review link for " + rv.Draft.Label + " revoked"})
		}
		out = *rv
		return nil
	})
	if errors.Is(err, errReviewMissing) {
		writeError(w, http.StatusNotFound, "review_not_found", "this case has no broker review link with that id", nil)
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"review": summarizeReview(out, now)})
}

func indexOfReview(rvs []store.BrokerReview, id string) int {
	for i, rv := range rvs {
		if rv.ID == id {
			return i
		}
	}
	return -1
}

// ---- broker routes (the token is the credential) ----

// reviewByToken resolves the request's X-Review-Token to its case and link,
// or writes the refusal. A token that is malformed, names no case, or matches
// no link is a 404 that counts against the client's failure budget (like an
// invalid API key) and is recorded as an anonymous auth denial. A real link
// that was revoked or has expired is a 410.
func (a *api) reviewByToken(w http.ResponseWriter, r *http.Request) (store.Case, store.BrokerReview, bool) {
	client := clientKey(r)
	if ok, wait := a.reviewFails.allowed(client); !ok {
		secs := max(1, int(math.Ceil(wait.Seconds())))
		w.Header().Set("Retry-After", fmt.Sprint(secs))
		writeError(w, http.StatusTooManyRequests, "auth_rate_limited",
			fmt.Sprintf("too many invalid review links from this client; try again in %d s", secs), map[string]int{"retry_after_s": secs})
		return store.Case{}, store.BrokerReview{}, false
	}
	bad := func() (store.Case, store.BrokerReview, bool) {
		a.reviewFails.fail(client)
		a.auditAuthDenied(r)
		writeError(w, http.StatusNotFound, "not_found", "this review link is not valid", nil)
		return store.Case{}, store.BrokerReview{}, false
	}
	tok := strings.TrimSpace(r.Header.Get(reviewTokenHeader))
	caseID, ok := parseReviewToken(tok)
	if !ok {
		return bad()
	}
	c, err := a.d.Cases.Get(r.Context(), caseID)
	if errors.Is(err, store.ErrNotFound) {
		return bad()
	}
	if err != nil {
		a.fail(w, r, err)
		return store.Case{}, store.BrokerReview{}, false
	}
	hash := reviewTokenHash(tok)
	i := indexOfReview(c.BrokerReviews, reviewIDOf(hash))
	if i < 0 || subtle.ConstantTimeCompare([]byte(c.BrokerReviews[i].TokenHash), []byte(hash)) != 1 {
		return bad()
	}
	rv := c.BrokerReviews[i]
	if err := reviewOpen(rv, time.Now().UTC()); err != nil {
		a.auditAuthDenied(r)
		a.reviewGone(w, err)
		return store.Case{}, store.BrokerReview{}, false
	}
	return c, rv, true
}

func reviewOpen(rv store.BrokerReview, now time.Time) error {
	switch {
	case rv.RevokedAt != nil:
		return errReviewRevoked
	case !now.Before(rv.ExpiresAt):
		return errReviewExpired
	}
	return nil
}

func (a *api) reviewGone(w http.ResponseWriter, err error) {
	if errors.Is(err, errReviewRevoked) {
		writeError(w, http.StatusGone, "link_revoked", "the case owner revoked this review link", nil)
		return
	}
	writeError(w, http.StatusGone, "link_expired", "this review link has expired; ask the case owner for a new one", nil)
}

// reviewHeaders mark a broker answer as never to be indexed or cached.
func reviewHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("Cache-Control", "no-store")
}

// brokerView is what GET /v1/review and a decision return to the broker.
type brokerView struct {
	ID         string                 `json:"id"`
	Status     string                 `json:"status"`
	Subject    string                 `json:"subject"`
	DraftLabel string                 `json:"draft_label"`
	CreatedAt  time.Time              `json:"created_at"`
	ExpiresAt  time.Time              `json:"expires_at"`
	Decisions  []store.ReviewDecision `json:"decisions"`
}

func brokerReviewOf(rv store.BrokerReview, now time.Time) brokerView {
	s := summarizeReview(rv, now)
	return brokerView{ID: s.ID, Status: s.Status, Subject: s.Subject, DraftLabel: s.DraftLabel, CreatedAt: s.CreatedAt, ExpiresAt: s.ExpiresAt, Decisions: s.Decisions}
}

// getReview is GET /v1/review: the read-only packet for the broker. Nothing
// but the link's own snapshot is returned: no case id, no other link, no
// owner, no history.
func (a *api) getReview(w http.ResponseWriter, r *http.Request) {
	reviewHeaders(w)
	c, rv, ok := a.reviewByToken(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC()
	if rv.FirstViewedAt == nil {
		if updated, err := a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
			i := indexOfReview(latest.BrokerReviews, rv.ID)
			if i < 0 {
				return errReviewMissing
			}
			if latest.BrokerReviews[i].FirstViewedAt == nil {
				latest.BrokerReviews[i].FirstViewedAt = &now
			}
			return nil
		}); err == nil {
			rv = updated.BrokerReviews[indexOfReview(updated.BrokerReviews, rv.ID)]
		}
		// A failed first-view stamp does not stop the broker reading.
	}
	a.auditSingle(r, auditStep{Tenant: tenantOfCase(c), Object: objectRef("case", c.ID)})
	writeJSON(w, http.StatusOK, a.brokerPacket(rv, now))
}

func (a *api) brokerPacket(rv store.BrokerReview, now time.Time) map[string]any {
	d := rv.Draft
	notes := d.Notes
	if notes == nil {
		notes = []string{}
	}
	cited := d.Prediction.CitedRulings
	if d.Plan != nil && d.Plan.Supporting != "" && !contains(cited, d.Plan.Supporting) {
		cited = append(append([]string(nil), cited...), d.Plan.Supporting)
	}
	return map[string]any{
		"review":       brokerReviewOf(rv, now),
		"product_name": d.ProductName,
		"product":      d.Product,
		"prediction": map[string]any{
			"origin_country": d.Prediction.Origin, "confidence": d.Prediction.Confidence, "calibrated_confidence": d.Prediction.CalibratedConfidence,
			"decisive_step": d.Prediction.DecisiveStep, "key_factors": nonNil(d.Prediction.KeyFactors),
		},
		"cited_rulings": a.precedentRefs(cited),
		"plan":          d.Plan,
		"draft":         map[string]any{"format": d.Format, "content": d.Content, "notes": notes},
		"shared_at":     rv.CreatedAt,
		"disclaimer":    domain.Disclaimer,
	}
}

func contains(xs []string, x string) bool {
	for _, s := range xs {
		if s == x {
			return true
		}
	}
	return false
}

func nonNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

type decisionRequest struct {
	Decision string `json:"decision"`
	Comment  string `json:"comment"`
}

// cleanComment is a broker comment as stored: plain text, CRLF as LF, no
// control characters but newline and tab, trimmed, at most MaxReviewComment
// characters. It is never interpreted; clients render it as text.
func cleanComment(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", fmt.Errorf("%w: comment is not valid UTF-8", domain.ErrInvalid)
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if n := utf8.RuneCountInString(s); n > MaxReviewComment {
		return "", fmt.Errorf("%w: comment is %d characters; the most is %d", domain.ErrInvalid, n, MaxReviewComment)
	}
	return s, nil
}

// decideReview is POST /v1/review/decision: the broker's answer. It changes
// only this link's record on the case, never the case's analysis.
func (a *api) decideReview(w http.ResponseWriter, r *http.Request) {
	reviewHeaders(w)
	c, rv, ok := a.reviewByToken(w, r)
	if !ok {
		return
	}
	var in decisionRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	switch in.Decision {
	case decisionApprove, decisionChanges, decisionFile:
	default:
		a.fail(w, r, fmt.Errorf("%w: decision must be approve, changes or file_request", domain.ErrInvalid))
		return
	}
	comment, err := cleanComment(in.Comment)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if in.Decision == decisionChanges && comment == "" {
		a.fail(w, r, fmt.Errorf("%w: say what should change in the comment", domain.ErrInvalid))
		return
	}
	if len(rv.Decisions) >= maxReviewDecisions {
		writeError(w, http.StatusConflict, "conflict", fmt.Sprintf("this link has its most answers (%d); ask the case owner for a new link", maxReviewDecisions), nil)
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: tenantOfCase(c), Object: objectRef("case", c.ID), Detail: in.Decision}); err != nil {
		a.fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	var out store.BrokerReview
	_, err = a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
		i := indexOfReview(latest.BrokerReviews, rv.ID)
		if i < 0 {
			return errReviewMissing
		}
		cur := &latest.BrokerReviews[i]
		if err := reviewOpen(*cur, now); err != nil { // revoked or expired meanwhile
			return err
		}
		if len(cur.Decisions) >= maxReviewDecisions {
			return errReviewFull
		}
		if cur.FirstViewedAt == nil {
			cur.FirstViewedAt = &now
		}
		cur.Decisions = append(cur.Decisions, store.ReviewDecision{Decision: in.Decision, Comment: comment, At: now})
		latest.Record(store.Event{Kind: "broker_decision", Summary: "broker answered " + decisionWords(in.Decision) + " on " + cur.Draft.Label, At: now})
		out = *cur
		return nil
	})
	switch {
	case errors.Is(err, errReviewRevoked), errors.Is(err, errReviewExpired):
		a.reviewGone(w, err)
		return
	case errors.Is(err, errReviewMissing):
		writeError(w, http.StatusNotFound, "not_found", "this review link is not valid", nil)
		return
	case errors.Is(err, errReviewFull):
		writeError(w, http.StatusConflict, "conflict", fmt.Sprintf("this link has its most answers (%d); ask the case owner for a new link", maxReviewDecisions), nil)
		return
	case err != nil:
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"review": brokerReviewOf(out, now)})
}

func decisionWords(d string) string {
	switch d {
	case decisionApprove:
		return "approve"
	case decisionChanges:
		return "request changes"
	}
	return "file as a binding ruling request"
}

// ownerReviews is the case's links as its reader sees them: none on a pinned
// demo for anyone but the admin token (no link can be made there, D200).
func (a *api) ownerReviews(r *http.Request, c store.Case) []reviewSummary {
	if a.demoReadOnly(r, c) {
		return []reviewSummary{}
	}
	return reviewsField(c.BrokerReviews, time.Now().UTC())
}
