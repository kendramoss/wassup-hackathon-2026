package evalkit

import (
	"fmt"
	"slices"
	"strings"
)

// Markdown renders a summary as a results section. Runs through the LiteLLM
// proxy are labelled "proxy, development only".
func Markdown(title, split string, s Summary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n", title)
	if slices.Contains(s.Providers, "litellm") {
		b.WriteString("**Proxy, development only** (LiteLLM). Not an official number; re-run on Vertex AI.\n\n")
	}
	fmt.Fprintf(&b, "- Split: `%s` · provider: %s · model: %s · prompt: %s\n", split,
		strings.Join(s.Providers, ", "), strings.Join(s.Models, ", "), strings.Join(s.PromptVersions, ", "))
	fmt.Fprintf(&b, "- **Accuracy: %d / %d = %.1f%% (95%% CI %.1f–%.1f)**", s.Correct, s.N, 100*s.Acc, 100*s.Lo, 100*s.Hi)
	if s.Errors > 0 {
		fmt.Fprintf(&b, " — %d failed calls counted as wrong", s.Errors)
	}
	b.WriteString("\n")
	for _, m := range []string{"regex", "manual"} {
		if v, ok := s.ByMethod[m]; ok {
			p, lo, hi := Wilson(v[1], v[0])
			fmt.Fprintf(&b, "  - %s-labelled: %d / %d = %.1f%% (%.1f–%.1f)\n", m, v[1], v[0], 100*p, 100*lo, 100*hi)
		}
	}
	fp, flo, fhi := Wilson(s.FastK, s.FastN)
	fmt.Fprintf(&b, "- Fast path (triage auto: confidence at or above the bar and in scope): covers %d / %d = %.1f%%, accuracy %.1f%% (%.1f–%.1f)\n",
		s.FastN, s.N, pct(s.FastN, s.N), 100*fp, 100*flo, 100*fhi)
	fmt.Fprintf(&b, "- Cost per case: $%.4f (promo) / $%.4f (list) · mean tokens in %.0f / out %.0f · latency p50 %.1f s / p95 %.1f s\n\n",
		s.CostPerCase, s.CostPerCaseList, s.MeanInTok, s.MeanOut, float64(s.LatencyP50)/1000, float64(s.P95)/1000)
	b.WriteString("| Confidence | n | correct | accuracy (95% CI) |\n|---|---:|---:|---|\n")
	rows := append(append([]Bin{}, s.Bins...), s.AtLeast95, s.AtLeast90)
	for i, bin := range rows {
		label := bin.Label
		if i >= len(s.Bins) {
			label = "cumulative " + label
		}
		p, lo, hi := Wilson(bin.K, bin.N)
		fmt.Fprintf(&b, "| %s | %d | %d | %.1f%% (%.1f–%.1f) |\n", label, bin.N, bin.K, 100*p, 100*lo, 100*hi)
	}
	return b.String()
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}
