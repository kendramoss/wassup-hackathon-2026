import Link from "next/link";
import { Flag, RulingLink } from "../ui";
import { JudgeTabs } from "./judge-tabs";
import "./judge-workflow.css";

const parts = [
  { sku: "HG-1001", name: "Main housing", country: "Thailand" },
  { sku: "HG-2001", name: "Heating element", country: "Thailand" },
  { sku: "HG-3001", name: "Electric motor", country: "China" },
  { sku: "HG-4001", name: "Circuit board assembly", country: "China" },
] as const;

const workflow = [
  {
    label: "Product",
    subtitle: "Gemini 3.8 Flash · routing",
    content: (
      <div className="workflowSplit">
        <div className="workflowLead">
          <h3>Start with your parts list</h3>
          <p>Enter where each part is made. The router selects the applicable origin rules.</p>
          <a href="/samples/heatgun-bom.csv" download>Download the sample CSV →</a>
        </div>
        <figure className="workflowArtifact">
          <figcaption className="workflowFile">heatgun-bom.csv</figcaption>
          <table className="workflowFileTable" aria-label="Excerpt from the heat-gun parts list">
            <thead>
              <tr><th scope="col">Part no.</th><th scope="col">Component</th><th scope="col">Made in</th></tr>
            </thead>
            <tbody>
              {parts.map(({ sku, name, country }) => (
                <tr key={sku}>
                  <td>{sku}</td><th scope="row">{name}</th>
                  <td><span className="workflowCountry"><Flag country={country} />{country}</span></td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="workflowArtifactFoot">Final assembly in Thailand</p>
        </figure>
      </div>
    ),
  },
  {
    label: "Origin",
    subtitle: "Gemini 4 Argon · prediction",
    content: (
      <div className="workflowSplit">
        <div className="workflowLead">
          <h3>Trace the likely origin</h3>
          <p>Read the prediction alongside the rulings and see where more evidence is needed.</p>
          <Link href="/examples/revocation-alert">See a ruling-change alert →</Link>
        </div>
        <figure className="workflowArtifact">
          <figcaption className="workflowFile">Origin result · Kobalt heat gun</figcaption>
          <div className="workflowOrigin">
            <p className="workflowOriginValue"><Flag country="China" size={22} />Likely China</p>
            <span className="lp-review-tag">Needs more facts</span>
          </div>
          <dl className="workflowEvidence">
            <div><dt>CBP’s ruling on this setup</dt><dd>China <RulingLink id="N328954" className="lp-ruling-pill" /></dd></div>
            <div><dt>Earlier rulings cited</dt><dd><RulingLink id="N348083" className="lp-ruling-pill" /><RulingLink id="N351922" className="lp-ruling-pill" /></dd></div>
          </dl>
        </figure>
      </div>
    ),
  },
  {
    label: "Plan",
    subtitle: "Planner + verifier",
    content: (
      <div className="workflowSplit">
        <div className="workflowLead">
          <h3>Test a production move</h3>
          <p>The planner proposes moves. The verifier checks earlier rulings for support and conflicts.</p>
          <div className="workflowLinks">
            <Link href="/examples/plan-rule-check">See a tariff-shift check →</Link>
            <Link href="/examples/part102-plan">See a Part 102 plan →</Link>
          </div>
        </div>
        <figure className="workflowArtifact">
          <figcaption className="workflowFile">Recorded production move</figcaption>
          <div className="workflowRoute">
            <div className="workflowPlant"><span>Before</span><strong className="workflowCountry"><Flag country="China" size={14} />China</strong></div>
            <span aria-hidden="true">→</span>
            <div className="workflowPlant"><span>Proposed</span><strong className="workflowCountry"><Flag country="Thailand" size={14} />Thailand</strong></div>
          </div>
          <ul className="workflowMoveParts" aria-label="Production moved in the recorded plan">
            <li>Motor and fan assembly</li><li>Circuit board</li><li>Switch, cord and minor parts</li>
          </ul>
          <div className="workflowPlanSource">
            <span>Supporting ruling <RulingLink id="N345938" className="lp-ruling-pill" /></span>
            <span title="Recorded 18 Oct production run. The pinned case shows 0.96.">Recorded 18 Oct check: <strong>0.96</strong></span>
          </div>
          <p className="workflowArtifactFoot workflowCaution">Not a blind test: some moves were added after the change was public.</p>
        </figure>
      </div>
    ),
  },
  {
    label: "Draft",
    subtitle: "Ruling request template",
    content: (
      <div className="workflowSplit">
        <div className="workflowLead">
          <h3>Prepare a broker-ready draft</h3>
          <p>A template combines your case facts and supporting rulings. Your broker checks it before filing.</p>
        </div>
        <figure className="workflowArtifact workflowDraft">
          <figcaption className="workflowFile">Ruling request preview <span className="lp-review-tag">Broker review</span></figcaption>
          <div className="workflowDocument">
            <p>To U.S. Customs and Border Protection</p>
            <p><strong>Re: Country of origin of the Kobalt heat gun</strong></p>
            <p>We request a ruling on the country of origin after moving component production to Thailand.</p>
            <p className="workflowSignature">For review before filing</p>
          </div>
        </figure>
      </div>
    ),
  },
];

export function JudgeWorkflow() {
  return (
    <section className="lp-section judgeWorkflow" id="how-it-works" aria-labelledby="f2h">
      <div className="lp-section-head">
        <span className="lp-figure-label">Fig. 2</span>
        <h2 id="f2h">How it works</h2>
      </div>
      <JudgeTabs label="How it works" items={workflow} />
    </section>
  );
}
