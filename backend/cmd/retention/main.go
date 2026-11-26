// Command retention runs the workspace case retention purge (#875).
//
// Usage: go run ./cmd/retention [-apply] [-workspace ws_...] [-limit N]
//
// It reads the same environment as the server (CASES_STORE, CASES_DIR,
// FIRESTORE_DATABASE, AUDIT_*) and needs RETENTION=on. Without -apply it is a
// dry run: it prints what would be deleted and changes nothing. With -apply,
// for each case past its workspace's policy it writes a tombstone, records a
// system case.delete audit event (detail "retention") when the audit trail
// is on, then deletes the case. Pinned demo cases are never purged.
//
// The operator schedules it (for example a daily Cloud Run job); nothing in
// the server runs it.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retention"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err == nil {
		err = run(context.Background(), cfg, os.Args[1:], os.Stdout, time.Now())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "retention:", err)
		os.Exit(1)
	}
}

// stores are what one run works on.
type stores struct {
	ret   retention.Store
	cases store.Cases
	audit *audit.Service
}

func open(ctx context.Context, cfg config.Config) (stores, error) {
	var s stores
	switch {
	case cfg.CasesStore == config.CasesFirestore:
		fs, err := store.NewFirestore(ctx, cfg.GCPProject, cfg.FirestoreDatabase)
		if err != nil {
			return s, err
		}
		s.ret, s.cases = fs.Retention(), fs
		if cfg.Audit {
			ps, err := audit.NewPseudonymizer(cfg.AuditActorKeyID, []byte(cfg.AuditActorKey))
			if err != nil {
				return s, err
			}
			s.audit = audit.NewService(fs.Audit(), ps, audit.Options{Retention: time.Duration(cfg.AuditRetentionDays) * 24 * time.Hour})
		}
	case cfg.CasesStore == config.CasesFile && cfg.CasesDir != "":
		cs, err := store.NewFile(cfg.CasesDir)
		if err != nil {
			return s, err
		}
		rs, err := retention.NewFile(cfg.CasesDir)
		if err != nil {
			return s, err
		}
		s.ret, s.cases = rs, cs
		if cfg.Audit && cfg.AuditDir != "" {
			ps, err := audit.NewPseudonymizer(cfg.AuditActorKeyID, []byte(cfg.AuditActorKey))
			if err != nil {
				return s, err
			}
			as, err := audit.NewFile(cfg.AuditDir)
			if err != nil {
				return s, err
			}
			s.audit = audit.NewService(as, ps, audit.Options{Retention: time.Duration(cfg.AuditRetentionDays) * 24 * time.Hour})
		}
	default:
		return s, errors.New("retention needs CASES_STORE=firestore or file: an in-memory store has nothing to purge")
	}
	return s, nil
}

var openStores = open

func run(ctx context.Context, cfg config.Config, args []string, out io.Writer, now time.Time) error {
	fl := flag.NewFlagSet("retention", flag.ContinueOnError)
	apply := fl.Bool("apply", false, "delete (default: dry run)")
	ws := fl.String("workspace", "", "only this workspace")
	limit := fl.Int("limit", 0, "stop after this many deletions (0: none)")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if !cfg.Retention {
		return errors.New("RETENTION is off")
	}
	s, err := openStores(ctx, cfg)
	if err != nil {
		return err
	}
	rep, err := retention.Purge(ctx, s.ret, store.RetentionCases{Cases: s.cases}, retention.AuditRecorder(s.audit),
		retention.Options{Now: now.UTC(), DryRun: !*apply, Workspace: *ws, Limit: *limit})
	enc := json.NewEncoder(out)
	enc.SetIndent("", " ")
	if e := enc.Encode(rep); e != nil && err == nil {
		err = e
	}
	return err
}
