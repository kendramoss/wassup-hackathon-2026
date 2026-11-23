package verifier

import (
	"context"
	"encoding/json"
	"testing"
)

// answerOrigins is a final answer with chosen origins, a claimed change and a
// direct citation of N350001.
func answerOrigins(a, b string, changes bool) string {
	ch := "false"
	if changes {
		ch = "true"
	}
	return `{"origin_a":"` + a + `","origin_b":"` + b + `","changes_origin":` + ch +
		`,"supporting_ruling":"N350001","counter_ruling":null,"confidence":0.96,"reasoning":"x"}`
}

func directSearch() *scriptSearch {
	return &scriptSearch{hits: []Hit{{ID: "N350001", Date: d2025, Subject: "hair dryer", Origin: "Thailand"}}}
}

func freshOrigins(t *testing.T, tk Task, a, b string, changes bool) Result {
	t.Helper()
	return verifyScript(t, directSearch(), tk, answerOrigins(a, b, changes), call("c1", "search_rulings", `{"query":"q"}`))
}

// Fresh acceptance: a claimed change between origins that normalise to the same
// country (alias, spelling, case, punctuation) is not supported; real distinct
// origins with an eligible direct citation still are.
func TestFreshRejectsEquivalentOriginsButAcceptsDistinct(t *testing.T) {
	for name, c := range map[string][2]string{
		"identical":            {"Thailand", "Thailand"},
		"case and spacing":     {"thailand ", " THAILAND"},
		"alias USA":            {"USA", "United States of America"},
		"alias PRC":            {"PRC", "China"},
		"alias Viet Nam":       {"Viet Nam", "Vietnam"},
		"Thai is Thailand":     {"Thai", "Thailand"},
		"Burma is Myanmar":     {"Burma", "myanmar"},
		"alias Korea":          {"Republic of Korea", "South Korea"},
		"article":              {"The Philippines", "Philippines"},
		"alternatives reorder": {"Indonesia/Malaysia", "Malaysia or Indonesia"},
	} {
		tk := task()
		tk.Target = ""
		res := freshOrigins(t, tk, c[0], c[1], true)
		if res.Verdict.PrecedentSupported {
			t.Errorf("%s: equivalent origins %q/%q accepted: %+v", name, res.OriginA, res.OriginB, res.Verdict)
		}
		if res.Verdict.SupportingRuling != "N350001" || len(res.Dropped) != 0 {
			t.Errorf("%s: validated citation must stay and nothing be dropped: %+v %v", name, res.Verdict, res.Dropped)
		}
	}
	for name, c := range map[string][2]string{
		"China to Thailand":         {"China", "Thailand"},
		"PRC to Thailand":           {"PRC", "Thailand"},
		"China to Thailand/Vietnam": {"China", "Thailand/Vietnam"},
		"Burma to Thai":             {"Burma", "Thai"},
	} {
		tk := task()
		tk.Target = ""
		res := freshOrigins(t, tk, c[0], c[1], true)
		if !res.Verdict.PrecedentSupported || res.Verdict.SupportingRuling != "N350001" {
			t.Errorf("%s: genuine distinct change rejected: %+v", name, res.Verdict)
		}
	}
	// The existing target rule is unchanged.
	if res := freshOrigins(t, task(), "China", "Thailand", true); !res.Verdict.PrecedentSupported {
		t.Errorf("distinct change to target rejected: %+v", res.Verdict)
	}
}

// Unknown or missing origins are never invented into a change.
func TestFreshRejectsUnknownOrMissingOrigins(t *testing.T) {
	for name, c := range map[string][2]string{
		"empty a": {"", "Thailand"}, "empty b": {"China", ""}, "both empty": {"", ""},
		"unknown a": {"Unknown", "Thailand"}, "unknown b": {"China", "unknown"},
		"unknown member": {"Unknown/China", "Thailand"}, "trailing slash": {"China/", "Thailand"},
		"leading slash": {"China", "/Thailand"}, "double slash": {"China//Vietnam", "Thailand"},
		"comma list a": {"China, Thailand", "China"}, "comma list b": {"China", "Thailand, Vietnam"},
	} {
		tk := task()
		tk.Target = ""
		if res := freshOrigins(t, tk, c[0], c[1], true); res.Verdict.PrecedentSupported {
			t.Errorf("%s: accepted: %+v", name, res.Verdict)
		}
	}
}

