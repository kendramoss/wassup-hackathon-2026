package httpapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// alertHarness predicts the heat gun, then rewrites the stored case as an
// answer dated 1 March 2025 that cited N345891 (revoked by H346161 on
// 2025-05-05) and 080651 (a pending proposal), read N023537 (revoked
// effective 2026-11-29), and whose plan rests on N345891 too.
func alertHarness(t *testing.T) (harness, store.Cases, string) {
	t.Helper()
	var st store.Cases
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { st = d.Cases })
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	_, err := st.Update(context.Background(), id, func(c *store.Case) error {
		c.Date = "2025-03-01"
		c.Prediction.CitedRulings = []string{"N345891", "080651"}
		c.Prediction.Precedents = []string{"N345891", "080651", "N023537", "N350002"}
		c.Plan = &planner.Result{Target: "Thailand", Plans: []domain.Plan{{Description: "PCBA in Thailand", Shown: true,
			Verdict: &domain.Verdict{PrecedentSupported: true, SupportingRuling: "N345891", Confidence: 0.93}}}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hs, st, id
}

func pinToday(t *testing.T, day string) {
	t.Helper()
	d, _ := time.Parse(time.DateOnly, day)
	old := alertsNow
	alertsNow = func() time.Time { return d }
	t.Cleanup(func() { alertsNow = old })
}

func TestCaseShowsRevocationAlerts(t *testing.T) {
	pinToday(t, "2026-11-22")
	hs, _, id := alertHarness(t)
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	ra, ok := got["ruling_alerts"].(map[string]any)
	if !ok {
		t.Fatalf("no ruling_alerts: %v", got["ruling_alerts"])
	}
	counts := ra["counts"].(map[string]any)
	if counts["final"] != 1.0 || counts["proposed"] != 1.0 || counts["read_only"] != 1.0 {
		t.Errorf("counts %v", counts)
	}
	items := ra["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("items %v", items)
	}
	first := items[0].(map[string]any)
	if first["ruling"] != "N345891" || first["status"] != "final" || first["by"] != "H346161" || first["in_effect"] != true || first["before_case"] != nil {
		t.Errorf("first %v", first)
	}
	if roles := first["roles"].([]any); len(roles) != 3 {
		t.Errorf("cited, plan support and read merged: %v", roles)
	}
	if third := items[2].(map[string]any); third["ruling"] != "N023537" || third["in_effect"] != nil {
		t.Errorf("scheduled change on a ruling only read: %v", third)
	}
	if ra["rechecked_at"] != nil {
		t.Error("not re-checked yet")
	}
	// The stored answer is untouched.
	if cited := got["prediction"].(map[string]any)["cited_rulings"].([]any); len(cited) != 2 || cited[0] != "N345891" {
		t.Errorf("answer changed: %v", cited)
	}
	_, list := hs.do(t, "GET", "/v1/cases", "")
	row := list["cases"].([]any)[0].(map[string]any)
	badge, _ := row["ruling_alerts"].(map[string]any)
	if badge["final"] != 1.0 || badge["proposed"] != 1.0 || badge["read_only"] != nil {
		t.Errorf("list badge %v", row["ruling_alerts"])
	}
}

func TestCaseWithoutChangedRulingsHasNoAlertField(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t)) // cites N350001, unchanged
	if _, ok := c["ruling_alerts"]; ok {
		t.Errorf("ruling_alerts on a case citing no changed ruling: %v", c["ruling_alerts"])
	}
	_, list := hs.do(t, "GET", "/v1/cases", "")
	if _, ok := list["cases"].([]any)[0].(map[string]any)["ruling_alerts"]; ok {
		t.Error("badge on an unaffected case")
	}
}

