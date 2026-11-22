package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/triage"
)

// RegimeGeneral is the regime of the general (substantial transformation)
// engine. The router has no constant for it.
const RegimeGeneral = "substantial_transformation"

// ScopeState says what is known about the legal regime of a dispatched case.
// An empty router label is never read as a confirmed general regime.
type ScopeState string

const (
	// ScopeNoRouter: no router is configured (deployment choice).
	ScopeNoRouter ScopeState = "no_router"
	// ScopeConfiguredFailed: a configured router errored (D98).
	ScopeConfiguredFailed ScopeState = "configured_failed"
	// ScopeConfirmed: the router (or a pinned stored label) gave a regime.
	ScopeConfirmed ScopeState = "confirmed"
	// ScopeLabelMissing: a pinned dispatch had no stored label (a legacy
	// case); the general engine answers and no regime is confirmed.
	ScopeLabelMissing ScopeState = "label_missing"
)

// SpecialistState is the outcome of the selected specialist engine.
type SpecialistState string

const (
	SpecialistNone         SpecialistState = ""             // no specialist regime selected
	SpecialistOK           SpecialistState = "ok"           // the specialist answered
	SpecialistFailed       SpecialistState = "failed"       // enabled, but it errored
	SpecialistUnconfigured SpecialistState = "unconfigured" // selected regime has no enabled engine
)

// LabelMissingReason is the triage reason for a pinned dispatch with no stored
// router label (a legacy case): the regime is unknown, so it never auto-accepts.
const LabelMissingReason = "scope unavailable: the case has no stored scope label, so the legal regime is not confirmed; an expert reviews"

// SpecialistUnavailableReason is the triage reason when the engine of the
// regime the router selected errored. The prediction is then a provisional
// general one; the regime's rules were not applied.
func SpecialistUnavailableReason(regime string) string {
	return fmt.Sprintf("specialist unavailable: the %s engine failed, so the legal regime is not applied and the prediction is a provisional general one; an expert reviews", regime)
}

// Dispatch is the routing and engine selection for one case, before triage.
type Dispatch struct {
	// Result is what supplies the prediction: the specialist's when it
	// answered, else the general engine's (provisional when Specialist is
	// failed or unconfigured).
	Result engine.Result
	// Label is the router label; nil unless Scope is ScopeConfirmed.
	Label *router.Label
	Scope ScopeState
	// Selected is the specialist regime the label chose ("" for general),
	// whether or not its engine is enabled.
	Selected   string
	Specialist SpecialistState
}

// Regime is the regime whose engine actually produced Result.
func (d Dispatch) Regime() string {
	if d.Specialist == SpecialistOK {
		return d.Selected
	}
	return RegimeGeneral
}

// PredictionRegime reports the regime whose engine produced a stored
// prediction, from its trace prompt version. It is "" when the version is not
// one of the known engine versions (callers must treat "" as unknown, never
// as the router label or as general). Unlike the router label, it stays true after a
// specialist failure left a provisional general prediction on a
// specialist-labelled case.
//
// Known namespace: the base versions engine.PromptVersion (general),
// TextileVersion and TextileVersion2 (textile), USMCAVersion and USMCAVersion2, PharmaVersion,
// each optionally followed by "+<suffix>" (rules, precedent reasoning). A new
// prompt version must be added here; it is never guessed from a prefix.
func PredictionRegime(p domain.Prediction) string {
	base, _, _ := strings.Cut(p.Trace.PromptVersion, "+")
	switch base {
	case engine.PromptVersion:
		return RegimeGeneral
	case engine.TextileVersion, engine.TextileVersion2:
		return router.RegimeTextile
	case engine.USMCAVersion, engine.USMCAVersion2:
		return router.RegimeUSMCA
	case engine.PharmaVersion:
		return router.RegimePharma
	}
	return ""
}

// pinned makes dispatch skip the router and use a stored label (nil: the
// case has none; ScopeLabelMissing, which decide always sends to review with
// LabelMissingReason). For the watch re-check (#294).
type pinned struct{ label *router.Label }

