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
          <p><b>26.8 percentage points</b> above the 63.4% first-country baseline.</p>
        </article>
        <article className="judge-card judge-kpi">
          <p className="judge-number">96.0%</p>
          <h3>Auto-accepted accuracy</h3>
          <p>79.6% auto-accepted</p>
          <p className="judge-badge judge-review">20.4% routed to human review</p>
        </article>
        <article className="judge-card judge-kpi">
          <p className="judge-number">9/9</p>
          <h3>Plan precision</h3>
          <p>0 false positives across 18 CBP before/after pairs, with the starting origin given.</p>
        </article>
      </div>

      <nav className="judge-badges" aria-label="Benchmark evidence">
        <Link className="judge-badge" href="/method#clean">377-ruling test set · Jan–Sep 2026 →</Link>
        <Link className="judge-badge" href="/method#model">Zero fine-tuning →</Link>
        <Link className="judge-badge" href="/accuracy#misses">Inspect all 37 failures →</Link>
        <Link className="judge-badge" href="/accuracy#plan-checks">See the plan test →</Link>
      </nav>
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
    beforeNote: "Median paid via Korean export vouchers",
    after: "$0.0078",
    afterNote: "Model cost per prediction at promo rates",
  },
];

const chinaDuty = 834;
const thailandDuty = 334;
const manufacturingCost = 215;
const dutySavings = chinaDuty - thailandDuty;
const netSavings = dutySavings - manufacturingCost;

const waterfall = [
  { label: "China duty (41.7%)", value: chinaDuty, start: 0, end: chinaDuty, kind: "total" },
  { label: "Thailand duty (16.7%)", value: thailandDuty, start: chinaDuty, end: dutySavings, kind: "deduction" },
  { label: "Added manufacturing cost", value: manufacturingCost, start: dutySavings, end: netSavings, kind: "deduction" },
  { label: "Net annual savings", value: netSavings, start: 0, end: netSavings, kind: "total" },
];

function DutyWaterfall() {
  return (
    <ol className="judge-waterfall" aria-label="Illustrative annual savings calculation">
      {waterfall.map(({ label, value, start, end, kind }, index) => {
        const isNet = index === waterfall.length - 1;
        const prefix = kind === "deduction" ? "−" : "";
        const amount = `${prefix}$${value}k`;
        return (
          <li key={label} className={isNet ? "judge-waterfall-net" : undefined}>
            <div className="judge-waterfall-track" aria-hidden="true">
              <span
                className={`judge-waterfall-bar judge-waterfall-${kind}`}
                style={{ bottom: `${(Math.min(start, end) / chinaDuty) * 100}%`, height: `${(value / chinaDuty) * 100}%` }}
              />
              {!isNet && <span className="judge-waterfall-connector" style={{ bottom: `${(end / chinaDuty) * 100}%` }} />}
            </div>
            <b className="judge-waterfall-amount">{amount}{isNet && <small>/yr</small>}</b>
            <span className="judge-waterfall-label">{label}</span>
          </li>
        );
      })}
    </ol>
  );
}

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
          <Link className="judge-source" href="/method#check">Check the benchmark sources →</Link>
        </article>

        <article className="judge-card judge-impact judge-simulation">
          <p className="judge-badge judge-review">Illustrative unit-economics case study</p>
          <h3>$2M annual heat-gun line</h3>
          <DutyWaterfall />
          <p className="judge-impact-note">A broker confirms classification and duty rates.</p>
        </article>
      </div>
    </section>
  );
}
