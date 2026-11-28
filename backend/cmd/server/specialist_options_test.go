package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
)

// D970 development selected the missing-fact rule for the pharmaceutical and
// Part 102 engines, whose prompts equal the deployed ones. Textile and Japan are
// not changed by that evidence.
func TestPharmaAndPart102EnginesCarryTheMeasuredMissingFactOption(t *testing.T) {
	cfg := config.Config{EngineModel: "gemini-4-argon", PrecedentK: 5, MaxOutputTokens: 8000}
	p := pharmaOptions(cfg)
	if p.System != engine.PharmaSystem || p.Version != engine.PharmaVersion || p.Model != "gemini-4-argon" || p.K != 5 || p.MaxOutputTokens != 8000 {
		t.Errorf("pharma options changed beyond the rule: %+v", p)
	}
	if !strings.Contains(p.Rules, engine.MissingFactRule) || p.RulesVersion != engine.MissingFactVersion || !p.RequireSingleCountry {
		t.Error("pharma needs the missing-fact rule and the single-country guard")
	}
	u := part102Options(cfg)
	base := engine.USMCAProduction()
	if u.System != base.System || u.Version != base.Version || u.Schema == nil || u.Model != "gemini-4-argon" || u.K != 5 || u.MaxOutputTokens != 8000 {
		t.Errorf("Part 102 options changed beyond the rule: %+v", u)
	}
	if !strings.Contains(u.Rules, engine.MissingFactRule) || u.RulesVersion != engine.MissingFactVersion || u.RequireSingleCountry {
		t.Error("Part 102 needs the rule and no single-country guard (rules (c) and (d) can name several countries)")
	}
}

func TestMissingFactOptionIsWiredOnlyForPharmaAndPart102(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "specialist_options.go" {
			continue
		}
		b, _ := os.ReadFile(f)
		for _, bad := range []string{"WithMissingFactRule", "TextileSystem3", "WithUnknownStatusRule", "RequireSingleCountry"} {
			if strings.Contains(string(b), bad) {
				t.Errorf("%s uses %s: textile and Japan constructors must stay as deployed", f, bad)
			}
		}
	}
	b, _ := os.ReadFile("main.go")
	for _, want := range []string{"pharmaOptions(cfg)", "part102Options(cfg)", "engine.TextileSystem2", "engine.TextileSystem,"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("main.go lost %s", want)
		}
	}
}
