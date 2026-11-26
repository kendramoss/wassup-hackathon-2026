// Package audit is the tenant-scoped, append-only audit stream (#284).
//
// It records WHO (a keyed pseudonym of a verified identity, never an email,
// token or key), did WHAT (a closed action enum), to WHICH opaque object, WHEN,
// with what OUTCOME, under which request ID. Events of one tenant form a hash
// chain with a per-tenant monotonic sequence, so reordering, editing, deleting
// and truncating are detectable by Verify.
//
// What this is not: the chain, its head and its retention checkpoint are
// written by the application into the same store. Anyone who can write that
// store (a cloud administrator included) can rewrite all of them consistently.
// It is tamper EVIDENCE at the application level, not WORM or immutable
// storage, and it is one input to a broader SOC 2 readiness program, not a
// certification or a compliance claim.
//
// Every string an event carries is checked against a closed enum or a strict
// pattern before it is stored, so free text (emails, names, facts, error
// messages, request bodies) cannot reach the trail.
package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Action is what happened. The set is closed.
type Action string

const (
	CaseCreate Action = "case.create"
	CaseUpdate Action = "case.update"
	CaseExport Action = "case.export"
	CaseDelete Action = "case.delete"
	CasePin    Action = "case.pin"
	// CaseRead appears only as a denial: allowed reads are not audited.
	CaseRead Action = "case.read"

	WorkspaceCreate       Action = "workspace.create"
	WorkspaceInvite       Action = "workspace.invite"
	WorkspaceJoin         Action = "workspace.join"
	WorkspaceRoleChange   Action = "workspace.role_change"
	WorkspaceMemberRemove Action = "workspace.member_remove"
	WorkspaceLeave        Action = "workspace.leave"
	// WorkspaceRetention sets a workspace's case retention policy (#875).
	WorkspaceRetention Action = "workspace.retention"

	APIKeyIssue   Action = "apikey.issue"
	APIKeyList    Action = "apikey.list"
	APIKeyDisable Action = "apikey.disable"

	AdminWatchRun Action = "admin.watch_run"
	AuditRead     Action = "audit.read"
	AuditVerify   Action = "audit.verify"
	AuditExport   Action = "audit.export" // #871
	AuthDenied    Action = "auth.denied"

	// Broker review links (D200). The object is always the case.
	ReviewCreate   Action = "review.create"
	ReviewRevoke   Action = "review.revoke"
	ReviewView     Action = "review.view"
	ReviewDecision Action = "review.decision"
)

type actionSpec struct {
	// details are the allowed values of Event.Detail; nil means detail must be empty.
	details []string
	// sensitive actions fail closed: the intent must be stored before the
	// operation runs.
	sensitive bool
}

var actionSpecs = map[Action]actionSpec{
	CaseCreate:            {},
	CaseUpdate:            {details: []string{"", "rename", "economics", "answers", "scenarios", "compare", "ask", "watch", "plan", "bom_version"}},
	CaseExport:            {details: []string{"", "markdown", "cfr177"}},
	CaseDelete:            {details: []string{"", "retention"}, sensitive: true},
	CasePin:               {details: []string{"", "pin", "unpin"}},
	CaseRead:              {},
	WorkspaceCreate:       {},
	WorkspaceInvite:       {sensitive: true},
	WorkspaceJoin:         {sensitive: true},
	WorkspaceRoleChange:   {details: []string{"", "viewer", "editor", "admin"}, sensitive: true},
	WorkspaceMemberRemove: {sensitive: true},
	WorkspaceLeave:        {sensitive: true},
	WorkspaceRetention:    {details: []string{"", "keep", "expire"}, sensitive: true},
	APIKeyIssue:           {sensitive: true},
	APIKeyList:            {},
	APIKeyDisable:         {sensitive: true},
	AdminWatchRun:         {},
	AuditRead:             {},
	AuditVerify:           {},
	AuditExport:           {details: []string{"", "jsonl", "csv"}},
	AuthDenied:            {},
	// Creating a link hands case content to someone outside the system: it
	// fails closed like issuing an API key. Revoking never waits on the trail.
	ReviewCreate:   {details: []string{"setup", "plan"}, sensitive: true},
	ReviewRevoke:   {},
	ReviewView:     {},
	ReviewDecision: {details: []string{"approve", "changes", "file_request"}},
}

