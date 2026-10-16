package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalsummary"
)

func writeSummaryFixture(t *testing.T, dir, name, provider string, confidence float64) {
	t.Helper()
	row := map[string]any{
		"ruling": "N1", "correct": true, "confidence": confidence,
		"llm_provider": provider, "latency_ms": 1200, "cost_usd": 0.008,
	}
	b, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeScopeFixture(t *testing.T, dir, name string, inScope bool) {
	t.Helper()
	b, err := json.Marshal(map[string]any{"ruling": "N1", "in_scope": inScope})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSelectSummaryRunPrefersVertexAndRefusesMixedLabels(t *testing.T) {
	dir := t.TempDir()
	proxyEngine := "final-gemini-4-argon-prec-engine-v1-litellm.jsonl"
	vertexEngine := "final-gemini-4-argon-prec-engine-v1-vertex.jsonl"
	proxyScope := "scope-final-router-v2.jsonl"
	vertexScope := "scope-final-router-v2-vertex.jsonl"
	for _, name := range []string{proxyEngine, proxyScope} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := selectSummaryRun(dir, "final")
	if err != nil || got.Official || got.File != proxyEngine || got.ScopeFile != proxyScope {
		t.Fatalf("proxy fallback = %+v %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(dir, vertexEngine), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := selectSummaryRun(dir, "final"); err == nil || !strings.Contains(err.Error(), vertexScope) {
		t.Fatalf("mixed vertex engine and proxy scope: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, vertexScope), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = selectSummaryRun(dir, "final")
	if err != nil || !got.Official || got.File != vertexEngine || got.ScopeFile != vertexScope {
		t.Fatalf("vertex = %+v %v", got, err)
	}
}

func TestSummaryCmdMarksVertexOfficial(t *testing.T) {
	dir := t.TempDir()
	for _, split := range []string{"final", "dev"} {
		writeSummaryFixture(t, dir, split+"-gemini-4-argon-prec-engine-v1-vertex.jsonl", "vertex", 0.96)
		writeScopeFixture(t, dir, "scope-"+split+"-router-v2-vertex.jsonl", true)
	}
	out := filepath.Join(dir, "summary.json")
	if err := summaryCmd([]string{"-results", dir, "-out", out}); err != nil {
		t.Fatal(err)
	}
	s := loadSummary(t, out)
	if !strings.Contains(s.Status, "official") || strings.Contains(s.Status, "produced through the LiteLLM proxy") {
		t.Fatalf("status = %q", s.Status)
	}
	if !strings.Contains(s.Status, "development set on Vertex AI") {
		t.Fatalf("status should keep the dev split labelled as the development set: %q", s.Status)
	}
	byKey := map[string]evalsummary.Item{}
	for _, it := range s.Items {
		byKey[it.Key] = it
	}
	finalAcc := byKey["final_accuracy"]
	if !strings.Contains(finalAcc.Source, "final-gemini-4-argon-prec-engine-v1-vertex.jsonl") || finalAcc.DevelopmentOnly {
		t.Fatalf("final accuracy = %+v", finalAcc)
	}
	fast := byKey["final_fastpath_accuracy"]
	if fast.N != 1 || !strings.Contains(fast.Source, "scope-final-router-v2-vertex.jsonl") || !strings.Contains(fast.Label, "0.92") {
		t.Fatalf("fast path = %+v", fast)
	}
	lat := byKey["final_latency_p50_s"]
	if !strings.Contains(lat.Label, "Vertex AI") || strings.Contains(lat.Label, "via proxy") {
		t.Fatalf("latency label = %q", lat.Label)
	}
	dev := byKey["dev_accuracy"]
	if !dev.DevelopmentOnly || !strings.Contains(dev.Source, "dev-gemini-4-argon-prec-engine-v1-vertex.jsonl") {
		t.Fatalf("dev accuracy = %+v", dev)
	}
}

func TestSummaryCmdKeepsProxyFallback(t *testing.T) {
	dir := t.TempDir()
	for _, split := range []string{"final", "dev"} {
		writeSummaryFixture(t, dir, split+"-gemini-4-argon-prec-engine-v1-litellm.jsonl", "litellm", 0.80)
		writeScopeFixture(t, dir, "scope-"+split+"-router-v2.jsonl", false)
	}
	out := filepath.Join(dir, "summary.json")
	if err := summaryCmd([]string{"-results", dir, "-out", out}); err != nil {
		t.Fatal(err)
	}
	s := loadSummary(t, out)
	if s.Status != summaryStatusProxy {
		t.Fatalf("status = %q", s.Status)
	}
	for _, it := range s.Items {
		if it.Key == "final_latency_p95_s" && !strings.Contains(it.Label, "via proxy") {
			t.Fatalf("proxy latency label = %q", it.Label)
		}
		if it.Key == "final_fastpath_coverage" && it.Value != 0 {
			t.Fatalf("out-of-scope row counted on the fast path: %+v", it)
		}
		if it.Key == "final_accuracy" && !strings.Contains(it.Source, "litellm.jsonl") {
			t.Fatalf("source = %s", it.Source)
		}
	}
}

func loadSummary(t *testing.T, path string) evalsummary.Summary {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var s evalsummary.Summary
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return s
}
