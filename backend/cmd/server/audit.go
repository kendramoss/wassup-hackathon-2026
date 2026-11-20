package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// auditPurgeEvery is how often the in-process sweeper removes events older
// than the retention. It runs once at start as well.
const auditPurgeEvery = 6 * time.Hour

// newAudit builds the audit trail (#284), or returns nil when it is off.
//
// Off is the default unless an operator supplies AUDIT_ACTOR_KEY (see
// config.Audit): the trail is never enabled in production without that secret,
// and no key is generated here. When it is off nothing is recorded and no
// audit evidence exists, and the log says so.
func newAudit(ctx context.Context, cfg config.Config, cases store.Cases, logger *slog.Logger) (*audit.Service, error) {
	if !cfg.Audit {
		logger.Warn("audit trail NOT enabled: no events are recorded and no audit evidence is produced (set AUDIT_ACTOR_KEY, or AUDIT=on with it)",
			"audit", cfg.AuditMode)
		return nil, nil
	}
	ps, err := audit.NewPseudonymizer(cfg.AuditActorKeyID, []byte(cfg.AuditActorKey))
	if err != nil {
		return nil, err
	}
	var st audit.Store
	switch cfg.AuditStore {
	case config.CasesFirestore:
		fs, ok := cases.(*store.Firestore)
		if !ok { // cases live elsewhere: the trail still goes to Firestore, on its own client
			if fs, err = store.NewFirestore(ctx, cfg.GCPProject, cfg.FirestoreDatabase); err != nil {
				return nil, err
			}
		}
		st = fs.Audit()
	case config.CasesFile:
		if st, err = audit.NewFile(cfg.AuditDir); err != nil {
			return nil, err
		}
	case config.CasesMemory:
		st = audit.NewMemory()
	default:
		return nil, fmt.Errorf("audit: unknown store %q", cfg.AuditStore)
	}
	svc := audit.NewService(st, ps, audit.Options{Retention: time.Duration(cfg.AuditRetentionDays) * 24 * time.Hour})
	go svc.PurgeEvery(ctx, auditPurgeEvery, func(deleted int, err error) {
		if err != nil {
			logger.Error("audit retention purge failed", "deleted", deleted)
			return
		}
		if deleted > 0 {
			logger.Info("audit retention purge", "deleted", deleted, "retention_days", cfg.AuditRetentionDays)
		}
	})
	logger.Info("audit trail on", "store", cfg.AuditStore, "retention_days", cfg.AuditRetentionDays, "actor_key_id", cfg.AuditActorKeyID)
	if cfg.AuditStore == config.CasesMemory {
		logger.Warn("audit trail is in memory: it is lost on restart and not shared between instances")
	}
	return svc, nil
}
