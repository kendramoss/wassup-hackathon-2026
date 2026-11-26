package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/cross"
)

// refreshItem is one origin ruling a refresh found, with whether it passed
// the checks that let it into a later test set or the precedent pool.
type refreshItem struct {
	Ruling  string   `json:"ruling"`
	Date    string   `json:"date"`
	Subject string   `json:"subject,omitempty"`
	Valid   bool     `json:"valid"`
	Reasons []string `json:"reasons,omitempty"`
}

// refreshReport is what one refresh did (D931): the ranges it walked, what
// CROSS answered, and the new origin rulings with their checks.
type refreshReport struct {
	Date        string        `json:"date"`
	StartedUTC  string        `json:"started_utc"`
	FinishedUTC string        `json:"finished_utc"`
	Previous    refreshPrior  `json:"previous"`
	Ranges      []string      `json:"ranges"`
	Tried       int           `json:"tried"`
	Found       int           `json:"found"`
	Errors      int           `json:"errors"`
	Origin      int           `json:"origin"`
	Saved       int           `json:"saved"`
	Valid       int           `json:"valid"`
	DateMin     string        `json:"date_min,omitempty"`
	DateMax     string        `json:"date_max,omitempty"`
	Items       []refreshItem `json:"items"`
}

// refreshPrior is where the earlier probe indexes left off.
type refreshPrior struct {
	Indexes  []string `json:"indexes"`
	LastN    string   `json:"last_n,omitempty"`
	LastH    string   `json:"last_h,omitempty"`
	LastDate string   `json:"last_date,omitempty"`
}

// lastProbed returns the highest New York and Headquarters numbers found in
// the probe rows, and the latest ruling date among them.
func lastProbed(rows []probeRow) (n, h int, date string) {
	for _, r := range rows {
		if !r.Found || len(r.Ruling) < 2 {
			continue
		}
		v, err := strconv.Atoi(r.Ruling[1:])
		if err != nil {
			continue
		}
		switch r.Ruling[0] {
		case 'N':
			n = max(n, v)
		case 'H':
			h = max(h, v)
		}
		if r.Date > date {
			date = r.Date
		}
	}
	return n, h, date
}

func readProbeRows(paths []string) ([]probeRow, error) {
	var rows []probeRow
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			var r probeRow
			if json.Unmarshal(sc.Bytes(), &r) == nil {
				rows = append(rows, r)
			}
		}
		f.Close()
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

// checkSaved checks every saved ruling of rows: the raw file reads as a CROSS
// ruling with the same number and a date, the ruling is not already labelled,
// and its text has a facts section.
func checkSaved(rows []probeRow, rawDir string, known map[string]bool) []refreshItem {
	var items []refreshItem
	for _, r := range rows {
		if !r.Saved {
			continue
		}
		it := refreshItem{Ruling: r.Ruling, Date: r.Date}
		b, err := os.ReadFile(filepath.Join(rawDir, r.Ruling+".json"))
		var cr cross.Ruling
		switch {
		case err != nil:
			it.Reasons = append(it.Reasons, "raw file missing")
		case json.Unmarshal(b, &cr) != nil:
			it.Reasons = append(it.Reasons, "raw file is not CROSS JSON")
		default:
			it.Subject = strings.TrimSpace(cr.Subject)
			if !strings.EqualFold(strings.ReplaceAll(cr.RulingNumber, " ", ""), r.Ruling) {
				it.Reasons = append(it.Reasons, "ruling number differs: "+cr.RulingNumber)
			}
			if _, err := cr.Date(); err != nil {
				it.Reasons = append(it.Reasons, "no ruling date")
			}
			if facts, _ := corpus.SplitFacts(cr.Text); strings.TrimSpace(facts) == "" {
				it.Reasons = append(it.Reasons, "no facts section")
			}
		}
		if known[r.Ruling] {
			it.Reasons = append(it.Reasons, "already labelled")
		}
		it.Valid = len(it.Reasons) == 0
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Ruling < items[j].Ruling })
	return items
}

func summarise(rep *refreshReport, rows []probeRow, items []refreshItem) {
	for _, r := range rows {
		rep.Tried++
		if r.Error != "" {
			rep.Errors++
		}
		if r.Found {
			rep.Found++
		}
		if r.Origin {
			rep.Origin++
		}
		if r.Saved {
			rep.Saved++
		}
	}
	rep.Items = items
	if rep.Items == nil {
		rep.Items = []refreshItem{}
	}
	for _, it := range items {
		if !it.Valid {
			continue
		}
		rep.Valid++
		if rep.DateMin == "" || it.Date < rep.DateMin {
			rep.DateMin = it.Date
		}
		if it.Date > rep.DateMax {
			rep.DateMax = it.Date
		}
	}
}

