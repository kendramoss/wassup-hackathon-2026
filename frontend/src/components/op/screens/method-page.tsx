import Link from "next/link";
import { Main } from "../chrome";
import { MethodArchitecture, MethodExperiments } from "../method-architecture";
import { blob } from "@/lib/method";
import { kobalt, kobaltSources } from "@/lib/method-inspector";

function MethodGuardrails() {
  return (
    <section className="mi-section" aria-labelledby="mi-guardrails">
      <header className="mi-section-title"><h2 id="mi-guardrails">Safeguards</h2></header>
      <div className="mi-triple-grid">
        <article id="clean" className="mi-card" data-tone="evidence">
          <h3>Only earlier rulings</h3>
          <figure className="mi-figure" aria-labelledby="mi-timeline">
            <figcaption id="mi-timeline">Illustrated 2022 cutoff</figcaption>
            <dl className="mi-timeline">
              <div><dt>Earlier</dt><dd>Eligible</dd></div>
              <div><dt>{kobalt.setupRuling.label}</dt><dd>Cutoff</dd></div>
              <div><dt>Later</dt><dd>Blocked</dd></div>
            </dl>
            <p className="read mi-note">Same-day rulings are blocked. {kobalt.supporting.id} and {kobalt.backtestRuling.id} are later. The saved demo uses {kobalt.cutoff.label}.</p>
          </figure>
          <p className="read mi-note">The Jan–Sep 2026 test set of 377 CBP rulings was frozen before scoring.</p>
          <a className="mi-link" href={blob("docs/EVALUATION.md#2-how-the-validation-and-test-sets-are-built")}>Blind holdout protocol <span aria-hidden="true">↗</span></a>
        </article>

        <article id="plans" className="mi-card" data-tone="evidence">
          <h3>Rulings for and against</h3>
          <div className="mi-figure">
            <dl className="mi-evidence-pair">
              <div>
                <dt>For</dt>
                <dd><a href={"https://rulings.cbp.gov/ruling/" + kobalt.supporting.id}>{kobalt.supporting.id}</a></dd>
              </div>
              <div>
                <dt>Against</dt>
                <dd>{kobalt.counterRuling === null ? "0 found" : kobalt.counterRuling}</dd>
              </div>
            </dl>
            <div className="mi-score">
              <strong>{kobalt.verifierScore.toFixed(2)}</strong>
              <span>Shown · bar {kobalt.planBar.toFixed(2)}</span>
            </div>
            <p className="read mi-note">Later replay using the {kobalt.supporting.label} ruling. No qualifying counter-ruling recorded.</p>
          </div>
          <a href={blob(kobaltSources.plan)} className="mi-link">Recorded plan <span aria-hidden="true">↗</span></a>
        </article>

        <article className="mi-card" data-tone="evidence">
          <h3>Fixed rule check</h3>
          <div className="mi-figure">
            <p>19 CFR 102.20</p>
            <p className="read mi-note">Checks whether tariff codes change as the rule requires. Missing inputs leave it undetermined.</p>
          </div>
          <Link className="mi-link" href="/examples/rule-check">Recorded rule check <span aria-hidden="true">→</span></Link>
        </article>
      </div>
      <p className="read mi-footnote">A move needs eligible support to clear the gate. Only CBP’s ruling binds.</p>
    </section>
  );
}

export function MethodPage() {
  return (
    <Main className="mi">
      <header id="model" className="mi-hero">
        <p className="eyebrow">Method</p>
        <h1>How an answer is made and checked</h1>
        <p className="read mi-lead">The live service uses Gemini without fine-tuning. Each answer retrieves only CBP rulings dated before the case.</p>
        <details className="mi-limits">
          <summary>What this does not show</summary>
          <p className="read mi-note">Gemini’s pretraining exposure is unknown: we can’t tell which rulings it saw in training. The ruling search (BM25) weighs words by counts taken from every ruling, later ones included. Earlier test rulings can serve as precedents for later ones.</p>
        </details>
      </header>

      <MethodArchitecture />
      <MethodGuardrails />
      <MethodExperiments />

      <section id="check" className="mi-section mi-sources" aria-labelledby="mi-sources">
        <header className="mi-section-title"><h2 id="mi-sources">Check the evidence</h2></header>
        <div className="mi-source-grid">
          <Link className="mi-link" href="/accuracy">Benchmarks and known failures <span aria-hidden="true">→</span></Link>
          <a className="mi-link" href={blob("docs/EVALUATION.md#15-a-held-out-part-102-plan-set-22-nov-2026-d570-d571")}>Part 102 evaluation <span aria-hidden="true">↗</span></a>
          <a className="mi-link" href={blob("docs/ARCHITECTURE.md")}>Architecture and source <span aria-hidden="true">↗</span></a>
          <Link className="mi-link" href="/examples/plan-rule-check">Move rule check <span aria-hidden="true">→</span></Link>
          <Link className="mi-link" href="/examples/part102-plan">Part 102 plan <span aria-hidden="true">→</span></Link>
          <Link className="mi-link" href="/examples/revocation-alert">Revocation alert <span aria-hidden="true">→</span></Link>
          <a className="mi-link" href={blob("docs/NUMBERS.md#market-and-time-from-hackathon-researchevidencefeesledgermd")}>Fig. 4 before figures <span aria-hidden="true">↗</span></a>
          <a className="mi-link" href={blob("eval/results/plan-heatgun-2022-local-litellm.json")}>Recorded heat-gun plan, 33 s (proxy run, demo fixture) <span aria-hidden="true">↗</span></a>
          <a className="mi-link" href={blob("docs/NUMBERS.md#evaluation-results-page--served-by-get-v1evalsummary")}>Cost per prediction, $0.0078 promo ($0.0157 list) <span aria-hidden="true">↗</span></a>
        </div>
        <details className="mi-limits">
          <summary>Where the time and cost figures come from</summary>
          <p className="read mi-note">21 days: the team’s count of 247 Asian-origin CBP rulings, median to a binding CBP ruling.</p>
          <p className="read mi-note">20 hours: CBP’s estimate across all ruling subjects, not only origin.</p>
          <p className="read mi-note">About $3,600: median of 23 subsidized Korean exporter purchases (KOTRA export-voucher records).</p>
          <p className="read mi-note">These are not like-for-like populations and not a universal broker fee. Unde’s figures are non-binding predictions. Only CBP’s ruling binds. The savings story is illustrative.</p>
        </details>
        <p className="read mi-note">The Kobalt backtest helped shape the search. It was not a blind test.</p>
      </section>
    </Main>
  );
}
