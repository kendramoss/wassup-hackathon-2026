// Package workflowcheck holds a tiny stdlib-only parser for the restricted
// YAML subset used by .github/workflows/smoke-nonprod.yml, and the tests that
// keep that workflow dispatch-only, offline, secret-free and read-only. The
// parser fails closed: anything outside the subset (tabs, anchors, flow
// collections, multi-document files) is an error, never a guess.
//
// Run: go test -race -count=1 -v ./internal/readiness/workflowcheck/
package workflowcheck
