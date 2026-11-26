package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/bom"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/erpimport"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// ERP import (#880, ERP_IMPORT). POST /v1/products/erp-import reads a BOM
// export from an ERP (generic CSV, SAP S/4HANA, NetSuite, or JSON items) and
// reports, per assembly, whether it is complete. With "commit": true it also
// files each complete assembly as a case and predicts it, as predict does.
//
// Re-importing is idempotent. Each assembly's case ID is derived from the
// caller's scope (owner and workspace), the source label and the ERP's own
// key for the assembly, so the same export lands on the same cases: an
// unchanged assembly is left alone (no model call), a changed one gets its
// new BOM and a new prediction ("bom_updated" in its history, as a BOM
// version does), and a new one is created. A dry run (commit false, the
// default) calls no model and says which of the three each would be.

// MaxERPCommit caps the assemblies one committed import predicts; larger
// exports are split by the caller (a dry run reads up to erpimport.MaxItems).
const MaxERPCommit = MaxBatch

type erpImportRequest struct {
	Format string `json:"format"`
	// Source names the ERP or export ("sap-prod"); it is part of each case's
	// key, so two ERPs with the same item numbers do not collide. Default:
	// the format.
	Source       string               `json:"source"`
	CSV          string               `json:"csv"`
	Items        []erpimport.JSONItem `json:"items"`
	Commit       bool                 `json:"commit"`
	Family       string               `json:"family"`
	Jurisdiction string               `json:"jurisdiction"`
}

// ERPRow is one assembly's outcome.
type ERPRow struct {
	ExternalID string `json:"external_id"`
	Name       string `json:"name"`
	// Status: "new", "changed" or "unchanged" on a dry run; "created",
	// "updated", "unchanged" or "failed" on a commit.
	Status       string   `json:"status"`
	CaseID       string   `json:"case_id"`
	Rows         []int    `json:"rows"`
	Origin       string   `json:"predicted_origin,omitempty"`
	Route        string   `json:"triage_route,omitempty"`
	Jurisdiction string   `json:"jurisdiction,omitempty"`
	CostUSD      float64  `json:"cost_usd,omitempty"`
	Error        *string  `json:"error"`
	Components   int      `json:"components"`
	HTS          string   `json:"hts,omitempty"`
	Countries    []string `json:"countries"`
}

var sourcePattern = strings.NewReplacer(" ", "-")

