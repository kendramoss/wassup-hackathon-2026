// Package extract drafts a structured product from pasted facts text, so the
// "Paste a description" path can still feed the plan search. The model reads
// the numbered paragraphs and lists components, operations and countries;
// code then builds plants, checks groups against the family template, cites
// paragraphs, and lists what the text cannot answer. The user edits the draft
// before anything is saved.
package extract

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

// PromptVersion is recorded with every draft.
const PromptVersion = "extract-v3"

const system = `You turn a product description from a U.S. Customs ruling request into a parts list. The paragraphs are numbered [1], [2], ...

List:
- product: the product name.
- components: every part the text names, one per entry. If the text lists several parts together ("the PCBA, motor, fan ... are sourced from China"), make one entry per part. country is the country the text STATES for that part; if the text mentions a part without saying where it is made (for example a screw named only in an assembly step), give country "" — never infer it from other parts. paragraph is where the part is first named with its country (or first named, if no country).
- operations: the assembly or processing the text describes as something done (fitting, soldering, fastening, testing...), one entry per sentence that describes it, keeping that sentence's steps together in the name; with the country where it happens and its paragraph. A statement of where parts are produced or sourced is not an operation.
- plants: a plant or supplier the text names, with its country. Usually the text names none: return an empty list then.
- stated_component_count: the number of components the text states in total (e.g. "24 components"), or null.
- notes: anything the text leaves unclear about where a part is made.

For "group", use one of the operation groups given (the id), or "" if none fits. Use only what the text says; never invent parts, countries or plants.`

// Row is a drafted component or operation with where it came from.
type Row struct {
	Name            string `json:"name"`
	MadeIn          string `json:"made_in"`
	Plant           string `json:"plant"`
	Group           string `json:"group"`
	SourceParagraph int    `json:"source_paragraph"`
	GroupGuessed    bool   `json:"group_guessed"`
}

// Plant is a drafted plant; NameGuessed is true when the text named no plant
// and the name is "<Country> plant".
type Plant struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Country      string   `json:"country"`
	Capabilities []string `json:"capabilities"`
	NameGuessed  bool     `json:"name_guessed"`
}

// Question is something the plan search needs that the text cannot answer.
type Question struct {
	PlantID string   `json:"plant_id"`
	Ask     string   `json:"ask"`
	Options []string `json:"options"`
}

// Draft is the editable result.
type Draft struct {
	Product    domain.Product `json:"product"`
	Plants     []Plant        `json:"plants"`
	Components []Row          `json:"components"`
	Operations []Row          `json:"operations"`
	Warnings   []string       `json:"warnings"`
	Questions  []Question     `json:"questions"`
	Paragraphs int            `json:"paragraphs"`
	Trace      domain.Trace   `json:"trace"`
}

type answer struct {
	Product    string `json:"product"`
	Components []struct {
		Name      string `json:"name"`
		Country   string `json:"country"`
		Group     string `json:"group"`
		Paragraph int    `json:"paragraph"`
	} `json:"components"`
	Operations []struct {
		Name      string `json:"name"`
		Country   string `json:"country"`
		Group     string `json:"group"`
		Paragraph int    `json:"paragraph"`
	} `json:"operations"`
	Plants []struct {
		Name    string `json:"name"`
		Country string `json:"country"`
	} `json:"plants"`
	StatedCount *int     `json:"stated_component_count"`
	Notes       []string `json:"notes"`
}

var rowSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"name": map[string]any{"type": "string"}, "country": map[string]any{"type": "string"},
		"group": map[string]any{"type": "string"}, "paragraph": map[string]any{"type": "integer"},
	},
	"required": []string{"name", "country", "group", "paragraph"},
}

var schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"product":    map[string]any{"type": "string"},
		"components": map[string]any{"type": "array", "items": rowSchema},
		"operations": map[string]any{"type": "array", "items": rowSchema},
		"plants": map[string]any{"type": "array", "items": map[string]any{"type": "object",
			"properties": map[string]any{"name": map[string]any{"type": "string"}, "country": map[string]any{"type": "string"}},
			"required":   []string{"name", "country"}}},
		"stated_component_count": map[string]any{"type": []string{"integer", "null"}},
		"notes":                  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	},
	"required": []string{"product", "components", "operations", "plants", "stated_component_count", "notes"},
}

