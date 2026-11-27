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
  // D950: the AKFTA and ACFTA checks (AKFTA_CHECK, ACFTA_CHECK, off) on the
  // authority worked examples, first run. Pinned to the case files by
  // scripts/japan-evidence.test.mjs; the reached counts are enforced by
  // backend/internal/aseanfta/reference_test.go.
  akftaExamples: 8,
  akftaReached: 8,
  acftaExamples: 5,
  acftaReached: 3,
};

// D952, D955 and D957 held-out results of the textile rules check
// (TEXTILE_RULES, off), facts plus suggested subheading. d952 is final and
// final2 pooled; older, pool and pooled are the second test (D955); third is
// the third test (D957). Pinned to eval/textile102/summary.json,
// summary-fresh.json and summary-third.json by scripts/japan-evidence.test.mjs.
export const textileRules = {
  d952: { rulings: 88, inScope: 41, decided: 14, agreed: 13, pct: "92.9%", interval: "68.5–98.7" },
  older: { rulings: 134, inScope: 133, decided: 69, agreed: 31, pct: "44.9%", interval: "33.8–56.6" },
  pool: { rulings: 185, inScope: 125, decided: 53, agreed: 48, pct: "90.6%", interval: "79.7–95.9" },
  pooled: { rulings: 319, inScope: 258, decided: 122, agreed: 79, pct: "64.8%", interval: "55.9–72.7" },
  // D957 third test: 2012-2018 rulings, textile labeller fixed after D956;
  // pinned to eval/textile102/summary-third.json. a and b are its two periods.
  third: {
    rulings: 62, inScope: 58, decided: 27, agreed: 26, pct: "96.3%", interval: "81.7–99.3",
    a: { decided: 13, agreed: 13 }, b: { decided: 14, agreed: 13 },
  },
  // After D956, not part of its registered figure: the same second-test rulings
  // against the labels of the fixed labeller (eval/textile102/relabelled/summary-fresh.json).
  restated: { decided: 118, agreed: 110, pct: "93.2%", interval: "87.2–96.5" },
  // After the result, not part of the registered figure: CBP's holding read
  // for the 43 disagreements (eval/textile102/posthoc-d956.json).
  labelErrors: 34,
  correctedAgreed: 113,
  correctedInterval: "86.6–96.1",
};

// D943 held-out result of the AD/CVD exposure check with the scope reader
// (ADCVD_CHECK and ADCVD_SCOPE_READER, off), the shipped method with the
// pointer-scope fix. Pinned to eval/adcvd/heldout/summary.json by
// scripts/japan-evidence.test.mjs; earlier is D940, pinned to
// eval/adcvd/summary.json.
export const adcvdCheck = {
  rulings: 44,
  within: 18,
  flagged: 8,
  correctFlags: 8,
  flaggedTotal: 14,
  precisionPct: "57.1%",
  precisionInterval: "32.6–78.6",
  recallPct: "44.4%",
  recallInterval: "24.6–66.3",
  plainCorrect: 8,
  plainTotal: 16,
  plainPct: "50.0%",
  baselinePct: "40.9%",
  earlier: {
    rulings: 35,
    within: 12,
    flagged: 11,
    correctFlags: 11,
    flaggedTotal: 21,
    precisionPct: "52.4%",
    precisionInterval: "32.4–71.7",
    recallPct: "91.7%",
    recallInterval: "64.6–98.5",
    baselinePct: "34.3%",
  },
};

// D941 tariff watch test. Pinned to eval/tariffwatch/summary.json by
// scripts/tariffwatch-evidence.test.mjs. Day 2 is 10 Jan 2027.
export const tariffWatch = {
  lines: 10024,
  affected: 121,
  found: 120,
  falseAlarms: 0,
  precisionInterval: "96.9–100.0",
  recallPct: "99.2%",
  recallInterval: "95.5–99.9",
  rateRight: 4,
  rateLines: 4,
};