// KnownOriginA is the baseline: a verdict whose origin_a disagrees is
// unsupported; agreeing (also through an alias) passes.
func TestFreshKnownBaselineContradictionIsUnsupported(t *testing.T) {
	tk := task()
	tk.Target = ""
	tk.KnownOriginA, tk.KnownRulingA = "China", "N328954"
	if res := freshOrigins(t, tk, "Vietnam", "Thailand", true); res.Verdict.PrecedentSupported {
		t.Errorf("origin_a Vietnam contradicts known China: %+v", res.Verdict)
	}
	if res := freshOrigins(t, tk, "PRC", "Thailand", true); !res.Verdict.PrecedentSupported {
		t.Errorf("origin_a alias of the known baseline rejected: %+v", res.Verdict)
	}
	// Ambiguous baselines fail closed instead of skipping the check.
	for _, k := range []string{"unknown", "China, Vietnam", "Unknown/China"} {
		amb := tk
		amb.KnownOriginA = k
		if res := freshOrigins(t, amb, "China", "Thailand", true); res.Verdict.PrecedentSupported {
			t.Errorf("ambiguous baseline %q accepted: %+v", k, res.Verdict)
		}
	}
	burma := tk
	burma.KnownOriginA = "Myanmar"
	if res := freshOrigins(t, burma, "Burma", "Thailand", true); !res.Verdict.PrecedentSupported {
		t.Errorf("Burma against Myanmar baseline rejected: %+v", res.Verdict)
	}
	// Counterexample: the same verdict is fine against a different baseline, so
	// the rule is about agreement with the task's baseline, not about origins.
	other := tk
	other.KnownOriginA = "Vietnam"
	if res := freshOrigins(t, other, "Vietnam", "Thailand", true); !res.Verdict.PrecedentSupported {
		t.Errorf("verdict consistent with its own baseline rejected: %+v", res.Verdict)
	}
	if res := freshOrigins(t, other, "China", "Thailand", true); res.Verdict.PrecedentSupported {
		t.Errorf("China verdict against Vietnam baseline accepted: %+v", res.Verdict)
	}
}

// The cache applies the same rule inside cachedViolation: Peek and Verify both
// miss a stored contradictory verdict, and the repair goes to the val2 slot.
func TestCachedOriginInconsistentVerdictIsAMissAndRepaired(t *testing.T) {
	if ValidationEpoch != "val2" {
		t.Fatalf("epoch = %q", ValidationEpoch)
	}
	base := cacheTask()
	base.Target = ""
	known := base
	known.KnownOriginA, known.KnownRulingA = "China", "N328954"
	type tc struct {
		task    Task
		a, b    string
		changes bool
	}
	for name, c := range map[string]tc{
		"alias equal":        {base, "USA", "United States", true},
		"Thai Thailand":      {base, "Thai", "Thailand", true},
		"Burma Myanmar":      {base, "Burma", "Myanmar", true},
		"unknown member":     {base, "Unknown/China", "Thailand", true},
		"trailing slash":     {base, "China/", "Thailand", true},
		"comma list":         {base, "China, Thailand", "China", true},
		"USA aliases":        {base, "US", "United States", true},
		"identical":          {base, "Thailand", "Thailand", true},
		"unknown":            {base, "Unknown", "Thailand", true},
		"missing (legacy)":   {base, "", "", false},
		"changes_origin off": {base, "China", "Thailand", false},
		"baseline conflict":  {known, "Vietnam", "Thailand", true},
	} {
		t.Run(name, func(t *testing.T) {
			bad := goodResult(c.task)
			bad.OriginA, bad.OriginB, bad.ChangesOrigin = c.a, c.b, c.changes // full val2 proof, bad origins
			ch, inner, blobs := newFixedCache()
			inner.make = func(tk Task) Result { return goodResult(tk) }
			seed(t, ch, c.task, bad)
			orig := string(blobs.m[ch.key(c.task)])
			if _, ok, _ := ch.Peek(context.Background(), c.task); ok {
				t.Fatal("Peek replayed an origin-inconsistent verdict")
			}
			r, err := ch.Verify(context.Background(), c.task)
			if err != nil || r.Cached || inner.calls != 1 {
				t.Fatalf("replayed: %+v calls=%d err=%v", r, inner.calls, err)
			}
			if string(blobs.m[ch.key(c.task)]) != orig {
				t.Error("original slot overwritten")
			}
			if _, ok := blobs.m[ch.repairSlot(ch.key(c.task))]; !ok {
				t.Error("repair not stored in the current repair slot")
			}
			if r, _ = ch.Verify(context.Background(), c.task); !r.Cached || inner.calls != 1 {
				t.Errorf("repaired verdict not reused: %+v calls=%d", r, inner.calls)
			}
		})
	}
	// An ambiguous baseline never replays a supported verdict.
	amb := base
	amb.KnownOriginA = "unknown"
	ac, _, _ := newFixedCache()
	seed(t, ac, amb, goodResult(amb))
	if _, ok, _ := ac.Peek(context.Background(), amb); ok {
		t.Error("ambiguous baseline replayed")
	}
	// Control: genuinely distinct origins (and an aliased known baseline) replay.
	c, inner, _ := newFixedCache()
	good := goodResult(known)
	good.OriginA = "PRC"
	seed(t, c, known, good)
	if r, _ := c.Verify(context.Background(), known); !r.Cached || inner.calls != 0 {
		t.Errorf("distinct-origin verdict missed: %+v", r)
	}
}