// Extractor drafts products.
type Extractor struct {
	model llm.Model
	id    string
}

// New returns an extractor using modelID.
func New(m llm.Model, modelID string) *Extractor { return &Extractor{model: m, id: modelID} }

// Paragraphs splits facts on blank lines, as CBP restatements are written.
func Paragraphs(facts string) []string {
	var out []string
	for _, p := range regexp.MustCompile(`\n\s*\n`).Split(facts, -1) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Extract drafts a product from facts. tm may be nil (no family): rows then
// have no groups and there are no capability questions.
func (x *Extractor) Extract(ctx context.Context, facts, name string, tm *planner.Template) (Draft, error) {
	paras := Paragraphs(facts)
	if len(paras) == 0 {
		return Draft{}, fmt.Errorf("extract: %w: facts are required", domain.ErrInvalid)
	}
	var b strings.Builder
	if tm != nil {
		b.WriteString("OPERATION GROUPS:\n")
		for _, g := range tm.Groups {
			fmt.Fprintf(&b, "- %s: %s (%s)\n", g.ID, g.Name, strings.Join(g.Operations, "; "))
		}
		b.WriteString("\n")
	}
	b.WriteString("TEXT:\n")
	for i, p := range paras {
		fmt.Fprintf(&b, "[%d] %s\n\n", i+1, corpus.Truncate(p, 4000))
	}
	resp, err := x.model.Generate(ctx, llm.Request{Model: x.id, System: system,
		Messages:   []llm.Message{{Role: llm.RoleUser, Content: b.String()}},
		JSONSchema: schema, SchemaName: "product_draft", MaxOutputTokens: 6000})
	if err != nil {
		return Draft{}, fmt.Errorf("extract: %w", err)
	}
	var a answer
	if err := resp.DecodeJSON(&a); err != nil {
		return Draft{}, fmt.Errorf("extract: %w", err)
	}
	d := build(a, paras, name, tm)
	d.Trace = domain.Trace{Provider: resp.Provider, ModelID: resp.ModelID, PromptVersion: PromptVersion,
		InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens, LatencyMS: resp.Latency.Milliseconds()}
	return d, nil
}

var nonID = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string { return strings.Trim(nonID.ReplaceAllString(strings.ToLower(s), "_"), "_") }

// build turns the model's answer into a checked draft. Pure, for testing.
func build(a answer, paras []string, name string, tm *planner.Template) Draft {
	d := Draft{Plants: []Plant{}, Components: []Row{}, Operations: []Row{}, Warnings: []string{}, Questions: []Question{}, Paragraphs: len(paras)}
	plantFor := map[string]string{} // country -> plant id
	addPlant := func(pname, country string, guessed bool) string {
		if id, ok := plantFor[country]; ok && guessed {
			return id
		}
		id := slug(pname)
		for _, p := range d.Plants {
			if p.ID == id {
				return id
			}
		}
		d.Plants = append(d.Plants, Plant{ID: id, Name: pname, Country: country, Capabilities: []string{}, NameGuessed: guessed})
		if _, ok := plantFor[country]; !ok {
			plantFor[country] = id
		}
		return id
	}
	for _, p := range a.Plants {
		if c := domain.NormalizeCountry(p.Country); c != "" && strings.TrimSpace(p.Name) != "" {
			addPlant(strings.TrimSpace(p.Name), c, false)
		}
	}
	checkGroup := func(g string) (string, bool) {
		if tm == nil {
			return "", false
		}
		g = strings.TrimSpace(g)
		if _, ok := tm.Group(g); ok {
			return g, false
		}
		return "", g != ""
	}
	para := func(n int, what string) int {
		if n < 1 || n > len(paras) {
			d.Warnings = append(d.Warnings, fmt.Sprintf("%q: the paragraph cited (%d) is not in the text", what, n))
			return 0
		}
		return n
	}
	seen := map[string]bool{}
	var noCountry []string
	for _, c := range a.Components {
		n, country := strings.TrimSpace(c.Name), domain.NormalizeCountry(c.Country)
		if n == "" || seen[strings.ToLower(n)] {
			continue
		}
		seen[strings.ToLower(n)] = true
		if country == "" {
			noCountry = append(noCountry, n)
			continue
		}
		g, bad := checkGroup(c.Group)
		row := Row{Name: n, MadeIn: country, Plant: addPlant(country+" plant", country, true), Group: g,
			SourceParagraph: para(c.Paragraph, n), GroupGuessed: tm != nil && (g == "" || bad)}
		d.Components = append(d.Components, row)
	}
	for _, o := range a.Operations {
		n, country := strings.TrimSpace(o.Name), domain.NormalizeCountry(o.Country)
		if n == "" || country == "" {
			continue
		}
		g, bad := checkGroup(o.Group)
		d.Operations = append(d.Operations, Row{Name: n, MadeIn: country, Plant: addPlant(country+" plant", country, true), Group: g,
			SourceParagraph: para(o.Paragraph, n), GroupGuessed: tm != nil && (g == "" || bad)})
	}
	if len(noCountry) > 0 {
		d.Warnings = append(d.Warnings, fmt.Sprintf("The text mentions %s without saying where %s made; add %s if %s count.",
			strings.Join(noCountry, ", "), map[bool]string{true: "it is", false: "they are"}[len(noCountry) == 1],
			map[bool]string{true: "it", false: "them"}[len(noCountry) == 1], map[bool]string{true: "it should", false: "they should"}[len(noCountry) == 1]))
	}
	// Capabilities: what each plant does now; the plan search needs more.
	does := map[string]map[string]bool{}
	for _, r := range append(append([]Row{}, d.Components...), d.Operations...) {
		if r.Group == "" {
			continue
		}
		if does[r.Plant] == nil {
			does[r.Plant] = map[string]bool{}
		}
		does[r.Plant][r.Group] = true
	}
	for i := range d.Plants {
		for g := range does[d.Plants[i].ID] {
			d.Plants[i].Capabilities = append(d.Plants[i].Capabilities, g)
		}
		sort.Strings(d.Plants[i].Capabilities)
		if tm == nil {
			continue
		}
		var opts []string
		for _, g := range tm.Groups {
			if g.Movable && !does[d.Plants[i].ID][g.ID] {
				opts = append(opts, g.ID)
			}
		}
		if len(opts) > 0 {
			d.Questions = append(d.Questions, Question{PlantID: d.Plants[i].ID,
				Ask: fmt.Sprintf("What else could the %s make?", d.Plants[i].Name), Options: opts})
		}
	}
	if a.StatedCount != nil && *a.StatedCount > len(d.Components) {
		d.Warnings = append(d.Warnings, fmt.Sprintf("The text says %d components; it names %d.", *a.StatedCount, len(d.Components)))
	}
	ungrouped := 0
	for _, r := range d.Components {
		if r.Group == "" {
			ungrouped++
		}
	}
	if tm != nil && ungrouped > 0 {
		d.Warnings = append(d.Warnings, fmt.Sprintf("%d component(s) have no operation group; the plan search cannot move them until you pick one.", ungrouped))
	}
	for _, n := range a.Notes {
		if n = strings.TrimSpace(n); n != "" {
			d.Warnings = append(d.Warnings, n)
		}
	}
	pname := strings.TrimSpace(name)
	if pname == "" {
		pname = strings.TrimSpace(a.Product)
	}
	d.Product = domain.Product{Name: pname, Plants: []domain.Plant{}, Components: []domain.Component{}, Operations: []domain.Operation{}}
	if tm != nil {
		d.Product.Family = tm.Family
	}
	for _, p := range d.Plants {
		d.Product.Plants = append(d.Product.Plants, domain.Plant{ID: p.ID, Name: p.Name, Country: p.Country, Capabilities: p.Capabilities})
	}
	for _, r := range d.Components {
		d.Product.Components = append(d.Product.Components, domain.Component{Name: r.Name, MadeIn: r.MadeIn, Plant: r.Plant, Group: r.Group})
	}
	for _, r := range d.Operations {
		d.Product.Operations = append(d.Product.Operations, domain.Operation{Name: r.Name, Plant: r.Plant, Group: r.Group})
	}
	return d
}
