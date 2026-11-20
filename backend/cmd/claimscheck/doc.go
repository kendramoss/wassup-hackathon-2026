// Command claimscheck reconciles the numbers and code claims in
// docs/claims-reconciliation.md with the committed evaluation artifacts.
//
// It is read-only and offline: it recomputes the headline counts from the
// official result rows, the scope rows, the labelled corpora and the facts
// directory, verifies the checksums of every file it reads, and then checks
// that the document states exactly those values and that every code or test
// it cites still exists. It never writes a file and never updates a number;
// when the artifacts and the document disagree it exits non-zero.
//
// Run it from backend/:
//
//	go run ./cmd/claimscheck
//
// Flags: -root (repository root, default ".."), -doc (default
// <root>/docs/claims-reconciliation.md), -print (print the recomputed figure
// table in the document's format instead of checking) and -print-manifest
// (print the checksum manifest in the document's format). Exit status 0
// means every check passed, 1 means at least one check failed, 2 means a bad
// command line.
//
// This is a standalone command rather than part of the readiness-eval
// tooling so that it has one owner and no dependency on model, network or
// credential code. It uses only the standard library.
package main
