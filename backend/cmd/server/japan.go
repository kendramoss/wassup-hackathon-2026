package main

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpengine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jurisdiction"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/spend"
)

// japanModel names the same gateway model used for the Japan evaluation.
const japanModel = jpengine.DefaultModel

// japanAuthority registers the Japan authority. Japan is unavailable when
// JAPAN_ENGINE is off. The fake provider uses an offline fixture. Dedicated
// JAPAN_LITELLM_* settings (either one set) keep the validated private
// gateway path with japanModel. Otherwise, with the vertex provider, Japan
// reuses the production engineModel (already gated, retried and metered)
// and cfg.EngineModel; no new client is built. With no usable model Japan
// stays unavailable (fail closed).
func japanAuthority(cfg config.Config, engineModel llm.Model, meter *spend.Meter, logger *slog.Logger) (jurisdiction.Authority, error) {
	if !cfg.JapanEngine {
		return jurisdiction.Authority{}, nil
	}
	var model llm.Model
	modelID := japanModel
	if cfg.LLMProvider == config.ProviderFake {
		// A case with a parts list and no answers yet comes back undetermined,
		// so the questions of D917 can be tried locally; an answer settles it.
		model = &llm.Fake{Respond: func(r llm.Request) (llm.Response, error) {
			if in := r.Messages[len(r.Messages)-1].Content; strings.Contains(in, "\nMaterial: ") && !strings.Contains(in, "\nA: ") {
				return llm.Response{Text: `{"outcome":"undetermined","criterion":"NONE","origin_country":"","deciding_rule":"Change of tariff heading","confidence":0.5,"reasoning":"Illustrative result from the local fake model: the parts' origin status is not given.","cited_rulings":[]}`}, nil
			}
			return llm.Response{Text: `{"outcome":"qualifies","criterion":"CTH","origin_country":"Vietnam","deciding_rule":"Change of tariff heading","confidence":0.74,"reasoning":"Illustrative result from the local fake model.","cited_rulings":[]}`}, nil
		}}
	} else {
		base := strings.TrimSpace(os.Getenv("JAPAN_LITELLM_BASE_URL"))
		key := strings.TrimSpace(os.Getenv("JAPAN_LITELLM_API_KEY"))
		if base == "" && key == "" {
			if cfg.LLMProvider != config.ProviderVertex || engineModel == nil || strings.TrimSpace(cfg.EngineModel) == "" {
				return jurisdiction.Authority{}, nil
			}
			model, modelID = engineModel, cfg.EngineModel
			return newJapanAuthority(model, modelID)
		}
		u, err := url.Parse(base)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || key == "" {
			return jurisdiction.Authority{}, fmt.Errorf("japan gateway requires an HTTPS URL and an API key")
		}
		raw := llm.NewLiteLLM(base, key, cfg.LLMTimeout)
		gate := llm.NewGate(cfg.LLMMaxInFlight, cfg.LLMCoolDown)
		paced := llm.WithPace(meter.Attempts(raw), cfg.LLMCallGap, gate)
		model = llm.WithLogging(llm.WithRetry(paced, cfg.LLMMaxRetries, time.Second), logger)
	}
	return newJapanAuthority(model, modelID)
}

func newJapanAuthority(model llm.Model, modelID string) (jurisdiction.Authority, error) {
	japan, err := jpengine.NewDefault(model, modelID, jpengine.DefaultK)
	if err != nil {
		return jurisdiction.Authority{}, err
	}
	return jurisdiction.Authority{
		Market:     jurisdiction.JP,
		Agency:     "Japan Customs",
		Rules:      "Preferential origin under the named trade agreement",
		Precedents: "Japan Customs advance rulings",
		Engine:     japan,
	}, nil
}
