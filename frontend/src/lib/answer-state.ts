// Clarification answer state, kept in step with the questions the API returns.
// Re-analysis can grow, shrink, reorder or replace the questions while the
// form stays mounted, so state is carried by question text, never by index.

export type AnswerState = { answers: string[]; unknown: boolean[] };

export function blankAnswers(qs: readonly string[]): AnswerState {
  return { answers: qs.map(() => ""), unknown: qs.map(() => false) };
}

// Map each new question to the old slot with the same text (each old slot
// used once); questions with no match start blank.
export function reconcileAnswers(prevQs: readonly string[], prev: AnswerState, nextQs: readonly string[]): AnswerState {
  const used = new Set<number>();
  const answers: string[] = [];
  const unknown: boolean[] = [];
  for (const q of nextQs) {
    let at = -1;
    for (let i = 0; i < prevQs.length; i++) {
      if (!used.has(i) && prevQs[i] === q) {
        at = i;
        break;
      }
    }
    if (at >= 0) used.add(at);
    answers.push(at >= 0 ? (prev.answers[at] ?? "") : "");
    unknown.push(at >= 0 ? (prev.unknown[at] ?? false) : false);
  }
  return { answers, unknown };
}

export function buildAnswerPayload(qs: readonly string[], s: AnswerState): { question: string; answer: string }[] {
  return qs.map((q, i) => ({ question: q, answer: s.unknown[i] ? "I don't know." : (s.answers[i] ?? "").trim() })).filter((a) => a.answer);
}
