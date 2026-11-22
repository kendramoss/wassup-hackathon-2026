"use client";

// A recorded, read-only example of a Part 102 plan search (judge panel round
// 11, item 3): the scored D501 search for N348465, an actuator finished in
// Canada from Chinese parts. Each move carries the 19 CFR 102.20 check as on
// a Part 102 case (D441), the shown plans are in the D500 order, and the
// broker draft's Part 102 question and tariff-shift paragraph come from the
// export package (D502, D520). Rebuilt by `go run ./cmd/tariffshift
// plans102-example -check` (in make reproduce). No request is made.
import type { ReactNode } from "react";
import type { Plan } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { groupName } from "@/lib/families";
import { conf, formatDate } from "@/lib/format";
import ex from "@/lib/part102-plan-example.json";
import { Main } from "../chrome";
import { ExampleTour } from "../example-tour";
import { PlanRuleChip, PlanRuleNote } from "../plan-rule";
import { Country, RulingLink, cx } from "../ui";

const E = copy.part102Example;
const repo = copy.landing.repo;
const sheet = "border border-edge bg-sheet px-5 py-4 shadow-[4px_4px_0_var(--shadow)]";

// One move as the builder writes it (the JSON's inferred types differ per
// array, so the page reads one shape).
type Move = {
  key: string;
  description: string;
  parts_moved: string[];
  moves_motor: boolean;
  shown: boolean;
  rejection_kind?: string;
  verdict?: { precedent_supported: boolean; supporting_ruling?: string; confidence: number };
  engine_origin?: string;
  rule_check?: { status: string; review?: boolean };
};
const asPlan = (m: Move | typeof ex.baseline) => m as unknown as Plan;
const shown = ex.shown as Move[];
const others = ex.others as Move[];
const motorMoves = others.filter((m) => m.moves_motor);
const otherMoves = others.filter((m) => !m.moves_motor);
const isSmallest = (m: Move) => m.parts_moved.length === 1 && m.moves_motor;

export function Part102PlanExample() {
  return (
    <Main className="flex max-w-[860px] flex-col gap-6">
      <div>
        <p className="eyebrow">{E.eyebrow}</p>
        <h1 className="mt-1 text-[28px] tracking-[-0.05em]">{E.title}</h1>
        <p className="duo mt-2 max-w-[72ch] text-[13.5px]">{E.lede}</p>
        <ExampleTour at="/examples/part102-plan" />
      </div>

      <section aria-labelledby="p102-product" className={sheet}>
        <h2 id="p102-product" className="eyebrow mb-2">
          {E.product}
        </h2>
        <p className="read text-[17px]">{ex.product}</p>
        <dl className="duo mt-2 grid grid-cols-1 gap-x-4 gap-y-1 text-[13px] sm:grid-cols-[max-content_minmax(0,1fr)]">
          <dt className="text-dim">{E.cbpSubject}</dt>
          <dd>{ex.subject}</dd>
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
          <dt className="text-dim">{E.order}</dt>
          <dd>
            <code>{ex.ranking}</code>: {E.orderBody}
          </dd>
        </dl>
      </section>

      <section aria-labelledby="p102-codes" className={sheet}>
        <h2 id="p102-codes" className="eyebrow mb-1">
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
                Made in <Country name={p.made_in} /> · {groupName(p.group, ex.family)}
              </span>
            </li>
          ))}
        </ul>
      </section>

      <section aria-labelledby="p102-moves" className="flex flex-col gap-3">
        <article className={cx(sheet, "flex flex-col gap-1.5")} data-move="baseline">
          <h2 id="p102-moves" className="text-[14px] text-ink">
            {E.current}
          </h2>
          <p className="duo max-w-[72ch] text-[13px] text-body">{E.currentBody}</p>
          <Row label={E.rule}>
            <PlanRuleChip plan={asPlan(ex.baseline)} />
          </Row>
        </article>

        <h2 className="eyebrow mt-2">{E.shownHead}</h2>
        {shown.map((m) => (
          <MoveCard key={m.key} m={m} />
        ))}

        <div className="mt-2">
          <h2 className="eyebrow">{E.motorHead(motorMoves.length)}</h2>
          <p className="duo mt-1 max-w-[72ch] text-[13px] text-body">{E.motorNote}</p>
        </div>
        {motorMoves.map((m) => (
          <MoveCard key={m.key} m={m} />
        ))}

        <div className="mt-2">
          <h2 className="eyebrow">{E.otherHead(otherMoves.length)}</h2>
          <p className="duo mt-1 max-w-[72ch] text-[13px] text-body">{E.otherNote}</p>
        </div>
        {otherMoves.map((m) => (
          <MoveCard key={m.key} m={m} />
        ))}
      </section>

      <section aria-labelledby="p102-draft" className={cx(sheet, "flex flex-col gap-3")}>
        <div>
          <h2 id="p102-draft" className="eyebrow">
            {E.draft}
          </h2>
          <p className="duo mt-1 max-w-[72ch] text-[13px] text-dim">
            {ex.draft.plan}. {E.draftNote}
          </p>
        </div>
        <Quote label={E.question} text={ex.draft.question} />
        <Quote label={E.paragraph} text={ex.draft.tariff_shift} />
        {ex.draft.notes.map((n) => (
          <Quote key={n} label={E.reviewer} text={n} dim />
        ))}
      </section>

      <section className={sheet}>
        <p className="duo max-w-[72ch] text-[13px] text-body">{E.honest}</p>
        <p className="duo mt-1.5 max-w-[72ch] text-[13px] text-dim">{E.nameNote}</p>
      </section>

      <p className="duo text-[12.5px] text-dim">
        {E.source}{" "}
        <a href={`${repo}/blob/main/${ex.run}`} className="text-ok underline underline-offset-2 [overflow-wrap:anywhere]">
          {ex.run}
        </a>{" "}
        ·{" "}
        <a href={`${repo}/blob/main/${ex.fixture}`} className="text-ok underline underline-offset-2 [overflow-wrap:anywhere]">
          {ex.fixture}
        </a>{" "}
        · <code className="[overflow-wrap:anywhere]">cd backend && go run ./cmd/tariffshift plans102-example -check</code>
      </p>
    </Main>
  );
}

