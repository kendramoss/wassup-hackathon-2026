package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/webhooks"
)

// repo is an eval/rescore directory with the committed baselines and a month's result.json.
func repo(t *testing.T, cbpCorrect, cbpCases int) string {
	t.Helper()
	dir := t.TempDir()
	hist := `{"rule":{"alpha":0.01,"min_cases":20,"min_baseline":30},"series":{
 "cbp":[{"id":"cbp_final","kind":"baseline","correct":340,"cases":377},{"id":"cbp_final2","kind":"baseline","correct":77,"cases":83}],
 "japan":[{"id":"jp_fresh_1126","kind":"baseline","correct":37,"cases":44}]}}`
	write(t, filepath.Join(dir, "history.json"), hist)
	if err := os.MkdirAll(filepath.Join(dir, "2026-12"), 0o755); err != nil {
		t.Fatal(err)
	}
	res := map[string]any{"month": "2026-12",
		"cbp":   map[string]any{"correct": cbpCorrect, "cases": cbpCases, "run": "rescore-2026-12"},
		"japan": map[string]any{"correct": 0, "cases": 0}}
	b, _ := json.Marshal(res)
	write(t, filepath.Join(dir, "2026-12", "result.json"), string(b))
	return dir
}

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func do(t *testing.T, cfg config.Config, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := run(context.Background(), cfg, args, &out)
	return out.String(), err
}

func TestRecordThenCheckFlagsARealDropAndIsReproducible(t *testing.T) {
	dir := repo(t, 70, 100) // 70% against 90% earlier
	out, err := do(t, config.Config{}, "record", "-dir", dir, "-month", "2026-12")
	if err != nil || !strings.Contains(out, "DRIFT cbp 2026-12") {
		t.Fatalf("record: %v\n%s", err, out)
	}
	// The history now holds the month; the drift file holds the alert.
	var rep struct {
		Results []struct {
			Series, Month string
			Alert         bool
		}
	}
	b, _ := os.ReadFile(filepath.Join(dir, "drift.json"))
	if err := json.Unmarshal(b, &rep); err != nil || len(rep.Results) != 1 || !rep.Results[0].Alert || rep.Results[0].Series != "cbp" {
		t.Fatalf("drift.json: %s %v", b, err)
	}
	// Japan scored nothing this month: no point, no result.
	h, _ := os.ReadFile(filepath.Join(dir, "history.json"))
	if strings.Contains(string(h), `"2026-12"`) && strings.Count(string(h), `"2026-12"`) != 1 {
		t.Fatalf("history: %s", h)
	}
	// Offline recompute agrees with the committed file; a tampered file does not.
	if out, err := do(t, config.Config{}, "check", "-dir", dir); err != nil || !strings.Contains(out, "DRIFT cbp 2026-12") {
		t.Fatalf("check: %v\n%s", err, out)
	}
	write(t, filepath.Join(dir, "drift.json"), strings.Replace(string(b), `"alert": true`, `"alert": false`, 1))
	if _, err := do(t, config.Config{}, "check", "-dir", dir); err == nil {
		t.Fatal("a tampered drift.json passed the check")
	}
	// Recording again changes nothing.
	if _, err := do(t, config.Config{}, "record", "-dir", dir, "-month", "2026-12"); err != nil {
		t.Fatal(err)
	}
	if h2, _ := os.ReadFile(filepath.Join(dir, "history.json")); string(h2) != string(h) {
		t.Fatal("recording twice changed the history")
	}
}

func TestNoDropNoAlertLine(t *testing.T) {
	dir := repo(t, 92, 100)
	out, err := do(t, config.Config{}, "record", "-dir", dir, "-month", "2026-12")
	if err != nil || strings.Contains(out, "DRIFT") || !strings.Contains(out, "ok cbp 2026-12") {
		t.Fatalf("%v\n%s", err, out)
	}
}

