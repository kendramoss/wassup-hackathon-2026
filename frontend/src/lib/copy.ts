// Everything Origin Proof says that isn't returned by the API, in one place.
// Wording rules: plans are "precedent-supported" at most, never approved,
// cheapest or guaranteed; follow-up questions make no accuracy claim; money
// is "illustrative — editable" everywhere.
export const copy = {
  name: "Origin Proof",
  tagline: "Predicts the country of origin U.S. Customs would likely assign, citing earlier CBP rulings, and finds production moves that earlier rulings support.",
  disclaimer: "Not a ruling. Confirm with a licensed customs broker.",
  nav: { cases: "Cases", accuracy: "How accurate is it?" },

  cases: {
    title: "Cases",
    empty: "No cases yet. Start one, or load a demo with real CBP facts.",
    newCase: "+ New case",
    recheckAll: "Re-check all",
    demos: "Demos · real CBP facts",
    columns: ["Product", "Likely origin", "Status", "Ruling watch", "Scenarios", "Where you left off"],
    notAnalysed: "Not analysed yet",
  },

  product: {
    title: "Describe your product",
    lede: "Where each part is made and what each plant can do. Or paste what you would tell CBP.",
    tabs: { parts: "Parts and plants", paste: "Paste a description", bom: "Import BOM" },
    name: "Product name",
    family: "Product family",
    date: "Case date",
    dateHelp: "Only CBP rulings dated before this date are used.",
    facts: "What you would tell CBP",
    factsHelp: "Components, where each is made, and the processing in each country, as you would write to CBP.",
    go: "Find the likely origin",
    working: "Finding the likely origin…",
    bomDrop: "Drop a bill of materials (CSV, Excel or PDF), or choose a file",
    bomGuide: "Columns: name and country (required); plant, group, type, plant_capabilities (optional). A PDF needs those as a table with a header row.",
    bomSample: "Download sample (heat gun)",
  },

  result: {
    eyebrow: "Likely origin",
    questions: "CBP would likely ask about this",
    questionsWhy: "Answering adds these facts and runs the analysis again.",
    idk: "I don't know",
    rerun: "Add answers and re-run",
    rerunHint: "Enabled once one answer adds a fact",
    optional: "Answering is optional",
    conflict: "Earlier rulings disagree",
    keyFactors: "Key factor",
    rulings: "Earlier CBP rulings read",
    ask: "Ask about this case",
    scenarios: "Try scenarios",
  },

  plan: {
    best: "Best plan",
    none: `No plan cleared the 0.92 bar`,
    supported: "Precedent-supported",
    searching: "Searching moves",
    usually: "can take a minute or two",
    draft: "Draft the ruling request →",
    draftClosest: "Draft for the closest move →",
    illustrative: "illustrative — editable",
    compare: "Compare countries",
  },

  money: "Money · illustrative — editable",
  proxy: "proxy run",
  retry: "Try again",
};
