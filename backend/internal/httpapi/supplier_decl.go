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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/media"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/supplierdecl"
)

// Supplier origin declarations (SUPPLIER_DECLARATIONS, #916, D937). A case
// editor asks a supplier for the origin of some of the case's parts through
// an expiring, revocable link; the supplier opens it without an account,
// sees only those parts and answers per part. It is modelled on the broker
// review link (D200, reviews.go) and keeps its rules:
//
//   - the token is "usd_" + the case id's 12 hex digits + "_" + 32 random
//     bytes (base64url). Only its SHA-256 is stored, inside the case document;
//     the case id in the token only says which document to read;
//   - it travels in the X-Declaration-Token header, never in a path or query,
//     and the page's address carries it in the fragment, so it is in no
//     request log; logs and the audit trail name the route pattern and the
//     case only;
//   - the routes have their own rate class and invalid tokens spend a
//     per-client failure budget; a revoked or lapsed link is 410;
//   - no email is sent from here: the create route returns the link for the
//     requester to send, and the supplier's email, kept for the requester's
//     records, is never logged, never audited and never shown to the supplier.
//
// An answer is stored on the case with who and when, beside the value the
// user typed for the part. It never replaces it: both are shown, and the
// declared origin goes to the predict input as a sourced fact (supplierdecl).

const (
	declarationTokenHeader = "X-Declaration-Token"
	maxDeclarationParts    = supplierdecl.MaxParts
)

var (
	declarationTokenPattern = regexp.MustCompile(`^usd_([0-9a-f]{12})_([A-Za-z0-9_-]{43})$`)
	declarationIDPattern    = regexp.MustCompile(`^sd_[0-9a-f]{16}$`)

	errDeclMissing = errors.New("no such supplier request")
	errDeclRevoked = errors.New("supplier link revoked")
	errDeclExpired = errors.New("supplier link expired")
	errDeclFull    = errors.New("supplier request limit reached")
	errDeclPinned  = errors.New("supplier link closed: the case is a pinned demo")
	errDeclStale   = errors.New("the parts list changed")
)

