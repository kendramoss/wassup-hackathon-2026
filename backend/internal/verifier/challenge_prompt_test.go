package verifier

import (
	"strings"
	"testing"
)

func TestFunctionalContrastPromptHasSeparateCacheIdentity(t *testing.T) {
	for _, view := range []string{"", ViewSkeptic, ViewProduct} {
		v := New(nil, nil, Options{Prompt: "v4", View: view})
		want := "verifier-v4"
		if view != "" {
			want += "-" + view
		}
		if v.Version() != want {
			t.Fatalf("view %q version=%q want %q", view, v.Version(), want)
		}
		if v.Version() == New(nil, nil, Options{Prompt: "v2", View: view}).Version() {
			t.Fatal("new instructions share legacy cache identity")
		}
	}
}

func TestFunctionalContrastPromptKeepsEvidenceBurdenAndViews(t *testing.T) {
	for _, view := range []string{"", ViewSkeptic, ViewProduct} {
		s := System4(6, view)
		for _, part := range []string{"at most 6 calls", "supporting_ruling", "confidence", "COUNTER-SEARCH", "functional", "reverse", "missing", "earlier ruling"} {
			if !strings.Contains(s, part) {
				t.Errorf("view %q lacks %q", view, part)
			}
		}
		if view != "" && !strings.Contains(s, viewText[view]) {
			t.Errorf("view %q lost its perspective", view)
		}
	}
}
