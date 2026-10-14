// Everything the landing page says, in one place. Rewrite this once the
// hackathon idea and theme are settled; the page layout won't need to change.
export const site = {
  name: "Wassup",
  tagline: "An AI agent that uses real tools, not guesses.",
  description:
    "Ask a question in plain language. The agent decides which tools to call, runs them, and answers from the results, so you can see exactly where each answer came from.",

  // Problem statement / theme for the judges (Problem Alignment, 25%).
  problem: {
    title: "The problem",
    body: "Language models answer confidently even when they can't know the answer, like the current time somewhere. This agent calls a tool instead of guessing and shows every call it made.",
  },

  // "Try it" prompts on the landing page and the empty chat.
  examples: [
    "What time is it in Tokyo?",
    "Compare the time in London and New York",
    "Is it night in Sydney right now?",
  ],

  // What Gemini decides vs. what is plain code (Technical Merit, 40%).
  aiDecides: [
    "Whether a question needs a tool at all",
    "Which tool to call and with what arguments",
    "How to turn tool results into an answer",
  ],
  codeDoes: [
    "Runs the tools (get_time reads the real clock)",
    "Validates input and limits request size",
    "Keeps the backend private behind service-account auth",
  ],

  stack: [
    { name: "Gemini 3.5 Flash", detail: "on Vertex AI" },
    { name: "Agent Development Kit", detail: "Go" },
    { name: "Next.js", detail: "frontend" },
    { name: "Cloud Run", detail: "both services, auto-deployed from GitHub" },
  ],

  // Shown in the footer only when set.
  links: {
    demoVideo: null as string | null,
    deck: null as string | null,
    repo: null as string | null,
  },
};
