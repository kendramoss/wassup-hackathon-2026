package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/cross"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// supersessionPool is one labels file and the name its rulings carry in the
// map's "pools" field.
type supersessionPool struct{ name, path string }

// supersession writes the map of revoked and modified rulings (D320). It
// reads every ruling's subject from the committed labels files and, when the
// raw CROSS texts are present, the full text (the committed facts files are
// cut before CBP's analysis, so they never hold the EFFECT ON OTHER RULINGS
// section). With -check it rebuilds the map from the same inputs and fails
// when the committed file differs; without the raw texts it checks only the
// edges the committed subject lines prove.
func supersession(args []string) error {
	fl := flag.NewFlagSet("supersession", flag.ExitOnError)
	rawDir := fl.String("raw", "../data/corpus/raw", "raw CROSS JSON (optional; subject lines only without it)")
	out := fl.String("out", "internal/domain/supersession.json", "map file (embedded by internal/domain)")
	check := fl.Bool("check", false, "verify the committed map instead of writing it")
	notices := fl.String("notices", bulletinNotices, "Customs Bulletin notice excerpts (D400)")
	reads := fl.String("reads", bulletinReads, "model reads of flagged notices (D400)")
	_ = fl.Parse(args)
	pools := []supersessionPool{
		{"general", "data/labels.jsonl"},
		{"usmca", "data/labels_usmca.jsonl"},
		{"final2", "../eval/data/labels_final2.jsonl"},
		{"final2", "../eval/data/labels_usmca_final2.jsonl"},
	}
	var labels []corpus.Label
	poolOf := map[string][]string{}
	dateOf := map[string]time.Time{}
	for _, p := range pools {
		ls, err := corpus.LoadLabels(p.path)
		if err != nil {
			return err
		}
		for _, l := range ls {
			if !contains(poolOf[l.Ruling], p.name) {
				poolOf[l.Ruling] = append(poolOf[l.Ruling], p.name)
			}
			if _, ok := dateOf[l.Ruling]; ok {
				continue
			}
			d, _ := l.Time()
			dateOf[l.Ruling] = d
			labels = append(labels, l)
		}
	}
	// Raw texts CBP returned that no labels file lists (fetched for the
	// Part 102 index but not labelled) are read too: their revocation
	// letters can name labelled rulings.
	var extra []cross.Ruling
	if ents, err := os.ReadDir(*rawDir); err == nil {
		for _, e := range ents {
			id, ok := strings.CutSuffix(e.Name(), ".json")
			if !ok || dateOf[id] != (time.Time{}) || !domain.ValidRulingID(id) {
				continue
			}
			var r cross.Ruling
			b, err := os.ReadFile(filepath.Join(*rawDir, e.Name()))
			if err != nil {
				return err
			}
			if err := json.Unmarshal(b, &r); err != nil {
				return fmt.Errorf("parse %s: %w", e.Name(), err)
			}
			d, err := r.Date()
			if err != nil {
				return fmt.Errorf("%s: %w", e.Name(), err)
			}
			dateOf[id] = d
			extra = append(extra, r)
		}
	}
	lookup := func(id string) (time.Time, bool) { d, ok := dateOf[id]; return d, ok }

	var edges []domain.Supersession
	withText, subjectOnly := 0, 0
	for _, l := range labels {
		in := corpus.SupersessionInput{ID: l.Ruling, Subject: l.Subject}
		in.Date, _ = l.Time()
		b, err := os.ReadFile(filepath.Join(*rawDir, l.Ruling+".json"))
		switch {
		case err == nil:
			var doc struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(b, &doc); err != nil {
				return fmt.Errorf("parse %s: %w", l.Ruling, err)
			}
			in.Text = doc.Text
			withText++
		case errors.Is(err, fs.ErrNotExist):
			subjectOnly++
		default:
			return fmt.Errorf("read %s: %w", l.Ruling, err)
		}
		edges = append(edges, corpus.DetectSupersession(in, lookup)...)
	}
	for _, r := range extra {
		d, _ := r.Date()
		edges = append(edges, corpus.DetectSupersession(corpus.SupersessionInput{ID: r.RulingNumber, Date: d, Subject: r.Subject, Text: r.Text}, lookup)...)
	}
	edges = dedupeEdges(edges)
	// Without the raw texts only subject lines are read: check that those
	// edges are in the committed map, then rebuild the Customs Bulletin part
	// on top of the committed corpus edges and compare it in full.
	noRaw := *check && withText == 0
	var cm domain.SupersessionMap
	if noRaw {
		committed, err := os.ReadFile(*out)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(committed, &cm); err != nil {
			return err
		}
		var base []domain.Supersession
		inMap := map[string]bool{}
		for _, e := range cm.Edges {
			if e.Source == domain.SourceCustomsBulletin {
				continue
			}
			inMap[e.Ruling+"|"+e.By] = true
			e.Bulletin = nil
			base = append(base, e)
		}
		for _, e := range edges {
			if !inMap[e.Ruling+"|"+e.By] {
				return fmt.Errorf("%s lacks %s superseded by %s (subject line of %s)", *out, e.Ruling, e.By, e.SourceRuling)
			}
		}
		fmt.Printf("supersession: no raw texts; the %d subject-line edges are all in %s\n", len(edges), *out)
		edges = base
	}
	// Customs Bulletin notices (D400): a final change the corpus already
	// holds adds its citation to the corpus edge; any other becomes a new
	// edge dated by its effective date.
	bEdges, proposed, bStats, err := bulletinMerge(*notices, *reads)
	if err != nil {
		return err
	}
	corpus.SortSupersessions(bEdges)
	have := map[string]int{}
	for i, e := range edges {
		have[e.Ruling+"|"+e.By] = i
	}
	for _, b := range bEdges {
		if i, ok := have[b.Ruling+"|"+b.By]; ok {
			edges[i].Bulletin = b.Bulletin
			bStats["bulletin_edges_confirming_corpus"]++
			continue
		}
		edges = append(edges, b)
		bStats["bulletin_edges_new"]++
	}
	kept := proposed[:0]
	for _, p := range proposed {
		if _, ok := have[p.Ruling+"|"+p.By]; ok {
			continue // the corpus holds the final letter
		}
		p.Pools = poolOf[p.Ruling]
		kept = append(kept, p)
	}
	proposed = kept
	sort.Slice(proposed, func(i, j int) bool {
		a, b := proposed[i], proposed[j]
		if a.Ruling != b.Ruling {
			return a.Ruling < b.Ruling
		}
		return a.By < b.By
	})
	for i := range edges {
		edges[i].Pools = poolOf[edges[i].Ruling]
	}
	corpus.SortSupersessions(edges)

	m := domain.SupersessionMap{Version: corpus.SupersessionVersion, Rule: corpus.SupersessionRule, Edges: edges, Proposed: proposed}
	m.Counts = supersessionCounts(edges, len(labels)+len(extra), withText+len(extra))
	for k, v := range bStats {
		m.Counts[k] = v
	}
	m.Counts["proposed"] = len(proposed)
	m.Counts["proposed_in_pool"] = 0
	for _, p := range proposed {
		if len(p.Pools) > 0 {
			m.Counts["proposed_in_pool"]++
		}
	}
	b, _ := json.MarshalIndent(m, "", " ")
	b = append(b, '\n')

	if !*check {
		fmt.Printf("supersession: %d rulings read (%d with full text, %d subject only), %d edges (%d new from %d Customs Bulletin notices), %d proposed, %d rulings in a precedent pool superseded\n",
			len(labels)+len(extra), withText+len(extra), subjectOnly, len(edges), bStats["bulletin_edges_new"], bStats["bulletin_notices"], len(proposed), m.Counts["superseded_in_pool"])
		return os.WriteFile(*out, b, 0o644)
	}
	if !noRaw {
		committed, err := os.ReadFile(*out)
		if err != nil {
			return err
		}
		if !bytes.Equal(committed, b) {
			return fmt.Errorf("%s differs from a rebuild from the raw texts; run go run ./cmd/corpus supersession", *out)
		}
		fmt.Printf("supersession: %s matches a rebuild (%d edges, %d proposed)\n", *out, len(edges), len(proposed))
		return nil
	}
	// Counts that need the raw texts (rulings read, with full text) are not
	// rebuilt here; the edges, the proposals and every count derived from
	// them are.
	e1, _ := json.Marshal(m.Edges)
	e2, _ := json.Marshal(cm.Edges)
	p1, _ := json.Marshal(m.Proposed)
	p2, _ := json.Marshal(cm.Proposed)
	if !bytes.Equal(e1, e2) || !bytes.Equal(p1, p2) {
		return fmt.Errorf("%s: edges or proposals differ from the committed corpus edges plus the Customs Bulletin notices; run go run ./cmd/corpus supersession with the raw texts", *out)
	}
	for k, v := range m.Counts {
		if k == "rulings_read" || k == "rulings_with_full_text" {
			continue
		}
		if cm.Counts[k] != v {
			return fmt.Errorf("%s: count %s is %d, a rebuild gives %d", *out, k, cm.Counts[k], v)
		}
	}
	fmt.Printf("supersession: the Customs Bulletin part of %s matches a rebuild (%d edges, %d proposed)\n", *out, len(edges), len(proposed))
	return nil
}

