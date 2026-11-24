import type { FamilyAnswer, FamilyFactsBody, FamilyIntake, FamilyQuestion, SuppliedFamilyFacts } from "@/lib/api/client";

// The family questions (#770, D810-D813). Pure logic, no React. The server
// decides what is asked: a question is "missing" (asked here), "answered"
// (asked, with the saved answer filled in) or "known" (the parts list says
// it; not asked). A not_modelled intake asks nothing and gets no control.

export type FamilyForm = Record<string, string>; // question id -> country, "yes" or "no"

// asked: the questions a control is shown for.
export function asked(fi: FamilyIntake | undefined): FamilyQuestion[] {
  if (!fi || fi.status === "not_modelled") return [];
  return fi.questions.filter((q) => q.state === "missing" || q.state === "answered");
}

// initialForm fills the form from the saved answers the intake read.
export function initialForm(fi: FamilyIntake | undefined): FamilyForm {
  const f: FamilyForm = {};
  for (const q of asked(fi)) {
    const a = q.answer;
    if (!a) continue;
    if (q.kind === "country" && a.country) f[q.id] = a.country;
    if (q.kind === "yes_no" && a.yes !== undefined) f[q.id] = a.yes ? "yes" : "no";
  }
  return f;
}

// A country as typed: letters, spaces and . ' ( ) -, 2 to 60 characters.
export const COUNTRY = /^\p{L}[\p{L} .'()-]{1,59}$/u;

// formErrors names the questions typed wrong.
export function formErrors(fi: FamilyIntake | undefined, f: FamilyForm): string[] {
  return asked(fi)
    .filter((q) => q.kind === "country" && (f[q.id] ?? "").trim() !== "" && !COUNTRY.test((f[q.id] ?? "").trim()))
    .map((q) => q.id);
}

// body is every answer in the form (a save replaces the saved set), with the
// saved answers' time as read for the precondition. Blank fields are left
// out: an answer is never invented.
export function body(fi: FamilyIntake | undefined, f: FamilyForm, saved?: SuppliedFamilyFacts): FamilyFactsBody {
  const answers: Record<string, FamilyAnswer> = {};
  for (const q of asked(fi)) {
    const v = (f[q.id] ?? "").trim();
    if (!v) continue;
    answers[q.id] = q.kind === "country" ? { country: v } : { yes: v === "yes" };
  }
  return { expected_updated_at: saved?.updated_at ?? "", answers };
}

// changed: the form differs from what is saved.
export function changed(fi: FamilyIntake | undefined, f: FamilyForm): boolean {
  const a = initialForm(fi);
  const keys = new Set([...Object.keys(a), ...Object.keys(f)]);
  for (const k of keys) if ((a[k] ?? "").trim() !== (f[k] ?? "").trim()) return true;
  return false;
}

export function answerText(a: FamilyAnswer | undefined, yes = "Yes", no = "No"): string {
  if (!a) return "";
  if (a.country) return a.country;
  return a.yes === undefined ? "" : a.yes ? yes : no;
}
