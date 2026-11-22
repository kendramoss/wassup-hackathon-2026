package mcpserver

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

const unsupported = `{"id":"case_0123456789ab","jurisdiction":"eu","jurisdiction_detail":{"code":"eu","name":"European Union","source":"chosen","supported":false},"prediction":{"origin_country":"","confidence":0},"triage":{"route":"needs_review","reason":"Origin rules for the European Union aren't supported yet"}}`

// predict_origin passes the market through and, for a market with no
// engine, says so and gives no origin (D680).
func TestPredictPassesTheMarketAndNamesUnsupportedOnes(t *testing.T) {
	api := &stubAPI{body: map[string]string{"POST /v1/cases/predict": unsupported}, code: map[string]int{"POST /v1/cases/predict": 201}}
	cs := connect(t, api, false, map[string]string{"Authorization": "Bearer op_abc"})
	res, txt, out := call(t, cs, "predict_origin", map[string]any{"facts": "A heat gun assembled in Thailand from Chinese parts.", "jurisdiction": "eu"})
	if res.IsError {
		t.Fatal(txt)
	}
	var body map[string]any
	b, _ := io.ReadAll(api.last().Body)
	_ = json.Unmarshal(b, &body)
	if body["jurisdiction"] != "eu" {
		t.Errorf("jurisdiction not forwarded: %s", b)
	}
	if !strings.Contains(txt, "Market: European Union. Origin rules for the European Union aren't supported yet. No origin was predicted.") || strings.Contains(txt, "Likely origin") {
		t.Errorf("summary: %s", txt)
	}
	if out["jurisdiction"] != "eu" {
		t.Errorf("structured content: %v", out)
	}
	if res, _, _ := call(t, cs, "predict_origin", map[string]any{"facts": "A heat gun assembled in Thailand.", "jurisdiction": "fr"}); !res.IsError {
		t.Error("an unknown market passed the schema")
	}
}
