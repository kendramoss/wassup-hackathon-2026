package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/bulletin"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// bulletinIssue is one row of eval/data/bulletins/index.jsonl: a fetched
// Customs Bulletin and the hash of the PDF the notices were read from.
type bulletinIssue struct {
	bulletin.Issue
	SHA256 string `json:"sha256"`
	Pages  int    `json:"pages"`
}

func (i bulletinIssue) base() string { return fmt.Sprintf("v%dn%d", i.Volume, i.Number) }

const (
	bulletinReads   = "../eval/data/bulletins/model_reads.jsonl"
	bulletinIndex   = "../eval/data/bulletins/index.jsonl"
	bulletinNotices = "../eval/data/bulletins/notices.jsonl"
	bulletinDir     = "../data/corpus/bulletins"
)

// bulletins fetches Customs Bulletin PDFs and cuts their text into
// ruling-letter notices (D400).
//
//	corpus bulletins fetch   [-index F] [-dir DIR] [-delay 1s]
//	    Download each indexed PDF to DIR/pdf/v<vol>n<no>.pdf (skips files
//	    present; checks the hash). Then convert with
//	    python3 eval/bulletins/pdftext.py DIR (needs pypdf).
//	corpus bulletins extract [-index F] [-dir DIR] [-out F] [-check]
//	    Cut DIR/text/*.txt into notices and write the committed excerpts.
//	corpus bulletins parse   [-notices F]
//	    Print each notice's parse and any problem (no files written).
//	corpus bulletins model   [-notices F] [-out F] [-model M] [-cap USD]
//	    Send each flagged notice to the model once (D400) and append its
//	    reply to the committed reads. LITELLM_BASE_URL, LITELLM_API_KEY.
func bulletins(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: corpus bulletins fetch|extract [flags]")
	}
	switch args[0] {
	case "fetch":
		return bulletinsFetch(args[1:])
	case "extract":
		return bulletinsExtract(args[1:])
	case "parse":
		return bulletinsParse(args[1:])
	case "model":
		return bulletinsModel(args[1:])
	}
	return fmt.Errorf("unknown bulletins command %q", args[0])
}

func loadBulletinIndex(path string) ([]bulletinIssue, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []bulletinIssue
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var i bulletinIssue
		if err := json.Unmarshal(sc.Bytes(), &i); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, i)
	}
	return out, sc.Err()
}

func bulletinsFetch(args []string) error {
	fl := flag.NewFlagSet("bulletins fetch", flag.ExitOnError)
	index := fl.String("index", bulletinIndex, "bulletin index")
	dir := fl.String("dir", bulletinDir, "output directory (pdf/ and text/)")
	delay := fl.Duration("delay", time.Second, "pause between downloads")
	_ = fl.Parse(args)
	issues, err := loadBulletinIndex(*index)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(*dir, "pdf"), 0o755); err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	got, failed := 0, 0
	for _, is := range issues {
		path := filepath.Join(*dir, "pdf", is.base()+".pdf")
		if b, err := os.ReadFile(path); err == nil {
			if sum := sha256.Sum256(b); is.SHA256 != "" && hex.EncodeToString(sum[:]) != is.SHA256 {
				return fmt.Errorf("%s: hash differs from the index", path)
			}
			continue
		}
		req, _ := http.NewRequest(http.MethodGet, is.URL, nil)
		req.Header.Set("User-Agent", "unde-labs-corpus/1 (research; one request per second)")
		resp, err := client.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", is.base(), err)
			failed++
			time.Sleep(*delay)
			continue
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK || !bytes.HasPrefix(b, []byte("%PDF")) {
			fmt.Fprintf(os.Stderr, "%s: HTTP %d, not a PDF (%v)\n", is.base(), resp.StatusCode, err)
			failed++
			time.Sleep(*delay)
			continue
		}
		if sum := sha256.Sum256(b); is.SHA256 != "" && hex.EncodeToString(sum[:]) != is.SHA256 {
			fmt.Fprintf(os.Stderr, "%s: CBP's file differs from the indexed one (re-published?)\n", is.base())
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return err
		}
		got++
		time.Sleep(*delay)
	}
	fmt.Printf("bulletins: %d indexed, %d downloaded, %d failed\n", len(issues), got, failed)
	return nil
}