func (a *api) specialist(regime string) Predictor {
	switch regime {
	case router.RegimeTextile:
		return a.d.Textile
	case router.RegimeUSMCA:
		return a.d.USMCA
	case router.RegimePharma:
		return a.d.Pharma
	}
	return nil
}

func isSpecialist(regime string) bool {
	return regime == router.RegimeTextile || regime == router.RegimeUSMCA || regime == router.RegimePharma
}

// dispatch routes the case and picks the engine. The general engine starts
// beside the router so the general route loses no time. If the router then
// selects an enabled specialist, that speculative call is cancelled, its
// error ignored, and the specialist runs; the general engine is called again
// only if the specialist fails (provisional answer). A general error is
// returned only when nothing else can answer. st may be nil.
func (a *api) dispatch(ctx context.Context, c domain.Case, st *stages, pin *pinned) (Dispatch, error) {
	d := Dispatch{Scope: ScopeNoRouter}
	general := func(cx context.Context) (engine.Result, error) {
		if st != nil {
			cx = engine.WithProgress(cx, st.engineProgress)
		}
		res, err := a.d.Engine.Predict(cx, c)
		if err == nil {
			st.end(engine.StageEngine) // a cached prediction reports no stages of its own
			if a.d.Calibration != nil {
				res.Prediction = a.d.Calibration.Apply(res.Prediction)
			}
		}
		return res, err
	}
	gctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		gdone chan struct{} // non-nil while a speculative general call exists
		gres  engine.Result
		gerr  error
	)
	startGeneral := func() {
		gdone = make(chan struct{})
		go func() {
			defer close(gdone)
			gres, gerr = general(gctx)
		}()
	}
	var label *router.Label
	switch {
	case pin != nil:
		if pin.label != nil {
			l := *pin.label
			label, d.Scope = &l, ScopeConfirmed
		} else {
			d.Scope = ScopeLabelMissing
		}
	case a.d.Router != nil:
		routerFailed := false
		done := make(chan struct{})
		st.begin("router")
		go func() {
			defer close(done)
			defer st.end("router") // a failed router is done too: scope unknown
			l, err := a.d.Router.Classify(ctx, c.Facts)
			if err != nil {
				a.d.Logger.WarnContext(ctx, "router failed; scope unknown", "request_id", requestRef(ctx), "err", logsafe.Err(err))
				routerFailed = true // read after done
				return
			}
			label = &l
		}()
		startGeneral()
		<-done
		if routerFailed {
			d.Scope = ScopeConfiguredFailed
		} else {
			d.Scope = ScopeConfirmed
		}
	default:
		startGeneral()
	}
	d.Label = label
	if label != nil && isSpecialist(label.Regime) {
		d.Selected = label.Regime
	}
	spec := a.specialist(d.Selected)
	if d.Selected != "" && spec == nil {
		d.Specialist = SpecialistUnconfigured
	}
	if spec != nil {
		have := false
		if gdone != nil {
			cancel() // not needed unless the specialist fails
			<-gdone
			have = gerr == nil
			if !have {
				st.settle() // the cancelled call may have left "engine" running
			}
		}
		sctx := ctx
		if st != nil {
			sctx = engine.WithProgress(ctx, st.engineProgress)
		}
		sres, serr := spec.Predict(sctx, c)
		st.settle() // the specialist reports its own stages; close them if it failed
		if serr == nil {
			d.Result, d.Specialist = sres, SpecialistOK
			return d, nil
		}
		a.d.Logger.WarnContext(ctx, "specialist engine failed; provisional general prediction needs review", "request_id", requestRef(ctx), "regime", d.Selected, "err", logsafe.Err(serr))
		d.Specialist = SpecialistFailed
		if !have {
			var err error
			if gres, err = general(ctx); err != nil {
				return Dispatch{}, fmt.Errorf("%s engine failed (%w) and the general engine failed (%w)", d.Selected, serr, err)
			}
		}
		d.Result = gres
		return d, nil
	}
	if gdone == nil {
		startGeneral()
	}
	<-gdone
	if gerr != nil {
		return Dispatch{}, gerr
	}
	d.Result = gres
	return d, nil
}

