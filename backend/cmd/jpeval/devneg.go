package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpcorpus"
)

// devNegFile holds development negatives (D913): development approvals
// with one added fact that breaks every tariff-shift and value rule, so a
// correct answer is "not shown to qualify". Built by `jpeval dev-negatives`.
const devNegFile = "dev-negatives.jsonl"

// devNegCount is how many negatives the file holds.
const devNegCount = 80

type devNegRow struct {
	ID        string `json:"id"`
	Source    string `json:"source"`
	Date      string `json:"date"`
	Agreement string `json:"agreement"`
	HS        string `json:"hs"`
	Facts     string `json:"facts"`
}

// devNegFact is the added fact: a non-originating material of the good's
// own six-digit code, from a country party to no Japan agreement, worth 70%
// of the ex-works price, used without further working. It fails CC, CTH and
// CTSH, every listed value threshold, and de minimis.
func devNegFact(hs string) string {
	return fmt.Sprintf("Additional fact: the producer also uses a semi-finished article of HS %s, imported from Brazil, which is a party to none of the agreements concerned. It is non-originating, has no proof of origin and is incorporated into the good without further working. Its value is 70%% of the ex-works price of the good.", hs)
}

func devNegatives(args []string) error {
	fs := flag.NewFlagSet("dev-negatives", flag.ExitOnError)
	check := fs.Bool("check", false, "fail if the committed file differs")
	_ = fs.Parse(args)
	c, err := jpcorpus.Load(dataDir)
	if err != nil {
		return err
	}
	sp, err := jpcorpus.LoadSplits(filepath.Join(dataDir, "splits.json"))
	if err != nil {
		return err
	}
	labels, err := jpcorpus.LoadLabels(filepath.Join(dataDir, "labels-dev.jsonl"))
	if err != nil {
		return err
	}
	ids := append([]string(nil), sp.DevEval...)
	sort.Strings(ids)
	var b bytes.Buffer
	n := 0
	for _, id := range ids {
		l, ok := labels[id]
		r, ok2 := c.Ruling(id)
		if !ok || !ok2 || l.Outcome != "positive" || l.Agreement == "GSP" || l.Agreement == "Non-preferential" {
			continue
		}
		switch l.Criterion {
		case "CC", "CTH", "CTSH":
		default:
			continue
		}
		hs := digitsOnly(r.HS)
		if len(hs) < 6 {
			continue
		}
		hs = hs[:4] + "." + hs[4:6]
		facts, err := c.CaseFacts(id)
		if err != nil {
			return err
		}
		d, err := r.Time()
		if err != nil {
			return err
		}
		row := devNegRow{ID: "devneg-" + id, Source: id, Date: d.Format("2006-01-02"), Agreement: r.Agreement, HS: r.HS,
			Facts: strings.TrimSpace(facts) + "\n\n" + devNegFact(hs)}
		line, err := json.Marshal(row)
		if err != nil {
			return err
		}
		b.Write(append(line, '\n'))
		if n++; n == devNegCount {
			break
		}
	}
	if n < devNegCount {
		return fmt.Errorf("only %d development approvals fit; want %d", n, devNegCount)
	}
	out := filepath.Join(outDir, devNegFile)
	if *check {
		old, err := os.ReadFile(out)
		if err != nil || !bytes.Equal(old, b.Bytes()) {
			return errors.Join(errors.New(out+" is stale: run go run ./cmd/jpeval dev-negatives"), err)
		}
		fmt.Println("dev negatives ok", n)
		return nil
	}
	fmt.Println("wrote", n, "dev negatives")
	return os.WriteFile(out, b.Bytes(), 0o644)
}

func devNegCases() ([]candidateInput, error) {
	f, err := os.ReadFile(filepath.Join(outDir, devNegFile))
	if err != nil {
		return nil, err
	}
	var items []candidateInput
	for _, line := range bytes.Split(bytes.TrimSpace(f), []byte("\n")) {
		var row devNegRow
		if err := json.Unmarshal(line, &row); err != nil {
			return nil, err
		}
		d, err := time.Parse(time.DateOnly, row.Date)
		if err != nil {
			return nil, err
		}
		items = append(items, candidateInput{
			label: jpcorpus.Label{ID: row.ID, Agreement: row.Agreement, Outcome: "negative"},
			caseValue: domain.Case{ID: row.ID, Date: d, Facts: row.Facts,
				Japan: &domain.JapanInput{Agreement: row.Agreement, HS: row.HS}},
		})
	}
	if len(items) != devNegCount {
		return nil, fmt.Errorf("%s has %d negatives; want %d", devNegFile, len(items), devNegCount)
	}
	return items, nil
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
