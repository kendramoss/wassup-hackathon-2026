"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useRef, useState } from "react";
import { api, withStaleFlags, type CaseResponse, type TargetSummary } from "@/lib/api/client";
import { VIEWER_REASON, caseReadOnly } from "@/lib/roles";
import { copy } from "@/lib/copy";
import { conf, money, originShort } from "@/lib/format";
import { CaseShell, type CaseCtx } from "../case-shell";
import { DemoNote, Elapsed, ErrorBox, Footer, Main, Next } from "../chrome";
import { BandNet, OpsDetail, OpsScope, StaleMoneyBanner } from "../ops-result";
import { netText } from "../ops-view";
import { TierChip, moveKey, movePhrase } from "../plan-bits";
import { PlanRuleChip, PlanRuleNote } from "../plan-rule";
import { ReviewNote } from "../review-note";
import { Button, ButtonLink, Chip, Country, Dimension, Flag, Fold, MiniRuler, Ruler, cx } from "../ui";

export function CompareView() {
  return <CaseShell tab="plan">{(ctx) => <Compare {...ctx} />}</CaseShell>;
}

const statusTag: Record<string, React.ReactNode> = {
  plan: <Chip tone="ok">Plan</Chip>,
  below_bar: <Chip tone="ask">△ Below the bar</Chip>,
  no_plan: <Chip>No supported plan</Chip>,
  no_plant: <Chip>No capable plant</Chip>,
};

// The case stores the latest ranking (D36) though the OpenAPI CaseResponse
// doesn't list it yet.
function storedRanking(c: CaseResponse): TargetSummary[] | null {
  const t = (c as CaseResponse & { targets?: { ranked?: TargetSummary[] } }).targets;
  return t?.ranked ?? null;
}

