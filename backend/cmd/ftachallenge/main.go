// Command ftachallenge scores the frozen FTA challenge datasets (ATIGA,
// AKFTA, ACFTA and RCEP) against the production checks, without model calls.
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
)

func loadDataset(dir, feature, split string) (*Dataset, error) {
	data, err := os.ReadFile(filepath.Join(dir, feature+"-"+split+".json"))
	if err != nil {
		return nil, err
	}
	ds, err := parseDataset(data)
	if err != nil {
		return nil, fmt.Errorf("%s-%s: %w", feature, split, err)
	}
	if err := ds.Validate(); err != nil {
		return nil, fmt.Errorf("%s-%s: %w", feature, split, err)
	}
	return ds, nil
}

func loadAll(dir string) (dev, test map[string]*Dataset, err error) {
	dev, test = map[string]*Dataset{}, map[string]*Dataset{}
	for _, f := range features {
		if dev[f], err = loadDataset(dir, f, "dev"); err != nil {
			return
		}
		if test[f], err = loadDataset(dir, f, "test"); err != nil {
			return
		}
		if err = ValidateSplits(dev[f], test[f]); err != nil {
			err = fmt.Errorf("%s: %w", f, err)
			return
		}
	}
	return
}

func loadPosthoc(dir string) (map[string]*Dataset, error) {
	out := map[string]*Dataset{}
	for _, f := range features {
		if _, err := os.Stat(filepath.Join(dir, f+"-posthoc.json")); err != nil {
			continue
		}
		ds, err := loadDataset(dir, f, "posthoc")
		if err != nil {
			return nil, err
		}
		out[f] = ds
	}
	return out, nil
}

func resultsPath(dir, feature, split, tag string) string {
	name := feature + "-" + split
	if tag != "" {
		name += "-" + tag
	}
	return filepath.Join(dir, "results", name+".json")
}

