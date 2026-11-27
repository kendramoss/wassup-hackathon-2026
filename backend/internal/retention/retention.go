// Package retention keeps a workspace's cases for as long as its admin says,
// and no longer (#875).
//
// A workspace admin sets a policy: cases not updated for CaseDays days are
// hard-deleted by a purge run (cmd/retention, scheduled by the operator; there
// is no in-process timer). Each deletion first writes a tombstone (the case
// ID, when, why, under which policy), so "was case X deleted, and why" stays
// answerable after the case itself is gone. Tombstones hold no case content.
//
// Off unless RETENTION=on. Pinned cases (the seeded demos) are never purged.
// Cases outside a workspace (anonymous or personal) have no policy and are not
// touched.
package retention

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
)

// Limits on a policy.
const (
	MinCaseDays  = 7
	MaxCaseDays  = 3650
	MinAuditDays = 7
)

// Policy is one workspace's retention setting. CaseDays 0 means keep forever.
type Policy struct {
	Workspace string `json:"workspace" firestore:"workspace"`
	CaseDays  int    `json:"case_days" firestore:"case_days"`
	// AuditDays is how long this workspace's audit events are kept, set by its
	// admin; 0 means the global AUDIT_RETENTION_DAYS. It can only shorten the
	// global window, never lengthen it.
	AuditDays int       `json:"audit_days" firestore:"audit_days"`
	UpdatedAt time.Time `json:"updated_at" firestore:"updated_at"`
}

// Reasons a tombstone records.
const (
	ReasonPolicy = "retention_policy"
	ReasonUser   = "user_delete"
)

// Tombstone records a hard-deleted case. It never holds case content.
type Tombstone struct {
	CaseID     string    `json:"case_id" firestore:"case_id"`
	Workspace  string    `json:"workspace,omitempty" firestore:"workspace"`
	DeletedAt  time.Time `json:"deleted_at" firestore:"deleted_at"`
	Reason     string    `json:"reason" firestore:"reason"`
	PolicyDays int       `json:"policy_days,omitempty" firestore:"policy_days"`
	// LastUpdated is when the case was last changed before it was deleted.
	LastUpdated time.Time `json:"last_updated" firestore:"last_updated"`
}

var (
	workspacePattern = regexp.MustCompile(`^ws_[0-9a-f]{12}$`)
	casePattern      = regexp.MustCompile(`^case_[0-9a-f]{12}$`)
)

// ErrInvalid wraps policy validation failures.
var ErrInvalid = errors.New("retention: invalid")

// Validate checks a policy.
func (p Policy) Validate() error {
	if !workspacePattern.MatchString(p.Workspace) {
		return fmt.Errorf("%w: workspace id", ErrInvalid)
	}
	if p.CaseDays != 0 && (p.CaseDays < MinCaseDays || p.CaseDays > MaxCaseDays) {
		return fmt.Errorf("%w: case_days must be 0 (keep forever) or %d to %d", ErrInvalid, MinCaseDays, MaxCaseDays)
	}
	if p.AuditDays != 0 && (p.AuditDays < MinAuditDays || p.AuditDays > MaxCaseDays) {
		return fmt.Errorf("%w: audit_days must be 0 (the global window) or %d to %d", ErrInvalid, MinAuditDays, MaxCaseDays)
	}
	return nil
}

// Store keeps policies and tombstones.
type Store interface {
	Policy(ctx context.Context, workspace string) (Policy, bool, error)
	SetPolicy(ctx context.Context, p Policy) error
	Policies(ctx context.Context) ([]Policy, error)
	AddTombstone(ctx context.Context, t Tombstone) error
	// Tombstones lists a workspace's tombstones, newest first, at most limit.
	Tombstones(ctx context.Context, workspace string, limit int) ([]Tombstone, error)
}

// Case is what a purge needs to know about a case.
type Case struct {
	ID        string
	Workspace string
	UpdatedAt time.Time
	Pinned    bool
	// Derived are blobs the case's own content points at (supplier
	// documents), purged with it when CACHE_PURGE is on.
	Derived []blob.Ref
}

// Cases is the case store as a purge sees it.
type Cases interface {
	WorkspaceCases(ctx context.Context, workspace string) ([]Case, error)
	DeleteCase(ctx context.Context, id string) error
}

// Recorder records one audit event per deletion. Nil: none.
type Recorder func(ctx context.Context, t Tombstone) error

// Options for a purge run.
type Options struct {
	Now       time.Time
	DryRun    bool
	Workspace string // only this workspace; "" all with a policy
	Limit     int    // stop after this many deletions; <= 0 no limit
	// Caches, when set, removes what each deleted case derived (#875): the
	// cached predictions, verdicts and advice only that case used, and its
	// supplier documents. It runs after the tombstone and the audit event and
	// before the delete; a failure stops the run with the case still there.
	Caches CachePurger
	// AuditWindow, when set, is the longest an audit event may stay in a
	// workspace's trail when its policy has no shorter one (the global
	// AUDIT_RETENTION_DAYS). Needs Audit.
	AuditWindow time.Duration
	// Audit purges workspace audit trails (see AuditPurger). Nil: none.
	Audit AuditPurger
}