func supersessionCounts(edges []domain.Supersession, read, withText int) map[string]int {
	c := map[string]int{"rulings_read": read, "rulings_with_full_text": withText, "edges": len(edges)}
	superseded := map[string]bool{}
	inPool := map[string]bool{}
	perPool := map[string]bool{}
	by := map[string]bool{}
	for _, e := range edges {
		c["edges_"+e.Kind]++
		c["source_"+e.Source]++
		superseded[e.Ruling] = true
		by[e.By] = true
		if len(e.Pools) > 0 {
			inPool[e.Ruling] = true
			for _, p := range e.Pools {
				if !perPool[p+"|"+e.Ruling] {
					perPool[p+"|"+e.Ruling] = true
					c["superseded_in_pool_"+p]++
				}
			}
		}
	}
	c["superseded_rulings"] = len(superseded)
	c["superseding_rulings"] = len(by)
	c["superseded_in_pool"] = len(inPool)
	return c
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// dedupeEdges keeps one edge per (ruling, superseding ruling): the first
// read, which is the superseding ruling's own text when it is in the corpus.
func dedupeEdges(es []domain.Supersession) []domain.Supersession {
	seen := map[string]bool{}
	out := es[:0]
	for _, e := range es {
		k := e.Ruling + "|" + e.By
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, e)
	}
	return out
}
