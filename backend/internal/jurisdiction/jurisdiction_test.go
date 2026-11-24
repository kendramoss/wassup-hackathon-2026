package jurisdiction

import (
	"context"
	"errors"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
)

// These sentences are written for the tests; none is from the D680 probe
// sets, which are scored once and never tuned on.
func TestDetect(t *testing.T) {
	for _, tc := range []struct {
		name, text, code, hint string
		source                 string
		conflict               bool
	}{
		{"no cue", "Kettle: heating element from China, body moulded and assembled in Vietnam.", US, "", SourceDefault, false},
		{"CBP", "Will CBP treat the kettle assembled in Vietnam as a product of Vietnam?", US, "", SourceDetected, false},
		{"authority BOI", "Please assess this for a BOI application: pumps assembled in Slovakia.", EU, "", SourceDetected, false},
		{"authority HMRC", "HMRC origin question: scooters assembled in Taiwan.", UK, "", SourceDetected, false},
		{"adjective customs", "Which origin will Australian customs give a tent sewn in Laos?", AU, "", SourceDetected, false},
		{"entry into place", "The finished kettles are imported into Japan by our distributor.", JP, "", SourceDetected, false},
		{"entry into city", "We bring the lamps in through Rotterdam; they are imported into the Netherlands.", EU, "", SourceDetected, false},
		{"stop, then U.S.", "Parts made in China are imported into Thailand for further processing. The finished pumps are exported to the United States.", US, "", SourceDetected, false},
		{"stop, once", "Once imported into Mexico, the forgings are machined. What is the origin of the forgings?", US, "", SourceDefault, false},
		{"processing in the place", "Cells are imported into Malaysia. Packs are assembled in Malaysia and sold worldwide.", US, "", SourceDefault, false},
		{"last entry wins", "Boards are imported into the Philippines. The finished lamps are imported into Canada.", CA, "", SourceDetected, false},
		{"conflict", "For CBP and for the Canada Border Services Agency: what origin applies?", US, CA, SourceDetected, true},
		{"U.S. authority beats a non-U.S. entry", "Steel is imported into Canada in coils. HTSUS 7326.", US, CA, SourceDetected, false},
		{"currency is a hint", "Speakers assembled in Vietnam, priced at 40 EUR.", US, EU, SourceDefault, false},
		{"sold in is a hint", "Speakers assembled in Vietnam and sold in Germany.", US, EU, SourceDefault, false},
		{"CAD drawings are not a price", "Engineering and CAD drawings are made in Canada; assembly is in Mexico.", US, "", SourceDefault, false},
		{"export-side customs", "Documents: invoice and Malaysian customs declaration.", US, "", SourceDefault, false},
		{"customs in a port city", "Bikes cleared through customs in Gdansk. Frames welded in Taiwan.", EU, "", SourceDetected, false},
		{"brings into", "Our firm brings garden hoses made in Vietnam into Canada each spring.", CA, "", SourceDetected, false},
		{"member-state agency", "Will the Zoll accept Turkey as the origin of these towels?", EU, "", SourceDetected, false},
		{"stop: for <verb>ing", "Peppers are imported to South Korea for drying and grinding.", US, "", SourceDefault, false},
		{"stop: also in", "Carbon is imported into Korea from China. Also in Korea, the filters are pressed.", US, "", SourceDefault, false},
		{"other market", "Sofas imported into India from Vietnam. Which origin?", Other, "", SourceDetected, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			j := Detect(tc.text)
			if j.Code != tc.code || j.Hint != tc.hint || j.Source != tc.source || j.Conflict != tc.conflict {
				t.Errorf("Detect = %+v, want code %s hint %q source %s conflict %v", j, tc.code, tc.hint, tc.source, tc.conflict)
			}
			if j.Source == SourceDetected && j.Cue == "" {
				t.Errorf("detected with no cue: %+v", j)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	j, err := Resolve("uk", "imported into the United States")
	if err != nil || j.Code != UK || j.Source != SourceChosen {
		t.Fatalf("chosen: %+v %v", j, err)
	}
	if _, err := Resolve("fr", ""); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("unknown code: %v", err)
	}
	if j, _ := Resolve("", "HMRC asks"); j.Code != UK || j.Source != SourceDetected {
		t.Fatalf("detected: %+v", j)
	}
	if v := Of(nil); v.Code != US || v.Source != SourceDefault {
		t.Fatalf("legacy case: %+v", v)
	}
}

type fakeEngine struct{}

func (fakeEngine) Predict(context.Context, domain.Case) (engine.Result, error) {
	return engine.Result{}, nil
}

func TestRegistry(t *testing.T) {
	r := Default()
	if !r.Supported(US) || r.Supported(EU) {
		t.Fatalf("default registry: %v", r.SupportedCodes())
	}
	if err := r.Register(Authority{Market: EU}); err == nil {
		t.Fatal("a non-U.S. market without an engine registered")
	}
	if err := r.Register(Authority{Market: Other, Engine: fakeEngine{}}); err == nil {
		t.Fatal("other registered")
	}
	if err := r.Register(Authority{Market: US}); err == nil {
		t.Fatal("U.S. registered twice")
	}
	if err := r.Register(Authority{Market: EU, Engine: fakeEngine{}}); err != nil {
		t.Fatal(err)
	}
	if got := r.SupportedCodes(); len(got) != 2 || got[0] != US || got[1] != EU {
		t.Fatalf("supported = %v", got)
	}
	var nilReg *Registry
	if !nilReg.Supported(US) || nilReg.Supported(UK) {
		t.Fatal("nil registry is U.S. only")
	}
	if got := UnsupportedReason(EU); got != "Origin rules for the European Union aren't supported yet" {
		t.Fatalf("reason = %q", got)
	}
	if d := r.DetailOf(&domain.Jurisdiction{Code: KR, Source: SourceChosen}); d.Name != "South Korea" || d.Supported || d.FollowUps {
		t.Fatalf("detail = %+v", d)
	}
	// Only the United States has plan, compare, draft and broker review: a
	// registered non-U.S. market is supported but has no follow-ups (#678).
	if d := r.DetailOf(nil); !d.Supported || !d.FollowUps {
		t.Fatalf("U.S. detail = %+v", d)
	}
	if d := r.DetailOf(&domain.Jurisdiction{Code: EU, Source: SourceChosen}); !d.Supported || d.FollowUps {
		t.Fatalf("registered EU detail = %+v", d)
	}
	for _, m := range r.List() {
		if want := m["code"] == US; m["follow_ups"] != want {
			t.Fatalf("list %v: follow_ups want %v", m, want)
		}
	}
}

// Northern Ireland (D767): EU or UK rules can apply under the Windsor
// Framework, so naming it as the destination asks the user to pick; it
// never picks the UK. Work done there is not a destination.
func TestDetectNorthernIreland(t *testing.T) {
	for _, text := range []string{
		"The lamps are imported into Northern Ireland from Poland. Which origin?",
		"Our company is shipping smart luminaires to our Belfast distribution hub.",
		"Heaters assembled in Vietnam, delivered to a warehouse in Belfast.",
	} {
		j := Detect(text)
		if j.Code != Other || j.Cue != CueNorthernIreland || j.Hint != UK || !j.Conflict || j.Source != SourceDetected {
			t.Errorf("%q: %+v", text, j)
		}
	}
	for _, c := range []struct{ text, code string }{
		{"Goods imported into the United Kingdom through Felixstowe.", UK},
		{"Pumps assembled in Northern Ireland are shipped to the United States.", US},
		{"Kettles assembled in Belfast from Chinese parts.", US},
	} {
		if j := Detect(c.text); j.Code != c.code {
			t.Errorf("%q: %+v, want %s", c.text, j, c.code)
		}
	}
}
