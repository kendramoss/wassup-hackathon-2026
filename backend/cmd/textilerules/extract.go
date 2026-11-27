package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/textilerules"
)

// evalDir holds the committed extractions and results (run from backend/).
const evalDir = "../eval/textile102"

// ExtractPrompt names the extraction prompt below (D951).
const ExtractPrompt = "textile-extract-v2"

// v1 (dev only) had no materials list: a foreign material in a heading the
// rule excepts could not be seen (N352976, covered yarn, 5604).

const extractSystem = `You read a U.S. Customs (CBP) ruling text about a textile or apparel good and copy out facts about how it was made. You never decide a country of origin and you ignore any holding or conclusion in the text: you only report the operations and where the text says they took place.

If the text covers several goods or several manufacturing scenarios, use the FIRST good and the FIRST scenario described.

Return JSON:
- good_hts: the HTSUS code the text gives for the finished good, copied exactly as written (e.g. "6104.43.2020"), or "" if none. good_hts_quote: the shortest verbatim fragment of the text that contains the code, or "".
- suggested_hts: the six-digit HS subheading (e.g. "6104.43") you would classify the finished good in, from its description, or "" if you cannot tell. This is a guess for use only when the text gives no code; say it as you would a broker's first suggestion.
- materials: each material the text names as going into the good (fabric, yarn, filaments, thread, rubber thread, lining, trim; not packaging): name (short, from the text); hts (the HTSUS code or heading the text gives for THAT material, e.g. "5402" or "heading 5402", copied as written, "" if none is given for it; never a code given for another item); hts_quote (verbatim fragment containing it, or ""); country (the country name the text says it was made in or comes from; "" if not stated or a placeholder).
- knit_to_shape: "yes" only if the text says the major parts of the good are knitted or crocheted directly to the shape used in the good (fully fashioned or seamless knit garments, gloves knit to shape); "no" only if the text says or plainly shows the good is made from fabric that is cut to shape (cut and sewn), or is woven, or is not knit; otherwise "not_stated". knit_to_shape_quote: verbatim fragment supporting a yes or no, else "".
- two_or_more_components: "yes" if the text says the good is made by joining two or more cut or separate component parts (panels, front and back, sleeves, body and lining); "no" if it is made from a single piece of fabric or one component (for example a scarf or towel cut and hemmed from one piece); otherwise "not_stated". components_quote: verbatim fragment, else "".
- cotton_wool_16: "yes" if the text says the good (or its fabric) is of cotton or of wool, or a blend with 16 percent or more cotton by weight; "no" if the fibre content is stated and is none of those (for example 100 percent polyester, or cotton under 16 percent); otherwise "not_stated". fibre_quote: verbatim fragment with the fibre content, else "".
- fabric_of_wool: "yes" if the fabric is of wool or fine animal hair, "no" if the fabric's fibre is stated and is neither, else "not_stated".
- fibre_form: "filaments" if the text says the yarn or good is of continuous filaments, "staple" if of staple fibres or spun yarn, else "not_stated".
- raw_material_country: the country where the raw fibre or polymer is stated to be grown or made, as a country name, else "".
- stages: every manufacturing operation the text describes, in the order it happens. Each: process (one of: spinning = staple fibre spun into yarn; extrusion = filaments or yarn extruded from polymer; fabric_making = fabric knitted, woven, crocheted, felted or made nonwoven; knit_to_shape = parts knitted directly to shape; dyeing; printing; finishing = one finishing operation, with op naming it exactly as one of: bleaching, shrinking, fulling, napping, decating, permanent stiffening, weighting, permanent embossing, moireing; cutting; assembly = sewing or joining the components into the finished good; embroidery; packing; other), op (only for finishing, else ""), country (the country NAME the text says it took place in; "" if the text gives a placeholder such as "Country A" or no country), quote (a verbatim fragment of the text that describes the operation).
- complete: true only if the text gives a country for every operation from the formation of the fabric (or yarn, if the text starts there) to the finished good, and no such operation is described without one; false otherwise, including whenever the fabric, yarn or any component is simply "imported" or "sourced" with no country.

Copy codes and quotes exactly. Do not infer a code, a country or an operation the text does not state.`

var extractSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"good_hts":               map[string]any{"type": "string"},
		"good_hts_quote":         map[string]any{"type": "string"},
		"suggested_hts":          map[string]any{"type": "string"},
		"knit_to_shape":          map[string]any{"type": "string", "enum": []string{"yes", "no", "not_stated"}},
		"knit_to_shape_quote":    map[string]any{"type": "string"},
		"two_or_more_components": map[string]any{"type": "string", "enum": []string{"yes", "no", "not_stated"}},
		"components_quote":       map[string]any{"type": "string"},
		"cotton_wool_16":         map[string]any{"type": "string", "enum": []string{"yes", "no", "not_stated"}},
		"fibre_quote":            map[string]any{"type": "string"},
		"fabric_of_wool":         map[string]any{"type": "string", "enum": []string{"yes", "no", "not_stated"}},
		"fibre_form":             map[string]any{"type": "string", "enum": []string{"filaments", "staple", "not_stated"}},
		"raw_material_country":   map[string]any{"type": "string"},
		"stages": map[string]any{"type": "array", "items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"process": map[string]any{"type": "string", "enum": []string{"spinning", "extrusion", "fabric_making", "knit_to_shape", "dyeing", "printing", "finishing", "cutting", "assembly", "embroidery", "packing", "other"}},
				"op":      map[string]any{"type": "string"},
				"country": map[string]any{"type": "string"},
				"quote":   map[string]any{"type": "string"},
			},
			"required": []string{"process", "op", "country", "quote"},
		}},
		"materials": map[string]any{"type": "array", "items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":      map[string]any{"type": "string"},
				"hts":       map[string]any{"type": "string"},
				"hts_quote": map[string]any{"type": "string"},
				"country":   map[string]any{"type": "string"},
			},
			"required": []string{"name", "hts", "hts_quote", "country"},
		}},
		"complete": map[string]any{"type": "boolean"},
	},
	"required": []string{"good_hts", "good_hts_quote", "suggested_hts", "knit_to_shape", "knit_to_shape_quote", "two_or_more_components", "components_quote",
		"cotton_wool_16", "fibre_quote", "fabric_of_wool", "fibre_form", "raw_material_country", "stages", "materials", "complete"},
}

type extracted struct {
	GoodHTS      string `json:"good_hts"`
	GoodHTSQuote string `json:"good_hts_quote"`
	Suggested    string `json:"suggested_hts"`
	KTS          string `json:"knit_to_shape"`
	KTSQuote     string `json:"knit_to_shape_quote"`
	Comps        string `json:"two_or_more_components"`
	CompsQuote   string `json:"components_quote"`
	CW           string `json:"cotton_wool_16"`
	FibreQuote   string `json:"fibre_quote"`
	Wool         string `json:"fabric_of_wool"`
	FibreForm    string `json:"fibre_form"`
	RawCountry   string `json:"raw_material_country"`
	Stages       []struct {
		Process string `json:"process"`
		Op      string `json:"op"`
		Country string `json:"country"`
		Quote   string `json:"quote"`
	} `json:"stages"`
	Materials []struct {
		Name     string `json:"name"`
		HTS      string `json:"hts"`
		HTSQuote string `json:"hts_quote"`
		Country  string `json:"country"`
	} `json:"materials"`
	Complete bool `json:"complete"`
}

// InputRow is one committed extraction: the check's facts and provenance.
type InputRow struct {
	Ruling string             `json:"ruling"`
	Split  string             `json:"split"`
	Source string             `json:"source"` // "facts" or "full_text"
	Facts  textilerules.Facts `json:"facts"`
	// Suggested is the model's guess at the good's subheading, a guess and
	// not a quote; read only in the "+suggested" evaluation.
	Suggested string   `json:"suggested_hts,omitempty"`
	Dropped   []string `json:"dropped,omitempty"` // fields or stages whose quote failed the check
	Error     string   `json:"error,omitempty"`
	Model     string   `json:"model,omitempty"`
	Prompt    string   `json:"prompt"`
	CostUSD   float64  `json:"cost_usd"`
}

