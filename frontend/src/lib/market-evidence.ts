// Single source for the RCEP held-out counts shown on the landing and Accuracy
// pages (D691, D716, D717). Pinned to eval/rcep/japan-summary.json by
// scripts/landing-market-evidence.test.mjs and scripts/accuracy-copy.test.mjs.
// Canada and Japan carry no scores.
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
    { name: "Japan origin", status: "Not available yet", note: "Turns on only when the Japan model is configured. Not measured." },
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
