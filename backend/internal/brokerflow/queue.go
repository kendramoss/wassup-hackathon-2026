// Package brokerflow holds the rules of the broker workflow: the review
// queue's states and review targets, the 19 CFR 177.2 element check and the
// filing package. It has no HTTP and no storage; internal/httpapi applies it
// to cases.
package brokerflow

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// ErrInvalid is a request the rules refuse; ErrConflict is a transition the
// item's current state does not allow.
var (
	ErrInvalid  = errors.New("brokerflow: invalid")
	ErrConflict = errors.New("brokerflow: conflict")
)

// Queue states.
const (
	StateQueued   = "queued"
	StateAssigned = "assigned"
	StateInReview = "in_review"
	StateDecided  = "decided"
)

// Priorities and their review targets: how long after queueing a decision
// is due.
var reviewTargets = map[string]time.Duration{
	"urgent": 4 * time.Hour,
	"high":   24 * time.Hour,
	"normal": 72 * time.Hour,
	"low":    7 * 24 * time.Hour,
}

// Priorities lists the accepted priorities, most urgent first.
func Priorities() []string { return []string{"urgent", "high", "normal", "low"} }

// ReviewTarget is the time allowed for a priority.
func ReviewTarget(priority string) (time.Duration, bool) {
	d, ok := reviewTargets[priority]
	return d, ok
}

// Outcomes a reviewer or a sign-off may give.
var outcomes = map[string]bool{"approve": true, "reject": true, "changes": true}

// ValidOutcome reports whether o is approve, reject or changes.
func ValidOutcome(o string) bool { return outcomes[o] }

// MaxText caps a reason, comment or note.
const MaxText = 2000

// CleanText trims s and refuses one that is too long or has control
// characters other than newlines and tabs.
func CleanText(s, field string, required bool) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" && required {
		return "", fmt.Errorf("%w: %s is required", ErrInvalid, field)
	}
	if len([]rune(s)) > MaxText {
		return "", fmt.Errorf("%w: %s is longer than %d characters", ErrInvalid, field, MaxText)
	}
	for _, r := range s {
		if r < 0x20 && r != '\n' && r != '\t' {
			return "", fmt.Errorf("%w: %s has a control character", ErrInvalid, field)
		}
	}
	return s, nil
}

// Enqueue puts a case in the queue, or re-queues a decided one. A case
// already waiting or in review keeps its place; only its priority and
// reason change (and its due time, from the original queue time).
func Enqueue(cur *store.QueueItem, priority, reason, by string, now time.Time) (*store.QueueItem, error) {
	if priority == "" {
		priority = "normal"
	}
	target, ok := reviewTargets[priority]
	if !ok {
		return nil, fmt.Errorf("%w: priority must be one of %s", ErrInvalid, strings.Join(Priorities(), ", "))
	}
	reason, err := CleanText(reason, "reason", true)
	if err != nil {
		return nil, err
	}
	if cur != nil && cur.State != StateDecided {
		next := clone(cur)
		next.Priority, next.Reason, next.DueAt = priority, reason, cur.QueuedAt.Add(target)
		next.Log = append(next.Log, store.QueueStep{Action: "reprioritise", By: by, At: now, Detail: priority})
		return next, nil
	}
	next := &store.QueueItem{State: StateQueued, Priority: priority, Reason: reason, QueuedBy: by, QueuedAt: now, DueAt: now.Add(target)}
	if cur != nil {
		next.Log = append(clone(cur).Log, store.QueueStep{Action: "requeue", By: by, At: now, Detail: priority})
	} else {
		next.Log = []store.QueueStep{{Action: "enqueue", By: by, At: now, Detail: priority}}
	}
	return next, nil
}

// Transition is one move of a queued item.
type Transition struct {
	Action   string // assign | start | decide | release
	Assignee string // assign: who reviews
	Outcome  string // decide
	Reason   string // decide (required), release (optional)
	By       string
}

