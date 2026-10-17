package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/cross"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// usmcaTerms are the CROSS search terms whose hits are candidate Part 102
// (19 CFR 102.11 / 102.20) rulings (D61).
var usmcaTerms = []string{"102.20", "102.11"}

// usmcaFetch pages through CROSS search results for usmcaTerms, newest first,
// down to -since, writes the union to -index and downloads every hit not
// already in -raw (resume-safe, about one request a second).
func usmcaFetch(args []string) error {
	fl := flag.NewFlagSet("usmca-fetch", flag.ExitOnError)
	rawDir := fl.String("raw", "../data/corpus/raw", "output directory for CROSS JSON")
	index := fl.String("index", "../data/corpus/usmca_index.jsonl", "search hits (one JSON object per line)")
	since := fl.String("since", "2015-01-01", "oldest ruling date to collect")
	delay := fl.Duration("delay", time.Second, "pause between requests")
	_ = fl.Parse(args)
	if _, err := time.Parse(time.DateOnly, *since); err != nil {
		return fmt.Errorf("-since: %w", err)
	}
	client := cross.NewHTTPClient(userAgent)
	hits := map[string]cross.SearchHit{}
	for _, term := range usmcaTerms {
		for page := 1; ; page++ {
			var rs []cross.SearchHit
			var err error
			for attempt := 0; attempt < 4; attempt++ {
				ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
				rs, _, err = client.SearchPage(ctx, term, page, 100)
				cancel()
				if err == nil {
					break
				}
				time.Sleep(time.Duration(5*(attempt+1)) * time.Second)
			}
			if err != nil {
				return err
			}
			time.Sleep(*delay)
			if len(rs) == 0 {
				break
			}
			oldest := "9999"
			for _, h := range rs {
				d := h.RulingDate[:min(10, len(h.RulingDate))]
				oldest = min(oldest, d)
				if d < *since || !domain.ValidRulingID(h.RulingNumber) {
					continue
				}
				hits[h.RulingNumber] = h
			}
			fmt.Fprintf(os.Stderr, "search %q page %d: oldest %s, %d candidates\n", term, page, oldest, len(hits))
			if oldest < *since {
				break
			}
		}
	}
	ids := make([]string, 0, len(hits))
	for id := range hits {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	f, err := os.Create(*index)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, id := range ids {
		b, _ := json.Marshal(hits[id])
		_, _ = w.Write(append(b, '\n'))
	}
	if err := errors.Join(w.Flush(), f.Close()); err != nil {
		return err
	}
	if err := os.MkdirAll(*rawDir, 0o755); err != nil {
		return err
	}
	var done, skipped, missing int
	for _, id := range ids {
		out := filepath.Join(*rawDir, id+".json")
		if st, err := os.Stat(out); err == nil && st.Size() > 100 {
			skipped++
			continue
		}
		if corpus.TextileTariffs(hits[id].Tariffs) {
			skipped++ // textile goods follow 102.21, not this regime
			continue
		}
		r, err := fetchWithRetry(client, id)
		if errors.Is(err, cross.ErrNotFound) {
			missing++
			continue
		}
		if err != nil {
			return err
		}
		time.Sleep(*delay)
		b, _ := json.Marshal(r)
		if err := os.WriteFile(out, b, 0o644); err != nil {
			return err
		}
		done++
		if done%50 == 0 {
			fmt.Fprintf(os.Stderr, "fetched %d\n", done)
		}
	}
	fmt.Fprintf(os.Stderr, "usmca-fetch: %d candidates in %s; %d written, %d skipped, %d missing\n", len(ids), *index, done, skipped, missing)
	return nil
}

// usmcaLabel labels the Part 102 rulings among the -index hits: those whose
// raw text applies 19 CFR 102.11 or 102.20 to a non-textile good
// (corpus.USMCARegime). Rulings already in -labels keep that row (some are
// hand-checked); the rest get a regex label (corpus.LabelHolding). Writes
// -out and the facts of every labelled ruling to -facts.
func usmcaLabel(args []string) error {
	fl := flag.NewFlagSet("usmca-label", flag.ExitOnError)
	rawDir := fl.String("raw", "../data/corpus/raw", "CROSS JSON directory")
	index := fl.String("index", "../data/corpus/usmca_index.jsonl", "search hits from usmca-fetch")
	labelsPath := fl.String("labels", "../eval/data/labels.jsonl", "existing labels (rows reused as they are)")
	out := fl.String("out", "../eval/data/labels_usmca.jsonl", "output labels")
	factsDir := fl.String("facts", "../data/corpus/facts", "facts directory to write")
	_ = fl.Parse(args)

	existing := map[string]corpus.Label{}
	labels, err := corpus.LoadLabels(*labelsPath)
	if err != nil {
		return err
	}
	for _, l := range labels {
		existing[l.Ruling] = l
	}
	ids, err := indexIDs(*index)
	if err != nil {
		return err
	}
	var rows []corpus.Label
	var reused, notPart102, noRaw int
	for _, id := range ids {
		b, err := os.ReadFile(filepath.Join(*rawDir, id+".json"))
		if errors.Is(err, os.ErrNotExist) {
			noRaw++
			continue
		}
		if err != nil {
			return err
		}
		var r cross.Ruling
		if err := json.Unmarshal(b, &r); err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		if !corpus.USMCARegime(r.Text) {
			notPart102++
			continue
		}
		facts, meta := corpus.SplitFacts(r.Text)
		if strings.TrimSpace(facts) == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(*factsDir, id+".txt"), []byte(facts+"\n"), 0o644); err != nil {
			return err
		}
		if l, ok := existing[id]; ok {
			rows = append(rows, l)
			reused++
			continue
		}
		rows = append(rows, corpus.LabelHolding(r.RulingNumber, r.RulingDate, r.Collection, r.Subject, r.Text, meta))
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Date != rows[j].Date {
			return rows[i].Date < rows[j].Date
		}
		return rows[i].Ruling < rows[j].Ruling
	})
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	withOrigin := 0
	for _, l := range rows {
		if l.Origin() != "" {
			withOrigin++
		}
		b, _ := json.Marshal(l)
		_, _ = w.Write(append(b, '\n'))
	}
	if err := errors.Join(w.Flush(), f.Close()); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "usmca-label: %d rulings (%d with a primary origin; %d reused from %s), %d not Part 102, %d without raw text -> %s\n",
		len(rows), withOrigin, reused, *labelsPath, notPart102, noRaw, *out)
	return nil
}

// indexIDs reads the ruling numbers from a usmca-fetch index.
func indexIDs(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var ids []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var h cross.SearchHit
		if err := json.Unmarshal(sc.Bytes(), &h); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		ids = append(ids, h.RulingNumber)
	}
	return ids, sc.Err()
}