// decide is the single triage policy site. inScope is false when clarify
// must not run (out of scope, specialist regimes, scope unavailable).
// Family bars and the fallback-model check keep their existing order.
func (a *api) decide(d Dispatch) (domain.Triage, bool) {
	p := d.Result.Prediction
	switch d.Specialist {
	case SpecialistOK:
		// Every applicable gate must pass (#300): the regime's own bar, and
		// the fallback bar when the requested identity is not the primary.
		// Neither fast path overrides the other.
		var tr domain.Triage
		switch d.Selected {
		case router.RegimeTextile:
			tr = triage.Textile(p, a.d.TextileFastPath)
		case router.RegimeUSMCA:
			tr = triage.USMCA(p, a.d.USMCAFastPath)
		default:
			tr = triage.Pharma(p, a.d.PharmaFastPath)
		}
		if triage.AnsweredByFallback(p.Trace, a.d.EngineModel) {
			tr = triage.AllGates(tr, triage.Fallback(p, a.d.FallbackFastPath, true, ""))
		}
		return tr, false
	case SpecialistFailed:
		return domain.Triage{Route: domain.RouteNeedsReview, Reason: SpecialistUnavailableReason(d.Selected)}, false
	}
	if d.Scope == ScopeLabelMissing {
		return domain.Triage{Route: domain.RouteNeedsReview, Reason: LabelMissingReason}, false
	}
	if d.Scope == ScopeConfiguredFailed {
		// Scope was not established: keep the provisional prediction, never
		// auto-accept, and skip clarify (inScope false), which would turn the
		// review into questions as if the case were known to be in scope.
		return domain.Triage{Route: domain.RouteNeedsReview, Reason: ScopeUnavailableReason}, false
	}
	// A selected regime without an enabled engine (SpecialistUnconfigured)
	// is out of scope here: needs_review, never auto-accepted (D61/D62).
	inScope, scopeReason, subfamily := true, "", ""
	if d.Label != nil {
		inScope, scopeReason = d.Label.Scope()
		subfamily = d.Label.Subfamily
	}
	// The fallback-model guard runs before the family bars (#300): a family bar
	// must not accept what the fallback model answered unless FALLBACK_FAST_PATH
	// allows it.
	if triage.AnsweredByFallback(p.Trace, a.d.EngineModel) {
		return triage.Fallback(p, a.d.FallbackFastPath, inScope, scopeReason), inScope
	}
	if a.d.FamilyBars != nil {
		return triage.DecideByFamily(p, *a.d.FamilyBars, subfamily, a.d.FastPathConf, inScope, scopeReason), inScope
	}
	if a.d.Calibration != nil {
		return triage.DecideCalibrated(p, a.d.CalibratedFastPath, a.d.FastPathConf, inScope, scopeReason), inScope
	}
	return triage.Decide(p, a.d.FastPathConf, inScope, scopeReason), inScope
}

// Classification is a dispatch with its triage decision.
type Classification struct {
	Dispatch
	Triage  domain.Triage
	InScope bool // false: clarify does not apply
}

// Predictor is the exported entry to the same dispatch and triage the API
// uses, for callers outside this package (the case agent's what_if, #319). It
// satisfies caseagent.Predictor. Build it from the Deps before the Agent is
// attached to them (the agent does not take part in dispatch).
type CasePredictor struct{ a *api }

// NewCasePredictor returns a CasePredictor over d's engines, router and bars.
func NewCasePredictor(d Deps) *CasePredictor {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &CasePredictor{a: &api{d: d}}
}

// Classify routes the case, picks the engine and decides triage.
func (p *CasePredictor) Classify(ctx context.Context, c domain.Case) (Classification, error) {
	d, err := p.a.dispatch(ctx, c, nil, nil)
	if err != nil {
		return Classification{}, err
	}
	tr, in := p.a.decide(d)
	return Classification{Dispatch: d, Triage: tr, InScope: in}, nil
}

// Predict returns the dispatched engine result (caseagent.Predictor). It
// hides triage and whether the result is a provisional general one after a
// specialist failure; a caller that shows or acts on the result (#319 Ask
// what_if) must use Classify and surface Triage and Specialist.
func (p *CasePredictor) Predict(ctx context.Context, c domain.Case) (engine.Result, error) {
	cl, err := p.Classify(ctx, c)
	return cl.Result, err
}
