"use client";

// Accuracy page trust sections (review follow-up): how to reproduce every
// number, and every held-out miss with its CBP ruling. The misses come from
// failure-cases.json, built at commit time by eval/charts/build_trust_data.py
// from the official held-out run, so the page needs no new endpoint and the
// file is checked for staleness by `--check`. Measured findings get bars with
// their numbers; our reading of the misses is plain text.
import { useState } from "react";
import data from "@/lib/failure-cases.json";
import { copy } from "@/lib/copy";
import { Chip, RulingLink, ScrollX, cx } from "./ui";

const T = copy.trust;
const repo = copy.landing.repo;
const pct = (v: number, d = 1) => `${(v * 100).toFixed(d)}%`;

const commands = `git clone ${repo}.git && cd wassup-hackathon-2026
make reproduce
# or one at a time:
make verify-data                                         # data files match eval/SHA256SUMS
cd backend && go run ./cmd/claimscheck && cd ..         # headline counts, from the result rows
python3 -I eval/charts/build_evaluation.py . --check     # calibration, curves, split funnel
python3 -I eval/charts/build_trust_data.py . --check     # failure cases, corpus counts`;

export function Reproduce() {
  return (
    <section id="reproduce" aria-labelledby="repro-h" className="mt-6 border border-edge bg-sheet px-5 py-4 shadow-[4px_4px_0_var(--shadow)]">
      <div className="flex flex-wrap items-baseline gap-3">
        <h2 id="repro-h" className="eyebrow">
          {T.reproTitle}
        </h2>
        <a href={`${repo}/blob/main/docs/EVALUATION.md`} target="_blank" rel="noreferrer" className="ml-auto text-[12px] text-ok underline underline-offset-2">
          How the test set is built →
        </a>
      </div>
      <p className="read mt-2 max-w-[80ch] text-[13.5px] text-body">{T.reproLead}</p>
      <ScrollX label={T.reproTitle}>
        <pre className="mt-3 border border-line bg-white px-4 py-3 font-mono text-[12px] leading-relaxed">
          <code>{commands}</code>
        </pre>
      </ScrollX>
      <p className="read mt-2 text-[12.5px] text-dim">
        {T.reproNote} Also in the repository: the{" "}
        <a href={`${repo}/blob/main/docs/MODEL-CARD.md`} target="_blank" rel="noreferrer" className="text-ok underline underline-offset-2">
          model card
        </a>{" "}
        and the{" "}
        <a href={`${repo}/blob/main/docs/DATASHEET.md`} target="_blank" rel="noreferrer" className="text-ok underline underline-offset-2">
          datasheet for the ruling corpus
        </a>
        .
      </p>
    </section>
  );
}

const subject = (s: string) => s.replace(/^RE:\s*/i, "");

export function KnownFailures() {
  const [all, setAll] = useState(false);
  const autoMisses = data.misses.filter((m) => m.auto_accepted);
  const rows = all ? data.misses : autoMisses;
  return (
    <section id="misses" aria-labelledby="miss-h" className="mt-6 border border-edge bg-sheet px-5 py-4 shadow-[4px_4px_0_var(--shadow)]">
      <div className="flex flex-wrap items-center gap-3">
        <h2 id="miss-h" className="eyebrow">
          {T.failTitle}
        </h2>
        <Chip tone="ok">official · Vertex AI</Chip>
        <span className="ml-auto text-[11px] text-dim">
          {data.misses.length} of {data.n} wrong · {autoMisses.length} auto-accepted
        </span>
      </div>
      <p className="read mt-2 max-w-[80ch] text-[14px] text-body">{T.failLead(data.misses.length, autoMisses.length, data.n)}</p>

      <div className="mt-5 grid gap-6 lg:grid-cols-2">
        <div>
          <div className="flex flex-wrap items-baseline gap-2">
            <h3 className="eyebrow">{T.byOriginTitle}</h3>
            <span className="text-[11px] text-dim">{T.byOriginNote}</span>
          </div>
          <div className="mt-3 flex flex-col gap-2">
            {data.by_origin.map((o) => {
              const v = o.correct / o.n;
              return (
                <div key={o.origin} className="grid grid-cols-[minmax(0,130px)_minmax(0,1fr)] items-center gap-x-3 gap-y-0.5 text-[12.5px] sm:grid-cols-[minmax(0,140px)_minmax(0,1fr)_auto]">
                  <span className="truncate">{o.origin}</span>
                  <span className="relative h-3 bg-wash" aria-hidden="true">
                    <i className={cx("absolute inset-y-0 left-0", o.origin === "China" ? "bg-ink" : "bg-ok")} style={{ width: `${v * 100}%` }} />
                    <i className="absolute -inset-y-1 w-px bg-ink" style={{ left: `${o.ci_low * 100}%` }} />
                    <i className="absolute -inset-y-1 w-px bg-ink" style={{ left: `${o.ci_high * 100}%` }} />
                  </span>
                  <span className="col-span-2 whitespace-nowrap tabular-nums sm:col-span-1 sm:text-right">
                    <b className="font-medium">{pct(v)}</b>{" "}
                    <span className="text-dim">
                      {o.correct}/{o.n} ({(o.ci_low * 100).toFixed(1)}–{(o.ci_high * 100).toFixed(1)})
                    </span>
                  </span>
                </div>
              );
            })}
          </div>
        </div>
        <div>
          <h3 className="eyebrow">{T.chinaTitle}</h3>
          <ul className="read mt-3 flex list-disc flex-col gap-1.5 pl-5 text-[13.5px]">
            {T.china(data.china).map((t) => (
              <li key={t}>{t}</li>
            ))}
          </ul>
          <h3 className="eyebrow mt-5">{T.patternsTitle}</h3>
          <p className="mt-1 text-[11.5px] text-dim">{T.patternsNote}</p>
          <ul className="read mt-2 flex list-disc flex-col gap-1.5 pl-5 text-[13.5px]">
            {T.patterns.map((t) => (
              <li key={t}>{t}</li>
            ))}
          </ul>
        </div>
      </div>

      <ScrollX label={T.failTitle}>
        <table className="mt-5 w-full min-w-[760px] border-collapse text-[12.5px]">
          <thead>
            <tr>
              {T.columns.map((h) => (
                <th key={h} scope="col" className="border-b border-line py-2 pr-3 text-left text-[11px] font-medium uppercase tracking-[0.08em] text-dim">
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((m) => (
              <tr key={m.ruling} className="border-b border-dashed border-line align-top">
                <td className="py-2 pr-3">
                  <RulingLink id={m.ruling} />
                </td>
                <td className="whitespace-nowrap py-2 pr-3 tabular-nums text-dim">{m.date}</td>
                <td className="duo py-2 pr-3">{subject(m.subject)}</td>
                <td className="whitespace-nowrap py-2 pr-3">{m.cbp_held}</td>
                <td className="whitespace-nowrap py-2 pr-3">{m.predicted}</td>
                <td className="py-2 pr-3 tabular-nums">{m.confidence.toFixed(2)}</td>
                <td className="py-2">{m.auto_accepted ? <Chip tone="fail">{T.auto}</Chip> : <Chip>{T.review}</Chip>}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </ScrollX>
      <button type="button" onClick={() => setAll(!all)} className="mt-3 min-h-8 text-[12.5px] text-ok hover:underline" aria-expanded={all}>
        {all ? T.showAuto : T.showAll(data.misses.length)}
      </button>
      <p className="duo mt-2 text-[11.5px] text-dim">{T.source}</p>
    </section>
  );
}
