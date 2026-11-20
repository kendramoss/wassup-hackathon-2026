// Package smokecheck is the offline half of the #276 manual smoke. Its tests
// start the real httpapi handler on a loopback listener, wire the real
// familycheck.Checker to a scripted llm.Fake, and exercise POST
// /v1/products/family-check over real HTTP: a supported family, "none", an
// invented family, malformed model output, a provider error, and malformed
// requests. The tests make no connection except to that loopback listener, read no
// credentials and call no model. They do not prove native Vertex behaviour.
//
// Run: go test -race -count=1 -v ./internal/readiness/smokecheck/
package smokecheck