func cmdExtract(args []string) error {
	fl := flag.NewFlagSet("extract", flag.ExitOnError)
	split := fl.String("split", "dev", "dev, final, final2, older or pool (older and pool are D955)")
	source := fl.String("source", "facts", "facts or full_text")
	raw := fl.String("raw", "", "directory of raw CROSS ruling JSON (full_text only; not committed)")
	model := fl.String("model", "gemini-3.8-flash", "gateway model")
	workers := fl.Int("workers", 4, "parallel calls")
	confirm := fl.Bool("confirm-heldout", false, "required for final and final2 (D951: once)")
	retry := fl.Bool("retry-errors", false, "development only: re-run the rows of an existing file that failed")
	force := fl.Bool("force", false, "development only: overwrite an existing file")
	_ = fl.Parse(args)
	if (*retry || *force) && *split != "dev" {
		return errors.New("-retry-errors and -force are for the development split only")
	}
	if *split != "dev" && !*confirm {
		return errors.New("held-out extraction runs once (D951); pass -confirm-heldout")
	}
	if *source == "full_text" && *raw == "" {
		return errors.New("-raw is required for full_text")
	}
	base, key := os.Getenv("LITELLM_BASE_URL"), os.Getenv("LITELLM_API_KEY")
	if base == "" || key == "" {
		return errors.New("needs LITELLM_BASE_URL and LITELLM_API_KEY")
	}
	ids, err := splitIDs(*split)
	if err != nil {
		return err
	}
	out := filepath.Join(evalDir, fmt.Sprintf("inputs-%s-%s.jsonl", *split, *source))
	var prev []InputRow
	if _, err := os.Stat(out); err == nil {
		switch {
		case *retry:
			if prev, err = readInputs(out); err != nil {
				return err
			}
		case !*force:
			return fmt.Errorf("%s exists; extraction runs once", out)
		}
	}
	ll := llm.NewLiteLLM(base, key, 180*time.Second)
	ll.SkipCache(true)
	ctx := context.Background()
	rows := make([]InputRow, len(ids))
	var wg sync.WaitGroup
	sem := make(chan struct{}, *workers)
	for i, id := range ids {
		if prev != nil && prev[i].Ruling == id && prev[i].Error == "" {
			rows[i] = prev[i]
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			rows[i] = extractOne(ctx, ll, *model, id, *split, *source, *raw)
		}()
	}
	wg.Wait()
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	errs, cost, called := 0, 0.0, 0
	for i, r := range rows {
		fresh := prev == nil || prev[i].Error != ""
		if fresh {
			called++
			cost += r.CostUSD
		}
		if r.Error != "" {
			errs++
		}
		b, _ := json.Marshal(r)
		w.Write(append(b, '\n'))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	runlog := map[string]any{
		"run": fmt.Sprintf("textile102-extract-%s-%s", *split, *source), "prereg": "D951",
		"cases": len(ids), "calls": called, "retry_errors": *retry, "errors": errs, "cost_usd": round4(cost), "model": *model,
		"prompt": ExtractPrompt, "llm_provider": "litellm", "llm_no_cache": true,
		"held_out": *split != "dev", "split": *split, "source": *source,
		"git_revision": revision(), "time": time.Now().Format(time.RFC3339),
		"output": "eval/textile102/" + filepath.Base(out),
	}
	if err := appendRunlog(runlog); err != nil {
		return err
	}
	fmt.Printf("%s: %d rulings, %d errors, $%.4f\n", out, len(ids), errs, cost)
	return nil
}

func extractOne(ctx context.Context, ll *llm.LiteLLM, model, id, split, source, rawDir string) InputRow {
	row := InputRow{Ruling: id, Split: split, Source: source, Prompt: ExtractPrompt, Model: model}
	text, err := readText(id, split, source, rawDir)
	if err != nil {
		row.Error = err.Error()
		return row
	}
	t := &llm.CostTally{}
	cctx := llm.ContextWithCostTally(ctx, t)
	var resp llm.Response
	for attempt := 0; attempt < 2; attempt++ {
		resp, err = ll.Generate(cctx, llm.Request{
			Model: model, System: extractSystem,
			Messages:        []llm.Message{{Role: llm.RoleUser, Content: text}},
			JSONSchema:      extractSchema,
			SchemaName:      "textile102_extract",
			MaxOutputTokens: 8192,
		})
		if err == nil {
			break
		}
	}
	row.CostUSD = round4(t.USD())
	if err != nil {
		row.Error = "model call failed: " + err.Error()
		return row
	}
	var x extracted
	if err := resp.DecodeJSON(&x); err != nil {
		row.Error = "bad JSON: " + err.Error()
		return row
	}
	row.Facts, row.Dropped = toFacts(x, text)
	row.Suggested = strings.TrimSpace(x.Suggested)
	fmt.Fprintf(os.Stderr, "%s %s %s $%.4f\n", split, source, id, row.CostUSD)
	return row
}

// toFacts turns an extraction into the check's facts. Every claim needs a
// verbatim quote from the text: a yes or no without one becomes not stated, a
// code whose quote lacks its digits is dropped, and a stage without a
// verbatim quote is dropped, which also clears Complete.
func toFacts(x extracted, text string) (textilerules.Facts, []string) {
	norm := normText(text)
	var dropped []string
	in := func(q string) bool { q = normText(q); return q != "" && strings.Contains(norm, q) }
	tri := func(v, q, field string) textilerules.Tri {
		var t textilerules.Tri
		_ = t.UnmarshalText([]byte(v))
		if t == textilerules.Unknown {
			return t
		}
		if !in(q) {
			dropped = append(dropped, field)
			return textilerules.Unknown
		}
		return t
	}
	f := textilerules.Facts{}
	if code := strings.TrimSpace(x.GoodHTS); code != "" {
		if d := textilerules.Digits(code); d != "" && in(x.GoodHTSQuote) && strings.Contains(digitsOnly(normText(x.GoodHTSQuote)), d) {
			f.GoodHTS = code
		} else {
			dropped = append(dropped, "good_hts")
		}
	}
	f.KnitToShape = tri(x.KTS, x.KTSQuote, "knit_to_shape")
	f.TwoOrMoreComponents = tri(x.Comps, x.CompsQuote, "two_or_more_components")
	f.CottonWool16 = tri(x.CW, x.FibreQuote, "cotton_wool_16")
	// The fibre quote also supports the wool fact: both are read from it.
	f.FabricOfWool = tri(x.Wool, x.FibreQuote, "fabric_of_wool")
	if x.FibreForm == "filaments" || x.FibreForm == "staple" {
		f.Fibre = x.FibreForm
	}
	f.RawMaterialCountry = strings.TrimSpace(x.RawCountry)
	f.Complete = x.Complete
	for _, s := range x.Stages {
		if !in(s.Quote) {
			dropped = append(dropped, "stage:"+s.Process)
			f.Complete = false
			continue
		}
		if strings.TrimSpace(s.Country) == "" {
			f.Complete = false
		}
		f.Stages = append(f.Stages, textilerules.Stage{Process: s.Process, Op: s.Op, Country: strings.TrimSpace(s.Country), Quote: strings.TrimSpace(s.Quote)})
	}
	for _, m := range x.Materials {
		mat := textilerules.Material{Name: strings.TrimSpace(m.Name), Country: strings.TrimSpace(m.Country)}
		if code := strings.TrimSpace(m.HTS); code != "" {
			if d := textilerules.Digits(code); d != "" && in(m.HTSQuote) && strings.Contains(digitsOnly(normText(m.HTSQuote)), d) {
				mat.HTS = code
			} else {
				dropped = append(dropped, "material:"+m.Name)
			}
		}
		f.Materials = append(f.Materials, mat)
	}
	if len(f.Stages) == 0 {
		f.Complete = false
	}
	return f, dropped
}

func normText(s string) string {
	s = strings.NewReplacer("“", `"`, "”", `"`, "’", "'", "‘", "'", " ", " ").Replace(s)
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func digitsOnly(s string) string { return textilerules.Digits(s) }

func readText(id, split, source, rawDir string) (string, error) {
	if source == "full_text" {
		b, err := os.ReadFile(filepath.Join(rawDir, id+".json"))
		if err != nil {
			return "", err
		}
		var r struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(b, &r); err != nil {
			return "", err
		}
		if r.Text == "" {
			return "", fmt.Errorf("%s: empty text", id)
		}
		return r.Text, nil
	}
	dir := "data/facts"
	switch split {
	case "final2":
		dir = "../eval/data/facts_final2"
	case "older":
		dir = "data/facts_older"
	}
	b, err := os.ReadFile(filepath.Join(dir, id+".txt"))
	return string(b), err
}

func round4(x float64) float64 { return math.Round(x*10000) / 10000 }

func appendRunlog(entry map[string]any) error {
	f, err := os.OpenFile("../eval/results/RUNLOG.jsonl", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}

func revision() string {
	out, err := exec.Command("git", "rev-parse", "--short=8", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	rev := strings.TrimSpace(string(out))
	if st, err := exec.Command("git", "status", "--porcelain", "--untracked-files=no", "--", ".").Output(); err == nil && len(strings.TrimSpace(string(st))) > 0 {
		rev += "-dirty"
	}
	return rev
}