func (a *api) erpImport(w http.ResponseWriter, r *http.Request) {
	if !a.mayCreate(w, r) {
		return
	}
	start := time.Now()
	in, rows, err := a.erpImportInput(w, r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	format := strings.ToLower(strings.TrimSpace(in.Format))
	var res erpimport.Result
	if format == erpimport.JSON {
		res = erpimport.FromJSON(in.Items)
	} else {
		if res, err = erpimport.Parse(format, rows); err != nil {
			a.fail(w, r, fmt.Errorf("%w: %v", domain.ErrInvalid, err))
			return
		}
	}
	source := strings.ToLower(sourcePattern.Replace(strings.TrimSpace(in.Source)))
	if source == "" {
		source = format
	}
	if len(source) > 64 {
		a.fail(w, r, fmt.Errorf("%w: source is longer than 64 characters", domain.ErrInvalid))
		return
	}
	family := strings.TrimSpace(in.Family)
	if family != "" {
		if _, ok := a.d.Templates[family]; !ok {
			a.fail(w, r, fmt.Errorf("%w: family %q has no template; known: %s", domain.ErrInvalid, family, a.families()))
			return
		}
	}
	owner, workspace := a.principalOf(r).newCaseScope()
	if in.Commit {
		if owner == "" && workspace == "" {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "importing into cases needs sign-in or an API key, so re-imports find the same cases", nil)
			return
		}
		if len(res.Items) > MaxERPCommit {
			a.fail(w, r, fmt.Errorf("%w: %d complete assemblies; a committed import predicts at most %d, so split the export", domain.ErrInvalid, len(res.Items), MaxERPCommit))
			return
		}
		if err := a.auditBegin(r, auditStep{Tenant: tenantOfCase(store.Case{Owner: owner, Workspace: workspace}), Object: "batch"}); err != nil {
			a.fail(w, r, err)
			return
		}
	}
	out := make([]ERPRow, len(res.Items))
	sem := make(chan struct{}, BatchWorkers)
	var wg sync.WaitGroup
	for i, it := range res.Items {
		out[i] = ERPRow{ExternalID: it.ExternalID, Name: it.Name, Rows: it.Rows, HTS: it.HTS,
			CaseID: erpCaseID(owner, workspace, source, it.ExternalID), Components: len(it.Product.Components), Countries: productCountries(&it.Product)}
		wg.Add(1)
		go func(row *ERPRow, it erpimport.Item) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			a.erpImportOne(r, row, it, in, family, source, start)
		}(&out[i], it)
	}
	wg.Wait()
	summary := map[string]any{"assemblies": len(out), "problems": len(res.Problems), "lines": res.Lines}
	counts := map[string]int{}
	cost := 0.0
	for _, row := range out {
		counts[row.Status]++
		cost += row.CostUSD
	}
	for k, v := range counts {
		summary[k] = v
	}
	summary["cost_usd"] = float64(int64(cost*1e6+0.5)) / 1e6
	summary["latency_ms"] = time.Since(start).Milliseconds()
	a.respond(w, http.StatusOK, map[string]any{"format": res.Format, "source": source, "committed": in.Commit,
		"rows": out, "problems": res.Problems, "summary": summary})
}

// erpImportOne compares one assembly with its case and, on a commit, creates
// or updates it.
func (a *api) erpImportOne(r *http.Request, row *ERPRow, it erpimport.Item, in erpImportRequest, family, source string, start time.Time) {
	fail := func(err error) {
		_, _, msg := a.errorFor(r, err)
		row.Status, row.Error = "failed", &msg
	}
	existing, err := a.d.Cases.Get(r.Context(), row.CaseID)
	exists := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		fail(err)
		return
	}
	if exists {
		owner, workspace := a.principalOf(r).newCaseScope()
		if existing.Owner != owner || existing.Workspace != workspace || !a.visible(r, existing) {
			fail(fmt.Errorf("%w: the case for this assembly belongs to someone else", domain.ErrInvalid))
			return
		}
	}
	unchanged := exists && existing.Product != nil && erpimport.Fingerprint(*existing.Product) == erpimport.Fingerprint(it.Product)
	if !in.Commit {
		switch {
		case !exists:
			row.Status = "new"
		case unchanged:
			row.Status = "unchanged"
		default:
			row.Status = "changed"
		}
		return
	}
	if unchanged {
		row.Status = "unchanged"
		row.Origin, row.Route = existing.Prediction.Origin, string(existing.Triage.Route)
		return
	}
	p := it.Product
	req := predictRequest{Date: planner.Date{Time: time.Now().UTC().Truncate(24 * time.Hour)}, Product: &p, Family: family,
		DisplayName: it.Name, Jurisdiction: strings.TrimSpace(in.Jurisdiction)}
	if family == "" {
		req.Facts = erpimport.Facts(p)
	}
	sc, c, err := a.newCase(r, req)
	if err != nil {
		fail(err)
		return
	}
	sc.ID, c.ID = row.CaseID, row.CaseID
	if !exists {
		saved, err := a.analyseAndSave(r, sc, c, start, nil)
		if err != nil {
			fail(err)
			return
		}
		erpResult(row, "created", saved)
		return
	}
	next := sc
	if err := a.analyse(r, &next, c, nil); err != nil {
		fail(err)
		return
	}
	ev := predictionEvent("bom_updated", next, start)
	ev.Summary = fmt.Sprintf("BOM re-imported from %s (%s); %s", source, it.ExternalID, ev.Summary)
	saved, err := a.d.Cases.Update(r.Context(), row.CaseID, func(latest *store.Case) error {
		latest.Product, latest.Facts, latest.Family, latest.UpdatedAt = next.Product, next.Facts, next.Family, time.Now().UTC()
		latest.Jurisdiction = next.Jurisdiction
		latest.Prediction, latest.Router, latest.Triage, latest.Clarify = next.Prediction, next.Router, next.Triage, next.Clarify
		latest.Answers, latest.OriginalFacts = nil, ""
		clearDerived(latest)
		latest.CurrentOrigin = nil // a ruling on the previous BOM no longer applies
		record(r.Context(), latest, ev)
		return nil
	})
	if err != nil {
		fail(err)
		return
	}
	erpResult(row, "updated", saved)
}

