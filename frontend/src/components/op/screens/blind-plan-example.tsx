"use client";

// A recorded, read-only blind-test pair (#913): one of the 9 moves the
// precedent check showed in the post-cutoff flip test (D170, D192), picked
// because the product is known, the move is one step and the later ruling is
// dated after the 17 Oct freeze. The data is committed
// (eval/data/flips_post.jsonl and the known-a result rows); no request is made.
import { copy } from "@/lib/copy";
import { formatDate } from "@/lib/format";
import ex from "@/lib/blind-plan-example.json";
import { Main } from "../chrome";
import { Fold, RulingLink, Country } from "../ui";

const E = copy.blindPlan;
const repo = copy.landing.repo;
const sheet = "border border-edge bg-sheet px-5 py-4 shadow-[4px_4px_0_var(--shadow)]";

export function BlindPlanExample() {
  const c = ex.check;
  return (
    <Main className="flex max-w-[860px] flex-col gap-6">
      <div>
        <p className="eyebrow">{E.eyebrow}</p>
        <h1 className="mt-1 text-[28px] tracking-[-0.05em]">{E.title}</h1>
        <p className="duo mt-2 max-w-[72ch] text-[13.5px]">{E.lede}</p>
      </div>

      <section aria-labelledby="bp-rulings" className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {([[E.earlier, ex.earlier], [E.later, ex.later]] as const).map(([label, r]) => (
          <div key={r.ruling} className={sheet}>
            <h2 className="eyebrow mb-2">{label}</h2>
            <p className="read text-[17px]">
              <RulingLink id={r.ruling} /> <span className="text-dim">·</span> {formatDate(r.date)}
            </p>
            <p className="duo mt-2 text-[13px]">
              <span className="text-dim">{E.madeIn}: </span>
              <Country name={r.origin} />
            </p>
          </div>
        ))}
      </section>
      <h2 id="bp-rulings" className="sr-only">
        {ex.product}
      </h2>

      <section aria-labelledby="bp-moved" className={sheet}>
        <h2 id="bp-moved" className="eyebrow mb-1.5">
          {E.moved}
        </h2>
        <p className="read text-[15px]">{ex.moved}</p>
      </section>

      <section aria-labelledby="bp-check" className={sheet}>
        <h2 id="bp-check" className="eyebrow mb-1.5">
          {E.checkTitle}
        </h2>
        <p className="read text-[15px]">{E.checkBody(c.origin_after)}</p>
        <p className="duo mt-1.5 text-[13px] text-ok">{E.matched}</p>
        <p className="duo mt-1.5 text-[13px] text-body">
          {E.support} <RulingLink id={c.supporting_ruling} />
        </p>
        <p className="duo mt-1.5 text-[13px] text-body">{E.confidence(c.confidence, c.bar)}</p>
      </section>

      <section aria-labelledby="bp-dates" className={sheet}>
        <h2 id="bp-dates" className="eyebrow mb-1.5">
          {E.datesTitle}
        </h2>
        <p className="duo text-[13.5px] text-body">{E.dates(formatDate(ex.freeze.date), formatDate(ex.later.date), formatDate(ex.checked_before_ruling))}</p>
      </section>

      <div>
        <Fold title={E.details}>
          <p className="duo max-w-[72ch] text-[13px] text-dim">{E.notPlan}</p>
          <p className="duo mt-1.5 max-w-[72ch] text-[13px] text-dim">{E.edge}</p>
          <p className="duo mt-1.5 max-w-[72ch] text-[13px] text-dim">{E.pairFrozen(formatDate(ex.pair_frozen.date), ex.pair_frozen.commit)}</p>
          <p className="duo mt-1.5 max-w-[72ch] text-[13px] text-dim">{E.whole}</p>
          <p className="duo mt-1.5 text-[12.5px] text-dim">
            {E.source}{" "}
            <a href={`${repo}/blob/main/${ex.data}`} className="text-ok underline underline-offset-2">
              {ex.data}
            </a>
            ,{" "}
            <a href={`${repo}/blob/main/${ex.result}`} className="text-ok underline underline-offset-2">
              {ex.result}
            </a>
          </p>
        </Fold>
      </div>
    </Main>
  );
}
