package tariffshift

import (
	"fmt"
	"sort"
	"strings"
)

// MaxCombinations caps how many code combinations Robust evaluates (D430).
// Three suggested codes for each of four parts is 81; beyond that a
// deterministic sample is taken.
const MaxCombinations = 81

// Outcome is one answer the check gave over the combinations, with how many
// combinations gave it.
type Outcome struct {
	Status string `json:"status"`
	Origin string `json:"origin,omitempty"`
	Count  int    `json:"count"`
}

// Robustness says whether the check's answer depends on which suggested
// code each part carries (D430). The check is run on combinations of each
// part's codes: the one it carries and the other codes suggested for it.
type Robustness struct {
	// Stable is true when every combination evaluated gives the same answer
	// (the same status and, when decided, the same origin).
	Stable bool `json:"stable"`
	// Combinations is how many combinations there are; Evaluated how many
	// were run. Sampled is true when there were more than MaxCombinations
	// and a deterministic sample was run (every single-part change from the
	// codes carried, then evenly spaced combinations).
	Combinations int  `json:"combinations"`
	Evaluated    int  `json:"evaluated"`
	Sampled      bool `json:"sampled"`
	// Parts lists the parts that have more than one code, in input order.
	Parts []string `json:"parts"`
	// DependsOn lists the parts for which changing that part's code alone
	// changes the answer. It can be empty when the answer is not stable
	// but changes only when several codes change together.
	DependsOn []string  `json:"depends_on,omitempty"`
	Outcomes  []Outcome `json:"outcomes"`
	Summary   string    `json:"summary"`
}

// Robust runs the check on combinations of codes. alts[i] lists the other
// codes for in.Materials[i] (the code it carries comes first); a material
// with no alternatives keeps its code. It returns nil when no material has
// an alternative.
func (r *Rules) Robust(in Input, alts [][]string) *Robustness {
	choices := make([][]string, len(in.Materials))
	var multi []int
	for i, m := range in.Materials {
		seen := map[string]bool{Digits(m.HTS): true}
		choices[i] = []string{m.HTS}
		if i < len(alts) && strings.TrimSpace(m.HTS) != "" {
			for _, a := range alts[i] {
				if d := Digits(a); d != "" && !seen[d] {
					seen[d] = true
					choices[i] = append(choices[i], a)
				}
			}
		}
		if len(choices[i]) > 1 {
			multi = append(multi, i)
		}
	}
	if len(multi) == 0 {
		return nil
	}
	rb := &Robustness{Combinations: count(multi, choices)}
	for _, i := range multi {
		rb.Parts = append(rb.Parts, in.Materials[i].Name)
	}
	rb.Sampled = rb.Combinations > MaxCombinations

	// A combination is the index of the code each multi-code part takes.
	var combos [][]int
	if !rb.Sampled {
		combos = allCombos(multi, choices)
	} else {
		combos = sampleCombos(multi, choices, MaxCombinations)
	}
	key := func(res Result) string { return res.Status + "|" + res.Origin }
	outcome := map[string]*Outcome{}
	var order []string
	keys := make([]string, len(combos))
	for ci, c := range combos {
		v := in
		v.Materials = append([]Material(nil), in.Materials...)
		for k, i := range multi {
			v.Materials[i].HTS = choices[i][c[k]]
		}
		res := r.Evaluate(v)
		k := key(res)
		keys[ci] = k
		if outcome[k] == nil {
			outcome[k] = &Outcome{Status: res.Status, Origin: res.Origin}
			order = append(order, k)
		}
		outcome[k].Count++
	}
	rb.Evaluated = len(combos)
	for _, k := range order {
		rb.Outcomes = append(rb.Outcomes, *outcome[k])
	}
	rb.Stable = len(order) == 1

	if !rb.Stable {
		// A part matters when two combinations that differ only in its code
		// give different answers.
		for k, i := range multi {
			groups := map[string]string{}
			for ci, c := range combos {
				g := maskKey(c, k)
				if prev, ok := groups[g]; ok && prev != keys[ci] {
					rb.DependsOn = append(rb.DependsOn, in.Materials[i].Name)
					break
				}
				groups[g] = keys[ci]
			}
		}
	}
	rb.Summary = summary(rb)
	return rb
}

func allCombos(multi []int, choices [][]string) [][]int {
	out := [][]int{make([]int, len(multi))}
	for k, i := range multi {
		var next [][]int
		for _, c := range out {
			for j := range choices[i] {
				d := append([]int(nil), c...)
				d[k] = j
				next = append(next, d)
			}
		}
		out = next
	}
	sort.SliceStable(out, func(a, b int) bool { return less(out[a], out[b]) })
	return out
}

// sampleCombos takes the codes carried, every single-part change from them,
// then evenly spaced combinations in mixed-radix order, up to max.
func sampleCombos(multi []int, choices [][]string, max int) [][]int {
	seen := map[string]bool{}
	var out [][]int
	add := func(c []int) {
		if len(out) < max && !seen[fmt.Sprint(c)] {
			seen[fmt.Sprint(c)] = true
			out = append(out, c)
		}
	}
	add(make([]int, len(multi)))
	for k, i := range multi {
		for j := 1; j < len(choices[i]); j++ {
			c := make([]int, len(multi))
			c[k] = j
			add(c)
		}
	}
	// Evenly spaced combinations: the n-th in mixed-radix order, with n
	// stepping through the whole range.
	total := count(multi, choices)
	left := max - len(out)
	for s := 0; s < left*4 && len(out) < max; s++ {
		n := int(float64(s) * float64(total) / float64(left*4))
		c := make([]int, len(multi))
		for k := len(multi) - 1; k >= 0; k-- {
			r := len(choices[multi[k]])
			c[k] = n % r
			n /= r
		}
		add(c)
	}
	return out
}

// maxCount bounds count so the product cannot overflow; past it the sample
// spreads over the first parts' codes only, which no real parts list
// reaches (20 parts with 3 codes each is about 3.5e9).
const maxCount = 1 << 50

// count is the number of combinations, at most maxCount.
func count(multi []int, choices [][]string) int {
	n := 1
	for _, i := range multi {
		if n > maxCount/len(choices[i]) {
			return maxCount
		}
		n *= len(choices[i])
	}
	return n
}

func less(a, b []int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

func maskKey(c []int, skip int) string {
	var b strings.Builder
	for i, v := range c {
		if i == skip {
			b.WriteString("*,")
			continue
		}
		fmt.Fprintf(&b, "%d,", v)
	}
	return b.String()
}

func summary(rb *Robustness) string {
	n := fmt.Sprintf("%d combinations", rb.Evaluated)
	if rb.Evaluated == 1 {
		n = "1 combination"
	}
	if rb.Sampled {
		n = fmt.Sprintf("a sample of %d of %d combinations", rb.Evaluated, rb.Combinations)
	}
	if rb.Stable {
		return fmt.Sprintf("the same answer for every suggested code (%s of the codes suggested for %s)", n, strings.Join(rb.Parts, ", "))
	}
	if len(rb.DependsOn) > 0 {
		return fmt.Sprintf("the answer depends on the code for %s (%s)", strings.Join(rb.DependsOn, ", "), n)
	}
	return fmt.Sprintf("the answer depends on which codes are chosen together (%s)", n)
}
