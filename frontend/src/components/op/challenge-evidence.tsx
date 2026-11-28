import { challengePercent, type ChallengeFeature } from "@/lib/challenge-evidence";
import { copy } from "@/lib/copy";
import { Fold } from "./ui";

export function ChallengeEvidence({ features }: { features: readonly ChallengeFeature[] }) {
  if (features.length === 0) return null;
  const link = "text-ok underline underline-offset-2 max-sm:inline-flex max-sm:min-h-11 max-sm:items-center";
  return (
    <section id="challenge-tests" aria-labelledby="challenge-tests-heading" className="mt-8 border-t border-line pt-5">
      <h2 id="challenge-tests-heading" className="text-sm font-semibold">Generated challenge tests</h2>
      <p className="read mt-2 text-sm text-dim">
        Generated variations of published rules and examples, plus illustrative policy changes, including expected refusals and requests for missing facts. Separate from real-case studies; errors stay in the counts.
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
                {f.label}
                {f.public_note ? <span className="mt-1 block text-[12px] leading-snug text-dim">{f.public_note}</span> : null}
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
