import { challengePercent, type ChallengeFeature } from "@/lib/challenge-evidence";
import { copy } from "@/lib/copy";
import { Fold } from "./ui";

const capabilityLabels: Record<string, string> = {
  textile_live_v2: "Textile origin model", part102_specialist: "U.S. marking origin", japan_live_unmodified: "Japan preference model", legal_routing: "Choosing the right rules",
};
const metricLabels: Record<string, string> = {
  textile_uncertain_reviewed: "Missing facts reviewed", pharma_uncertain_reviewed: "Missing facts reviewed", part102_uncertain_reviewed: "Missing facts reviewed",
  pharma_route_eligible: "In current routing scope", part102_auto_accepted_wrong: "Wrong auto-accepts",
  japan_decided_agreed: "Decided cases correct", japan_model_uncertain_undetermined: "Model withheld on missing facts", japan_criterion_valid: "Valid qualifying criteria",
  routing_appropriate: "Specialist routing correct", routing_negative_controls: "General controls correct", routing_pharma: "Pharma routing correct",
};
function metricLabel(metric: ChallengeFeature["metrics"][number]): string {
  const bar = metric.label.match(/\((study|deployed) bar ([0-9.]+)/);
  if (bar) return `Decided correct, confidence ≥ ${bar[2]}${bar[1] === "study" ? " (study)" : ""}`;
  return metricLabels[metric.id] ?? metric.label;
}

export function ChallengeEvidence({ features }: { features: readonly ChallengeFeature[] }) {
  if (features.length === 0) return null;
  const link = "text-ok underline underline-offset-2 max-sm:inline-flex max-sm:min-h-11 max-sm:items-center";
  return (
    <section id="challenge-tests" aria-labelledby="challenge-tests-heading" className="mt-8 border-t border-line pt-5">
      <h2 id="challenge-tests-heading" className="text-sm font-semibold">Generated challenge tests</h2>
      <p className="read mt-2 text-sm text-dim">
        Generated rule, case and document variations, including expected refusals and requests for missing facts. Separate from real-case studies; policy changes are illustrative. Errors stay in the counts.
      </p>
      <table className="mt-3 w-full border-collapse text-sm">
        <thead>
          <tr className="border-b border-line">
            <th scope="col" className="py-2 pr-3 text-left font-medium text-dim">Capability</th>
            <th scope="col" className="py-2 text-right font-medium text-dim">Checks passed</th>
          </tr>
        </thead>
        <tbody>
          {features.map((f) => (
            <tr key={f.id} className="border-b border-dashed border-line align-top">
              <th scope="row" className="py-3 pr-3 text-left font-normal">
                {capabilityLabels[f.id] ?? f.label}
                {f.scope_note ? <span className="mt-1 block text-[12px] leading-snug text-dim">{f.scope_note}</span> : null}
                {f.public_note ? <span className="mt-1 block text-[12px] leading-snug text-dim">{f.public_note}</span> : null}
                {f.metrics.length > 0 ? <dl className="mt-2 space-y-1 text-[12px] leading-snug">
                  {f.metrics.map((metric) => <div key={metric.id} className="flex flex-wrap gap-x-2">
                    <dt>{metricLabel(metric)}</dt><dd className="font-semibold tabular-nums">{metric.num}/{metric.den}</dd>
                  </div>)}
                </dl> : null}
              </th>
              <td className="py-3 text-right tabular-nums">
                <strong>{challengePercent(f)}</strong>
                <span className="block text-[12px] text-dim">{f.test.matched}/{f.test.cases}</span>
                {f.test.errors > 0 ? <span className="mt-1 block text-[12px] text-dim">{f.test.errors} {f.test.errors === 1 ? "error" : "errors"} included</span> : null}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <div className="mt-3">
        <Fold title="Sources and test limits">
          <div className="read flex max-w-[90ch] flex-col gap-3 text-[12.5px] leading-snug text-dim">
            <p>
              Related variations can share the same source rules or examples. Case counts are not counts of independent legal decisions. Each record identifies its development and test data, expected answers, failures and limits.
            </p>
            <ul className="list-disc space-y-2 pl-5">
              {features.map((f) => (
                <li key={f.id}>
                  <a href={`${copy.landing.repo}/blob/main/${f.sourcePath}`} className={link}>{f.label}: {f.source_families} source groups · results and limits</a>
                  <ul className="mt-1 list-disc space-y-1 pl-5">
                    {f.limitations.map((limit, index) => <li key={index}>{limit}</li>)}
                  </ul>
                </li>
              ))}
            </ul>
          </div>
        </Fold>
      </div>
    </section>
  );
}
