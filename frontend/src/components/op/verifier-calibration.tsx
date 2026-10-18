import type { EvalSummary } from "@/lib/api/client";
import { Chip, ScrollX } from "./ui";

type Calibration = NonNullable<EvalSummary["verifier_calibration"]>;
type Split = Calibration["splits"][number];
type Rate = { k?: number; n?: number; value?: number; ci_low?: number; ci_high?: number };

const pct = (v?: number) => (v === undefined ? "—" : `${Math.round(v * 100)}%`);
const ci = (r?: Rate) => (r?.ci_low !== undefined && r?.ci_high !== undefined ? `${Math.round(r.ci_low * 100)}–${Math.round(r.ci_high * 100)}` : "");
const kn = (r?: Rate) => (r?.n ? `${r.k ?? 0} of ${r.n}` : "none");

// How often the precedent check was right at each confidence (D86), from
// the scored flip pairs. Pooled counts add up every run of the same pairs;
// the pair-level line counts each pair once, which is where recall is honest.
export function VerifierCalibration({ c }: { c: Calibration }) {
  const shown = c.splits.reduce((s, x) => s + (x.shown.real ?? 0), 0);
  const wrong = c.splits.reduce((s, x) => s + (x.shown.wrong ?? 0), 0);
  const pairs = c.splits.reduce((s, x) => s + (x.pair_level.real_change_pairs ?? 0), 0);
  const every = c.splits.reduce((s, x) => s + (x.pair_level.shown_every_run ?? 0), 0);
  const bar = c.bar.toFixed(2);
  return (
    <section className="mt-6 border border-edge bg-sheet px-5 py-4 shadow-[4px_4px_0_var(--shadow)]" aria-labelledby="check-h">
      <div className="flex flex-wrap items-center gap-3">
        <h2 id="check-h" className="eyebrow">
          Precedent check
        </h2>
        <span className="ml-auto text-[11px] text-dim">{c.prompt} · moves that change origin, scored against CBP</span>
      </div>
      <p className="mt-3 text-[22px] leading-snug tracking-[-0.04em]">
        {shown} checks reached the {bar} bar across all runs; {wrong === 0 ? "none was wrong" : `${wrong} ${wrong === 1 ? "was" : "were"} wrong`}.
      </p>
      <p className="duo mt-2 max-w-[80ch] text-[13px] text-body">
        It&apos;s cautious, not thorough: a real change cleared the bar in every run for {every} of {pairs} pairs (about {pct(pairs ? every / pairs : undefined)}). The rest
        show as below the bar, for a broker to look at, so a missing plan doesn&apos;t mean no move works.
      </p>
      <div className="mt-4 flex flex-col gap-6">
        {c.splits.map((s) => (
          <SplitTable key={s.split} s={s} bar={c.bar} />
        ))}
      </div>
      <p className="duo mt-4 text-xs text-dim">{c.note} Counts in the bands add up every run of the same pairs, so their n is larger than the number of pairs.</p>
    </section>
  );
}

function SplitTable({ s, bar }: { s: Split; bar: number }) {
  const pl = s.pair_level;
  return (
    <div>
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="text-[13px] font-medium text-ink">{s.label}</h3>
        <Chip tone={s.development_only ? undefined : "ok"}>{s.development_only ? "development set" : "2026 pairs"}</Chip>
        <span className="text-[11px] text-dim">
          {s.pairs} pairs · {s.runs.length} runs · {s.rows} checks{s.errors ? ` · ${s.errors} without a verdict, left out` : ""}
        </span>
      </div>
      {s.caveat && <p className="duo mt-2 border-l-2 border-ask-line pl-3 text-xs text-body">{s.caveat}</p>}
      <ScrollX label={`${s.label}: right by confidence`}>
        <table className="mt-2 w-full min-w-[520px] border-collapse text-[12.5px]">
          <caption className="sr-only">Precedent-supported checks by confidence, and how many were real origin changes</caption>
          <thead>
            <tr>
              {["Check confidence", "Real changes", "Right", "95% interval"].map((h) => (
                <th key={h} scope="col" className="border-b border-line py-2 pr-3 text-left text-[11px] font-medium uppercase tracking-[0.08em] text-dim">
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {s.bands.map((b) => (
              <tr key={b.label} className="border-b border-dashed border-line">
                <td className="py-2 pr-3 tabular-nums">
                  {b.label}
                  {(b.min ?? 0) >= bar ? <span className="ml-2 text-[11px] text-ok">shown</span> : null}
                </td>
                <td className="py-2 pr-3 tabular-nums">{kn(b.correct)}</td>
                <td className="py-2 pr-3 tabular-nums">{pct(b.correct?.value)}</td>
                <td className="py-2 pr-3 tabular-nums text-dim">{ci(b.correct)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </ScrollX>
      <p className="duo mt-2 text-xs text-dim">
        At the bar: {s.shown.real} right, {s.shown.wrong} wrong ({pct(s.shown.precision?.value)}, {ci(s.shown.precision)}). Each pair once: {pl.shown_every_run} of{" "}
        {pl.real_change_pairs} real changes shown in every run, {pl.shown_some_runs} in some; {pl.wrong_any_run} no-change {pl.wrong_any_run === 1 ? "pair" : "pairs"} ever shown.
      </p>
    </div>
  );
}