func newDeclarationToken(caseID string) (string, error) {
	hexID, ok := strings.CutPrefix(caseID, "case_")
	if !ok || len(hexID) != 12 {
		return "", fmt.Errorf("declaration token: unexpected case id shape")
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "usd_" + hexID + "_" + base64.RawURLEncoding.EncodeToString(b), nil
}

// declarationPath is the page's address for tok: the token rides in the URL
// fragment, which browsers never send to a server.
func declarationPath(tok string) string { return "/supplier-declaration#" + tok }

func declarationTokenHash(tok string) string {
	h := sha256.Sum256([]byte("unde-supplier-declaration:" + tok))
	return hex.EncodeToString(h[:])
}

func declarationIDOf(hash string) string { return "sd_" + hash[:16] }

func parseDeclarationToken(tok string) (string, bool) {
	m := declarationTokenPattern.FindStringSubmatch(tok)
	if m == nil {
		return "", false
	}
	return "case_" + m[1], true
}

func indexOfRequest(rqs []store.SupplierRequest, id string) int {
	for i, rq := range rqs {
		if rq.ID == id {
			return i
		}
	}
	return -1
}

// requestStatus is a request's status at now.
func requestStatus(rq store.SupplierRequest, now time.Time) string {
	switch {
	case rq.RevokedAt != nil:
		return "revoked"
	case !now.Before(rq.ExpiresAt):
		return "expired"
	}
	answered := map[int]bool{}
	for _, a := range rq.Answers {
		answered[a.Part] = true
	}
	switch {
	case len(rq.Parts) > 0 && len(answered) >= len(rq.Parts):
		return "answered"
	case len(answered) > 0:
		return "partly_answered"
	case rq.FirstViewedAt != nil:
		return "viewed"
	}
	return "pending"
}

func productPartNames(p *domain.Product) []string {
	if p == nil {
		return nil
	}
	out := make([]string, len(p.Components))
	for i, c := range p.Components {
		out[i] = c.Name
	}
	return out
}

// ---- views ----

// declRequestView is a request as its requester sees it: never the token
// hash.
type declRequestView struct {
	ID            string               `json:"id"`
	Status        string               `json:"status"`
	SupplierName  string               `json:"supplier_name"`
	SupplierEmail string               `json:"supplier_email,omitempty"`
	CreatedAt     time.Time            `json:"created_at"`
	CreatedBy     string               `json:"created_by,omitempty"`
	ExpiresAt     time.Time            `json:"expires_at"`
	RevokedAt     *time.Time           `json:"revoked_at,omitempty"`
	FirstViewedAt *time.Time           `json:"first_viewed_at,omitempty"`
	Parts         []store.SupplierPart `json:"parts"`
	Answered      int                  `json:"answered"`
}

func requestViewOf(rq store.SupplierRequest, now time.Time) declRequestView {
	answered := map[int]bool{}
	for _, a := range rq.Answers {
		answered[a.Part] = true
	}
	return declRequestView{ID: rq.ID, Status: requestStatus(rq, now), SupplierName: rq.SupplierName, SupplierEmail: rq.SupplierEmail,
		CreatedAt: rq.CreatedAt, CreatedBy: rq.CreatedBy, ExpiresAt: rq.ExpiresAt, RevokedAt: rq.RevokedAt, FirstViewedAt: rq.FirstViewedAt,
		Parts: append([]store.SupplierPart{}, rq.Parts...), Answered: len(answered)}
}

// declDocView describes an attached file to the requester: never its
// storage key.
type declDocView struct {
	Name     string `json:"name"`
	MIMEType string `json:"mime_type"`
	Bytes    int    `json:"bytes"`
	Pages    int    `json:"pages,omitempty"`
}

func docViewOf(d *store.SupplierDocument) *declDocView {
	if d == nil {
		return nil
	}
	return &declDocView{Name: d.Name, MIMEType: d.MIMEType, Bytes: d.Bytes, Pages: d.Pages}
}

// declFactView is one part's declaration in effect beside the user's value.
type declFactView struct {
	Part        string       `json:"part"`
	PartIndex   int          `json:"part_index"`
	UserMadeIn  string       `json:"user_made_in"`
	Declared    string       `json:"declared_origin"`
	HTS         string       `json:"hts,omitempty"`
	Differs     bool         `json:"differs"`
	Status      string       `json:"status"`
	Source      string       `json:"source"`
	Supplier    string       `json:"supplier"`
	RequestID   string       `json:"request_id"`
	DeclaredOn  string       `json:"declared_on"`
	ValidUntil  string       `json:"valid_until"`
	DaysLeft    int          `json:"days_left"`
	HowMade     string       `json:"how_made"`
	SignerName  string       `json:"signer_name"`
	SignerTitle string       `json:"signer_title"`
	Document    *declDocView `json:"document,omitempty"`
}

func factViewOf(f supplierdecl.Fact, now time.Time) declFactView {
	return declFactView{Part: f.PartName, PartIndex: f.Part, UserMadeIn: f.UserOrigin, Declared: f.Origin, HTS: f.HTS, Differs: f.Differs, Status: f.Status,
		Source: supplierdecl.Source, Supplier: f.Supplier, RequestID: f.Request, DeclaredOn: f.DeclaredOn(), ValidUntil: f.ValidUntil, DaysLeft: f.DaysLeft(now),
		HowMade: f.HowMade, SignerName: f.SignerName, SignerTitle: f.SignerTitle, Document: docViewOf(f.Document)}
}

// supplierDeclarations is the case's declaration view for its readers: the
// requests (newest first) and the declaration in effect for each part.
func supplierDeclarations(c store.Case, now time.Time) map[string]any {
	reqs := make([]declRequestView, 0, len(c.SupplierRequests))
	for i := len(c.SupplierRequests) - 1; i >= 0; i-- {
		reqs = append(reqs, requestViewOf(c.SupplierRequests[i], now))
	}
	parts := []declFactView{}
	for _, f := range supplierdecl.Facts(c.Product, c.SupplierRequests, now) {
		parts = append(parts, factViewOf(f, now))
	}
	return map[string]any{"requests": reqs, "parts": parts}
}

// withSupplierFacts adds the declared origins that hold to the facts the
// engine reads, beside what the user said (never replacing it).
func (a *api) withSupplierFacts(c store.Case, facts string) string {
	if !a.d.SupplierDeclarations {
		return facts
	}
	return supplierdecl.WithFacts(facts, supplierdecl.Facts(c.Product, c.SupplierRequests, time.Now().UTC()))
}

// ---- requester routes ----

type declRequestBody struct {
	SupplierName  string `json:"supplier_name"`
	SupplierEmail string `json:"supplier_email"`
	Parts         []int  `json:"parts"`
	ExpiresInDays *int   `json:"expires_in_days"`
}

// createSupplierRequest is POST /v1/cases/{id}/supplier-declarations: a new
// link for one supplier and some of the case's parts. The token is returned
// once and never stored; nothing is sent to the supplier.
func (a *api) createSupplierRequest(w http.ResponseWriter, r *http.Request) {
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if c.Pinned { // a public demo is not shared out, even by the admin token
		writeError(w, http.StatusForbidden, "forbidden", "this is a pinned demo case; start a new case to ask a supplier", nil)
		return
	}
	var in declRequestBody
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	name, err := supplierdecl.CleanLine(in.SupplierName, "supplier_name", supplierdecl.MaxName, true)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	email, err := supplierdecl.CleanEmail(in.SupplierEmail)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	days := supplierdecl.DefaultDays
	if in.ExpiresInDays != nil {
		days = *in.ExpiresInDays
	}
	if days < 1 || days > supplierdecl.MaxDays {
		a.fail(w, r, fmt.Errorf("%w: expires_in_days must be 1 to %d", domain.ErrInvalid, supplierdecl.MaxDays))
		return
	}
	names := productPartNames(c.Product)
	if len(names) == 0 {
		a.fail(w, r, fmt.Errorf("%w: the case has no parts list to ask a supplier about", domain.ErrInvalid))
		return
	}
	parts, err := requestedParts(in.Parts, names)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	if supplierdecl.ActiveCount(c.SupplierRequests, now) >= supplierdecl.MaxActive {
		a.declFull(w)
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: tenantOfCase(c), Object: objectRef("case", c.ID)}); err != nil {
		a.fail(w, r, err)
		return
	}
	tok, err := newDeclarationToken(c.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	hash := declarationTokenHash(tok)
	rq := store.SupplierRequest{ID: declarationIDOf(hash), TokenHash: hash, SupplierName: name, SupplierEmail: email, CreatedAt: now,
		CreatedBy: identityOf(r.Context()).email, ExpiresAt: now.Add(time.Duration(days) * 24 * time.Hour), Parts: parts}
	_, err = a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
		if latest.Pinned {
			return errDeclPinned
		}
		if supplierdecl.ActiveCount(latest.SupplierRequests, now) >= supplierdecl.MaxActive {
			return errDeclFull
		}
		// The parts were read from the case as loaded: they must still be
		// the same at this position in the latest.
		latestNames := productPartNames(latest.Product)
		for _, p := range parts {
			if p.Index >= len(latestNames) || latestNames[p.Index] != p.Name {
				return errDeclStale
			}
		}
		rqs := pruneRequests(latest.SupplierRequests, now)
		if len(rqs) >= supplierdecl.MaxRequests {
			return errDeclFull
		}
		latest.SupplierRequests = append(rqs, rq)
		record(r.Context(), latest, store.Event{Kind: "supplier_request_created", Summary: fmt.Sprintf("supplier declaration requested for %d part(s), expires %s", len(parts), rq.ExpiresAt.Format(time.DateOnly))})
		return nil
	})
	switch {
	case errors.Is(err, errDeclFull):
		a.declFull(w)
		return
	case errors.Is(err, errDeclPinned):
		writeError(w, http.StatusForbidden, "forbidden", "this is a pinned demo case; start a new case to ask a supplier", nil)
		return
	case errors.Is(err, errDeclStale):
		writeError(w, http.StatusConflict, "conflict", "the parts list changed while the request was made; reload and ask again", nil)
		return
	case err != nil:
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"request": requestViewOf(rq, now), "token": tok, "declaration_path": declarationPath(tok)})
}

