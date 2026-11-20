// Package logsafe is a stdlib-only log sanitiser for log/slog (#285).
//
// It is a LEAF: it imports no application, model, domain or store package, so
// any of them may import it without creating a cycle. It knows nothing about
// the types it protects; errors describe themselves through two optional
// methods (LogClass and LogStatus) that the owning package may add later, and
// logsafe validates the answers against a closed set.
//
// Scope. logsafe protects LOGS, ERRORS, DEBUG and METRICS. It must never be
// applied to authorized case data, model input or requested exports, which
// stay accurate (docs/design/data-handling.md). It is not a PII detector: it
// is default-deny by closed allowlists, and it does not read or inspect free
// text to decide whether it is sensitive.
//
// Guarantees (each has a test):
//   - unknown attribute keys and group names are dropped, never printed; only
//     a constant count (masked_attrs) is kept;
//   - a value is passed only if its key has a rule and the value satisfies it;
//     otherwise the key stays and the value becomes the constant "[masked]"
//     (no length, prefix or type information);
//   - LogValuer, Stringer, struct, map and other Any values are NOT called or
//     formatted unless they are logsafe's own error value;
//   - an error is reduced to a closed class (and an optional HTTP status
//     number); Error() is never called;
//   - the record message must be in a closed, known-source set, else it is the
//     constant "[message masked]";
//   - any panic or failure while sanitising replaces the record by a constant
//     one; the original attributes are never forwarded;
//   - net/http's own error log, bridged with ErrorLog, never prints the
//     original line.
//
// Nothing here reads configuration, environment, keys or the audit setting.
package logsafe
