import type { CaseResponse } from "./api/client";

// Which rules decided a case (D51 textile, D61 USMCA, D62 pharmaceutical).
// Those engines run only with their server flags on; otherwise the router
// sends the case out of scope. Moves aren't modelled under these rules, so
// the plan search is off for them, as for textile.
export type Regime = { key: "textile" | "usmca" | "pharma"; label: string };

const byPrompt: Record<string, Regime> = {
  textile: { key: "textile", label: "Textile rules (19 CFR 102.21)" },
  usmca: { key: "usmca", label: "USMCA rules (19 CFR 102)" },
  pharma: { key: "pharma", label: "Pharmaceutical rules" },
};

export function regimeOf(c: CaseResponse | null | undefined): Regime | null {
  const trace = (c?.prediction as { trace?: { prompt_version?: string } } | undefined)?.trace;
  const pv = trace?.prompt_version ?? (c as { prompt_versions?: { engine?: string } } | undefined)?.prompt_versions?.engine ?? "";
  const fromPrompt = Object.keys(byPrompt).find((k) => pv.startsWith(k));
  if (fromPrompt) return byPrompt[fromPrompt];
  const reason = (c?.triage?.reason ?? "").toLowerCase();
  if (reason.startsWith("out of scope")) return null;
  if (reason.startsWith("textile rules")) return byPrompt.textile;
  if (reason.startsWith("usmca")) return byPrompt.usmca;
  if (reason.startsWith("pharmaceutical rules")) return byPrompt.pharma;
  return null;
}

// "out of scope: textile goods follow …" → the router's own reason.
export function outOfScope(c: CaseResponse | null | undefined): string | null {
  const reason = c?.triage?.reason ?? "";
  return /^out of scope:/i.test(reason) ? reason.replace(/^out of scope:\s*/i, "") : null;
}
