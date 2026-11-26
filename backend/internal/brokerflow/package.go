package brokerflow

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// PackageSchema names the JSON layout of a filing package.
const PackageSchema = "undelabs.erulings-package.v1"

// Package is the eRulings-ready filing package: the letter, the element
// check, the sign-offs and the exhibits the case can generate. It is
// prepared for the requester or broker to submit through eRulings
// (https://erulings.cbp.gov); nothing here submits it.
type Package struct {
	Schema      string                `json:"schema"`
	CaseID      string                `json:"case_id"`
	PreparedAt  time.Time             `json:"prepared_at"`
	Status      string                `json:"status"`
	Ready       bool                  `json:"ready"`
	Submitter   store.FilingSubmitter `json:"submitter"`
	Elements    []store.FilingElement `json:"elements"`
	Approvals   []store.Approval      `json:"approvals"`
	Letter      string                `json:"letter_markdown"`
	Materials   []Material            `json:"exhibit_a_materials"`
	Steps       []Step                `json:"exhibit_b_steps"`
	Files       []store.FilingFile    `json:"files"`
	HowToSubmit []string              `json:"how_to_submit"`
}

// Material is one row of Exhibit A.
type Material struct {
	Name   string `json:"name"`
	MadeIn string `json:"made_in"`
	HTS    string `json:"hts,omitempty"`
	Plant  string `json:"plant,omitempty"`
}

// Step is one row of Exhibit B.
type Step struct {
	Name    string `json:"name"`
	Plant   string `json:"plant,omitempty"`
	Country string `json:"country,omitempty"`
}

// HowToSubmit is what the person filing does with the package.
var HowToSubmit = []string{
	"Sign in to eRulings (https://erulings.cbp.gov) and start a new ruling request, type: country of origin.",
	"Paste or upload letter.pdf as the request, and upload the exhibits listed under files.",
	"Attach the files marked to_attach (photographs or drawings, invoices) before submitting.",
	"Record the eRulings reference here as status submitted, then acknowledged and ruling_issued as CBP answers.",
}

// Exhibits reads Exhibit A and B from the product.
func Exhibits(p *domain.Product) ([]Material, []Step) {
	if p == nil {
		return nil, nil
	}
	var ms []Material
	for _, c := range p.Components {
		m := Material{Name: c.Name, MadeIn: c.MadeIn, HTS: c.HTS}
		if pl, ok := p.PlantByID(c.Plant); ok {
			m.Plant = pl.Name
		}
		ms = append(ms, m)
	}
	var ss []Step
	for _, o := range p.Operations {
		s := Step{Name: o.Name}
		if pl, ok := p.PlantByID(o.Plant); ok {
			s.Plant, s.Country = pl.Name, pl.Country
		}
		ss = append(ss, s)
	}
	return ms, ss
}

// Build makes the package and its files. Times are the preparation time so
// the zip and its hash are reproducible.
func Build(caseID string, f *store.Filing, approvals []store.Approval, p *domain.Product) (Package, map[string][]byte, error) {
	ms, ss := Exhibits(p)
	pkg := Package{Schema: PackageSchema, CaseID: caseID, PreparedAt: f.PreparedAt, Status: f.Status, Ready: f.Ready,
		Submitter: f.Submitter, Elements: f.Elements, Approvals: append([]store.Approval{}, approvals...), Letter: f.Letter,
		Materials: ms, Steps: ss, HowToSubmit: HowToSubmit}
	files := map[string][]byte{
		"letter.md":  []byte(f.Letter),
		"letter.pdf": TextPDF("Request for a binding ruling", f.Letter),
	}
	var a bytes.Buffer
	w := csv.NewWriter(&a)
	_ = w.Write([]string{"material", "made_in", "hts", "plant"})
	for _, m := range ms {
		_ = w.Write([]string{m.Name, m.MadeIn, m.HTS, m.Plant})
	}
	w.Flush()
	files["exhibit-a-materials.csv"] = a.Bytes()
	var b strings.Builder
	b.WriteString("Exhibit B: production steps and where each takes place\n\n")
	for i, s := range ss {
		where := strings.Join(nonEmpty(s.Plant, s.Country), ", ")
		if where == "" {
			where = "not stated"
		}
		fmt.Fprintf(&b, "%d. %s: %s\n", i+1, s.Name, where)
	}
	files["exhibit-b-steps.txt"] = []byte(b.String())

	describe := map[string]string{
		"letter.md":               "The ruling-request letter (19 CFR 177.2), Markdown",
		"letter.pdf":              "The ruling-request letter, PDF",
		"exhibit-a-materials.csv": "Exhibit A: materials and their countries",
		"exhibit-b-steps.txt":     "Exhibit B: production steps and where each takes place",
	}
	for _, name := range []string{"letter.md", "letter.pdf", "exhibit-a-materials.csv", "exhibit-b-steps.txt"} {
		sum := sha256.Sum256(files[name])
		pkg.Files = append(pkg.Files, store.FilingFile{Name: name, Kind: "generated", Description: describe[name], SHA256: hex.EncodeToString(sum[:]), Bytes: len(files[name])})
	}
	pkg.Files = append(pkg.Files, store.FilingFile{Name: "Exhibit C", Kind: "to_attach", Description: "Photographs or drawings of the article (19 CFR 177.2(b)(3))"})
	pkg.Files = append(pkg.Files, f.Attachments...)
	js, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return Package{}, nil, err
	}
	files["package.json"] = js
	return pkg, files, nil
}

// Hash is the SHA-256 of the package JSON.
func Hash(files map[string][]byte) string {
	sum := sha256.Sum256(files["package.json"])
	return hex.EncodeToString(sum[:])
}

// Zip writes the files in a fixed order with a fixed time.
func Zip(files map[string][]byte, at time.Time) ([]byte, error) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for _, name := range []string{"package.json", "letter.pdf", "letter.md", "exhibit-a-materials.csv", "exhibit-b-steps.txt"} {
		data, ok := files[name]
		if !ok {
			continue
		}
		w, err := z.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: at.UTC()})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
