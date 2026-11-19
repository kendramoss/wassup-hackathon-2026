// Pinned-demo Ask follow-ups (#309). A demo is never written, so the server
// cannot remember what this visitor asked. The drawer sends the visitor's own
// completed, visible turns and the server (which already has the seeded demo
// chat) adds them to the agent's context.

export const ASK_HISTORY_MAX = 6; // caseagent.MaxHistory
export type AskHistoryTurn = { question: string; answer: string };

type Shown = { question?: string; answer?: string };

// Completed turns the visitor asked in this visit: `isLocal` selects them
// (seeded turns are the server's already). A turn with no visible question or
// answer is skipped, and a pending or failed ask is never in `chat`. Only the
// last ASK_HISTORY_MAX are sent; the server refuses more.
export function askHistory(chat: readonly Shown[], isLocal: (t: Shown) => boolean): AskHistoryTurn[] {
  const out: AskHistoryTurn[] = [];
  for (const t of chat) {
    const question = (t.question ?? "").trim();
    const answer = (t.answer ?? "").trim();
    if (isLocal(t) && question && answer) out.push({ question, answer });
  }
  return out.slice(-ASK_HISTORY_MAX);
}

// The ask request body: history only for a pinned demo and only when there is
// some; a normal case's saved chat is the server's sole source.
export function askBody(question: string, pinned: boolean, history: readonly AskHistoryTurn[]) {
  return pinned && history.length > 0 ? { question, history: history.slice(-ASK_HISTORY_MAX) } : { question };
}