func TestWatchRunListsRevocationAlerts(t *testing.T) {
	pinToday(t, "2026-11-22")
	hs, _, id := alertHarness(t)
	if code, _ := hs.do(t, "POST", "/v1/watch/run?alerts_only=true", ""); code != http.StatusForbidden {
		t.Errorf("alerts-only run without the admin token = %d", code)
	}
	calls := hs.engine.calls
	_, run := hs.doAs(t, "POST", "/v1/watch/run?alerts_only=true", "", map[string]string{"X-Admin-Token": testAdmin})
	if hs.engine.calls != calls || run["checked"] != 0.0 {
		t.Errorf("alerts_only re-ran cases: %d calls, %v", hs.engine.calls-calls, run)
	}
	al := run["ruling_alerts"].(map[string]any)[id].(map[string]any)
	if al["final"] != 1.0 || al["proposed"] != 1.0 || len(al["rulings"].([]any)) != 1 {
		t.Errorf("alerts %v", al)
	}
	_, full := hs.doAs(t, "POST", "/v1/watch/run", "", map[string]string{"X-Admin-Token": testAdmin})
	if full["checked"] != 1.0 || full["ruling_alerts"].(map[string]any)[id] == nil {
		t.Errorf("full run %v", full)
	}
}

// Re-check now is the existing per-case watch: afterwards the alert stays
// (the stored answer still cites the ruling) and says when it was re-checked.
func TestRecheckAfterRevocationMarksTheAlert(t *testing.T) {
	pinToday(t, "2026-11-22")
	hs, _, id := alertHarness(t)
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/watch", "")
	if code != http.StatusOK {
		t.Fatalf("watch %d %v", code, out)
	}
	ra := out["ruling_alerts"].(map[string]any)
	if ra["rechecked_at"] == nil {
		t.Errorf("re-checked after the change: %v", ra)
	}
	if cited := out["prediction"].(map[string]any)["cited_rulings"].([]any); cited[0] != "N345891" {
		t.Errorf("a re-check must not replace the stored answer: %v", cited)
	}
}

// A case dated after the revocation that still cites the ruling (stored
// before the guard) is flagged before_case.
func TestAlertOnAnswerStoredBeforeTheGuard(t *testing.T) {
	c := store.Case{Date: "2025-06-01", Prediction: domain.Prediction{CitedRulings: []string{"N345891"}}}
	as := caseRulingAlerts(c, time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC))
	if len(as) != 1 || !as[0].BeforeCase {
		t.Errorf("%+v", as)
	}
	if f := rulingAlertsField(store.Case{Date: "bad", Prediction: domain.Prediction{CitedRulings: []string{"N357771"}}}, time.Now()); f != nil {
		t.Errorf("unchanged ruling: %v", f)
	}
}

// D531: the case read says whether the change bears on the answer. The
// stored answer cites N344694 (China), modified by H344999 (Taiwan).
func TestCaseAlertSaysWhetherTheChangeBearsOnTheAnswer(t *testing.T) {
	pinToday(t, "2026-11-22")
	for _, tc := range []struct{ origin, relevance string }{{"Vietnam", "not_answer"}, {"China", "touches_answer"}} {
		var st store.Cases
		hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { st = d.Cases })
		_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
		id := c["id"].(string)
		if _, err := st.Update(context.Background(), id, func(c *store.Case) error {
			c.Date = "2025-12-22"
			c.Prediction.Origin = tc.origin
			c.Prediction.CitedRulings = []string{"N344694"}
			c.Prediction.Precedents = []string{"N344694"}
			c.Plan = nil
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
		ra, _ := got["ruling_alerts"].(map[string]any)
		items, _ := ra["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("%s: items %v", tc.origin, ra)
		}
		a := items[0].(map[string]any)
		if a["relevance"] != tc.relevance || a["held_before"] != "China" || a["held_after"] != "Taiwan" || a["answer"] != tc.origin {
			t.Errorf("%s: %v", tc.origin, a)
		}
		_, list := hs.do(t, "GET", "/v1/cases", "")
		found := false
		for _, x := range list["cases"].([]any) {
			row := x.(map[string]any)
			if row["id"] != id {
				continue
			}
			found = true
			n, _ := row["ruling_alerts"].(map[string]any)
			if (tc.relevance == "touches_answer") != (n["touches_answer"] == 1.0) {
				t.Errorf("%s: list badge %v", tc.origin, n)
			}
		}
		if !found {
			t.Errorf("%s: case not in the list", tc.origin)
		}
	}
}
