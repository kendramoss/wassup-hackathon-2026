package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// #714: a CBP change to a ruling a draft or a shared packet relies on is
// shown beside the artefact, derived on read. Fixtures from the shipped
// supersession map: N344694 (China) modified by H344999 (Taiwan) on
// 2026-05-06; N345891 revoked by H346161 on 2025-05-05 (same origin held);
// N023537 revoked effective 2026-11-29; 080651 only proposed.

func editCase(t *testing.T, cs store.Cases, id string, f func(*store.Case)) {
	t.Helper()
	if _, err := cs.Update(context.Background(), id, func(c *store.Case) error { f(c); return nil }); err != nil {
		t.Fatal(err)
	}
}

func notesOf(out map[string]any) string {
	var b strings.Builder
	for _, n := range out["notes"].([]any) {
		b.WriteString(n.(string) + "\n")
	}
	return b.String()
}

func TestDraftNotesNameChangedRulingBesideTheLetter(t *testing.T) {
	var cs store.Cases
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { cs = d.Cases })
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	editCase(t, cs, id, func(c *store.Case) {
		c.Prediction.Origin = "China"
		c.Prediction.CitedRulings = []string{"", "N344694", "N023537", "080651"} // empty id, scheduled, proposed
	})
	draft := func(day string) (string, string) {
		pinToday(t, day)
		code, out := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"preview":true}`)
		if code != http.StatusOK {
			t.Fatalf("export = %d %v", code, out)
		}
		return out["content"].(string), notesOf(out)
	}
	before, notesBefore := draft("2026-05-05") // the day before the change
	if strings.Contains(notesBefore, "H344999") {
		t.Errorf("a change not yet in effect is in the notes:\n%s", notesBefore)
	}
	content, notes := draft("2026-11-22")
	if !strings.Contains(notes, "N344694 modified by H344999 on 2026-05-06 (bears on the answer)") {
		t.Errorf("notes lack the change:\n%s", notes)
	}
	for _, no := range []string{"N023537 revoked", "080651", "H314991", "H346396"} {
		if strings.Contains(notes, no) {
			t.Errorf("a scheduled or proposed change is in the notes (%s):\n%s", no, notes)
		}
	}
	if content != before || strings.Contains(content, "H344999") {
		t.Error("the letter text changed with the ruling change")
	}

	// The same change reads as not touching an answer elsewhere.
	editCase(t, cs, id, func(c *store.Case) { c.Prediction.Origin = "Vietnam" })
	_, notes = draft("2026-11-22")
	if !strings.Contains(notes, "N344694 modified by H344999 on 2026-05-06 (does not touch the answer directly)") {
		t.Errorf("relevance wording:\n%s", notes)
	}
}

func TestDraftNotesCoverSupportingAndCurrentSetupRulingsOnly(t *testing.T) {
	pinToday(t, "2026-11-22")
	hs, cs, id := alertHarness(t)
	editCase(t, cs, id, func(c *store.Case) {
		c.Prediction.CitedRulings = nil
		c.Prediction.Precedents = []string{"N344694"} // read, not relied on
		c.CurrentOrigin = &planner.KnownOrigin{Country: "China", Ruling: "N345891"}
	})
	_, out := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"preview":true}`)
	notes := notesOf(out)
	if !strings.Contains(notes, "N345891 revoked by H346161 on 2025-05-05") {
		t.Errorf("the current setup's ruling is not flagged:\n%s", notes)
	}
	if strings.Contains(notes, "N344694") {
		t.Errorf("a ruling only read is flagged:\n%s", notes)
	}
}

func TestDraftWithoutRelianceOnAChangedRulingIsUnchanged(t *testing.T) {
	pinToday(t, "2026-11-22")
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t)) // cites N350001, unchanged
	_, out := hs.do(t, "POST", "/v1/cases/"+c["id"].(string)+"/export", `{"preview":true}`)
	if n := notesOf(out); strings.Contains(n, "CBP changed") || strings.Contains(n, "revoked by") || strings.Contains(n, "modified by") {
		t.Errorf("notes on an unaffected draft:\n%s", n)
	}
}

// ---- the shared packet and the owner's review row ----

// frozenLink makes a link as alice, then rewrites the stored link as one
// shared at created whose frozen draft cites cited (origin) and, when plan
// is set, rests on it. The link stays open for a week.
func frozenLink(t *testing.T, hs harness, cs store.Cases, created time.Time, origin string, cited []string, plan *store.ReviewPlan) (id, tok string) {
	t.Helper()
	alice := visitor(rvAlice)
	id = hs.newCase(t, alice)
	tok, _ = makeLink(t, hs, id, `{}`, alice)
	editCase(t, cs, id, func(c *store.Case) {
		rv := &c.BrokerReviews[0]
		rv.CreatedAt, rv.ExpiresAt = created, time.Now().UTC().Add(7*24*time.Hour)
		rv.Draft.Prediction.Origin, rv.Draft.Prediction.CitedRulings, rv.Draft.Plan = origin, cited, plan
	})
	return id, tok
}

