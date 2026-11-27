package textilerules

import (
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// opWords maps words in an operation's name to the operations of 102.21.
// The order is the order of the checks; a name can match several (a
// "cut and sew" line is cutting and assembly).
var opWords = []struct {
	words []string
	stage Stage
}{
	{[]string{"knit to shape", "knitted to shape", "fully fashion", "seamless knit", "linking"}, Stage{Process: ProcKnitToShape}},
	{[]string{"knit", "weav", "woven", "crochet", "nonwoven", "non-woven", "felt", "fabric formation", "fabric production", "fabric making", "fabric-making"}, Stage{Process: ProcFabricMaking}},
	{[]string{"spin"}, Stage{Process: ProcSpinning}},
	{[]string{"extru"}, Stage{Process: ProcExtrusion}},
	{[]string{"dye"}, Stage{Process: StageDyeing}},
	{[]string{"print"}, Stage{Process: StagePrint}},
	{[]string{"bleach"}, Stage{Process: StageFinish, Op: "bleaching"}},
	{[]string{"shrink", "sanfor"}, Stage{Process: StageFinish, Op: "shrinking"}},
	{[]string{"fulling"}, Stage{Process: StageFinish, Op: "fulling"}},
	{[]string{"napping", "brushing"}, Stage{Process: StageFinish, Op: "napping"}},
	{[]string{"decat"}, Stage{Process: StageFinish, Op: "decating"}},
	{[]string{"stiffen"}, Stage{Process: StageFinish, Op: "permanent stiffening"}},
	{[]string{"weighting"}, Stage{Process: StageFinish, Op: "weighting"}},
	{[]string{"emboss"}, Stage{Process: StageFinish, Op: "permanent embossing"}},
	{[]string{"moire"}, Stage{Process: StageFinish, Op: "moireing"}},
	{[]string{"cut"}, Stage{Process: StageCutting}},
	{[]string{"sew", "stitch", "assembl", "garment making", "cmt", "make-up", "make up"}, Stage{Process: ProcAssembly}},
	{[]string{"embroider"}, Stage{Process: ProcEmbroidery}},
	{[]string{"pack"}, Stage{Process: StagePacking}},
}

// FromProduct reads a product into the check's facts: its HTS code, the
// operations in order with the country of each plant, and the optional
// textile facts. notes lists what could not be read. Complete is true only
// when every operation was recognised and has a plant with a country.
func FromProduct(p domain.Product) (Facts, []string) {
	f := Facts{GoodHTS: p.HTS}
	var notes []string
	complete := len(p.Operations) > 0
	for _, o := range p.Operations {
		pl, ok := p.PlantByID(o.Plant)
		country := ""
		if ok {
			country = pl.Country
		}
		if country == "" {
			complete = false
			notes = append(notes, "operation "+o.Name+" has no plant country")
		}
		name := strings.ToLower(o.Name)
		matched, kts := false, false
		for _, w := range opWords {
			if w.stage.Process == ProcFabricMaking && kts { // "knit to shape" is not also fabric-making
				continue
			}
			for _, x := range w.words {
				if strings.Contains(name, x) {
					s := w.stage
					s.Country = country
					f.Stages = append(f.Stages, s)
					matched, kts = true, kts || s.Process == ProcKnitToShape
					break
				}
			}
		}
		if !matched {
			complete = false
			notes = append(notes, "operation "+o.Name+" is not one the textile rules read")
		}
	}
	f.Complete = complete
	if t := p.Textile; t != nil {
		f.KnitToShape, f.TwoOrMoreComponents = tri(t.KnitToShape), tri(t.TwoOrMoreComponents)
		f.CottonWool16, f.FabricOfWool = tri(t.CottonWool16), tri(t.FabricOfWool)
		f.Fibre = t.FibreForm
	}
	return f, notes
}

func tri(s string) Tri {
	var t Tri
	_ = t.UnmarshalText([]byte(s))
	return t
}
