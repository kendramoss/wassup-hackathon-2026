// Command adcvdreal writes the pool of (product, order) questions for the
// realistic AD/CVD test (D945): for every product the orders of its country
// the retrieval proposes, plus every order the plain HTS-and-country check
// matches. The pool is the same for the models that label and for the method
// that is scored, so a flag outside the pool counts as wrong and no pair is
// silently labelled outside the order.
//
//	go run ./cmd/adcvdreal -orders ../eval/adcvd/orders-2026-11-27.v2.json -queries ../eval/adcvd/real/dev/queries.jsonl -k 12 -out pairs.jsonl
//
// No model is called.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/adcvd"
)

// Query is one product: an importer's description, its origin and the HTS code.
type Query struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Origin      string `json:"origin"`
	HTS         string `json:"hts"`
}

// Pair is one question: an order group read against a product.
type Pair struct {
	Key         string   `json:"key"`
	ID          string   `json:"id"`
	Cases       []string `json:"cases"`
	Product     string   `json:"product"`
	Country     string   `json:"country"`
	Description string   `json:"description"`
	ScopeText   string   `json:"scope_text"`
	Rank        int      `json:"rank"`      // 1-based rank in the retrieval, 0 if only the plain check proposed it
	HTSMatch    bool     `json:"hts_match"` // the plain HTS-and-country check matches it
}

func readQueries(path string) ([]Query, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Query
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var q Query
		if err := json.Unmarshal(sc.Bytes(), &q); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, sc.Err()
}

// Pool returns the pairs of one query: the top k of the retrieval and the plain check's matches.
func Pool(t *adcvd.Table, q Query, k int) []Pair {
	seen := map[string]int{}
	var out []Pair
	add := func(c adcvd.Candidate, rank int, htsMatch bool) {
		id := strings.Join(c.Cases, " ")
		if i, ok := seen[id]; ok {
			if htsMatch {
				out[i].HTSMatch = true
			}
			return
		}
		seen[id] = len(out)
		out = append(out, Pair{
			Key: adcvd.PairKey(adcvd.Order{Product: c.Product, Country: c.Country, ScopeText: c.Scope}, q.Description),
			ID:  q.ID, Cases: c.Cases, Product: c.Product, Country: c.Country, Description: q.Description, ScopeText: c.Scope, Rank: rank, HTSMatch: htsMatch,
		})
	}
	for i, c := range t.Candidates(adcvd.Query{Description: q.Description, Country: q.Origin, HTS: q.HTS}, k) {
		add(c, i+1, false)
	}
	res := t.Check(adcvd.Input{Parts: []adcvd.Part{{HTS: q.HTS, Origin: q.Origin}}})
	if len(res.Parts) == 1 {
		for _, m := range res.Parts[0].Matches {
			if g, ok := t.GroupOf(m.Cases); ok {
				add(g, 0, true)
			}
		}
	}
	return out
}

func main() {
	orders := flag.String("orders", "", "orders file")
	queries := flag.String("queries", "", "queries file (id, description, origin, hts)")
	k := flag.Int("k", 12, "retrieved candidates per product")
	out := flag.String("out", "", "pairs file")
	countries := flag.String("countries", "", "read origin names (one per line) from this file and print those a country with an order covers")
	flag.Parse()
	t, err := adcvd.LoadFile(*orders)
	must(err)
	if *countries != "" {
		b, err := os.ReadFile(*countries)
		must(err)
		for _, l := range strings.Split(string(b), "\n") {
			if strings.TrimSpace(l) != "" && t.HasCountry(l) {
				fmt.Println(l)
			}
		}
		return
	}
	qs, err := readQueries(*queries)
	must(err)
	f, err := os.Create(*out)
	must(err)
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()
	n := 0
	for _, q := range qs {
		for _, p := range Pool(t, q, *k) {
			b, _ := json.Marshal(p)
			fmt.Fprintln(w, string(b))
			n++
		}
	}
	fmt.Println("pairs:", n, "queries:", len(qs))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "adcvdreal:", err)
		os.Exit(1)
	}
}
