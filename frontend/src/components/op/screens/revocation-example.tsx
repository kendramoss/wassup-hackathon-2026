"use client";

// A recorded, read-only revocation alert (D480, D481, D490) for the judge's
// tour (judge panel round 10, item 2b): no demo case relies on a ruling CBP
// later changed, so none shows the box. The data is one committed
// development answer, N356467, whose cited ruling N344694 CBP modified in
// H344999 after the case date; the box is worked out by the API's rule and
// rebuilt by `go run ./cmd/corpus alerts-example -check` (in make
// reproduce). No request is made and nothing is re-checked.
import type { CaseResponse } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { formatDate } from "@/lib/format";
import { badgeText } from "@/lib/ruling-alerts";
import ex from "@/lib/revocation-example.json";
import { Main } from "../chrome";
import { ExampleTour } from "../example-tour";
import { RulingAlertsBanner } from "../ruling-alerts";
import { Country, RulingLink } from "../ui";

const E = copy.revocationExample;
const repo = copy.landing.repo;
const sheet = "border border-edge bg-sheet px-5 py-4 shadow-[4px_4px_0_var(--shadow)]";

// The recorded alert, in the shape the case API sends, so the box is the one
// a saved case shows.
const recordedCase = { id: "", pinned: false, ruling_alerts: ex.ruling_alerts } as unknown as CaseResponse;

export function RevocationExample() {
  const badge = badgeText(ex.ruling_alerts.counts);
  const item = ex.ruling_alerts.items[0];
  return (
    <Main className="flex max-w-[860px] flex-col gap-6">
      <div>
        <p className="eyebrow">{E.eyebrow}</p>
        <h1 className="mt-1 text-[28px] tracking-[-0.05em]">{E.title}</h1>
        <p className="duo mt-2 max-w-[72ch] text-[13.5px]">{E.lede}</p>
        <ExampleTour at="/examples/revocation-alert" />
      </div>

      <section aria-labelledby="rx-answer" className={sheet}>
        <h2 id="rx-answer" className="eyebrow mb-2">
          {E.answer}
        </h2>
        <p className="read text-[17px]">{ex.subject}</p>
        <dl className="duo mt-2 grid grid-cols-1 gap-x-4 gap-y-1 text-[13px] sm:grid-cols-[max-content_minmax(0,1fr)]">
          <dt className="text-dim">{E.caseDate}</dt>
          <dd>{formatDate(ex.case_date)}</dd>
          <dt className="text-dim">{E.predicted}</dt>
          <dd>
            <Country name={ex.predicted_origin} /> <span className="text-dim">({ex.confidence.toFixed(2)} stated)</span>
          </dd>
          <dt className="text-dim">{E.cited}</dt>
          <dd>
            {ex.cited.map((id, i) => (
              <span key={id}>
                {i > 0 ? ", " : ""}
                <RulingLink id={id} />
              </span>
            ))}
          </dd>
          <dt className="text-dim">{copy.planRuleExample.cbpHeld}</dt>
          <dd>
            <Country name={ex.cbp_origin} /> <span className="text-dim">(<RulingLink id={ex.ruling} />)</span>
          </dd>
        </dl>
      </section>

      <section aria-labelledby="rx-box" className="flex flex-col gap-2">
        <div>
          <h2 id="rx-box" className="eyebrow">
            {E.box}
          </h2>
          <p className="duo mt-1 max-w-[72ch] text-[13px] text-body">{E.boxNote(formatDate(ex.as_of))}</p>
        </div>
        <RulingAlertsBanner c={recordedCase} setCase={() => {}} recorded />
        {badge ? (
          <p className="duo text-[13px] text-body">
            {E.listBadge} <span className={badge.attention ? "font-medium text-ask" : "text-dim"}>{badge.text}</span>
          </p>
        ) : null}
      </section>

      <section aria-labelledby="rx-change" className={sheet}>
        <h2 id="rx-change" className="eyebrow mb-2">
          {E.change}
        </h2>
        <ul className="duo flex flex-col gap-3 text-[13.5px]">
          {[...ex.changed_rulings, ...ex.changed_by].map((r) => (
            <li key={r.ruling} className="border-l-2 border-line pl-3">
              <p>
                <RulingLink id={r.ruling} /> <span className="text-dim">({formatDate(r.date)})</span> {r.subject}.
              </p>
              <p className="mt-0.5 text-body">
                {E.holding} <Country name={r.primary_origin} />. <q>{r.evidence[0]}</q>
              </p>
            </li>
          ))}
        </ul>
        {item.bulletin ? (
          <p className="duo mt-2 text-[12.5px] text-dim">
            {E.published}{" "}
            <a href={item.url} target="_blank" rel="noreferrer" className="text-ok underline underline-offset-2">
              {item.bulletin}
            </a>
            .
          </p>
        ) : null}
      </section>

      <section aria-labelledby="rx-recheck" className={sheet}>
        <h2 id="rx-recheck" className="eyebrow mb-2">
          {E.recheck}
        </h2>
        <ul className="duo flex list-disc flex-col gap-1.5 pl-5 text-[13.5px] text-body">
          {E.recheckBody.map((t) => (
            <li key={t}>{t}</li>
          ))}
        </ul>
        <p className="duo mt-2 max-w-[72ch] text-[13px] text-dim">{E.honest}</p>
      </section>

      <p className="duo text-[12.5px] text-dim">
        {E.source}{" "}
        <a href={`${repo}/blob/main/${ex.run}`} className="text-ok underline underline-offset-2 [overflow-wrap:anywhere]">
          {ex.run}
        </a>{" "}
        · <code className="[overflow-wrap:anywhere]">cd backend && go run ./cmd/corpus alerts-example -check</code>
      </p>
    </Main>
  );
}
