package extract

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/media"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

func png(w, h int) File {
	return File{Info: media.Info{MIMEType: media.PNG, Bytes: 10, Width: w, Height: h}, Data: []byte("\x89PNG\r\n\x1a\nxx")}
}

func TestExtractMediaRequiresFiles(t *testing.T) {
	_, err := New(llm.FakeText("{}"), "m").ExtractMedia(context.Background(), nil, "", nil)
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("err = %v", err)
	}
}

// The simulated answer maps onto the motorised tools template: every group
// is known, the part with no country is listed, not guessed, and the draft
// says it is simulated.
func TestSimulatedMediaAnswerMapsToTemplate(t *testing.T) {
	tms, err := planner.LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	tm := tms["motorised_tools_small_appliances"]
	d, err := New(llm.FakeText(SimulatedMediaAnswer), "m").ExtractMedia(context.Background(), []File{png(800, 600)}, "", &tm)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Simulated || d.Warnings[0] != SimulatedNote || d.Source != "media" || d.Trace.PromptVersion != MediaPromptVersion {
		t.Fatalf("draft = %+v", d)
	}
	if len(d.Components) != 4 || len(d.Operations) != 1 || len(d.Product.Components) != 4 {
		t.Fatalf("rows: %d components, %d operations", len(d.Components), len(d.Operations))
	}
	for _, r := range append(d.Components, d.Operations...) {
		if r.Group == "" || r.GroupGuessed || r.SourceFile != 1 || r.SourcePage != 1 {
			t.Errorf("row %+v", r)
		}
	}
	ws := strings.Join(d.Warnings, "\n")
	for _, want := range []string{"power cord without", "Hard to read, check against the document: fan.", "File 1: could not read an example row"} {
		if !strings.Contains(ws, want) {
			t.Errorf("warnings miss %q:\n%s", want, ws)
		}
	}
}

// Citations outside what was sent are dropped with a warning, never kept.
func TestMediaCitationsAreChecked(t *testing.T) {
	ans := `{"product":"X","components":[{"name":"motor","country":"China","group":"","file":3,"page":1,"legible":true},
{"name":"fan","country":"China","group":"","file":1,"page":5,"legible":true}],"operations":[],"plants":[],
"stated_component_count":4,"unreadable":[],"notes":[]}`
	d, err := New(llm.FakeText(ans), "m").ExtractMedia(context.Background(), []File{png(10, 10)}, "Given name", nil)
	if err != nil {
		t.Fatal(err)
	}
	if d.Components[0].SourceFile != 0 || d.Components[1].SourceFile != 1 || d.Components[1].SourcePage != 0 {
		t.Errorf("components = %+v", d.Components)
	}
	ws := strings.Join(d.Warnings, "\n")
	if !strings.Contains(ws, `"motor": the file cited (3) was not sent`) || !strings.Contains(ws, "The document says 4 components; it names 2.") {
		t.Errorf("warnings: %s", ws)
	}
	// Any answer from the fake provider is labelled simulated: it never
	// reads the files.
	if d.Product.Name != "Given name" || !d.Simulated {
		t.Errorf("product %q simulated %v", d.Product.Name, d.Simulated)
	}
}

func TestUnreadableWarningWording(t *testing.T) {
	for _, c := range []struct{ what, want string }{
		{"row 7", "File 1: could not read row 7."},
		{"row 7, the country is cut off.", "File 1: could not read row 7, the country is cut off."},
		{"The descriptions are cut off at the right edge.", "File 1, hard to read: The descriptions are cut off at the right edge."},
		{"HTS", "File 1: could not read HTS."},
	} {
		if got := unreadableWarning("File 1", c.what); got != c.want {
			t.Errorf("unreadableWarning(%q) = %q, want %q", c.what, got, c.want)
		}
	}
}
