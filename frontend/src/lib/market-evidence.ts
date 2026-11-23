// Landing market evidence (#664). The RCEP counts are held-out published
// positives from eval/rcep/japan-summary.json (D691, D716, D717), pinned by
// scripts/landing-market-evidence.test.mjs. Canada and Japan carry no scores.
export const rcepEvidence = {
  rulings: 98,
  yes: 71,
  undetermined: 27,
  criterionMatches: 71,
} as const;

export const marketEvidence = {
  heading: "What each market supports",
  rcepTitle: "RCEP tariff check on 98 unseen Japan Customs rulings",
  rcepYes: "Yes",
  rcepUndetermined: "Undetermined",
  rcepNote: "All 98 rulings approved the preference, so this shows how often the check finds it. It is not an accuracy test.",
  // "Status" is what the product offers, not a validation grade.
  markets: [
    { name: "U.S. origin", status: "Supported check", note: "Measured on held-out rulings." },
    { name: "RCEP tariff", status: "Supported check", note: "Tested on approvals only." },
    { name: "Canada marking", status: "Rules only", note: "No score." },
    { name: "Japan origin", status: "Needs setup", note: "Not available yet. Not measured." },
    { name: "Other markets", status: "Not yet", note: "" },
  ],
  detailsTitle: "Limits",
  details: [
    "No rulings that deny the preference were tested, so refusals are unmeasured.",
    "The calculator applies published rules without a model. The deciding rule matched in 71 of 71 yes answers.",
    "Customs specialist review is pending.",
  ],
  link: "See the full evidence →",
} as const;