func erpResult(row *ERPRow, status string, sc store.Case) {
	row.Status, row.Origin, row.Route = status, sc.Prediction.Origin, string(sc.Triage.Route)
	if sc.Jurisdiction != nil {
		row.Jurisdiction = sc.Jurisdiction.Code
	}
	if n := len(sc.History); n > 0 {
		row.CostUSD = sc.History[n-1].CostUSD
	}
}

// erpCaseID is the case an assembly lands on: the same scope, source and
// ERP key always give the same ID, in the case_<12 hex> form.
func erpCaseID(owner, workspace, source, externalID string) string {
	h := sha256.Sum256([]byte("erp-import:" + owner + "\x00" + workspace + "\x00" + source + "\x00" + externalID))
	return "case_" + hex.EncodeToString(h[:6])
}

// erpImportInput reads a JSON body, or a multipart upload with a "file" (CSV
// or .xlsx) and the other fields as form values.
func (a *api) erpImportInput(w http.ResponseWriter, r *http.Request) (erpImportRequest, [][]string, error) {
	var in erpImportRequest
	var src io.Reader
	if isMultipart(r) {
		r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
		if err := r.ParseMultipartForm(maxUpload); err != nil {
			return in, nil, fmt.Errorf("%w: upload: %v", domain.ErrInvalid, err)
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			return in, nil, fmt.Errorf("%w: a file field with the ERP export is required", domain.ErrInvalid)
		}
		defer f.Close()
		src = f
		in.Format, in.Source, in.Family, in.Jurisdiction = r.FormValue("format"), r.FormValue("source"), r.FormValue("family"), r.FormValue("jurisdiction")
		if v := r.FormValue("commit"); v != "" {
			b, err := strconv.ParseBool(v)
			if err != nil {
				return in, nil, fmt.Errorf("%w: commit must be true or false", domain.ErrInvalid)
			}
			in.Commit = b
		}
	} else {
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxUpload))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in); err != nil {
			return in, nil, fmt.Errorf("%w: request body: %v", domain.ErrInvalid, err)
		}
		src = strings.NewReader(in.CSV)
	}
	format := strings.ToLower(strings.TrimSpace(in.Format))
	known := false
	for _, f := range erpimport.Formats {
		known = known || f == format
	}
	if !known {
		return in, nil, fmt.Errorf("%w: format must be one of %s", domain.ErrInvalid, strings.Join(erpimport.Formats, ", "))
	}
	if format == erpimport.JSON {
		if len(in.Items) == 0 {
			return in, nil, fmt.Errorf("%w: format json needs items", domain.ErrInvalid)
		}
		if in.CSV != "" {
			return in, nil, fmt.Errorf("%w: give items or csv, not both", domain.ErrInvalid)
		}
		return in, nil, nil
	}
	if len(in.Items) > 0 {
		return in, nil, fmt.Errorf("%w: items are for format json", domain.ErrInvalid)
	}
	rows, err := bom.ReadRows(src)
	if err != nil {
		return in, nil, err
	}
	return in, rows, nil
}
