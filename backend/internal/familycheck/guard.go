package familycheck

import (
	"regexp"
	"strings"
)

// GuardVersion names the deterministic checks below (D783). A change to a
// rule needs a new version, like a prompt change.
const GuardVersion = "family-guard-v2"

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
	powerHeads = words("battery batterie cell pack bank station starter charger adapter supply module storage system converter inverter generator bms unit dock cradle board backup ups")
	materialRe = regexp.MustCompile(`cathode|anode|electrode|graphite|electrolyte|active material|precursor`)
	matHeads   = words("material powder graphite electrolyte precursor foil slurry")
	lightHeads = words("lamp light lantern chandelier sconce luminaire bulb lighting")
	petWords   = words("pet dog cat puppy kitten")
	petHeads   = words("bed house tree crate kennel")
	petRe      = regexp.MustCompile(`\b(pet|dog|cat|puppy|kitten)s?\b`)
	elecCabRe  = regexp.MustCompile(`distribution|switchgear|electrical|electric panel|server|network|telecom|battery`)
	cabHeads   = words("cabinet rack enclosure box")
	drugRe     = regexp.MustCompile(`\b(tablet|capsule|drug|medicine|medication|vaccine|ointment|cream|syrup|supplement|vitamin|lozenge)s?\b|active pharmaceutical|\bapi\b`)
	drugHeads  = words("tablet capsule drug medicine medication vaccine ointment cream syrup supplement vitamin lozenge api ingredient")
	toyRe      = regexp.MustCompile(`\btoys?\b|\bnovelty\b`)
	toolHeads  = words("drill sander mower trimmer blender nailer")
	proRe      = regexp.MustCompile(`industrial|commercial`)
	// Trucks and carts are left out: a hydraulic pallet truck is a machine.
	vehHeads = words("scooter bike bicycle e-bike car vehicle motorcycle moped kart")
	// Unit words read as the last word of "12V 100Ah" or "10000 mAh".
	unitWords = words("v w ah mah wh kwh kw mp mm cm m a va vdc vac in ft lb kg g")
)

var guardRules = []guardRule{
	{"battery-material", []string{"power_supplies_chargers"},
		func(p, h string) bool { return materialRe.MatchString(p) && matHeads[h] },
		"a battery material (a chemical input) is not a battery, charger or power supply"},
	{"battery-device", []string{"power_supplies_chargers"},
		func(p, h string) bool { return batteryCue.MatchString(p) && !powerHeads[h] && !matHeads[h] },
		"a device that runs on a battery is not a battery pack or power supply; choose the device's own family"},
	{"vehicle", []string{"power_supplies_chargers", "motors_pumps_compressors"},
		func(_, h string) bool { return vehHeads[h] },
		"a vehicle is not a battery pack or an industrial machine"},
	{"hand-tool-or-appliance", []string{"motors_pumps_compressors"},
		func(p, h string) bool { return toolHeads[h] && !proRe.MatchString(p) },
		"a hand-held power tool or small appliance belongs with motorised tools and small appliances, not industrial machines"},
	{"lighting", []string{"furniture"},
		func(_, h string) bool { return lightHeads[h] },
		"a lamp or light fitting is lighting, not furniture"},
	{"pet-product", []string{"furniture"},
		func(p, h string) bool { return petBefore(p, h) || petHeads[h] && petRe.MatchString(p) },
		"a pet bed or other pet product is not furniture"},
	{"electrical-cabinet", []string{"furniture"},
		func(p, h string) bool { return cabHeads[h] && elecCabRe.MatchString(p) },
		"an electrical, server or battery cabinet is electrical apparatus, not furniture"},
	{"drug", []string{"medical_devices", "display_camera_modules"},
		func(p, h string) bool { return drugRe.MatchString(p) && drugHeads[h] },
		"a drug or supplement is not a device; drugs are checked under the pharmaceutical rules"},
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
// input Check was given). It reads only the lines that name the product (the
// "Typed:", "Product name:" or "Product:" line) and the model's product name;
// with neither it does nothing. When a rule fires it takes the family away,
// sets the confidence to 0, puts a plain sentence in Reason, and keeps the
// model's family, reason and the rule for logs (ModelFamily, ModelReason,
// GuardRule; not in the JSON). It returns the rule's id ("" when none fired).
func Guard(text string, r Result) (Result, string) {
	if r.Family == None {
		return r, ""
	}
	var phrases []string
	for _, p := range append(ProductLines(text), r.ProductName) {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			phrases = append(phrases, p)
		}
	}
	for _, g := range guardRules {
		if !contains(g.families, r.Family) {
			continue
		}
		for _, p := range phrases {
			if h := headNoun(p); h != "" && g.match(p, h) {
				r.ModelFamily, r.ModelReason, r.GuardRule = r.Family, r.Reason, g.id
				name := r.Name
				if name == "" {
					name = r.Family
				}
				r.Reason = "Not planned as " + name + ": " + g.reason + "."
				r.Family, r.Name, r.Confidence = None, "", 0
				return r, g.id
			}
		}
	}
	return r, ""
}

// ProductLines are the lines of text that name the product: the "Typed:",
// "Product name:" and "Product:" (the coverage eval's subject) lines. A parts
// list never names the product, so text with none of them gives none.
func ProductLines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		for _, pre := range []string{"Typed:", "Product name:", "Product:"} {
			if strings.HasPrefix(line, pre) {
				out = append(out, strings.TrimSpace(strings.TrimPrefix(line, pre)))
				break
			}
		}
	}
	return out
}

// headNoun is the product word of a phrase: the last word before a "for",
// "with", "from", "in" or comma clause and after the last "of", skipping
// words with digits and unit words ("12V 100Ah"), in the singular:
// "Cordless drill with two batteries" -> "drill", "Floor lamps" -> "lamp",
// "Lithium battery 12V 100Ah" -> "battery", "We assemble lithium battery
// packs in Vietnam" -> "pack".
func headNoun(p string) string {
	toks := phraseWords(p)
	if len(toks) == 0 {
		return ""
	}
	return singular(toks[len(toks)-1])
}

// phraseWords are the words of the product phrase that headNoun reads, in
// order.
func phraseWords(p string) []string {
	p = " " + p + " "
	for _, sep := range []string{",", ";", "(", " for ", " with ", " from ", " in ", " made ", " - "} {
		if i := strings.Index(p, sep); i > 0 {
			p = p[:i]
		}
	}
	if i := strings.LastIndex(p, " of "); i >= 0 {
		p = p[i+len(" of "):]
	}
	var out []string
	for _, f := range strings.Fields(p) {
		if strings.ContainsAny(f, "0123456789") {
			continue
		}
		w := strings.Trim(tokenRe.FindString(f), "'-")
		if w == "" || unitWords[w] {
			continue
		}
		out = append(out, w)
	}
	return out
}

var tokenRe = regexp.MustCompile(`[a-z][a-z'-]*`)

func singular(w string) string {
	switch {
	case strings.HasSuffix(w, "ies") && len(w) > 4:
		return w[:len(w)-3] + "y"
	case strings.HasSuffix(w, "ches") || strings.HasSuffix(w, "shes") || strings.HasSuffix(w, "xes"):
		return w[:len(w)-2]
	case strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && len(w) > 3:
		return w[:len(w)-1]
	}
	return w
}

// petBefore reports whether a pet word comes right before the product word
// ("dog sofa", "pet bed"), not anywhere ("pet-friendly sofa").
func petBefore(p, head string) bool {
	toks := phraseWords(p)
	n := len(toks)
	return n >= 2 && singular(toks[n-1]) == head && petWords[singular(toks[n-2])]
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
