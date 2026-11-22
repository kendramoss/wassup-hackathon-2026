package mcpserver

import (
	"strings"
	"testing"
)

const caseWithAlerts = `{"id":"case_0123456789ab","display_name":"Heat gun","step":"origin",
 "prediction":{"origin_country":"China","confidence":0.9,"cited_rulings":["N345891","080651"]},
 "ruling_alerts":{"counts":{"final":1,"proposed":1,"read_only":1},"items":[
  {"ruling":"N345891","roles":["cited","plan_support"],"status":"final","kind":"revoked","by":"H346161","date":"2025-05-05","in_effect":true,"note":"revoked by H346161 on 2025-05-05"},
  {"ruling":"080651","roles":["cited"],"status":"proposed","kind":"revoked","by":"H346396","note":"CBP proposed to revoke it (H346396, Customs Bulletin vol. 60 no. 16, p. 58)"},
  {"ruling":"N023537","roles":["read"],"status":"final","kind":"revoked","by":"H314991","date":"2026-11-29","note":"revoked by H314991 effective 2026-11-29"}]}}`

func TestGetCaseWordsRulingAlerts(t *testing.T) {
	api := &stubAPI{body: map[string]string{"GET /v1/cases/case_0123456789ab": caseWithAlerts,
		"GET /v1/cases": `{"cases":[{"id":"case_0123456789ab","display_name":"Heat gun","predicted_origin":"China","step":"origin","ruling_alerts":{"final":1,"proposed":1}}]}`}}
	cs := connect(t, api, false, nil)
	_, txt, out := call(t, cs, "get_case", map[string]any{"case_id": "case_0123456789ab"})
	for _, want := range []string{
		"Ruling N345891 relied on by this case was revoked by H346161 on 2025-05-05. Re-check recommended.",
		"Ruling 080651 relied on by this case: CBP proposed to revoke it",
		"proposed, not final, so information only",
		"1 ruling(s) the prediction read but did not cite have changed since.",
		"The saved answer is unchanged",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in\n%s", want, txt)
		}
	}
	if out["ruling_alerts"] == nil {
		t.Error("structured output dropped ruling_alerts")
	}
	_, txt, _ = call(t, cs, "list_cases", map[string]any{})
	if !strings.Contains(txt, "Changed rulings it relies on: 1 final, 1 proposed.") {
		t.Errorf("list: %s", txt)
	}
}

func TestGetCaseWithoutAlertsSaysNothing(t *testing.T) {
	if l := rulingAlertLines(map[string]any{"id": "x"}); l != "" {
		t.Errorf("%q", l)
	}
}