func bulletinsExtract(args []string) error {
	fl := flag.NewFlagSet("bulletins extract", flag.ExitOnError)
	index := fl.String("index", bulletinIndex, "bulletin index")
	dir := fl.String("dir", bulletinDir, "directory with text/v<vol>n<no>.txt")
	out := fl.String("out", bulletinNotices, "notice excerpts (committed)")
	check := fl.Bool("check", false, "verify the committed excerpts instead of writing them")
	_ = fl.Parse(args)
	issues, err := loadBulletinIndex(*index)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	n := 0
	for _, is := range issues {
		raw, err := os.ReadFile(filepath.Join(*dir, "text", is.base()+".txt"))
		if errors.Is(err, fs.ErrNotExist) && *check {
			fmt.Printf("bulletins: no text for %s; nothing to check\n", is.base())
			return nil
		}
		if err != nil {
			return err
		}
		for _, nt := range bulletin.Extract(is.Issue, string(raw)) {
			b, _ := json.Marshal(nt)
			buf.Write(b)
			buf.WriteByte('\n')
			n++
		}
	}
	if *check {
		committed, err := os.ReadFile(*out)
		if err != nil {
			return err
		}
		if !bytes.Equal(committed, buf.Bytes()) {
			return fmt.Errorf("%s differs from a rebuild from the bulletin texts; run go run ./cmd/corpus bulletins extract", *out)
		}
		fmt.Printf("bulletins: %s matches a rebuild (%d notices)\n", *out, n)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	fmt.Printf("bulletins: %d issues, %d ruling-letter notices\n", len(issues), n)
	return os.WriteFile(*out, buf.Bytes(), 0o644)
}

// loadNotices reads the committed notice excerpts.
func loadNotices(path string) ([]bulletin.Notice, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []bulletin.Notice
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	for sc.Scan() {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var n bulletin.Notice
		if err := json.Unmarshal(sc.Bytes(), &n); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, n)
	}
	return out, sc.Err()
}

func bulletinsParse(args []string) error {
	fl := flag.NewFlagSet("bulletins parse", flag.ExitOnError)
	path := fl.String("notices", bulletinNotices, "notice excerpts")
	_ = fl.Parse(args)
	notices, err := loadNotices(*path)
	if err != nil {
		return err
	}
	bad := 0
	for _, n := range notices {
		r := bulletin.Parse(n)
		p := r.Problem()
		if p != "" {
			bad++
		}
		fmt.Printf("%d/%d p%d %s eff=%s by=%s expected=%d changes=%d %s\n  %.110s\n", n.Volume, n.Number, n.Page, r.Status, r.Effective, r.CommentsBy, r.Expected, len(r.Changes), p, n.Title)
		for _, c := range r.Changes {
			fmt.Printf("    %s %s by %s\n", c.Ruling, c.Kind, c.By)
		}
	}
	fmt.Printf("bulletins: %d notices, %d with a problem\n", len(notices), bad)
	return nil
}

func bulletinsModel(args []string) error {
	fl := flag.NewFlagSet("bulletins model", flag.ExitOnError)
	path := fl.String("notices", bulletinNotices, "notice excerpts")
	out := fl.String("out", bulletinReads, "model reads (committed)")
	model := fl.String("model", bulletin.ReadModel, "model (D400 pre-registers gemini-3.5-flash)")
	capUSD := fl.Float64("cap", 6, "stop when the reported spend reaches this")
	_ = fl.Parse(args)
	base, key := os.Getenv("LITELLM_BASE_URL"), os.Getenv("LITELLM_API_KEY")
	if base == "" || key == "" {
		return errors.New("LITELLM_BASE_URL and LITELLM_API_KEY must be set")
	}
	notices, err := loadNotices(*path)
	if err != nil {
		return err
	}
	done, err := bulletin.LoadReads(*out)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	ll := llm.NewLiteLLM(base, key, 2*time.Minute)
	ll.SkipCache(true)
	tally := &llm.CostTally{}
	ctx := llm.ContextWithCostTally(context.Background(), tally)
	f, err := os.OpenFile(*out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	n := 0
	for _, nt := range notices {
		if bulletin.Parse(nt).Problem() == "" {
			continue
		}
		if _, ok := done[nt.Key()]; ok {
			continue
		}
		if tally.USD() >= *capUSD {
			return fmt.Errorf("spend cap $%.2f reached", *capUSD)
		}
		before := tally.USD()
		rd := bulletin.ReadNotice(ctx, ll, *model, nt)
		rd.CostUSD = tally.USD() - before
		b, _ := json.Marshal(rd)
		if _, err := f.Write(append(b, '\n')); err != nil {
			return err
		}
		n++
		fmt.Printf("%s: %d rulings %s ($%.5f)\n", nt.Key(), len(rd.Rulings), rd.Error, rd.CostUSD)
	}
	fmt.Printf("bulletins: %d notices read, $%.5f reported by the gateway\n", n, tally.USD())
	return nil
}
