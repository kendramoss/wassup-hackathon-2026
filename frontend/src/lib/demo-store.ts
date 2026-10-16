// Remembers which demo a case came from, so the Plan step can reuse the demo's
// plan request (target, illustrative economics, backtest). Per-browser
// convenience only: a case without it plans from the inputs on the page.
const key = (caseId: string) => `op:demo:${caseId}`;

export function rememberDemo(caseId: string, demoId: string) {
  try {
    localStorage.setItem(key(caseId), demoId);
  } catch {}
}

export function demoFor(caseId: string): string | null {
  try {
    return localStorage.getItem(key(caseId));
  } catch {
    return null;
  }
}