func packetChanges(t *testing.T, hs harness, tok string) ([]any, string) {
	t.Helper()
	rec := serve(hs, "GET", "/v1/review", "", tokenHeader(tok))
	if rec.Code != http.StatusOK {
		t.Fatalf("packet = %d %s", rec.Code, rec.Body)
	}
	var pk map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &pk)
	rc, _ := pk["ruling_changes"].([]any)
	return rc, rec.Body.String()
}

func ownerCount(t *testing.T, hs harness, id string) (any, bool) {
	t.Helper()
	code, out := hs.doAs(t, "GET", "/v1/cases/"+id+"/reviews", "", visitor(rvAlice))
	if code != http.StatusOK {
		t.Fatalf("owner list = %d %v", code, out)
	}
	v, ok := out["reviews"].([]any)[0].(map[string]any)["changed_since_shared"]
	return v, ok
}

func utc(s string) time.Time { t, _ := time.Parse(time.RFC3339, s); return t }

func TestPacketAndReviewRowFlagChangesAfterSharing(t *testing.T) {
	cases := []struct {
		name    string
		created string
		today   string
		cited   []string
		plan    *store.ReviewPlan
		want    int
	}{
		{"effective after sharing", "2026-05-01T10:00:00Z", "2026-11-22", []string{"N344694"}, nil, 1},
		{"same calendar day, sharing later in the day", "2026-05-06T23:59:00Z", "2026-11-22", []string{"N344694"}, nil, 1},
		{"change earlier than sharing", "2026-05-07T00:00:00Z", "2026-11-22", []string{"N344694"}, nil, 0},
		{"scheduled, not yet in effect", "2026-09-01T00:00:00Z", "2026-11-22", []string{"N023537"}, nil, 0},
		{"scheduled, in effect today", "2026-09-01T00:00:00Z", "2026-11-29", []string{"N023537"}, nil, 1},
		{"proposed only", "2026-01-01T00:00:00Z", "2026-11-22", []string{"080651"}, nil, 0},
		{"plan support", "2026-05-01T10:00:00Z", "2026-11-22", nil, &store.ReviewPlan{Description: "p", PredictedOrigin: "China", Supporting: "N344694"}, 1},
		{"cited and supporting once", "2026-05-01T10:00:00Z", "2026-11-22", []string{"N344694"}, &store.ReviewPlan{Description: "p", PredictedOrigin: "China", Supporting: "N344694"}, 1},
		{"empty ids skipped", "2026-05-01T10:00:00Z", "2026-11-22", []string{""}, &store.ReviewPlan{Description: "p", Supporting: ""}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hs, cs := reviewHarness(t)
			pinToday(t, tc.today)
			id, tok := frozenLink(t, hs, cs, utc(tc.created), "China", tc.cited, tc.plan)
			rc, body := packetChanges(t, hs, tok)
			n, has := ownerCount(t, hs, id)
			if len(rc) != tc.want {
				t.Fatalf("ruling_changes = %v, want %d", rc, tc.want)
			}
			if tc.want == 0 {
				if strings.Contains(body, "ruling_changes") || has {
					t.Errorf("empty fields must be omitted: packet has %v, row has %v", strings.Contains(body, "ruling_changes"), has)
				}
				return
			}
			if n != float64(tc.want) {
				t.Errorf("changed_since_shared = %v, want %d", n, tc.want)
			}
			if strings.Contains(body, id) {
				t.Error("the packet carries the case id")
			}
			ch := rc[0].(map[string]any)
			if ch["status"] != "final" || ch["in_effect"] != true || ch["ruling"] == "" || ch["note"] == "" {
				t.Errorf("change = %v", ch)
			}
		})
	}
}

func TestPacketChangeWordsRelevanceFromTheFrozenDraft(t *testing.T) {
	pinToday(t, "2026-11-22")
	hs, cs := reviewHarness(t)
	_, tok := frozenLink(t, hs, cs, utc("2026-05-01T00:00:00Z"), "Vietnam", []string{"N344694"}, nil)
	rc, _ := packetChanges(t, hs, tok)
	if len(rc) != 1 || rc[0].(map[string]any)["relevance"] != domain.RelevanceNotAnswer {
		t.Errorf("a change away from the frozen answer: %v", rc)
	}
	// Plan support is judged against the frozen plan's own target.
	hs, cs = reviewHarness(t)
	_, tok = frozenLink(t, hs, cs, utc("2026-05-01T00:00:00Z"), "Vietnam", nil, &store.ReviewPlan{Description: "p", PredictedOrigin: "Taiwan", Supporting: "N344694"})
	rc, _ = packetChanges(t, hs, tok)
	if len(rc) != 1 || rc[0].(map[string]any)["relevance"] != domain.RelevanceTouchesAnswer || rc[0].(map[string]any)["relevance_role"] != domain.RolePlanSupport {
		t.Errorf("plan support: %v", rc)
	}
}

