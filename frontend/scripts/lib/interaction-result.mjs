// Reporting for the interaction audit, kept free of Playwright so the exit
// decision can be tested with `node --test` without a browser or dev server.
export function reportInteractionResult(issues, log = console.log) {
  const unique = [...new Set(issues)];
  for (const issue of unique) log(issue);
  log(unique.length ? `${unique.length} issue(s)` : "No interaction issues.");
  return unique.length;
}
