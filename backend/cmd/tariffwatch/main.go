// Command tariffwatch re-checks saved cases against a newer HTS revision's
// tariff data (#917, D938).
//
// Usage:
//
//	go run ./cmd/tariffwatch check -dataset rev.json [-as-of YYYY-MM-DD] [-workspace ws_...] [-apply] [-all] [-limit N]
//	go run ./cmd/tariffwatch dataset -new ch99-new.txt [-old ch99-old.txt] [-base base.json] [-hts-json export.json]
//	go run ./cmd/tariffwatch eval [-write]
//
// check reads the same environment as the server (CASES_STORE, CASES_DIR,
// FIRESTORE_DATABASE) and needs TARIFF_WATCH=on. For every case a plan
// search priced (the case carries the duty basis it was priced on) it diffs
// that basis against the data set for the entry day -as-of (default today,
// UTC) and records what changed, the old and new duty rate and the change in
// the case's annual duty figure and its plan's duty effect. Without -apply it
// is a dry run: it prints the report and writes nothing. With -apply it
// stores each case's result (replacing its last one, including a result of
// no change); the case's other fields and its update time are not touched.
// Pinned demo cases are never written. The report lists the affected cases;
// -all lists every case checked.
//
// dataset makes the data set of a newer HTS revision from the text of the
// Chapter 99 PDFs of the older and newer revisions (extracted with pdftotext
// or any extractor that keeps line order), starting from the data the server
// prices from today (or -base). It reads and writes files only; nothing
// calls the network. -hts-json adds the MFN lines of a USITC HTS JSON export
// of the newer revision.
//
// eval is the D941 accuracy test: it reads the notice texts in
// eval/tariffwatch and the Chapter 99 heading excerpts of Revisions 21, 22
// and 23, scores which China lines the loader says are affected and by how
// many points against the notices, and with -write stores
// eval/tariffwatch/summary.json. No model and no network.
//
// The operator runs it; nothing in the server does.
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

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffwatch"
)

