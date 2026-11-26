package main

import (
	"context"
	"log/slog"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retention"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// newRetention returns the retention store (#875) next to the cases, or nil
// when RETENTION is off.
func newRetention(ctx context.Context, cfg config.Config, cases store.Cases, logger *slog.Logger) (retention.Store, error) {
	if !cfg.Retention {
		return nil, nil
	}
	var st retention.Store = retention.NewMemory()
	kind := config.CasesMemory
	switch {
	case cfg.CasesStore == config.CasesFirestore:
		fs, ok := cases.(*store.Firestore)
		if !ok {
			var err error
			if fs, err = store.NewFirestore(ctx, cfg.GCPProject, cfg.FirestoreDatabase); err != nil {
				return nil, err
			}
		}
		st, kind = fs.Retention(), config.CasesFirestore
	case cfg.CasesStore == config.CasesFile && cfg.CasesDir != "":
		f, err := retention.NewFile(cfg.CasesDir)
		if err != nil {
			return nil, err
		}
		st, kind = f, config.CasesFile
	}
	logger.Info("retention on: workspace policies and tombstones", "store", kind)
	return st, nil
}
