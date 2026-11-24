package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpcorpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpengine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jprules"
)

// rulesDataDir holds the embedded rule sources hashed into a diagnosis.
const rulesDataDir = "internal/jprules/data"

const diagnoseBasis = "Counts describe rule availability before any model call: whether a verified rule " +
	"source can be shown for a case's agreement, code and date. They are not accuracy, recall, " +
	"resolved decisions or entitlement conclusions. A mapped case shows the rule row of the code's " +
	"earlier-edition parent codes behind an administrative-correlation note; the rule is not listed for " +
	"the code itself. No labels, scoring, precedents or model are used."

type diagnoseEligible struct {
	Total   int `json:"total"`
	PSR     int `json:"psr"`
	General int `json:"general"`
	Mapped  int `json:"mapped"`
}

type diagnoseProvenance struct {
	RealInference      bool   `json:"real_inference"`
	GatewayInitialized bool   `json:"gateway_initialized"`
	Basis              string `json:"basis"`
}

type diagnoseCandidate struct {
	GitRevision string            `json:"git_revision"`
	CodeDirty   bool              `json:"code_dirty"`
	Files       map[string]string `json:"file_sha256"`
}

// diagnoseReport is the offline rule-availability breakdown of a development
// split. It holds no label, prediction or model output.
type diagnoseReport struct {
	Split           string                    `json:"split"`
	Cases           int                       `json:"cases"`
	DevEvalOverlap  *int                      `json:"dev_eval_overlap,omitempty"`
	Eligible        diagnoseEligible          `json:"eligible"`
	Abstained       int                       `json:"abstained"`
	ReasonByAgree   map[string]map[string]int `json:"reason_by_agreement"`
	UnmatchedCodes  map[string]map[string]int `json:"unmatched_codes"`
	Candidate       diagnoseCandidate         `json:"candidate"`
	Provenance      diagnoseProvenance        `json:"provenance"`
	eligibleCaseIDs []string
}

// diagnoseSplit refuses every split but the development ones. It runs before
// any file is opened.
func diagnoseSplit(split string) error {
	if split != "dev_eval" && split != "dev" {
		return fmt.Errorf("split %q refused: diagnose reads development splits only (dev_eval, dev)", split)
	}
	return nil
}

// diagnoseCases builds one case per declared id from corpus facts alone: the
// ruling's agreement, code, date and translated facts. Rows outside ids are
// not read and nothing of them is kept.
func diagnoseCases(c *jpcorpus.Corpus, ids []string) ([]domain.Case, error) {
	out := make([]domain.Case, 0, len(ids))
	for _, id := range ids {
		r, ok := c.Ruling(id)
		if !ok {
			return nil, fmt.Errorf("case %s is not in the corpus", id)
		}
		d, err := r.Time()
		if err != nil {
			return nil, err
		}
		facts, err := c.CaseFacts(id)
		if err != nil {
			facts = "" // no translation: preflight reports no_facts
		}
		out = append(out, domain.Case{ID: jpcorpus.PrecedentID(id), Date: d, Facts: facts,
			Japan: &domain.JapanInput{Agreement: r.Agreement, HS: r.HS}})
	}
	return out, nil
}

func diagnoseCaseSet(split string, sp jpcorpus.Splits) []string {
	if split == "dev" {
		return sp.Dev
	}
	return sp.DevEval
}

func bump(m map[string]map[string]int, a, k string) {
	if m[a] == nil {
		m[a] = map[string]int{}
	}
	m[a][k]++
}