func main() {
	var cfg config.Config
	var err error
	if len(os.Args) > 1 && os.Args[1] == "check" { // dataset and eval read files only
		cfg, err = config.Load()
	}
	if err == nil {
		err = run(context.Background(), cfg, os.Args[1:], os.Stdout, time.Now())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "tariffwatch:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config, args []string, out io.Writer, now time.Time) error {
	if len(args) == 0 {
		return errors.New("usage: tariffwatch check|dataset|eval [flags]")
	}
	switch args[0] {
	case "check":
		return check(ctx, cfg, args[1:], out, now)
	case "dataset":
		return buildDataset(args[1:], out)
	case "eval":
		return evalCmd(args[1:], out)
	}
	return fmt.Errorf("unknown command %q (want check, dataset or eval)", args[0])
}

// openCases opens the case store the server uses; an in-memory store has
// nothing to check.
var openCases = func(ctx context.Context, cfg config.Config) (store.Cases, error) {
	switch {
	case cfg.CasesStore == config.CasesFirestore:
		return store.NewFirestore(ctx, cfg.GCPProject, cfg.FirestoreDatabase)
	case cfg.CasesStore == config.CasesFile && cfg.CasesDir != "":
		return store.NewFile(cfg.CasesDir)
	}
	return nil, errors.New("tariffwatch needs CASES_STORE=firestore or file: an in-memory store has nothing to check")
}

type checked struct {
	CaseID string             `json:"case_id"`
	Name   string             `json:"name,omitempty"`
	HTS    string             `json:"hts"`
	Result tariffwatch.Result `json:"result"`
}

type report struct {
	Apply           bool      `json:"apply"`
	DatasetRevision string    `json:"dataset_revision"`
	AsOf            string    `json:"as_of"`
	CheckedAt       time.Time `json:"checked_at"`
	Checked         int       `json:"checked"`
	Affected        int       `json:"affected"`
	// NoBasis counts the cases looked at that no plan search priced, so
	// there is no basis to re-check.
	NoBasis int       `json:"no_basis"`
	Written int       `json:"written"`
	Cases   []checked `json:"cases"`
}

func check(ctx context.Context, cfg config.Config, args []string, out io.Writer, now time.Time) error {
	fl := flag.NewFlagSet("check", flag.ContinueOnError)
	dsPath := fl.String("dataset", "", "the newer HTS revision's data set (JSON)")
	asOfS := fl.String("as-of", "", "entry day to check, YYYY-MM-DD (default today, UTC)")
	ws := fl.String("workspace", "", "only this workspace")
	apply := fl.Bool("apply", false, "store the results (default: dry run)")
	all := fl.Bool("all", false, "list every case checked, not only the affected")
	limit := fl.Int("limit", 0, "stop after this many cases checked (0: none)")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if !cfg.TariffWatch {
		return errors.New("TARIFF_WATCH is off")
	}
	if *dsPath == "" {
		return errors.New("check needs -dataset (see the dataset command)")
	}
	asOf := now.UTC()
	if *asOfS != "" {
		t, err := time.Parse(time.DateOnly, *asOfS)
		if err != nil {
			return fmt.Errorf("-as-of %q: want YYYY-MM-DD", *asOfS)
		}
		asOf = t
	}
	ds, err := tariffwatch.LoadFile(*dsPath)
	if err != nil {
		return err
	}
	ev, err := tariffwatch.NewEvaluator(ds)
	if err != nil {
		return err
	}
	cs, err := openCases(ctx, cfg)
	if err != nil {
		return err
	}
	var list []store.Case
	if *ws != "" {
		list, err = store.ListWorkspace(ctx, cs, *ws)
	} else {
		list, err = cs.List(ctx)
	}
	if err != nil {
		return err
	}
	rep := report{Apply: *apply, DatasetRevision: ds.Revision, AsOf: asOf.Format(time.DateOnly), CheckedAt: now.UTC(), Cases: []checked{}}
	for _, c := range list {
		if c.Pinned {
			continue
		}
		if c.DutyBasis == nil {
			rep.NoBasis++
			continue
		}
		if *limit > 0 && rep.Checked >= *limit {
			break
		}
		res, err := ev.Watch(caseInput(c), asOf, now)
		if err != nil {
			return fmt.Errorf("case %s: %w", c.ID, err)
		}
		rep.Checked++
		if res.Affected {
			rep.Affected++
		}
		if *apply {
			if _, err := cs.Update(ctx, c.ID, func(latest *store.Case) error {
				if latest.DutyBasis == nil || latest.DutyBasis.PricedAt != c.DutyBasis.PricedAt {
					return errSuperseded // a plan search priced it again meanwhile
				}
				latest.TariffWatch = &res
				return nil
			}); err != nil && !errors.Is(err, errSuperseded) {
				return fmt.Errorf("case %s: %w", c.ID, err)
			} else if err == nil {
				rep.Written++
			}
		}
		if res.Affected || *all {
			row := checked{CaseID: c.ID, HTS: c.DutyBasis.HTS, Result: res}
			if c.Product != nil {
				row.Name = c.Product.Name
			}
			rep.Cases = append(rep.Cases, row)
		}
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", " ")
	return enc.Encode(rep)
}

var errSuperseded = errors.New("case was priced again during the check")

// caseInput is what Watch needs of a stored case: the basis, the origin of
// the current setup and the best shown plan.
func caseInput(c store.Case) tariffwatch.CaseInput {
	in := tariffwatch.CaseInput{Basis: *c.DutyBasis}
	switch {
	case c.Plan != nil && c.Plan.Baseline.Prediction.Origin != "":
		in.BaseOrigin = c.Plan.Baseline.Prediction.Origin
	case c.CurrentOrigin != nil:
		in.BaseOrigin = c.CurrentOrigin.Country
	default:
		in.BaseOrigin = c.Prediction.Origin
	}
	if c.Plan != nil && len(c.Plan.Plans) > 0 {
		p := c.Plan.Plans[0]
		in.Plan = &tariffwatch.PlanInput{To: p.Prediction.Origin, DutyEffectYear: p.DutyEffectYear}
	}
	return in
}

func buildDataset(args []string, out io.Writer) error {
	fl := flag.NewFlagSet("dataset", flag.ContinueOnError)
	newP := fl.String("new", "", "text of the newer revision's Chapter 99 PDF")
	oldP := fl.String("old", "", "text of the older revision's Chapter 99 PDF")
	baseP := fl.String("base", "", "data set to start from (default: the data the server prices from)")
	htsP := fl.String("hts-json", "", "USITC HTS JSON export of the newer revision, for its MFN lines")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if *newP == "" {
		return errors.New("dataset needs -new")
	}
	var base *tariffwatch.Dataset
	var err error
	if *baseP != "" {
		base, err = tariffwatch.LoadFile(*baseP)
	} else {
		base, err = tariffwatch.Embedded()
	}
	if err != nil {
		return err
	}
	newText, err := os.ReadFile(*newP)
	if err != nil {
		return err
	}
	var oldText []byte
	if *oldP != "" {
		if oldText, err = os.ReadFile(*oldP); err != nil {
			return err
		}
	}
	ds, rep, err := tariffwatch.Revise(base, string(oldText), string(newText))
	if err != nil {
		return err
	}
	if *htsP != "" {
		f, err := os.Open(*htsP)
		if err != nil {
			return err
		}
		defer f.Close()
		if ds.MFN, err = tariffwatch.MFNFromHTSJSON(f); err != nil {
			return err
		}
	}
	ds.Source = "Chapter 99 text of the older and newer HTS revision, applied to " + base.Revision + " data (cmd/tariffwatch dataset)"
	if err := ds.Validate(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "applied: %v\nnot modelled (headings that changed): %v\nwarnings: %v\n", rep.Applied, rep.NotModelled, rep.Warnings)
	enc := json.NewEncoder(out)
	enc.SetIndent("", " ")
	return enc.Encode(ds)
}
