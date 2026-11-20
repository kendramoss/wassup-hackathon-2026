package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// run is main with injectable arguments and streams.
func run(args []string, out, errw io.Writer) int {
	fs := flag.NewFlagSet("claimscheck", flag.ContinueOnError)
	fs.SetOutput(errw)
	root := fs.String("root", "..", "repository root (default is right when run from backend/)")
	doc := fs.String("doc", "", "document to check (default <root>/docs/claims-reconciliation.md)")
	print := fs.Bool("print", false, "print the recomputed figure table instead of checking")
	printSums := fs.Bool("print-manifest", false, "print the checksum manifest instead of checking")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *doc == "" {
		*doc = filepath.Join(*root, "docs", "claims-reconciliation.md")
	}
	if *printSums {
		man, err := Manifest(*root)
		if err != nil {
			fmt.Fprintln(errw, "problem:", err)
			return 1
		}
		fmt.Fprint(out, RenderManifest(man))
		return 0
	}
	if *print {
		kv, problems := Compute(*root)
		fmt.Fprint(out, RenderTable(kv))
		for _, p := range problems {
			fmt.Fprintln(errw, "problem:", p)
		}
		if len(problems) > 0 {
			return 1
		}
		return 0
	}
	rep := Check(*root, *doc)
	for _, s := range rep.Passed {
		fmt.Fprintln(out, "ok:  ", s)
	}
	for _, s := range rep.Notes {
		fmt.Fprintln(out, "note:", s)
	}
	for _, s := range rep.Failures {
		fmt.Fprintln(out, "FAIL:", s)
	}
	if len(rep.Failures) > 0 {
		fmt.Fprintf(out, "claimscheck: %d check(s) failed\n", len(rep.Failures))
		return 1
	}
	fmt.Fprintln(out, "claimscheck: all checks passed")
	return 0
}