// requestedParts checks the positions and pairs each with its name now.
func requestedParts(idx []int, names []string) ([]store.SupplierPart, error) {
	if len(idx) == 0 || len(idx) > maxDeclarationParts {
		return nil, fmt.Errorf("%w: parts must list 1 to %d part positions", domain.ErrInvalid, maxDeclarationParts)
	}
	seen := map[int]bool{}
	out := make([]store.SupplierPart, 0, len(idx))
	for _, i := range idx {
		switch {
		case i < 0 || i >= len(names):
			return nil, fmt.Errorf("%w: parts: %d is not a position in the case's parts list (0 to %d)", domain.ErrInvalid, i, len(names)-1)
		case seen[i]:
			return nil, fmt.Errorf("%w: parts: position %d is listed twice", domain.ErrInvalid, i)
		}
		seen[i] = true
		out = append(out, store.SupplierPart{Index: i, Name: names[i]})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Index < out[b].Index })
	return out, nil
}

func (a *api) declFull(w http.ResponseWriter) {
	writeError(w, http.StatusConflict, "conflict",
		fmt.Sprintf("this case has the most supplier requests it can hold (%d open, %d kept); revoke one first", supplierdecl.MaxActive, supplierdecl.MaxRequests), nil)
}

// pruneRequests drops the oldest closed requests while the case holds the
// most it keeps. Open requests are never dropped.
func pruneRequests(rqs []store.SupplierRequest, now time.Time) []store.SupplierRequest {
	out := append([]store.SupplierRequest(nil), rqs...)
	for len(out) >= supplierdecl.MaxRequests {
		i := -1
		for j, rq := range out {
			if !supplierdecl.Open(rq, now) {
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

// listSupplierRequests is GET /v1/cases/{id}/supplier-declarations.
func (a *api) listSupplierRequests(w http.ResponseWriter, r *http.Request) {
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out := map[string]any{"case_id": c.ID, "requests": []declRequestView{}, "parts": []declFactView{}}
	if !a.demoReadOnly(r, c) {
		for k, v := range supplierDeclarations(c, time.Now().UTC()) {
			out[k] = v
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// revokeSupplierRequest is DELETE /v1/cases/{id}/supplier-declarations/{request}:
// the link stops working at once (410) and what it declared stops counting.
// The record stays, so the case keeps the history.
func (a *api) revokeSupplierRequest(w http.ResponseWriter, r *http.Request) {
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if a.refuseDemoEdit(w, r, c) {
		return
	}
	id := r.PathValue("request")
	if !declarationIDPattern.MatchString(id) || indexOfRequest(c.SupplierRequests, id) < 0 {
		declNotFound(w)
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: tenantOfCase(c), Object: objectRef("case", c.ID)}); err != nil {
		a.fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	var out store.SupplierRequest
	_, err = a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
		i := indexOfRequest(latest.SupplierRequests, id)
		if i < 0 {
			return errDeclMissing
		}
		rqs := append([]store.SupplierRequest(nil), latest.SupplierRequests...)
		if rqs[i].RevokedAt == nil { // revoking twice keeps the first time
			rqs[i].RevokedAt = &now
			record(r.Context(), latest, store.Event{Kind: "supplier_request_revoked", Summary: "supplier declaration link revoked"})
		}
		latest.SupplierRequests = rqs
		out = rqs[i]
		return nil
	})
	if errors.Is(err, errDeclMissing) {
		declNotFound(w)
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"request": requestViewOf(out, now)})
}

func declNotFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "declaration_not_found", "this case has no supplier request with that id", nil)
}

// supplierDocument is GET /v1/cases/{id}/supplier-declarations/{request}/parts/{part}/document:
// the file the supplier attached to its newest answer for the part. It is
// sent as an attachment of the type its bytes were checked to be.
func (a *api) supplierDocument(w http.ResponseWriter, r *http.Request) {
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if a.demoReadOnly(r, c) { // a demo never carries supplier data
		writeError(w, http.StatusForbidden, "forbidden", "a pinned demo case has no supplier documents", nil)
		return
	}
	id := r.PathValue("request")
	part, perr := strconv.Atoi(r.PathValue("part"))
	i := indexOfRequest(c.SupplierRequests, id)
	if !declarationIDPattern.MatchString(id) || i < 0 || perr != nil {
		declNotFound(w)
		return
	}
	var doc *store.SupplierDocument
	for _, an := range c.SupplierRequests[i].Answers {
		if an.Part == part && an.Document != nil {
			doc = an.Document
		}
	}
	if doc == nil || a.d.SupplierDocs == nil {
		declNotFound(w)
		return
	}
	b, ok, err := a.d.SupplierDocs.Get(r.Context(), supplierDocKey(doc.SHA256))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if !ok {
		declNotFound(w)
		return
	}
	h := w.Header()
	h.Set("Content-Type", doc.MIMEType)
	h.Set("Content-Disposition", `attachment; filename="`+doc.Name+`"`)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Length", strconv.Itoa(len(b)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func supplierDocKey(sha string) string { return "supplier-doc-" + sha }

// expiringDeclarations is GET /v1/supplier-declarations/expiring: the
// declarations on the caller's cases that run out within days (default 30, at
// most a year), soonest first. include_expired=true adds the ones that have
// already lapsed. Like the review queue it lists the caller's own view.
func (a *api) expiringDeclarations(w http.ResponseWriter, r *http.Request) {
	if a.d.StrictAuth && !a.principalOf(r).Verified() {
		a.fail(w, r, errNeedsVerified)
		return
	}
	q := r.URL.Query()
	days := supplierdecl.DefaultExpiringDays
	if v := q.Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > supplierdecl.MaxExpiringDays {
			a.fail(w, r, fmt.Errorf("%w: days must be 1 to %d", domain.ErrInvalid, supplierdecl.MaxExpiringDays))
			return
		}
		days = n
	}
	withExpired := false
	switch q.Get("include_expired") {
	case "", "false":
	case "true":
		withExpired = true
	default:
		a.fail(w, r, fmt.Errorf("%w: include_expired must be true or false", domain.ErrInvalid))
		return
	}
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
	now := time.Now().UTC()
	type item struct {
		CaseID     string `json:"case_id"`
		CaseName   string `json:"case_name"`
		Part       string `json:"part"`
		PartIndex  int    `json:"part_index"`
		Supplier   string `json:"supplier"`
		Origin     string `json:"declared_origin"`
		ValidUntil string `json:"valid_until"`
		DaysLeft   int    `json:"days_left"`
		Status     string `json:"status"`
	}
	items := []item{}
	seen := map[string]bool{}
	for _, c := range cs {
		if seen[c.ID] || IsHealthProbeID(c.ID) || c.Pinned || len(c.SupplierRequests) == 0 || !a.visible(r, c) {
			continue
		}
		seen[c.ID] = true
		for _, f := range supplierdecl.Facts(c.Product, c.SupplierRequests, now) {
			left := f.DaysLeft(now)
			switch {
			case f.Status == supplierdecl.StatusValid && left <= days, f.Status == supplierdecl.StatusExpired && withExpired:
				items = append(items, item{CaseID: c.ID, CaseName: summaryRow(c).DisplayName, Part: f.PartName, PartIndex: f.Part, Supplier: f.Supplier,
					Origin: f.Origin, ValidUntil: f.ValidUntil, DaysLeft: left, Status: f.Status})
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].ValidUntil != items[j].ValidUntil {
			return items[i].ValidUntil < items[j].ValidUntil
		}
		if items[i].CaseID != items[j].CaseID {
			return items[i].CaseID < items[j].CaseID
		}
		return items[i].PartIndex < items[j].PartIndex
	})
	writeJSON(w, http.StatusOK, map[string]any{"days": days, "as_of": now.Format(time.DateOnly), "items": items})
}

// ---- supplier routes (the token is the credential) ----

// declByToken resolves the request's X-Declaration-Token to its case and
// request, or writes the refusal, exactly as reviewByToken does for a broker:
// a token that is malformed, names no case or matches no request is a 404
// that counts against the client's failure budget and is recorded as an
// anonymous auth denial; a real request that was revoked or has lapsed is a
// 410.
func (a *api) declByToken(w http.ResponseWriter, r *http.Request) (store.Case, store.SupplierRequest, bool) {
	client := a.clientKey(r)
	release, ok, wait := a.declFails.begin(client)
	if !ok {
		secs := max(1, int(math.Ceil(wait.Seconds())))
		w.Header().Set("Retry-After", fmt.Sprint(secs))
		writeError(w, http.StatusTooManyRequests, "auth_rate_limited",
			fmt.Sprintf("too many invalid links from this client; try again in %d s", secs), map[string]int{"retry_after_s": secs})
		return store.Case{}, store.SupplierRequest{}, false
	}
	defer release()
	bad := func() (store.Case, store.SupplierRequest, bool) {
		a.declFails.fail(client)
		a.auditAuthDenied(r)
		writeError(w, http.StatusNotFound, "not_found", "this link is not valid", nil)
		return store.Case{}, store.SupplierRequest{}, false
	}
	tok := strings.TrimSpace(r.Header.Get(declarationTokenHeader))
	caseID, ok := parseDeclarationToken(tok)
	if !ok {
		return bad()
	}
	c, err := a.d.Cases.Get(r.Context(), caseID)
	if errors.Is(err, store.ErrNotFound) {
		return bad()
	}
	if err != nil {
		a.fail(w, r, err)
		return store.Case{}, store.SupplierRequest{}, false
	}
	hash := declarationTokenHash(tok)
	i := indexOfRequest(c.SupplierRequests, declarationIDOf(hash))
	if i < 0 || subtle.ConstantTimeCompare([]byte(c.SupplierRequests[i].TokenHash), []byte(hash)) != 1 {
		return bad()
	}
	rq := c.SupplierRequests[i]
	if c.Pinned {
		a.auditAuthDenied(r)
		a.declGone(w, errDeclPinned)
		return store.Case{}, store.SupplierRequest{}, false
	}
	if err := declOpen(rq, time.Now().UTC()); err != nil {
		a.auditAuthDenied(r)
		a.declGone(w, err)
		return store.Case{}, store.SupplierRequest{}, false
	}
	return c, rq, true
}

func declOpen(rq store.SupplierRequest, now time.Time) error {
	switch {
	case rq.RevokedAt != nil:
		return errDeclRevoked
	case !now.Before(rq.ExpiresAt):
		return errDeclExpired
	}
	return nil
}

func (a *api) declGone(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errDeclPinned):
		writeError(w, http.StatusGone, "link_revoked", "this case is a pinned demo; its links are closed", nil)
	case errors.Is(err, errDeclRevoked):
		writeError(w, http.StatusGone, "link_revoked", "the requester revoked this link", nil)
	default:
		writeError(w, http.StatusGone, "link_expired", "this link has expired; ask the requester for a new one", nil)
	}
}

