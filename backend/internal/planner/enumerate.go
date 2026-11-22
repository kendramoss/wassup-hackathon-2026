package planner

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// option is one movable group and a plant in the target country that can
// perform it.
type option struct{ group, plant string }

// options lists, in template group order then plant order, each movable group
// not yet in the target country with each target plant that can perform it.
func options(p domain.Product, t Template, target string) []option {
	var opts []option
	for _, g := range t.Groups {
		if !g.Movable {
			continue
		}
		cur, present := groupPlant(p, g.ID)
		if !present && len(groupComponents(p, g.ID)) == 0 {
			continue // the product has no such group
		}
		if present && domain.NormalizeCountry(cur.Country) == target {
			continue
		}
		if !present && allComponentsIn(p, g.ID, target) {
			continue
		}
		for _, pl := range p.Plants {
			if domain.NormalizeCountry(pl.Country) == target && pl.Can(g.ID) {
				opts = append(opts, option{g.ID, pl.ID})
			}
		}
	}
	return opts
}

// Enumerate lists every single and double move of movable operation groups to
// a plant in the target country that can perform them. Groups already
// performed in the target country are not moved. Order is deterministic:
// template group order, then plant order.
func Enumerate(p domain.Product, t Template, target string) []domain.Move {
	opts := options(p, t, domain.NormalizeCountry(target))
	var moves []domain.Move
	for _, o := range opts {
		moves = append(moves, domain.Move{{Group: o.group, ToPlant: o.plant}})
	}
	for i := 0; i < len(opts); i++ {
		for j := i + 1; j < len(opts); j++ {
			if opts[i].group == opts[j].group {
				continue
			}
			moves = append(moves, domain.Move{{Group: opts[i].group, ToPlant: opts[i].plant}, {Group: opts[j].group, ToPlant: opts[j].plant}})
		}
	}
	return moves
}

// EnumerateUpTo is Enumerate plus moves of three distinct groups, added only
// while the total stays at most maxMoves (D84). CBP's real change can move
// three groups at once (the heat gun's N357771), which pairs never cover.
// When not every triple fits, the triples whose groups cost the most where
// they are made now come first (costs: the request's group costs by plant),
// ties in template order. Singles and pairs keep Enumerate's order, so their
// cached verdicts are reused. maxMoves <= 0 adds no triples.
func EnumerateUpTo(p domain.Product, t Template, target string, costs map[string]map[string]float64, maxMoves int) []domain.Move {
	moves := Enumerate(p, t, target)
	room := maxMoves - len(moves)
	if room <= 0 {
		return moves
	}
	opts := options(p, t, domain.NormalizeCountry(target))
	type triple struct {
		move domain.Move
		cost float64
	}
	current := func(group string) float64 {
		from, ok := groupPlant(p, group)
		if !ok {
			return 0
		}
		return costs[group][from.ID]
	}
	var ts []triple
	for i := 0; i < len(opts); i++ {
		for j := i + 1; j < len(opts); j++ {
			for k := j + 1; k < len(opts); k++ {
				a, b, c := opts[i], opts[j], opts[k]
				if a.group == b.group || a.group == c.group || b.group == c.group {
					continue
				}
				ts = append(ts, triple{
					move: domain.Move{{Group: a.group, ToPlant: a.plant}, {Group: b.group, ToPlant: b.plant}, {Group: c.group, ToPlant: c.plant}},
					cost: current(a.group) + current(b.group) + current(c.group),
				})
			}
		}
	}
	sort.SliceStable(ts, func(i, j int) bool { return ts[i].cost > ts[j].cost })
	for i := 0; i < len(ts) && i < room; i++ {
		moves = append(moves, ts[i].move)
	}
	return moves
}

func allComponentsIn(p domain.Product, group, country string) bool {
	for _, c := range p.Components {
		if c.Group == group && domain.NormalizeCountry(c.MadeIn) != country {
			return false
		}
	}
	return true
}

// Describe is a one-line description of a move for the UI, the draft letter
// and the case agent, e.g. "Move the motor-fan assembly and the PCBA (SMT) to
// Thailand". Group names are read in lower case after "the", so a name never
// starts with a capital mid-sentence, while acronyms ("PCBA (SMT)", "LED
// PCBA") keep theirs (D220). The verifier never reads it: it reads the facts.
func Describe(p domain.Product, t Template, m domain.Move) string {
	return DescribeAt(p, t, m, t.Version)
}

