// Command auditverify re-checks an audit export (GET /v1/audit/export,
// JSONL) offline (#871): the summary digest and the hash chain, with the
// same rules as GET /v1/audit/verify.
//
// Usage: go run ./cmd/auditverify export.jsonl   (or - for stdin)
//
// Exit 0: intact. Exit 1: findings or a digest mismatch. Exit 2: unreadable.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
)

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, stdin io.Reader, out, errOut io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(errOut, "usage: auditverify <export.jsonl | ->")
		return 2
	}
	in := stdin
	if args[0] != "-" {
		f, err := os.Open(args[0])
		if err != nil {
			fmt.Fprintln(errOut, "auditverify:", err)
			return 2
		}
		defer f.Close()
		in = f
	}
	res, err := audit.VerifyExport(in)
	if err != nil {
		fmt.Fprintln(errOut, "auditverify:", err)
		return 2
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", " ")
	_ = enc.Encode(res)
	if !res.OK {
		return 1
	}
	return 0
}