// supplierPartView is one requested part as the supplier sees it: its name
// and the supplier's own newest answer for it. Nothing the case says about
// the part (the origin the user typed, its code, its plant) is shown.
type supplierPartView struct {
	Index  int             `json:"index"`
	Name   string          `json:"name"`
	Answer *supplierAnswer `json:"answer,omitempty"`
}

type supplierAnswer struct {
	Origin      string       `json:"origin"`
	HTS         string       `json:"hts,omitempty"`
	HowMade     string       `json:"how_made"`
	Document    *declDocView `json:"document,omitempty"`
	SignerName  string       `json:"signer_name"`
	SignerTitle string       `json:"signer_title"`
	ValidUntil  string       `json:"valid_until"`
	AnsweredAt  time.Time    `json:"answered_at"`
}

func supplierPartsOf(rq store.SupplierRequest) []supplierPartView {
	out := make([]supplierPartView, 0, len(rq.Parts))
	for _, p := range rq.Parts {
		pv := supplierPartView{Index: p.Index, Name: p.Name}
		for _, an := range rq.Answers {
			if an.Part == p.Index {
				pv.Answer = &supplierAnswer{Origin: an.Origin, HTS: an.HTS, HowMade: an.HowMade, Document: docViewOf(an.Document), SignerName: an.SignerName,
					SignerTitle: an.SignerTitle, ValidUntil: an.ValidUntil, AnsweredAt: an.AnsweredAt}
			}
		}
		out = append(out, pv)
	}
	return out
}

