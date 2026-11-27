package audit

import (
	"context"
	"fmt"
	"time"
)

// PurgeTenant removes the events of tenant older than window, in chunks, the
// same way the global sweeper does: the store records the checkpoint (through
// sequence and hash) before it deletes, so the retained suffix still verifies
// and the head never moves. window may be shorter than the global retention;
// it is never longer (a longer one is cut to it). dryRun counts the events
// without removing any. After a real purge that removed events, one system
// audit.purge event is appended to the tenant, so the trail itself shows that
// a workspace window was applied.
func (s *Service) PurgeTenant(ctx context.Context, tenant string, window time.Duration, dryRun bool) (int, error) {
	if window <= 0 || window > s.opt.Retention {
		window = s.opt.Retention
	}
	now := s.opt.Now()
	cutoff := now.Add(-window)
	total := 0
	if dryRun {
		var cur uint64
		for {
			evs, err := s.store.Range(ctx, tenant, Range{Cursor: cur, Limit: s.opt.PurgeChunk})
			if err != nil {
				return total, err
			}
			for _, e := range evs {
				if !e.Time.Before(cutoff) {
					return total, nil
				}
				total++
				cur = e.Seq
			}
			if len(evs) < s.opt.PurgeChunk {
				return total, nil
			}
		}
	}
	for {
		r, err := s.store.Purge(ctx, tenant, cutoff, now, s.opt.PurgeChunk)
		if err != nil {
			return total, err
		}
		total += r.Deleted
		if r.Deleted < s.opt.PurgeChunk {
			break
		}
	}
	if total > 0 {
		ws, ok := workspaceOfTenant(tenant)
		if !ok {
			return total, nil
		}
		obj, _ := ObjectRef("workspace", ws)
		if _, err := s.Record(ctx, Draft{Event: Event{Tenant: tenant, ActorKind: ActorSystem, Action: AuditPurge,
			Phase: PhaseSingle, Outcome: Allowed, Detail: "workspace", Object: obj}}); err != nil {
			return total, fmt.Errorf("audit: record purge: %w", err)
		}
	}
	return total, nil
}

func workspaceOfTenant(tenant string) (string, bool) {
	const p = "ws:"
	if len(tenant) <= len(p) || tenant[:len(p)] != p {
		return "", false
	}
	return tenant[len(p):], true
}
