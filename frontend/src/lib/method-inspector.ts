export const kobaltSources = {
  prediction: "eval/results/demo-seed-heatgun-predict-vertex-prod.json",
  plan: "eval/results/plan-heatgun-2022-relaxed-search-litellm.json",
  ask: "eval/results/ask-heatgun-2022-litellm.json",
  fixture: "backend/internal/demos/fixtures/heatgun_2022.json",
};

export const kobalt = {
  setupRuling: { id: "N328954", date: "2022-11-15", label: "15 Nov 2022" },
  cutoff: { date: "2026-01-14", label: "14 Jan 2026" },
  route: "assembly",
  regime: "substantial_transformation",
  origin: "China",
  statedConfidence: 0.85,
  triage: "needs_info",
  citations: ["N348083", "N351922"],
  target: "Thailand",
  movesSearched: 7,
  movesShown: 1,
  supporting: { id: "N345938", date: "2025-03-06", label: "6 Mar 2025" },
  counterRuling: null,
  verifierScore: 0.96,
  planBar: 0.92,
  backtestRuling: { id: "N357771", date: "2026-01-29", label: "29 Jan 2026" },
  askTool: "what_if",
  askChange: "motor-fan assembly made in Thailand",
  askOrigin: "Thailand",
  askStatedConfidence: 0.98,
} as const;

type Field = readonly [name: string, type: string];

type Stage = {
  id: string;
  title: string;
  label: string;
  tone: "model" | "review" | "evidence";
  input: readonly Field[];
  output: readonly Field[];
  guard: string;
  trace: readonly Field[];
  source: keyof typeof kobaltSources;
  recording: string;
};

export const methodStages = [
  {
    id: "router",
    title: "Router",
    label: "Gemini 3.8 Flash",
    tone: "model",
    input: [["facts", "string"]],
    output: [["regime", "string"], ["subfamily", "string"]],
    guard: "Market routing runs first. The U.S. router handles U.S. cases.",
    trace: [["Route", "Assembly"], ["Regime", "Substantial transformation"]],
    source: "prediction",
    recording: "Vertex production snapshot",
  },
  {
    id: "origin",
    title: "Origin engine",
    label: "Gemini 4 Argon + BM25",
    tone: "model",
    input: [["date", "ISO date"], ["facts", "string"], ["exclude_rulings?", "ID[]"]],
    output: [["origin_country", "string"], ["cited_rulings", "ID[]"]],
    guard: "Rulings must predate the case and pass its exclusions. Citations come from the retrieved set.",
    trace: [["Origin", kobalt.origin], ["Cited", kobalt.citations.join(", ")], ["Cutoff", kobalt.cutoff.label]],
    source: "prediction",
    recording: "Vertex production snapshot",
  },
  {
    id: "triage",
    title: "Confidence gate",
    label: "0.92 recorded bar",
    tone: "review",
    input: [["confidence", "0–1"], ["calibrated_confidence?", "0–1"]],
    output: [["route", "auto | needs_review | needs_info"]],
    guard: "The saved run used stated ≥0.92. Today, the main engine auto-accepts in-scope cases at calibrated ≥0.90. Other cases go to review or ask for more facts.",
    trace: [["Stated confidence", kobalt.statedConfidence.toFixed(2)], ["Route", "Needs more facts"], ["Question", "How was the heating element made in Thailand?"]],
    source: "prediction",
    recording: "Vertex production snapshot",
  },
  {
    id: "planner",
    title: "Operation planner",
    label: "Go search",
    tone: "evidence",
    input: [["product", "Product"], ["target", "country"]],
    output: [["plans", "Plan[]"], ["rejected", "Plan[]"], ["stats", "Stats"]],
    guard: "Moves use groups in the product template and target plants that can perform them. Each candidate is re-predicted.",
    trace: [["Target", kobalt.target], ["Searched → shown", kobalt.movesSearched + " → " + kobalt.movesShown], ["Shown move", "Motor-fan, PCBA, switch, cord and minor parts → Thailand"]],
    source: "plan",
    recording: "Gateway replay",
  },
  {
    id: "verifier",
    title: "Dual-sided verifier",
    label: "Gemini 4 Argon",
    tone: "evidence",
    input: [["FactsA / FactsB", "before / after"], ["Target", "country"], ["Before", "cutoff"]],
    output: [["supporting_ruling?", "ID"], ["counter_ruling?", "ID"], ["confidence", "0–1"]],
    guard: "Search both sides. Show a move only when the verdict reaches the target, contains eligible support and scores at least 0.92.",
    trace: [["For", kobalt.supporting.id], ["Against", kobalt.counterRuling ?? "None accepted"], ["Score", kobalt.verifierScore.toFixed(2)]],
    source: "plan",
    recording: "Gateway replay",
  },
  {
    id: "ask",
    title: "Ask agent",
    label: "ADK what-if",
    tone: "model",
    input: [["question", "string"], ["case", "saved case"]],
    output: [["what_ifs", "WhatIf[]"], ["steps", "Step[]"], ["cited_rulings", "ID[]"]],
    guard: "What-ifs keep the case date and run the same routing and review checks. Citations must come from the case or the tools’ returned evidence.",
    trace: [["Tool", kobalt.askTool], ["Change", kobalt.askChange], ["Origin", kobalt.askOrigin], ["Stated confidence", kobalt.askStatedConfidence.toFixed(2)]],
    source: "ask",
    recording: "Earlier gateway replay",
  },
] as const satisfies readonly Stage[];

export const methodExperiments = [
  {
    title: "Tariff shifts from model memory",
    failed: "The Part 102 model invented a compressor exception. Quoting the rule text still failed the adoption check.",
    fix: "Go checks the encoded rule separately. Certain conflicts get a review flag.",
    failedDecision: "D551",
    fixDecision: "D591",
  },
  {
    title: "One-sided precedent search",
    failed: "The early verifier could support a move without a targeted search for counterexamples.",
    fix: "Search for support and counter-rulings. Tie the score to eligible evidence.",
    failedDecision: "D74",
    fixDecision: "D74",
  },
  {
    title: "Moves limited to two groups",
    failed: "The early planner never tried the three-group Kobalt move.",
    fix: "Try eligible triples alongside singles and pairs.",
    failedDecision: "D84",
    fixDecision: "D84",
  },
] as const;
