package brokerflow

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/export"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

var t0 = time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)

func TestQueueLifecycle(t *testing.T) {
	q, err := Enqueue(nil, "high", "model below the bar", "a@x.com", t0)
	if err != nil || q.State != StateQueued || !q.DueAt.Equal(t0.Add(24*time.Hour)) {
		t.Fatalf("enqueue = %+v, %v", q, err)
	}
	if _, err := Apply(q, Transition{Action: "decide", Outcome: "approve", Reason: "ok", By: "a@x.com"}, t0); !errors.Is(err, ErrConflict) {
		t.Errorf("decide before start = %v, want conflict", err)
	}
	q2, err := Apply(q, Transition{Action: "assign", Assignee: " Rev@X.com ", By: "a@x.com"}, t0)
	if err != nil || q2.State != StateAssigned || q2.Assignee != "rev@x.com" {
		t.Fatalf("assign = %+v, %v", q2, err)
	}
	if q.State != StateQueued {
		t.Error("Apply changed the current item")
	}
	if _, err := Apply(q2, Transition{Action: "start", By: "other@x.com"}, t0); !errors.Is(err, ErrConflict) {
		t.Errorf("start by someone else = %v, want conflict", err)
	}
	q3, err := Apply(q2, Transition{Action: "start", By: "rev@x.com"}, t0.Add(time.Hour))
	if err != nil || q3.State != StateInReview {
		t.Fatalf("start = %+v, %v", q3, err)
	}
	if _, err := Apply(q3, Transition{Action: "decide", Outcome: "approve", By: "rev@x.com"}, t0); !errors.Is(err, ErrInvalid) {
		t.Errorf("decide without reason = %v, want invalid", err)
	}
	if _, err := Apply(q3, Transition{Action: "decide", Outcome: "maybe", Reason: "x", By: "rev@x.com"}, t0); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad outcome = %v, want invalid", err)
	}
	late := t0.Add(25 * time.Hour)
	q4, err := Apply(q3, Transition{Action: "decide", Outcome: "changes", Reason: "need the BOM costs", By: "rev@x.com"}, late)
	if err != nil || q4.State != StateDecided || q4.Decision.OnTime || q4.Decision.Reason != "need the BOM costs" {
		t.Fatalf("decide = %+v, %v", q4, err)
	}
	if len(q4.Log) != 4 {
		t.Errorf("log has %d steps, want 4", len(q4.Log))
	}
	if Overdue(q4, late) {
		t.Error("a decided item is never overdue")
	}
	q5, err := Enqueue(q4, "normal", "again after changes", "a@x.com", late)
	if err != nil || q5.State != StateQueued || q5.Decision != nil || len(q5.Log) != 5 {
		t.Fatalf("requeue = %+v, %v", q5, err)
	}
}

func TestQueueReleaseAndReprioritise(t *testing.T) {
	q, _ := Enqueue(nil, "", "check", "a@x.com", t0)
	if q.Priority != "normal" {
		t.Errorf("default priority = %q", q.Priority)
	}
	if _, err := Enqueue(nil, "asap", "x", "a", t0); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad priority = %v", err)
	}
	if _, err := Enqueue(nil, "low", "  ", "a", t0); !errors.Is(err, ErrInvalid) {
		t.Errorf("empty reason = %v", err)
	}
	q, _ = Apply(q, Transition{Action: "start", By: "rev@x.com"}, t0)
	if q.Assignee != "rev@x.com" {
		t.Errorf("self-start did not take the review: %+v", q)
	}
	q, err := Apply(q, Transition{Action: "release", Reason: "out today", By: "rev@x.com"}, t0)
	if err != nil || q.State != StateQueued || q.Assignee != "" {
		t.Fatalf("release = %+v, %v", q, err)
	}
	up, err := Enqueue(q, "urgent", "customer escalation", "b@x.com", t0.Add(time.Hour))
	if err != nil || !up.DueAt.Equal(t0.Add(4*time.Hour)) || up.State != StateQueued {
		t.Fatalf("reprioritise = %+v, %v", up, err)
	}
	if !Overdue(up, t0.Add(5*time.Hour)) {
		t.Error("past due should be overdue")
	}
	a, b := &store.QueueItem{DueAt: t0.Add(time.Hour), QueuedAt: t0}, &store.QueueItem{DueAt: t0.Add(2 * time.Hour), QueuedAt: t0}
	if !Less(a, b, t0) || Less(b, a, t0) {
		t.Error("earlier due should come first")
	}
}

