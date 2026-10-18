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
            "Cases are stored in Google Cloud Firestore. To predict an origin or search for moves, Unde sends the case's product facts to Gemini on Google Cloud Vertex AI. Under Google Cloud's terms, that data isn't used to train Google's models.",
            "The server logs each request's method, path, status and timing, not its contents.",
            "Unde doesn't sell your data or share it with anyone else.",
          ],
        },
        {
          h: "Keeping and deleting it",
          p: [
            "Cases are kept until you delete them, from the Cases page, or until Unde shuts down. Clearing your cookies hides an anonymous visitor's cases from that browser for good.",
            "To have your data removed, or to ask what Unde holds about you, email the address below.",
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
    progress: "Where you are on the page",
    goTo: (fig: string) => `Go to ${fig}`,
    figs: ["fig.1", "fig.2", "fig.3"],
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
  },

  money: "Money · illustrative — editable",
  proxy: "proxy run",
  retry: "Try again",
};
