// Pinned-demo Ask follow-ups (#309). A demo is never written, so the server
// cannot remember what this visitor asked. The drawer sends the visitor's own
// completed, visible turns and the server (which already has the seeded demo
// chat) adds them to the agent's context.

export const ASK_HISTORY_MAX = 6; // caseagent.MaxHistory
export type AskHistoryTurn = { question: string; answer: string };

export const ASK_ANSWER_MAX = 4000; // runes; the server refuses a longer history answer (400)
// What the agent sees in place of a completed answer longer than the limit.
// The whole answer is replaced, never cut to a prefix, so no half-fact reaches
// the model; the visitor still sees the original answer.
export const ASK_OMITTED = "[Earlier answer omitted from context because it exceeded the limit; ask the user to restate relevant details.]";

type Shown = { question?: string; answer?: string };

// Completed turns the visitor asked in this visit: `isLocal` selects them
// (seeded turns are the server's already). A turn with no visible question or
// answer is skipped, and a pending or failed ask is never in `chat`. Only the
// last ASK_HISTORY_MAX are sent; the server refuses more. An over-long answer
// is replaced by ASK_OMITTED and `omitted` says so, so the UI can tell the
// visitor. A question is capped by the input (2000) so it is sent as is.
export function askContext(chat: readonly Shown[], isLocal: (t: Shown) => boolean): { history: AskHistoryTurn[]; omitted: boolean } {
  const all: { turn: AskHistoryTurn; omitted: boolean }[] = [];
  for (const t of chat) {
    const question = (t.question ?? "").trim();
    const answer = (t.answer ?? "").trim();
    if (!isLocal(t) || !question || !answer) continue;
    const long = [...answer].length > ASK_ANSWER_MAX;
    all.push({ turn: { question, answer: long ? ASK_OMITTED : answer }, omitted: long });
  }
  const last = all.slice(-ASK_HISTORY_MAX);
  return { history: last.map((x) => x.turn), omitted: last.some((x) => x.omitted) };
}

export const askHistory = (chat: readonly Shown[], isLocal: (t: Shown) => boolean) => askContext(chat, isLocal).history;
// The ask request body: history only for a pinned demo and only when there is
// some; a normal case's saved chat is the server's sole source.
export function askBody(question: string, pinned: boolean, history: readonly AskHistoryTurn[]) {
  return pinned && history.length > 0 ? { question, history: history.slice(-ASK_HISTORY_MAX) } : { question };
}
