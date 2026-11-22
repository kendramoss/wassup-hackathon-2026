import Link from "next/link";
import type { CaseResponse } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { japanAgreements } from "@/lib/japan-preference";
import { Main } from "./chrome";
import { Country, Flag, Fold } from "./ui";

const criteria: Record<string, string> = {
  WO: "Wholly obtained",
  PE: "Made entirely from originating materials",
  CC: "Change of tariff chapter",
  CTH: "Change of tariff heading",
  CTSH: "Change of tariff subheading",
  RVC: "Regional value content",
  SP: "Product-specific requirement",
};

function SourceList({ title, sources }: { title: string; sources: { citation: string; url: string }[] }) {
  const distinct = [...new Map(sources.map((source) => [source.url, source])).values()];
  return (
    <div className="mt-3">
      <h2 className="text-[13px] font-medium">{title}</h2>
      <ul className="mt-1 flex flex-col gap-1 text-[13px]">
        {distinct.map(({ citation, url }) => (
          <li key={url}>
            {/^https:\/\//i.test(url) ? (
              <a href={url} target="_blank" rel="noreferrer" className="text-ok underline underline-offset-2">{citation}</a>
            ) : citation}
          </li>
        ))}
      </ul>
    </div>
  );
}

export function JapanPreference({ c }: { c: CaseResponse }) {
  const C = copy.japan;
  const prediction = c.prediction;
  const result = prediction?.japan_preference;
  const origin = result?.outcome === "qualifies" ? prediction?.origin_country : undefined;
  const heading = result?.outcome === "qualifies" ? C.qualifies : result?.outcome === "does_not_qualify" ? C.doesNotQualify : C.undetermined;
  const agreement = result?.agreement ? japanAgreements[result.agreement] ?? result.agreement : undefined;
  return (
    <>
      <section aria-label={C.scope} className="border-b border-dashed border-guide">
        <div className="column mx-auto max-w-[1240px] px-4 pb-8 pt-10 sm:px-10">
          <p className="eyebrow">{C.scope}</p>
          <h1 className="mt-2 flex items-center gap-3 text-[32px] font-normal leading-none tracking-[-0.05em] sm:text-[52px]">
            <Flag country="Japan" size={36} />
            {heading}
          </h1>
          {agreement && <p className="mt-3 text-sm text-dim">{agreement}</p>}
          {origin && <p className="mt-2 text-sm">{C.origin} <Country name={origin} /></p>}
          {c.llm_provider === "fake" && <p className="mt-3 text-sm text-dim">{C.simulated}</p>}
          <p className="duo mt-4 max-w-[70ch] text-sm text-body">{C.review}</p>
          {result?.review_reason && <p className="duo mt-1 max-w-[70ch] text-sm text-dim [overflow-wrap:anywhere]">{result.review_reason}</p>}
          {!result && <p className="duo mt-1 max-w-[70ch] text-sm text-dim">{C.noResult}</p>}
        </div>
      </section>
      <Main className="flex flex-col items-start gap-4">
        <p className="duo text-[13px] text-dim">{C.approvalsOnly}</p>
        {c.id && (
          <Link href={`/cases/new?from=${encodeURIComponent(c.id)}`} className="inline-flex min-h-10 items-center text-sm text-ok underline underline-offset-2">
            {C.edit}
          </Link>
        )}
        {result && (
          <div className="w-full max-w-[75ch] [overflow-wrap:anywhere]">
            <Fold title={C.details}>
              {result.hs && <p className="text-[13px]">{C.tariffCode} {result.hs}</p>}
              {result.criterion && <p className="mt-1 text-[13px]">{criteria[result.criterion] ?? result.criterion}</p>}
              {result.deciding_rule && <p className="duo mt-2 whitespace-pre-line text-[13px] text-body">{result.deciding_rule}</p>}
              {prediction.reasoning && <p className="duo mt-2 whitespace-pre-line text-[13px] text-body">{prediction.reasoning}</p>}
              {result.sources?.length ? <SourceList title={C.rule} sources={result.sources} /> : null}
              {result.precedent_sources?.length ? <SourceList title={C.precedents} sources={result.precedent_sources} /> : null}
            </Fold>
          </div>
        )}
      </Main>
    </>
  );
}
