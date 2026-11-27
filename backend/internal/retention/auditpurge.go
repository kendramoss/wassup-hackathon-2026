package retention

import (
	"context"
	"sort"
	"time"
)

// AuditPurger removes a tenant's audit events older than a window
// (audit.Service.PurgeTenant). dryRun only counts.
type AuditPurger interface {
	PurgeTenant(ctx context.Context, tenant string, window time.Duration, dryRun bool) (int, error)
}

// AuditResult is one workspace's audit purge.
type AuditResult struct {
	Workspace string `json:"workspace"`
	// Days is the window applied: the workspace's audit_days, cut to the
	// global window when that is shorter.
	Days   int `json:"days"`
	Events int `json:"events"`
}

// purgeAudit applies each workspace's audit window. A workspace with
// audit_days 0 keeps the global window, which the server's own sweeper
// enforces, so it is not touched here.
func purgeAudit(ctx context.Context, ps []Policy, rep *Report, o Options) error {
	if o.Audit == nil || o.AuditWindow <= 0 {
		return nil
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].Workspace < ps[j].Workspace })
	global := int(o.AuditWindow.Hours() / 24)
	for _, p := range ps {
		if p.AuditDays <= 0 || (o.Workspace != "" && p.Workspace != o.Workspace) || p.Validate() != nil {
			continue
		}
		days := p.AuditDays
		if days > global {
			days = global
		}
		n, err := o.Audit.PurgeTenant(ctx, "ws:"+p.Workspace, time.Duration(days)*24*time.Hour, o.DryRun)
		rep.Audit = append(rep.Audit, AuditResult{Workspace: p.Workspace, Days: days, Events: n})
		if err != nil {
			return err
		}
	}
	return nil
}
