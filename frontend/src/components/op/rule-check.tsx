import type { CaseResponse } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import Link from "next/link";
import { regimeOf } from "@/lib/regime";
import { Country, Fold, cx } from "./ui";

type RuleCheck = NonNullable<CaseResponse["rule_check"]>;

// The 19 CFR 102.20 rule check (D363): a deterministic tariff-shift test from
// the HTS codes on the parts list, shown beside the model's answer. It is
// display only; the verdict above, the route and auto-accept never read it.
// The backend sends it only when the product gives codes, so most cases show
// nothing here. What the screen shows (coordinator review of #544): the full
// panel only when the check decided; a compact, calm line when it did not;
// on a Part 102 case without part codes, a line saying how to run it; on a
// substantial-transformation case without part codes, nothing.
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

export type RuleCheckDisplay =
  | { kind: "none" }
  | { kind: "panel"; rc: RuleCheck }
  | { kind: "line"; text: string; addCodes: boolean; jump?: "plants" | "good" };

function isPart102(c: CaseResponse): boolean {
  if (c.rule_check) return c.rule_check.applies === "part102";
  return c.router?.regime === "usmca_part102" || regimeOf(c)?.key === "usmca";
}

// ruleCheckDisplay picks the display state from the case as the API sends it.
export function ruleCheckDisplay(c: CaseResponse): RuleCheckDisplay {
  const R = copy.ruleCheck;
  const rc = c.rule_check;
  if (!rc) {
    // No check: on a Part 102 case with a parts list, say how to run it.
    const parts = c.product?.components ?? [];
    if (!isPart102(c) || parts.length === 0) return { kind: "none" };
    const missing = parts.filter((p) => !p.hts?.trim()).length;
    const noGood = !c.product?.hts?.trim();
    // Every code given but no check: it is switched off on this server.
    if (missing === 0 && !noGood) return { kind: "none" };
    return { kind: "line", text: R.addCodes(missing, noGood), addCodes: true };
  }
  if (rc.result.status === "decided") return { kind: "panel", rc };
  if (rc.result.status === "out_of_scope") return { kind: "line", text: R.outOfScope, addCodes: false };
  const missing = (rc.result.materials ?? []).filter((m) => m.status === "missing_hts").length;
  // No rule picked although the product has a code: the check stopped for
  // another reason, so the line names that reason (#552). The country of
  // production is not known (no plant, or several plants and no operations),
  // or the product's code does not pick one rule (for example a heading).
  if (!rc.result.rule && c.product?.hts?.trim()) {
    if (!rc.country?.trim()) return { kind: "line", text: R.noCountry, addCodes: false, jump: "plants" };
    return { kind: "line", text: R.goodCode(rc.result.reason), addCodes: false, jump: "good" };
  }
  if (!rc.result.rule || missing > 0) {
    return { kind: "line", text: R.addCodes(missing, !rc.result.rule), addCodes: true };
  }
  return { kind: "line", text: headline(rc).text, addCodes: false };
}

// Where each line's link lands on the product step (it focuses the field).
const jumpTo = {
  parts: { field: "field-part-hts-0", label: copy.ruleCheck.addCodesLink },
  plants: { field: "field-plants", label: copy.ruleCheck.plantsLink },
  good: { field: "field-good-hts", label: copy.ruleCheck.goodCodeLink },
};

export function RuleCheckPanel({ c }: { c: CaseResponse }) {
  const d = ruleCheckDisplay(c);
  if (d.kind === "none") return null;
  const R = copy.ruleCheck;
  if (d.kind === "line") {
    return (
      <p data-rule-check="line" className="duo max-w-[75ch] text-[13px] text-dim">
        <span className="text-body">{R.title}:</span> {d.text}
        {d.addCodes || d.jump ? (
          <>
            {" "}
            <Link href={`/cases/new?from=${encodeURIComponent(c.id ?? "")}#${jumpTo[d.jump ?? "parts"].field}`} className="text-ok underline underline-offset-2">
              {jumpTo[d.jump ?? "parts"].label}
            </Link>
          </>
        ) : null}
      </p>
    );
  }
  const rc = d.rc;
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
        {r.origin ? (
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