// Old val1 proofs, and stored entries without origin fields, are a one-time
// miss; an unsupported verdict with nothing to prove still replays.
func TestVal1AndOriginlessEntriesMissOnce(t *testing.T) {
	tk := cacheTask()
	val1 := goodResult(tk)
	val1.Proof.Epoch = "val1"
	var raw map[string]any // a stored #295 blob has no origin keys at all
	b, _ := json.Marshal(goodResult(tk))
	_ = json.Unmarshal(b, &raw)
	delete(raw, "OriginA")
	delete(raw, "OriginB")
	delete(raw, "ChangesOrigin")
	noOrigins, _ := json.Marshal(raw)
	for name, blob := range map[string][]byte{"val1 proof": mustJSON(val1), "no origin fields": noOrigins} {
		t.Run(name, func(t *testing.T) {
			c, inner, blobs := newFixedCache()
			blobs.m[c.key(tk)] = blob
			if _, ok, _ := c.Peek(context.Background(), tk); ok {
				t.Fatal("replayed")
			}
			if r, _ := c.Verify(context.Background(), tk); r.Cached || inner.calls != 1 {
				t.Errorf("not re-verified once: %+v", r)
			}
			if r, _ := c.Verify(context.Background(), tk); !r.Cached || inner.calls != 1 {
				t.Errorf("repair not reused: %+v calls=%d", r, inner.calls)
			}
		})
	}
	if checkerVersion(NewCacheIn(New(nil, nil, Options{}), &memBlobs{m: map[string][]byte{}}, "m").inner) != PromptVersion {
		t.Error("prompt version changed")
	}
}

// constChecker is a goroutine-safe fake (Majority runs its inner checker concurrently).
type constChecker struct{ r Result }

func (c constChecker) Verify(context.Context, Task) (Result, error) { return c.r, nil }

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

// A fresh inner verdict that is inconsistent (a fake inner, as a panel lane or
// a legacy path could produce) is sanitized: support cleared, citations kept,
// nothing dropped or invented, not stored, and no loop.
func TestFreshOriginInconsistentVerdictIsSanitizedNotStored(t *testing.T) {
	tk := cacheTask()
	tk.Target = ""
	bad := goodResult(tk)
	bad.OriginA, bad.OriginB = "USA", "United States"
	blobs := &memBlobs{m: map[string][]byte{}}
	r, err := NewCacheIn(&fixedChecker{make: func(Task) Result { return bad }}, blobs, "m").Verify(context.Background(), tk)
	if err != nil || r.Verdict.PrecedentSupported || r.Verdict.SupportingRuling != "N350001" || len(r.Dropped) != 0 {
		t.Errorf("sanitize = %+v err=%v", r, err)
	}
	if cachedViolation(r, tk) != "" {
		t.Errorf("sanitized result still violates: %s", cachedViolation(r, tk))
	}
	if len(blobs.m) != 0 {
		t.Error("stored")
	}
	// Support dropped for another reason too: flags and dropped list stay coherent.
	both := bad
	both.Proof = ProofFor(tk) // citation unproven as well
	r, _ = sanitize(both, tk)
	if r.Verdict.PrecedentSupported || r.Verdict.SupportingRuling != "" || len(r.Dropped) == 0 || cachedViolation(r, tk) != "" {
		t.Errorf("combined sanitize = %+v", r)
	}
}

