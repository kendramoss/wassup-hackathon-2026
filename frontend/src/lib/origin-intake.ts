import type { CaseResponse, OriginFactsBody, OriginRules, SuppliedOriginFacts } from "@/lib/api/client";

// The EU and UK origin intake (#770, D800-D802). Pure logic, no React. A
// need is one of three things:
//  - "ask": missing information a control can supply (a row, a yes/no fact,
//    a share, a code or an origin the parts list lacks)
//  - "product": something that belongs on the product itself (where it is
//    made, the parts list): no control here, a link to the product
//  - "person": the rule needs a person's reading; never a control
// Answers are check inputs saved beside the case; the product is unchanged.

export type Need = NonNullable<OriginRules["result"]["needs"]>[number];
export type NeedClass = "ask" | "product" | "person";

export function classify(n: Need): NeedClass {
  if (n.kind === "review") return "person";
  if (n.kind === "row" || n.kind === "fact" || n.kind === "share") return "ask";
  if (n.kind === "code" && (n.key === "good_code" || n.key === "material_hts" || n.key === "material_origin")) return "ask";
  return "product";
}

// The form: what has been typed or picked but not saved yet. Shares are in
// percent, as typed.
export type IntakeForm = {
  row?: string;
  answers: Record<string, boolean>;
  good?: string;
  value: Record<number, string>;
  weight: Record<number, string>;
  hts: Record<number, string>;
  origin: Record<number, string>;
};

export const blankForm = (): IntakeForm => ({ answers: {}, value: {}, weight: {}, hts: {}, origin: {} });

// pct reads a percentage as typed ("70", "70%", "12.5") into a share from 0
// to 1. undefined: blank. NaN: not a percentage from 0 to 100.
export function pct(s: string | undefined): number | undefined {
  const t = (s ?? "").replace(/[%\s]/g, "");
  if (!t) return undefined;
  if (!/^\d+(\.\d+)?$/.test(t)) return NaN;
  const n = Number(t);
  return n >= 0 && n <= 100 ? Math.round(n * 100) / 10000 : NaN;
}

export const HS = /^[0-9]{4}(\.?[0-9]{2}){0,3}$/;

// formErrors names each field typed wrong, by need key and part.
export function formErrors(f: IntakeForm): string[] {
  const out: string[] = [];
  for (const [i, s] of Object.entries(f.value)) if (Number.isNaN(pct(s))) out.push(`value:${i}`);
  for (const [i, s] of Object.entries(f.weight)) if (Number.isNaN(pct(s))) out.push(`weight:${i}`);
  for (const [i, s] of Object.entries(f.hts)) if (s.trim() && !HS.test(s.trim())) out.push(`hts:${i}`);
  if (f.good?.trim() && !HS.test(f.good.trim())) out.push("good");
  return out;
}

// hasInput: the form adds at least one fact.
export function hasInput(f: IntakeForm): boolean {
  return Boolean(
    f.row ||
      Object.keys(f.answers).length ||
      f.good?.trim() ||
      [f.value, f.weight, f.hts, f.origin].some((m) => Object.values(m).some((s) => s.trim() !== "")),
  );
}

// body merges what was saved with what the form adds, for PUT
// /cases/{id}/origin-facts/{market}. Every part fact names the part as it
// stands, so a later parts change leaves it unread rather than misapplied.
export function body(c: CaseResponse, saved: SuppliedOriginFacts | undefined, f: IntakeForm): OriginFactsBody {
  const parts = c.product?.components ?? [];
  const out: OriginFactsBody = {};
  const good = f.good?.trim() || saved?.good_hts;
  if (good) out.good_hts = good;
  const row = f.row ?? saved?.row;
  if (row) out.row = row;
  const answers = { ...(saved?.answers ?? {}), ...f.answers };
  if (Object.keys(answers).length) out.answers = answers;
  const mats = new Map<number, NonNullable<OriginFactsBody["materials"]>[number]>();
  for (const m of saved?.materials ?? []) {
    const p = parts[m.index];
    // A saved fact for a part that changed is dropped here, not resent.
    if (!p || p.name !== m.name || (p.hts ?? "") !== (m.part_hts ?? "")) continue;
    mats.set(m.index, { ...m });
  }
  const touch = (i: number) => {
    const p = parts[i];
    if (!p) return undefined;
    const m = mats.get(i) ?? { index: i, name: p.name, ...(p.hts ? { part_hts: p.hts } : {}) };
    mats.set(i, m);
    return m;
  };
  for (const [k, s] of Object.entries(f.value)) {
    const v = pct(s);
    const m = v !== undefined && !Number.isNaN(v) ? touch(Number(k)) : undefined;
    if (m) m.value_share = v;
  }
  for (const [k, s] of Object.entries(f.weight)) {
    const v = pct(s);
    const m = v !== undefined && !Number.isNaN(v) ? touch(Number(k)) : undefined;
    if (m) m.weight_share = v;
  }
  for (const [k, s] of Object.entries(f.hts)) {
    const m = s.trim() ? touch(Number(k)) : undefined;
    if (m) m.hts = s.trim();
  }
  for (const [k, s] of Object.entries(f.origin)) {
    const m = s.trim() ? touch(Number(k)) : undefined;
    if (m) m.origin = s.trim();
  }
  if (mats.size) out.materials = [...mats.values()].sort((a, b) => a.index - b.index);
  return out;
}

// suppliedCount is how many facts a saved set holds, for a short line.
export function suppliedCount(s: SuppliedOriginFacts | undefined): number {
  if (!s) return 0;
  let n = (s.row ? 1 : 0) + (s.good_hts ? 1 : 0) + Object.keys(s.answers ?? {}).length;
  for (const m of s.materials ?? []) n += [m.hts, m.origin, m.value_share, m.weight_share].filter((x) => x !== undefined && x !== "").length;
  return n;
}