function MoveCard({ m }: { m: Move }) {
  const v = m.verdict;
  const flagged = Boolean(m.rule_check?.review);
  const engineDisagrees = m.engine_origin === ex.target && m.rule_check?.status !== "shifts";
  return (
    <article
      data-move={m.shown ? "shown" : m.moves_motor ? "motor" : "other"}
      className={cx("flex flex-col gap-1.5 border bg-sheet px-5 py-4 shadow-[4px_4px_0_var(--shadow)]", flagged ? "border-ask-line" : m.shown ? "border-ok" : "border-edge")}
    >
      <h3 className="text-[14px] text-ink">{m.description}</h3>
      <p className="duo text-[12.5px] text-dim">{E.parts(m.parts_moved.join(", "))}</p>
      {isSmallest(m) ? <p className="duo text-[13px] text-ink">{E.smallest}</p> : null}
      <Row label={E.precedent}>
        <span className="duo text-[13px] text-body">
          {!v ? (
            E.noCheck
          ) : v.precedent_supported && v.supporting_ruling ? (
            <>
              {E.supportedBy} (<RulingLink id={v.supporting_ruling} asOf={ex.date} support />
              ), {E.at(conf(v.confidence))}
            </>
          ) : (
            E.notSupported(conf(v.confidence))
          )}{" "}
          <span className="text-dim">{m.shown ? E.shown : m.rejection_kind === "below_confidence_bar" ? E.belowBar : v ? E.noSupport : ""}</span>
        </span>
      </Row>
      <Row label={E.rule}>
        <span className="flex flex-col gap-1">
          <PlanRuleChip plan={asPlan(m)} />
          <PlanRuleNote plan={asPlan(m)} />
        </span>
      </Row>
      <p className="duo text-[12.5px] text-dim">
        {E.engine(m.engine_origin ?? "")} {engineDisagrees ? <span className="text-ask">{E.engineDisagrees}</span> : null}
      </p>
    </article>
  );
}

function Quote({ label, text, dim }: { label: string; text: string; dim?: boolean }) {
  return (
    <div className="flex flex-col gap-1">
      <span className="text-[11px] font-medium uppercase tracking-[0.08em] text-dim">{label}</span>
      {text.split("\n\n").map((para) => (
        <p key={para} className={cx("read max-w-[72ch] border-l-2 border-line pl-3 text-[13.5px]", dim ? "text-dim" : "text-body")}>
          {para}
        </p>
      ))}
    </div>
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
