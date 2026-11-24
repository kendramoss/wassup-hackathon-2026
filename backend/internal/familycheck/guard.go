package familycheck

import (
	"regexp"
	"strings"
)

// GuardVersion names the deterministic checks below (D783). A change to a
// rule needs a new version, like a prompt change.
const GuardVersion = "family-guard-v1"

// guardRule sends a model's match to "none" (review) when the product line
// names a product that only shares a cue with the family: a cordless drill is
// not a battery pack, a floor lamp is not furniture, a tablet is not a medical
// device, toy binoculars are not an optical instrument (#766-#769). The rules
// only ever take a match away; they never pick a family, so a wrong rule costs
// a plan, never a wrong template.
type guardRule struct {
	id       string
	families []string
	// match reports whether the rule fires for one product phrase (lower
	// case) and its head noun (the phrase's last word, singular).
	match  func(phrase, head string) bool
	reason string
}

var (
	batteryCue = regexp.MustCompile(`batter|cordless|rechargeable|lithium|li-ion`)
	// Head nouns of goods the power supplies, chargers and battery packs
	// template covers.
	powerHeads = words("battery batterie cell pack bank station starter charger adapter supply module storage system converter inverter generator bms unit dock cradle")
	materialRe = regexp.MustCompile(`cathode|anode|electrode|graphite|electrolyte|active material|precursor`)
	matHeads   = words("material powder graphite electrolyte precursor foil slurry")
	lightHeads = words("lamp light lantern chandelier sconce luminaire bulb lighting")
	petRe      = regexp.MustCompile(`\b(pet|dog|cat|puppy|kitten)s?\b`)
	elecCabRe  = regexp.MustCompile(`distribution|switchgear|electrical|electric panel|server|network|telecom|battery`)
	cabHeads   = words("cabinet rack enclosure box")
	drugRe     = regexp.MustCompile(`\b(tablet|capsule|drug|medicine|medication|vaccine|ointment|cream|syrup|supplement|vitamin|gel|lozenge)s?\b|active pharmaceutical|\bapi\b`)
	drugHeads  = words("tablet capsule drug medicine medication vaccine ointment cream syrup supplement vitamin gel lozenge solution api ingredient")
	toyRe      = regexp.MustCompile(`\btoys?\b|novelty|\bkids?'?\b|children'?s?|\bplay\b|\bplayset\b`)
	toolHeads  = words("drill sander mower trimmer blender nailer")
	vehHeads   = words("scooter bike bicycle e-bike car truck vehicle motorcycle cart moped kart")
)

var guardRules = []guardRule{
	{"battery-material", []string{"power_supplies_chargers"},
		func(p, h string) bool { return materialRe.MatchString(p) && matHeads[h] },
		"a battery material (a chemical input) is not a battery, charger or power supply"},
	{"battery-device", []string{"power_supplies_chargers"},
		func(p, h string) bool { return batteryCue.MatchString(p) && !powerHeads[h] && !matHeads[h] },
		"a device that runs on a battery is not a battery pack or power supply: plan it under the device's own family"},
	{"vehicle", []string{"power_supplies_chargers", "motors_pumps_compressors"},
		func(_, h string) bool { return vehHeads[h] },
		"a vehicle is not a battery pack or an industrial machine"},
	{"hand-tool-or-appliance", []string{"motors_pumps_compressors"},
		func(_, h string) bool { return toolHeads[h] },
		"a hand-held power tool or small appliance belongs with motorised tools and small appliances, not industrial machines"},
	{"lighting", []string{"furniture"},
		func(_, h string) bool { return lightHeads[h] },
		"a lamp or light fitting is lighting, not furniture"},
	{"pet-product", []string{"furniture"},
		func(p, _ string) bool { return petRe.MatchString(p) },
		"a pet bed or pet product is not furniture (D138 called this match a stretch)"},
	{"electrical-cabinet", []string{"furniture"},
		func(p, h string) bool { return cabHeads[h] && elecCabRe.MatchString(p) },
		"an electrical, server or battery cabinet is electrical apparatus, not furniture"},
	{"drug", []string{"medical_devices", "display_camera_modules"},
		func(p, h string) bool { return drugRe.MatchString(p) && drugHeads[h] },
		"a drug or supplement is not a device; pharmaceutical origin has its own engine"},
	{"toy", []string{"medical_devices", "display_camera_modules"},
		func(p, _ string) bool { return toyRe.MatchString(p) },
		"a toy or novelty with optics or a medical look is not a medical device or optical instrument"},
}

// GuardRules lists the rule ids, for tests and the eval.
func GuardRules() []string {
	out := make([]string, len(guardRules))
	for i, g := range guardRules {
		out[i] = g.id
	}
	return out
}

// Guard applies the deterministic checks to a model's answer for text (the
// input Check was given). It returns the result, with the family taken away
// when a rule fires, and the rule's id ("" when none fired).
func Guard(text string, r Result) (Result, string) {
	if r.Family == None {
		return r, ""
	}
	var phrases []string
	for _, p := range []string{ProductLine(text), r.ProductName} {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			phrases = append(phrases, p)
		}
	}
	for _, g := range guardRules {
		if !contains(g.families, r.Family) {
			continue
		}
		for _, p := range phrases {
			if g.match(p, headNoun(p)) {
				from := r.Family
				r.Family, r.Name = None, ""
				r.Reason = "Not matched to " + from + ": " + g.reason + " (" + GuardVersion + ", rule " + g.id + "). Model reason: " + r.Reason
				return r, g.id
			}
		}
	}
	return r, ""
}

// ProductLine is the line of text that names the product: the "Typed:",
// "Product name:" or "Product:" line, else the first non-empty line.
func ProductLine(text string) string {
	first := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, pre := range []string{"Typed:", "Product name:", "Product:"} {
			if strings.HasPrefix(line, pre) {
				return strings.TrimSpace(strings.TrimPrefix(line, pre))
			}
		}
		if first == "" {
			first = line
		}
	}
	return first
}

var tokenRe = regexp.MustCompile(`[a-z][a-z'-]*`)

// headNoun is the last word of a product phrase before any "for", "with",
// "from" or comma clause and after the last "of", in the singular:
// "Cordless drill with two batteries" -> "drill", "Floor lamps" -> "lamp",
// "Set of chairs" and "The country of origin of two chairs" -> "chair".
func headNoun(p string) string {
	for _, sep := range []string{",", ";", "(", " for ", " with ", " from ", " - "} {
		if i := strings.Index(p, sep); i > 0 {
			p = p[:i]
		}
	}
	if i := strings.LastIndex(p, " of "); i >= 0 {
		p = p[i+len(" of "):]
	}
	toks := tokenRe.FindAllString(p, -1)
	if len(toks) == 0 {
		return ""
	}
	w := strings.Trim(toks[len(toks)-1], "'-")
	switch {
	case strings.HasSuffix(w, "ies") && len(w) > 4:
		w = w[:len(w)-3] + "y"
	case strings.HasSuffix(w, "ches") || strings.HasSuffix(w, "shes") || strings.HasSuffix(w, "xes"):
		w = w[:len(w)-2]
	case strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && len(w) > 3:
		w = w[:len(w)-1]
	}
	return w
}

func words(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
