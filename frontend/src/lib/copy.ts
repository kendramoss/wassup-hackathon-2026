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
    demoDate: (d: string) => `Rulings before ${d} only`,
    empty: "No cases yet. Cases you start show here, and only you see them.",
    newCase: "+ New case",
    recheckAll: "Re-check all",
    demos: "Demos from real rulings",
    demosNote: "Each has a real CBP ruling to compare with. They show how it works, not how often it is right.",
    demosMisses: "Where it was wrong →",
    moreDemos: "More demos, run live",
    moreDemosNote: "Opening one runs it now, so the result may differ from the recorded run.",
    howPicked: "How the demos were picked",
    // A demo's recorded development run (D191), on its card.
    recorded: {
      label: "Recorded test run",
      recordLink: "Open the recorded run (JSON) →",
      // The honesty label for a live card (#685 review): opening it only
      // fills the form; the origin runs on "Find the likely origin", and the
      // move search is a separate button.
      unseeded: "Live run: starts when you click Find origin",
      engine: "Unde:",
      agrees: "Agrees with CBP",
      disagrees: "Disagrees with CBP",
      plan: (plan: string, check: string, ruling: string) => `Plan: ${plan} (check ${check}${ruling ? `, ${ruling}` : ""}).`,
      includes: (items: string[]) => `What moves: ${items.join("; ")}.`,
      noPlan: "No backed plan.",
      // On a card that starts a live run: the run will search plans, nothing
      // is shown yet (judge panel round 3: "Plan included" read as ready).
      planRuns: "Live run, then you can search for moves",
      // A pinned demo read from CBP's text alone has no parts list to plan.
      originOnly: "Origin only: no parts list to plan",
      selection:
        "The ten family demos were picked by family, not by result: the first per-family check for vehicle parts, PCBA devices, power supplies, medical devices, paper and tissue products, off-road vehicles and machinery, semiconductor devices, golf and sporting goods, toys and build kits and heat pumps and heaters.",
      // D171 (23 checks, 7 off) and D661 live checks (6, 1 off): 29 and 8.
      disagreement: "Across all 29 per-family checks, Unde disagreed with CBP's holding on the current setup in 8. Development checks, one run each.",
    },
    columns: ["Product", "Likely origin", "Status", "Ruling watch", "Scenarios", "Where you left off"],
    notAnalysed: "Not analysed yet",
    // How supplied answers relate to the origin shown (D814).
    answers: {
      rechecked: "Re-checked with your answers",
      added: "Answers added. Open to see them.",
    } as Record<string, string>,
  },

  product: {
    title: "Describe your product",
    lede: "List each part and where it is made, or paste a description.",
    tabs: { parts: "Parts and plants", paste: "Paste a description", bom: "Import BOM" },
    name: "Product name",
    family: "Product family",
    date: "Case date",
    dateHelp: "Only rulings before this date are read.",
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
    bomMediaHelp: "Photos and scans are read by AI into a table you check. Up to 4 files, 8 MB, 10 PDF pages.",
    bomPhoto: "Take a photo",
    bomReading: "Reading the document…",
    bomBrowse: "Browse files",
    bomReplace: "Replace file",
    bomSamples: "Try a sample:",
    bomSamplesLoad: "Click one to load it here.",
    bomSamplesDownload: "Download the CSV",
    bomSamplePhoto: "Photo (synthetic)",
    bomSamplePhotoNote: "a synthetic, tilted rendering of the same table, to try photo import",
    bomTemplate: "Download CSV template",
    bomColumnsTitle: "Column format",
    bomColumnsNote: "Headers ignore case and accept the aliases shown. A PDF needs a table with a header row.",
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
      `Why · auto-accepted: confidence ${conf(o.value)}, ${conf(o.value) === o.bar.toFixed(2) ? "at" : "above"} the ${o.bar.toFixed(2)} auto-accept bar`,
    questionsNote: "optional",
    nextQuestions: (target: string) => `Answer CBP's questions, or plan a move to ${target}.`,
    suggestAnswer: (n: number) => (n === 1 ? "Answer CBP's likely question" : `Answer CBP's ${n} likely questions`),
    conflict: "Earlier rulings disagree",
    review: {
      title: "Not sure enough · worth a broker's look",
      body: (o: OriginConf) =>
        `Confidence ${conf(o.value)}${o.calibrated ? ` (model said ${conf(o.stated)})` : ""}. In testing, answers this unsure were right 68% of the time, against 96% above the bar.`,
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
    tariffRule: {
      match: /tariff-shift rule/,
      text: "19 CFR 102.20, the tariff-shift rule: a product takes the origin of the country where its foreign parts change tariff code in the way the rule names. Checked by code, not by a model.",
    },
    marking: {
      match: /marking rules/,
      text: "The Part 102 rules (19 CFR Part 102) CBP uses for some goods, such as those from Canada or Mexico, to decide country-of-origin marking.",
    },
    needsInfo: {
      match: /Needs more facts/,
      text: "Under the auto-accept bar, and the facts leave out something CBP would ask about. Answer the questions to re-run it.",
    },
    needsReview: {
      match: /Needs expert review/,
      text: "Under the auto-accept bar: a licensed customs broker should look at it before anyone relies on it.",
    },
  },

  // Revoked and modified rulings (D320, D400): said beside every ruling link.
  supersession: {
    by: (ruling: string, date: string) => `superseded by ${ruling}, ${date}`,
    notSupport: "not counted as support for this case",
    proposed: (kind: "revoked" | "modified", ruling: string) => `CBP proposed to ${kind === "revoked" ? "revoke" : "modify"} it in ${ruling}`,
  },
  plan: {
    // The stored backtest waits for the demo's record of the real change
    // (plan-step, judge panel round 7 N1).
    backtestLoading: "Loading the record of CBP's real change to compare with…",
    backtestFailed: "The record of CBP's real change did not load, so the backtest is not shown.",
    backtestRetry: "Try again",
    best: "Best plan",
    partsThatMove: "Parts that move",
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
    belowBarRow: (check: string) => `Below the bar (${check})`,
    // Ranking tiers (D340, D341): shown plans with a clear margin over the
    // bar rank first, then the plans at the bar, each by net a year.
    tier: {
      clear: "Clear margin",
      clearWhy: (from: string) => `check ${from} or more, no counter-ruling`,
      atBar: "At the bar",
      atBarWhy: "may not hold on re-check",
      smallest: "Smallest supported move",
      smallestLine: (phrase: string, n: number) => `Smallest supported move: ${phrase} (${n} ${n === 1 ? "part" : "parts"}).`,
      order: "clear margin first, then net",
      // D500: a Part 102 case read with the rule check.
      orderRule: "clear margin first, then the 102.20 check, then net",
      stored: "Stored before plans were ranked by tier (D341): this list is ranked by net alone. Search again to re-rank.",
    },
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
  // One sentence on what is new (judge panel round 4), on the landing only
  // (/method no longer shows it). Factual: what the product does, no claim
  // of being first. #685: the subtitle above it already says Unde finds the
  // moves that would change the origin, so this line keeps only what the
  // subtitle does not say.
  novel: {
    lead: "What is new:",
    text: "Unde shows only the moves an earlier CBP ruling backs.",
  },
  landing: {
    eyebrow: "fig.0 — US Customs country of origin",
    title: "Plan the production change before you build it.",
    sub: "See the origin U.S. Customs would likely give your product, and which production moves would change it.",
    demo: "Try the heat-gun demo →",
    fig1: "fig.1 — the heat gun",
    fig1Sub: "Kobalt 12.5-amp heat gun · real CBP rulings",
    fig1Label: "fig.1, the heat gun, drawn in 3D. In 2022, assembled in Thailand with the motor-fan assembly, the PCBA and the switch, cord and minor parts from China, CBP held China. A recorded 18 Oct production run made those three groups in Thailand with a 0.96 precedent check, above its 0.92 bar (docs/DEMO.md, section 6). The pinned demo on this site shows the same plan at 0.96. In its real 2026 change the manufacturer made the motor, fan, PCBA, knob, power cord and back cover in Thailand and kept the switch, wires and fasteners in China, and CBP held Thailand.",
    steps: [
      { title: "2022 setup", body: "Assembled in Thailand, key parts from China. CBP held China", ruling: "N328954" },
      { title: "Taken apart", body: "Each part and where it is made." },
      { title: "The plan", body: "Make the motor-fan, PCBA, switch and cord in Thailand. Backed by an earlier ruling", for: "N345938" },
      { title: "2026 setup", body: "The maker moved most of those parts. CBP held Thailand", ruling: "N357771" },
    ],
    stepOf: (n: number) => `Step ${n} of 4`,
    sheetStates: ["2022 setup · isometric", "exploded view", "planned move", "2026 setup · isometric"],
    planNote: "Unde's plan · backed by an earlier ruling",
    planMove: "Motor-fan, PCBA, switch + cord → Thailand",
    legendCn: "Made in China",
    legendTh: "Made in Thailand",
    drag: "Drag to turn",
    schematic: "Schematic from CBP ruling facts, not a manufacturer's drawing",
    fig2: "fig.2 — how it works",
    fig2Sub: "one case, four sheets · the heat gun",
    fig2Now: ["Product: every part and where it is made.", "Origin: the country CBP would likely assign.", "Plan: the move that would change it.", "Draft: the ruling request for a broker."],
    // Phones show all four sheets at once, so no single sheet is "now". This
    // is also the caption in the server-rendered HTML, before motion runs.
    fig2NowStatic: "Product → origin → plan → draft, read in order.",
    fig2Foot: "The line is the heat gun's facts moving through the case",
    // #516: the full "not blind" caveat lives on the figure's foot line; the
    // fixed-height Plan sheet carries only a short summary of the backtest.
    fig2BacktestNote: "Not a blind test: some moves were added after the change was public.",
    wires: ["facts", "origin", "moves"],
    fig3: "fig.3 — how accurate is it",
    fig3Title: "Tested on 377 CBP rulings, scored once",
    fig3Meta: "Jan–Sep 2026 · single-origin, multi-country U.S. rulings",
    fig3Caveat: "Prompts were not tuned on these rulings. The old 0.92 stated bar looked at both test splits; the 0.90 calibrated bar now used came from development data.",
    scaleHead: "A · how often it is right",
    squaresHead: "B · every ruling, as a square",
    claimAccuracy: "right. A simple rule gets 63.4%.",
    claimAuto: "right when it is sure (79.6% of cases). The rest go to a person.",
    // fig.3 row C (judge panel round 4): docs/NUMBERS.md, "Precedent check,
    // D170 + addendum (18 pairs), starting origin given", served as
    // planner_post_combined_known-a_* by GET /v1/eval/summary.
    planCheck: {
      head: "C · are the plans right?",
      meta: "18 later CBP before/after pairs · 3 runs",
      details: "Details",
      lead: "Every move is checked both ways, for and against, against earlier CBP rulings only. Tested on pairs of CBP rulings on one product before and after a change, at least one of them published after the 377 above, with CBP's starting origin given (gateway, not Vertex AI). Real changes found: 9 of 12 in each of 3 runs (75%).",
      rows: [
        { k: 9, n: 9, label: "plans shown were real changes", lo: 70, hi: 100, lowerIsBetter: false },
        { k: 9, n: 12, label: "real changes found", lo: 47, hi: 91, lowerIsBetter: false },
        { k: 0, n: 6, label: "false plans when nothing changed", lo: 0, hi: 39, lowerIsBetter: true },
      ],
      foot: "Small samples, so wide intervals. Without the starting origin: 5 of 6 shown were right in runs 1 and 2 and 5 of 5 in run 3, 5 of 12 found each time. The accuracy page also says about 29%: that is a stricter count on earlier pairs, where a pair counts only if it cleared the bar in every one of several runs, with and without the starting origin (17 of 59).",
      link: "Plan checks on the accuracy page →",
    },
    more: "How accurate is it? →",
    cta: "Open the cases →",
    alsoLink: "What you can do ↓",
    footerDisclaimer: "Not a ruling. Confirm with a licensed customs broker or a CBP binding ruling.",
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
    example: "Per-part codes and the rule check (recorded example)",
    // Judge panel round 10, item 2: the tour's last step also lists the
    // other recorded examples, and each example links the next.
    more: "More recorded examples:",
    examples: [
      { href: "/examples/rule-check", label: "Per-part codes and the rule check" },
      { href: "/examples/plan-rule-check", label: "A plan move the rule does not support" },
      { href: "/examples/part102-plan", label: "A Part 102 plan search, ranked by the rule" },
      { href: "/examples/revocation-alert", label: "A revoked-ruling alert" },
    ] as const,
    nextExample: (label: string) => `Next recorded example: ${label[0].toLowerCase()}${label.slice(1)} →`,
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

  // Self-service API keys (#788). Settings never shows a secret after it is
  // created; the Developers page points to the same place to make one.
  settings: {
    title: "Account settings",
    lead: "Manage the keys your integrations use with the Unde API and MCP endpoint.",
    signIn: "Sign in to manage your API keys. The demos stay open without signing in.",
    apiKeys: {
      menu: "Settings",
      heading: "Your API keys",
      intro: "Each key gets 200 requests per UTC day and its own rate-limit buckets. Cases a key creates stay with that key. Only you can see or revoke your keys.",
      unavailable: "Self-service keys are not available on this deployment. Ask the team for an admin-issued key instead.",
      loadFailed: "Your keys could not be loaded. Nothing was changed.",
      retry: "Try again",
      nameLabel: "New key name",
      namePlaceholder: "e.g. Claude Desktop",
      create: "Create key",
      creating: "Creating…",
      secretLead: "Copy this key now",
      secretNote: "It is shown only here. Unde stores only its hash, so a lost key must be replaced.",
      copy: "Copy",
      copied: "Key copied.",
      copyFailed: "Copy unavailable. Select the key and copy it yourself.",
      empty: "No keys yet. Create one above, then use it from your REST client or MCP agent.",
      active: "Active",
      revoked: "Revoked",
      created: "Created",
      perDay: "200 requests / UTC day",
      revoke: "Revoke",
      revoking: "Revoking…",
      revokeConfirm: "Revoke this key? Calls with it stop working at once. This cannot be undone.",
      developers: "How to use a key →",
    },
  },
  developers: {
    keyLead: "Need a key?",
    keyLink: "Sign in and create your own in Settings",
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
    more: "Scope and limits",
    origin: "A prediction from earlier CBP rulings, not a ruling. Have a licensed customs broker review it.",
    plan: "Suggestions, not rulings or CBP approvals. Have a licensed customs broker review a move first.",
    bestBounded: "Best of the moves searched, using the duty and cost assumptions shown, which may be prefilled or illustrative. When you leave a cost blank, the ranking may count it as zero, which is not a verified expense. Factors the model does not include are not counted.",
    compare: "A suggestion for a licensed customs broker to review, not a ruling.",
    compareBounded: "Ranked among the countries you chose, from the moves searched and the duty and cost inputs shown, which may be prefilled or illustrative. A missing required input leaves that figure unpriced, and factors the model does not include are not assessed.",
    scenarios: "What-ifs, not rulings. Have a licensed customs broker review a result first.",
    scenariosBounded: "Compared among the scenarios you entered, using the duty and cost inputs shown, which may be prefilled or illustrative. A missing required input leaves that figure unpriced, and factors the model does not include are not assessed.",
  },

  // What the tool is for and not for (#282). Reviewed wording from
  // docs/scope-and-jurisdiction.md; it says what the tool is for and makes no
  // claim that unsupported questions are detected or blocked (they are not).
  jurisdiction: {
    originScope: "U.S. origin only. Not for trade-agreement claims, certificates of origin or non-U.S. rules.",
    outOfScope: "This looks like a case this tool does not decide. A licensed customs broker should review it.",
    part102: "Marking origin under the Part 102 rules. This is not USMCA qualification or regional value content.",
    planScope: "Plans use the general origin test, except on Part 102 cases: there the Part 102 engine predicts each move and the 19 CFR 102.20 rule check orders the shown plans within each tier. Plans do not apply textile or pharmaceutical rules. Only products in a family with a plan template (28 families, listed in the family picker) are planned.",
    draftNote: "A draft ruling request for a broker to complete and file. It asks CBP a question; it is not a determination.",
    part177Label: "Ruling request letter (19 CFR Part 177)",
    part177Help: "Part 177 is how to ask CBP for a ruling. It is not an origin rule.",
    compareScope: "Compared on U.S. origin only. Section 232 duties are not included.",
    // Accuracy page, "What we don't claim" (listed unsupported in docs/scope-and-jurisdiction.md section 6).
    notCovered: "Outside scope: EU Binding Origin Information, USMCA regional value content and preferential claims, certificates of origin, duty drawback, and antidumping and countervailing duty. Section 232 duties are not included in cost figures.",
    // D680-D682: the market check reads the text, but misses about one in four.
    noDetector: "Pick the market when you start a case. The text check misses some.",
  },

  // The market a case is for (D680).
  market: {
    label: "Market",
    soon: "soon",
    unsupported: (name: string) => `Origin rules for ${name} aren't supported yet.`,
    unsupportedNext: "No answer yet. Entering the U.S.? Start a U.S. case.",
    startUS: "Start a U.S. case →",
    // What each market offers today, beside the picker (#733). Japan's line
    // follows the API's supported flag; it promises no accuracy.
    summary: ["U.S.: live", "Canada: rules check", "EU, UK: rules check, listed goods", "RCEP: tariff check", "Japan: trade preference", "Other markets: not yet"],
    summaryJapanOff: "Japan: not available yet.",
    // A case whose text names Northern Ireland (D767).
    northernIreland: "Northern Ireland: EU or UK rules can apply. Pick the market for a new case.",
    // In the case tabs, where Plan and Draft would be, on a non-U.S. case (#678).
    followUpsUSOnly: "Plans and drafts: U.S. only for now",
  },

  japan: {
    // One short availability line for places that mention Japan without a
    // configured gateway (#730). It does not promise a live check.
    notAvailable: "Japan: not available yet.",
    scope: "Trade preference in Japan",
    agreement: "Trade agreement",
    chooseAgreement: "Choose an agreement",
    tariffCode: "Product tariff code in Japan",
    tariffCodeFor: (edition: string) => `Product tariff code (HS ${edition})`,
    needAgreement: "Choose a trade agreement",
    needTariffCode: "Enter a 4, 6 or 9 digit tariff code",
    facts: "Materials and production",
    factsHelp: "List material countries, tariff codes and production locations.",
    factsExample: "Plywood is pressed in Vietnam from Malaysian veneer (4408.10). The veneer has proof of origin under RCEP.",
    check: "Check trade preference",
    checking: "Checking trade preference.",
    noResult: "Add the agreement and tariff code to check eligibility.",
    qualifies: "Likely qualifies",
    doesNotQualify: "Likely doesn’t qualify",
    undetermined: "Needs more evidence",
    review: "Not a ruling. Confirm with a Japanese customs specialist.",
    simulated: "Simulated result",
    approvalsOnly: "The published rulings used for testing all approved preferential treatment.",
    origin: "Likely origin",
    details: "Details",
    rule: "Product rule",
    precedents: "Earlier Japan Customs rulings",
    edit: "Edit inputs",
  },

  // The 19 CFR 102.20 rule check (D363), shown only when the parts list gives
  // HTS codes. It never restates the model's answer as the rule's.
  ruleCheck: {
    title: "Rule check (19 CFR 102.20)",
    decided: "under the tariff-shift rules, from the codes given.",
    differs: (m: string) => `under the tariff-shift rules, from the codes given. The model said ${m || "something else"}: worth a broker's look.`,
    deMinimis: (country: string, o: string) => `open on value: ${country || "the country of production"} if the parts that do not shift are worth no more than 7% of the good, otherwise ${o}.`,
    undetermined: "not decided from the codes given; it needs a fact the codes cannot show.",
    // The chapters in backend/internal/tariffshift/rules.json (D360, D460);
    // scripts/rule-check.test.mjs checks the two lists match.
    chapters: ["15", "21", "39", "40", "44", "48", "71", "73", "76", "82", "83", "84", "85", "87", "90", "94", "95"],
    outOfScope: (chapters: string[]) =>
      `not encoded for this product's chapter. Encoded: chapters ${chapters.slice(0, -1).join(", ")} and ${chapters[chapters.length - 1]}.`,
    outOfScopeTextile: "not encoded: a chapter note sends this code to the textile rules (19 CFR 102.21).",
    addCodes: (n: number, good: boolean) =>
      `add HTS codes for ${good ? "the product" : ""}${good && n ? " and " : ""}${n ? `${n} part${n === 1 ? "" : "s"}` : ""} to run it.`,
    addCodesLink: "Add codes",
    noCountry: "not run: the country where the product is made is not known. Add a plant and its final-assembly step.",
    plantsLink: "Add plants",
    goodCode: (reason: string) => `not run: ${reason.trim().replace(/[.]$/, "") || "the product's code does not pick one rule"}.`,
    goodCodeLink: "Edit the product code",
    informational: "This case is decided under substantial transformation; the Part 102 check is for comparison only.",
    note: "Checked by code from the HTS codes given. It does not change the answer above or what is auto-accepted. Not a ruling.",
    parts: "Each part",
    domestic: "same country",
    ruleText: "The rule, as published",
    encoded: (e: number, n: number) => `${e} of ${n} sentences of this rule are encoded; the others name goods in words and are left to a person.`,
    // D430: the check re-run over the other codes suggested for each part.
    robustStable: (n: number, total: number, sampled: boolean, parts: string) =>
      `Same answer for every suggested code: ${sampled ? `a sample of ${n} of ${total} combinations` : `${n} combination${n === 1 ? "" : "s"}`} of the codes suggested for ${parts}.`,
    robustDepends: (parts: string) => `The answer depends on the code for ${parts}. Check that code with your broker first.`,
    robustTogether: "The answer depends on which suggested codes are chosen together. Check the part codes with your broker first.",
  },

  // RCEP preference check (D690). A different question from the origin
  // answer, so the copy says so once and stays short (rule 16).
  rcep: {
    title: "RCEP tariff",
    meta: "separate from origin",
    lead: "The Regional Comprehensive Economic Partnership (RCEP) is a trade deal between Asia-Pacific countries. This checks whether the product can get the lower tariff. A rules check, not yet reviewed by a customs professional.",
    needsTitle: "What you need",
    needs: ["Product HS code", "Where it is made", "Each part's origin and value", "Sale price (FOB), optional"],
    tick: "Tick a part if your supplier declares it has RCEP origin.",
    needCode: "Add the product's HS code to check.",
    madeIn: "Made in",
    price: "Price (FOB)",
    part: "Part",
    declared: "RCEP origin",
    declaredFor: (n: string) => `${n}: RCEP origin`,
    value: "Value",
    valueFor: (n: string) => `${n}: value`,
    noCode: "no code",
    check: "Check",
    checking: "Checking",
    yes: "Qualifies",
    no: "Does not qualify",
    unsure: "Not sure yet",
    failing: "Parts that block it",
    details: "Details",
    failed: "The check failed. Try again.",
  },

  // Canada's marking rules (SOR/94-23, D720): a rules check only. Few words
  // (rule 16); the label stays visible with every answer (D725).
  canadaCheck: {
    title: "Country-of-origin marking for Canada (rules check)",
    run: "Check Canada rules",
    running: "Checking…",
    label: "Rules check only — no accuracy figure yet (no published rulings to test against)",
    decided: "under Canada's marking rules, from the codes given.",
    undetermined: "Not decided from the codes given.",
    outOfScope: "No Canadian rule for this code.",
    addCodes: "To run it, add:",
    needGood: "Product HS code",
    needPart: "HS code for at least one part",
    details: "Details",
    // Guidance on an undetermined answer: yes / no / not sure per fact.
    yes: "Yes",
    no: "No",
    notSure: "Not sure",
    toAdd: "To decide, add:",
    part: (n: number) => `Part ${n}`,
    review: "A person needs to read this rule.",
    why: "Why we ask",
    yourAnswers: "Your answers",
    clearAll: "Clear all",
    shareBad: "Enter 0 to 100",
    shareLabels: { value_share: "Value share", weight_share: "Weight share" } as Record<string, string>,
    answerLabels: { simple_assembly: "Simple assembly", gri_2a: "Parts only gathered together", bending_to_shape: "Bending to shape" } as Record<string, string>,
  },

  // The EU and UK non-preferential origin checks (D760-D766): rules checks
  // only. Few words (rule 16); the label stays visible with every answer, and
  // the source, version and hash sit under Details.
  originRules: {
    title: { eu: "EU origin (rules check)", uk: "UK origin (rules check)" } as Record<string, string>,
    question: { eu: "Origin under EU non-preferential rules?", uk: "Origin under UK non-preferential rules?" } as Record<string, string>,
    // The API sends the label; these are the same words (D767): the EU
    // publishes BOI outcomes without facts, HMRC no rulings.
    label: {
      eu: "Rules check only — no accuracy figure (no published origin decisions with their facts to test against)",
      uk: "Rules check only — no accuracy figure (no published origin rulings to test against)",
    } as Record<string, string>,
    decided: { eu: "under EU origin rules, from the codes given.", uk: "under UK origin rules, from the codes given." } as Record<string, string>,
    undetermined: "Not decided yet.",
    judgement: "Needs a substantial-transformation judgement.",
    judgementNext: "A customs adviser decides where it was last substantially processed.",
    toDecide: "To decide:",
    none: "None of these",
    running: "Checking…",
    addCodes: "To run it, add:",
    needGood: "Product HS code",
    needPart: "HS code for at least one part",
    details: "Details",
    rule: "Rule",
    outcome: { met: "met", not_met: "not met", open: "open" } as Record<string, string>,
    assumes: "Assumes",
    who: "Who applies it",
    source: "Source",
    version: "Version",
    retrieved: "Retrieved",
    hash: "SHA-256",
    licence: "Licence",
    // The intake (#770, D800-D802): missing information is asked, with the
    // document that settles it; a rule that is not modelled gets no control.
    missing: "Missing information",
    notModelled: "Rule not modelled",
    ask: "What would decide it",
    from: "From",
    yes: "Yes",
    no: "No",
    percent: "% of the good",
    recheck: "Re-check",
    rechecking: "Checking…",
    fixFields: "Fix the fields marked.",
    onProduct: "Add on the product",
    editProduct: "Edit product",
    person: "Needs a person",
    yours: "Your answers",
    yoursMeta: (n: number, date: string) => `${n} · you supplied · ${date}`,
    yoursNote: "Kept beside the case. The product is unchanged.",
    clear: "Clear answers",
    stale: "Some answers no longer fit the parts list; they were not used.",
    staleAll: "Your answers were from before the parts list changed. Answer again.",
    badShare: "Enter a number from 0 to 100.",
    badCode: "Enter an HS code, like 8529.90.",
    demo: "Pinned demo: answers can't be added.",
  },

  // Family questions (#770, D810-D813): what the family's rulings turn on,
  // each with the document that settles it. Not modelled: no controls.
  family: {
    title: "Missing information",
    notModelled: "Not modelled",
    lede: "The rulings cited turned on these facts.",
    from: "From",
    decided: "Decided origin",
    notEnough: "Not enough",
    known: "On the parts list",
    country: "Country",
    yes: "Yes",
    no: "No",
    save: "Save answers",
    saving: "Saving…",
    saved: "Saved. The origin above is unchanged; it was made before these answers.",
    complete: "All answered.",
    yours: "Your answers",
    yoursMeta: (n: number, date: string) => `${n} · you supplied · ${date}`,
    yoursNote: "Kept beside the case. The product and facts are unchanged.",
    clear: "Clear answers",
    stale: "Your answers were from before the parts list changed. Answer again.",
    dropped: "Some answers no longer fit this product; they were not used.",
    badCountry: "Pick a country from the list.",
    fixFields: "Fix the fields marked.",
    demo: "Pinned demo: answers can't be added.",
  },

  // The evidence report (#771, D803-D805): every check on its own card.
  report: {
    title: "All checks",
    lede: "Each check answers a different legal question. They are not combined.",
    notRuling: "Not a ruling.",
    download: "Download (.md)",
    print: "Print",
    loading: "Running the checks…",
    status: {
      answered: "Answered",
      needs_information: "Missing information",
      needs_judgement: "Rule not modelled",
      not_run: "Not run",
      not_applicable: "Not covered",
    } as Record<string, string>,
    missing: "Missing",
    notModelled: "A person decides",
    details: "Sources and limits",
    version: "Version",
    made: "Made",
    review: "Needs review",
    supplied: "Uses your answers",
    answer: "Answer",
    caseFacts: "Case facts",
    caseFactsNote: "As stored. Your answers to a check are kept apart.",
    link: "All checks for this product",
  },

  // The 102.20 rule check on each plan move (D441): a chip on the plan card
  // and each row of the moves table. Display only.
  planRule: {
    explain:
      "The 19 CFR 102.20 tariff-shift check of the product as it would be after this move, from the HTS codes on the parts list. It never changes the precedent check or which plans are shown. On a Part 102 case it orders the shown plans within each tier; otherwise it is display only. Not a ruling.",
    caption: (part102: boolean) =>
      part102
        ? "Each move also shows the 19 CFR 102.20 rule check of the product after the move. This case is decided under Part 102, so within each tier the moves the rule supports come first and the moves it gives to another country come last, flagged. It does not change which plans are shown."
        : "Each move also shows the 19 CFR 102.20 rule check, for comparison: this case is decided under substantial transformation. Display only: it does not change which plans are shown or their order.",
    // Country comparison (D500): the best move's check, and how it ranks.
    compareCaption: "Part 102: moves the 102.20 rule supports rank first",
    compareHow:
      "On a Part 102 case each country's best move is also checked against the 19 CFR 102.20 tariff-shift rule: within a tier, a move the rule supports ranks above one it cannot decide, and one it gives to another country ranks last, flagged for a broker. On a substantial-transformation case the check is shown for comparison only.",
  },

  // /examples/rule-check: a recorded, read-only example of the per-part codes
  // and the rule check (judge panel round 8, N11). The pinned demo case has no
  // part codes, so its own pages show neither.
  ruleCheckExample: {
    title: "Per-part codes and the rule check",
    eyebrow: "Recorded example",
    lede: "Codes suggested for each part, and what the tariff-shift rule decides from them. It is recorded: nothing here calls a model or saves anything.",
    details: "Details",
    detailsBody: "The heat-gun demo has no part codes, so its pages show neither feature. This is one development ruling from the per-part evaluation (D381, D382).",
    product: "The product",
    madeIn: "Made in",
    goodCode: "CBP's code for the product",
    fromRuling: "from the ruling",
    parts: "Suggested code for each part",
    partsNote: "The check uses the first code, then re-runs with the others.",
    why: "Why these codes",
    stated: (code: string) => `The ruling's text states ${code}.`,
    noneStated: "The ruling's text states no code for this part.",
    before: "With only the codes the ruling's text states",
    after: "With the suggested part codes",
    cbp: "What CBP held",
    agrees: "The check agrees with CBP's ruling.",
    differs: "The check does not agree with CBP's ruling.",
    ourNote: "Our note, added after the run",
    ourNoteShort: "Our note: the second code is the likely one. The check agrees under either.",
    robust: "Under every suggested code",
    honest: "One ruling, picked to show the check deciding. On most Part 102 rulings it still does not decide: the product is outside the encoded chapters, or a part needs a fact the codes cannot show. The method page has the counts.",
    source: "Rebuilt from committed files, no model call:",
    tourBack: "Back to the heat-gun tour",
  },

  // /examples/plan-rule-check: a recorded plan search with the 19 CFR 102.20
  // check on each move (D441, D442; judge panel round 10, item 2a). The demo
  // case has no part codes, so its plan shows no move check.
  planRuleExample: {
    title: "A plan move the tariff-shift rule does not support",
    eyebrow: "Recorded example",
    lede: "An earlier ruling backs a move, but the tariff-shift rule gives another country, so the move is flagged for a broker. It is recorded: nothing here calls a model or saves anything.",
    details: "Details",
    detailsBody: "The heat-gun demo has no part codes, so its plan shows no rule check. This is one committed development plan search, checked with 19 CFR 102.20.",
    product: "The product and the current setup",
    target: "Target country",
    cbpHeld: "CBP held",
    codes: "Codes the rule check used",
    codesNote: "Top suggestions from a recorded run, the product's code included.",
    plants: "Plants",
    moves: "Each move, with both checks",
    movesNote: "Precedent check: an AI agent reads earlier CBP rulings. Rule check: code applies the tariff-shift rule to the product after the move. The rule check never reorders the moves.",
    current: "Current setup",
    precedent: "Precedent check",
    rule: "Rule check (19 CFR 102.20)",
    supportedBy: "An earlier ruling supports it",
    at: (conf: string) => `check confidence ${conf}.`,
    notSupported: (conf: string) => `No earlier ruling supports it, check confidence ${conf}.`,
    belowBar: "Below the 0.92 bar, so not a shown plan.",
    noSupport: "Not a shown plan.",
    shown: "A shown plan.",
    engine: (c: string) => `Origin model after the move: ${c}.`,
    why: "Why the two disagree",
    whyBody: "Every move leaves final assembly in China, so the rule gives China. The precedent check found rulings where the steel tool body decides origin, so it backs machining it in Vietnam. A broker should settle that before work moves.",
    honest: "One search, picked because it is the only committed one where the two checks disagree (2 of 23 decided moves with a precedent check, D442). These cases are decided under substantial transformation, so 102.20 is shown for comparison here, not as the test. Neither flagged move cleared the 0.92 bar. Part 102 plan searches, where 102.20 is the test, are scored apart: three on development rulings (D502).",
    source: "Rebuilt from committed files, no model call:",
  },

  // /examples/part102-plan: the D630 plan search for N348465 with the
  // production Part 102 engine (usmca-v5), with the 102.20 check on each
  // move, the D500 order and the broker draft's Part 102 paragraphs (judge
  // panel round 11, item 3; round 13, item 3).
  part102Example: {
    title: "A Part 102 plan search, ranked by the tariff-shift rule",
    eyebrow: "Recorded example",
    lede: "An actuator finished in Canada from Chinese parts; CBP held China. For origin marking (19 CFR Part 102) the tariff-shift rule is the test, so each move is checked and ranked by it. It is recorded: nothing here calls a model or saves anything.",
    details: "Details",
    product: "The product and the current setup",
    cbpSubject: "CBP's subject",
    cbpHeld: "CBP held",
    target: "Target country",
    order: "Order of shown plans",
    orderBody: "Tier first, then the rule check (moves it says shift, then undetermined, then another country), then net per year (D500).",
    run: "This run",
    runBody: (agrees: number, moves: number, v1: number, before: number) =>
      `The production Part 102 engine, usmca-v5, predicted each move (D630, run on 22 Nov). On ${agrees} of ${moves} moves it agrees with the rule: Canada where the rule says the move shifts, China where it does not or only the de minimis value is open. In earlier runs of the same search, usmca-v1 agreed on ${v1} of ${moves} (D541) and the general engine on ${before} (D502).`,
    codes: "Codes the rule check used",
    codesNote: "Top suggestions for each part; the product's code is CBP's. The DC motor shares the product's heading, 8501, so it never makes the tariff change unless it moves.",
    current: "Current setup",
    currentBody: "Final assembly is already in Canada, but the Chinese motor stays in heading 8501, so the rule cannot give Canada (only the small-value exception is open). That is CBP's own reason for holding China.",
    shownHead: (n: number) => (n === 1 ? "The shown plan" : `The shown plans (${n})`),
    motorHead: (n: number) => `Other moves that bring the motor to Canada (${n})`,
    motorNote: "The rule gives Canada for each. None cleared the precedent bar.",
    otherHead: (n: number) => `Moves without the motor (${n})`,
    otherNote: "The Chinese motor still fails the rule, so each stays Chinese and none went to the precedent check.",
    smallest: "The smallest move the rule supports. Precedent did not clear it.",
    precedent: "Precedent check",
    rule: "Rule check (19 CFR 102.20)",
    supportedBy: "An earlier ruling supports it",
    at: (conf: string) => `check confidence ${conf}.`,
    notSupported: (conf: string) => `No earlier ruling supports it, check confidence ${conf}.`,
    noCheck: "Not checked against precedent: the origin model already kept China.",
    belowBar: "Below the 0.92 bar, so not a shown plan.",
    noSupport: "Not a shown plan.",
    shown: "A shown plan.",
    afterMove: "Origin after the move:",
    engine: (c: string) => `Origin after the move, Part 102 engine (usmca-v5): ${c}.`,
    engineV1: (c: string) => `usmca-v1 in the earlier run (D541): ${c}.`,
    engineBefore: (c: string) => `General engine in the earliest run (D502): ${c}.`,
    engineCorrected: "The general engine predicted Canada here, though the rule says the move does not shift; the Part 102 engine does not.",
    engineDisagrees: "The Part 102 engine and the rule disagree on this move.",
    parts: (names: string) => `Moves: ${names}.`,
    draft: "The broker draft for the first shown plan",
    draftNote: "Built by the same code as a real Part 102 draft. Only these parts of the letter are shown.",
    question: "The question asked",
    paragraph: "The tariff-shift paragraph",
    reviewer: "Note beside the letter",
    honest: "One development ruling, one plan search with the production Part 102 engine (usmca-v5), run once (D630), beside the earlier runs of the same search with usmca-v1 (D540, D541) and the general engine (D501, D502). The part codes are unverified suggestions and the plants are assumed. The precedent check varies from run to run on the same facts: the motor alone scored 0.85 here and with usmca-v1, 0.82 with the general engine, and the two shown plans are not the same two as in the usmca-v1 run. On the three development searches usmca-v1 agreed with the rule on 13 of 14 moves (D541); the other two were not re-run with usmca-v5, so its one miss, on the compressors, is not re-measured. On the held-out Part 102 plan set, usmca-v5 agrees with the rule on 6 of 6 decided moves, against 3 of 6 for usmca-v1 (D631); five of the six are moves where the rule keeps the country CBP held. This shows the mechanism on a seen ruling, not an accuracy figure.",
    nameNote: "The product's name was cleaned in D532, after the two earlier runs and before this one. The earlier runs' own facts still use CBP's subject as the name, so they are not shown here.",
    source: "Rebuilt from committed files, no model call:",
  },

  // /examples/revocation-alert: a recorded revocation alert (D480, D481,
  // D490; judge panel round 10, item 2b). No demo case shows one.
  revocationExample: {
    title: "A ruling the answer cited was later changed",
    eyebrow: "Recorded example",
    lede: "When CBP later changes a ruling a saved answer cited, the case shows this box. It is recorded: nothing here calls a model, saves or re-checks anything.",
    details: "Details",
    detailsBody: "No demo case relies on a ruling CBP later changed. This is one committed development answer whose cited ruling CBP modified after the case date.",
    answer: "The saved answer",
    caseDate: "Case date",
    predicted: "Predicted origin",
    cited: "Rulings it cited",
    box: "The box on the case",
    boxNote: (asOf: string) => `As a saved case would show it, as of ${asOf}.`,
    words: "CBP's words",
    boxDetail:
      "Worked out by the same rule as the API from the committed revocation map. It compares the origin the cited ruling held before the change, and the one the changing ruling holds, with the answer (D531). When a holding is not in the labels, it says so.",
    change: "What CBP changed",
    holding: "Origin held:",
    published: "Published in",
    listBadge: "On the cases list, under the case name:",
    recheck: "What a re-check does",
    recheckBody: [
      "Predicts again with rulings up to today. A revoked or modified ruling is not cited.",
      "The saved answer is never changed. The origin step shows whether the new answer differs.",
      "Same limits as any prediction. A pinned demo cannot be re-checked.",
    ],
    honest: "One answer, picked because it is the only committed one that shows the box for a change after its case date (2 development runs of N356467, D490). The map knows the corpus letters and 28 fetched Customs Bulletins, so an alert can be missing. Nobody is notified by email or push; the box shows when the case is opened.",
    source: "Rebuilt from committed files, no model call:",
  },

  // Per-part HTS suggestions (D380): offered, never filled in by themselves.
  partsHts: {
    suggest: "Suggest codes for all parts",
    again: "Suggest again",
    busy: "Reading earlier rulings…",
    why: "The tariff-shift rule check needs a code for the product and each part.",
    acceptAll: "Accept all top suggestions",
    accepted: (n: number) => `${n} code${n === 1 ? "" : "s"} filled in. They take effect when you run the prediction.`,
    unverified: "Unverified suggestions: check every code with your broker before you rely on it.",
    fromRulings: "earlier CBP ruling",
    fromModel: "model's reading, no ruling found",
    checkCode: "Check this code",
    use: "Use",
    using: "Using",
    none: "No suggestion",
    product: "Product",
    tooMany: (n: number) => `Suggesting the first ${n} parts without a code; run it again for the rest.`,
    noneMissing: "Every part already has a code.",
    reasonsDown: "Reasons are unavailable right now; only codes from earlier rulings are listed.",
    failed: "Couldn't get suggestions. Try again, or type the codes.",
    busyLimit: "Too many suggestions at once. Wait a moment and try again.",
  },

  // USMCA selected origin rule (D740): a rules check only. Few words; no
  // claim about the good itself.
  usmcaCheck: {
    title: "USMCA origin-rule check",
    meta: "U.S., selected rule only",
    lead: "Checks one USMCA rule for a good entering the U.S. Covers 8202.31, 8207.13 and 9503.00 only.",
    good: "Good HS code",
    hsPlaceholder: "HS code",
    originColumn: "USMCA origin",
    removeMark: "×",
    materials: "Materials",
    addMaterial: "Add material",
    removeMaterial: (n: number) => `Remove material ${n}`,
    materialCode: (n: number) => `Material ${n}: HS code`,
    materialOrigin: (n: number) => `Material ${n}: has USMCA origin`,
    originYes: "Yes",
    originNo: "No",
    originUnknown: "Not known",
    complete: "The list includes every material without USMCA origin or of unknown origin.",
    value: "Value method",
    valueNone: "Choose one",
    methodTv: "Transaction value",
    methodNc: "Net cost",
    amountTv: "Transaction value",
    amountNc: "Net cost",
    vnm: "Value of materials without USMCA origin or of unknown origin",
    sameUnit: "Same currency for both amounts.",
    simple: "Check the standard rule without using these special provisions: sets, accessories or packing, fungible-material or inventory methods, intermediate-material designation, cost averaging, de minimis, outside-party operations, special valuation, the parts route (Art. 4.2(d)), accumulation, remanufactured goods, automotive rules.",
    check: "Check",
    checking: "Checking",
    failed: "The check failed. Try again.",
    stale: "Facts changed. Check again.",
    fixInputs: "Fix the marked fields.",
    badHs: "Use 4 to 10 digits, like 8202.31.",
    badMaterialHs: "Use 2 to 10 digits, like 8202.39.",
    badAmount: "Digits only, up to 12 decimals, like 1250.50.",
    label: "Selected origin rule only",
    status: {
      met: "Rule met",
      not_met: "Rule not met",
      needs_information: "More facts needed",
      not_supported: "Not covered yet",
    },
    notMet: "Only the alternatives of the selected rule were checked. This does not say the good lacks USMCA origin.",
    met: "The selected rule is met on these facts. Other requirements are not checked.",
    notCovered: "This check covers 8202.31, 8207.13 and 9503.00 only.",
    needs: "Still needed",
    calc: "Regional value",
    exact: "exact",
    undefinedCalc: "Cannot be calculated: the amount is zero.",
    details: "Source and scope",
    how: "How it was checked",
    outcome: { satisfied: "met", not_satisfied: "not met", unknown: "open" },
    rule: "Rule",
    snapshot: "Snapshot",
    version: "Data",
    scope: "Scope",
    facts: {
      profile_unconfirmed: "Confirm the simple-goods statement.",
      materials_incomplete: "Confirm the material list is complete.",
      rvc_method_required: "Choose a value method.",
      origin_status_unknown: "Say whether it has USMCA origin.",
      material_hs_unknown: "Add its HS code.",
      material_hs_imprecise: "Add a longer HS code.",
      sibling_part_unconfirmed: "Confirm whether a material from the required sibling code is present.",
      insufficient_precision: "Add a longer HS code for the good.",
      unknown_value: "Add the amount.",
      zero_denominator: "The amount cannot be zero.",
      rvc_undefined: "The amount cannot be zero.",
    } as Record<string, string>,
    fieldLabels: {
      good_hs: "Good",
      rvc_method: "Value method",
      transaction_value: "Transaction value",
      net_cost: "Net cost",
      vnm: "Value of materials without USMCA origin or of unknown origin",
      materials: "Materials",
    } as Record<string, string>,
    materialLabel: (n: number) => `Material ${n}`,
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
    lead: "Designed for five roles. No customers or pilots yet.",
    roles: [
      {
        role: "Uses it",
        who: "Trade-compliance lead at an ASEAN contract manufacturer.",
        gets: "The likely origin and backed moves, before work moves.",
      },
      {
        role: "Pays for it",
        who: "Supply-chain head, or the U.S. brand.",
        gets: "Duty saved against added cost, per move.",
      },
      {
        role: "Works with it",
        who: "Licensed customs brokers and trade lawyers.",
        gets: "A draft to check and file. Unde does not replace them.",
      },
      {
        role: "Benefits: importer of record",
        who: "The U.S. company that pays the duty.",
        gets: "Early warning when a move would not change the origin.",
      },
      {
        role: "Benefits: plant operator",
        who: "The ASEAN plant that takes on the work.",
        gets: "Which operations to bring in.",
      },
    ],
  },

  // Accuracy page trust sections: reproduce, and every held-out miss.
  // Data: frontend/src/lib/failure-cases.json, built by
  // eval/charts/build_trust_data.py from the committed held-out run.
  trust: {
    reproTitle: "Reproduce these numbers",
    reproLead: "Headline counts are recomputed from committed files. No model calls, no account, a few minutes on a laptop. Other figures say reported or illustrative.",
    reproNote: "Each check fails loudly if a number has drifted from its source file.",
    // ScrollX regions need their own names: reusing the parent section's name
    // creates duplicate landmarks and fails axe's landmark-unique rule (#508).
    reproCommandsLabel: "Reproduction commands (scrollable)",
    failTitle: "Known failure cases",
    failTableLabel: "Misses table (scrollable)",
    failLead: (miss: number, auto: number, n: number) =>
      `All ${miss} of ${n}, not a selection. The ${auto} auto-accepted ones come first: nobody checks those, so they cost the most.`,
    byOriginTitle: "Accuracy by the origin CBP held",
    byOriginNote: "origins with 19 or more rulings",
    chinaTitle: "Which way the misses go",
    china: (c: { missed_china: number; missed_china_auto: number; said_china: number; said_china_auto: number; neither: number }) => [
      `CBP held China, Unde said another country: ${c.missed_china} (${c.missed_china_auto} auto-accepted). This is the costly direction for a move out of China.`,
      `Unde said China, CBP held another country: ${c.said_china} (${c.said_china_auto} auto-accepted).`,
      `Neither was China: ${c.neither}.`,
    ],
    columns: ["Ruling", "Date", "Product", "CBP held", "Unde said", "Confidence (model said)", "Route"],
    auto: "auto-accepted",
    review: "sent to review",
    showAll: (n: number) => `Show all ${n} misses`,
    showAuto: "Show only the auto-accepted misses",
    patternsTitle: "What the misses have in common",
    patternsNote: "Our reading, not a measurement.",
    patterns: [
      "Machining or assembly in the last country, where CBP held the imported core part decides. Two near-identical wheel-hub rulings were both missed at 0.95.",
      "Simple processing of one material credited as a transformation: wax melted and cast into a crayon, worked abalone shells.",
      "Choosing by the most subassemblies, where CBP held one essential part decides: an air compressor took the origin of its motor.",
      "Goods outside the product's target, food and natural products, still placed in scope by the router.",
      "Kits, where CBP rules item by item and the label follows one item.",
    ],
    source:
      "Source: the official run on the unseen test set, every row. Confidence is the calibrated value the product shows (auto-accept at 0.90), with the model's stated value in brackets (0.92 on that scale). Bars show 95% intervals. Rebuilt by eval/charts/build_trust_data.py.",
  },
};