// Apply returns the item after t, or an error. The current item is not
// changed.
func Apply(cur *store.QueueItem, t Transition, now time.Time) (*store.QueueItem, error) {
	if cur == nil {
		return nil, fmt.Errorf("%w: the case is not in the review queue", ErrConflict)
	}
	next := clone(cur)
	step := store.QueueStep{Action: t.Action, By: t.By, At: now}
	switch t.Action {
	case "assign":
		if cur.State == StateDecided {
			return nil, fmt.Errorf("%w: the review is already decided; queue it again first", ErrConflict)
		}
		who := strings.ToLower(strings.TrimSpace(t.Assignee))
		if who == "" || !strings.Contains(who, "@") || len(who) > 254 {
			return nil, fmt.Errorf("%w: assignee must be an email address", ErrInvalid)
		}
		next.State, next.Assignee, next.AssignedAt, next.StartedAt = StateAssigned, who, &now, nil
		step.Detail = who
	case "start":
		if cur.State != StateAssigned && cur.State != StateQueued {
			return nil, fmt.Errorf("%w: only a waiting review can start (it is %s)", ErrConflict, cur.State)
		}
		if cur.Assignee != "" && cur.Assignee != t.By {
			return nil, fmt.Errorf("%w: this review is assigned to someone else", ErrConflict)
		}
		if next.Assignee == "" {
			next.Assignee, next.AssignedAt = t.By, &now
		}
		next.State, next.StartedAt = StateInReview, &now
	case "decide":
		if cur.State != StateInReview {
			return nil, fmt.Errorf("%w: start the review before deciding (it is %s)", ErrConflict, cur.State)
		}
		if cur.Assignee != t.By {
			return nil, fmt.Errorf("%w: only the assigned reviewer can decide", ErrConflict)
		}
		if !ValidOutcome(t.Outcome) {
			return nil, fmt.Errorf("%w: outcome must be approve, reject or changes", ErrInvalid)
		}
		reason, err := CleanText(t.Reason, "reason", true)
		if err != nil {
			return nil, err
		}
		next.State = StateDecided
		next.Decision = &store.QueueDecision{Outcome: t.Outcome, Reason: reason, By: t.By, At: now, OnTime: !now.After(cur.DueAt)}
		step.Detail = t.Outcome
	case "release":
		if cur.State == StateDecided || cur.State == StateQueued {
			return nil, fmt.Errorf("%w: only an assigned or started review can be released (it is %s)", ErrConflict, cur.State)
		}
		reason, err := CleanText(t.Reason, "reason", false)
		if err != nil {
			return nil, err
		}
		next.State, next.Assignee, next.AssignedAt, next.StartedAt = StateQueued, "", nil, nil
		step.Detail = reason
	default:
		return nil, fmt.Errorf("%w: action must be assign, start, decide or release", ErrInvalid)
	}
	next.Log = append(next.Log, step)
	if len(next.Log) > maxQueueLog {
		next.Log = next.Log[len(next.Log)-maxQueueLog:]
	}
	return next, nil
}

const maxQueueLog = 100

// Overdue reports whether an undecided item is past its due time.
func Overdue(q *store.QueueItem, now time.Time) bool {
	return q != nil && q.State != StateDecided && now.After(q.DueAt)
}

// Less orders queue items for a reviewer: overdue first, then by due time,
// then by queue time.
func Less(a, b *store.QueueItem, now time.Time) bool {
	oa, ob := Overdue(a, now), Overdue(b, now)
	if oa != ob {
		return oa
	}
	if !a.DueAt.Equal(b.DueAt) {
		return a.DueAt.Before(b.DueAt)
	}
	return a.QueuedAt.Before(b.QueuedAt)
}

func clone(q *store.QueueItem) *store.QueueItem {
	c := *q
	c.Log = append([]store.QueueStep(nil), q.Log...)
	if q.Decision != nil {
		d := *q.Decision
		c.Decision = &d
	}
	return &c
}
