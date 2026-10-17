package planner

import (
	"context"
	"fmt"
	"math"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/nearmiss"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

// Measured costs and latencies behind Estimate (D87), all on gemini-4-argon
// at promo prices (evalkit.PromoPrices). Engine and check figures are from
// the fresh plan searches in eval/results/plan-{heatgun-2022,tublight-2025}-*.json
// (LiteLLM proxy, 17–18 Oct): engine $0.0076–0.0079 per call; a fresh
// precedent check $0.016–0.028 over 3.0–4.0 model calls. Call latency is the
// seeded engine dev run (eval/results/dev-gemini-4-argon-prec-engine-v1-seed-litellm.jsonl):
// p50 6.7 s, p95 9.8 s; verifier calls in those plan searches fit the same
// band (30–51 s for 6 fresh checks). Near-miss advice is 1,700–1,900 tokens
// in and 150 out per call (D81). The panel's band rate is the share of
// checks with confidence in [0.89, 0.95): 31/107 on the dev flip pairs and
// 17/41 on the 2026 pairs (eval/results/flips-*-verifier-v2-seed-*); each
// check in the band runs two more views (D83).
var (
	estEngineCall    = Range{0.0075, 0.0080} // USD per engine call
	estCheck         = Range{0.016, 0.028}   // USD per fresh precedent check
	estCheckCalls    = Range{3.0, 4.0}       // model calls per check
	estNearMissCall  = Range{0.005, 0.006}   // USD per advice call
	estPanelBandRate = Range{0.29, 0.415}    // share of checks the panel re-runs
	estCallSeconds   = Range{6.5, 10.0}      // seconds per model call
	estCachedSeconds = Range{1, 10}          // a replayed search: 1.3 s and 8 s in docs/DEMO.md
	// estFreshCheckShare is the share of moves with no cached prediction
	// that will need a check, i.e. the engine calls them the target: 3/6
	// (tub light) to 6/6 (heat gun).
	estFreshCheckShare = Range{0.5, 1.0}
)

// Range is a low–high estimate.
type Range struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// Estimate is what a plan search would cost before it runs (D87): no model
// call is made. Moves whose engine prediction or verdict is stored cost
// nothing; for the rest the range comes from measured searches.
type Estimate struct {
	Target string `json:"target"`
	Moves  int    `json:"moves"`
	// EngineCalls counts the current setup plus every move; EngineCached of
	// them are stored (D54).
	EngineCalls  int `json:"engine_calls"`
	EngineCached int `json:"engine_cached"`
	// ChecksCached moves have a stored verdict (D50); ChecksSkipped have a
	// stored prediction that misses the target, so no check runs.
	ChecksCached  int `json:"checks_cached"`
	ChecksSkipped int `json:"checks_skipped"`
	// ChecksFresh is how many precedent checks may run (a range: whether a
	// move needs one depends on a prediction not yet made).
	ChecksFresh   Range `json:"checks_fresh"`
	NearMissCalls Range `json:"near_miss_calls"`
	ModelCalls    Range `json:"model_calls"`
	CostUSD       Range `json:"cost_usd"`
	WallSeconds   Range `json:"wall_seconds"`
	// Panel, NearMiss and MaxInFlight are the settings the estimate used.
	Panel       bool   `json:"panel"`
	NearMiss    bool   `json:"near_miss"`
	MaxInFlight int    `json:"max_in_flight"`
	Note        string `json:"note"`
}

// Read-only cache lookups the estimate uses when the planner's collaborators
// offer them (engine.Cache, verifier.Cache, nearmiss.Cache).
type (
	predictionPeeker interface {
		Peek(ctx context.Context, c domain.Case) (engine.Result, bool, error)
	}
	verdictPeeker interface {
		Peek(ctx context.Context, t verifier.Task) (verifier.Result, bool, error)
	}
	advicePeeker interface {
		Has(ctx context.Context, t nearmiss.Task) (bool, error)
	}
)

// Estimate predicts the cost and wall time of Plan(req) from the caches and
// measured figures, without calling a model. Search settings come from
// Options: NearMiss, Panel and MaxInFlight.
func (p *Planner) Estimate(ctx context.Context, req Request) (Estimate, error) {
	if err := req.Validate(); err != nil {
		return Estimate{}, err
	}
	tm, ok := p.templates[req.Family]
	if !ok {
		return Estimate{}, fmt.Errorf("%w: %q", ErrUnknownFamily, req.Family)
	}
	target := domain.NormalizeCountry(req.Target)
	moves := Enumerate(req.Product, tm, target)
	est := Estimate{Target: target, Moves: len(moves), EngineCalls: len(moves) + 1,
		Panel: p.opts.Panel, NearMiss: p.opts.NearMiss != nil, MaxInFlight: p.opts.MaxInFlight}
	pp, _ := p.engine.(predictionPeeker)
	vp, _ := p.verifier.(verdictPeeker)
	ap, _ := p.opts.NearMiss.(advicePeeker)

	var knownA, knownRuling string
	if k := req.CurrentOrigin; k != nil && k.Country != "" {
		knownA, knownRuling = domain.NormalizeCountry(k.Country), k.Ruling
	}
	baseFacts := Render(req.Product, tm)
	peek := func(key, facts string) (engine.Result, bool, error) {
		if pp == nil {
			return engine.Result{}, false, nil
		}
		return pp.Peek(ctx, p.caseFor(req, key, facts))
	}
	if _, ok, err := peek("baseline", baseFacts); err != nil {
		return Estimate{}, err
	} else if ok {
		est.EngineCached++
	}
	unknown := 0   // moves with no stored prediction
	adviceMax := 0 // moves that may still need near-miss advice
	for _, m := range moves {
		facts := Render(Apply(req.Product, m), tm, groupsOf(m)...)
		r, predicted, err := peek(m.Key(), facts)
		if err != nil {
			return Estimate{}, err
		}
		if predicted {
			est.EngineCached++
			if r.Prediction.Origin != target {
				est.ChecksSkipped++
				continue
			}
		}
		// The verdict's key depends only on the facts, so a stored verdict
		// is found even when the prediction is not.
		var vr verifier.Result
		cached := false
		if vp != nil {
			if vr, cached, err = vp.Peek(ctx, verifier.Task{FactsA: baseFacts, FactsB: facts, Target: target, Before: req.Date.Time,
				Exclude: req.Exclude, KnownOriginA: knownA, KnownRulingA: knownRuling}); err != nil {
				return Estimate{}, err
			}
		}
		switch {
		case !cached && !predicted: // a check only if the engine says target
			unknown++
			adviceMax++
			continue
		case !cached:
			est.ChecksFresh.Min++
			est.ChecksFresh.Max++
			adviceMax++
			continue
		}
		est.ChecksCached++
		v := vr.Verdict
		if p.opts.NearMiss != nil && v.PrecedentSupported && v.Confidence >= p.opts.NearMissFloor && v.Confidence < p.opts.PlanConfidence {
			has := false
			if ap != nil {
				if has, err = ap.Has(ctx, nearmiss.Task{Facts: facts, Ruling: v.SupportingRuling, Before: req.Date.Time, Exclude: req.Exclude}); err != nil {
					return Estimate{}, err
				}
			}
			if !has {
				adviceMax++
			}
		}
	}
	est.ChecksFresh.Min += math.Floor(estFreshCheckShare.Min * float64(unknown))
	est.ChecksFresh.Max += math.Ceil(estFreshCheckShare.Max * float64(unknown))
	if p.opts.NearMiss != nil {
		est.NearMissCalls.Max = float64(min(adviceMax, p.opts.NearMissMax))
	}

	engineFresh := float64(est.EngineCalls - est.EngineCached)
	panel := Range{1, 1}
	if p.opts.Panel {
		panel = Range{1 + 2*estPanelBandRate.Min, 1 + 2*estPanelBandRate.Max}
	}
	checkRuns := Range{est.ChecksFresh.Min * panel.Min, est.ChecksFresh.Max * panel.Max}
	est.ModelCalls = Range{
		Min: math.Floor(engineFresh + checkRuns.Min*estCheckCalls.Min + est.NearMissCalls.Min),
		Max: math.Ceil(engineFresh + checkRuns.Max*estCheckCalls.Max + est.NearMissCalls.Max),
	}
	est.CostUSD = Range{
		Min: round3(engineFresh*estEngineCall.Min + checkRuns.Min*estCheck.Min + est.NearMissCalls.Min*estNearMissCall.Min),
		Max: round3(engineFresh*estEngineCall.Max + checkRuns.Max*estCheck.Max + est.NearMissCalls.Max*estNearMissCall.Max),
	}
	est.WallSeconds = Range{Min: p.wall(est, estCallSeconds.Min, est.ModelCalls.Min, false), Max: p.wall(est, estCallSeconds.Max, est.ModelCalls.Max, true)}
	est.Note = "Estimate from measured searches; cached moves cost nothing. A busy model provider (429/504) can make a search several times slower (D79)."
	return est, nil
}

// wall is the search's wall time for calls model calls of perCall seconds:
// the longest chain one move needs (an engine call, then its check's calls,
// then the panel's views and near-miss advice, each after the last), or the
// calls queued through MaxInFlight, whichever is longer.
func (p *Planner) wall(est Estimate, perCall, calls float64, high bool) float64 {
	if calls == 0 {
		if high {
			return estCachedSeconds.Max
		}
		return estCachedSeconds.Min
	}
	chain := 0.0
	if est.EngineCalls > est.EngineCached {
		chain++
	}
	if est.ChecksFresh.Max > 0 && (high || est.ChecksFresh.Min > 0) {
		perCheck := estCheckCalls.Min
		if high {
			perCheck = estCheckCalls.Max
		}
		chain += perCheck
		if p.opts.Panel && high {
			chain += perCheck // the views start once the first check is in
		}
	}
	if est.NearMissCalls.Max > 0 && high {
		chain++
	}
	secs := chain * perCall
	if p.opts.MaxInFlight > 0 {
		secs = math.Max(secs, calls*perCall/float64(p.opts.MaxInFlight))
	}
	return math.Round(secs)
}

func round3(x float64) float64 { return math.Round(x*1000) / 1000 }
