import type { CaseResponse } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { Country, Fold, cx } from "./ui";

type RuleCheck = NonNullable<CaseResponse["rule_check"]>;

// The 19 CFR 102.20 rule check (D363): a deterministic tariff-shift test from
// the HTS codes on the parts list, shown beside the model's answer. It is
// display only; the verdict above, the route and auto-accept never read it.
// The backend sends it only when the product gives codes, so most cases show
// nothing here.
export function headline(rc: RuleCheck): { text: string; tone?: "ask" } {
  const R = copy.ruleCheck;
  const r = rc.result;
  if (r.status === "decided") {
    if (rc.review) return { text: R.differs(rc.model_origin ?? ""), tone: "ask" };
    return { text: R.decided };
  }
  if (r.status === "out_of_scope") return { text: R.outOfScope };
  if (r.if_not_de_minimis?.origin) return { text: R.deMinimis(rc.country, r.if_not_de_minimis.origin) };
  return { text: R.undetermined };
}

// The API's reason is a clause; the panel shows it as a sentence.
function sentence(s: string): string {
  const t = s.trim();
  return t ? t[0].toUpperCase() + t.slice(1) + (/[.!?]$/.test(t) ? "" : ".") : t;
}

const statusWords: Record<string, string> = {
  shift: "shifts",
  no_shift: "does not shift",
  needs_fact: "needs a fact",
  missing_hts: "no HTS code",
};

export function RuleCheckPanel({ c }: { c: CaseResponse }) {
  const rc = c.rule_check;
  if (!rc) return null;
  const R = copy.ruleCheck;
  const r = rc.result;
  const h = headline(rc);
  return (
    <section
      aria-label={R.title}
      data-rule-check={r.status}
      className={cx("max-w-[75ch] border px-4 py-3", h.tone === "ask" ? "border-ask-line bg-ask-wash" : "border-line bg-sheet")}
    >
      <h2 className="eyebrow mb-1.5">{R.title}</h2>
      <p className="read text-[15px]">
        {r.status === "decided" && r.origin ? (
          <>
            <Country name={r.origin} />{" "}
          </>
        ) : null}
        {h.text}
      </p>
      <p className="duo mt-1.5 text-[13px] text-body">{sentence(r.reason)}</p>
      {r.basis ? <p className="duo mt-1 text-[12.5px] text-dim">{r.basis}</p> : null}
      <p className="duo mt-1 text-[12.5px] text-dim">
        {rc.applies === "informational" ? `${R.informational} ` : ""}
        {R.note}
      </p>
      <div className="mt-2">
        {r.materials?.length ? (
          <Fold title={R.parts} meta={`${r.materials.length}`}>
            <ul className="duo flex flex-col gap-1 text-[13px]">
              {r.materials.map((m, i) => (
                <li key={i}>
                  <span className="text-ink">{m.name}</span>
                  {m.hts ? <span className="text-dim"> · {m.hts}</span> : null}
                  {m.origin ? <span className="text-dim"> · {m.origin}{m.foreign ? "" : ` (${R.domestic})`}</span> : null}
                  {" · "}
                  {statusWords[m.status] ?? m.status}
                  {m.needs?.length ? <span className="text-dim"> ({m.needs.join("; ")})</span> : null}
                </li>
              ))}
            </ul>
          </Fold>
        ) : null}
        {r.rule ? (
          <Fold title={R.ruleText} meta={r.rule.id}>
            <p className="duo whitespace-pre-line text-[13px] text-body">{r.rule.text}</p>
            <p className="duo mt-1 text-[12px] text-dim">{R.encoded(r.rule.encoded ?? 0, r.rule.alternatives ?? 0)}</p>
          </Fold>
        ) : null}
      </div>
    </section>
  );
}
