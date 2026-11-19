// How an Ask what-if card reads (#319). The server saves the regime that
// answered, the review route and, when the result is not regime-confirmed, a
// provisional note. The auto-accept ruler is only honest for a result whose
// regime is known, whose route is auto and which carries no provisional note;
// everything else says Needs review or Provisional and gives its reason. A
// what-if with none of these fields (an old cached turn) is unknown, not auto.

export type WhatIfFields = {
  origin?: string;
  confidence?: number;
  regime?: string;
  specialist?: string;
  scope?: string;
  route?: string;
  triage_reason?: string;
  provisional?: string;
};

export type WhatIfView = {
  // confirmed: regime known, route auto, nothing provisional.
  // provisional: a provisional note is present (the answer is a stand-in).
  // review: everything else that is not confirmed.
  kind: "confirmed" | "review" | "provisional";
  showRuler: boolean;
  regimeLabel: string;
  headline: string; // "", "Needs review", "Needs more facts" or "Provisional answer"
  note: string; // why, in words
  originLabel: string; // "Provisional origin" or ""
  scenarioWarning: string; // beside "Add as a scenario"; "" when confirmed
};

const REGIME_LABEL: Record<string, string> = {
  textile_102_21: "Textile rules (19 CFR 102.21)",
  usmca_part102: "Part 102 marking rules (19 CFR 102.11)",
  pharma_api: "Pharmaceutical rules (active ingredient)",
  substantial_transformation: "Substantial transformation",
};
const UNCONFIRMED = "Regime not confirmed";

export function regimeWords(regime: string | undefined): string {
  return (regime && REGIME_LABEL[regime]) || UNCONFIRMED;
}

// Raw regime identifiers inside server text become words.
function inWords(s: string): string {
  return s
    .replace(/textile_102_21/g, "textile rules (19 CFR 102.21)")
    .replace(/usmca_part102/g, "Part 102 marking rules (19 CFR 102.11)")
    .replace(/pharma_api/g, "pharmaceutical rules")
    .replace(/substantial_transformation/g, "substantial transformation");
}

const SCENARIO_WARNING =
  "This result is not regime-confirmed. A scenario saved from it is re-run through the full regime policy and may come out differently or need review.";

export function whatIfView(w: WhatIfFields): WhatIfView {
  const known = !!w.regime && w.regime in REGIME_LABEL;
  const regimeLabel = regimeWords(w.regime);
  const provisional = (w.provisional ?? "").trim();
  const route = w.route;
  const scopeOk = !w.scope || w.scope === "confirmed";
  const reason = inWords((w.triage_reason ?? "").trim());

  if (provisional) {
    return {
      kind: "provisional",
      showRuler: false,
      regimeLabel,
      headline: "Provisional answer",
      note: [inWords(provisional).replace(/^./, (c) => c.toUpperCase()) + ".", route === "needs_review" || route === "needs_info" ? reason && reason + "." : ""].filter(Boolean).join(" "),
      originLabel: "Provisional origin",
      scenarioWarning: SCENARIO_WARNING,
    };
  }
  if (known && scopeOk && route === "auto") {
    return { kind: "confirmed", showRuler: true, regimeLabel, headline: "", note: "", originLabel: "", scenarioWarning: "" };
  }
  const headline = route === "needs_info" ? "Needs more facts" : "Needs review";
  let note = reason ? reason + "." : "";
  if (!known) note = (note + " The regime behind this answer was not recorded or is not confirmed.").trim();
  else if (!scopeOk) note = (note + " The legal regime for this case is not confirmed.").trim();
  else if (!route) note = (note + " No review route was recorded for this answer.").trim();
  return { kind: "review", showRuler: false, regimeLabel, headline, note, originLabel: "", scenarioWarning: SCENARIO_WARNING };
}