func supplierViewOf(rq store.SupplierRequest, now time.Time) map[string]any {
	return map[string]any{
		"request": map[string]any{"id": rq.ID, "status": requestStatus(rq, now), "supplier_name": rq.SupplierName, "created_at": rq.CreatedAt, "expires_at": rq.ExpiresAt},
		"parts":   supplierPartsOf(rq),
	}
}

// getSupplierRequest is GET /v1/supplier-declaration: the parts asked about,
// and the supplier's own answers so far. No case id, no product, no other
// part, no email.
func (a *api) getSupplierRequest(w http.ResponseWriter, r *http.Request) {
	reviewHeaders(w)
	c, rq, ok := a.declByToken(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC()
	if rq.FirstViewedAt == nil {
		if updated, err := a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
			i := indexOfRequest(latest.SupplierRequests, rq.ID)
			if i < 0 {
				return errDeclMissing
			}
			rqs := append([]store.SupplierRequest(nil), latest.SupplierRequests...)
			if rqs[i].FirstViewedAt == nil {
				rqs[i].FirstViewedAt = &now
			}
			latest.SupplierRequests = rqs
			return nil
		}); err == nil {
			rq = updated.SupplierRequests[indexOfRequest(updated.SupplierRequests, rq.ID)]
		}
		// A failed first-view stamp does not stop the supplier reading.
	}
	a.auditSingle(r, auditStep{Tenant: tenantOfCase(c), Object: objectRef("case", c.ID)})
	writeJSON(w, http.StatusOK, supplierViewOf(rq, now))
}

