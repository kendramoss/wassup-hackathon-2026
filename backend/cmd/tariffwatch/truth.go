package main

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
)

// The ground truth of the D941 test is read from the Federal Register notices
// that granted the exclusions and the maritime duties, not from the loader or
// from Chapter 99. The files are in eval/tariffwatch.

const statNo = `\d{4}\.\d{2}\.\d{4}`

var (
	clauseRE   = regexp.MustCompile(`statistical reporting numbers?\s+((?:` + statNo + `(?:\s*(?:,|or|and)\s*)*)+)\s*([^;)]{0,120})`)
	joinerRE   = regexp.MustCompile(`^\s*(?:,|or|and|,\s*or)\s*$`)
	codeRE     = regexp.MustCompile(statNo)
	pageHeader = regexp.MustCompile(`^(Federal Register|\d+ \| Federal)`)
	spaceRE    = regexp.MustCompile(`\s+`)
)

// exclusionNumbers reads the statistical reporting numbers in force out of
// one OCR of the U.S. note 20(vvv) list in 89 FR 46948. A number is a single
// item ("5) | 0304.83.5020") or sits in a description's "(described in
// statistical reporting number A prior to DATE; B or C effective DATE)". Every
// 10-digit number is read with the words after it (or after the group of
// numbers joined by commas, "or" and "and"): "prior to", or "effective"
// followed by a "through" end day before the next semicolon, make it history;
// anything else is in force.
func exclusionNumbers(ocr string) []string {
	var kept []string
	for _, l := range strings.Split(ocr, "\n") {
		if strings.HasPrefix(l, "#") || strings.HasPrefix(l, "=== PAGE") || pageHeader.MatchString(l) {
			continue
		}
		kept = append(kept, l)
	}
	flat := spaceRE.ReplaceAllString(strings.Join(kept, " "), " ")
	cur := map[string]bool{}
	locs := codeRE.FindAllStringIndex(flat, -1)
	for i := 0; i < len(locs); {
		j := i
		for j+1 < len(locs) && joinerRE.MatchString(flat[locs[j][1]:locs[j+1][0]]) {
			j++
		}
		qual := strings.TrimLeft(flat[locs[j][1]:], " ")
		if end := strings.IndexAny(qual, ";)"); end >= 0 {
			qual = qual[:end]
		}
		if k := codeRE.FindStringIndex(qual); k != nil {
			qual = qual[:k[0]]
		}
		history := strings.HasPrefix(qual, "prior to") || (strings.HasPrefix(qual, "effective") && strings.Contains(qual, "through"))
		if !history {
			for g := i; g <= j; g++ {
				cur[flat[locs[g][0]:locs[g][1]]] = true
			}
		}
		i = j + 1
	}
	return sortedKeys(cur)
}

// votedNumbers keeps the numbers that at least two of the OCR readings give,
// so a misread digit in one run does not become a number.
func votedNumbers(readings ...[]string) []string {
	votes := map[string]int{}
	for _, r := range readings {
		for _, n := range r {
			votes[n]++
		}
	}
	keep := map[string]bool{}
	for n, v := range votes {
		if v >= 2 {
			keep[n] = true
		}
	}
	return sortedKeys(keep)
}

// solarNumbers reads the numbers of the 14 solar equipment exclusions of
// 89 FR 76581 Annex B.
func solarNumbers(txt string) []string {
	cur := map[string]bool{}
	flat := spaceRE.ReplaceAllString(txt, " ")
	for _, m := range clauseRE.FindAllStringSubmatch(flat, -1) {
		for _, c := range codeRE.FindAllString(m[1], -1) {
			cur[c] = true
		}
	}
	return sortedKeys(cur)
}

var plusRE = regexp.MustCompile(`\+\s*(\d+)%`)

// maritimeLines reads the tariff lines and the additional rate of headings
// 9903.91.12 (chassis) and 9903.91.14 (ship-to-shore cranes) out of Annex V.A
// of 90 FR 48320. Both headings carry the same "+ 100%".
func maritimeLines(txt string) (lines []string, rate float64) {
	start := strings.Index(txt, "9903.91.12")
	end := strings.Index(txt, "9903.91.15")
	if start < 0 || end < start {
		return nil, 0
	}
	region := spaceRE.ReplaceAllString(txt[start:end], " ")
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(`\b(\d{4}\.\d{2}\.\d{2})(?:\d{2})?\b`).FindAllStringSubmatch(region, -1) {
		if !strings.HasPrefix(m[1], "99") {
			seen[m[1]] = true
		}
	}
	rates := map[float64]bool{}
	for _, m := range plusRE.FindAllStringSubmatch(region, -1) {
		v, _ := strconv.Atoi(m[1])
		rates[float64(v)/100] = true
	}
	if len(rates) == 1 {
		for r := range rates {
			rate = r
		}
	}
	return sortedKeys(seen), rate
}

// subheadings maps statistical reporting numbers to their sorted, distinct
// 8-digit subheadings.
func subheadings(nums []string) []string {
	seen := map[string]bool{}
	for _, n := range nums {
		seen[duty.Normalize(n)] = true
	}
	return sortedKeys(seen)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