// DescribeAt is Describe for a stored plan made with template version
// madeWith, so a group keeps the name it had then (GroupNameAt, D372).
func DescribeAt(p domain.Product, t Template, m domain.Move, madeWith string) string {
	if len(m) == 0 {
		return "No change: keep the current setup"
	}
	items := make([][]string, 0, len(m))
	country := ""
	sameCountry := true
	for i, gm := range m {
		items = append(items, groupItems(p, t, gm.Group, madeWith))
		pl, _ := p.PlantByID(gm.ToPlant)
		if i > 0 && pl.Country != country {
			sameCountry = false
		}
		country = pl.Country
	}
	if sameCountry {
		// One list for every group, joined once: "the motor stator, the
		// control boards and the wiring harness", not "the motor stator and
		// the control boards and ..." (D641).
		var all []string
		for _, it := range items {
			all = append(all, it...)
		}
		return "Move " + joinAnd(all) + " to " + country
	}
	parts := make([]string, len(m))
	for i, gm := range m {
		pl, _ := p.PlantByID(gm.ToPlant)
		parts[i] = joinAnd(items[i]) + " to " + pl.Country
	}
	return "Move " + joinAnd(parts)
}

// groupItems names a moved group in a plan sentence: "the " and the
// template's group name, unless that name describes none of this product's
// parts in the group. Then it names the parts, as Render does in the facts
// (D270): a power bank's battery cells filed under the template's "Power
// conversion or charging module" read "the two cylindrical lithium-ion
// battery cells", not a module the product does not have.
// It returns a list so Describe can join the items of several groups once
// (D641). A template name that fits stays one item.
func groupItems(p domain.Product, t Template, id, madeWith string) []string {
	name := GroupNameAt(p, t, id, madeWith)
	comps := groupComponents(p, id)
	if len(comps) == 0 {
		return []string{"the " + lowerWord(name)}
	}
	if describesAny(name, comps) {
		kept, rest := fittingItems(name, comps)
		return append([]string{"the " + lowerWord(kept)}, rest...)
	}
	parts := make([]string, 0, 3)
	for i, c := range comps {
		if i == 2 && len(comps) > 3 {
			parts = append(parts, fmt.Sprintf("%d other parts", len(comps)-2))
			break
		}
		parts = append(parts, "the "+lowerWord(c))
	}
	return parts
}

// fittingItems drops from a listed group name ("PCBA and sensors", "Base,
// harness and seals") each one-word item that names none of the parts filed
// under the group, so a PCR instrument with a main board, a monitor and no
// sensor reads "the PCBA and 4 other parts", not "the PCBA and sensors"
// (judge panel round 4, D290). It returns the shorter name and, as rest,
// the plan-sentence items for the parts that name does not cover. An item of more than one word is dropped the
// same way unless it is a process ("firebox forming" stays), so a merged
// Part 102 group name such as "Sugar, cocoa and dairy ingredients" reads "the
// sugar and cocoa" for a blend with no dairy (#633, D622). A name with a
// parenthesis is left whole. If dropping would leave nothing that fits, the
// name is kept as it is and rest is nil.
func fittingItems(name string, parts []string) (kept string, rest []string) {
	if strings.ContainsAny(name, "()") {
		return name, nil
	}
	items := strings.Split(strings.ReplaceAll(name, " and ", ", "), ", ")
	if len(items) < 2 {
		return name, nil
	}
	keep := make([]string, 0, len(items))
	for _, it := range items {
		it = strings.TrimSpace(it)
		if (len(strings.Fields(it)) == 1 || !isProcess(it)) && len(nameWords(it)) > 0 && !describesAny(it, parts) {
			continue
		}
		keep = append(keep, it)
	}
	if len(keep) == len(items) || len(keep) == 0 {
		return name, nil
	}
	if !describesAny(joinAnd(keep), parts) {
		return name, nil
	}
	// Parts the shorter name does not cover follow it, so the sentence
	// still says everything that moves: one such part by its name ("the
	// PCBA and the temperature control module", D641), more as a count
	// ("the PCBA and 4 other parts").
	var other []string
	for _, c := range parts {
		if !describesAny(joinAnd(keep), []string{c}) {
			other = append(other, c)
		}
	}
	switch len(other) {
	case 0:
		return joinAnd(keep), nil
	case 1:
		return joinAnd(keep), []string{"the " + lowerWord(other[0])}
	}
	return joinAnd(keep), []string{fmt.Sprintf("%d other parts", len(other))}
}

// isProcess reports whether a listed item names work rather than a thing:
// its last word is a process noun such as "forming", "machining",
// "lamination", "extrusion" or "treatment". Only the last word counts, and
// only words on the list: a suffix test read "insulating bases", "cooking
// surface", "cable insulation" and "wood flooring" as work, so a plan title
// kept them for a product that has none (#648).
func isProcess(item string) bool {
	words := strings.FieldsFunc(strings.ToLower(item), func(r rune) bool { return !unicode.IsLetter(r) })
	return len(words) > 0 && processNouns[words[len(words)-1]]
}

// Process nouns that end a template item naming a manufacturing step
// ("Film extrusion", "Head finishing", "Wire conductor making"). Words that
// also name a thing ("assembly", "casting", "coating", "packaging") are left
// out, so "club assembly" or "busbar assembly" read as parts.
var processNouns = map[string]bool{
	"blending": true, "compounding": true, "converting": true, "cutting": true,
	"drawing": true, "extrusion": true, "fabrication": true, "finishing": true,
	"forging": true, "forming": true, "grinding": true, "knitting": true,
	"lamination": true, "machining": true, "making": true, "mixing": true,
	"molding": true, "moulding": true, "packing": true, "plating": true,
	"polishing": true, "rolling": true, "sewing": true, "spinning": true,
	"stamping": true, "testing": true, "treatment": true, "weaving": true,
	"welding": true, "winding": true,
}

