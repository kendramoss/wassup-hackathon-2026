package hts

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Plausibility check on per-part codes (D471). The agent evaluation (D451,
// a12) saw codes from the part index that no reader would accept for a car
// part: heading 1704 (sugar confectionery) for a "centre bar" and 6401
// (rubber footwear) for "CV boots". The check is deterministic and only
// flags: a flagged code stays in the list, in its place, with a note asking
// the user to check it. It never removes or reorders a suggestion.

// farChapters are HTSUS chapters no part of a product in any of the planner
// families (machinery, electrical goods, vehicles, instruments, furniture,
// tools) is expected to fall in: food and farm goods, ores, fertilisers,
// explosives, furskins, clothing, footwear and headgear, precious metals and
// jewellery, musical instruments, arms and works of art.
var farChapters = map[int]string{
	1: "live animals", 2: "meat", 3: "fish", 4: "dairy, eggs and honey", 5: "other animal products",
	6: "live plants", 7: "vegetables", 8: "fruit and nuts", 9: "coffee, tea and spices", 10: "cereals",
	11: "milling products", 12: "oil seeds", 13: "lacs, gums and resins", 14: "vegetable plaiting materials",
	15: "fats and oils", 16: "meat and fish preparations", 17: "sugars and sugar confectionery",
	18: "cocoa", 19: "cereal and bakery preparations", 20: "vegetable and fruit preparations",
	21: "miscellaneous food preparations", 22: "beverages and spirits", 23: "food industry residues and animal feed",
	24: "tobacco", 26: "ores, slag and ash", 31: "fertilisers", 36: "explosives and matches", 43: "furskins",
	61: "knitted clothing", 62: "woven clothing", 64: "footwear", 65: "headgear", 66: "umbrellas and walking sticks",
	67: "feathers, artificial flowers and wigs", 71: "precious metals, stones and jewellery",
	92: "musical instruments", 93: "arms and ammunition", 97: "works of art and antiques",
}

// familyAllows are the far chapters a family's parts may still fall in.
var familyAllows = map[string][]int{
	"furniture":       {5, 14, 67},  // down filling, rattan, feathers
	"medical_devices": {61, 62, 65}, // gowns, caps
}

// materialChapters are the chapters of a material a part's name may state.
// A code in another material's chapter is flagged (a "steel bracket" coded
// as plastics); machinery, electrical and other article chapters never are.
var materialChapters = []struct {
	words *regexp.Regexp
	name  string
	ch    []int
}{
	{regexp.MustCompile(`\b(plastics?|nylon|abs|pvc|polypropylene|polyethylene|polycarbonate|acrylic|polystyrene|polyester|polyurethane|vinyl|epoxy|phenolic|resin)\b`), "plastic", []int{39}},
	{regexp.MustCompile(`\b(rubber|silicone)\b`), "rubber", []int{39, 40}},
	{regexp.MustCompile(`\b(steel|iron|stainless)\b`), "steel or iron", []int{72, 73}},
	{regexp.MustCompile(`\b(aluminum|aluminium)\b`), "aluminium", []int{76}},
	{regexp.MustCompile(`\b(copper|brass|bronze)\b`), "copper", []int{74}},
	{regexp.MustCompile(`\bglass\b`), "glass", []int{70}},
	{regexp.MustCompile(`\b(wood|wooden|plywood|timber)\b`), "wood", []int{44}},
	{regexp.MustCompile(`\b(paper|cardboard|paperboard|carton|cartons)\b`), "paper", []int{48}},
	{regexp.MustCompile(`\b(metal|metallic)\b`), "metal", []int{72, 73, 74, 75, 76, 78, 79, 80, 81, 82, 83}},
}

// coating is a material named only as a coating or plating ("rubber-coated
// handle"): it says nothing about the article's own material.
var coating = regexp.MustCompile(`\b[a-z]+[- ](coated|plated|clad|lined|covered|faced)\b`)

var materialChapterName = map[int]string{39: "plastics", 40: "rubber", 44: "wood", 48: "paper and paperboard", 70: "glass", 72: "iron and steel", 73: "articles of iron or steel", 74: "copper", 76: "aluminium"}

// preciousMaterial is true for headings 7106 to 7112: silver, gold and
// platinum unwrought or as powder, paste, wire or foil, clad metals and
// waste, which electronics use (silver paste, silver solder). The rest of
// chapter 71 (stones, pearls, jewellery, articles of precious metal) stays
// far.
func preciousMaterial(code string) bool {
	d := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, code)
	if len(d) < 4 {
		return false
	}
	h, _ := strconv.Atoi(d[:4])
	return h >= 7106 && h <= 7112
}

// Chapter is the HTSUS chapter of a code (its first two digits), or 0.
func Chapter(code string) int {
	d := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, code)
	if len(d) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(d[:2])
	return n
}

// PlausibilityNote is why a part's code looks wrong for the product, or ""
// when nothing does. family is the product's planner family ("" unknown);
// goodChapter the product's own chapter (its hts, else its top suggestion; 0
// unknown). With no family, the far chapters are checked only when the good
// is in chapters 72 to 96 (articles of base metal through miscellaneous
// manufactured articles), so a food or chemical product's parts are not.
func PlausibilityNote(family string, goodChapter int, part, code string) string {
	ch := Chapter(code)
	if ch == 0 {
		return ""
	}
	if what, far := farChapters[ch]; far && ch != goodChapter && !preciousMaterial(code) {
		allowed := false
		for _, a := range familyAllows[family] {
			allowed = allowed || a == ch
		}
		switch {
		case allowed:
		case family != "":
			return fmt.Sprintf("Chapter %02d (%s) is unusual for a part of this product family (%s); check this code.", ch, what, family)
		case goodChapter >= 72 && goodChapter <= 96:
			return fmt.Sprintf("Chapter %02d (%s) is far from the product's chapter %02d; check this code.", ch, what, goodChapter)
		}
	}
	if name, isMat := materialChapterName[ch]; isMat {
		lower := coating.ReplaceAllString(strings.ToLower(part), " ")
		var said []string
		for _, m := range materialChapters {
			if !m.words.MatchString(lower) {
				continue
			}
			for _, c := range m.ch {
				if c == ch {
					return ""
				}
			}
			said = append(said, m.name)
		}
		if len(said) > 0 {
			return fmt.Sprintf("The part's name says %s, but chapter %02d is %s; check this code.", strings.Join(said, " and "), ch, name)
		}
	}
	return ""
}

// FlagImplausible sets Check on every part candidate PlausibilityNote
// flags. Each part's candidates are copied first (they may be the cache's
// own slice). Nothing is removed or reordered. It returns how many
// candidates it flagged.
func FlagImplausible(res *PartsResult, prod domain.Product) int {
	good := Chapter(prod.HTS)
	if good == 0 && res.Good != nil && len(res.Good.Candidates) > 0 {
		good = Chapter(res.Good.Candidates[0].HTS)
	}
	n := 0
	for i := range res.Parts {
		res.Parts[i].Candidates = slices.Clone(res.Parts[i].Candidates)
		for j := range res.Parts[i].Candidates {
			c := &res.Parts[i].Candidates[j]
			if c.Check = PlausibilityNote(prod.Family, good, res.Parts[i].Name, c.HTS); c.Check != "" {
				n++
			}
		}
	}
	return n
}
