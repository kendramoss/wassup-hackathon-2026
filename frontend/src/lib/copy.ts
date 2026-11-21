// Everything Unde says that isn't returned by the API, in one place.
// Wording rules: plans are "precedent-supported" at most, never approved,
// cheapest or guaranteed; follow-up questions make no accuracy claim; money
// is "illustrative — editable" everywhere.
import { conf, type OriginConf } from "./format";

export const copy = {
  // Where calibrated values appear (judge panel round 3, D270). The map is
  // backend/internal/calibrate/map.json (cal-v2, D160): its lowest step is 0.60
  // from stated 0.65 (a value below the lowest step takes it), the next starts
  // at stated 0.90, and 0.98 or more maps to 0.99.
  calSteps:
    "Calibrated values come in steps: the map gives one value to each band of stated confidence (every stated confidence under 0.90 becomes 0.60, and 0.98 or more becomes 0.99), so many cases share the same value.",
  name: "Unde Labs",
  tagline: "Predicts the country of origin U.S. Customs would likely assign, citing earlier CBP rulings, and finds production moves that earlier rulings support.",
  nav: { cases: "Cases", accuracy: "How accurate is it?", accuracyShort: "Accuracy", method: "Method", api: "API · MCP" },

  cases: {
    title: "Cases",
    // A demo's case date sits just before CBP's later ruling: a backtest.
    demoDate: (d: string) => `Case dated ${d}: Unde read only CBP rulings from before then.`,
    empty: "No cases yet. Cases you start show here, and only you see them.",
    newCase: "+ New case",
    recheckAll: "Re-check all",
    demos: "Demos · real CBP facts",
    demosNote: "Demos were picked because CBP ruled on the same product, so you can compare the answer with CBP's. They show how it works, not how often it is right.",
    demosMisses: "Every held-out ruling it got wrong →",
    moreDemos: "More demos · not run on this site yet",
    moreDemosNote: "These cards have no pinned case on this site. Loading one starts a new case from CBP's facts, and the origin and plan run live, so the result is new and may differ from any recorded run.",
    // A demo's recorded development run (D191), on its card.
    recorded: {
      label: "Development run · not run on this site",
      recordLink: "View the committed run (JSON) →",
      unseeded: "No case here · starts a live run",
      engine: "Engine:",
      agrees: "Agrees with CBP",
      disagrees: "Disagrees with CBP",
      plan: (plan: string, check: string, ruling: string) => `First plan shown: ${plan}, check ${check}${ruling ? ` on ${ruling}` : ""}.`,
      includes: (items: string[]) => `What moves: ${items.join("; ")}.`,
      noPlan: "No plan shown at the bar.",
      // On a card that starts a live run: the run will search plans, nothing
      // is shown yet (judge panel round 3: "Plan included" read as ready).
      planRuns: "Runs a plan search too",
      // A pinned demo read from CBP's text alone has no parts list to plan.
      originOnly: "Origin only: CBP's text, no parts list to plan",
      selection:
        "The four newest demos were picked by family, not by result: the first per-family check for vehicle parts, PCBA devices, power supplies and medical devices. Across all 23 per-family checks the engine disagreed with CBP's holding on 7.",
    },
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
    bomFormats: "CSV, Excel (.xlsx), PDF, or a photo or scan (PNG, JPEG, WebP)",
    bomMediaHelp: "Photos and scanned PDFs are read by AI into a draft you check in Parts and plants. Up to 4 files, 8 MB together, 10 PDF pages.",
    bomPhoto: "Take a photo",
    bomReading: "Reading the document…",
    bomBrowse: "Browse files",
    bomReplace: "Replace file",
    bomSamples: "Sample dataset (heat gun, 17 rows):",
    bomSamplesLoad: "Click one to load it here.",
    bomSamplesDownload: "Download the CSV",
    bomSamplePhoto: "Photo (synthetic)",
    bomSamplePhotoNote: "a synthetic, tilted rendering of the same table, to try photo import",
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
    whyAuto: (o: OriginConf) =>
      `Why · auto-accepted: ${o.calibrated ? "calibrated " : ""}${conf(o.value)}, ${conf(o.value) === o.bar.toFixed(2) ? "at" : "above"} the ${o.bar.toFixed(2)} auto-accept bar`,
    questionsNote: "optional; answers add facts and re-run the analysis",
    nextQuestions: (target: string) => `Add the facts CBP would ask about, or plan a move to ${target}.`,
    suggestAnswer: (n: number) => (n === 1 ? "Answer CBP's likely question" : `Answer CBP's ${n} likely questions`),
    conflict: "Earlier rulings disagree",
    review: {
      title: "Below the auto-accept bar · worth a broker's look",
      body: (o: OriginConf) =>
        `${o.calibrated ? `Calibrated confidence is ${conf(o.value)} (the model stated ${conf(o.stated)})` : `Stated confidence is ${conf(o.value)}`}. On the 377 held-out rulings, cases under the bar were right 68% of the time, against 96% above it. No follow-up questions came back for this case.`,
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
      match: /precedent-check bar/,
      text: "A plan is shown only when its precedent check is at least 0.92. This bar is for the precedent check on a move. It is separate from the origin auto-accept bar (calibrated 0.90). In the historical D86 proxy pool, all 182 shown verdicts at or above it were right; later post-cutoff gateway scoring was right 9 of 9 with the starting origin and 5 of 6 without. These are proxy and gateway runs, not a guarantee.",
    },
    check: {
      match: /precedent check/i,
      text: "An AI agent searches and reads earlier CBP rulings for one where the same kind of change decided origin.",
    },
    calibrated: {
      match: /calibrated/,
      text: "The chance this origin is right, from a calibration map fitted on 463 development rulings. The model's own confidence runs high: on 377 held-out rulings it averaged 94.7% against 90.2% right; calibrated, 90.2%. Auto-accept needs 0.90 or more. Calibrated values come in steps, so many cases share one.",
    },
    stated: {
      match: /^confidence \d/,
      text: "The model's own confidence, not calibrated. The calibration map covers the general engine answered by Gemini 4 Argon; this prediction came from another engine or model, which has no map. Stated confidence runs high on average. Where a stated bar applies, auto-accept needs 0.92 or more.",
    },
    auto: {
      match: /Auto-accepted/,
      text: "Confident enough (calibrated 0.90 or more, the same cases as a stated 0.92) to pass Unde's configured confidence and scope checks. That is not legal approval: it is still a prediction, and a licensed customs broker or counsel should review it before you rely on it. On 377 held-out rulings these were right 96% of the time.",
    },
    hts: { match: /HTS/, text: "The US tariff code. It sets the duty rate." },
    needsInfo: {
      match: /Needs more facts/,
      text: "Under the auto-accept bar, and the facts leave out something CBP would ask about. Answer the questions to re-run it.",
    },
    needsReview: {
      match: /Needs expert review/,
      text: "Under the auto-accept bar: a licensed customs broker should look at it before anyone relies on it.",
    },
  },

  plan: {
    best: "Best plan",
    none: `No plan cleared the 0.92 precedent-check bar`,
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
    belowBarRow: (check: string) => `Check ${check}, under the 0.92 precedent-check bar`,
  },

  // Landing page (/). Numbers come only from docs/NUMBERS.md (Official or
  // Showable rows, with the qualifiers those rows require).
  // /privacy and /terms (D80): what the app actually stores and sends, for
  // Google's OAuth consent screen and for visitors. Keep in step with
  // lib/visitor.ts, lib/user.ts and the backend's request log.
  // /method, "How it's built and tested" (review follow-up).
  method: { footer: "How it's built and tested" },

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
  // One sentence on what is new (judge panel round 4), said the same on the
  // landing and on /method. Factual: what the product does, no claim of
  // being first.
  novel: {
    lead: "What is new here:",
    text: "Unde does not stop at classifying origin. It searches which production changes would change it, and shows a move only when an earlier CBP ruling supports it at the 0.92 precedent-check bar.",
  },
  landing: {
    // Phone-only folds (D270): the summary says what is inside.
    foldRoles: (roles: string[]) => `Show who uses it, pays for it and benefits (${roles.length} roles)`,
    foldStory: (beats: string[]) => `Read the story in ${beats.length} steps: ${beats.map((t) => t[0].toLowerCase() + t.slice(1)).join(", ")}`,
    eyebrow: "fig.0 — US Customs country of origin",
    title: "Plan the production change before you build it.",
    sub: "See the origin CBP would likely assign, and which production moves would change it, backed by CBP rulings.",
    demo: "Try the heat-gun demo →",
    fig1: "fig.1 — the heat gun",
    fig1Sub: "Kobalt 12.5-amp heat gun · real CBP rulings",
    fig1Label: "fig.1, the heat gun, drawn in 3D. In 2022, assembled in Thailand with the motor-fan assembly, the PCBA and the switch, cord and minor parts from China, CBP held China. A recorded 18 Oct production run made those three groups in Thailand with a 0.96 precedent check, above its 0.92 bar (docs/DEMO.md, section 6). The pinned demo on this site shows the same plan at 0.96. In its real 2026 change the manufacturer made the motor, fan, PCBA, knob, power cord and back cover in Thailand and kept the switch, wires and fasteners in China, and CBP held Thailand.",
    steps: [
      { title: "2022 setup", body: "Assembled in Thailand, key parts from China. CBP held China", ruling: "N328954" },
      { title: "Taken apart", body: "Each part, where it is made, and which decide the origin." },
      { title: "The plan", body: "Make the motor-fan assembly, the PCBA and the switch, cord and minor parts in Thailand. Recorded 18 Oct run: check 0.96, above the 0.92 bar; the pinned demo shows the same.", for: "N345938" },
      { title: "2026 setup", body: "The maker moved the motor, fan, PCBA, knob, cord and back cover, and kept the switch, wires and fasteners in China. CBP held Thailand", ruling: "N357771" },
    ],
    stepOf: (n: number) => `Step ${n} of 4`,
    sheetStates: ["2022 setup · isometric", "exploded view", "planned move", "2026 setup · isometric"],
    planNote: "Unde · recorded 18 Oct plan · check 0.96, above the 0.92 precedent-check bar",
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
    fig3Meta: "Jan–Sep 2026 · never used to tune it · scored once · Vertex AI",
    scaleHead: "A · measured on a scale",
    squaresHead: "B · every ruling, as a square",
    claimAccuracy: "origin accuracy, vs 63.4% for the best simple rule we recomputed (the first country named).",
    claimAuto: "correct on the 79.6% it auto-accepts; the rest go to expert review.",
    // fig.3 row C (judge panel round 4): docs/NUMBERS.md, "Precedent check,
    // D170 + addendum (18 pairs), starting origin given", served as
    // planner_post_combined_known-a_* by GET /v1/eval/summary.
    planCheck: {
      head: "C · the plan check, on later rulings",
      meta: "18 CBP flip pairs from Sep–Nov 2026 · each frozen, scored once · gateway, not Vertex AI",
      lead: "Every move is checked both ways, for and against, against earlier CBP rulings only. Tested on pairs of CBP rulings on one product before and after a change, at least one of them published after the 377 above, with CBP's starting origin given.",
      rows: [
        { k: 9, n: 9, label: "moves shown were real changes", lo: 70, hi: 100, lowerIsBetter: false },
        { k: 9, n: 12, label: "later real changes found, one run, starting origin given (75%)", lo: 47, hi: 91, lowerIsBetter: false },
        { k: 0, n: 6, label: "no-change pairs shown as a plan", lo: 0, hi: 39, lowerIsBetter: true },
      ],
      foot: "Small samples, so wide intervals. Without the starting origin: 5 of 6 shown were right, 5 of 12 found. The accuracy page also says about 29%: that is a stricter count on earlier pairs, where a pair counts only if it cleared the bar in every one of several runs, with and without the starting origin (17 of 59).",
      link: "Plan checks on the accuracy page →",
    },
    more: "How accurate is it? →",
    cta: "Open the cases →",
    whoTitle: "Who it's for",
    who: "For trade-compliance and industrialisation leads at manufacturers moving production out of China.",
    whoFact: "CBP's own figure is 3,628 ruling responses a year from its regional rulings office (NCSD), across all ruling subjects, not only origin.",
    // fig.4, the impact story (review follow-up). Every figure is a row in
    // docs/NUMBERS.md ("Impact story"); the firm is a composite, not a customer.
    story: {
      fig: "fig.4 — one decision, start to finish",
      title: "Does making the sub-assemblies in Thailand make the heat gun Thai?",
      label: "Illustrative example — composite, not a customer",
      beats: {
        decision: "The decision",
        oldWay: "The old way",
        withUnde: "With Unde",
        outcome: "The outcome, in duty",
        broker: "What still needs a broker",
      },
      decision: [
        "A contract manufacturer in Thailand assembles heat guns for a U.S. tool brand, $2M of them a year (an illustrative figure). The housing and the heating element are made in Thailand. The motor-fan assembly, the circuit board (PCBA) and the switch, cord and minor parts come from China.",
        "The brand asks whether making those parts in Thailand would make the heat gun Thai for U.S. duty. Moving production costs money, and if the heat gun is still Chinese afterwards, that money buys nothing.",
      ],
      oldWayLead:
        "Today the answer comes from a ruling request and an adviser. Unde gives a first, non-binding answer; the binding one still comes from CBP. Each bar pair is drawn to its own scale.",
      keyBefore: "Today",
      keyAfter: "With Unde",
      rows: [
        {
          label: "Preparing the question",
          before: "20 hours",
          beforeNote: "per ruling request, CBP's estimate (all ruling subjects)",
          after: "about a minute",
          afterNote: "estimated, not measured: origin, plan and draft for this case",
          ratio: 1 / 1200,
        },
        {
          label: "Waiting for an answer",
          before: "21 days",
          beforeNote: "median for CBP's binding ruling, Asian-origin rulings (team count, n = 247)",
          after: "4.6 s",
          afterNote: "median for a first, non-binding origin prediction (p95 5.7 s), measured on 377 held-out rulings",
          ratio: 4.6 / (21 * 24 * 3600),
        },
        {
          label: "Paying for an answer",
          before: "about US$3,600",
          beforeNote:
            "median paid for U.S. origin advice by Korean exporters with subsidised KOTRA vouchers (23 purchases, 2024–26, team-compiled), the only public price we found",
          after: "$0.0078",
          afterNote: "model cost per origin prediction (promo price), not a broker's fee; a plan runs several",
          ratio: 0.0078 / 3600,
        },
      ],
      minBar: "Bars shorter than 2 px are drawn at 2 px.",
      oldWayRisk: "And the costly mistake stays possible: move production, then learn the product is still Chinese.",
      withUnde: [
        "The manufacturer uploads its parts list. Unde names the origin CBP would likely assign and the earlier rulings it read. For this setup, CBP itself held China.",
        "It then tries moving groups of work to the Thai plant. The plan it shows makes the motor-fan assembly, the PCBA and the switch, cord and minor parts in Thailand. An earlier CBP ruling backs that move, and no ruling against it was found.",
        "It writes the ruling request as a draft for the manufacturer's customs broker.",
      ],
      withUndeRuling: "Supporting ruling:",
      outcomeLead: "On $2M of heat guns a year (illustrative input). Rates are for this product; the classification is to be confirmed by a broker.",
      duty: [
        { country: "China" as const, rate: "≈41.7%", sum: "$2,000,000 × 41.7%", value: "$834,000 a year", share: 0.417 },
        { country: "Thailand" as const, rate: "≈16.7%", sum: "$2,000,000 × 16.7%", value: "$334,000 a year", share: 0.167 },
      ],
      dutyGap: "About 25 duty points: $500,000 a year less duty.",
      dutyCost: "Making the parts in Thailand costs more in this example: $215,000 a year (illustrative costs).",
      dutyNet: "Net about $285,000 a year.",
      dutyHead: "duty a year on $2M of imports",
      broker: [
        "Confirm the tariff classification. It is inferred here.",
        "Check the facts against the real plants and suppliers.",
        "File the ruling request. Only CBP's ruling binds, and only for that request.",
        "Re-check the duty rates before moving anything; they change.",
      ],
      sources: "Sources and arithmetic: docs/NUMBERS.md and docs/DUTY-SOURCES.md.",
    },
    // "What you can do" (judge journey): every capability that is built and
    // running, each with a way in. Plain list, no numbers beyond NUMBERS.md
    // rows (14 families, 11 MCP tools).
    also: "Also built: a parts list read from a photo or scan, plan templates for 14 product families, and an MCP server for Gemini CLI, ADK agents and Claude.",
    alsoLink: "What you can do ↓",
    can: {
      title: "What you can do",
      lead: "Each of these is built and running on this site. Open one to try it.",
      items: [
        {
          key: "origin",
          what: "Find the likely origin",
          body: "Paste the facts you would send CBP, or list each part and where it is made. Gemini reads them against the most similar earlier CBP rulings and names the origin. When it is unsure, it asks what CBP would ask instead of guessing.",
          link: "Open the heat-gun result →",
        },
        {
          key: "import",
          what: "Import a parts list",
          body: "A CSV, Excel or PDF bill of materials, or a photo or scan of one. Gemini reads a photo or scan into a draft table that you check before anything runs.",
          link: "Start a case →",
        },
        {
          key: "plan",
          what: "Plan a production move",
          body: "For products in 14 families with a plan template, from heat guns and LiDAR to cables, chargers, pumps, hand tools and gas grills, it tries moving each group of work to another plant. An agent then checks each move against earlier rulings, for and against.",
          link: "See the heat-gun plan →",
        },
        {
          key: "compare",
          what: "Compare countries",
          body: "One plan search per country, ranked side by side, with the duty and cost inputs shown.",
          link: "See a recorded comparison →",
        },
        {
          key: "draft",
          what: "Draft for a broker",
          body: "A ruling request under 19 CFR Part 177, the regulation for asking CBP for a ruling, ready for a licensed customs broker to check and file.",
          link: "See the draft →",
        },
        {
          key: "mcp",
          what: "Use it from Gemini CLI, ADK agents or Claude",
          body: "The same tools over MCP (Model Context Protocol), the open protocol agents use to call other services. In Gemini CLI, one line adds it:",
          code: "gemini mcp add --transport http unde https://undelabs.dev/mcp",
          link: "Setup for Gemini CLI, Google ADK, Claude Code and Claude Desktop, and the 13 tools →",
        },
        {
          key: "method",
          what: "Check how it was tested",
          body: "No model is trained. The page says what Gemini sees, how every setting was chosen on development rulings only, how the test sets were kept clean, and what we tried that did not work.",
          link: "How it's built and tested →",
        },
        {
          key: "misses",
          what: "See where it fails",
          body: "Every held-out ruling it got wrong is listed by name, beside the commands that recompute the headline counts on the page from the repository.",
          link: "Known failure cases →",
        },
      ],
    },
    footerDisclaimer: "Not a ruling. Confirm with a licensed customs broker or a CBP binding ruling.",
    built: "Built with Gemini on Vertex AI, ADK for Go and Cloud Run.",
    roadmap: "Roadmap",
    repo: "https://github.com/kendramoss/wassup-hackathon-2026",
  },

  // The landing's demo tour (judge panel round 4, components/op/demo-tour.tsx).
  tour: {
    label: "Heat-gun tour",
    steps: ["The plan", "Backtest vs CBP's later ruling", "The broker draft"] as const,
    next: (step: string) => `Next: ${step[0].toLowerCase()}${step.slice(1)} →`,
    origin: "See the origin step first",
    // With the tour on, the case's own "Step 3 of 4" gives way to the tour's
    // count, so there is one counter on screen (judge panel round 5).
    stepOf: (n: number) => `Tour step ${n} of 3`,
    outside: "Outside the tour",
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
    bestBounded: "Best of the moves searched, using the duty and cost assumptions shown, which may be prefilled or illustrative. When you leave a cost blank, the ranking may count it as zero, which is not a verified expense. Factors the model does not include are not counted.",
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
    planScope: "Plans use the general origin test. They do not apply textile, Part 102 or pharmaceutical rules, and only products in a family with a plan template (14 families, listed in the family picker) are planned.",
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
      `Entered for ${was}; this plant is now ${now}. The country label or name changed; the server checks this on save and run, and lists the entry as stale only if it is a different plant.`,
    clearStale: "Remove this entry",
    removeGroup: "Remove this group's inputs",
    removedTitle: "Saved inputs for removed items",
    removedHelp: "These operating-cost inputs name a group or plant the product no longer has. They are kept until you remove them; a save is refused while they remain.",
    confirmOff: "Turn off operating costs and discard the inputs you entered? This cannot be undone before you save.",
    discard: "Discard inputs",
    keep: "Keep them",
    needHorizon: "A horizon above 0 is needed because a one-time cost is entered.",
    needBasis: "Choose whether labour and logistics are inside or on top of the group cost.",
    needUnit: "Add the unit these quantities use.",
    needMoqUnit: "Add the unit of the minimum order.",
    noAnnual: "No annual import value is set, so money figures will be unpriced and only the feasibility limits can be checked. What you save here replaces the stored import value, duty rates and group costs with what this form shows.",
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
    noneBand: "Inputs changed. Run the plan again to see which moves clear your operating inputs.",
    compare: "Compare again",
    hidden: "Hidden: inputs changed",
  },

  money: "Money · illustrative — editable",
  proxy: "proxy run",
  retry: "Try again",

  // Who uses Unde and who benefits (landing, after "Who it's for"; README).
  // Roles the product is designed for, not customers: there are none yet.
  // System properties only, so plain text and no numbers (the numbers are in
  // fig.4, each a row in docs/NUMBERS.md).
  stakeholders: {
    title: "Who uses it, and who benefits",
    lead: "Five roles meet in one production-move decision. Unde is designed around them. It has no customers or pilots yet, and none of these roles is a named firm.",
    roles: [
      {
        role: "Uses it",
        who: "The trade-compliance or industrialisation lead at an ASEAN contract manufacturer.",
        gets: "The origin CBP would likely assign, with the earlier rulings behind it, before work moves. Moves that earlier rulings support, and a draft ruling request.",
      },
      {
        role: "Pays for it",
        who: "The COO or VP of supply chain at that manufacturer, or the U.S. brand that sources from it.",
        gets: "A check before capital moves: the duty effect and the added cost of each move side by side, with the moves that no ruling supports set apart.",
      },
      {
        role: "Works with it",
        who: "Licensed customs brokers and trade lawyers.",
        gets: "A draft with the facts laid out and the cited rulings linked. They still confirm the classification and file the request. Unde does not replace them.",
      },
      {
        role: "Benefits: importer of record",
        who: "The U.S. company that declares the origin and pays the duty.",
        gets: "It carries the duty and the penalty risk if the origin is wrong. It gets an earlier warning when a planned move would leave the product Chinese, and a record of which rulings were read.",
      },
      {
        role: "Benefits: plant operator",
        who: "The ASEAN plant that would take on the work.",
        gets: "A clearer case for which operations to bring in, and early notice when a smaller move would not change the origin.",
      },
    ],
  },

  // Accuracy page trust sections: reproduce, and every held-out miss.
  // Data: frontend/src/lib/failure-cases.json, built by
  // eval/charts/build_trust_data.py from the committed held-out run.
  trust: {
    reproTitle: "Reproduce these numbers",
    reproLead: "Headline evaluation counts are recomputed from committed result rows. No model calls, no account, a few minutes on a laptop. Other figures are labelled reported or illustrative.",
    reproNote: "Each check fails loudly if a number has drifted from its source file.",
    failTitle: "Known failure cases",
    failLead: (miss: number, auto: number, n: number) =>
      `All ${miss} held-out rulings it got wrong, out of ${n}, not a selection. The ${auto} it auto-accepted come first: nobody checks those, so they are the costly ones. Each links to CBP's ruling.`,
    byOriginTitle: "Accuracy by the origin CBP held",
    byOriginNote: "same held-out run · 95% interval · origins with at least 19 rulings",
    chinaTitle: "Which way the misses go",
    china: (c: { missed_china: number; missed_china_auto: number; said_china: number; said_china_auto: number; neither: number }) => [
      `CBP held China, Unde said another country: ${c.missed_china} (${c.missed_china_auto} auto-accepted). This is the costly direction for a move out of China.`,
      `Unde said China, CBP held another country: ${c.said_china} (${c.said_china_auto} auto-accepted).`,
      `Neither was China: ${c.neither}.`,
    ],
    columns: ["Ruling", "Date", "Product (CBP's subject line)", "CBP held", "Unde said", "Calibrated (stated)", "Route"],
    auto: "auto-accepted",
    review: "sent to review",
    showAll: (n: number) => `Show all ${n} misses`,
    showAuto: "Show only the auto-accepted misses",
    patternsTitle: "What the misses have in common",
    patternsNote: "Our reading of the rulings above. A description, not a measurement.",
    patterns: [
      "Machining or assembly in the last country, where CBP held the imported core part decides. Two near-identical wheel-hub rulings were both missed at 0.95.",
      "Simple processing of one material credited as a transformation: wax melted and cast into a crayon, worked abalone shells.",
      "Choosing by the most subassemblies, where CBP held one essential part decides: an air compressor took the origin of its motor.",
      "Goods outside the product's target, food and natural products, still placed in scope by the router.",
      "Kits, where CBP rules item by item and the label follows one item.",
    ],
    source:
      "Source: the official held-out run, every row. Confidence is the calibrated value the product shows (auto-accept at 0.90), with the model's stated value in brackets (0.92 on that scale). Rebuilt by eval/charts/build_trust_data.py.",
  },
};