func TestEmptyMonthRecordsNothing(t *testing.T) {
	dir := repo(t, 0, 0)
	out, err := do(t, config.Config{}, "record", "-dir", dir, "-month", "2026-12")
	if err != nil || !strings.Contains(out, "scored nothing") {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, err := do(t, config.Config{}, "check", "-dir", dir); err != nil {
		t.Fatal(err)
	}
}

type fakes struct {
	sinks
	hits *[]string
	mu   *sync.Mutex
}

func fakeSinks(t *testing.T) fakes {
	t.Helper()
	var mu sync.Mutex
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		hits = append(hits, string(b))
		mu.Unlock()
	}))
	t.Cleanup(srv.Close)
	st := webhooks.NewMemory()
	for _, ws := range []string{"ws_aaaaaaaaaaaa", "ws_bbbbbbbbbbbb"} {
		if err := st.PutEndpoint(context.Background(), webhooks.Endpoint{ID: webhooks.NewID("wh_"), Workspace: ws, URL: srv.URL, Events: []string{webhooks.EventEvalDrift}, Enabled: true, Secret: webhooks.NewSecret()}); err != nil {
			t.Fatal(err)
		}
	}
	ps, _ := audit.NewPseudonymizer("k1", []byte(strings.Repeat("k", 32)))
	return fakes{sinks{audit.NewService(audit.NewMemory(), ps, audit.Options{}), webhooks.New(st, webhooks.Options{AllowInsecure: true, Client: http.DefaultClient})}, &hits, &mu}
}

func TestEmitIsOffByDefaultThenWritesAuditAndWebhooks(t *testing.T) {
	dir := repo(t, 70, 100)
	if _, err := do(t, config.Config{}, "record", "-dir", dir, "-month", "2026-12"); err != nil {
		t.Fatal(err)
	}
	s := fakeSinks(t)
	old := openSinks
	openSinks = func(context.Context, config.Config) (sinks, error) { return s.sinks, nil }
	defer func() { openSinks = old }()

	// Flag off: refuses, writes nothing.
	if _, err := do(t, config.Config{Audit: true, Webhooks: true}, "emit", "-dir", dir, "-month", "2026-12"); err == nil || !strings.Contains(err.Error(), "EVAL_DRIFT_ALERTS") {
		t.Fatalf("emit with the flag off: %v", err)
	}
	if evs, _ := s.aud.List(context.Background(), "platform", audit.ListQuery{Limit: 10}); len(evs.Events) != 0 || len(*s.hits) != 0 {
		t.Fatal("something was written with the flag off")
	}

	cfg := config.Config{EvalDriftAlerts: true, Audit: true, Webhooks: true}
	out, err := do(t, cfg, "emit", "-dir", dir, "-month", "2026-12")
	if err != nil {
		t.Fatalf("emit: %v\n%s", err, out)
	}
	s.hook.Wait()
	evs, err := s.aud.List(context.Background(), "platform", audit.ListQuery{Limit: 10})
	if err != nil || len(evs.Events) != 1 {
		t.Fatalf("audit: %+v %v", evs, err)
	}
	e := evs.Events[0]
	if e.Action != audit.EvalDrift || e.ActorKind != audit.ActorSystem || e.Actor != "" || e.Object != "evalmonth:cbp:2026-12" || e.Detail != "cbp" || e.Outcome != audit.Allowed {
		t.Fatalf("event: %+v", e)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(*s.hits) != 2 {
		t.Fatalf("webhooks received %d, want one per workspace", len(*s.hits))
	}
	var env webhooks.Envelope
	if err := json.Unmarshal([]byte((*s.hits)[0]), &env); err != nil || env.Type != "eval.drift" {
		t.Fatalf("envelope: %v %s", err, (*s.hits)[0])
	}
	data := env.Data.(map[string]any)
	if data["series"] != "cbp" || data["month"] != "2026-12" || data["cases"] != float64(100) || data["severity"] == nil {
		t.Fatalf("data: %v", data)
	}
	// Payload holds figures only.
	for _, banned := range []string{"case_", "ruling", "@"} {
		if strings.Contains((*s.hits)[0], banned) {
			t.Errorf("payload contains %q: %s", banned, (*s.hits)[0])
		}
	}
}

func TestEmitSendsNothingWhenThereIsNoAlert(t *testing.T) {
	dir := repo(t, 92, 100)
	if _, err := do(t, config.Config{}, "record", "-dir", dir, "-month", "2026-12"); err != nil {
		t.Fatal(err)
	}
	s := fakeSinks(t)
	old := openSinks
	openSinks = func(context.Context, config.Config) (sinks, error) { return s.sinks, nil }
	defer func() { openSinks = old }()
	out, err := do(t, config.Config{EvalDriftAlerts: true, Audit: true, Webhooks: true}, "emit", "-dir", dir, "-month", "2026-12")
	if err != nil || !strings.Contains(out, "no drift") {
		t.Fatalf("%v\n%s", err, out)
	}
	s.hook.Wait()
	if evs, _ := s.aud.List(context.Background(), "platform", audit.ListQuery{Limit: 10}); len(evs.Events) != 0 || len(*s.hits) != 0 {
		t.Fatal("an alert was sent without a drop")
	}
}
