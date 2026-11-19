package duty

import (
	"strings"
	"testing"
	"time"
)

// 8504.40.95 has a whole-subheading exclusion, 9405.42.8440 a described-product
// one; both end 2026-11-09 in fixtureAdditional.
func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func fixtureAdder(t *testing.T, json string) *Adder {
	t.Helper()
	a, err := parseAdditional([]byte(json))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func headings(ex []Extra) string {
	var h []string
	for _, e := range ex {
		h = append(h, e.Heading)
	}
	return strings.Join(h, ",")
}

func TestPartialExclusionBoundary(t *testing.T) {
	a := fixtureAdder(t, fixtureAdditional)
	cases := []struct {
		asOf              string
		may, expired      int
		wantExpiredStatus string
	}{
		{"2026-11-08", 1, 0, ""},        // day before: suggested
		{"2026-11-09", 1, 0, ""},        // end date: valid through end_date (inclusive)
		{"2026-11-10", 0, 1, "expired"}, // day after: historical only
		{"2027-06-01", 0, 1, "expired"},
		{"2026-10-01", 1, 0, ""}, // past-dated lookup keeps the then-valid suggestion
	}
	for _, c := range cases {
		r := a.ForAsOf("9405.42.84", "China", 0.042, day(c.asOf))
		if len(r.MayExclude) != c.may || len(r.Expired) != c.expired {
			t.Errorf("%s: may %v expired %v, want %d/%d", c.asOf, r.MayExclude, r.Expired, c.may, c.expired)
			continue
		}
		for _, e := range r.Expired {
			if e.Status != c.wantExpiredStatus {
				t.Errorf("%s: status %q", c.asOf, e.Status)
			}
		}
		// The duty itself never changes with a described-product exclusion.
		if headings(r.Extras) != "9903.05.31,9903.88.15" {
			t.Errorf("%s: extras %v", c.asOf, r.Extras)
		}
	}
}

func TestUTCBoundaryUsesUTCCalendarDate(t *testing.T) {
	a := fixtureAdder(t, fixtureAdditional)
	// 2026-11-09 23:30 in UTC-8 is 2026-11-10 07:30 UTC: past the end date.
	west := time.FixedZone("PST", -8*3600)
	r := a.ForAsOf("9405.42.84", "China", 0.042, time.Date(2026, 11, 9, 23, 30, 0, 0, west))
	if len(r.MayExclude) != 0 || len(r.Expired) != 1 {
		t.Errorf("UTC date 10 Nov: may %v expired %v", r.MayExclude, r.Expired)
	}
	// 2026-11-10 06:00 in UTC+9 is 2026-11-09 21:00 UTC: still valid.
	east := time.FixedZone("JST", 9*3600)
	r = a.ForAsOf("9405.42.84", "China", 0.042, time.Date(2026, 11, 10, 6, 0, 0, 0, east))
	if len(r.MayExclude) != 1 || len(r.Expired) != 0 {
		t.Errorf("UTC date 9 Nov: may %v expired %v", r.MayExclude, r.Expired)
	}
	r = a.ForAsOf("9405.42.84", "China", 0.042, time.Date(2026, 11, 9, 23, 59, 59, 0, time.UTC))
	if len(r.MayExclude) != 1 {
		t.Errorf("last second of end date: %v", r.MayExclude)
	}
}

func TestWholeExclusionOnlyRemovesDutyWhileValid(t *testing.T) {
	a := fixtureAdder(t, fixtureAdditional)
	for _, c := range []struct {
		asOf string
		want string
	}{
		{"2026-11-09", "9903.05.31"},            // excluded through the end date
		{"2026-11-10", "9903.05.31,9903.88.02"}, // lapsed: the stored list duty is back, no extension claimed
	} {
		r := a.ForAsOf("8504.40.95", "China", 0.042, day(c.asOf))
		if headings(r.Extras) != c.want {
			t.Errorf("%s: extras %q want %q", c.asOf, headings(r.Extras), c.want)
		}
	}
	if r := a.ForAsOf("8504.40.95", "China", 0.042, day("2026-11-10")); len(r.Expired) != 1 || r.Expired[0].Status != "expired" {
		t.Errorf("lapsed whole exclusion should be listed as historical: %v", r.Expired)
	}
}

func TestUnknownDatesGoToReviewNotActive(t *testing.T) {
	bad := strings.NewReplacer(`"end_date":"2026-11-09"`, `"end_date":"9 Nov 2026"`).Replace(fixtureAdditional)
	a := fixtureAdder(t, bad) // must not panic or fail to load
	r := a.ForAsOf("9405.42.84", "China", 0.042, day("2026-11-01"))
	if len(r.MayExclude) != 1 || r.MayExclude[0].Status != "review" || !strings.Contains(r.MayExclude[0].Description, "confirm") {
		t.Errorf("malformed end date: %v", r.MayExclude)
	}
	// Whole exclusion with unreadable date does not silently remove the duty.
	r = a.ForAsOf("8504.40.95", "China", 0.042, day("2026-11-01"))
	if headings(r.Extras) != "9903.05.31,9903.88.02" || len(r.MayExclude) != 1 || r.MayExclude[0].Status != "review" {
		t.Errorf("malformed whole: %v %v", r.Extras, r.MayExclude)
	}
	// Missing end date, and a zero as-of date, also go to review.
	empty := strings.NewReplacer(`,"end_date":"2026-11-09"`, ``).Replace(fixtureAdditional)
	r = fixtureAdder(t, empty).ForAsOf("9405.42.84", "China", 0.042, day("2026-11-01"))
	if len(r.MayExclude) != 1 || r.MayExclude[0].Status != "review" {
		t.Errorf("missing end date: %v", r.MayExclude)
	}
	r = fixtureAdder(t, fixtureAdditional).ForAsOf("9405.42.84", "China", 0.042, time.Time{})
	if len(r.MayExclude) != 1 || r.MayExclude[0].Status != "review" {
		t.Errorf("zero as-of: %v", r.MayExclude)
	}
}

func TestExclusionsStayNonApplied(t *testing.T) {
	a := fixtureAdder(t, fixtureAdditional)
	// Described-product and conditional exemptions never change the duty.
	r := a.ForAsOf("3004.90.92", "Thailand", 0.042, day("2026-11-01"))
	if headings(r.Extras) != "9903.05.77" || len(r.MayExclude) != 1 {
		t.Errorf("conditional: %v %v", r.Extras, r.MayExclude)
	}
	if r = a.ForAsOf("3004.90.92", "Thailand", 0.042, day("2030-01-01")); headings(r.Extras) != "9903.05.77" || len(r.MayExclude) != 1 {
		t.Errorf("conditional exemption has no end date and must stay: %v %v", r.Extras, r.MayExclude)
	}
}

func TestInjectedClockDrivesFor(t *testing.T) {
	a := fixtureAdder(t, fixtureAdditional)
	// Without a clock For is unchanged (no expiry filtering).
	if _, may := a.For("9405.42.84", "China", 0.042); len(may) != 1 {
		t.Errorf("legacy For: %v", may)
	}
	before := a.WithClock(func() time.Time { return day("2026-11-09") })
	after := a.WithClock(func() time.Time { return day("2026-11-10") })
	if _, may := before.For("9405.42.84", "China", 0.042); len(may) != 1 {
		t.Errorf("clock on end date: %v", may)
	}
	if _, may := after.For("9405.42.84", "China", 0.042); len(may) != 0 {
		t.Errorf("clock after end date: %v", may)
	}
	if _, may := a.For("9405.42.84", "China", 0.042); len(may) != 1 {
		t.Error("WithClock must not mutate the receiver")
	}
	var nilAdder *Adder
	if r := nilAdder.ForAsOf("9405.42.84", "China", 0.042, day("2026-11-01")); r.Extras != nil || r.MayExclude != nil {
		t.Error("nil adder")
	}
}

func TestEmbeddedExclusionsExpireAfterNov9(t *testing.T) {
	a, err := LoadAdditional()
	if err != nil {
		t.Fatal(err)
	}
	// Every embedded exclusion ends 2026-11-09; find any described-product one that lists today.
	var hit *Exclusion
	for i, e := range a.file.Exclusions {
		if !e.WholeSubheading && len(Normalize(e.HTS)) >= 10 {
			hit = &a.file.Exclusions[i]
			break
		}
	}
	if hit == nil {
		t.Skip("no described-product exclusion in embedded data")
	}
	for _, o := range []string{"China"} {
		var found bool
		for _, l := range a.file.Lists {
			if l.Heading == hit.Heading {
				found = true
			}
		}
		if !found {
			continue
		}
		on := a.ForAsOf(hit.HTS, o, 0.05, day("2026-11-09"))
		off := a.ForAsOf(hit.HTS, o, 0.05, day("2026-11-10"))
		dated := func(ex []Exclusion) (n int) {
			for _, e := range ex {
				if e.EndDate != "" {
					n++
				}
			}
			return
		}
		if dated(on.MayExclude) == 0 || dated(off.MayExclude) != 0 || len(off.Expired) == 0 {
			t.Errorf("%s: on %v, off %v/%v", hit.HTS, on.MayExclude, off.MayExclude, off.Expired)
		}
	}
}

func TestFillWithAsOfSplitsExpired(t *testing.T) {
	a := fixtureAdder(t, fixtureAdditional)
	rate := 0.042
	m := MFN{HTS: "9405.42.84", General: "4.2%", Rate: &rate, Supported: true}
	for _, c := range []struct {
		asOf       string
		check, exp int
	}{{"2026-11-09", 1, 0}, {"2026-11-10", 0, 1}} {
		r := m.FillWithAsOf(nil, []string{"China", "Thailand"}, a, day(c.asOf))
		if len(r.Check["China"]) != c.check || len(r.Expired["China"]) != c.exp || len(r.Extras["China"]) != 2 {
			t.Errorf("%s: %+v", c.asOf, r)
		}
		if r.Rates["China"] != 0.042+0.075+0.125 {
			t.Errorf("%s: rate %v", c.asOf, r.Rates["China"])
		}
	}
	// FillWith keeps its shape.
	if _, _, ex, _ := m.FillWith(nil, []string{"China"}, a); len(ex["China"]) != 2 {
		t.Error("FillWith changed")
	}
}
