package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
)

const substantial = "substantial_transformation"

type sseEvent struct {
	name string
	data map[string]any
}

// readEvents parses a server-sent event body (comments skipped).
func readEvents(t *testing.T, body string) []sseEvent {
	t.Helper()
	var out []sseEvent
	for _, block := range strings.Split(body, "\n\n") {
		var ev sseEvent
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event: "):
				ev.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev.data); err != nil {
					t.Fatalf("bad data %q: %v", line, err)
				}
			}
		}
		if ev.name != "" {
			out = append(out, ev)
		}
	}
	return out
}

// stageList is the stage events as "name status", in order.
func stageList(evs []sseEvent) []string {
	var out []string
	for _, e := range evs {
		if e.name == "stage" {
			out = append(out, e.data["name"].(string)+" "+e.data["status"].(string))
		}
	}
	return out
}

// realEngine is the engine over a fake model and a two-ruling index, so the
// precedents and engine stages come from the engine's own progress hook.
func realEngine(respond func(llm.Request) (llm.Response, error)) *engine.Engine {
	day := func(s string) time.Time { d, _ := time.Parse(time.DateOnly, s); return d }
	idx := retrieval.NewBM25([]domain.Ruling{
		{ID: "N350001", Date: day("2025-03-01"), Facts: "A hair dryer is assembled in Thailand; the motor is made in China.\n", Holding: "China."},
		{ID: "N350002", Date: day("2025-06-01"), Facts: "A heat gun motor, PCBA and fan are made in Thailand.\n", Holding: "Thailand."},
	}, engine.PrecedentText)
	return engine.New(&llm.Fake{Respond: respond}, idx, engine.Options{Model: "gemini-4-argon", K: 2, MaxOutputTokens: 8000})
}

func answerWith(conf string, wait time.Duration) func(llm.Request) (llm.Response, error) {
	return func(llm.Request) (llm.Response, error) {
		time.Sleep(wait)
		return llm.Response{Text: `{"origin_country":"China","essential_character":"motor","decisive_step":"motor in China","flip":null,"confidence":` + conf + `,"reasoning":"As N350001."}`}, nil
	}
}

func streamPredict(t *testing.T, hs harness, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/cases/predict", strings.NewReader(body))
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	return rec
}

func indexOf(xs []string, x string) int {
	for i, s := range xs {
		if s == x {
			return i
		}
	}
	return -1
}

func TestPredictStreamsStagesThenCase(t *testing.T) {
	keepAlive = 5 * time.Millisecond
	t.Cleanup(func() { keepAlive = 10 * time.Second })
	const facts = `{"date":"2026-01-01","facts":"A bicycle helmet: ABS shell moulded in Taiwan, assembled in China with a Chinese motor."}`
	tests := []struct {
		name string
		conf string
		want []string // stages that must appear, in this order
		none []string // stages that must not appear
	}{
		{"fast path: no clarify", "0.97", []string{"precedents started", "precedents done", "engine started", "engine done"}, []string{"clarify started"}},
		{"below the bar: clarify after the engine", "0.80", []string{"precedents started", "precedents done", "engine started", "engine done", "clarify started", "clarify done"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hs := newHarnessWith(t, substantial, func(d *Deps) {
				d.StreamPredict = true
				d.Engine = realEngine(answerWith(tt.conf, 30*time.Millisecond)) // lets a keep-alive through
			})
			rec := streamPredict(t, hs, facts)
			body := rec.Body.String()
			if rec.Code != 200 || rec.Header().Get("Content-Type") != "text/event-stream" {
				t.Fatalf("status %d type %q: %s", rec.Code, rec.Header().Get("Content-Type"), body)
			}
			if !strings.Contains(body, ": keep-alive\n\n") {
				t.Errorf("no keep-alive in %s", body)
			}
			evs := readEvents(t, body)
			got := stageList(evs)
			if len(got) == 0 || got[0] != "router started" || indexOf(got, "router done") < 0 {
				t.Errorf("router stages = %q", got)
			}
			at := -1
			for _, w := range tt.want {
				i := indexOf(got, w)
				if i <= at {
					t.Fatalf("stage %q out of order in %q", w, got)
				}
				at = i
			}
			for _, n := range tt.none {
				if indexOf(got, n) >= 0 {
					t.Errorf("unexpected %q in %q", n, got)
				}
			}
			for _, e := range evs {
				if e.name != "stage" {
					continue
				}
				ms, ok := e.data["ms"].(float64)
				if !ok || ms < 0 {
					t.Errorf("stage ms = %v", e.data["ms"])
				}
				if e.data["name"] == "engine" && e.data["status"] == "done" && ms < 30 {
					t.Errorf("engine done after %v ms, want at least the model's 30", ms)
				}
			}
			last := evs[len(evs)-1]
			if last.name != "done" {
				t.Fatalf("last event %q, want done: %s", last.name, body)
			}
			id, _ := last.data["id"].(string)
			if id == "" || last.data["prediction"] == nil || last.data["disclaimer"] != domain.Disclaimer || last.data["model_ids"] == nil {
				t.Errorf("done body = %v", last.data)
			}
			code, saved := hs.do(t, "GET", "/v1/cases/"+id, "")
			if code != 200 || saved["prediction"].(map[string]any)["origin_country"] != "China" {
				t.Errorf("streamed case not saved: %d %v", code, saved)
			}
			// The same request as JSON gives the same answer.
			code, plain := hs.do(t, "POST", "/v1/cases/predict", facts)
			if code != http.StatusCreated {
				t.Fatalf("json predict %d", code)
			}
			if plain["triage"].(map[string]any)["route"] != last.data["triage"].(map[string]any)["route"] ||
				(plain["clarify"] == nil) != (last.data["clarify"] == nil) {
				t.Errorf("stream and JSON differ: %v / %v", plain["triage"], last.data["triage"])
			}
		})
	}
}

