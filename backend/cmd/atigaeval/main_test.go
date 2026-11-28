package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrozenEvaluationMatchesCommittedReport(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "eval", "atiga")
	data, err := os.ReadFile(filepath.Join(dir, "evaluation-cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := evaluate(data)
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(dir, "evaluation-results.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(append(got, '\n'), want) {
		t.Fatal("committed ATIGA report differs from the frozen dataset and current engine")
	}
	var doc struct {
		BaselineSHA256 string         `json:"baseline_sha256"`
		Cases          []evidenceCase `json:"cases"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	base, err := os.ReadFile(filepath.Join(dir, "reference-cases.json"))
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(base)) != doc.BaselineSHA256 {
		t.Fatalf("original authority dataset changed: %v", err)
	}
	var original struct {
		Cases []evidenceCase `json:"cases"`
	}
	if err := json.Unmarshal(base, &original); err != nil {
		t.Fatal(err)
	}
	authority := map[string]evidenceCase{}
	for _, c := range original.Cases {
		authority[c.ID] = c
	}
	found := 0
	for _, c := range doc.Cases {
		o, exists := authority[c.ID]
		if c.Kind != "authority_example" {
			if exists {
				t.Errorf("authority example %s was recategorized", c.ID)
			}
			continue
		}
		found++
		got, _ := json.Marshal(c.Input)
		want, _ := json.Marshal(o.Input)
		gotExpected, _ := json.Marshal(c.Expect)
		wantExpected, _ := json.Marshal(o.Expect)
		if !exists || c.Source != o.Source || !bytes.Equal(got, want) || !bytes.Equal(gotExpected, wantExpected) {
			t.Errorf("authority example %s is new or changed", c.ID)
		}
	}
	if found != len(original.Cases) {
		t.Fatalf("authority group has %d cases, want %d", found, len(original.Cases))
	}
}

func TestEvaluateCountsEvidenceSeparately(t *testing.T) {
	data := []byte(`{"sources":{"treaty":{"url":"https://asean.org/"}},"cases":[
	{"id":"approved","kind":"authority_example","source":"treaty","input":{"hs":"9403.60","produced_in":"Malaysia","fob":100,"materials":[{"hs":"9403.60","country":"China","value":60}]},"expect":{"outcome":"yes"}},
	{"id":"refused","kind":"synthetic_control","source":"treaty","input":{"hs":"9403.60","produced_in":"Malaysia","fob":100,"materials":[{"hs":"9403.60","country":"China","value":61}]},"expect":{"outcome":"no","alternative":{"RVC40":"no"}}}
	]}`)
	r, err := evaluate(data)
	if err != nil {
		t.Fatal(err)
	}
	if r.DatasetSHA256 != fmt.Sprintf("%x", sha256.Sum256(data)) || r.Total.Cases != 2 || r.Total.Passed != 2 || r.Total.Failed != 0 {
		t.Fatalf("report = %+v", r)
	}
	for _, kind := range []string{"authority_example", "synthetic_control"} {
		if r.Groups[kind].Cases != 1 || r.Groups[kind].Passed != 1 {
			t.Errorf("group %s = %+v", kind, r.Groups[kind])
		}
	}
}

func TestEvaluateRetainsFailures(t *testing.T) {
	r, err := evaluate([]byte(`{"sources":{"treaty":{"url":"https://asean.org/"}},"cases":[{"id":"wrong-label","kind":"synthetic_control","source":"treaty","input":{"hs":"9403.60","produced_in":"Malaysia","wholly_obtained":true},"expect":{"outcome":"no","alternative":{"RVC40":"yes"}}}]}`))
	if err != nil || r.Total.Failed != 1 || r.Total.Passed != 0 || len(r.Results) != 1 || r.Results[0].Passed || len(r.Results[0].Failures) != 2 {
		t.Fatalf("failed case lost: %+v %v", r, err)
	}
}

func TestEvaluateRejectsUnscorableEvidence(t *testing.T) {
	good := `{"sources":{"treaty":{"url":"https://asean.org/"}},"cases":[{"id":"a","kind":"synthetic_control","source":"treaty","input":{"hs":"9403.60","produced_in":"Malaysia","wholly_obtained":true},"expect":{"outcome":"yes"}}]}`
	for name, raw := range map[string]string{
		"empty":           `{"cases":[]}`,
		"unknown-kind":    strings.Replace(good, "synthetic_control", "real_world_accuracy", 1),
		"unknown-source":  strings.Replace(good, `"source":"treaty"`, `"source":"missing"`, 1),
		"missing-label":   strings.Replace(good, `"expect":{"outcome":"yes"}`, `"expect":{}`, 1),
		"bad-label":       strings.Replace(good, `"expect":{"outcome":"yes"}`, `"expect":{"outcome":"maybe"}`, 1),
		"malformed-input": strings.Replace(good, `"hs":"9403.60"`, `"hs":"bad"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := evaluate([]byte(raw)); err == nil {
				t.Error("unscorable evidence accepted")
			}
		})
	}
}