// refresh is the scheduled corpus refresh (D931, #883). It reads every earlier
// probe index, walks the New York and Headquarters ruling numbers after the
// highest ones found, saves the new origin rulings, checks them and writes
// <out>/cbp_probe.jsonl and <out>/cbp_report.json. Resume-safe: numbers already
// in this run's index are skipped, so an interrupted run is finished by
// running it again with the same -date.
func refresh(args []string) error {
	fl := flag.NewFlagSet("refresh", flag.ExitOnError)
	date := fl.String("date", time.Now().UTC().Format("2006-01-02"), "run date; names the output directory")
	outRoot := fl.String("out-root", "../eval/refresh", "directory holding one sub-directory per run")
	previous := fl.String("previous", "../eval/data/final2_probe.jsonl", "earlier probe indexes (comma-separated); every ../eval/refresh/*/cbp_probe.jsonl except this run's is added")
	labelFiles := fl.String("labels", "../eval/data/labels.jsonl,../eval/data/labels_usmca.jsonl,../eval/data/labels_final2.jsonl,../eval/data/labels_usmca_final2.jsonl", "labels whose rulings are already known (comma-separated)")
	rawDir := fl.String("raw", "../data/corpus/raw", "where new CROSS JSON is saved")
	window := fl.Int("window", 4000, "most numbers to walk past the last one found, per collection")
	stopAfter := fl.Int("stop-after", 400, "stop a collection after this many numbers in a row are not found")
	since := fl.String("since", "2000-01-01", "save only origin rulings dated on or after this (default: every new one)")
	delay := fl.Duration("delay", time.Second, "pause between requests")
	recheck := fl.Int("recheck", 1500, "also walk again the numbers among the last N below the highest found that no earlier index found: CBP posts rulings in batches, after their number (0 = off)")
	noFetch := fl.Bool("no-fetch", false, "only check and report what this run's index already holds")
	_ = fl.Parse(args)

	out := filepath.Join(*outRoot, *date)
	index := filepath.Join(out, "cbp_probe.jsonl")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	var prevPaths []string
	for _, p := range strings.Split(*previous, ",") {
		if p = strings.TrimSpace(p); p != "" {
			prevPaths = append(prevPaths, p)
		}
	}
	more, _ := filepath.Glob(filepath.Join(*outRoot, "*", "cbp_probe.jsonl"))
	for _, p := range more {
		if filepath.Clean(p) != filepath.Clean(index) {
			prevPaths = append(prevPaths, p)
		}
	}
	sort.Strings(prevPaths)
	prev, err := readProbeRows(prevPaths)
	if err != nil {
		return err
	}
	lastN, lastH, lastDate := lastProbed(prev)
	if lastN == 0 {
		return errors.New("refresh: no earlier New York ruling found in -previous; cannot tell where to start")
	}
	rep := refreshReport{Date: *date, StartedUTC: time.Now().UTC().Format(time.RFC3339),
		Previous: refreshPrior{Indexes: prevPaths, LastN: fmt.Sprintf("N%06d", lastN), LastDate: lastDate}}
	if lastH > 0 {
		rep.Previous.LastH = fmt.Sprintf("H%06d", lastH)
	}
	type span struct {
		prefix   string
		from, to int
	}
	spans := []span{{"N", lastN + 1, lastN + *window}}
	if lastH > 0 {
		spans = append(spans, span{"H", lastH + 1, lastH + *window})
	}
	if *recheck > 0 && !*noFetch {
		// Numbers an earlier index found are skipped; the rest of the window
		// below the highest found number is walked again.
		tmp, err := os.CreateTemp("", "refresh-found-*.jsonl")
		if err != nil {
			return err
		}
		defer os.Remove(tmp.Name())
		for _, r := range prev {
			if r.Found {
				b, _ := json.Marshal(r)
				tmp.Write(append(b, '\n'))
			}
		}
		tmp.Close()
		for _, s := range []span{{"N", lastN - *recheck + 1, lastN}, {"H", lastH - *recheck + 1, lastH}} {
			if s.to <= 0 {
				continue
			}
			rep.Ranges = append(rep.Ranges, fmt.Sprintf("%s%06d-%s%06d (recheck)", s.prefix, max(s.from, 1), s.prefix, s.to))
			if err := probe([]string{"-prefix", s.prefix, "-from", strconv.Itoa(max(s.from, 1)), "-to", strconv.Itoa(s.to),
				"-stop-after", strconv.Itoa(*recheck + 1), "-since", *since, "-raw", *rawDir, "-index", index,
				"-delay", delay.String(), "-skip-found", tmp.Name()}); err != nil {
				return err
			}
		}
	}
	for _, s := range spans {
		rep.Ranges = append(rep.Ranges, fmt.Sprintf("%s%06d-%s%06d", s.prefix, s.from, s.prefix, s.to))
		if *noFetch {
			continue
		}
		err := probe([]string{"-prefix", s.prefix, "-from", strconv.Itoa(s.from), "-to", strconv.Itoa(s.to),
			"-stop-after", strconv.Itoa(*stopAfter), "-since", *since, "-raw", *rawDir, "-index", index,
			"-delay", delay.String()})
		if err != nil {
			return err
		}
	}
	rows, err := readProbeRows([]string{index})
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	known := map[string]bool{}
	for _, p := range strings.Split(*labelFiles, ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		ls, err := corpus.LoadLabels(p)
		if err != nil {
			return err
		}
		for _, l := range ls {
			known[l.Ruling] = true
		}
	}
	summarise(&rep, rows, checkSaved(rows, *rawDir, known))
	rep.FinishedUTC = time.Now().UTC().Format(time.RFC3339)
	b, err := json.MarshalIndent(rep, "", " ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "cbp_report.json"), append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "refresh: %d numbers tried, %d found, %d new origin rulings saved, %d pass the checks -> %s\n",
		rep.Tried, rep.Found, rep.Saved, rep.Valid, filepath.Join(out, "cbp_report.json"))
	return nil
}
