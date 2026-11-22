package main

// The recorded example on the site's /examples/revocation-alert page (judge
// panel round 10, item 2b): one committed development result row, N356467
// (an electric scooter, case date 2025-12-22), whose answer cited N344694;
// CBP modified N344694 in H344999 on 2026-05-06 (D481, D490). The box is the
// one a saved case shows, worked out with the same rule (domain.RulingAlerts)
// as of the count's fixed date. Nothing is called and nothing is re-checked:
//
//	go run ./cmd/corpus alerts-example [-check]   (in make reproduce)

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

const (
	alertExampleRuling = "N356467"
	alertExampleRun    = "eval/results/dev-gemini-4-argon-prec-engine-v1-litellm-reasoning-v1.jsonl"
	alertExampleOut    = "frontend/src/lib/revocation-example.json"
)

type alertExampleLabel struct {
	Ruling  string `json:"ruling"`
	Date    string `json:"date"`
	Subject string `json:"subject"`
	Origin  string `json:"primary_origin"`
	// Evidence is the holding sentence the label was read from (public
	// domain CBP text).
	Evidence []string `json:"evidence"`
}

type alertExampleFile struct {
	Note       string              `json:"note"`
	Ruling     string              `json:"ruling"`
	Subject    string              `json:"subject"`
	CaseDate   string              `json:"case_date"`
	Split      string              `json:"split"`
	Run        string              `json:"run"`
	Model      string              `json:"model"`
	Prompt     string              `json:"prompt"`
	Predicted  string              `json:"predicted_origin"`
	Confidence float64             `json:"confidence"`
	CBPOrigin  string              `json:"cbp_origin"`
	Cited      []string            `json:"cited"`
	Read       []string            `json:"read"`
	Reasoning  string              `json:"reasoning"`
	AsOf       string              `json:"as_of"`
	Alerts     map[string]any      `json:"ruling_alerts"`
	Changed    []alertExampleLabel `json:"changed_rulings"`
	ByRulings  []alertExampleLabel `json:"changed_by"`
	Counts     domain.AlertCounts  `json:"counts"`
}

func revocationAlertExample(args []string) error {
	fl := flag.NewFlagSet("alerts-example", flag.ExitOnError)
	root := fl.String("root", "..", "repository root")
	check := fl.Bool("check", false, "fail when the committed file is stale")
	_ = fl.Parse(args)
	asOf, _ := time.Parse(time.DateOnly, alertsAsOf)

	var row struct {
		Ruling     string   `json:"ruling"`
		Date       string   `json:"date"`
		Split      string   `json:"split"`
		Gold       string   `json:"gold"`
		Pred       string   `json:"pred"`
		Confidence float64  `json:"confidence"`
		Cited      []string `json:"cited"`
		Precedents []string `json:"precedents"`
		Reasoning  string   `json:"reasoning"`
		Model      string   `json:"model_id"`
		Prompt     string   `json:"prompt_version"`
	}
	f, err := os.Open(filepath.Join(*root, alertExampleRun))
	if err != nil {
		return err
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		if !bytes.Contains(sc.Bytes(), []byte(`"`+alertExampleRuling+`"`)) {
			continue
		}
		if err := json.Unmarshal(sc.Bytes(), &row); err == nil && row.Ruling == alertExampleRuling {
			break
		}
		row.Ruling = ""
	}
	f.Close()
	if row.Ruling != alertExampleRuling || row.Split != "dev" {
		return fmt.Errorf("%s: no development row in %s", alertExampleRuling, alertExampleRun)
	}

	// The same refs as the offline count (alerts.go): the row's own ruling,
	// the rulings it cited, the rest of the rulings it read.
	refs := []domain.RulingRef{{ID: row.Ruling, Role: domain.RoleSelf}}
	for _, id := range row.Cited {
		refs = append(refs, domain.RulingRef{ID: id, Role: domain.RoleCited})
	}
	for _, id := range row.Precedents {
		refs = append(refs, domain.RulingRef{ID: id, Role: domain.RoleRead})
	}
	cd, _ := time.Parse(time.DateOnly, row.Date)
	items := domain.RulingAlerts(cd, asOf, refs)
	counts := domain.CountAlerts(items)
	if counts.Final != 1 || counts.Proposed != 0 || len(items) != 1 || items[0].BeforeCase || !items[0].InEffect {
		return fmt.Errorf("%s no longer shows one in-force change since the case date: %+v", alertExampleRuling, items)
	}

	labels, err := alertExampleLabels(filepath.Join(*root, "eval", "data", "labels.jsonl"))
	if err != nil {
		return err
	}
	ex := alertExampleFile{
		Note:   "Recorded example, rebuilt from committed files by `cd backend && go run ./cmd/corpus alerts-example -check`. The answer is one committed development result row (no new model call). The box is worked out by the API's rule (domain.RulingAlerts, D480, D490) from the committed supersession map, as of " + alertsAsOf + ". Nothing is re-checked here.",
		Ruling: row.Ruling, Subject: labels[row.Ruling].Subject, CaseDate: row.Date, Split: row.Split, Run: alertExampleRun,
		Model: row.Model, Prompt: row.Prompt, Predicted: row.Pred, Confidence: row.Confidence, CBPOrigin: row.Gold,
		Cited: row.Cited, Read: row.Precedents, Reasoning: row.Reasoning, AsOf: alertsAsOf,
		Alerts: map[string]any{"counts": counts, "items": items},
	}
	for _, a := range items {
		ex.Changed = append(ex.Changed, labels[a.Ruling])
		ex.ByRulings = append(ex.ByRulings, labels[a.By])
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", " ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(ex); err != nil {
		return err
	}
	dest := filepath.Join(*root, alertExampleOut)
	if *check {
		old, err := os.ReadFile(dest)
		if err != nil || !bytes.Equal(old, buf.Bytes()) {
			return fmt.Errorf("%s is stale: run go run ./cmd/corpus alerts-example", alertExampleOut)
		}
		fmt.Printf("%s matches (%s: %s)\n", alertExampleOut, alertExampleRuling, items[0].Note)
		return nil
	}
	if err := os.WriteFile(dest, buf.Bytes(), 0o644); err != nil {
		return err
	}
	fmt.Printf("alerts-example %s: %s\n", alertExampleRuling, items[0].Note)
	return nil
}

func alertExampleLabels(path string) (map[string]alertExampleLabel, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]alertExampleLabel{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		var l alertExampleLabel
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			return nil, err
		}
		out[l.Ruling] = l
	}
	return out, sc.Err()
}
