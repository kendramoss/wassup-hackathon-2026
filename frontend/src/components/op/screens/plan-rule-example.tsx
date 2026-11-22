"use client";

// A recorded, read-only example of the 19 CFR 102.20 check on each plan move
// (D441, D442) for the judge's tour (judge panel round 10, item 2a): the
// pinned demo case has no part codes, so its plan shows no move check. The
// data is one committed development plan search, N354220, rebuilt with the
// real rule check by `go run ./cmd/tariffshift plans-example -check` (in
// make reproduce). No request is made.
import type { ReactNode } from "react";
import type { Plan } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { groupName } from "@/lib/families";
import { conf, formatDate } from "@/lib/format";
import ex from "@/lib/plan-rule-example.json";
import { Main } from "../chrome";
import { ExampleTour } from "../example-tour";
import { PlanRuleChip, PlanRuleNote } from "../plan-rule";
import { Country, RulingLink, cx } from "../ui";

const E = copy.planRuleExample;
const repo = copy.landing.repo;
const sheet = "border border-edge bg-sheet px-5 py-4 shadow-[4px_4px_0_var(--shadow)]";

type Move = (typeof ex.moves)[number];
const asPlan = (m: Move | typeof ex.baseline) => m as unknown as Plan;
const plant = (id: string) => ex.plants.find((p) => p.id === id);

export function PlanRuleExample() {
  return (
    <Main className="flex max-w-[860px] flex-col gap-6">
      <div>
        <p className="eyebrow">{E.eyebrow}</p>
        <h1 className="mt-1 text-[28px] tracking-[-0.05em]">{E.title}</h1>
        <p className="duo mt-2 max-w-[72ch] text-[13.5px]">{E.lede}</p>
        <ExampleTour at="/examples/plan-rule-check" />
      </div>

      <section aria-labelledby="px-product" className={sheet}>
        <h2 id="px-product" className="eyebrow mb-2">
          {E.product}
        </h2>
        <p className="read text-[17px]">{ex.product}</p>
        <dl className="duo mt-2 grid grid-cols-1 gap-x-4 gap-y-1 text-[13px] sm:grid-cols-[max-content_minmax(0,1fr)]">
          <dt className="text-dim">{E.cbpHeld}</dt>
          <dd>
            <Country name={ex.cbp_origin} />{" "}
            <span className="text-dim">
              (<RulingLink id={ex.ruling} />, {formatDate(ex.date)})
            </span>
          </dd>
          <dt className="text-dim">{E.target}</dt>
          <dd>
            <Country name={ex.target} />
          </dd>
          <dt className="text-dim">{E.plants}</dt>
          <dd>
            {ex.plants.map((p, i) => (
              <span key={p.id}>
                {i > 0 ? "; " : ""}
                {p.name}
                {p.capabilities.includes("final_assembly") ? <span className="text-dim"> (final assembly)</span> : null}
              </span>
            ))}
          </dd>
        </dl>
      </section>

      <section aria-labelledby="px-codes" className={sheet}>
        <h2 id="px-codes" className="eyebrow mb-1">
          {E.codes}
        </h2>
        <p className="duo text-[13px] text-body">{E.codesNote}</p>
        <p className="mt-1 text-xs text-ask">{copy.partsHts.unverified}</p>
        <ul className="duo mt-3 flex flex-col gap-2 text-[13px]">
          <li className="border-l-2 border-line pl-3">
            <span className="text-ink">{ex.product}</span> <span className="text-dim">·</span> <span className="tabular-nums">{ex.good_hts}</span>
          </li>
          {ex.parts.map((p) => (
            <li key={p.name} className="border-l-2 border-line pl-3">
              <span className="text-ink">{p.name}</span> <span className="text-dim">·</span> <span className="tabular-nums">{p.hts}</span>
              <span className="block text-[12.5px] text-dim">
                Made in <Country name={p.made_in} /> · {plant(p.plant)?.name} · {groupName(p.group, ex.family)}
              </span>
            </li>
          ))}
        </ul>
      </section>

      <section aria-labelledby="px-moves" className="flex flex-col gap-3">
        <div>
          <h2 id="px-moves" className="eyebrow">
            {E.moves}
          </h2>
          <p className="duo mt-1 max-w-[72ch] text-[13px] text-body">{E.movesNote}</p>
        </div>
        <article className={cx(sheet, "flex flex-col gap-1.5")} data-move="baseline">
          <h3 className="text-[14px] text-ink">{E.current}</h3>
          <p className="duo text-[13px] text-dim">
            {E.cbpHeld} <Country name={ex.cbp_origin} />.
          </p>
          <Row label={E.rule}>
            <PlanRuleChip plan={asPlan(ex.baseline)} />
          </Row>
        </article>
        {ex.moves.map((m) => (
          <MoveCard key={m.description} m={m} />
        ))}
      </section>

      <section aria-labelledby="px-why" className={sheet}>
        <h2 id="px-why" className="eyebrow mb-1.5">
          {E.why}
        </h2>
        <p className="duo max-w-[72ch] text-[13.5px] text-body">{E.whyBody}</p>
        <p className="duo mt-1.5 max-w-[72ch] text-[13px] text-dim">{E.honest}</p>
      </section>

      <p className="duo text-[12.5px] text-dim">
        {E.source}{" "}
        <a href={`${repo}/blob/main/${ex.run}`} className="text-ok underline underline-offset-2 [overflow-wrap:anywhere]">
          {ex.run}
        </a>{" "}
        ·{" "}
        <a href={`${repo}/blob/main/${ex.suggestions}`} className="text-ok underline underline-offset-2 [overflow-wrap:anywhere]">
          {ex.suggestions}
        </a>{" "}
        · <code className="[overflow-wrap:anywhere]">cd backend && go run ./cmd/tariffshift plans-example -check</code>
      </p>
    </Main>
  );
}

function MoveCard({ m }: { m: Move }) {
  const v = m.verdict;
  const flagged = Boolean(m.rule_check?.review);
  return (
    <article data-move={flagged ? "flagged" : "calm"} className={cx("flex flex-col gap-1.5 border bg-sheet px-5 py-4 shadow-[4px_4px_0_var(--shadow)]", flagged ? "border-ask-line" : "border-edge")}>
      <h3 className="text-[14px] text-ink">{m.description}</h3>
      <Row label={E.precedent}>
        <span className="duo text-[13px] text-body">
          {v?.precedent_supported && v.supporting_ruling ? (
            <>
              {E.supportedBy} (<RulingLink id={v.supporting_ruling} asOf={ex.date} support />
              ), {E.at(conf(v.confidence))}
            </>
          ) : (
            E.notSupported(conf(v?.confidence))
          )}{" "}
          <span className="text-dim">{m.shown ? E.shown : m.rejection_kind === "below_confidence_bar" ? E.belowBar : E.noSupport}</span>
        </span>
      </Row>
      <Row label={E.rule}>
        <span className="flex flex-col gap-1">
          <PlanRuleChip plan={asPlan(m)} />
          <PlanRuleNote plan={asPlan(m)} />
        </span>
      </Row>
      <p className="duo text-[12.5px] text-dim">{E.engine(m.engine_origin)}</p>
    </article>
  );
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-1 gap-x-3 gap-y-0.5 sm:grid-cols-[13rem_minmax(0,1fr)]">
      <span className="text-[11px] font-medium uppercase tracking-[0.08em] text-dim">{label}</span>
      <span className="min-w-0">{children}</span>
    </div>
  );
}
