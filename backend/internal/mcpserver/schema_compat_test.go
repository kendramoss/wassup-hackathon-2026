package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"
)

// D390: a Google ADK agent offered all of Unde's tools through an
// OpenAI-compatible LiteLLM gateway to Gemini got a 400 for the whole
// request. The cause was predict_origin's root
// anyOf: [{required: [facts]}, {required: [product]}]; each tool alone was
// accepted, and predict_origin was accepted once that anyOf was removed.
// Function-calling clients translate the root of an input schema into their
// own parameter object, so no tool may put anyOf, oneOf, allOf or not there.
// "facts or product" is checked in the handler instead.
func TestInputSchemasHaveNoRootComposition(t *testing.T) {
	cs := connectOpts(t, Options{API: &stubAPI{}, RulingSearch: true, MediaExtract: true})
	lt, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(lt.Tools) < 13 {
		t.Fatalf("listed %d tools, want every tool", len(lt.Tools))
	}
	for _, tool := range lt.Tools {
		b, _ := json.Marshal(tool.InputSchema)
		var root map[string]any
		if err := json.Unmarshal(b, &root); err != nil {
			t.Fatalf("%s: %v", tool.Name, err)
		}
		if root["type"] != "object" {
			t.Errorf("%s: root type %v, want object", tool.Name, root["type"])
		}
		for _, k := range []string{"anyOf", "oneOf", "allOf", "not"} {
			if _, ok := root[k]; ok {
				t.Errorf("%s: root of the input schema has %s", tool.Name, k)
			}
		}
	}
}

func TestPredictNeedsFactsOrProductBeforeAnyAPICall(t *testing.T) {
	api := &stubAPI{body: map[string]string{"POST /v1/cases/predict": predicted}}
	cs := connect(t, api, false, nil)
	for _, args := range []map[string]any{{}, {"family": "motorised_tools_small_appliances"}} {
		res, txt, out := call(t, cs, "predict_origin", args)
		if !res.IsError || !strings.Contains(txt, "facts") || obj(out["error"])["code"] != "invalid_input" {
			t.Errorf("args %v: error %v %q %v", args, res.IsError, txt, out)
		}
	}
	if len(api.seen) != 0 {
		t.Errorf("%d API calls made for an empty prediction", len(api.seen))
	}
	if res, txt, _ := call(t, cs, "predict_origin", map[string]any{"facts": "A heat gun assembled in Thailand from Chinese parts."}); res.IsError {
		t.Fatal(txt)
	}
}