function Compare({ id, c, setCase }: CaseCtx) {
  const router = useRouter();
  const origin = c.prediction?.origin_country;
  const options = useMemo(() => [...new Set((c.product?.plants ?? []).map((p) => p.country).filter(Boolean))], [c.product]);
  const [picked, setPicked] = useState<string[]>(() => {
    // A stored ranking was run on some countries: start from those, so the
    // chips match the table below them (#152).
    const stored = (storedRanking(c) ?? []).map((t) => t.target).filter((t): t is string => Boolean(t));
    if (stored.length) return stored.slice(0, 4);
    const others = options.filter((o) => o !== origin);
    return (others.length >= 2 ? others : options).slice(0, 4);
  });
  const [ranked, setRanked] = useState<TargetSummary[] | null>(() => storedRanking(c));
  // The stored ranking was priced from money that has since changed (#281):
  // its amounts are hidden until a new comparison runs. Which countries have a
  // supported move is not money and stays.
  const stale = Boolean(c.targets_economics_stale);
  const [since, setSince] = useState<number | null>(null);
  const [error, setError] = useState<unknown>(null);
  const abort = useRef<AbortController | null>(null);
  useEffect(() => () => abort.current?.abort(), []);

  const run = async () => {
    abort.current = new AbortController();
    setError(null);
    setSince(Date.now());
    try {
      const r = await api.compareTargets(id, picked, abort.current.signal);
      setRanked(r.targets ?? []);
      // A comparison just run is fresh by construction.
      setCase((d) => (d ? withStaleFlags(d, { plan_economics_stale: d.plan_economics_stale }) : d));
    } catch (e) {
      if (!(e instanceof DOMException && e.name === "AbortError")) setError(e);
    } finally {
      setSince(null);
    }
  };

  const draft = (t: TargetSummary) => {
    if (!t.best) return;
    router.push(`/cases/${encodeURIComponent(id)}/draft?move=${encodeURIComponent(moveKey(t.best))}&target=${encodeURIComponent(t.target ?? "")}`);
  };

  const top = ranked?.[0];
  // The 102.20 check on each country's best move (D441, D500).
  const ruleChecked = ranked?.some((t) => t.best?.rule_check);
  const part102 = ranked?.some((t) => t.best?.rule_check?.applies === "part102");
  const anyPlan = ranked?.some((t) => t.status === "plan");
  const shownPlanConf = c.plan?.plans?.[0]?.verdict?.confidence;

  return (
    <>
      <section aria-label="Comparison summary" className="border-b border-dashed border-guide">
        <div className="column relative mx-auto grid max-w-[1240px] items-end gap-8 px-4 pb-8 pt-10 sm:px-10 lg:grid-cols-[minmax(0,1fr)_280px_250px] lg:gap-11">
          {since ? (
            <>
              <div>
                <p className="eyebrow">Comparing {picked.length} countries</p>
                <h1 className="duo mt-1 text-[26px] font-normal leading-tight tracking-[-0.05em]">Searching moves to {picked.join(", ")}</h1>
              </div>
              <div>
                <p className="eyebrow">Elapsed</p>
                <p className="mt-1 text-[44px] leading-none tracking-[-0.05em]">
                  <Elapsed since={since} />
                </p>
                <p className="mt-2 text-xs text-dim">can take a minute or two · results arrive together</p>
              </div>
              <div>
                <p className="eyebrow">Progress</p>
                <div className="relative mt-3 h-1 overflow-hidden bg-wash" role="progressbar" aria-label="Searching">
                  <i className="absolute inset-y-0 w-1/3 animate-[slide_1.6s_ease-in-out_infinite] bg-ok" />
                </div>
              </div>
            </>
          ) : top?.best ? (
            <>
              <div>
                <p className={cx("eyebrow", !anyPlan && "text-ask")}>Step 3 of 4 · {anyPlan ? "Best country" : "No country cleared the 0.92 precedent-check bar"}</p>
                <h1 className="duo mt-1 text-[26px] font-normal leading-tight tracking-[-0.05em]">
                  {anyPlan ? "Move " : "Closest: move "}
                  {movePhrase(top.best)} to{" "}
                  <span className="inline-flex items-center gap-2 whitespace-nowrap">
                    <Flag country={top.target} size={26} />
                    {top.target}
                  </span>
                </h1>
                <div className="mt-4 flex flex-wrap gap-2">
                  {anyPlan ? <Chip tone="ok">{copy.plan.supported}</Chip> : <Chip tone="ask">△ Below the bar · {conf(top.best.verdict?.confidence)}</Chip>}
                  <Chip>
                    Reaches <Country name={top.best.prediction?.origin_country} /> · origin {originShort(top.best.prediction)}
                  </Chip>
                </div>
              </div>
              <div>
                <BandNet plan={top.best} below={!anyPlan} stale={stale} />
              </div>
              <div>
                <p className="eyebrow">Precedent check</p>
                <Ruler value={top.best.verdict?.confidence} caption={`check confidence ${conf(top.best.verdict?.confidence)}`} />
              </div>
            </>
          ) : (
            <div>
              <p className="eyebrow">Step 3 of 4 · {copy.plan.compare}</p>
              <h1 className="duo mt-1 text-[26px] font-normal leading-tight tracking-[-0.05em]">Plans per country, ranked.</h1>
              {ranked && <p className="duo mt-2 text-sm text-ask">No country found a precedent-supported move.</p>}
            </div>
          )}
        </div>
      </section>
      <Main className="flex flex-col gap-7">
        <fieldset>
          <legend className="text-[11px] uppercase tracking-[0.08em] text-dimension">Countries to compare · 2–4</legend>
          <div className="mt-2.5 flex flex-wrap gap-2.5">
            {options.map((o) => {
              const on = picked.includes(o);
              return (
                <label key={o} className={cx("inline-flex cursor-pointer items-center gap-2 border px-2.5 py-1.5 text-[13px]", on ? "border-ok text-ok shadow-[2px_2px_0_var(--shadow)]" : "border-line bg-sheet")}>
                  <input
                    type="checkbox"
                    className="sr-only"
                    checked={on}
                    disabled={Boolean(since) || Boolean(c.pinned)}
                    onChange={() => setPicked(on ? picked.filter((x) => x !== o) : picked.length < 4 ? [...picked, o] : picked)}
                  />
                  <Country name={o} />
                  <span aria-hidden="true">{on ? "×" : "+"}</span>
                </label>
              );
            })}
          </div>
          {options.length < 2 && <p className="duo mt-2 text-xs text-ask">Add plants in at least two countries on the Product step to compare them.</p>}
        </fieldset>
        {/* A pinned demo keeps its recorded comparison: a new one wouldn't be
            saved, so its drafts couldn't be made (18 Oct). */}
        {c.pinned ? (
          <>
            <DemoNote
              title={c.display_name}
              message={
                ranked
                  ? "Pinned demo: its recorded comparison is shown. Start a new case to compare others."
                  : "Pinned demo: comparisons here are not saved."
              }
            />
            {!ranked && <RecordedCompare c={c} />}
          </>
        ) : (
          !since && (
            <div>
              <Button onClick={run} disabled={picked.length < 2 || caseReadOnly(c)} title={caseReadOnly(c) ? VIEWER_REASON : undefined}>
                {ranked ? "Compare again" : "Compare these countries"}
              </Button>
            </div>
          )
        )}
        {error ? <ErrorBox error={error} onRetry={run} demoTitle={c.display_name} /> : null}
        {stale && ranked && !since && <StaleMoneyBanner kind="targets" onRun={c.pinned ? undefined : run} readOnly={caseReadOnly(c)} busy={picked.length < 2} />}

        {(ranked || since) && (
          <div className="relative overflow-x-auto">
            <table className="w-full min-w-[820px] border-collapse border border-edge bg-sheet text-[13px] shadow-[4px_4px_0_var(--shadow)]">
              <caption className="pb-2.5 text-left text-[11px] uppercase tracking-[0.08em] text-dimension">
                Ranked · plan (clear margin first, then at the bar), then below the bar, then no supported plan, then no capable plant
                {part102 ? `; ${copy.planRule.compareCaption}` : ""}
              </caption>
              <thead>
                <tr>
                  {["#", "Country", "Status", "Best move", "Precedent check", "Net per year", ""].map((h, i) => (
                    <th key={i} scope="col" className="border-b border-line bg-paper px-3.5 py-2.5 text-left text-[11px] font-medium uppercase tracking-[0.08em] text-dim">
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {since
                  ? picked.map((t) => (
                      <tr key={t} className="border-b border-line text-dim">
                        <td className="px-3.5 py-3.5">—</td>
                        <td className="px-3.5 py-3.5 text-[15px] text-ink">
                          <Country name={t} />
                        </td>
                        <td className="px-3.5 py-3.5" colSpan={5}>
                          Searching moves to {t}…
                        </td>
                      </tr>
                    ))
                  : ranked!.map((t) => (
                      <tr key={t.target} className={cx("border-b border-line align-top last:border-0", !t.best && "text-dim")}>
                        <td className="px-3.5 py-3.5 font-mono text-dim">{t.rank}</td>
                        <td className="px-3.5 py-3.5 text-[15px] tracking-[-0.03em] text-ink">
                          <Country name={t.target} size={13} />
                        </td>
                        <td className="px-3.5 py-3.5">{statusTag[t.status ?? ""] ?? t.status}</td>
                        <td className="duo max-w-[34ch] px-3.5 py-3.5 text-ink">
                          {t.best ? t.best.description : `No move to ${t.target} found precedent support`}
                          <span className="mt-1 block text-[11.5px] text-dim">
                            {t.best ? `Reaches ${t.best.prediction?.origin_country} · origin ${originShort(t.best.prediction)} · ` : ""}
                            {t.moves_searched} moves searched
                          </span>
                          {t.best?.rule_check?.label ? (
                            <span className="mt-1.5 block">
                              <PlanRuleChip plan={t.best} explain />
                              <PlanRuleNote plan={t.best} className="mt-1" />
                            </span>
                          ) : null}
                        </td>
                        <td className="px-3.5 py-3.5">
                          {t.best ? (
                            <>
                              <MiniRuler value={t.best.verdict?.confidence} />
                              <span className="text-xs text-dim">check {conf(t.best.verdict?.confidence)} · precedent-check bar 0.92</span>
                              {t.status === "plan" && (
                                <span className="mt-1.5 block">
                                  <TierChip plan={t.best} />
                                </span>
                              )}
                            </>
                          ) : (
                            "—"
                          )}
                        </td>
                        <td className="whitespace-nowrap px-3.5 py-3.5 text-[17px] tracking-[-0.04em] text-dim">
                          {t.best ? (stale ? <span className="text-[13px]">{copy.stale.hidden}</span> : netText(t.best)) : "—"}
                          {t.best && !stale && !t.best.operations && (
                            <span className="block text-[11px] tracking-normal">
                              {money(t.best.duty_effect_per_year)} saved − {money(t.best.added_cost_per_year)}
                            </span>
                          )}
                          {t.best?.operations && !stale && (
                            <>
                              <OpsScope ops={t.best.operations} className="mt-1 max-w-[30ch] whitespace-normal text-[11px] tracking-normal" />
                              <OpsDetail ops={t.best.operations} className="mt-1 tracking-normal" />
                            </>
                          )}
                        </td>
                        <td className="px-3.5 py-3.5">
                          {t.best && (t.status === "plan" || t.status === "below_bar") ? (
                            <button type="button" className="whitespace-nowrap text-xs text-ok hover:underline" onClick={() => draft(t)}>
                              Draft{t.status === "below_bar" ? " (below bar)" : ""} →
                            </button>
                          ) : null}
                        </td>
                      </tr>
                    ))}
              </tbody>
            </table>
          </div>
        )}
        {ranked && !since && <ReviewNote place="compare" />}
        <div>
          <Fold title="How the ranking works">
            <p className="duo max-w-[90ch] text-[13px] text-dim">
              Countries with a shown plan rank by their best plan, the same way plans rank on the Plan step: a clear-margin plan (check 0.93 or more, no counter-ruling) ranks above one at the bar, then by net per year. Moves only go to plants your product lists; a country with no capable plant ranks last rather than being invented. Money is illustrative — editable on the Plan step. Precedent checks vary between runs.
            </p>
            {ruleChecked ? <p className="duo mt-2 max-w-[90ch] text-[13px] text-dim">{copy.planRule.compareHow}</p> : null}
          </Fold>
        </div>
      </Main>
      <Footer>
        <ButtonLink variant="ghost" href={`/cases/${encodeURIComponent(id)}/plan`}>
          ← Back to plan
        </ButtonLink>
        <Next>{since ? "Searching each country in parallel. Usually ~30–40 s." : ranked ? `Draft for ${top?.best ? top.target : "a country"}, or go back to the plan.` : "Pick 2–4 countries and compare them."}</Next>
        <div className="ml-auto flex flex-wrap items-center gap-2.5">
          {since && (
            <Button variant="ghost" onClick={() => abort.current?.abort()}>
              Cancel
            </Button>
          )}
          {top?.best && !anyPlan && shownPlanConf !== undefined && (
            <span className="hidden text-xs text-dim sm:inline">Weaker than the {c.plan?.target} plan ({conf(shownPlanConf)})</span>
          )}
          {top?.best && (top.status === "plan" || top.status === "below_bar") && !since ? (
            <Button onClick={() => draft(top)}>Draft for {top.target} →</Button>
          ) : since ? null : top ? (
            <ButtonLink href={`/cases/${encodeURIComponent(id)}/draft`}>Draft for the current setup →</ButtonLink>
          ) : null}
        </div>
      </Footer>
    </>
  );
}

// A pinned demo with no recorded comparison is a dead end for a visitor, so
// point to the pinned demo of the same product whose comparison is recorded
// (judge journey: the heat gun's comparison is its own demo case).
function RecordedCompare({ c }: { c: CaseResponse }) {
  const [href, setHref] = useState<string | null>(null);
  useEffect(() => {
    let live = true;
    api.listCases().then(
      (r) => {
        const other = (r?.cases ?? []).find((x) => x.pinned && x.id !== c.id && x.product_name === c.product?.name && /compare/i.test(x.display_name ?? ""));
        if (live && other) setHref(`/cases/${encodeURIComponent(other.id)}/compare`);
      },
      () => {},
    );
    return () => {
      live = false;
    };
  }, [c.id, c.product?.name]);
  if (!href) return null;
  return (
    <p className="duo text-[13.5px]">
      <Link href={href} className="text-ok underline underline-offset-2">
        See a recorded comparison →
      </Link>
    </p>
  );
}
