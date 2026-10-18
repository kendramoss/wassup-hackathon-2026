// Command corpus fetches CBP rulings and builds the facts corpus.
//
//	corpus fetch   [-labels F] [-raw DIR] [-from DIR] [-delay 1s]
//	    Download every ruling in the labels file from rulings.cbp.gov into
//	    DIR/<id>.json (resume-safe, ~1 request/s). With -from, copy existing
//	    CROSS JSON files from a local directory instead of downloading.
//	corpus build   [-raw DIR] [-facts DIR]
//	    Split each ruling into its facts section (corpus.SplitFacts) and write
//	    DIR/<id>.txt plus DIR/split_meta.jsonl.
//	corpus embed   [-facts DIR] [-out FILE] [-dims 768]
//	    Embed the facts of every labelled ruling (gemini-embedding-001 via the
//	    configured LLM provider) into a vector index for hybrid retrieval.
//	corpus hts-index [-labels F] [-raw DIR] [-out FILE]
//	    List the rulings whose text states the HTS subheading CBP applied
//	    (data/hts_index.json, for POST /v1/products/hts-suggest, D64).
//	corpus compare -a DIR -b DIR
//	    Compare two facts directories (port check against the research split).
//	corpus usmca-fetch [-raw DIR] [-index F] [-since 2015-01-01]
//	    Collect the CROSS rulings that cite 19 CFR 102.20 or 102.11 (newest
//	    first, down to -since) into an index and download them (D61).
//	corpus usmca-label [-raw DIR] [-index F] [-labels F] [-out F] [-facts DIR]
//	    Label the Part 102 rulings among them (non-textile goods; regex
//	    holding as in labels.jsonl) and write their facts files (D61).
//
// Defaults assume the command runs from backend/.
package main

import (
	"bufio"
	"context"
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

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/cross"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
)

const userAgent = cross.UserAgent

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: corpus fetch|build|compare|embed|reasoning|usmca-fetch|usmca-label|hts-index [flags]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "fetch":
		err = fetch(os.Args[2:])
	case "build":
		err = build(os.Args[2:])
	case "compare":
		err = compare(os.Args[2:])
	case "embed":
		err = embed(os.Args[2:])
	case "reasoning":
		err = reasoningFile(os.Args[2:])
	case "usmca-fetch":
		err = usmcaFetch(os.Args[2:])
	case "usmca-label":
		err = usmcaLabel(os.Args[2:])
	case "hts-index":
		err = htsIndex(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "corpus:", err)
		os.Exit(1)
	}
}

func fetch(args []string) error {
	fl := flag.NewFlagSet("fetch", flag.ExitOnError)
	labelsPath := fl.String("labels", "../eval/data/labels.jsonl", "labels file listing the rulings")
	rawDir := fl.String("raw", "../data/corpus/raw", "output directory for CROSS JSON")
	fromDir := fl.String("from", "", "copy CROSS JSON from this local directory instead of downloading")
	delay := fl.Duration("delay", time.Second, "pause between downloads")
	_ = fl.Parse(args)

	labels, err := corpus.LoadLabels(*labelsPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*rawDir, 0o755); err != nil {
		return err
	}
	client := cross.NewHTTPClient(userAgent)
	var done, skipped, missing int
	for _, l := range labels {
		out := filepath.Join(*rawDir, l.Ruling+".json")
		if st, err := os.Stat(out); err == nil && st.Size() > 100 {
			skipped++
			continue
		}
		var r cross.Ruling
		if *fromDir != "" {
			b, err := os.ReadFile(filepath.Join(*fromDir, l.Ruling+".json"))
			if errors.Is(err, fs.ErrNotExist) {
				missing++
				continue
			}
			if err != nil {
				return err
			}
			if err := json.Unmarshal(b, &r); err != nil {
				return fmt.Errorf("%s: %w", l.Ruling, err)
			}
		} else {
			r, err = fetchWithRetry(client, l.Ruling)
			if errors.Is(err, cross.ErrNotFound) {
				missing++
				continue
			}
			if err != nil {
				return err
			}
			time.Sleep(*delay)
		}
		b, _ := json.Marshal(r)
		if err := os.WriteFile(out, b, 0o644); err != nil {
			return err
		}
		done++
		if done%100 == 0 {
			fmt.Fprintf(os.Stderr, "fetched %d\n", done)
		}
	}
	fmt.Fprintf(os.Stderr, "fetch: %d written, %d already present, %d missing\n", done, skipped, missing)
	return nil
}

func fetchWithRetry(c *cross.HTTPClient, id string) (cross.Ruling, error) {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		r, err := c.Ruling(ctx, id)
		cancel()
		if err == nil || errors.Is(err, cross.ErrNotFound) {
			return r, err
		}
		lastErr = err
		time.Sleep(time.Duration(5*(attempt+1)) * time.Second)
	}
	return cross.Ruling{}, lastErr
}

