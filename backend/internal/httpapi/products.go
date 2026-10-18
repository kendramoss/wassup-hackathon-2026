package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/bom"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/extract"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/familycheck"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

const maxUpload = 5 << 20

// importProduct turns an uploaded bill of materials into a structured product
// (it does not create a case; send the product to /v1/cases/predict). Accepts
// multipart/form-data with a "file" (CSV or .xlsx) and fields name,
// description, family; or JSON {name, description, family, csv}.
func (a *api) importProduct(w http.ResponseWriter, r *http.Request) {
	var name, desc, family, fallback string
	var src io.Reader
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
		if err := r.ParseMultipartForm(maxUpload); err != nil {
			a.fail(w, r, fmt.Errorf("%w: upload: %v", domain.ErrInvalid, err))
			return
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			a.fail(w, r, fmt.Errorf("%w: a file field with the CSV or Excel BOM is required", domain.ErrInvalid))
			return
		}
		defer f.Close()
		name, desc, family, src = r.FormValue("name"), r.FormValue("description"), r.FormValue("family"), f
		fallback = r.FormValue("fallback_name")
	} else {
		var in struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Family      string `json:"family"`
			CSV         string `json:"csv"`
			// FallbackName names the product when neither the request, the
			// sheet nor the family check does (e.g. the file's name).
			FallbackName string `json:"fallback_name"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxUpload))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in); err != nil {
			a.fail(w, r, fmt.Errorf("%w: request body: %v", domain.ErrInvalid, err))
			return
		}
		name, desc, family, src = in.Name, in.Description, in.Family, strings.NewReader(in.CSV)
		fallback = in.FallbackName
	}
	rows, err := bom.ReadRows(src)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	// The name and family the user gave win; then the sheet's own columns;
	// then, for the family, the AI check on the parts (18 Oct).
	info := a.importInfo(r, rows, name, desc, family)
	name, family = info.name, info.family
	if name == "" {
		name = strings.TrimSpace(fallback)
	}
	var tm *planner.Template
	if family != "" {
		t, ok := a.d.Templates[family]
		if !ok {
			a.fail(w, r, fmt.Errorf("%w: family %q has no template; known: %s", domain.ErrInvalid, family, a.families()))
			return
		}
		tm = &t
	}
	opts := bom.Options{Name: name, Description: desc, Family: family, Template: tm}
	p, warnings, err := bom.Parse(rows, opts)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	// Optional money columns prefill the economics (#235); a sheet without
	// them answers exactly as before.
	m, mw := bom.ParseMoney(rows, opts)
	warnings = append(warnings, mw...)
	out := map[string]any{"product": p, "family": family}
	if info.nameSource != "" {
		out["name_source"] = info.nameSource
	}
	if info.familySource != "" {
		out["family_source"], out["family_reason"] = info.familySource, info.familyReason
	}
	sources := map[string]string{}
	if m.HTS != "" {
		if code, ok := duty.LiveHTS(m.HTS); ok {
			p.HTS, sources["hts"] = code, "file"
			out["product"] = p
		} else {
			warnings = append(warnings, fmt.Sprintf("hts %q is not an 8- or 10-digit HTS number; ignored", m.HTS))
		}
	}
	if m.AnnualImportValueUSD > 0 || len(m.GroupCosts) > 0 {
		out["economics"] = planner.Economics{Illustrative: true, AnnualImportValueUSD: m.AnnualImportValueUSD, GroupCosts: m.GroupCosts}
		if m.AnnualImportValueUSD > 0 {
			sources["annual_import_value_usd"] = "file"
		}
		if len(m.GroupCosts) > 0 {
			sources["group_costs"] = "file"
		}
	}
	if len(sources) > 0 {
		out["economics_sources"] = sources
	}
	if warnings == nil {
		warnings = []string{}
	}
	out["warnings"] = warnings
	writeJSON(w, http.StatusOK, out)
}

// ProductExtractor drafts a product from facts text (extract.Extractor).
type ProductExtractor interface {
	Extract(ctx context.Context, facts, name string, tm *planner.Template) (extract.Draft, error)
}

// extractProduct drafts an editable structured product from pasted facts.
// Nothing is saved: the UI sends the edited product to /v1/cases/predict.
func (a *api) extractProduct(w http.ResponseWriter, r *http.Request) {
	if a.d.Extractor == nil {
		writeError(w, http.StatusServiceUnavailable, "internal", "extractor is not configured", nil)
		return
	}
	var in struct {
		Facts  string `json:"facts"`
		Family string `json:"family"`
		Name   string `json:"name"`
	}
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if strings.TrimSpace(in.Facts) == "" || len([]rune(in.Facts)) > domain.MaxFactsRunes {
		a.fail(w, r, fmt.Errorf("%w: facts are required, up to %d characters", domain.ErrInvalid, domain.MaxFactsRunes))
		return
	}
	var tm *planner.Template
	if in.Family != "" {
		t, ok := a.d.Templates[in.Family]
		if !ok {
			a.fail(w, r, fmt.Errorf("%w: family %q has no template; known: %s", domain.ErrInvalid, in.Family, a.families()))
			return
		}
		tm = &t
	}
	d, err := a.d.Extractor.Extract(r.Context(), in.Facts, in.Name, tm)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

type importMeta struct {
	name, nameSource                   string
	family, familySource, familyReason string
}

// importInfo settles the product name and family for an import. Given
// values win; then the sheet's product and family columns (a family by its
// ID or name); then, with a family checker, a sure match (>= 0.6) on the
// parts, and the checker's product name for a sheet that names none. A
// failed check leaves them as they were.
func (a *api) importInfo(r *http.Request, rows [][]string, name, desc, family string) importMeta {
	m := importMeta{name: name, family: family}
	if family != "" {
		m.familySource = "given"
	}
	sheetName, sheetFamily := bom.SheetInfo(rows)
	if m.name == "" && sheetName != "" {
		m.name, m.nameSource = sheetName, "file"
	}
	if m.family == "" && sheetFamily != "" {
		key := strings.ToLower(sheetFamily)
		for id, t := range a.d.Templates {
			if strings.ToLower(id) == key || strings.ToLower(t.Name) == key {
				m.family, m.familySource = id, "file"
			}
		}
	}
	if (m.family != "" && m.name != "") || a.d.Family == nil {
		return m
	}
	p, _, err := bom.Parse(rows, bom.Options{Name: m.name, Description: desc})
	if err != nil && len(p.Components) == 0 {
		return m
	}
	var parts []string
	for _, c := range p.Components {
		parts = append(parts, c.Name)
	}
	text := "Parts: " + strings.Join(parts, ", ")
	if m.name != "" {
		text = "Product name: " + m.name + "\n" + text
	}
	res, err := a.d.Family.Check(r.Context(), text)
	if err != nil {
		a.d.Logger.WarnContext(r.Context(), "import: family check failed; family left as given", "request_id", RequestID(r.Context()), "err", err)
		return m
	}
	if m.family == "" && res.Family != familycheck.None && res.Confidence >= 0.6 {
		m.family, m.familySource, m.familyReason = res.Family, "ai", res.Reason
	}
	if m.name == "" && strings.TrimSpace(res.ProductName) != "" {
		m.name, m.nameSource = strings.TrimSpace(res.ProductName), "ai"
	}
	return m
}
