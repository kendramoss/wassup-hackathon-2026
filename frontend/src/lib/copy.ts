// Everything Unde says that isn't returned by the API, in one place.
// Wording rules: plans are "precedent-supported" at most, never approved,
// cheapest or guaranteed; follow-up questions make no accuracy claim; money
// is "illustrative — editable" everywhere.
export const copy = {
  name: "Unde Labs",
  tagline: "Predicts the country of origin U.S. Customs would likely assign, citing earlier CBP rulings, and finds production moves that earlier rulings support.",
  nav: { cases: "Cases", accuracy: "How accurate is it?", api: "API" },

  cases: {
    title: "Cases",
    empty: "No cases yet. Cases you start show here, and only you see them.",
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
    // Steps of the analysis as the backend streams them (D71).
    stages: {
      router: "Checking which origin rules apply",
      precedents: "Finding earlier CBP rulings",
      engine: "Weighing the facts against them",
      clarify: "Checking what CBP would ask",
    },
    bomDrop: "Drag and drop your bill of materials",
    bomDropActive: "Release to import",
    bomFormats: "CSV, Excel (.xlsx) or PDF · one row per part",
    bomBrowse: "Browse files",
    bomReplace: "Replace file",
    bomSamples: "Sample dataset (heat gun, 17 rows):",
    bomTemplate: "Download CSV template",
    bomColumnsTitle: "Data requirements",
    bomColumnsNote: "Headers are matched without case and accept the aliases shown. A PDF needs these columns as a table with a header row.",
    bomColumns: [
      { field: "name", required: true, format: "Text. Also: component, part, item, description", example: "Motor-fan assembly" },
      { field: "country", required: true, format: "Country where it is made. Also: made in, origin, COO", example: "China" },
      { field: "plant", required: false, format: "Text. Also: factory, site, supplier. Defaults to the country", example: "Rayong plant" },
      { field: "group", required: false, format: "Operation group from the product family", example: "pcba" },
      { field: "type", required: false, format: "component (default), operation or plant", example: "component" },
      { field: "plant_capabilities", required: false, format: "Groups the plant could also do, separated by ;", example: "pcba; motor_fan_assembly" },
    ],
  },

  result: {
    eyebrow: "Likely origin",
    questions: "CBP would likely ask about this",
    questionsWhy: "Answering adds these facts and runs the analysis again.",
    idk: "I don't know",
    rerun: "Add answers and re-run",
    rerunHint: "Enabled once one answer adds a fact",
    optional: "Answering is optional",
    whyAuto: (c: string) => `Why · auto-accepted: ${c}, ${c === "0.92" ? "at" : "above"} the 0.92 bar`,
    questionsNote: "optional; answers add facts and re-run the analysis",
    nextQuestions: (target: string) => `Add the facts CBP would ask about, or plan a move to ${target}.`,
    suggestAnswer: (n: number) => (n === 1 ? "Answer CBP's likely question" : `Answer CBP's ${n} likely questions`),
    conflict: "Earlier rulings disagree",
    review: {
      title: "Below the 0.92 bar · worth a broker's look",
      body: (c: string) => `Confidence is ${c}. On the 377 held-out rulings, cases under the bar were right 68% of the time, against 96% above it. No follow-up questions came back for this case.`,
      draft: "Draft a note for the broker →",
      addFacts: "Add facts",
    },
    keyFactors: "Key factor",
    rulings: "Earlier CBP rulings read",
    ask: "Ask about this case",
    scenarios: "Try scenarios",
  },

  // Terms a first-time reader may not know (#232), explained on hover,
  // focus or tap at their first place on each screen (components/op/term.tsx).
  terms: {
    pcba: { match: /PCBA/, text: "Printed circuit board assembly: the populated circuit board." },
    bar: {
      match: /0\.92 bar/,
      text: "A plan is shown only when its precedent check is at least 0.92. On every run we scored, none above it was wrong.",
    },
    check: {
      match: /precedent check/i,
      text: "An AI agent searches and reads earlier CBP rulings for one where the same kind of change decided origin.",
    },
    auto: {
      match: /Auto-accepted/,
      text: "Confident enough (0.92 or more) to pass Unde's configured confidence and scope checks. That is not legal approval: it is still a prediction, and a licensed customs broker or counsel should review it before you rely on it. On 377 held-out rulings these were right 96% of the time.",
    },
    hts: { match: /HTS/, text: "The US tariff code. It sets the duty rate." },
    needsInfo: {
      match: /Needs more facts/,
      text: "Under the 0.92 bar, and the facts leave out something CBP would ask about. Answer the questions to re-run it.",
    },
    needsReview: {
      match: /Needs expert review/,
      text: "Under the 0.92 bar: a licensed customs broker should look at it before anyone relies on it.",
    },
  },

  plan: {
    best: "Best plan",
    none: `No plan cleared the 0.92 bar`,
    supported: "Precedent-supported",
    searching: "Searching moves",
    usually: "usually 1–2 minutes; longer if the model provider is busy",
    usuallySentence: "The search usually takes 1–2 minutes; longer if the model provider is busy.",
    slow: "Taking longer than usual. The model provider may be busy; the search keeps going.",
    cut: "The search stopped before it finished. Try again: moves already checked are kept, so it picks up where it left off.",
    draft: "Draft the ruling request →",
    draftClosest: "Draft for the closest move →",
    illustrative: "illustrative — editable",
    illustrativeShort: "illustrative",
    compare: "Compare countries",
    // Evidence heading over the lead move: a move under the bar isn't
    // "supported", it's the closest the check came.
    whySupported: "Why it's supported",
    closestEvidence: "Precedent check: closest move",
    // A greyed row's reason, instead of the API's "precedent-supported but
    // verifier confidence 0.90 is below 0.92" (one name for the check).
    // The precedent check's search path (D80), folded under each checked row.
    trail: {
      title: "How the check found this",
      count: (s: number, r: number) => `${s} ${s === 1 ? "search" : "searches"}, ${r} read`,
      searches: "Searches",
      searched: "Searched",
      returned: "returned",
      nothing: "returned nothing",
      read: "Read",
      notFound: "not found",
    },
    // Near-miss advice (D81) under a row that fell short of the bar.
    nearMiss: {
      label: "Advice · not a check",
      note: "It doesn't change the bar or the verdict.",
      short: "Why it fell short:",
      wouldMatch: "Would match",
      theRuling: "the supporting ruling",
      if: "if:",
    },
    agreement: (views: string, agree: string) => `${views} views, ${agree} agree`,
    // D85: a move the search added because a near miss's advice named it.
    estimate: {
      cached: "Already checked: replays in seconds, no cost.",
    },
    advice: {
      tried: "Tried because the advice suggested it",
      pending: "The move the advice suggested",
    },
    belowBarRow: (check: string) => `Check ${check}, under the 0.92 bar`,
  },

  // Landing page (/). Numbers come only from docs/NUMBERS.md (Official or
  // Showable rows, with the qualifiers those rows require).
  // /privacy and /terms (D80): what the app actually stores and sends, for
  // Google's OAuth consent screen and for visitors. Keep in step with
  // lib/visitor.ts, lib/user.ts and the backend's request log.
  legal: {
    contact: "anasqumhiyeh@gmail.com",
    updated: "18 October 2026",
    footer: { privacy: "Privacy", terms: "Terms" },
    privacy: {
      title: "Privacy policy",
      intro: "Unde is run by Unde Labs. This page says what Unde stores when you use it, where that data goes, and how to remove it.",
      sections: [
        {
          h: "What you enter",
          p: [
            "The product description, bill of materials, dates and money figures you type or import into a case. Unde stores each case so you can come back to it.",
            "Don't enter information you aren't allowed to share with a service provider.",
          ],
        },
        {
          h: "If you sign in with Google",
          p: [
            "Google sends Unde an ID token with your email address and name. Unde uses them to show who is signed in and to keep your cases in your workspace. It doesn't ask Google for anything else and never sees your Google password.",
            "The token sits in an HttpOnly cookie (op_user) until it expires, usually within an hour, or until you sign out.",
          ],
        },
        {
          h: "Cookies",
          p: [
            "op_visitor: a random ID that keeps your cases separate from other visitors' when you aren't signed in. HttpOnly, kept for one year.",
            "op_user: the Google ID token described above, only if you sign in.",
            "Unde uses no analytics, advertising or tracking cookies.",
          ],
        },
        {
          h: "Where your data goes",
          p: [
            "Unde's deployment is intended to store cases in Google Cloud Firestore. To predict an origin or search for moves, Unde sends the case's product facts to Gemini on Google Cloud Vertex AI. How Google handles that data is set by Google Cloud's terms for the service, which Unde does not control.",
            "In the current release, our server logs record the kind of request (the route pattern, such as GET /v1/cases/{id}), its status and how long it took, a request reference, and counts such as model tokens. They are designed not to record your product description, bill of materials, supplier or plant names, costs, questions, answers, case names, email addresses, tokens or API keys. When something fails, the log keeps the kind of failure (for example \"model unavailable\") and a status number, not the text of the error. Model prompts and answers are not written to the logs at any level.",
            "The request reference shown in the X-Request-ID header is generated or derived by the server. If you send your own, it is replaced by a one-way hash, so something you put in that header does not appear in our logs. The hash is not anonymisation.",
            "Unde doesn't sell your data or share it with anyone else.",
          ],
        },
        {
          h: "Keeping and deleting it",
          p: [
            "Cases are kept until you delete them, from the Cases page, or until Unde shuts down. Clearing your cookies hides an anonymous visitor's cases from that browser for good.",
            "To have your data removed, or to ask what Unde holds about you, email the address below.",
            "Your case, its history and what you export are stored and shown to you and your workspace as you entered them, with the same accuracy as before. The history keeps the email of the member who made each change and shows it to the people who can see that case, until the case is deleted. History lines written from now on no longer repeat the text of a question or an old case name; lines written earlier are not rewritten. Case history is deleted with its case. Resetting a case clears its chat but not the earlier lines of its history. Cached predictions and precedent-check results are stored separately and are not removed by deleting the case. We do not control copies kept elsewhere, such as any backups or provider-side records.",
          ],
        },
      ],
    },
    terms: {
      title: "Terms of service",
      intro: "These terms apply when you use Unde, a tool from Unde Labs that predicts the US country of origin CBP would likely assign to a product and searches for production moves that could change it.",
      sections: [
        {
          h: "Not a ruling or legal advice",
          p: [
            "Unde's results are predictions, checked against published CBP rulings. They aren't a binding ruling, legal advice or customs advice. Confirm any decision with a licensed customs broker or by requesting a CBP binding ruling.",
            "Money figures are illustrative and depend on the inputs you give.",
          ],
        },
        {
          h: "Using Unde",
          p: [
            "Use Unde for lawful purposes only. Don't try to get round its rate limits, read other people's cases or overload the service.",
            "You keep the rights to what you enter. You give Unde Labs permission to store and process it to provide the service, as the privacy policy describes.",
          ],
        },
        {
          h: "Availability",
          p: [
            "Unde is a hackathon project, provided as is, with no warranty. It may change, slow down, stop or lose data without notice. Keep your own copy of anything you need.",
            "To the extent the law allows, Unde Labs isn't liable for losses from using Unde or relying on its results.",
          ],
        },
        {
          h: "Changes",
          p: ["These terms may change. The date above shows the latest version; using Unde after a change means you accept it."],
        },
      ],
    },
  },
  landing: {
    eyebrow: "fig.0 — US Customs country of origin",
    title: "Plan the production change before you build it.",
    sub: "See the origin CBP would likely assign, and which production moves would change it, backed by CBP rulings.",
    demo: "Try the heat-gun demo →",
    fig1: "fig.1 — the heat gun",
    fig1Sub: "Kobalt 12.5-amp heat gun · real CBP rulings",
    fig1Label: "fig.1, the heat gun, drawn in 3D. In 2022, assembled in Thailand with the motor-fan assembly, the PCBA and the switch, cord and minor parts from China, CBP held China. Unde's plan makes those three groups in Thailand, precedent-supported at 0.96, above its 0.92 bar. The manufacturer moved the same parts, and the outer shells, and in 2026 CBP held Thailand.",
    steps: [
      { title: "2022 setup", body: "Assembled in Thailand, key parts from China. CBP held China", ruling: "N328954" },
      { title: "Taken apart", body: "Each part, where it is made, and which decide the origin." },
      { title: "The plan", body: "Make the motor-fan assembly, the PCBA and the switch, cord and minor parts in Thailand. Precedent check 0.96, above the 0.92 bar", for: "N345938" },
      { title: "2026 setup", body: "The maker moved the same parts, and the outer shells. CBP held Thailand", ruling: "N357771" },
    ],
    stepOf: (n: number) => `Step ${n} of 4`,
    sheetStates: ["2022 setup · isometric", "exploded view", "planned move", "2026 setup · isometric"],
    planNote: "Unde · plan · check 0.96, above the 0.92 bar",
    planMove: "Motor-fan, PCBA, switch + cord → Thailand",
    legendCn: "Made in China",
    legendTh: "Made in Thailand",
    drag: "Drag to turn",
    schematic: "Schematic from CBP ruling facts, not a manufacturer's drawing",
    fig2: "fig.2 — how it works",
    fig2Sub: "one case, four sheets · the heat gun",
    fig2Now: ["Product: every part and where it is made.", "Origin: the country CBP would likely assign.", "Plan: the move that would change it.", "Draft: the ruling request for a broker."],
    fig2Foot: "The line is the heat gun's facts moving through the case",
    wires: ["facts", "origin", "moves"],
    fig3: "fig.3 — how accurate is it",
    fig3Title: "Test sheet · 377 held-out CBP rulings",
    fig3Meta: "Jan–Sep 2026 · held out of the precedent pool · Vertex AI",
    scaleHead: "A · measured on a scale",
    squaresHead: "B · every ruling, as a square",
    claimAccuracy: "origin accuracy, vs 60.5% for the best simple rule (most-mentioned country).",
    claimAuto: "correct on the 79.6% it auto-accepts; the rest go to expert review.",
    bothWays: "every move is checked both ways, for and against",
    more: "How accurate is it? →",
    cta: "Open the cases →",
    whoTitle: "Who it's for",
    who: "For trade-compliance and industrialisation leads at manufacturers moving production out of China.",
    whoFact: "CBP estimates 20 hours of preparation per ruling request (all ruling subjects). Asian-origin rulings take a median of 21 days (team count, n = 247).",
    footerDisclaimer: "Not a ruling. Confirm with a licensed customs broker or a CBP binding ruling.",
    built: "Built with Gemini on Vertex AI, ADK for Go and Cloud Run.",
    repo: "https://github.com/kendramoss/wassup-hackathon-2026",
  },

  signIn: {
    button: "Sign in with Google",
    signOut: "Sign out",
    expired: "Your sign-in expired. Sign in again.",
    failed: "Sign-in didn't work. Try again.",
    account: (email: string) => `Account: ${email}`,
    signedInAs: "Signed in as",
    // 401 unauthenticated for a visitor with no sign-in (#283, strict mode).
    // A visitor id is not a sign-in, so this says to sign in, not to retry.
    required: "Sign in to open your cases. The demos stay open without signing in.",
    requiredLead: "Sign-in required",
    demosLink: "Open a demo case →",
  },
  access: {
    viewerRefused: "Your role in this workspace is viewer, so you can read this case but not change it. Ask a workspace admin for editor access.",
    recheck: "Check my access again",
    readOnlyNote: "You can read this case but not change it: your role in this workspace is viewer. You can still preview a letter. Editor access is needed to change it.",
  },

  // Broker-review framing (#278): one short note beside the result on the
  // origin, plan, compare, scenarios and draft screens (components/op/
  // review-note.tsx). Local and once per place; no banner, no nav disclaimer.
  // The best-of lines are bounded to the moves searched and the inputs the
  // model uses; they read the same for any plan result and use no API field.
  review: {
    label: "Review and scope",
    origin: "Even a high-confidence result is a prediction from earlier CBP rulings, not a ruling. Have a licensed customs broker review it before you rely on it.",
    plan: "These plans are suggestions, not rulings or CBP approvals. Have a licensed customs broker review a move before you act on it.",
    bestBounded: "Best of the moves searched, using the duty and cost assumptions shown, which may be prefilled or illustrative. In the legacy plan ranking a blank cost may count as zero, which is not a verified expense. Factors the model does not include are not counted.",
    compare: "This ranking is a suggestion for a licensed customs broker to review, not a ruling or a CBP approval.",
    compareBounded: "Ranked among the countries you chose, from the moves searched and the duty and cost inputs shown, which may be prefilled or illustrative. A missing required input leaves that figure unpriced, and factors the model does not include are not assessed.",
    scenarios: "Scenarios are what-ifs, not rulings. Have a licensed customs broker review a result before you act on it.",
    scenariosBounded: "Compared among the scenarios you entered, using the duty and cost inputs shown, which may be prefilled or illustrative. A missing required input leaves that figure unpriced, and factors the model does not include are not assessed.",
  },

  // What the tool is for and not for (#282). Reviewed wording from
  // docs/scope-and-jurisdiction.md; it says what the tool is for and makes no
  // claim that unsupported questions are detected or blocked (they are not).
  jurisdiction: {
    originScope: "U.S. origin only. Not for trade-agreement claims, certificates of origin or non-U.S. rules.",
    outOfScope: "This looks like a case this tool does not decide. A licensed customs broker should review it.",
    part102: "Marking origin under the Part 102 rules. This is not USMCA qualification or regional value content.",
    planScope: "Plans use the general origin test. They do not apply textile, Part 102 or pharmaceutical rules, and only motorised tools and small appliances, LiDAR modules and LED lights are planned.",
    draftNote: "A draft ruling request for a broker to complete and file. It asks CBP a question; it is not a determination.",
    part177Label: "Ruling request letter (19 CFR Part 177)",
    part177Help: "Part 177 is how to ask CBP for a ruling. It is not an origin rule.",
    compareScope: "Compared on U.S. origin only. Section 232 duties are not included.",
    // Accuracy page, "What we don't claim" (listed unsupported in docs/scope-and-jurisdiction.md section 6).
    notCovered: "Not covered: EU Binding Origin Information; USMCA regional value content and preferential-origin claims; certificates of origin; duty drawback; antidumping and countervailing duty questions. Section 232 duties are not included in cost figures.",
    noDetector: "Unde is for U.S. origin questions. It has no automatic check that detects or blocks a question about another country's rules, so do not use it for one.",
  },

  // Operating costs (#281). New keys only; the review and privacy blocks are untouched.
  ops: {
    section: "Operating costs",
    optional: "optional",
    switchLabel: "Include operating costs when ranking moves",
    off: "Off: moves are ranked on duty saved and the group costs above, as before. Nothing about operating costs is sent.",
    removes: "Turning this off removes the operating-cost inputs from this save.",
    scope: "Illustrative and editable. Only the terms you enter are modelled; everything else is left out of both sides of the comparison, not counted as zero.",
    horizon: "Horizon (years)",
    horizonHelp: "Above 0. Needed when you enter a one-time cost (CapEx or retooling). One-time costs are spread evenly over it: no discounting, residual value or tax.",
    maxLead: "Your lead-time limit (days)",
    maxLeadHelp: "Whole days. A move relocates several groups at once; their lead times are assumed to run in parallel, so the move takes the longest of them. This is an assumption, not a schedule.",
    note: "Note",
    noteHelp: "Free text. It changes no result.",
    basis: "Labour and logistics are",
    basisNone: "Not set",
    basisIncluded: "Already inside the group cost",
    basisAdditional: "On top of the group cost",
    basisHelp:
      "Set this once you enter labour or logistics, so nothing is counted twice. Already inside: the group cost above already contains them, so they only break it down and are not added again. On top: they are extra and are added to the group cost.",
    unit: "Unit",
    unitHelp: "Case-sensitive, and no conversion is done: pcs and PCS are different units. Demand and the minimum order must use the same unit.",
    demand: "Annual demand (units)",
    orders: "Orders a year",
    labour: "Labour (USD a year)",
    logistics: "Logistics (USD a year)",
    capex: "CapEx (USD, one time)",
    retooling: "Retooling (USD, one time)",
    lead: "Lead time (days)",
    moq: "Minimum order (units)",
    moqUnit: "Minimum-order unit",
    blankNote: "Leave a field blank if you do not know it. A blank is not entered; 0 means a known zero.",
    oneTimeHere: "One-time cost to set up this plant. Only counted if a move goes here.",
    sunkHere: "This is the plant today. Capital already spent is sunk: it is ignored and never refunded.",
    staleRef: (was: string, now: string) =>
      `Entered for ${was}; this plant is now ${now}. If it is a different plant, Unde treats these figures as not entered and lists them as stale.`,
    clearStale: "Remove this entry",
    needHorizon: "A horizon above 0 is needed because a one-time cost is entered.",
    needBasis: "Choose whether labour and logistics are inside or on top of the group cost.",
    needUnit: "Add the unit these quantities use.",
    needMoqUnit: "Add the unit of the minimum order.",
    serverIssues: "Operating-cost inputs to fix",
    unplaced: "Not tied to one field",
  },
  // Operating-cost results and staleness (#281).
  opsResult: {
    scopeLine: "Modelled terms only, for the moves searched and the inputs you entered. It does not say any setup is the best possible one.",
    notModelledLead: "Not modelled",
    unknownLead: "Unknown",
    detail: "How this figure is built",
    baseline: "Today",
    proposed: "With this move",
    duty: "Duty a year",
    operating: "Operating cost a year",
    amortized: "One-time costs a year (amortized)",
    total: "Total a year",
    net: "Net a year (modelled terms)",
    horizon: (y: number) => `Horizon ${y} ${y === 1 ? "year" : "years"}.`,
    assumptions: "Assumptions",
    unknownTitle: "Unknown inputs",
    sunk: "Ignored as sunk capital (current plant, never refunded)",
    stale: "Entered for a plant that has since changed, so treated as not entered",
    feasibility: "Feasibility",
    noMoney: "No modelled saving",
    rejectedOps: "Left out on the operating inputs you entered",
    rejectedOpsStale: "Left out on operating inputs that have since changed. Run the plan again to see why.",
    noneBand: "No move is both precedent-supported and worth it on the operating inputs you entered.",
  },
  stale: {
    title: "Inputs changed, run the plan again",
    plan: "The money inputs changed after this plan was run, so its amounts are hidden rather than shown as current. The origin result, precedent checks and the draft letter do not use them and stay as they were.",
    targets: "The money inputs changed after this comparison was run, so its amounts are hidden rather than shown as current. Which countries have a supported move, and the draft letter, stay as they were.",
    run: "Run the plan again",
    compare: "Compare again",
    hidden: "Hidden: inputs changed",
  },

  money: "Money · illustrative — editable",
  proxy: "proxy run",
  retry: "Try again",
};
