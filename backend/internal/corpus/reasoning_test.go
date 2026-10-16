package corpus

import (
	"strings"
	"testing"
)

func TestReasoningKeepsApplicationDropsBoilerplate(t *testing.T) {
	text := `FACTS: The soap base is made in the UK and cut in Canada.
LAW AND ANALYSIS: The test for determining whether a substantial transformation will occur is whether an article emerges with a new name. See Texas Instruments Inc. v. United States, 681 F.2d 778 (C.C.P.A. 1982).
Regarding the country of origin of the soap, the soap base imparts the essential character. The cutting in Canada does not result in a substantial transformation of the soap base. Therefore the country of origin is the United Kingdom.
The holding set forth above applies only to the specific factual situation.`
	got := Reasoning(text)
	if !strings.Contains(got, "Regarding the country of origin of the soap") || !strings.HasSuffix(got, "substantial transformation of the soap base.") ||
		strings.Contains(got, "Texas Instruments") || strings.Contains(got, "holding set forth") {
		t.Errorf("Reasoning = %q", got)
	}
	if Reasoning("no analysis here") != "" {
		t.Error("expected empty reasoning")
	}
}
