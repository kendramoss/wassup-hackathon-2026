"use client";

import { useCallback, useEffect, useState } from "react";
import { api, type CaseResponse, type EvidenceCheck, type EvidenceReport, type OriginMarket } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { formatDate } from "@/lib/format";
import { hasFollowUps } from "@/lib/market";
import { caseReadOnly } from "@/lib/roles";
import { CaseShell, type CaseCtx } from "../case-shell";
import { ErrorBox, Main, StepEyebrow } from "../chrome";
import { OriginRulesCheck } from "../origin-rules-check";
import { Button, Chip, Country, Fold, Spinner, cx } from "../ui";

// All checks on one product (#771, D803-D805): every check the server runs,
// each on its own card with the question it answers, its outcome, what is
// missing and the document that settles it, and its sources and limits under
// a fold (rule 16). Never one verdict: there is no summary line or score.
// The EU and UK cards ask their missing information in place (#770).

export function ChecksView() {
  return <CaseShell tab="checks">{(ctx) => <Checks {...ctx} />}</CaseShell>;
}

const R = copy.report;

export function statusTone(s: string): "ask" | undefined {
  return s === "needs_information" ? "ask" : undefined;
}

function Checks({ id, c, reload: reloadCase }: CaseCtx) {
  const [rep, setRep] = useState<EvidenceReport | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [n, setN] = useState(0);
  const load = useCallback(() => setN((x) => x + 1), []);
  useEffect(() => {
    let live = true;
    const ac = new AbortController();
    api
      .evidenceReport(id, ac.signal)
      .then((r) => live && (setRep(r), setError(null)))
      .catch((e) => live && setError(e));
    return () => {
      live = false;
      ac.abort();
    };
  }, [id, n]);
  const download = () => {
    if (!rep) return;
    const a = document.createElement("a");
    a.href = URL.createObjectURL(new Blob([rep.markdown], { type: "text/markdown" }));
    a.download = `evidence-report-${id}.md`;
    a.click();
    // Some browsers start the download after click returns.
    setTimeout(() => URL.revokeObjectURL(a.href), 1000);
  };
  // Every fold prints open, from the Print button or the browser's own.
  useEffect(() => {
    const open = () => document.querySelectorAll<HTMLDetailsElement>("#main details").forEach((d) => (d.open = true));
    window.addEventListener("beforeprint", open);
    return () => window.removeEventListener("beforeprint", open);
  }, []);
  const print = () => {
    document.querySelectorAll<HTMLDetailsElement>("#main details").forEach((d) => (d.open = true));
    window.print();
  };
  return (
    <>
      <section aria-label={R.title} className="border-b border-dashed border-guide">
        <div className="column mx-auto max-w-[1240px] px-4 pb-6 pt-10 sm:px-10">
          <StepEyebrow tab="checks" followUps={hasFollowUps(c)} />
          <h1 className="mt-1 text-[34px] font-normal leading-tight tracking-[-0.04em] sm:text-[44px]">{R.title}</h1>
          <p className="read mt-3 max-w-[60ch] text-[15px]">{R.lede}</p>
          <p className="duo mt-1 text-[13px] text-dim">
            {R.notRuling}
            {rep ? ` ${formatDate(rep.generated_at)}, ${new Date(rep.generated_at).toISOString().slice(11, 16)} UTC.` : ""}
          </p>
          {rep ? (
            <div className="no-print mt-4 flex flex-wrap gap-2.5">
              <Button variant="ghost" onClick={download}>
                {R.download}
              </Button>
              <Button variant="ghost" onClick={print}>
                {R.print}
              </Button>
            </div>
          ) : null}
        </div>
      </section>
      <Main className="flex flex-col gap-5">
        {error ? <ErrorBox error={error} onRetry={load} /> : null}
        {!rep && !error ? <Spinner label={R.loading} /> : null}
        {rep ? (
          <>
            <div data-checks className="grid gap-5 lg:grid-cols-2">
              {rep.checks.map((ch) => (
                <CheckCard key={ch.id} ch={ch} c={c} onSaved={() => (load(), reloadCase())} />
              ))}
            </div>
            <CaseFacts rep={rep} />
          </>
        ) : null}
      </Main>
    </>
  );
}