func TestCleanTextRejectsControlCharacters(t *testing.T) {
	if _, err := CleanText("a\x00b", "reason", true); !errors.Is(err, ErrInvalid) {
		t.Errorf("NUL = %v", err)
	}
	if _, err := CleanText(strings.Repeat("x", MaxText+1), "reason", true); !errors.Is(err, ErrInvalid) {
		t.Errorf("too long = %v", err)
	}
	if s, err := CleanText(" a\n\tb ", "reason", true); err != nil || s != "a\n\tb" {
		t.Errorf("clean = %q, %v", s, err)
	}
}

func product() *domain.Product {
	return &domain.Product{
		Name:   "Cordless drill",
		Plants: []domain.Plant{{ID: "p1", Name: "Monterrey plant", Country: "Mexico"}},
		Components: []domain.Component{
			{Name: "Motor", MadeIn: "China", HTS: "8501.10"},
			{Name: "Housing", MadeIn: "Mexico", Plant: "p1"},
		},
		Operations: []domain.Operation{{Name: "Final assembly", Plant: "p1"}},
	}
}

func letter() string {
	return export.CFR177(export.Input{ProductName: "Cordless drill", Product: product(), Facts: "The drill is assembled in Mexico.", Requested: "Mexico"})
}

func fullSubmitter() store.FilingSubmitter {
	return store.FilingSubmitter{
		RequesterName: "Acme Tools Inc.", RequesterAddress: "1 Main St, Austin, TX", RequesterEmail: "trade@acme.example",
		Interest: "importer", ImporterOfRecord: "the requester", NoPriorConsideration: true, Confidential: "none",
		SignerName: "Jo Smith", SignerTitle: "Trade Compliance Manager",
		Manufacturers:        map[string]string{"Monterrey plant": "Acme de Mexico S.A., Monterrey, MID MXACMMON123MON"},
		CountryOfExportation: "Mexico", FirstShipment: "March 2027", ProductModel: "Cordless drill, model D-20",
		ProductDescription: "A battery-powered hand drill for home and trade use.",
		HTSReference:       "8467.21", Marking: "Made in Mexico, molded on the housing", LegalBasis: "19 CFR 102.20(n)",
		PriorAdvice: "None", Series: "is not", Sample: "Not sent",
	}
}

func TestCheckOnTheGeneratedLetter(t *testing.T) {
	raw := letter()
	es := Check(raw)
	if Ready(es) {
		t.Fatal("an unfilled letter is ready")
	}
	missing := map[string]bool{}
	for _, e := range es {
		if e.Status == "missing" {
			missing[e.Key] = true
		}
	}
	for _, k := range []string{"parties", "transaction", "merchandise", "position", "prior", "confidential", "signature"} {
		if !missing[k] {
			t.Errorf("%s should be missing on the raw letter", k)
		}
	}
	if missing["port"] {
		t.Error("port is required only if known; its placeholder is optional")
	}
	sub, err := CleanSubmitter(fullSubmitter())
	if err != nil {
		t.Fatal(err)
	}
	filled := Fill(raw, sub)
	es = Check(filled)
	for _, e := range es {
		if e.Required && e.Status != "present" {
			t.Errorf("%s still missing: %s", e.Key, e.Detail)
		}
	}
	if !Ready(es) {
		t.Fatal("a fully answered letter is not ready")
	}
	for _, want := range []string{"Acme Tools Inc.", "/s/ Jo Smith", "MID MXACMMON123MON", "does not request confidential treatment", "Not yet known", "Expected first shipment: March 2027."} {
		if !strings.Contains(filled, want) {
			t.Errorf("filled letter lacks %q", want)
		}
	}
	if strings.Contains(filled, "be withheld from disclosure") {
		t.Error("both confidentiality statements kept")
	}
}

