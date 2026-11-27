// Command evaldrift raises an alert when a monthly re-score falls below the
// earlier months (#921, D948).
//
// Usage:
//
//	go run ./cmd/evaldrift record -month YYYY-MM [-dir ../eval/rescore]
//	go run ./cmd/evaldrift check [-dir ../eval/rescore]
//	go run ./cmd/evaldrift emit -month YYYY-MM [-dir ../eval/rescore]
//
// record reads <dir>/<month>/result.json (written by eval/rescore/summarize.py),
// adds the month's CBP and Japan figures to <dir>/history.json (a series with no
// cases that month adds nothing; recording the same figures again changes
// nothing, different figures for a recorded month are refused), recomputes
// <dir>/drift.json and prints one line per month. check recomputes from the
// history alone, prints the lines and fails if drift.json differs, so an alert
// can be reproduced offline with no model and no network. Neither needs a flag.
//
// emit sends the alerts of one month: a system audit event (action eval.drift,
// tenant platform) when AUDIT is on, and an eval.drift webhook to every
// subscribed endpoint of every workspace when WEBHOOKS is on. It needs
// EVAL_DRIFT_ALERTS=on and reads the same environment as the server (CASES_STORE,
// CASES_DIR, FIRESTORE_DATABASE, AUDIT_*, WEBHOOKS_SECRET_KEY). Run it once per
// month after record; a receiver deduplicates on X-Webhook-Id and a second run
// sends a second event. Deliveries the first attempt does not complete are
// retried by the server's sweep.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/envelope"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evaldrift"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/webhooks"
)

func main() {
	var cfg config.Config
	var err error
	// Only emit reads the server's environment; record and check are offline.
	if len(os.Args) > 1 && os.Args[1] == "emit" {
		cfg, err = config.Load()
	}
	if err == nil {
		err = run(context.Background(), cfg, os.Args[1:], os.Stdout)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "evaldrift:", err)
		os.Exit(1)
	}
}

// sinks are where an alert goes; either may be nil.
type sinks struct {
	aud  *audit.Service
	hook *webhooks.Service
}

var openSinks = open

func open(ctx context.Context, cfg config.Config) (sinks, error) {
	var s sinks
	var hooks webhooks.Store
	var fs *store.Firestore
	if cfg.CasesStore == config.CasesFirestore {
		var err error
		if fs, err = store.NewFirestore(ctx, cfg.GCPProject, cfg.FirestoreDatabase); err != nil {
			return s, err
		}
	}
	retention := time.Duration(cfg.AuditRetentionDays) * 24 * time.Hour
	if cfg.Audit {
		ps, err := audit.NewPseudonymizer(cfg.AuditActorKeyID, []byte(cfg.AuditActorKey))
		if err != nil {
			return s, err
		}
		switch {
		case fs != nil:
			s.aud = audit.NewService(fs.Audit(), ps, audit.Options{Retention: retention})
		case cfg.CasesStore == config.CasesFile && cfg.AuditDir != "":
			as, err := audit.NewFile(cfg.AuditDir)
			if err != nil {
				return s, err
			}
			s.aud = audit.NewService(as, ps, audit.Options{Retention: retention})
		default:
			return s, errors.New("AUDIT=on needs CASES_STORE=firestore, or file with AUDIT_DIR: an in-memory trail would be lost")
		}
	}
	if cfg.Webhooks {
		switch {
		case fs != nil:
			hooks = fs.Webhooks()
		case cfg.CasesStore == config.CasesFile && cfg.CasesDir != "":
			f, err := webhooks.NewFile(filepath.Join(cfg.CasesDir, "webhooks"))
			if err != nil {
				return s, err
			}
			hooks = f
		default:
			return s, errors.New("WEBHOOKS=on needs CASES_STORE=firestore or file: in-memory endpoints are not shared with the server")
		}
		sealer, err := envelope.NewSealer([]byte(cfg.WebhooksSecretKey), webhooks.SealPurpose)
		if err != nil {
			return s, err
		}
		s.hook = webhooks.New(webhooks.NewSealedStore(hooks, sealer), webhooks.Options{})
	}
	return s, nil
}

// resultFile is the part of result.json this command reads.
type resultFile struct {
	CBP struct {
		Correct int    `json:"correct"`
		Cases   int    `json:"cases"`
		Run     string `json:"run"`
	} `json:"cbp"`
	Japan struct {
		Correct int `json:"correct"`
		Cases   int `json:"cases"`
	} `json:"japan"`
}