func TestPredictStreamErrorPath(t *testing.T) {
	keepAlive = 5 * time.Millisecond
	t.Cleanup(func() { keepAlive = 10 * time.Second })
	hs := newHarnessWith(t, substantial, func(d *Deps) {
		d.StreamPredict = true
		d.Engine = realEngine(func(llm.Request) (llm.Response, error) { return llm.Response{}, llm.ErrUnavailable })
	})
	rec := streamPredict(t, hs, `{"date":"2026-01-01","facts":"A heat gun is assembled in Thailand."}`)
	evs := readEvents(t, rec.Body.String())
	if rec.Code != 200 || len(evs) == 0 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	last := evs[len(evs)-1]
	if last.name != "error" || last.data["code"] != "model_unavailable" || last.data["retryable"] != true || last.data["message"] == "" {
		t.Errorf("last event = %+v", last)
	}
	got := stageList(evs)
	if indexOf(got, "engine started") < 0 || indexOf(got, "engine done") >= 0 {
		t.Errorf("a failed engine reported done: %q", got)
	}
	if _, list := hs.do(t, "GET", "/v1/cases", ""); len(list["cases"].([]any)) != 0 {
		t.Errorf("a failed stream saved a case: %v", list["cases"])
	}

	// Input errors are plain JSON before the stream opens.
	rec = streamPredict(t, hs, `{"facts":"x: assembled in Thailand from Chinese parts."}`)
	var e map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &e)
	if rec.Code != http.StatusBadRequest || e["code"] != "invalid_input" || strings.HasPrefix(rec.Header().Get("Content-Type"), "text/event-stream") {
		t.Errorf("bad input: %d %q %v", rec.Code, rec.Header().Get("Content-Type"), e)
	}
}

func TestPredictStreamOffAnswersJSON(t *testing.T) {
	hs := newHarness(t, substantial) // StreamPredict off: the default
	rec := streamPredict(t, hs, `{"date":"2026-01-01","facts":"A heat gun is assembled in Thailand."}`)
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != http.StatusCreated || out["id"] == nil ||
		rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("flag off: %d %q %v", rec.Code, rec.Header().Get("Content-Type"), err)
	}
}

// slowRouter holds Classify until the engine has started, so the router's
// done is reported from its goroutine while the engine runs.
type slowRouter struct{ started chan struct{} }

func (s slowRouter) Classify(ctx context.Context, facts string) (router.Label, error) {
	select {
	case <-s.started:
	case <-ctx.Done():
	}
	return fakeRouter{substantial}.Classify(ctx, facts)
}

func TestPredictStreamRouterBesideEngine(t *testing.T) {
	started := make(chan struct{})
	var once sync.Once
	hs := newHarnessWith(t, substantial, func(d *Deps) {
		d.StreamPredict = true
		d.Router = slowRouter{started}
		d.Engine = realEngine(func(req llm.Request) (llm.Response, error) {
			once.Do(func() { close(started) })
			time.Sleep(20 * time.Millisecond)
			return answerWith("0.97", 0)(req)
		})
	})
	evs := readEvents(t, streamPredict(t, hs, `{"date":"2026-01-01","facts":"A heat gun is assembled in Thailand."}`).Body.String())
	got := stageList(evs)
	if indexOf(got, "router done") < indexOf(got, "engine started") || evs[len(evs)-1].name != "done" {
		t.Errorf("stages = %q, last %q", got, evs[len(evs)-1].name)
	}
}

// A cached prediction reports no stages of its own; the engine stage is still
// reported (started and done at once), so the list completes.
func TestStagesEndWithoutBegin(t *testing.T) {
	var got []string
	st := newStages(time.Now(), func(name, status string, _ int64) { got = append(got, name+" "+status) })
	st.begin("router")
	st.begin("router") // already running: no second event
	st.end(engine.StageEngine)
	st.end(engine.StageEngine) // already done: no second event
	st.settle()
	want := []string{"router started", "engine started", "engine done", "router done"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("events = %q, want %q", got, want)
	}
	var none *stages // the JSON path: every call is a no-op
	none.begin("router")
	none.end("router")
	none.settle()
}
