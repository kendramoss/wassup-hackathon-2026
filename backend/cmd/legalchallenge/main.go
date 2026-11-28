// Command legalchallenge scores the three rule engines of the legal-rules
// challenge study (USMCA preferential rules, 19 CFR 102.21 textile origin
// rules and tariff-watch rate changes) on frozen, source-grounded synthetic
// cases. It makes no model call and no network call.
//
//	go run ./cmd/legalchallenge validate           integrity and input validity, before a freeze
//	go run ./cmd/legalchallenge run -split dev     score the development cases
//	go run ./cmd/legalchallenge run -split test -freeze <sha>
//	                                               score the reserved cases once, at the frozen commit
//	go run ./cmd/legalchallenge summarize -candidate <sha>
//	go run ./cmd/legalchallenge check              recompute everything and compare with the committed bytes
//
// The expected labels are written beforehand by eval/challenges/legal-rules
// (a reference oracle that shares no code with the candidates). The datasets
// are synthetic, not independent real-world decisions.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func nowNs() int64 { return time.Now().UnixNano() }

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "validate":
		err = cmdValidate(os.Args[2:])
	case "run":
		err = cmdRun(os.Args[2:])
	case "summarize":
		err = cmdSummarize(os.Args[2:])
	case "check":
		err = CheckCommitted(dataRoot())
		if err == nil {
			fmt.Println("committed results and summary reproduce")
		}
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "legalchallenge:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: legalchallenge validate | run -split dev|test [-freeze sha] | summarize -candidate sha | check")
	os.Exit(2)
}

func cmdValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	min := fs.Int("min-test", 100, "least reserved test cases per feature")
	_ = fs.Parse(args)
	dev, test, srcs, err := LoadAll(dataRoot())
	if err != nil {
		return err
	}
	probs := CheckIntegrity(dev, test, srcs, IntegrityOptions{MinTest: *min, Features: featureIDs()})
	probs = append(probs, CheckSources(srcs, repoRoot())...)
	probs = append(probs, CheckInputValidity(append(append([]Case{}, dev...), test...))...)
	for _, f := range Features {
		fa := Families(byFeature(dev, f.ID), byFeature(test, f.ID))
		fmt.Printf("%-20s dev %3d  test %3d  test source families %2d (shared with dev %d)  templates %d (shared %d)\n", f.ID,
			len(byFeature(dev, f.ID)), len(byFeature(test, f.ID)), fa.TestSourceFamilies, fa.SourceFamiliesSharedWithDev, fa.TestTemplates, fa.TemplatesSharedWithDev)
	}
	if len(probs) > 0 {
		return fmt.Errorf("%d problems:\n%s", len(probs), strings.Join(probs, "\n"))
	}
	fmt.Println("datasets are well formed; every input is valid or deliberately invalid")
	return nil
}

func git(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repoRoot()}, args...)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out.String(), nil
}

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	split := fs.String("split", "", "dev or test")
	freeze := fs.String("freeze", "", "full sha of the frozen commit (required for test)")
	_ = fs.Parse(args)
	if *split != "dev" && *split != "test" {
		return fmt.Errorf("-split must be dev or test")
	}
	root := dataRoot()
	resPath := filepath.Join(root, "results-"+*split+".jsonl")
	if *split == "test" {
		if err := FrozenGuard(git, *freeze, resPath); err != nil {
			return err
		}
	}
	dev, test, srcs, err := LoadAll(root)
	if err != nil {
		return err
	}
	if p := CheckIntegrity(dev, test, srcs, IntegrityOptions{MinTest: 100, Features: featureIDs()}); len(p) > 0 {
		return fmt.Errorf("datasets are not well formed:\n%s", strings.Join(p, "\n"))
	}
	cases := dev
	if *split == "test" {
		cases = test
	}
	rows, lat := RunSplit(cases)
	var buf bytes.Buffer
	if err := WriteRows(&buf, rows); err != nil {
		return err
	}
	if err := os.WriteFile(resPath, buf.Bytes(), 0o644); err != nil {
		return err
	}
	lb, _ := json.MarshalIndent(lat, "", " ")
	if err := os.WriteFile(filepath.Join(root, "latency-"+*split+".json"), append(lb, '\n'), 0o644); err != nil {
		return err
	}
	for _, f := range Features {
		s, err := SummarizeSplit(f.ID, byFeature(cases, f.ID), rowsFor(rows, f.ID))
		if err != nil {
			return err
		}
		fmt.Printf("%-20s %s: %d cases, %d matched, %d mismatched, %d errors\n", f.ID, *split, s.Cases, s.Matched, s.Mismatched, s.Errors)
	}
	return nil
}

func cmdSummarize(args []string) error {
	fs := flag.NewFlagSet("summarize", flag.ExitOnError)
	cand := fs.String("candidate", "", "full sha of the frozen candidate commit")
	_ = fs.Parse(args)
	if !shaRE.MatchString(*cand) {
		return fmt.Errorf("-candidate must be the full 40-character commit sha")
	}
	root := dataRoot()
	lat := map[string]map[string]Latency{}
	for _, sp := range []string{"dev", "test"} {
		b, err := os.ReadFile(filepath.Join(root, "latency-"+sp+".json"))
		if err != nil {
			return err
		}
		m := map[string]Latency{}
		if err := json.Unmarshal(b, &m); err != nil {
			return err
		}
		lat[sp] = m
	}
	sum, err := BuildSummary(root, *cand, lat)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(sum, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "summary.json"), append(b, '\n'), 0o644)
}
