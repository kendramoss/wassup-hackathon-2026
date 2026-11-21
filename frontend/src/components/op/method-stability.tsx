"use client";

// Repeat runs of the frozen engine (D190, D240, D241), drawn from
// GET /v1/eval/summary: each run a dot on one accuracy scale, with how many
// rulings changed answer. Nothing here is computed beyond the summary's own
// values; the official single runs stay the published figures.
import { useEffect, useState } from "react";
import { api, type EvalSummary } from "@/lib/api/client";
import { ErrorBox } from "./chrome";
import { Spinner, cx } from "./ui";
import { decisionHref } from "@/lib/method";

type Item = NonNullable<EvalSummary["items"]>[number];

const LO = 0.86;
const HI = 0.97;
const x = (v: number) => `${((v - LO) / (HI - LO)) * 100}%`;
const pct = (v?: number) => (v === undefined ? "—" : `${(v * 100).toFixed(1)}%`);

type Row = { title: string; note: string; runs: { label: string; it?: Item; official?: boolean }[]; flips?: Item; refs: string[] };

export function Stability() {
  const [data, setData] = useState<EvalSummary | null>(null);
  const [error, setError] = useState<unknown>(null);
  const load = () => {
    setError(null);
    api.evalSummary().then(setData, setError);
  };
  useEffect(() => {
    api.evalSummary().then(setData, setError);
  }, []);

  if (error) return <div className="mt-4"><ErrorBox error={error} onRetry={load} /></div>;
  if (!data) return <div className="mt-4"><Spinner label="Loading the repeat runs…" /></div>;

  const get = (k: string) => data.items?.find((i) => i.key === k);
  const rows: Row[] = [
    {
      title: "Development set",
      note: "three runs, gateway",
      runs: [1, 2, 3].map((i) => ({ label: `run ${i}`, it: get(`repeat_dev_accuracy_run${i}`) })),
      flips: get("repeat_dev_answer_flips"),
      refs: ["D190"],
    },
    {
      title: "Held-out 1",
      note: "official Vertex AI run, two gateway repeats",
      runs: [
        { label: "official", it: get("repeat_final_accuracy_official"), official: true },
        { label: "repeat 1", it: get("repeat_final_accuracy_repeat1") },
        { label: "repeat 2", it: get("repeat_final_accuracy_repeat2") },
      ],
      flips: get("repeat_final_answer_flips"),
      refs: ["D240", "D241"],
    },
    {
      title: "Held-out 2",
      note: "official gateway run, two gateway repeats",
      runs: [
        { label: "official", it: get("repeat_final2_accuracy_official"), official: true },
        { label: "repeat 1", it: get("repeat_final2_accuracy_repeat1") },
        { label: "repeat 2", it: get("repeat_final2_accuracy_repeat2") },
      ],
      flips: get("repeat_final2_answer_flips"),
      refs: ["D240", "D241"],
    },
  ].filter((r) => r.runs.every((u) => u.it?.value !== undefined));

  if (rows.length === 0) return <p className="mt-4 text-[13px] text-dim">The repeat runs are not in this build&apos;s summary.</p>;

  // The heat-gun plan check, run again three times (D321).
  const planRuns = [1, 2, 3].map((i) => ({ i, shown: get(`plan_stability_heatgun_run${i}_shown_moves`), best: get(`plan_stability_heatgun_run${i}_best_check`) }));
  const planShown = get("plan_stability_heatgun_runs_shown");
  const planSame = get("plan_stability_heatgun_same_moves");
  const earlier = get("plan_stability_heatgun_earlier_runs_shown");
  const hasPlan = planShown?.value !== undefined && planRuns.every((r) => r.shown?.value !== undefined && r.best?.value !== undefined);
  // After the cord, knobs and covers were split from the switch and wiring
  // (D330), two more runs (D331, D332): where CBP's own move landed.
  const splitShown = get("plan_split_heatgun_cbp_move_shown");
  const splitRuns = [1, 2].map((i) => ({
    i,
    check: get(`plan_split_heatgun_run${i}_cbp_move_check`),
    rank: get(`plan_split_heatgun_run${i}_cbp_move_rank`),
    four: get(`plan_split_heatgun_run${i}_four_group_check`),
    top: get(`plan_split_heatgun_run${i}_top_plan_changed_parts`),
  }));
  const hasSplit = splitShown?.value !== undefined && splitRuns.every((r) => r.check?.value !== undefined && r.rank?.value !== undefined && r.top?.value !== undefined);

  return (
    <figure
      aria-label={rows
        .map((r) => `${r.title}, ${r.runs[0].it?.n} rulings: ${r.runs.map((u) => `${u.label} ${pct(u.it?.value)}`).join(", ")}; answer changed on ${r.flips?.value ?? "—"}.`)
        .join(" ")}
      className="mt-5 max-w-[960px] border border-edge bg-sheet px-4 py-4 shadow-[4px_4px_0_var(--shadow)] sm:px-5"
    >
      <figcaption className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <span className="eyebrow">Same engine, run again</span>
        <span className="text-[11px] text-dim sm:ml-auto">accuracy per run · every call fresh, no cache · from GET /v1/eval/summary</span>
      </figcaption>
      <ul className="mt-4 flex flex-col">
        {rows.map((r) => (
          <li key={r.title} className="grid gap-x-4 gap-y-2 border-b border-dashed border-line py-3 text-[12.5px] md:grid-cols-[200px_minmax(0,1fr)_230px]">
            <div>
              <p className="font-medium text-ink">
                {r.title} <span className="tabular-nums font-normal text-dim">· {r.runs[0].it?.n} rulings</span>
              </p>
              <p className="text-[11.5px] text-dim">{r.note}</p>
            </div>
            <div className="relative h-6 self-center" aria-hidden="true">
              <i className="absolute inset-x-0 top-1/2 h-px bg-line" />
              {r.runs.map((u, i) => (
                <i
                  key={u.label}
                  className={cx("absolute size-2.5 -translate-x-1/2 rotate-45", u.official ? "bg-ok" : "border border-ink bg-white")}
                  style={{ left: x(u.it!.value!), top: `${4 + i * 6}px` }}
                />
              ))}
            </div>
            <div className="text-[12px] text-dim md:text-right">
              <ul className="tabular-nums">
                {r.runs.map((u) => (
                  <li key={u.label}>
                    <span className={cx(u.official && "text-ink")}>{u.label}</span> <b className="font-medium text-ink">{pct(u.it?.value)}</b>
                  </li>
                ))}
              </ul>
              <p className="tabular-nums">
                answer changed on <b className="font-medium text-ink">{r.flips?.value ?? "—"}</b> of {r.flips?.n ?? "—"} ·{" "}
                {r.refs.map((id, i) => (
                  <span key={id}>
                    {i > 0 ? ", " : ""}
                    <a href={decisionHref(id)} target="_blank" rel="noreferrer" className="font-mono text-ok underline underline-offset-2">
                      {id}
                    </a>
                  </span>
                ))}
              </p>
            </div>
          </li>
        ))}
      </ul>
      <div className="mt-2 grid gap-x-4 text-[11px] text-dimension md:grid-cols-[200px_minmax(0,1fr)_230px]">
        <span className="max-md:hidden" />
        <span className="relative h-4">
          {[0.88, 0.9, 0.92, 0.94, 0.96].map((t) => (
            <span key={t} className="absolute -translate-x-1/2 tabular-nums" style={{ left: x(t) }}>
              {Math.round(t * 100)}%
            </span>
          ))}
        </span>
      </div>
      {hasPlan && (
        <div className="mt-3 grid gap-x-4 gap-y-2 border-t border-dashed border-line py-3 text-[12.5px] md:grid-cols-[200px_minmax(0,1fr)_230px]">
          <div>
            <p className="font-medium text-ink">
              Plan check, heat gun <span className="tabular-nums font-normal text-dim">· {planShown?.n} runs</span>
            </p>
            <p className="text-[11.5px] text-dim">production settings, gateway, every call fresh</p>
          </div>
          <p className="self-center text-[12px] text-dim">
            A plan was shown in <b className="font-medium text-ink">{planShown?.value} of {planShown?.n}</b> runs
            {planSame?.value === 1 ? ", the same moves each time" : ", with different moves"}. Earlier recorded runs, with older settings, showed one in {earlier?.value} of {earlier?.n}.
          </p>
          <div className="text-[12px] text-dim md:text-right">
            <ul className="tabular-nums">
              {planRuns.map((r) => (
                <li key={r.i}>
                  run {r.i}: {r.shown?.value} moves shown, best check <b className="font-medium text-ink">{r.best?.value?.toFixed(2)}</b>
                </li>
              ))}
            </ul>
            <a href={decisionHref("D322")} target="_blank" rel="noreferrer" className="font-mono text-ok underline underline-offset-2">
              D322
            </a>
          </div>
        </div>
      )}
      {hasSplit && (
        <div className="grid gap-x-4 gap-y-2 border-t border-dashed border-line py-3 text-[12.5px] md:grid-cols-[200px_minmax(0,1fr)_230px]">
          <div>
            <p className="font-medium text-ink">
              Heat gun, cord split from switch <span className="tabular-nums font-normal text-dim">· {splitShown?.n} runs</span>
            </p>
            <p className="text-[11.5px] text-dim">same settings; template changed after seeing N357771, not a blind test</p>
          </div>
          <p className="self-center text-[12px] text-dim">
            CBP&apos;s own 2026 move (motor-fan assembly, PCBA, and power cord, knobs and covers) was shown in{" "}
            <b className="font-medium text-ink">
              {splitShown?.value} of {splitShown?.n}
            </b>{" "}
            runs. It was not ranked first: plans that move less cost less and rank above it, so the top plan moved fewer of the parts CBP&apos;s change moved.
          </p>
          <div className="text-[12px] text-dim md:text-right">
            <ul className="tabular-nums">
              {splitRuns.map((r) => (
                <li key={r.i}>
                  run {r.i}: check <b className="font-medium text-ink">{r.check?.value?.toFixed(2)}</b>, rank {r.rank?.value} of {r.rank?.n}; top plan moves {r.top?.value} of {r.top?.n}
                </li>
              ))}
            </ul>
            <a href={decisionHref("D332")} target="_blank" rel="noreferrer" className="font-mono text-ok underline underline-offset-2">
              D332
            </a>
          </div>
        </div>
      )}
      <p className="mt-3 text-[12px] text-dim">
        Filled marker: the official run, which is the published figure. The first held-out official run used Vertex AI and predates the fixed seed; the repeats went through an
        OpenAI-compatible gateway to the same Gemini model. Two repeats per held-out split is a small sample.
      </p>
    </figure>
  );
}