func build(args []string) error {
	fl := flag.NewFlagSet("build", flag.ExitOnError)
	rawDir := fl.String("raw", "../data/corpus/raw", "CROSS JSON directory")
	factsDir := fl.String("facts", "../data/corpus/facts", "output directory for facts files")
	_ = fl.Parse(args)

	files, err := filepath.Glob(filepath.Join(*rawDir, "*.json"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	if err := os.MkdirAll(*factsDir, 0o755); err != nil {
		return err
	}
	metaF, err := os.Create(filepath.Join(*factsDir, "split_meta.jsonl"))
	if err != nil {
		return err
	}
	defer metaF.Close()
	w := bufio.NewWriter(metaF)
	defer w.Flush()
	for _, fn := range files {
		b, err := os.ReadFile(fn)
		if err != nil {
			return err
		}
		var r cross.Ruling
		if err := json.Unmarshal(b, &r); err != nil {
			return fmt.Errorf("%s: %w", fn, err)
		}
		facts, meta := corpus.SplitFacts(r.Text)
		// Trailing newline as in the research facts files, so prompts match.
		if err := os.WriteFile(filepath.Join(*factsDir, r.RulingNumber+".txt"), []byte(facts+"\n"), 0o644); err != nil {
			return err
		}
		line, _ := json.Marshal(struct {
			Ruling string `json:"ruling"`
			corpus.SplitMeta
		}{r.RulingNumber, meta})
		_, _ = w.Write(append(line, '\n'))
	}
	fmt.Fprintf(os.Stderr, "build: %d facts files in %s\n", len(files), *factsDir)
	return nil
}

func compare(args []string) error {
	fl := flag.NewFlagSet("compare", flag.ExitOnError)
	a := fl.String("a", "../data/corpus/facts", "first facts directory")
	b := fl.String("b", "", "second facts directory")
	_ = fl.Parse(args)
	if *b == "" {
		return errors.New("compare: -b is required")
	}
	files, err := filepath.Glob(filepath.Join(*a, "*.txt"))
	if err != nil {
		return err
	}
	var same, differ, onlyA int
	var examples []string
	for _, fa := range files {
		name := filepath.Base(fa)
		ba, err := os.ReadFile(fa)
		if err != nil {
			return err
		}
		bb, err := os.ReadFile(filepath.Join(*b, name))
		if errors.Is(err, fs.ErrNotExist) {
			onlyA++
			continue
		}
		if err != nil {
			return err
		}
		if string(ba) == string(bb) {
			same++
			continue
		}
		differ++
		if len(examples) < 10 {
			examples = append(examples, fmt.Sprintf("%s (%d vs %d bytes, first difference at byte %d)",
				strings.TrimSuffix(name, ".txt"), len(ba), len(bb), firstDiff(string(ba), string(bb))))
		}
	}
	fmt.Printf("compare: %d identical, %d different, %d only in %s\n", same, differ, onlyA, *a)
	for _, e := range examples {
		fmt.Println("  differs:", e)
	}
	return nil
}

func firstDiff(a, b string) int {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

func embed(args []string) error {
	fl := flag.NewFlagSet("embed", flag.ExitOnError)
	labelsPath := fl.String("labels", "../eval/data/labels.jsonl", "labels file")
	factsDir := fl.String("facts", "../data/corpus/facts", "facts directory")
	out := fl.String("out", "../data/corpus/embeddings-768.bin", "output index")
	model := fl.String("model", "gemini-embedding-001", "embedding model")
	dims := fl.Int("dims", 768, "output dimensions")
	batch := fl.Int("batch", 50, "texts per request")
	_ = fl.Parse(args)
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	var emb llm.Embedder
	switch cfg.LLMProvider {
	case config.ProviderVertex:
		v, err := llm.NewVertex(context.Background(), llm.VertexOptions{Project: cfg.GCPProject, Location: cfg.GCPLocation, Timeout: cfg.LLMTimeout})
		if err != nil {
			return err
		}
		emb = v
	default:
		return fmt.Errorf("embed needs vertex, got %q", cfg.LLMProvider)
	}
	labels, err := corpus.LoadLabels(*labelsPath)
	if err != nil {
		return err
	}
	type item struct{ id, text string }
	var items []item
	for _, l := range labels {
		b, err := os.ReadFile(filepath.Join(*factsDir, l.Ruling+".txt"))
		if err != nil {
			continue
		}
		items = append(items, item{l.Ruling, retrieval.EmbedText(string(b))})
	}
	idx := retrieval.NewVectors(*model, *dims)
	tokens := 0
	for i := 0; i < len(items); i += *batch {
		end := min(i+*batch, len(items))
		texts := make([]string, end-i)
		for j := range texts {
			texts[j] = items[i+j].text
		}
		var vecs [][]float32
		var u llm.Usage
		for attempt := 0; ; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			vecs, u, err = emb.Embed(ctx, *model, texts, *dims)
			cancel()
			if err == nil || attempt == 3 {
				break
			}
			time.Sleep(time.Duration(5*(attempt+1)) * time.Second)
		}
		if err != nil {
			return fmt.Errorf("embed batch at %d: %w", i, err)
		}
		tokens += u.InputTokens
		for j, v := range vecs {
			idx.Add(items[i+j].id, retrieval.TextHash(items[i+j].text), v)
		}
		fmt.Fprintf(os.Stderr, "embedded %d/%d\n", end, len(items))
	}
	if err := idx.Save(*out); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "embed: %d vectors (%d dims, %d input tokens) -> %s\n", idx.Len(), *dims, tokens, *out)
	return nil
}
