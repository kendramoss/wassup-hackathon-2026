package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/bom"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

const maxUpload = 5 << 20

// importProduct turns an uploaded bill of materials into a structured product
// (it does not create a case; send the product to /v1/cases/predict). Accepts
// multipart/form-data with a "file" (CSV or .xlsx) and fields name,
// description, family; or JSON {name, description, family, csv}.
func (a *api) importProduct(w http.ResponseWriter, r *http.Request) {
	var name, desc, family string
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
	} else {
		var in struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Family      string `json:"family"`
			CSV         string `json:"csv"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxUpload))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in); err != nil {
			a.fail(w, r, fmt.Errorf("%w: request body: %v", domain.ErrInvalid, err))
			return
		}
		name, desc, family, src = in.Name, in.Description, in.Family, strings.NewReader(in.CSV)
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
	rows, err := bom.ReadRows(src)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	p, warnings, err := bom.Parse(rows, bom.Options{Name: name, Description: desc, Family: family, Template: tm})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if warnings == nil {
		warnings = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"product": p, "family": family, "warnings": warnings})
}