// Actions lists every action, for coverage tests and docs.
func Actions() []Action {
	out := make([]Action, 0, len(actionSpecs))
	for a := range actionSpecs {
		out = append(out, a)
	}
	return out
}

// Known reports whether a is in the closed set.
func (a Action) Known() bool { _, ok := actionSpecs[a]; return ok }

// Sensitive reports whether a fails closed: callers must store the intent
// before the operation and refuse the operation (5xx, no state change) if they
// cannot.
func (a Action) Sensitive() bool { return actionSpecs[a].sensitive }

// ActorKind is the class of caller. Only user and apikey carry a pseudonym.
type ActorKind string

const (
	ActorUser      ActorKind = "user"
	ActorAPIKey    ActorKind = "apikey"
	ActorAdmin     ActorKind = "admin" // the shared admin token: no individual
	ActorAnonymous ActorKind = "anonymous"
	ActorSystem    ActorKind = "system"
)

// Phase is where an event sits in an operation.
type Phase string

const (
	PhaseIntent     Phase = "intent"
	PhaseCompletion Phase = "completion"
	PhaseSingle     Phase = "single"
)

// Outcome is how it went.
type Outcome string

const (
	Allowed Outcome = "allowed"
	Denied  Outcome = "denied"
	Failed  Outcome = "failed"
)

// Reason is why an event is a denial or a failure. The set is closed.
type Reason string

const (
	ReasonNone              Reason = ""
	ReasonForbidden         Reason = "forbidden"
	ReasonHidden            Reason = "hidden"
	ReasonUnauthenticated   Reason = "unauthenticated"
	ReasonInvalidCredential Reason = "invalid_credential"
	ReasonInvalidInput      Reason = "invalid_input"
	ReasonConflict          Reason = "conflict"
	ReasonNotFound          Reason = "not_found"
	ReasonUpstream          Reason = "upstream"
	ReasonUnavailable       Reason = "unavailable"
	ReasonInternal          Reason = "internal"
	ReasonSuperseded        Reason = "superseded"
	ReasonAuditUnavailable  Reason = "audit_unavailable"
	ReasonRateLimited       Reason = "rate_limited"
)

var reasons = map[Reason]bool{
	ReasonNone: true, ReasonForbidden: true, ReasonHidden: true, ReasonUnauthenticated: true, ReasonInvalidCredential: true,
	ReasonInvalidInput: true, ReasonConflict: true, ReasonNotFound: true, ReasonUpstream: true, ReasonUnavailable: true,
	ReasonInternal: true, ReasonSuperseded: true, ReasonAuditUnavailable: true, ReasonRateLimited: true,
}

// ReasonForStatus maps an HTTP status to the closed reason a failed
// completion carries.
func ReasonForStatus(status int) Reason {
	switch {
	case status == 400:
		return ReasonInvalidInput
	case status == 401:
		return ReasonUnauthenticated
	case status == 403:
		return ReasonForbidden
	case status == 404:
		return ReasonNotFound
	case status == 409:
		return ReasonConflict
	case status == 429:
		return ReasonRateLimited
	case status == 502:
		return ReasonUpstream
	case status == 503:
		return ReasonUnavailable
	}
	return ReasonInternal
}