func TestFillConfidentialWithhold(t *testing.T) {
	s := fullSubmitter()
	s.Confidential = "supplier names and costs"
	out := Fill(letter(), s)
	if !strings.Contains(out, "namely supplier names and costs, be withheld") || strings.Contains(out, "does not request confidential") {
		t.Errorf("withhold statement wrong:\n%s", out[strings.Index(out, "## 10"):strings.Index(out, "## 11")])
	}
}

func TestCleanSubmitterRejects(t *testing.T) {
	for _, s := range []store.FilingSubmitter{
		{Interest: "spy"},
		{Series: "maybe"},
		{RequesterName: "[FILL IN: x]"},
		{Manufacturers: map[string]string{"p": ""}},
	} {
		if _, err := CleanSubmitter(s); !errors.Is(err, ErrInvalid) {
			t.Errorf("CleanSubmitter(%+v) = %v, want invalid", s, err)
		}
	}
}

func TestFilingStatusMoves(t *testing.T) {
	if !CanMove(FilingPrepared, FilingSubmitted) || CanMove(FilingPrepared, FilingRuling) || CanMove(FilingRuling, FilingWithdrawn) || CanMove(FilingWithdrawn, FilingSubmitted) {
		t.Error("status moves wrong")
	}
}

func TestTextPDF(t *testing.T) {
	pdf := TextPDF("t", strings.Repeat("## Head\n\nA line (with parens) and § 177.2 — dash.\n", 80))
	s := string(pdf)
	if !strings.HasPrefix(s, "%PDF-1.4") || !strings.HasSuffix(s, "%%EOF\n") {
		t.Fatal("not a PDF")
	}
	if n := strings.Count(s, "/Type /Page "); n < 2 {
		t.Errorf("pages = %d, want several", n)
	}
	if !strings.Contains(s, `\(with parens\)`) || !strings.Contains(s, `\247`) || !strings.Contains(s, `\227`) {
		t.Error("escaping or WinAnsi mapping wrong")
	}
	// xref offsets point at the objects.
	i := strings.Index(s, "xref\n")
	lines := strings.Split(s[i:], "\n")
	off := lines[3][:10]
	n := 0
	for _, c := range off {
		n = n*10 + int(c-'0')
	}
	if !strings.HasPrefix(s[n:], "1 0 obj") {
		t.Errorf("xref for object 1 points at %q", s[n:n+10])
	}
}

func TestBuildAndZipAreReproducible(t *testing.T) {
	sub := fullSubmitter()
	l := Fill(letter(), sub)
	f := &store.Filing{Status: FilingPrepared, PreparedAt: t0, Submitter: sub, Letter: l, Elements: Check(l)}
	f.Ready = Ready(f.Elements)
	pkg, files, err := Build("c1", f, []store.Approval{{Decision: "approve", By: "b@x.com", Role: "broker", At: t0}}, product())
	if err != nil {
		t.Fatal(err)
	}
	if !pkg.Ready || len(pkg.Materials) != 2 || pkg.Materials[1].Plant != "Monterrey plant" || len(pkg.Steps) != 1 || pkg.Steps[0].Country != "Mexico" {
		t.Errorf("package = %+v", pkg)
	}
	if !strings.Contains(string(files["exhibit-a-materials.csv"]), "Motor,China,8501.10,") {
		t.Errorf("exhibit A = %s", files["exhibit-a-materials.csv"])
	}
	z1, err := Zip(files, t0)
	if err != nil {
		t.Fatal(err)
	}
	_, files2, _ := Build("c1", f, []store.Approval{{Decision: "approve", By: "b@x.com", Role: "broker", At: t0}}, product())
	z2, _ := Zip(files2, t0)
	if !bytes.Equal(z1, z2) || Hash(files) != Hash(files2) {
		t.Error("the package is not reproducible")
	}
	r, err := zip.NewReader(bytes.NewReader(z1), int64(len(z1)))
	if err != nil || len(r.File) != 5 {
		t.Fatalf("zip = %v files, %v", len(r.File), err)
	}
}