// Panel and Majority: the winning lane's origins travel with its verdict, and
// the cache in front rejects an inconsistent winner at the boundary.
func TestPanelAndMajorityOriginBoundaries(t *testing.T) {
	tk := cacheTask()
	tk.Target = ""
	lane := func(conf float64, a, b string) Result {
		r := goodResult(tk)
		r.Verdict.Confidence, r.OriginA, r.OriginB = conf, a, b
		return r
	}
	for name, mk := range map[string]func(a, b *stub) Checker{
		"panel": func(x, y *stub) Checker {
			return NewPanel(&stub{r: lane(0.90, "USA", "United States")}, []Checker{x, y}, 0.89, 0.95)
		},
		"majority": func(x, y *stub) Checker { return NewMajority(constChecker{x.r}, 3) },
	} {
		t.Run(name+" aliases", func(t *testing.T) {
			x, y := &stub{r: lane(0.92, "USA", "United States")}, &stub{r: lane(0.93, "USA", "United States")}
			blobs := &memBlobs{m: map[string][]byte{}}
			r, err := NewCacheIn(mk(x, y), blobs, "m").Verify(context.Background(), tk)
			if err != nil || r.Verdict.PrecedentSupported || len(blobs.m) != 0 {
				t.Errorf("alias majority supported/stored: %+v err=%v stored=%d", r.Verdict, err, len(blobs.m))
			}
		})
		t.Run(name+" distinct", func(t *testing.T) {
			x, y := &stub{r: lane(0.92, "China", "Thailand")}, &stub{r: lane(0.93, "China", "Thailand")}
			first := mk(x, y)
			if p, ok := first.(*Panel); ok {
				p.first = &stub{r: lane(0.90, "China", "Thailand")}
			}
			blobs := &memBlobs{m: map[string][]byte{}}
			r, err := NewCacheIn(first, blobs, "m").Verify(context.Background(), tk)
			if err != nil || !r.Verdict.PrecedentSupported || r.OriginA != "China" {
				t.Errorf("distinct majority rejected: %+v err=%v", r.Verdict, err)
			}
		})
	}
	// Differing lanes: the median supported lane is consistent, the dissenter is
	// inconsistent and in the minority, so the consistent verdict stands.
	good1, good2 := &stub{r: lane(0.92, "China", "Thailand")}, &stub{r: lane(0.93, "China", "Thailand")}
	r, _ := NewPanel(&stub{r: lane(0.90, "USA", "United States")}, []Checker{good1, good2}, 0.89, 0.95).Verify(context.Background(), tk)
	if !r.Verdict.PrecedentSupported || r.OriginA != "China" || cachedViolation(r, tk) != "" {
		t.Errorf("panel median should be a consistent lane: %+v", r)
	}
}

// Unit checks of the shared helper, including the ambiguous cases.
func TestOriginViolationUnits(t *testing.T) {
	for name, c := range map[string]struct{ a, b string }{
		"Thai/Thailand":        {"Thai", "Thailand"},
		"Burma/Myanmar":        {"Burma", "myanmar"},
		"unknown member":       {"Unknown/China", "Thailand"},
		"unknown member b":     {"China", "Thailand/unknown"},
		"trailing slash":       {"China/", "Thailand"},
		"leading slash":        {"China", "/Thailand"},
		"double slash":         {"China//Vietnam", "Thailand"},
		"comma list":           {"China, Thailand", "China"},
		"comma list b":         {"China", "Thailand, Vietnam"},
		"empty":                {"", "Thailand"},
		"whitespace":           {"  ", "Thailand"},
		"or with empty member": {"China or ", "Thailand"},
		"only unknown":         {"unknown", "unknown"},
	} {
		if originViolation(c.a, c.b, Task{}) == "" {
			t.Errorf("%s: %q -> %q accepted", name, c.a, c.b)
		}
	}
	for name, c := range map[string]struct{ a, b string }{
		"China/Thailand":  {"China", "Thailand"},
		"PRC/Thai":        {"PRC", "Thai"},
		"Burma/Thailand":  {"Burma", "Thailand"},
		"set vs member":   {"Thailand/China", "Thailand"},
		"or alternatives": {"China or Vietnam", "Thailand"},
	} {
		if why := originViolation(c.a, c.b, Task{}); why != "" {
			t.Errorf("%s: genuine change rejected: %s", name, why)
		}
	}
	// Baseline: aliases match; contradiction and ambiguous baselines fail closed.
	if originViolation("PRC", "Thai", Task{KnownOriginA: "China"}) != "" {
		t.Error("alias baseline rejected")
	}
	if originViolation("Burma", "Thailand", Task{KnownOriginA: "Myanmar"}) != "" {
		t.Error("Burma/Myanmar baseline rejected")
	}
	if originViolation("China", "Thailand", Task{KnownOriginA: "Türkiye"}) == "" {
		t.Error("baseline Türkiye vs China accepted")
	}
	for _, k := range []string{"unknown", "China, Vietnam", "Unknown/China", "China/"} {
		if originViolation("China", "Thailand", Task{KnownOriginA: k}) == "" {
			t.Errorf("ambiguous baseline %q bypassed the contradiction check", k)
		}
	}
	if originViolation("China", "Thailand", Task{KnownOriginA: "  "}) != "" {
		t.Error("blank KnownOriginA means no baseline")
	}
}