// Words too general to show that a group name fits a part.
var vagueWords = map[string]bool{"and": true, "or": true, "the": true, "of": true, "with": true, "for": true, "part": true, "minor": true, "other": true, "assembly": true, "module": true, "component": true, "unit": true, "smt": true}

// Words that stand for one another in group and part names.
var sameWords = map[string][]string{"pcba": {"printed", "circuit", "board"}, "pcb": {"printed", "circuit", "board"}}

// nameWords is a name's significant words, lower case, with a plural "s"
// dropped ("lids" and "lid" match), expanded with their synonyms.
func nameWords(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) }) {
		if strings.HasSuffix(w, "es") && len(w) > 4 && strings.HasSuffix(w, "xes") {
			w = strings.TrimSuffix(w, "es")
		} else if strings.HasSuffix(w, "s") && len(w) > 3 && !strings.HasSuffix(w, "ss") {
			w = strings.TrimSuffix(w, "s")
		}
		if len(w) < 3 || vagueWords[w] {
			continue
		}
		out[w] = true
		for _, x := range sameWords[w] {
			out[x] = true
		}
	}
	return out
}

// describesAny reports whether a group name shares a significant word with
// at least one of the parts filed under it.
func describesAny(name string, parts []string) bool {
	nw := nameWords(name)
	for _, c := range parts {
		for w := range nameWords(c) {
			if nw[w] {
				return true
			}
		}
	}
	return false
}

// GroupName is a group's name in the template, else its id with spaces.
func GroupName(t Template, id string) string {
	if g, ok := t.Group(id); ok && g.Name != "" {
		return g.Name
	}
	return strings.ReplaceAll(id, "_", " ")
}

// GroupNameFor is a group's name for a plan made now: the template's name.
func GroupNameFor(p domain.Product, t Template, id string) string {
	return GroupNameAt(p, t, id, t.Version)
}

// GroupNameAt is a group's name for a plan made with template version
// madeWith (the stored plan's template_versions entry). The group's former
// name applies only when that version is older than the split (D342, D372):
// the pinned heat-gun case was planned before the power cord, knobs and
// covers left "Switch, cord and minor parts" (D330), and its cord is still
// in that group. It is never inferred from the parts alone, so a cordless
// tool planned today reads "Switch, wiring and minor parts". A product that
// files anything under the split-out group was filed after the split and
// reads the new name whatever the version.
func GroupNameAt(p domain.Product, t Template, id, madeWith string) string {
	g, ok := t.Group(id)
	if ok && g.Former != nil && g.Former.Name != "" && versionBefore(madeWith, g.Former.Since) && usesGroup(p, id) && !usesGroup(p, g.Former.Until) {
		return g.Former.Name
	}
	return GroupName(t, id)
}

// usesGroup says whether the product files any component or operation under
// the group.
func usesGroup(p domain.Product, id string) bool {
	for _, c := range p.Components {
		if c.Group == id {
			return true
		}
	}
	for _, o := range p.Operations {
		if o.Group == id {
			return true
		}
	}
	return false
}

// lowerWord lower-cases the first letter of an ordinary word ("Motor-fan
// assembly" -> "motor-fan assembly") and leaves an acronym alone ("PCBA (SMT)",
// "LED PCBA (SMT)", "RX and TX PCBAs").
func lowerWord(s string) string {
	r := []rune(s)
	if len(r) >= 2 && unicode.IsUpper(r[0]) && !unicode.IsUpper(r[1]) {
		r[0] = unicode.ToLower(r[0])
	}
	return string(r)
}

// Redescribe rewrites every move's description in a stored plan search with
// DescribeAt at the template version it was made with, so a result saved
// before D220 reads the same as a new one. The
// move, verdict and money are untouched. A result whose family has no
// template, or a case with no product, is left as stored.
func Redescribe(r *Result, p *domain.Product, templates map[string]Template) {
	if r == nil || p == nil {
		return
	}
	t, ok := templates[r.Family]
	if !ok {
		return
	}
	madeWith := r.Templates[r.Family]
	fix := func(ps []domain.Plan) []domain.Plan {
		if ps == nil {
			return nil
		}
		out := append([]domain.Plan(nil), ps...)
		for i := range out {
			out[i].Description = DescribeAt(*p, t, out[i].Move, madeWith)
		}
		return out
	}
	r.Baseline.Description = DescribeAt(*p, t, r.Baseline.Move, madeWith)
	r.Plans, r.Rejected = fix(r.Plans), fix(r.Rejected)
	if r.Backtest != nil && len(r.Plans) > 0 {
		bt := *r.Backtest
		bt.PlanMoves = r.Plans[0].Description
		r.Backtest = &bt
	}
}
