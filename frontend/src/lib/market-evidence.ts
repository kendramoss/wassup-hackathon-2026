// Single source for the RCEP held-out counts shown on the landing and Accuracy
// pages (D691, D716, D717). Pinned to eval/rcep/japan-summary.json by
// scripts/landing-market-evidence.test.mjs and scripts/accuracy-copy.test.mjs.
// Canada, the EU and the UK carry no score. Japan's offline model test (D741) is below.
export const rcepEvidence = {
  rulings: 98,
  yes: 71,
  undetermined: 27,
  refusals: 0,
  errors: 0,
  criterionMatches: 71,
  criteriaCompared: 71,
  syntheticDevelopment: 162,
  syntheticUnseen: 71,
};

const r = rcepEvidence;

export const marketEvidence = {
  heading: "Which markets are tested",
  rcepTitle: `RCEP trade-deal check on ${r.rulings} unseen Japan Customs rulings`,
  rcepYes: "Yes",
  rcepUndetermined: "Undetermined",
  legendNote: "The calculator's answer on whether the product gets the trade-deal rate.",
  rcepNote: `All ${r.rulings} rulings approved the preference, so this shows how often the check finds it. It is not an accuracy test.`,
  marketsLabel: "Market status",
  // Each status says what was shown, not a validation grade.
  markets: [
    { name: "U.S. origin", status: "Held-out rulings", note: "Single-origin U.S. substantial-transformation rulings only." },
    { name: "RCEP tariff", status: "Approvals only", note: "No refusals tested." },
    { name: "Canada marking", status: "Rules only", note: "No score." },
    { name: "EU and UK origin", status: "Rules only", note: "No score." },
    { name: "Japan origin", status: "Open", note: "Tested on new rulings. See Accuracy." },
    { name: "Other markets", status: "Not yet", note: "No rules or tests." },
  ],
  detailsTitle: "Limits",
  details: [
    "No rulings that deny the preference were tested, so refusals are unmeasured.",
    `The calculator applies published rules without a model. The deciding rule matched in ${r.criterionMatches} of ${r.criteriaCompared} yes answers.`,
    "Customs specialist review is pending.",
  ],
  link: "See the full evidence →",
};

// D741 offline Japan model evaluation. Pinned to the two committed D741
// metrics files under eval/jp by scripts/japan-evidence.test.mjs. Experimental,
// positive-only. It is the earlier engine evidence (before D910's rule text
// and D885's evidence), kept on the Accuracy page as a labelled earlier test.
export const japanEvidence = {
  cases: 633,
  criterionMatch: 260,
  otherQualifies: 6,
  undetermined: 363,
  errors: 4,
  matchPct: "41.07%",
  matchInterval: "37.31–44.95",
  recall: 266,
  recallPct: "42.02%",
  recallInterval: "38.24–45.90",
  syntheticRefused: 8,
  syntheticCases: 8,
  syntheticInterval: "67.56–100",
};

// Japan is open on the live site, on the production engine model. D920 and
// D922 measured that model and evidence through the gateway. Pinned to
// eval/jp/*d919-fresh-metrics.json and eval/jp/d922-answered-summary.json by
// scripts/japan-evidence.test.mjs.
export const japanMeasured = {
  unseen: 44,
  unseenRight: 37,
  unseenInterval: "70.6–92.1",
  unseenAnswered: 42,
  unseenAnsweredPct: "95.5%",
  unseenAnsweredInterval: "84.9–98.7",
  dev: 400,
  devRight: 272,
  devAnswered: 374,
  devAnsweredPct: "93.5%",
  devAnsweredInterval: "90.7–95.5",
  negatives: 80,
  negativesApproved: 0,
  euCases: 181,
  euAnswered: 179,
};

// D932 held-out result of the USMCA extended rules (flag USMCA_RULES_EXT, on
// since #910), and D927's reference test of the ATIGA check (ATIGA_CHECK, on
// since #910). Pinned to eval/usmca/summary-held_out.json and
// eval/atiga/reference-cases.json by scripts/japan-evidence.test.mjs.
export const rulesChecks = {
  usmcaProducts: 59,
  usmcaDecided: 23,
  usmcaAgreed: 21,
  usmcaPct: "91.3%",
  usmcaInterval: "73.2–97.6",
  atigaExamples: 7,
  atigaReached: 7,
};