func readResults(path string) (ResultsFile, error) {
	var rf ResultsFile
	b, err := os.ReadFile(path)
	if err != nil {
		return rf, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	return rf, dec.Decode(&rf)
}

func engineUnchanged(candidate string) error {
	cmd := exec.Command("git", "diff", "--quiet", candidate, "--", "backend/internal/atiga", "backend/internal/aseanfta", "backend/internal/rcep")
	cmd.Dir = filepath.Join("..")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("engine sources differ from candidate %s: %v %s", candidate, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func writeFile(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func run() error {
	dir := flag.String("dir", "../eval/challenges/fta", "challenge directory")
	mode := flag.String("mode", "run", "run, summary or verify")
	split := flag.String("split", "", "dev or test (run mode)")
	tag := flag.String("tag", "", "suffix for the results file, such as run1")
	candidate := flag.String("candidate", "", "full git SHA of the frozen candidate")
	skipGit := flag.Bool("skip-git-check", false, "do not compare engine sources with the candidate")
	flag.Parse()

	dev, test, err := loadAll(*dir)
	if err != nil {
		return err
	}
	needCandidate := func() error {
		if !sha40.MatchString(*candidate) {
			return fmt.Errorf("-candidate must be a full 40-character git SHA")
		}
		if !*skipGit {
			return engineUnchanged(*candidate)
		}
		return nil
	}
	switch *mode {
	case "run":
		if *split != "dev" && *split != "test" && *split != "posthoc" {
			return fmt.Errorf("-split must be dev, test or posthoc")
		}
		if err := needCandidate(); err != nil {
			return err
		}
		if *split == "posthoc" {
			ph, err := loadPosthoc(*dir)
			if err != nil {
				return err
			}
			for _, f := range features {
				ds, ok := ph[f]
				if !ok {
					continue
				}
				rf := NewResultsFile(ds, nil, *candidate, *tag, ScoreDataset(ds))
				b, err := marshal(rf)
				if err != nil {
					return err
				}
				if err := writeFile(resultsPath(*dir, f, "posthoc", *tag), b); err != nil {
					return err
				}
				fmt.Printf("%s posthoc: %d cases, %d matched, %d mismatched, %d errors\n", f, rf.Totals.Cases, rf.Totals.Matched, rf.Totals.Mismatched, rf.Totals.Errors)
			}
			return nil
		}
		for _, f := range features {
			ds, other := dev[f], (*Dataset)(nil)
			if *split == "test" {
				ds, other = test[f], dev[f]
			}
			rf := NewResultsFile(ds, other, *candidate, *tag, ScoreDataset(ds))
			b, err := marshal(rf)
			if err != nil {
				return err
			}
			if err := writeFile(resultsPath(*dir, f, *split, *tag), b); err != nil {
				return err
			}
			fmt.Printf("%s %s: %d cases, %d matched, %d mismatched, %d errors\n", f, *split, rf.Totals.Cases, rf.Totals.Matched, rf.Totals.Mismatched, rf.Totals.Errors)
		}
	case "summary":
		if err := needCandidate(); err != nil {
			return err
		}
		devRes, testRes, first, ph := map[string]ResultsFile{}, map[string]ResultsFile{}, map[string]ResultsFile{}, map[string]ResultsFile{}
		lat := map[string]Latency{}
		for _, f := range features {
			if devRes[f], err = readResults(resultsPath(*dir, f, "dev", "")); err != nil {
				return err
			}
			if testRes[f], err = readResults(resultsPath(*dir, f, "test", "")); err != nil {
				return err
			}
			if r, err := readResults(resultsPath(*dir, f, "dev", "run1")); err == nil {
				first[f] = r
			}
			if r, err := readResults(resultsPath(*dir, f, "posthoc", "")); err == nil {
				ph[f] = r
			}
			lat[f] = MeasureLatency(test[f], 20)
		}
		s := BuildSummary(dev, test, devRes, testRes, first, ph, lat)
		if err := ValidateSummary(s, dev, test, devRes, testRes, ph); err != nil {
			return err
		}
		b, err := marshal(s)
		if err != nil {
			return err
		}
		return writeFile(filepath.Join(*dir, "summary.json"), b)
	case "verify":
		return verify(*dir, dev, test)
	default:
		return fmt.Errorf("unknown -mode %q", *mode)
	}
	return nil
}

// verify scores both splits again and requires the committed results to
// match byte for byte, and the committed summary to match apart from latency.
func verify(dir string, dev, test map[string]*Dataset) error {
	devRes, testRes := map[string]ResultsFile{}, map[string]ResultsFile{}
	for _, f := range features {
		for _, split := range []string{"dev", "test"} {
			committed, err := readResults(resultsPath(dir, f, split, ""))
			if err != nil {
				return err
			}
			ds, other := dev[f], (*Dataset)(nil)
			if split == "test" {
				ds, other = test[f], dev[f]
			}
			rf := NewResultsFile(ds, other, committed.CandidateRevision, "", ScoreDataset(ds))
			want, err := marshal(rf)
			if err != nil {
				return err
			}
			got, err := os.ReadFile(resultsPath(dir, f, split, ""))
			if err != nil {
				return err
			}
			if err := EquivalentResults(got, want); err != nil {
				return fmt.Errorf("%s %s: committed results differ from a fresh scoring: %w", f, split, err)
			}
			if split == "dev" {
				devRes[f] = committed
			} else {
				testRes[f] = committed
			}
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, "summary.json"))
	if err != nil {
		return err
	}
	var s Summary
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return err
	}
	phSets, err := loadPosthoc(dir)
	if err != nil {
		return err
	}
	phRes := map[string]ResultsFile{}
	for f, ds := range phSets {
		committed, err := readResults(resultsPath(dir, f, "posthoc", ""))
		if err != nil {
			return err
		}
		want, err := marshal(NewResultsFile(ds, nil, committed.CandidateRevision, "", ScoreDataset(ds)))
		if err != nil {
			return err
		}
		got, err := os.ReadFile(resultsPath(dir, f, "posthoc", ""))
		if err != nil {
			return err
		}
		if err := EquivalentResults(got, want); err != nil {
			return fmt.Errorf("%s posthoc: committed results differ from a fresh scoring: %w", f, err)
		}
		phRes[f] = committed
	}
	if err := ValidateSummary(s, dev, test, devRes, testRes, phRes); err != nil {
		return err
	}
	first := map[string]ResultsFile{}
	lat := map[string]Latency{}
	for _, f := range s.Features {
		lat[f.ID] = Latency{f.LatencyP50Ms, f.LatencyP95Ms}
		if r, err := readResults(resultsPath(dir, f.ID, "dev", "run1")); err == nil {
			first[f.ID] = r
		}
	}
	want, err := marshal(BuildSummary(dev, test, devRes, testRes, first, phRes, lat))
	if err != nil {
		return err
	}
	have, err := marshal(s)
	if err != nil {
		return err
	}
	if !bytes.Equal(want, have) {
		return fmt.Errorf("committed summary differs from one rebuilt from the results")
	}
	fmt.Println("verified: results and summary reproduce (latency and last-bit RVC noise excepted)")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
