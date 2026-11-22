import Link from "next/link";
import "./judge-proof.css";

export function JudgeProof() {
  return (
    <section id="proof" className="judge-section" aria-labelledby="f3h">
      <header className="judge-heading">
        <p className="judge-figure">Fig. 3</p>
        <h2 id="f3h">Accuracy and reliability</h2>
      </header>

      <div className="judge-kpis">
        <article className="judge-card judge-kpi">
          <p className="judge-number">90.2%</p>
          <h3>Overall accuracy</h3>
          <p>vs. <b>63.4%</b> for the first country named</p>
        </article>
        <article className="judge-card judge-kpi">
          <p className="judge-number">96.0%</p>
          <h3>High-confidence accuracy</h3>
          <p>79.6% auto-accepted</p>
          <p className="judge-badge judge-review">20.4% routed to human review</p>
        </article>
        <article className="judge-card judge-kpi">
          <p className="judge-number">9/9</p>
          <h3>Plan precision</h3>
          <p>0 false positives across 18 CBP before/after pairs, with the starting origin given</p>
        </article>
      </div>

      <div className="judge-badges">
        <Link className="judge-badge" href="/method#clean">Blind holdout · 377 Jan–Sep 2026 rulings</Link>
        <Link className="judge-badge" href="/method#model">Zero fine-tuning</Link>
        <Link className="judge-badge" href="/accuracy#misses">Inspect all 37 failures →</Link>
      </div>
      <details id="benchmark-method" className="judge-details">
        <summary>Methodology &amp; Confidence Intervals</summary>
        <div className="judge-details-body read">
          <p>
            The Vertex AI run matched 340 of 377 CBP rulings (95% confidence interval 86.8%–92.8%). The first-country baseline was 63.4% (58.4%–68.1%). Auto-accepted cases matched 288 of 300 (93.1%–97.7%). Overall accuracy was 90.2%, 89.9%, and 90.2% across the original run and two gateway repeats.
          </p>
          <p>
            The engine could cite only earlier rulings. Pretraining exposure is unknown. The research prototype scored this window before the repository froze it. The initial confidence threshold used both splits. A development-only recalibration selected the same cases. The production engine uses no fine-tuning.
          </p>
          <p>
            In each of three gateway runs, given CBP’s starting origin, the plan check got 9/9 shown moves right (70.1%–100%), found 9/12 real changes (46.8%–91.1%), and showed 0/6 unchanged controls (0%–39.0%). Without the starting origin, it found 5/12 real changes each time. The strict 17/59 figure uses earlier pairs and counts only changes shown in every run, with and without starting origin.
          </p>
          <p><Link href="/accuracy#plan-checks">See the plan check results →</Link></p>
        </div>
      </details>
    </section>
  );
}

const benchmarks = [
  {
    label: "Wait time",
    before: "21 days",
    beforeNote: "Median to a binding CBP ruling",
    after: "4.6 s",
    afterNote: "Median to an origin prediction",
  },
  {
    label: "Prep time",
    before: "20 hours",
    beforeNote: "CBP paperwork estimate",
    after: "33 s",
    afterNote: "Recorded heat-gun plan search",
  },
  {
    label: "Cost",
    before: "~$3,600",
    beforeNote: "Median paid for origin advice",
    after: "$0.0078",
    afterNote: "Model cost per prediction at promo rates",
  },
];

const unitEconomics = [
  { operator: "", amount: "$834k", label: "China duty (41.7%)" },
  { operator: "−", amount: "$334k", label: "Thailand duty (16.7%)" },
  { operator: "−", amount: "$215k", label: "Added manufacturing cost" },
];

export function JudgeImpact() {
  return (
    <section id="impact" className="judge-section" aria-labelledby="storyh">
      <header className="judge-heading">
        <p className="judge-figure">Fig. 4</p>
        <h2 id="storyh">Time, cost and potential savings</h2>
      </header>

      <div className="judge-impact-grid">
        <article className="judge-card judge-impact">
          <h3>Measured benchmarks</h3>
          <p>Traditional rulings and advice alongside Unde’s preliminary checks.</p>
          <table className="judge-benchmarks">
            <thead>
              <tr>
                <th scope="col"><span className="sr-only">Benchmark</span></th>
                <th scope="col">Traditional</th>
                <th scope="col">Unde Labs</th>
              </tr>
            </thead>
            <tbody>
              {benchmarks.map((row) => (
                <tr key={row.label}>
                  <th scope="row">{row.label}</th>
                  <td><b>{row.before}</b><span>{row.beforeNote}</span></td>
                  <td><b className="judge-measured">{row.after}</b><span>{row.afterNote}</span></td>
                </tr>
              ))}
            </tbody>
          </table>
          <details className="judge-details">
            <summary>Benchmark sources</summary>
            <div className="judge-details-body read">
              <p>
                Ruling wait comes from a team count of 247 Asian-origin rulings. CBP estimates paperwork time for all ruling subjects. The advice median comes from 23 subsidised Korean exporter purchases. The 4.6 s prediction timing and model cost use the 377-ruling Vertex AI evaluation. The 33 s comes from an earlier recorded plan search, before the demo’s current configuration.
              </p>
            </div>
          </details>
        </article>

        <article className="judge-card judge-impact judge-simulation">
          <p className="judge-badge judge-review">Illustrative unit-economics case study</p>
          <h3>$2M annual heat-gun line</h3>
          <dl className="judge-equation" aria-label="Illustrative annual savings calculation">
            {unitEconomics.map(({ operator, amount, label }) => (
              <div key={label}>
                <dt>{label}</dt>
                <dd>{operator && <span className="judge-operator">{operator} </span>}{amount}</dd>
              </div>
            ))}
          </dl>
          <div className="judge-total">
            <p className="judge-number">$285k<span>/yr</span></p>
            <p>Illustrative net savings</p>
          </div>
          <p className="judge-impact-note">Simulated for a composite manufacturer. A broker confirms classification and duty rates.</p>
        </article>
      </div>
    </section>
  );
}
