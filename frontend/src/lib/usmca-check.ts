import type { UsmcaCheckRequest } from "@/lib/api/client";
import { copy } from "@/lib/copy";

// The USMCA origin-rule check form (D740). Pure logic, no React: what the
// person typed stays exactly as typed, a bad value is named rather than fixed,
// and amounts travel as strings so nothing is coerced into a number.

export const MAX_MATERIALS = 20;

export type OriginAnswer = "unknown" | "originating" | "non_originating";
export type Method = "" | "transaction_value" | "net_cost";
export type MaterialRow = { hs: string; origin: OriginAnswer };
export type UsmcaForm = {
  hs: string;
  method: Method;
  tv: string; // transaction value, kept apart so a method switch never reinterprets it
  nc: string; // net cost
  vnm: string;
  materials: MaterialRow[];
  complete: boolean; // never true until the person says so
  simple: boolean; // never true until the person says so
};

export const HS_GOOD = /^[0-9]{4}(\.?[0-9]{2}){0,3}$/;
export const HS_MATERIAL = /^[0-9]{2}(\.?[0-9]{2}){0,4}$/;
// The same limits as the engine: 30 integer digits, 12 fraction digits.
export const AMOUNT = /^(0|[1-9][0-9]{0,29})(\.[0-9]{1,12})?$/;

export const amountOf = (f: UsmcaForm): string => (f.method === "net_cost" ? f.nc : f.method === "transaction_value" ? f.tv : "");

export function blankForm(hs = ""): UsmcaForm {
  return { hs, method: "", tv: "", nc: "", vnm: "", materials: [], complete: false, simple: false };
}

export type FormErrors = { hs?: string; amount?: string; vnm?: string; materials: (string | undefined)[] };
const C = copy.usmcaCheck;

// check reports one message per field holding an invalid value. Blank is not
// invalid: it means unknown, and the server says whether it matters.
export function check(f: UsmcaForm): FormErrors {
  const e: FormErrors = { materials: f.materials.map((m) => (m.hs.trim() && !HS_MATERIAL.test(m.hs.trim()) ? C.badMaterialHs : undefined)) };
  if (!f.hs.trim() || !HS_GOOD.test(f.hs.trim())) e.hs = C.badHs;
  if (f.method && amountOf(f).trim() && !AMOUNT.test(amountOf(f).trim())) e.amount = C.badAmount;
  if (f.method && f.vnm.trim() && !AMOUNT.test(f.vnm.trim())) e.vnm = C.badAmount;
  return e;
}

export function hasErrors(e: FormErrors): boolean {
  return Boolean(e.hs || e.amount || e.vnm || e.materials.some(Boolean));
}

const PROVISIONS = [
  "sets", "accessories_packing", "fungibles", "intermediate_materials", "cost_averaging",
  "de_minimis", "outside_party_operations", "special_valuation", "parts_route",
] as const;

// body is the stateless request for the facts as they stand. A blank amount or
// code is left out (unknown, never zero). The one statement about the unmodeled
// provisions is sent only when it was ticked.
export function body(f: UsmcaForm): UsmcaCheckRequest {
  const profile: NonNullable<UsmcaCheckRequest["profile"]> = { simple_goods_confirmed: f.simple, materials_list_complete: f.complete };
  if (f.simple) for (const p of PROVISIONS) profile[p] = "not_applicable";
  const amount = amountOf(f).trim();
  const vnm = f.vnm.trim();
  const materials = f.materials.map((m) => ({
    ...(m.hs.trim() ? { hs: m.hs.trim() } : {}),
    originating: m.origin,
  }));
  return {
    destination: "US",
    regime: "usmca",
    good_hs: f.hs.trim(),
    ...(f.method ? { rvc_method: f.method } : {}),
    ...(f.method === "transaction_value" && amount ? { transaction_value: amount } : {}),
    ...(f.method === "net_cost" && amount ? { net_cost: amount } : {}),
    ...(f.method && vnm ? { vnm } : {}),
    ...(materials.length ? { materials } : {}),
    profile,
  };
}

// keyOf identifies the facts a result was computed for; null while they are invalid.
export function keyOf(f: UsmcaForm): string | null {
  return hasErrors(check(f)) ? null : JSON.stringify(body(f));
}

const RVC_FIELDS = new Set(["rvc_method", "transaction_value", "net_cost", "vnm"]);

// needsValue is true when the value fields can change the answer: the server
// asked for one, or the person already started on them.
export function needsValue(f: UsmcaForm, res: { rvc?: unknown; missing_facts?: { field: string }[] } | null | undefined): boolean {
  if (f.method || f.tv.trim() || f.nc.trim() || f.vnm.trim()) return true;
  if (res?.rvc) return true;
  return Boolean(res?.missing_facts?.some((m) => RVC_FIELDS.has(m.field)));
}

// factText says a missing fact in plain words, naming the material or the
// amount it is about. Unknown codes fall back to the server's own sentence.
export function factText(m: { code: string; field: string; message: string }): string {
  const text = C.facts[m.code] ?? m.message;
  const mat = /^materials\[(\d+)\]/.exec(m.field);
  if (mat) return `${C.materialLabel(Number(mat[1]) + 1)}: ${text}`;
  const label = C.fieldLabels[m.field];
  return label && ["unknown_value", "zero_denominator", "rvc_undefined"].includes(m.code) ? `${label}: ${text}` : text;
}

// factList is the plain sentences for a list of missing facts, each once.
export function factList(facts: { code: string; field: string; message: string }[] | undefined): string[] {
  return [...new Set((facts ?? []).map(factText))];
}

// sequencer lets only the newest check land: starting one, or editing the
// facts, retires every earlier one, and a retired check's abort signal fires.
export function sequencer() {
  let n = 0;
  let ctl: AbortController | null = null;
  return {
    start(): { id: number; signal: AbortSignal } {
      ctl?.abort();
      ctl = new AbortController();
      return { id: ++n, signal: ctl.signal };
    },
    retire(): void {
      n += 1;
      ctl?.abort();
      ctl = null;
    },
    live: (id: number) => id === n,
  };
}

// exactNote is the exact percentage when it says more than the rounded one:
// a fraction such as 200/3, or a value that differs from the display. A whole
// number already shown as 60.00 needs no second line.
export function exactNote(rvc: { exact?: string; display?: string }): string | null {
  if (!rvc.exact) return null;
  if (rvc.exact.includes("/")) return rvc.exact;
  return Number(rvc.exact) === Number(rvc.display) ? null : rvc.exact;
}

// runner runs one check at a time for a form: a new run or a retire() silences
// every earlier one, so a slow answer never lands on newer facts.
export function runner<T>() {
  const s = sequencer();
  return {
    async run(call: (signal: AbortSignal) => Promise<T>, h: { busy: (b: boolean) => void; done: (r: T) => void; fail: (e: unknown) => void }) {
      const { id, signal } = s.start();
      h.busy(true);
      try {
        const r = await call(signal);
        if (s.live(id)) h.done(r);
      } catch (e) {
        if (s.live(id)) h.fail(e);
      } finally {
        if (s.live(id)) h.busy(false);
      }
    },
    retire: () => s.retire(),
  };
}
