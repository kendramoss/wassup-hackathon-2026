package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/cross"
)

// probeRow is one line of the probe index: what CROSS answered for one
// ruling number. Every probed number is listed, found or not, so the
// candidate set can be rebuilt and checked (D150).
type probeRow struct {
	Ruling     string `json:"ruling"`
	Found      bool   `json:"found"`
	Date       string `json:"date,omitempty"`
	Collection string `json:"collection,omitempty"`
	Category   string `json:"category,omitempty"`
	// Origin is true when the text names a country of origin question:
	// the CATEGORY line mentions origin, or the text says "country of origin".
	Origin bool   `json:"origin,omitempty"`
	Saved  bool   `json:"saved,omitempty"`
	Error  string `json:"error,omitempty"`
}

var (
	categoryLine = regexp.MustCompile(`CATEGORY:\s*([A-Za-z ,;&/]{1,80})`)
	originCue    = regexp.MustCompile(`(?i)count(?:r)?y of origin`)
)

// probe walks a range of ruling numbers (-prefix N, -from, -to) through
// GET /api/ruling/<id>, about one request a second, and records each answer
// in -index. Rulings that raise an origin question and are dated on or after
// -since are saved to -raw. CROSS's search endpoint now refuses scripted
// requests ("Security parameters are missing"), so new rulings are found by
// number instead: New York rulings are numbered in order of issue (D150).
// Resume-safe: numbers already in -index are skipped.
func probe(args []string) error {
	fl := flag.NewFlagSet("probe", flag.ExitOnError)
	prefix := fl.String("prefix", "N", "collection prefix: N (New York) or H (Headquarters)")
	from := fl.Int("from", 364000, "first number")
	to := fl.Int("to", 367500, "last number")
	stopAfter := fl.Int("stop-after", 400, "stop after this many numbers in a row are not found")
	since := fl.String("since", "2026-09-26", "save only rulings dated on or after this")
	rawDir := fl.String("raw", "../data/corpus/raw", "output directory for CROSS JSON")
	index := fl.String("index", "../eval/data/final2_probe.jsonl", "probe index (appended)")
	delay := fl.Duration("delay", time.Second, "pause between requests")
	_ = fl.Parse(args)
	if *prefix != "N" && *prefix != "H" {
		return fmt.Errorf("-prefix must be N or H")
	}
	seen := map[string]bool{}
	if f, err := os.Open(*index); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			var r probeRow
			if json.Unmarshal(sc.Bytes(), &r) == nil && r.Error == "" {
				seen[r.Ruling] = true
			}
		}
		f.Close()
	}
	if err := os.MkdirAll(*rawDir, 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(*index, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	client := cross.NewHTTPClient(userAgent)
	miss, found, saved := 0, 0, 0
	for n := *from; n <= *to; n++ {
		id := fmt.Sprintf("%s%06d", *prefix, n)
		if seen[id] {
			continue
		}
		r, err := fetchWithRetry(client, id)
		time.Sleep(*delay)
		row := probeRow{Ruling: id}
		switch {
		case errors.Is(err, cross.ErrNotFound):
			miss++
		case err != nil:
			row.Error = err.Error()
		default:
			miss = 0
			found++
			row.Found = true
			row.Date = r.RulingDate[:min(10, len(r.RulingDate))]
			row.Collection = strings.ToUpper(r.Collection)
			if m := categoryLine.FindStringSubmatch(r.Text[:min(800, len(r.Text))]); m != nil {
				c := m[1]
				for _, stop := range []string{"TARIFF", "  "} {
					if i := strings.Index(c, stop); i >= 0 {
						c = c[:i]
					}
				}
				row.Category = strings.TrimSpace(c)
			}
			row.Origin = strings.Contains(strings.ToLower(row.Category), "origin") || originCue.MatchString(r.Text)
			if row.Origin && row.Date >= *since {
				b, _ := json.Marshal(r)
				if err := os.WriteFile(filepath.Join(*rawDir, id+".json"), b, 0o644); err != nil {
					return err
				}
				row.Saved = true
				saved++
			}
		}
		b, _ := json.Marshal(row)
		if _, err := out.Write(append(b, '\n')); err != nil {
			return err
		}
		if (found+miss)%100 == 0 {
			fmt.Fprintf(os.Stderr, "probe %s: %d found, %d saved, %d not found in a row\n", id, found, saved, miss)
		}
		if miss >= *stopAfter {
			fmt.Fprintf(os.Stderr, "probe: %d numbers in a row not found; stopping at %s\n", miss, id)
			break
		}
	}
	fmt.Fprintf(os.Stderr, "probe: %d found, %d saved -> %s\n", found, saved, *index)
	return nil
}
