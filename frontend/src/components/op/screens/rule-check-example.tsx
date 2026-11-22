"use client";

// A recorded, read-only example of the per-part HTS suggestions (D380) and
// the 19 CFR 102.20 rule check (D363), for the judge's tour (judge panel
// round 8, N11): the pinned demo case has no part codes, so its pages show
// neither. The data is one development ruling from the committed D382 run,
// rebuilt with the real rule check by `go run ./cmd/tariffshift
// parts-example -check` (in make reproduce). No request is made.
import type { CaseResponse, PartHtsCandidate } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import ex from "@/lib/rule-check-example.json";
import { Main } from "../chrome";
import { ExampleTour } from "../example-tour";
import { RuleCheckPanel, headline } from "../rule-check";
import { Chip, Country, RulingLink } from "../ui";

type RuleCheck = NonNullable<CaseResponse["rule_check"]>;

const E = copy.ruleCheckExample;
const P = copy.partsHts;
const repo = copy.landing.repo;

// The recorded result, in the shape the case API sends, so the panel is the
// same one a case shows.
function asCase(result: RuleCheck["result"], robustness?: RuleCheck["robustness"]): CaseResponse {
  const rc: RuleCheck = { rules: "19 CFR 102.20", applies: "part102", country: ex.country, country_from: "final assembly plant", review: false, note: "", result, robustness };
  return { id: "", rule_check: rc } as CaseResponse;
}

const sheet = "border border-edge bg-sheet px-5 py-4 shadow-[4px_4px_0_var(--shadow)]";

export function RuleCheckExample() {
  const before = ex.before as RuleCheck["result"];
  const after = ex.after as RuleCheck["result"];
  const beforeRc = asCase(before).rule_check!;
  const open = (before.materials ?? []).filter((m) => m.status === "missing_hts").map((m) => m.name);
  return (
    <Main className="flex max-w-[860px] flex-col gap-6">
      <div>
        <p className="eyebrow">{E.eyebrow}</p>
        <h1 className="mt-1 text-[28px] tracking-[-0.05em]">{E.title}</h1>
        <p className="duo mt-2 max-w-[72ch] text-[13.5px]">{E.lede}</p>
        <ExampleTour at="/examples/rule-check" />
      </div>

      <section aria-labelledby="ex-product" className={sheet}>
        <h2 id="ex-product" className="eyebrow mb-2">
          {E.product}
        </h2>
        <p className="read text-[17px]">{ex.product}</p>
        <dl className="duo mt-2 grid grid-cols-1 gap-x-4 gap-y-1 text-[13px] sm:grid-cols-[max-content_minmax(0,1fr)]">
          <dt className="text-dim">{E.madeIn}</dt>
          <dd>
            <Country name={ex.country} />
          </dd>
          <dt className="text-dim">{E.goodCode}</dt>
          <dd>
            <span className="tabular-nums [overflow-wrap:anywhere]">{ex.good_hts}</span>{" "}
            <span className="text-dim">
              ({E.fromRuling} <RulingLink id={ex.ruling} />, {ex.date})
            </span>
          </dd>
        </dl>
      </section>

      <section aria-labelledby="ex-parts" className={sheet}>
        <h2 id="ex-parts" className="eyebrow mb-1">
          {E.parts}
        </h2>
        <p className="duo text-[13px] text-body">{E.partsNote}</p>
        <p className="mt-1 text-xs text-ask">{P.unverified}</p>
        <ul className="mt-3 flex flex-col gap-4">
          {ex.parts.map((p) => (
            <li key={p.name} className="border-l-2 border-line pl-3">
              <p className="text-[14px] text-ink">
                {p.name} <span className="text-dim">·</span> <Country name={p.origin} className="text-[13px] text-body" />
              </p>
              <p className="duo text-[12.5px] text-dim">{"stated_hts" in p && p.stated_hts ? E.stated(p.stated_hts) : E.noneStated}</p>
              <Suggestions cands={p.suggestions as PartHtsCandidate[]} />
              {"note" in p && p.note ? (
                <p data-example-note className="duo mt-2 max-w-[72ch] border-l-2 border-ask-line pl-2.5 text-[12.5px] text-body">
                  <span className="text-ask">{E.ourNote}:</span> {p.note.replace(/^Our note, added after the run: /, "")}
                </p>
              ) : null}
            </li>
          ))}
        </ul>
      </section>

      <section aria-labelledby="ex-before" className={sheet}>
        <h2 id="ex-before" className="eyebrow mb-1.5">
          {E.before}
        </h2>
        <p className="duo text-[13.5px] text-body">
          {copy.ruleCheck.title}: {open.length ? copy.ruleCheck.addCodes(open.length, false) : headline(beforeRc).text}
          {open.length ? <span className="text-dim"> ({open.join(", ")})</span> : null}
        </p>
      </section>

      <section aria-labelledby="ex-after" className="flex flex-col gap-2">
        <h2 id="ex-after" className="eyebrow">
          {E.after}
        </h2>
        <RuleCheckPanel c={asCase(after, ex.robustness as RuleCheck["robustness"])} />
      </section>

      <section aria-labelledby="ex-cbp" className={sheet}>
        <h2 id="ex-cbp" className="eyebrow mb-1.5">
          {E.cbp}
        </h2>
        <p className="read text-[15px]">
          <Country name={ex.cbp_origin} /> <span className="text-dim">·</span> <RulingLink id={ex.ruling} />
        </p>
        <p className="duo mt-1.5 text-[13px] text-body">{ex.agrees ? E.agrees : E.differs}</p>
        <p className="duo mt-1.5 max-w-[72ch] text-[13px] text-dim">{E.honest}</p>
      </section>

      <p className="duo text-[12.5px] text-dim">
        {E.source}{" "}
        <a href={`${repo}/blob/main/${ex.run}`} className="text-ok underline underline-offset-2">
          {ex.run}
        </a>{" "}
        · <code className="[overflow-wrap:anywhere]">cd backend && go run ./cmd/tariffshift parts-example -check</code>
      </p>
    </Main>
  );
}

// The recorded suggestions, read only: the same labels as the parts table,
// without the "Use" buttons.
function Suggestions({ cands }: { cands: PartHtsCandidate[] }) {
  if (!cands.length) return <p className="text-[12.5px] text-dim">{P.none}</p>;
  return (
    <ol className="mt-1.5 flex flex-col gap-1.5">
      {cands.map((c, i) => {
        const rulings = c.source === "rulings";
        return (
          <li key={c.hts} className="flex flex-wrap items-baseline gap-x-2.5 gap-y-1 text-[12.5px]">
            <span className={i === 0 ? "min-w-16 border border-ok bg-ok-wash px-2 text-ok tabular-nums" : "min-w-16 border border-line bg-white px-2 tabular-nums"}>{c.hts}</span>
            <Chip tone={rulings ? "ok" : "ask"}>{rulings ? P.fromRulings : P.fromModel}</Chip>
            <span className="duo min-w-0 basis-60 flex-1 text-body">
              {c.reason}
              {rulings && c.supporting_rulings?.length ? (
                <span className="text-dim">
                  {" "}
                  (
                  {c.supporting_rulings.map((r, k) => (
                    <span key={r.id}>
                      {k > 0 && ", "}
                      <RulingLink id={r.id ?? ""} />
                    </span>
                  ))}
                  )
                </span>
              ) : null}
            </span>
          </li>
        );
      })}
    </ol>
  );
}
