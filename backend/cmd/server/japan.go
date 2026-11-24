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

// japanAuthority registers a gateway-backed authority or the offline fixture.
// Japan is unavailable unless JAPAN_ENGINE=on: gateway settings alone never
// make it public, and nothing falls back to Vertex.
func japanAuthority(cfg config.Config, meter *spend.Meter, logger *slog.Logger) (jurisdiction.Authority, error) {
	if !cfg.JapanEngine {
		return jurisdiction.Authority{}, nil
	}
	var model llm.Model
	if cfg.LLMProvider == config.ProviderFake {
		model = &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
			return llm.Response{Text: `{"outcome":"qualifies","criterion":"CTH","origin_country":"Vietnam","deciding_rule":"Change of tariff heading","confidence":0.74,"reasoning":"Illustrative result from the local fake model.","cited_rulings":[]}`}, nil
		}}
	} else {
		base := strings.TrimSpace(os.Getenv("JAPAN_LITELLM_BASE_URL"))
		key := strings.TrimSpace(os.Getenv("JAPAN_LITELLM_API_KEY"))
		if base == "" && key == "" {
			return jurisdiction.Authority{}, nil
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
	japan, err := jpengine.NewDefault(model, japanModel, jpengine.DefaultK)
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