// Event is one audit record, as stored and as returned by the API.
type Event struct {
	Tenant    string    `json:"tenant"`
	Seq       uint64    `json:"seq"`
	Time      time.Time `json:"time"`
	ActorKind ActorKind `json:"actor_kind"`
	Actor     string    `json:"actor,omitempty"`
	Target    string    `json:"target,omitempty"`
	Action    Action    `json:"action"`
	Phase     Phase     `json:"phase"`
	Outcome   Outcome   `json:"outcome"`
	Reason    Reason    `json:"reason,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	Object    string    `json:"object,omitempty"`
	Route     string    `json:"route,omitempty"`
	RequestID string    `json:"request_id,omitempty"`
	// Ref is the Seq of the intent a completion belongs to (0: none).
	Ref uint64 `json:"ref,omitempty"`
	// Suppressed counts best-effort denial events dropped by the flood limiter
	// since the previous event written for this tenant by this process.
	Suppressed int    `json:"suppressed,omitempty"`
	PrevHash   string `json:"prev_hash"`
	Hash       string `json:"hash"`
}

// Genesis is the PrevHash of the first event of a tenant.
var Genesis = strings.Repeat("0", 64)

var (
	tenantPattern = regexp.MustCompile(`^(ws:ws_[0-9a-f]{12}|own:(u_|key_)?[0-9a-f]{32}|platform)$`)
	actorPattern  = regexp.MustCompile(`^a[a-z0-9]{1,8}_[0-9a-f]{32}$`)
	// An object is either an ID the server itself minted (and so holds no
	// user text by construction), "new" for an API key not yet issued, or a
	// keyed opaque hash of an ID the requester supplied and the server did not
	// confirm (see Pseudonymizer.Opaque).
	objectPattern  = regexp.MustCompile(`^(case:(case_[0-9a-f]{12}|h_[0-9a-f]{16})|workspace:(ws_[0-9a-f]{12}|h_[0-9a-f]{16})|apikey:([0-9a-f]{16}|new|h_[0-9a-f]{16})|batch)$`)
	routePattern   = regexp.MustCompile(`^[A-Z]{3,7} /[A-Za-z0-9/_{}.-]{0,110}$`)
	requestPattern = regexp.MustCompile(`^(h_)?[0-9a-f]{16}$`)
	hashPattern    = regexp.MustCompile(`^[0-9a-f]{64}$`)
	keyIDPattern   = regexp.MustCompile(`^[a-z0-9]{1,8}$`)
)

// ErrInvalid wraps every validation failure.
var ErrInvalid = errors.New("audit: invalid event")

// ValidTenant reports whether s is a tenant string.
func ValidTenant(s string) bool { return tenantPattern.MatchString(s) }

// NormalizeRequestID returns the request ID to store for an UNTRUSTED value:
// "h_" plus the first 16 hex of its SHA-256, always, whatever its shape (a
// 16-hex or "h_" value is just as easy for a client to plant as any other
// text). A request that went through the server's pipeline carries a
// logsafe.Ref instead (Draft.RequestRef), which is stored unchanged so that
// the log line, the response header and the audit event show one id. ""
// stays "".
func NormalizeRequestID(id string) string {
	if id == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(id))
	return "h_" + hex.EncodeToString(sum[:8])
}

// validate checks the fields a caller supplies (everything except the
// chain fields, which Seal sets).
func (e Event) validateFields() error {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
	}
	spec, ok := actionSpecs[e.Action]
	if !ok {
		return bad("unknown action %q", e.Action)
	}
	if !tenantPattern.MatchString(e.Tenant) {
		return bad("tenant %q", e.Tenant)
	}
	switch e.ActorKind {
	case ActorUser, ActorAPIKey:
		if !actorPattern.MatchString(e.Actor) {
			return bad("actor of a %s must be a pseudonym", e.ActorKind)
		}
	case ActorAdmin, ActorAnonymous, ActorSystem:
		if e.Actor != "" {
			return bad("actor of kind %s must be empty", e.ActorKind)
		}
	default:
		return bad("unknown actor kind %q", e.ActorKind)
	}
	if e.Target != "" && !actorPattern.MatchString(e.Target) {
		return bad("target must be a pseudonym")
	}
	switch {
	case e.Phase == PhaseIntent && e.Outcome == Allowed:
	case e.Phase == PhaseCompletion && (e.Outcome == Allowed || e.Outcome == Failed):
	case e.Phase == PhaseSingle:
	default:
		return bad("phase %q cannot have outcome %q", e.Phase, e.Outcome)
	}
	if !reasons[e.Reason] {
		return bad("unknown reason %q", e.Reason)
	}
	if (e.Outcome == Allowed) != (e.Reason == ReasonNone) {
		return bad("a reason is required exactly when the outcome is not allowed")
	}
	okDetail := e.Detail == "" && spec.details == nil
	for _, d := range spec.details {
		okDetail = okDetail || d == e.Detail
	}
	if !okDetail {
		return bad("detail %q is not allowed for %s", e.Detail, e.Action)
	}
	if e.Object != "" && !objectPattern.MatchString(e.Object) {
		return bad("object %q", e.Object)
	}
	if e.Route != "" && !routePattern.MatchString(e.Route) {
		return bad("route %q", e.Route)
	}
	if e.RequestID != "" && !requestPattern.MatchString(e.RequestID) {
		return bad("request id must be normalized")
	}
	if e.Phase != PhaseCompletion && e.Ref != 0 {
		return bad("only a completion has a ref")
	}
	if e.Suppressed < 0 {
		return bad("suppressed")
	}
	return nil
}

// Validate checks a complete stored event, chain fields included.
func (e Event) Validate() error {
	if err := e.validateFields(); err != nil {
		return err
	}
	if e.Seq == 0 || e.Time.IsZero() || !hashPattern.MatchString(e.PrevHash) || !hashPattern.MatchString(e.Hash) {
		return fmt.Errorf("%w: chain fields", ErrInvalid)
	}
	return nil
}

// ComputeHash is the event hash: SHA-256 over a fixed, newline-delimited
// encoding. Every field is enum- or pattern-checked, so none can contain a
// newline and the encoding is unambiguous.
func (e Event) ComputeHash() string {
	fields := []string{
		"v1", e.Tenant, strconv.FormatUint(e.Seq, 10), strconv.FormatInt(e.Time.UTC().UnixMicro(), 10),
		string(e.ActorKind), e.Actor, e.Target, string(e.Action), string(e.Phase), string(e.Outcome), string(e.Reason),
		e.Detail, e.Object, e.Route, e.RequestID, strconv.FormatUint(e.Ref, 10), strconv.Itoa(e.Suppressed), e.PrevHash,
	}
	sum := sha256.Sum256([]byte(strings.Join(fields, "\n")))
	return hex.EncodeToString(sum[:])
}

// Head is the newest event of a tenant: the chain's tip. Seq 0 is an empty chain.
type Head struct {
	Seq  uint64    `json:"seq"`
	Hash string    `json:"hash"`
	Time time.Time `json:"time"`
}

// Seal turns a validated draft into the next event after head: it sets the
// sequence, the time (never earlier than the previous event, microsecond
// precision), the previous hash and the hash. Stores call it inside their own
// atomic section; it is pure, so a retried transaction may call it again.
func Seal(draft Event, head Head, now time.Time) (Event, error) {
	if err := draft.validateFields(); err != nil {
		return Event{}, err
	}
	t := now.UTC().Truncate(time.Microsecond)
	if t.Before(head.Time) {
		t = head.Time
	}
	prev := head.Hash
	if head.Seq == 0 {
		prev = Genesis
	}
	draft.Seq, draft.Time, draft.PrevHash = head.Seq+1, t, prev
	draft.Hash = draft.ComputeHash()
	return draft, nil
}

// HeadOf is the head after e.
func HeadOf(e Event) Head { return Head{Seq: e.Seq, Hash: e.Hash, Time: e.Time} }

// Checkpoint records that a tenant's events up to ThroughSeq were purged by
// retention, so the retained suffix can still be verified.
type Checkpoint struct {
	ThroughSeq  uint64    `json:"through_seq"`
	ThroughHash string    `json:"through_hash"`
	ThroughTime time.Time `json:"through_time"`
	PurgedAt    time.Time `json:"purged_at"`
}

// ObjectRef returns "<kind>:<id>" when id has the exact format of an ID the
// server mints for that kind (kind is case, workspace or apikey), so it can be
// stored raw. Any other id is text a requester chose and the caller must use
// Pseudonymizer.Opaque instead.
func ObjectRef(kind, id string) (string, bool) {
	var ok bool
	switch kind {
	case "case":
		ok = caseIDPattern.MatchString(id)
	case "workspace":
		ok = workspaceIDPattern.MatchString(id)
	case "apikey":
		ok = apiKeyIDPattern.MatchString(id)
	}
	if !ok {
		return "", false
	}
	return kind + ":" + id, true
}

var (
	caseIDPattern      = regexp.MustCompile(`^case_[0-9a-f]{12}$`)
	workspaceIDPattern = regexp.MustCompile(`^ws_[0-9a-f]{12}$`)
	apiKeyIDPattern    = regexp.MustCompile(`^[0-9a-f]{16}$`)
)

// ValidActor reports whether s is a pseudonym (for the actor filter).
func ValidActor(s string) bool { return actorPattern.MatchString(s) }

// ValidObject reports whether s is an object reference.
func ValidObject(s string) bool { return objectPattern.MatchString(s) }
