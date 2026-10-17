package planner

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// followable picks the one move to search from near-miss advice (D85): for
// the advised near misses, highest check first, the near miss's own groups
// plus the groups its advice names, when that is a move to the target the
// plants can perform and has not been searched yet. Groups are matched in the
// advice text first (group names, component names, operations), and only if
// none match, by the Mapper.
func (p *Planner) followable(ctx context.Context, req Request, tm Template, target string, searched []domain.Move, res *Result) (domain.Move, bool) {
	seen := map[string]bool{}
	for _, m := range searched {
		seen[m.Key()] = true
	}
	opts := options(req.Product, tm, target)
	var advised []domain.Plan
	for _, pl := range res.Rejected {
		if pl.NearMiss != nil && pl.Verdict != nil && len(pl.Move) > 0 {
			advised = append(advised, pl)
		}
	}
	sort.SliceStable(advised, func(i, j int) bool { return advised[i].Verdict.Confidence > advised[j].Verdict.Confidence })
	for _, pl := range advised {
		in := map[string]bool{}
		for _, gm := range pl.Move {
			in[gm.Group] = true
		}
		var cands []domain.OperationGroup
		for _, g := range tm.Groups {
			if !in[g.ID] && hasOption(opts, g.ID) {
				og := g.OperationGroup
				og.Components = groupComponents(req.Product, g.ID)
				cands = append(cands, og)
			}
		}
		if len(cands) == 0 {
			continue
		}
		named := MatchGroups(pl.NearMiss.WouldMatch, cands)
		if len(named) == 0 && p.opts.Mapper != nil {
			ids, u, err := p.opts.Mapper.MapGroups(ctx, pl.NearMiss.WouldMatch, cands)
			if err == nil {
				res.Stats.NearMissCalls++
				res.Stats.NearMissInTok += u.InputTokens
				res.Stats.NearMissOutTok += u.OutputTokens
				named = ids
			}
		}
		m := extend(pl.Move, named, opts, cands)
		if len(m) > len(pl.Move) && len(m) <= domain.MaxGroupsPerMove && !seen[m.Key()] {
			return m, true
		}
	}
	return nil, false
}

func hasOption(opts []option, group string) bool {
	for _, o := range opts {
		if o.group == group {
			return true
		}
	}
	return false
}

// extend adds the named groups (only candidates) to move, in template order
// as Enumerate lists them, so a move is keyed the same however it was found.
func extend(move domain.Move, named []string, opts []option, cands []domain.OperationGroup) domain.Move {
	plant := map[string]string{}
	for _, gm := range move {
		plant[gm.Group] = gm.ToPlant
	}
	ok := map[string]bool{}
	for _, c := range cands {
		ok[c.ID] = true
	}
	for _, id := range named {
		if !ok[id] || plant[id] != "" {
			continue
		}
		for _, o := range opts {
			if o.group == id {
				plant[id] = o.plant // the first target plant that can do it
				break
			}
		}
	}
	var out domain.Move
	added := map[string]bool{}
	for _, o := range opts {
		if pl := plant[o.group]; pl == o.plant && !added[o.group] {
			out = append(out, domain.GroupMove{Group: o.group, ToPlant: o.plant})
			added[o.group] = true
		}
	}
	return out
}

var (
	termSplit = regexp.MustCompile(`\s*(?:,|;|/|\band\b|\bor\b|\bwith\b|\bfrom\b)\s*`)
	parens    = regexp.MustCompile(`\([^)]*\)`)
	leading   = regexp.MustCompile(`^(?:the|a|an|make|makes|produce|build|its|all|other)\s+`)
)

// terms are the phrases that name a group: its name, its components' names
// and the pieces of its operations, lower-cased, without parentheses or
// leading articles and verbs, at least four letters.
func terms(g domain.OperationGroup) []string {
	var src []string
	src = append(src, g.Name)
	src = append(src, g.Components...)
	src = append(src, g.Operations...)
	var out []string
	for _, s := range src {
		s = strings.ToLower(parens.ReplaceAllString(s, " "))
		for _, t := range termSplit.Split(s, -1) {
			t = strings.TrimSpace(t)
			for {
				u := leading.ReplaceAllString(t, "")
				if u == t {
					break
				}
				t = u
			}
			if len(t) >= 4 {
				out = append(out, t)
			}
		}
	}
	return out
}

// MatchGroups returns the ids of the groups whose name, components or
// operations the advice names, in the order given.
func MatchGroups(advice string, groups []domain.OperationGroup) []string {
	text := " " + strings.ToLower(advice) + " "
	var ids []string
	for _, g := range groups {
		for _, t := range terms(g) {
			if mentions(text, t) {
				ids = append(ids, g.ID)
				break
			}
		}
	}
	return ids
}

// mentions reports whether text names term as whole words (a plural s or es
// allowed).
func mentions(text, term string) bool {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(term) + `(?:s|es)?\b`)
	return re.MatchString(text)
}
