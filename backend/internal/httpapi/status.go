package httpapi

import (
	"context"
	"net/http"
	"sort"
	"sync"
	"time"
)

// StatusPage serves GET /v1/status (#885): a public, cheap summary for a
// status page and uptime monitors. It never runs a check on the request
// path: it returns the last snapshot of the deep health checks and, when
// that is older than Every, starts one refresh in the background. So a
// monitor polling every few seconds costs at most one model call per Every.
// Error texts are not returned; each component is "operational",
// "degraded" or "unknown" (no snapshot yet).
type StatusPage struct {
	Checks []HealthCheck
	// Every is how often the checks may run; 5 minutes when zero.
	Every time.Duration
	// SLOs are published with the status (docs/design/sla-and-status.md).
	SLOs []SLO
	Now  func() time.Time

	mu       sync.Mutex
	snapshot *statusSnapshot
	running  bool
}

// SLO is one service-level objective as published.
type SLO struct {
	Name      string  `json:"name"`
	Objective float64 `json:"objective_percent"`
	Window    string  `json:"window"`
	Measures  string  `json:"measures"`
}

// DefaultSLOs are the objectives in docs/design/sla-and-status.md.
var DefaultSLOs = []SLO{
	{Name: "availability", Objective: 99.5, Window: "30d", Measures: "share of /v1 requests answered without a 5xx, excluding 503 from configured limits"},
	{Name: "case_read_latency", Objective: 99, Window: "30d", Measures: "share of GET /v1/cases/{id} answered in under 1 s"},
	{Name: "prediction_latency", Objective: 95, Window: "30d", Measures: "share of POST /v1/cases/predict answered in under 60 s"},
}

type statusSnapshot struct {
	at         time.Time
	components map[string]bool
}

type statusComponent struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func (s *StatusPage) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *StatusPage) every() time.Duration {
	if s.Every > 0 {
		return s.Every
	}
	return 5 * time.Minute
}

// refresh runs every check once and stores the snapshot.
func (s *StatusPage) refresh(ctx context.Context) {
	res := map[string]bool{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, c := range s.Checks {
		wg.Add(1)
		go func(c HealthCheck) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			err := c.Run(cctx)
			mu.Lock()
			res[c.Name] = err == nil
			mu.Unlock()
		}(c)
	}
	wg.Wait()
	s.mu.Lock()
	s.snapshot = &statusSnapshot{at: s.now(), components: res}
	s.running = false
	s.mu.Unlock()
}

// kick starts a background refresh when the snapshot is missing or stale.
func (s *StatusPage) kick() {
	s.mu.Lock()
	stale := s.snapshot == nil || s.now().Sub(s.snapshot.at) >= s.every()
	if !stale || s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()
	go s.refresh(context.Background())
}

// ServeHTTP implements http.Handler.
func (s *StatusPage) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	s.kick()
	s.mu.Lock()
	snap := s.snapshot
	s.mu.Unlock()
	overall := "unknown"
	comps := []statusComponent{}
	body := map[string]any{"slos": s.SLOs}
	if snap != nil {
		overall = "operational"
		for name, ok := range snap.components {
			st := "operational"
			if !ok {
				st, overall = "degraded", "degraded"
			}
			comps = append(comps, statusComponent{Name: name, Status: st})
		}
		sort.Slice(comps, func(i, j int) bool { return comps[i].Name < comps[j].Name })
		body["checked_at"] = snap.at.UTC().Format(time.RFC3339)
	} else {
		for _, c := range s.Checks {
			comps = append(comps, statusComponent{Name: c.Name, Status: "unknown"})
		}
	}
	body["status"], body["components"] = overall, comps
	writeJSON(w, http.StatusOK, body)
}