type declDocumentBody struct {
	Name          string `json:"name"`
	ContentBase64 string `json:"content_base64"`
}

type declAnswerBody struct {
	Origin      string            `json:"origin"`
	HTS         string            `json:"hts"`
	HowMade     string            `json:"how_made"`
	SignerName  string            `json:"signer_name"`
	SignerTitle string            `json:"signer_title"`
	ValidUntil  string            `json:"valid_until"`
	Document    *declDocumentBody `json:"document"`
}

var (
	errDeclDocTooLarge = errors.New("document too large")
	errDeclDocType     = errors.New("document type")
)

var unsafeFileChars = regexp.MustCompile(`[^A-Za-z0-9._() -]+`)

// safeFileName is a file name fit for a header and a list: no path, no
// quotes, no control characters, at most 80 characters.
func safeFileName(s string) string {
	if i := strings.LastIndexAny(s, `/\`); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimLeft(strings.TrimSpace(unsafeFileChars.ReplaceAllString(s, "")), ".")
	if len(s) > 80 {
		s = s[:80]
	}
	if s == "" {
		return "document"
	}
	return s
}

// checkDeclDocument decodes and checks an attached file by its bytes, as an
// upload is (media.Check): the type is sniffed, never read from the name.
func checkDeclDocument(d *declDocumentBody) ([]byte, store.SupplierDocument, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(d.ContentBase64))
	if err != nil || len(b) == 0 {
		return nil, store.SupplierDocument{}, fmt.Errorf("%w: document.content_base64 must be the file's bytes in base64", domain.ErrInvalid)
	}
	if len(b) > supplierdecl.MaxDocBytes {
		return nil, store.SupplierDocument{}, fmt.Errorf("%w: the document is larger than %d KB", errDeclDocTooLarge, supplierdecl.MaxDocBytes>>10)
	}
	info, err := media.Check(b)
	switch {
	case errors.Is(err, media.ErrUnsupported):
		return nil, store.SupplierDocument{}, fmt.Errorf("%w: %v", errDeclDocType, err)
	case err != nil:
		return nil, store.SupplierDocument{}, fmt.Errorf("%w: document: %v", domain.ErrInvalid, err)
	case info.Pages > media.MaxPDFPages:
		return nil, store.SupplierDocument{}, fmt.Errorf("%w: the PDF has %d pages; the limit is %d", domain.ErrInvalid, info.Pages, media.MaxPDFPages)
	}
	sum := sha256.Sum256(b)
	return b, store.SupplierDocument{Name: safeFileName(d.Name), MIMEType: info.MIMEType, Bytes: info.Bytes, Pages: info.Pages, SHA256: hex.EncodeToString(sum[:])}, nil
}

// putSupplierAnswer is PUT /v1/supplier-declaration/parts/{part}: the
// supplier's declaration for one part. A later answer for the same part
// replaces it in effect; the earlier ones stay as history.
func (a *api) putSupplierAnswer(w http.ResponseWriter, r *http.Request) {
	reviewHeaders(w)
	c, rq, ok := a.declByToken(w, r)
	if !ok {
		return
	}
	part, perr := strconv.Atoi(r.PathValue("part"))
	asked := false
	for _, p := range rq.Parts {
		asked = asked || (perr == nil && p.Index == part)
	}
	if !asked {
		writeError(w, http.StatusNotFound, "part_not_found", "this link does not ask about that part", nil)
		return
	}
	var in declAnswerBody
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	ans, err := supplierdecl.Clean(supplierdecl.Answer{Origin: in.Origin, HTS: in.HTS, HowMade: in.HowMade, SignerName: in.SignerName, SignerTitle: in.SignerTitle, ValidUntil: in.ValidUntil}, now)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var docBytes []byte
	if in.Document != nil {
		var doc store.SupplierDocument
		docBytes, doc, err = checkDeclDocument(in.Document)
		switch {
		case errors.Is(err, errDeclDocTooLarge):
			writeError(w, http.StatusRequestEntityTooLarge, "invalid_input", err.Error(), nil)
			return
		case errors.Is(err, errDeclDocType):
			writeError(w, http.StatusUnsupportedMediaType, "invalid_input", err.Error(), nil)
			return
		case err != nil:
			a.fail(w, r, err)
			return
		}
		if a.d.SupplierDocs == nil {
			writeError(w, http.StatusServiceUnavailable, "internal", "document storage is not configured; send the answer without a document", nil)
			return
		}
		ans.Document = &doc
	}
	if len(rq.Answers) >= supplierdecl.MaxAnswers {
		writeError(w, http.StatusConflict, "conflict", "this link has its most answers; ask the requester for a new link", nil)
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: tenantOfCase(c), Object: objectRef("case", c.ID)}); err != nil {
		a.fail(w, r, err)
		return
	}
	if docBytes != nil {
		if err := a.d.SupplierDocs.Put(r.Context(), supplierDocKey(ans.Document.SHA256), docBytes); err != nil {
			a.fail(w, r, err)
			return
		}
	}
	ans.Part, ans.AnsweredAt = part, now
	var out store.SupplierRequest
	_, err = a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
		if latest.Pinned {
			return errDeclPinned
		}
		i := indexOfRequest(latest.SupplierRequests, rq.ID)
		if i < 0 {
			return errDeclMissing
		}
		if err := declOpen(latest.SupplierRequests[i], now); err != nil { // revoked or lapsed meanwhile
			return err
		}
		if len(latest.SupplierRequests[i].Answers) >= supplierdecl.MaxAnswers {
			return errDeclFull
		}
		names := productPartNames(latest.Product)
		var asked store.SupplierPart
		for _, p := range latest.SupplierRequests[i].Parts {
			if p.Index == part {
				asked = p
			}
		}
		if part >= len(names) || names[part] != asked.Name { // the parts list changed under the link
			return errDeclStale
		}
		rqs := append([]store.SupplierRequest(nil), latest.SupplierRequests...)
		cur := rqs[i]
		cur.Answers = append(append([]store.SupplierAnswer(nil), cur.Answers...), withPartName(ans, asked.Name))
		if cur.FirstViewedAt == nil {
			cur.FirstViewedAt = &now
		}
		rqs[i] = cur
		latest.SupplierRequests = rqs
		latest.Record(store.Event{Kind: "supplier_answered", Summary: "supplier declared the origin of " + asked.Name, At: now})
		out = cur
		return nil
	})
	switch {
	case errors.Is(err, errDeclRevoked), errors.Is(err, errDeclExpired), errors.Is(err, errDeclPinned):
		a.declGone(w, err)
		return
	case errors.Is(err, errDeclMissing):
		writeError(w, http.StatusNotFound, "not_found", "this link is not valid", nil)
		return
	case errors.Is(err, errDeclFull):
		writeError(w, http.StatusConflict, "conflict", "this link has its most answers; ask the requester for a new link", nil)
		return
	case errors.Is(err, errDeclStale):
		writeError(w, http.StatusConflict, "conflict", "the parts list changed since this link was made; ask the requester for a new link", nil)
		return
	case err != nil:
		a.fail(w, r, err)
		return
	}
	var pv supplierPartView
	for _, p := range supplierPartsOf(out) {
		if p.Index == part {
			pv = p
		}
	}
	v := supplierViewOf(out, now)
	v["part"] = pv
	delete(v, "parts")
	writeJSON(w, http.StatusOK, v)
}

func withPartName(a store.SupplierAnswer, name string) store.SupplierAnswer {
	a.PartName = name
	return a
}