// CachePurger removes what one case derived (blob.Purger).
type CachePurger interface {
	PurgeCase(ctx context.Context, caseID string, extra []blob.Ref) (blob.PurgeReport, error)
}

// Report is what a purge run did, or would do in a dry run.
type Report struct {
	DryRun     bool        `json:"dry_run"`
	Workspaces int         `json:"workspaces"`
	Checked    int         `json:"checked"`
	Deleted    []Tombstone `json:"deleted"`
	Skipped    int         `json:"skipped"` // pinned, or an ID the server did not mint
	Truncated  bool        `json:"truncated"`
	// Caches totals the derived entries removed for the deleted cases.
	Caches blob.PurgeReport `json:"caches"`
	// Audit lists, per workspace with a shorter audit window, the events
	// removed (or, in a dry run, that would be).
	Audit []AuditResult `json:"audit,omitempty"`
}

// Purge deletes, for every workspace with a policy, the cases not updated for
// longer than the policy allows. For each it writes the tombstone, then
// records the audit event, then deletes; a failure stops the run before the
// case is deleted, so a deleted case always has a tombstone.
func Purge(ctx context.Context, st Store, cs Cases, rec Recorder, o Options) (Report, error) {
	rep := Report{DryRun: o.DryRun, Deleted: []Tombstone{}}
	if o.Now.IsZero() {
		return rep, errors.New("retention: Now is required")
	}
	ps, err := st.Policies(ctx)
	if err != nil {
		return rep, err
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].Workspace < ps[j].Workspace })
	for _, p := range ps {
		if p.CaseDays <= 0 || (o.Workspace != "" && p.Workspace != o.Workspace) || p.Validate() != nil {
			continue
		}
		rep.Workspaces++
		cutoff := o.Now.Add(-time.Duration(p.CaseDays) * 24 * time.Hour)
		cases, err := cs.WorkspaceCases(ctx, p.Workspace)
		if err != nil {
			return rep, err
		}
		sort.Slice(cases, func(i, j int) bool { return cases[i].UpdatedAt.Before(cases[j].UpdatedAt) })
		for _, c := range cases {
			rep.Checked++
			if c.Workspace != p.Workspace || !c.UpdatedAt.Before(cutoff) {
				continue
			}
			if c.Pinned || !casePattern.MatchString(c.ID) {
				rep.Skipped++
				continue
			}
			if o.Limit > 0 && len(rep.Deleted) == o.Limit {
				rep.Truncated = true
				return rep, nil
			}
			t := Tombstone{CaseID: c.ID, Workspace: p.Workspace, DeletedAt: o.Now.UTC(), Reason: ReasonPolicy,
				PolicyDays: p.CaseDays, LastUpdated: c.UpdatedAt.UTC()}
			if !o.DryRun {
				pr, err := deleteCase(ctx, st, cs, rec, o.Caches, t, c.Derived)
				rep.Caches.Add(pr)
				if err != nil {
					return rep, err
				}
			}
			rep.Deleted = append(rep.Deleted, t)
		}
	}
	return rep, purgeAudit(ctx, ps, &rep, o)
}

// Delete writes t's tombstone, records it, and deletes the case.
func Delete(ctx context.Context, st Store, cs Cases, rec Recorder, t Tombstone) error {
	_, err := deleteCase(ctx, st, cs, rec, nil, t, nil)
	return err
}

// DeleteWithCaches is Delete plus the purge of what the case derived.
func DeleteWithCaches(ctx context.Context, st Store, cs Cases, rec Recorder, caches CachePurger, t Tombstone, derived []blob.Ref) (blob.PurgeReport, error) {
	return deleteCase(ctx, st, cs, rec, caches, t, derived)
}

func deleteCase(ctx context.Context, st Store, cs Cases, rec Recorder, caches CachePurger, t Tombstone, derived []blob.Ref) (blob.PurgeReport, error) {
	var pr blob.PurgeReport
	if !casePattern.MatchString(t.CaseID) {
		return pr, fmt.Errorf("%w: case id", ErrInvalid)
	}
	if err := st.AddTombstone(ctx, t); err != nil {
		return pr, fmt.Errorf("retention: tombstone %s: %w", t.CaseID, err)
	}
	if rec != nil {
		if err := rec(ctx, t); err != nil {
			return pr, fmt.Errorf("retention: audit %s: %w", t.CaseID, err)
		}
	}
	if caches != nil {
		var err error
		if pr, err = caches.PurgeCase(ctx, t.CaseID, derived); err != nil {
			return pr, fmt.Errorf("retention: caches %s: %w", t.CaseID, err)
		}
	}
	if err := cs.DeleteCase(ctx, t.CaseID); err != nil {
		return pr, fmt.Errorf("retention: delete %s: %w", t.CaseID, err)
	}
	return pr, nil
}
