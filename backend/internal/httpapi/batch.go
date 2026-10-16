package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/bom"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

// MaxBatch caps a portfolio triage request; BatchWorkers is how many cases
// are analysed at once.
const (
	MaxBatch     = 20
	BatchWorkers = 4
)

type batchItem struct {
	Name  string `json:"name"`
	Facts string `json:"facts"`
	Date  string `json:"date"` // YYYY-MM-DD; default today
}

// BatchRow is one product's triage outcome.
type BatchRow struct {
	Name       string   `json:"name"`
	CaseID     string   `json:"case_id,omitempty"`
	Origin     string   `json:"predicted_origin,omitempty"`
	Confidence *float64 `json:"confidence,omitempty"`
	Route      string   `json:"triage_route,omitempty"`
	OutOfScope bool     `json:"out_of_scope"`
	Reason     string   `json:"reason,omitempty"`
	CostUSD    float64  `json:"cost_usd,omitempty"`
	Error      *string  `json:"error"` // null when the case was analysed
}

// batch triages a portfolio (D47): up to MaxBatch products, each by its
// facts, analysed in parallel and saved as ordinary cases. One product's
// failure does not fail the rest. Accepts JSON {items: [{name, facts, date}]}
// or multipart/form-data with a "file" (CSV or .xlsx: name, facts or
// description, optional date).
func (a *api) batch(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	items, err := a.batchItems(w, r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if len(items) == 0 || len(items) > MaxBatch {
		a.fail(w, r, fmt.Errorf("%w: give 1 to %d products (got %d)", domain.ErrInvalid, MaxBatch, len(items)))
		return
	}
	today := time.Now().UTC().Format(time.DateOnly)
	rows := make([]BatchRow, len(items))
	sem := make(chan struct{}, BatchWorkers)
	var wg sync.WaitGroup
	for i, it := range items {
		rows[i].Name = strings.TrimSpace(it.Name)
		if rows[i].Name == "" {
			rows[i].Name = firstWords(it.Facts, 8)
		}
		wg.Add(1)
		go func(i int, it batchItem) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			ds := strings.TrimSpace(it.Date)
			if ds == "" {
				ds = today
			}
			d, err := time.Parse(time.DateOnly, ds)
			if err != nil {
				msg := fmt.Sprintf("date %q is not YYYY-MM-DD", it.Date)
				rows[i].Error = &msg
				return
			}
			sc, err := a.createCase(r, predictRequest{Date: planner.Date{Time: d}, Facts: it.Facts}, time.Now())
			if err != nil {
				_, _, msg := a.errorFor(r, err)
				rows[i].Error = &msg
				return
			}
			conf := sc.Prediction.Confidence
			rows[i].CaseID, rows[i].Origin, rows[i].Confidence = sc.ID, sc.Prediction.Origin, &conf
			rows[i].Route, rows[i].Reason = string(sc.Triage.Route), sc.Triage.Reason
			rows[i].OutOfScope = strings.HasPrefix(sc.Triage.Reason, "out of scope")
			if n := len(sc.History); n > 0 {
				rows[i].CostUSD = sc.History[n-1].CostUSD
			}
		}(i, it)
	}
	wg.Wait()

	summary := map[string]any{"total": len(rows)}
	counts := map[string]int{string(domain.RouteAuto): 0, string(domain.RouteNeedsReview): 0, string(domain.RouteNeedsInfo): 0}
	outOfScope, failed, cost := 0, 0, 0.0
	for _, row := range rows {
		switch {
		case row.Error != nil:
			failed++
		default:
			counts[row.Route]++
			if row.OutOfScope {
				outOfScope++
			}
			cost += row.CostUSD
		}
	}
	for k, v := range counts {
		summary[k] = v
	}
	summary["out_of_scope"], summary["failed"] = outOfScope, failed
	summary["cost_usd"] = float64(int64(cost*1e6+0.5)) / 1e6
	summary["latency_ms"] = time.Since(start).Milliseconds()
	a.respond(w, http.StatusOK, map[string]any{"rows": rows, "summary": summary})
}

func (a *api) batchItems(w http.ResponseWriter, r *http.Request) ([]batchItem, error) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		var in struct {
			Items []batchItem `json:"items"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxUpload))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in); err != nil {
			return nil, fmt.Errorf("%w: request body: %v", domain.ErrInvalid, err)
		}
		return in.Items, nil
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		return nil, fmt.Errorf("%w: upload: %v", domain.ErrInvalid, err)
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("%w: a file field with the CSV or Excel sheet is required", domain.ErrInvalid)
	}
	defer f.Close()
	return batchRows(f)
}

// batchRows reads a sheet with a header row: name, facts (or description),
// optional date.
func batchRows(src io.Reader) ([]batchItem, error) {
	rows, err := bom.ReadRows(src)
	if err != nil {
		return nil, err
	}
	for len(rows) > 0 && strings.TrimSpace(strings.Join(rows[0], "")) == "" {
		rows = rows[1:]
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("%w: the sheet needs a header row and at least one product row", domain.ErrInvalid)
	}
	col := map[string]int{}
	for i, h := range rows[0] {
		switch strings.ToLower(strings.TrimSpace(h)) {
		case "name", "product", "product name", "sku":
			col["name"] = i
		case "facts", "description", "product description":
			col["facts"] = i
		case "date", "case date":
			col["date"] = i
		}
	}
	fi, ok := col["facts"]
	if !ok {
		return nil, fmt.Errorf("%w: header row needs a facts (or description) column (got %q)", domain.ErrInvalid, rows[0])
	}
	cell := func(row []string, key string) string {
		if i, ok := col[key]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	var items []batchItem
	for _, row := range rows[1:] {
		if fi >= len(row) || strings.TrimSpace(row[fi]) == "" {
			continue
		}
		items = append(items, batchItem{Name: cell(row, "name"), Facts: cell(row, "facts"), Date: cell(row, "date")})
	}
	return items, nil
}