// diagnoseRun counts preflight outcomes for the cases. It uses no model.
func diagnoseRun(split string, cases []domain.Case, rules *jprules.Table) diagnoseReport {
	r := diagnoseReport{Split: split, Cases: len(cases), ReasonByAgree: map[string]map[string]int{},
		UnmatchedCodes: map[string]map[string]int{},
		Provenance:     diagnoseProvenance{Basis: diagnoseBasis}}
	for _, c := range cases {
		pf := jpengine.Preflight(rules, c)
		agr := pf.Agreement
		if agr == "" {
			agr = "(none)"
		}
		bump(r.ReasonByAgree, agr, string(pf.Reason))
		switch {
		case pf.Eligible:
			r.Eligible.Total++
			r.eligibleCaseIDs = append(r.eligibleCaseIDs, c.ID)
			switch pf.Reason {
			case jprules.ReasonGeneral:
				r.Eligible.General++
			case jprules.ReasonMapped:
				r.Eligible.Mapped++
			default:
				r.Eligible.PSR++
			}
		default:
			r.Abstained++
		}
		switch pf.Reason {
		case jprules.ReasonNoMatch, jprules.ReasonTooBroad, jprules.ReasonWithheld, jprules.ReasonExcludedGeneral, jprules.ReasonEditionHint:
			bump(r.UnmatchedCodes, agr, pf.HS)
		}
	}
	return r
}

// diagnoseDevelopment loads the corpus from dir and diagnoses the split. The
// corpus load reads every record; only the declared development ids are used.
func diagnoseDevelopment(split, dir string, rules *jprules.Table) (diagnoseReport, error) {
	if err := diagnoseSplit(split); err != nil {
		return diagnoseReport{}, err
	}
	c, err := jpcorpus.Load(dir)
	if err != nil {
		return diagnoseReport{}, err
	}
	sp, err := jpcorpus.LoadSplits(filepath.Join(dir, "splits.json"))
	if err != nil {
		return diagnoseReport{}, err
	}
	ids := diagnoseCaseSet(split, sp)
	cases, err := diagnoseCases(c, ids)
	if err != nil {
		return diagnoseReport{}, err
	}
	r := diagnoseRun(split, cases, rules)
	if split == "dev" {
		n := 0
		for _, id := range sp.DevEval {
			if slices.Contains(sp.Dev, id) {
				n++
			}
		}
		r.DevEvalOverlap = &n
	}
	files := map[string]string{}
	for name, p := range map[string]string{
		"data/jp/rulings.jsonl":           filepath.Join(dir, "rulings.jsonl"),
		"data/jp/translations-en.jsonl":   filepath.Join(dir, "translations-en.jsonl"),
		"data/jp/splits.json":             filepath.Join(dir, "splits.json"),
		"jprules/data/schedules.json":     filepath.Join(rulesDataDir, "schedules.json"),
		"jprules/data/ajcep_general.json": filepath.Join(rulesDataDir, "ajcep_general.json"),
		"jprules/data/correlations.json":  filepath.Join(rulesDataDir, "correlations.json"),
	} {
		b, err := os.ReadFile(p)
		if err != nil {
			return diagnoseReport{}, err
		}
		files[name] = fmt.Sprintf("%x", sha256.Sum256(b))
	}
	r.Candidate = diagnoseCandidate{GitRevision: revision(), CodeDirty: codeDirty(), Files: files}
	return r, nil
}

// diagnoseCommand prints the offline rule-availability diagnosis of a
// development split as JSON. It reads no environment, builds no model client,
// opens no network connection and writes under data/ or eval/ never.
func diagnoseCommand(args []string) error {
	fs := flag.NewFlagSet("diagnose", flag.ExitOnError)
	split := fs.String("split", "dev_eval", "dev_eval or dev")
	out := fs.String("out", "", "also write the report to this new file (exclusive create, never under data/ or eval/)")
	_ = fs.Parse(args)
	if fs.NArg() != 0 {
		return errors.New("no positional arguments")
	}
	if err := diagnoseSplit(*split); err != nil {
		return err
	}
	if *out != "" {
		if err := safePlanPath(*out); err != nil {
			return err
		}
	}
	rules, err := jprules.Default()
	if err != nil {
		return err
	}
	r, err := diagnoseDevelopment(*split, dataDir, rules)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", " ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if *out != "" {
		f, err := os.OpenFile(*out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return errors.New("the report file cannot be created exclusively; choose a new path")
		}
		_, werr := f.Write(b)
		if err := errors.Join(werr, f.Close()); err != nil {
			return err
		}
	}
	_, err = os.Stdout.Write(b)
	return err
}
