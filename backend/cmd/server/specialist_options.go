package main

import (
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
)

// pharmaOptions are the deployed pharmaceutical engine's options (pharma-v1,
// K and token limit from the configuration) plus the missing-fact rule and the
// single-country guard selected on development in the D970 specialist study.
func pharmaOptions(cfg config.Config) engine.Options {
	return engine.WithMissingFactRule(engine.Options{Model: cfg.EngineModel, K: cfg.PrecedentK, MaxOutputTokens: cfg.MaxOutputTokens,
		System: engine.PharmaSystem, Version: engine.PharmaVersion}, true)
}

// part102Options are the deployed Part 102 engine's options (usmca-v5, D602)
// plus the missing-fact rule, without the single-country guard: rules (c) and
// (d) of 102.11 can name several countries.
func part102Options(cfg config.Config) engine.Options {
	o := engine.USMCAProduction()
	o.Model, o.K, o.MaxOutputTokens = cfg.EngineModel, cfg.PrecedentK, cfg.MaxOutputTokens
	return engine.WithMissingFactRule(o, false)
}