func run(ctx context.Context, cfg config.Config, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: evaldrift record|check|emit [-dir DIR] [-month YYYY-MM]")
	}
	fl := flag.NewFlagSet(args[0], flag.ContinueOnError)
	dir := fl.String("dir", "../eval/rescore", "the eval/rescore directory")
	month := fl.String("month", "", "the month, YYYY-MM")
	if err := fl.Parse(args[1:]); err != nil {
		return err
	}
	histPath, reportPath := filepath.Join(*dir, "history.json"), filepath.Join(*dir, "drift.json")
	h, err := evaldrift.Load(histPath)
	if err != nil {
		return err
	}
	switch args[0] {
	case "record":
		if *month == "" {
			return errors.New("record needs -month")
		}
		b, err := os.ReadFile(filepath.Join(*dir, *month, "result.json"))
		if err != nil {
			return err
		}
		var r resultFile
		if err := json.Unmarshal(b, &r); err != nil {
			return fmt.Errorf("%s result.json: %w", *month, err)
		}
		for _, s := range []struct {
			name           string
			correct, cases int
			run            string
		}{{"cbp", r.CBP.Correct, r.CBP.Cases, r.CBP.Run}, {"japan", r.Japan.Correct, r.Japan.Cases, ""}} {
			if s.cases == 0 {
				fmt.Fprintf(out, "%s %s: scored nothing; nothing recorded\n", s.name, *month)
				continue
			}
			if _, err := h.Record(s.name, evaldrift.Point{ID: *month, Kind: evaldrift.KindMonthly, Correct: s.correct, Cases: s.cases, Run: s.run}); err != nil {
				return err
			}
		}
		if err := h.Save(histPath); err != nil {
			return err
		}
		rep := evaldrift.Compute(h, h.Rule)
		if err := evaldrift.SaveReport(reportPath, rep); err != nil {
			return err
		}
		printMonth(out, rep, *month)
		return nil
	case "check":
		rep := evaldrift.Compute(h, h.Rule)
		want, _ := json.MarshalIndent(rep, "", " ")
		got, err := os.ReadFile(reportPath)
		if err != nil {
			return err
		}
		if string(got) != string(want)+"\n" {
			return fmt.Errorf("%s differs from a recomputation of %s: run evaldrift record", reportPath, histPath)
		}
		printMonth(out, rep, *month)
		return nil
	case "emit":
		if *month == "" {
			return errors.New("emit needs -month")
		}
		if !cfg.EvalDriftAlerts {
			return errors.New("EVAL_DRIFT_ALERTS is off: nothing sent (record and check need no flag)")
		}
		rep := evaldrift.Compute(h, h.Rule)
		var alerts []evaldrift.Result
		for _, a := range rep.Alerts() {
			if a.Month == *month {
				alerts = append(alerts, a)
			}
		}
		if len(alerts) == 0 {
			fmt.Fprintf(out, "no drift in %s: nothing sent\n", *month)
			return nil
		}
		s, err := openSinks(ctx, cfg)
		if err != nil {
			return err
		}
		for _, a := range alerts {
			if s.aud != nil {
				_, err := s.aud.Record(ctx, audit.Draft{Event: audit.Event{Tenant: "platform", ActorKind: audit.ActorSystem, Action: audit.EvalDrift,
					Phase: audit.PhaseSingle, Outcome: audit.Allowed, Detail: a.Series, Object: "evalmonth:" + a.Series + ":" + a.Month}})
				if err != nil {
					return fmt.Errorf("audit: %w", err)
				}
				fmt.Fprintf(out, "audit event recorded for %s %s\n", a.Series, a.Month)
			}
			if s.hook != nil {
				n := s.hook.EmitPlatform(ctx, webhooks.EventEvalDrift, payload(a, rep.Rule))
				fmt.Fprintf(out, "eval.drift queued for %d endpoint(s) for %s %s\n", n, a.Series, a.Month)
			}
		}
		if s.hook != nil {
			s.hook.Wait()
		}
		if s.aud == nil && s.hook == nil {
			fmt.Fprintln(out, "AUDIT and WEBHOOKS are both off: the alert went nowhere")
		}
		return nil
	}
	return fmt.Errorf("unknown command %q", args[0])
}

func printMonth(out io.Writer, rep evaldrift.Report, month string) {
	for _, r := range rep.Results {
		if month == "" || r.Month == month {
			fmt.Fprintln(out, r.Line())
		}
	}
}

// payload is what a webhook carries: figures and the rule, no case or ruling text.
func payload(a evaldrift.Result, r evaldrift.Rule) map[string]any {
	return map[string]any{
		"series": a.Series, "month": a.Month,
		"correct": a.ThisMonth.Correct, "cases": a.ThisMonth.Cases, "accuracy": a.ThisMonth.Accuracy,
		"baseline_correct": a.Baseline.Correct, "baseline_cases": a.Baseline.Cases, "baseline_accuracy": a.Baseline.Accuracy,
		"p_value": a.PValue, "severity": a.Severity, "rule": r,
	}
}