export function CheckCard({ ch, c, onSaved }: { ch: EvidenceCheck; c: CaseResponse; onSaved?: () => void }) {
  const market: OriginMarket | null = ch.id === "eu_origin" ? "eu" : ch.id === "uk_origin" ? "uk" : null;
  const canAnswer = market && ch.status === "needs_information" && !c.pinned && !caseReadOnly(c);
  return (
    <section
      aria-labelledby={`ck-${ch.id}`}
      data-check={ch.id}
      data-status={ch.status}
      className={cx("min-w-0 break-inside-avoid border bg-sheet px-4 py-3", ch.status === "not_run" || ch.status === "not_applicable" ? "border-dashed border-line" : "border-line")}
    >
      <h2 id={`ck-${ch.id}`} className="text-[15px] font-medium tracking-[-0.02em]">
        {ch.title}
      </h2>
      <p className="duo mt-0.5 text-[13px] text-dim">{ch.question}</p>
      <p className="mt-2 flex flex-wrap items-center gap-2">
        <Chip tone={statusTone(ch.status)}>{R.status[ch.status] ?? ch.status}</Chip>
        {ch.review ? <Chip>{R.review}</Chip> : null}
        {ch.uses_supplied ? <Chip>{R.supplied}</Chip> : null}
      </p>
      <p className="read mt-2 text-[15px] outline-none" data-outcome tabIndex={-1} aria-live="polite">
        {ch.origin ? <OriginOutcome outcome={ch.outcome} origin={ch.origin} /> : ch.outcome}
      </p>
      {ch.reason ? <p className="duo mt-1 text-[13px] text-body">{ch.reason}</p> : null}
      <p className="duo mt-1 text-[12.5px] text-dim">{ch.label}</p>
      {ch.missing?.length ? (
        <div className="duo mt-2 text-[13px]">
          <p className="text-body">{R.missing}</p>
          <ul className="mt-0.5 list-disc pl-4">
            {ch.missing.map((m, i) => (
              <li key={i}>
                {m.label}
                {m.evidence ? <span className="block text-[12.5px] text-dim">{m.evidence}</span> : null}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      {ch.not_modelled?.length ? (
        <div className="duo mt-2 text-[13px]">
          <p className="text-body">{R.notModelled}</p>
          <ul className="mt-0.5 list-disc pl-4">
            {ch.not_modelled.map((m, i) => (
              <li key={i} className="break-words">
                {m}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      <div className="mt-2">
        {canAnswer ? (
          <Fold title={R.answer}>
            <OriginRulesCheck c={c} market={market} onSaved={onSaved} bare />
          </Fold>
        ) : null}
        <Fold title={R.details} meta={ch.rule_version || undefined}>
          <div className="duo flex flex-col gap-1.5 text-[12.5px] text-dim">
            {ch.sources?.map((s, i) => (
              <p key={i} className="break-words">
                {s.url ? (
                  <a href={s.url} className="text-ok underline underline-offset-2" rel="noreferrer" target="_blank">
                    {s.title}
                  </a>
                ) : (
                  s.title
                )}
                {s.version ? ` · ${s.version}` : ""}
              </p>
            ))}
            {ch.rule_version ? (
              <p>
                {R.version}: {ch.rule_version}
              </p>
            ) : null}
            {ch.limitations?.length ? (
              <ul className="list-disc pl-4">
                {ch.limitations.map((l, i) => (
                  <li key={i}>{l}</li>
                ))}
              </ul>
            ) : null}
            <p>
              {R.made}: {formatDate(ch.at)}
            </p>
          </div>
        </Fold>
      </div>
    </section>
  );
}

// OriginOutcome shows "Origin: China (basis)" with the flag: the words
// before the colon, the country, then the basis in grey.
export function OriginOutcome({ outcome, origin }: { outcome: string; origin: string }) {
  const m = outcome.match(/^([^:]+):\s*([^(]+?)\s*(\(.*\))?$/);
  if (!m) return <>{outcome}</>;
  return (
    <>
      {m[1]}: <Country name={origin} />
      {m[3] ? <span className="duo text-[13px] text-dim"> {m[3]}</span> : null}
    </>
  );
}

// Supplied lists the answers given to a check, apart from the case facts,
// with who gave them and when; a set saved before the parts list changed is
// marked not used.
export function ReportSupplied({ rep }: { rep: EvidenceReport }) {
  const C = copy.originRules;
  const sets = Object.entries(rep.supplied ?? {}).filter(([, s]) => s);
  if (!sets.length) return null;
  return (
    <div data-report-supplied className="mt-2 flex flex-col gap-2">
      <p className="text-body">{C.yours}</p>
      {sets.map(([m, s]) => (
        <div key={m}>
          <p className="text-dim">
            {m.toUpperCase()} · {C.yoursMeta(0, formatDate(s!.updated_at)).replace(/^0 · /, "")}
          </p>
          {s!.stale ? <p className="text-ask">{C.staleAll}</p> : null}
          <ul className="list-disc pl-4">
            {s!.good_hts ? <li>HS {s!.good_hts}</li> : null}
            {s!.row ? <li>{s!.row === "none" ? C.none : s!.row}</li> : null}
            {Object.entries(s!.answers ?? {}).map(([k, b]) => (
              <li key={k}>
                {k}: {b ? C.yes : C.no}
              </li>
            ))}
            {(s!.materials ?? []).map((x) => (
              <li key={x.index}>
                {x.name}:{" "}
                {[x.hts, x.origin, x.value_share !== undefined ? `${Math.round(x.value_share * 10000) / 100}% value` : "", x.weight_share !== undefined ? `${Math.round(x.weight_share * 10000) / 100}% weight` : ""].filter(Boolean).join(", ")}
              </li>
            ))}
          </ul>
        </div>
      ))}
    </div>
  );
}

function CaseFacts({ rep }: { rep: EvidenceReport }) {
  const p = rep.case.product;
  return (
    <section aria-label={R.caseFacts}>
      <Fold title={R.caseFacts} meta={formatDate(rep.case.updated_at)}>
        <div className="duo flex flex-col gap-2 text-[13px]">
          <p className="text-dim">{R.caseFactsNote}</p>
          {p ? (
            <>
              <p>
                {p.name}
                {p.hts ? <span className="text-dim"> · HS {p.hts}</span> : null}
              </p>
              {p.components?.length ? (
                <ul className="list-disc pl-4">
                  {p.components.map((x, i) => (
                    <li key={i}>
                      {x.name}
                      <span className="text-dim">
                        {" "}
                        · {x.hts || "no code"} · {x.made_in || "origin not given"}
                      </span>
                    </li>
                  ))}
                </ul>
              ) : null}
            </>
          ) : null}
          {rep.case.facts?.trim() ? <p className="whitespace-pre-line break-words text-body">{rep.case.facts.trim()}</p> : null}
          <ReportSupplied rep={rep} />
        </div>
      </Fold>
    </section>
  );
}