func TestPacketIgnoresLiveCaseEditsAndReadsWriteNothing(t *testing.T) {
	pinToday(t, "2026-11-22")
	hs, cs := reviewHarness(t)
	id, tok := frozenLink(t, hs, cs, utc("2026-05-01T00:00:00Z"), "China", []string{"N344694"}, nil)
	first, _ := packetChanges(t, hs, tok) // stamps the first view
	if len(first) != 1 {
		t.Fatalf("changes = %v", first)
	}
	// The owner edits the live case: no citation, a different current setup.
	editCase(t, cs, id, func(c *store.Case) {
		c.Prediction.CitedRulings, c.Prediction.Origin = []string{"N345891"}, "Vietnam"
		c.CurrentOrigin = &planner.KnownOrigin{Country: "China", Ruling: "N345891"}
	})
	before, _ := cs.Get(context.Background(), id)
	again, body1 := packetChanges(t, hs, tok)
	if !reflect.DeepEqual(first, again) {
		t.Errorf("the frozen packet moved with the live case: %v then %v", first, again)
	}
	_, body2 := packetChanges(t, hs, tok)
	if body1 != body2 {
		t.Error("two reads of the packet differ")
	}
	ownerCount(t, hs, id)
	after, _ := cs.Get(context.Background(), id)
	if !reflect.DeepEqual(before, after) {
		t.Error("reading the packet or the review list changed the stored case")
	}
}

func TestOwnerCountKeepsExpiredApprovedAndSkipsRevokedWhilePublicStays410(t *testing.T) {
	pinToday(t, "2026-11-22")
	hs, cs := reviewHarness(t)
	id, tok := frozenLink(t, hs, cs, utc("2026-05-01T00:00:00Z"), "China", []string{"N344694"}, nil)
	editCase(t, cs, id, func(c *store.Case) { // approved, then the link ran out
		c.BrokerReviews[0].Decisions = []store.ReviewDecision{{Decision: decisionApprove, At: utc("2026-05-02T00:00:00Z")}}
		c.BrokerReviews[0].ExpiresAt = time.Now().UTC().Add(-time.Hour)
	})
	if n, ok := ownerCount(t, hs, id); !ok || n != 1.0 {
		t.Errorf("expired approved link: changed_since_shared = %v (%v), want 1", n, ok)
	}
	if rec := serve(hs, "GET", "/v1/review", "", tokenHeader(tok)); rec.Code != http.StatusGone || strings.Contains(rec.Body.String(), "ruling_changes") {
		t.Errorf("expired packet = %d %s, want 410 without changes", rec.Code, rec.Body)
	}
	editCase(t, cs, id, func(c *store.Case) { now := time.Now().UTC(); c.BrokerReviews[0].RevokedAt = &now })
	if _, ok := ownerCount(t, hs, id); ok {
		t.Error("a revoked link carries a count")
	}
	if rec := serve(hs, "GET", "/v1/review", "", tokenHeader(tok)); rec.Code != http.StatusGone {
		t.Errorf("revoked packet = %d", rec.Code)
	}
}

func TestReviewCountPrivacy(t *testing.T) {
	pinToday(t, "2026-11-22")
	hs, cs := reviewHarness(t)
	id, tok := frozenLink(t, hs, cs, utc("2026-05-01T00:00:00Z"), "China", []string{"N344694"}, nil)
	if code, out := hs.doAs(t, "GET", "/v1/cases/"+id+"/reviews", "", visitor(rvBob)); code == http.StatusOK {
		t.Errorf("another visitor reads the review list: %v", out)
	}
	// The broker's own decision view stays what it was: no count in it.
	code, out := hs.doAs(t, "POST", "/v1/review/decision", `{"decision":"approve"}`, tokenHeader(tok))
	if code != http.StatusOK || out["review"].(map[string]any)["changed_since_shared"] != nil {
		t.Errorf("decision view = %d %v", code, out)
	}
	// A pinned demo shows no links to a visitor.
	editCase(t, cs, id, func(c *store.Case) { c.Pinned = true })
	_, got := hs.doAs(t, "GET", "/v1/cases/"+id, "", visitor(rvAlice))
	if rv, ok := got["broker_reviews"]; ok {
		t.Errorf("demo case shows reviews: %v", rv)
	}
}

func TestReviewWithoutChangesIsUnchanged(t *testing.T) {
	hs, _ := reviewHarness(t)
	alice := visitor(rvAlice)
	id := hs.newCase(t, alice)
	tok, rv := makeLink(t, hs, id, `{}`, alice)
	if _, ok := rv["changed_since_shared"]; ok {
		t.Errorf("new link summary: %v", rv)
	}
	if _, body := packetChanges(t, hs, tok); strings.Contains(body, "ruling_changes") {
		t.Error("packet carries ruling_changes with nothing changed")
	}
	if _, ok := ownerCount(t, hs, id); ok {
		t.Error("owner row carries a count with nothing changed")
	}
}
