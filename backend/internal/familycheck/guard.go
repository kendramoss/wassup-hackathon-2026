package familycheck

import (
	"regexp"
	"strings"
)

// GuardVersion names the deterministic checks below (D783, D786, D789). A
// change to a rule needs a new version, like a prompt change. v4 (D823)
// added printers, stands and the optical instrument template; v5 (D827)
// moves the lifting rules to the off-road and lifting machinery template and
// splits lenses between the optical and display templates; v6 (D850) keeps a
// whole machine off the machinery parts template.
const GuardVersion = "family-guard-v6"

// guardRule sends a model's match to "none" (review) when the product line
// names a product that only shares a cue with the family: a cordless drill is
// not a battery pack, a floor lamp is not furniture, a vitamin supplement is
// not a medical device, toy binoculars are not an optical instrument
// (#766-#769). The rules only ever take a match away; they never pick a
// family, so a wrong rule costs a plan, never a wrong template.
//
// Each rule reads the product word of its own vocabulary (productWord): the
// last word of the phrase, before any clause ("made in", "for", "that"...),
// that the rule knows as a product. A phrase with no such word is left alone.
type guardRule struct {
	id       string
	families []string
	match    func(phrase string) bool
	reason   string
}

var (
	batteryCue = regexp.MustCompile(`batter|cordless|rechargeable|lithium|li-ion`)
	// Goods of the power supplies, chargers and battery packs template, and
	// the words that describe them as a whole ("battery protection circuit",
	// "battery pack assembly", "storage cabinet").
	powerWords = words("battery batterie cell pack bank station starter charger adapter supply module storage system converter inverter generator bms unit dock cradle board backup ups assembly circuit cabinet protection pcm pcb pcba rack")
	// Devices that run on a battery.
	deviceWords = words("drill driver saw sander grinder mower trimmer blower vacuum flashlight light lamp lantern torch earbud earphone headphone headset speaker laptop computer notebook tablet phone smartphone watch smartwatch tracker camera drone toy scooter bike bicycle e-bike truck car vehicle cigarette e-cigarette vape inflator tool thermometer monitor shaver toothbrush fan heater router controller trainer collar wand")
	materialRe  = regexp.MustCompile(`cathode|anode|electrode|graphite|electrolyte|active material|precursor`)
	matWords    = words("material powder graphite electrolyte precursor foil slurry")
	lightWords  = words("lamp light lantern chandelier sconce luminaire bulb lighting flashlight torch")
	furnWords   = words("table chair desk sofa couch bed cabinet shelf shelving bookcase stool bench ottoman wardrobe dresser recliner stand nightstand futon headboard mattress cart rack sideboard")
	petWords    = words("pet dog cat puppy kitten")
	petOnly     = words("house tree crate kennel")
	elecCabRe   = regexp.MustCompile(`distribution|switchgear|electrical|electric panel|server|network|telecom|battery`)
	cabWords    = words("cabinet rack enclosure box")
	// Drug words that name a drug on their own, and dosage forms that need a
	// drug context (a tablet can be a computer, a capsule a camera).
	drugSure  = words("drug medicine medication vaccine ointment syrup supplement vitamin lozenge pill softgel gummy")
	drugForms = words("tablet capsule caplet cream")
	drugCtx   = regexp.MustCompile(`\b(pills?|dosage|\d+\s?(mg|mcg)|mg|mcg|supplements?|vitamins?|pharmaceutical|drugs?|medicines?|medication)\b`)
	notDrug   = regexp.MustCompile(`android|medical|computer|display|\bpc\b|endoscop|camera|coffee|graphics|drawing`)
	devWords  = words("device monitor thermometer pad electrode needle syringe sensor lens scope camera module")
	toyRe     = regexp.MustCompile(`\btoys?\b|\bnovelty\b`)
	toolWords = words("drill sander mower trimmer blender nailer")
	// Machines the hand-tool rule leaves alone: floor sanders, rock drills,
	// ride-on mowers, industrial blenders.
	proRe        = regexp.MustCompile(`industrial|commercial|floor|pneumatic|hydraulic|\brock\b|ride-on|riding|stationary|bench|press`)
	machineWords = words("machine pump compressor motor robot press mixer generator crusher conveyor drill sander mower trimmer blender nailer")
	// Trucks and carts are left out: a hydraulic pallet truck is a machine.
	vehWords = words("scooter bike bicycle e-bike car vehicle motorcycle moped kart")
	// An automated guided vehicle or mobile robot is a handling machine, not
	// a vehicle (D822: N337718, N343416).
	agvRe = regexp.MustCompile(`guided vehicle|\bagvs?\b|\bamrs?\b|autonomous mobile robot`)
	// Printers, 3D printers included ("3D" is dropped as a word with a digit),
	// and additive-manufacturing machines named as such.
	printerWords = words("printer")
	printRe      = regexp.MustCompile(`\b3d[- ]print|three-dimensional print|additive[- ]manufactur`)
	// Lifting and mobile machines, and stands and dollies that only hold a
	// raised load (jack stands, engine stands, wheel dollies: D821).
	liftWords  = words("jack lift hoist winch crane forklift stacker telehandler platform truck vehicle agv machine")
	standWords = words("stand dolly")
	// Optical and measuring instruments, and the network optics and smart
	// eyewear that share their words.
	instWords = words("lens window prism sight scope riflescope telescope binocular monocular microscope magnifier loupe meter multimeter gauge sensor transducer tester analyzer analyser chromatograph spectrometer spectrophotometer thermometer hygrometer barometer level square rule ruler tape measure caliper micrometer rangefinder theodolite instrument glass eyewear spectacle sunglass")
	netWords  = words("transceiver cable cord patch jumper connector")
	// Camera and display modules (display template) and the words that make
	// a lens part of one; standalone optics (optical template).
	modWords = words("camera webcam dashcam module display screen panel monitor")
	inModRe  = regexp.MustCompile(`camera|webcam|dashcam|display|screen|\blcd\b|oled|projector|\bmodule`)
	optWords = words("lens window prism sight scope riflescope telescope binocular monocular microscope magnifier loupe")
	eyeWords = words("glass eyewear spectacle")
	smartRe  = regexp.MustCompile(`\bsmart\b|\bar\b|augmented reality|camera glasses`)
	// Whole machines and working units, which are not loose machinery parts
	// (D850), and the part words that make a phrase a part of one ("pump
	// housing", "motor shaft", "spare parts for compressors"). Cylinders and
	// valves are left out: the machinery parts template cites air cylinders
	// (N303499) and a ball valve (N308442).
	wholeMachineWords = words("machine machinery lathe robot pump compressor motor generator engine turbine conveyor mixer crusher press extruder excavator loader forklift")
	machPartWords     = words("part component casting forging housing casing body shaft gear rotor stator impeller diffuser runner bearing race ring seal gasket bracket plate cover block head barrel rod spindle bushing sleeve flange coupling pulley sprocket blade vane frame base end kit valve mount cylinder piston liner filter hose belt chain nozzle screw spring wheel")
	// "Motor vehicle wheel bearings": a vehicle word after "motor" ends the
	// machine reading.
	machVehWords = words("vehicle car truck automobile")
	// Unit and chemistry words of "12V 100Ah LFP", "12 volt", "100 amp hour".
	unitWords = words("v w ah mah wh kwh kw mp mm cm m a va vdc vac ft lb kg g volt amp hour lfp sla agm lifepo nmc")
	// Words that start a clause after the product: "made in", "assembled
	// from", "that we sell", "for dogs".
	cutWords = words("for with from in made assembled manufactured produced imported sold sell sells that which we by to at using")
)

var guardRules = []guardRule{
	{"battery-material", []string{"power_supplies_chargers"},
		func(p string) bool {
			return materialRe.MatchString(p) && matWords[productWord(p, matWords, powerWords)]
		},
		"a battery material (a chemical input) is not a battery, charger or power supply"},
	{"battery-device", []string{"power_supplies_chargers"},
		func(p string) bool {
			return batteryCue.MatchString(p) && deviceWords[productWord(p, deviceWords, powerWords, matWords)]
		},
		"a device that runs on a battery is not a battery pack or power supply; choose the device's own family"},
	{"vehicle", []string{"power_supplies_chargers", "motors_pumps_compressors"},
		func(p string) bool {
			return vehWords[productWord(p, vehWords, powerWords, machineWords, liftWords, standWords)] && !agvRe.MatchString(p)
		},
		"a vehicle is not a battery pack or an industrial machine"},
	{"printer", []string{"motors_pumps_compressors", "offroad_vehicles_machinery"},
		func(p string) bool {
			return printRe.MatchString(p) || printerWords[productWord(p, printerWords, machineWords, liftWords)]
		},
		"no template is written from 3D-printer or printer rulings; sent to review"},
	{"support-stand", []string{"offroad_vehicles_machinery"},
		func(p string) bool { return standWords[productWord(p, standWords, liftWords, vehWords)] },
		"a stand or dolly that holds a load does not lift it; it is not lifting or mobile machinery"},
	{"whole-machine", []string{"machinery_parts"},
		func(p string) bool {
			return wholeMachineWords[productWord(p, wholeMachineWords, machPartWords, machVehWords)]
		},
		"a complete machine, motor, pump or compressor is not a loose machinery part"},
	{"hand-tool-or-appliance", []string{"motors_pumps_compressors"},
		func(p string) bool { return toolWords[productWord(p, machineWords)] && !proRe.MatchString(p) },
		"a hand-held power tool or small appliance belongs with motorised tools and small appliances, not industrial machines"},
	{"lighting", []string{"furniture"},
		func(p string) bool { return lightWords[productWord(p, lightWords, furnWords)] },
		"a lamp or light fitting is lighting, not furniture"},
	{"lighting-optics", []string{"optical_precision_instruments"},
		func(p string) bool { return lightWords[productWord(p, lightWords, instWords)] },
		"a lamp, flashlight or light fitting is lighting, not an optical or measuring instrument"},
	{"pet-product", []string{"furniture"},
		func(p string) bool {
			w := productWord(p, furnWords, petOnly)
			return w != "" && (petBefore(p, w) || petOnly[w] && hasPetWord(p) || w == "bed" && hasPetWord(p))
		},
		"a pet bed or other pet product is not furniture"},
	{"electrical-cabinet", []string{"furniture"},
		func(p string) bool { return cabWords[productWord(p, furnWords, cabWords)] && elecCabRe.MatchString(p) },
		"an electrical, server or battery cabinet is electrical apparatus, not furniture"},
	{"drug", []string{"medical_devices", "display_camera_modules", "optical_precision_instruments"},
		func(p string) bool {
			w := productWord(p, drugSure, drugForms, devWords)
			return drugSure[w] || drugForms[w] && drugCtx.MatchString(p) && !notDrug.MatchString(p)
		},
		"a drug or supplement is not a device; drugs are checked under the pharmaceutical rules"},
	{"toy", []string{"medical_devices", "display_camera_modules", "optical_precision_instruments"},
		func(p string) bool { return toyRe.MatchString(p) },
		"a toy or novelty with optics or a medical look is not a medical device or optical instrument"},
	{"toy-machine", []string{"offroad_vehicles_machinery"},
		func(p string) bool { return toyRe.MatchString(p) },
		"a toy or novelty is not lifting or mobile machinery"},
	{"network-optics", []string{"optical_precision_instruments"},
		func(p string) bool { return netWords[productWord(p, netWords, instWords)] },
		"a fibre optic cable or transceiver is communication equipment, not an optical or measuring instrument"},
	{"camera-display-module", []string{"optical_precision_instruments"},
		func(p string) bool { return modWords[productWord(p, modWords, instWords)] },
		"a camera or display module, its lens included, belongs with display modules and cameras"},
	{"standalone-optics", []string{"display_camera_modules"},
		func(p string) bool { return optWords[productWord(p, optWords, modWords)] && !inModRe.MatchString(p) },
		"a standalone lens or optical element belongs with optical and measuring instruments"},
	{"smart-eyewear", []string{"optical_precision_instruments"},
		func(p string) bool { return smartRe.MatchString(p) && eyeWords[productWord(p, eyeWords, instWords)] },
		"smart glasses are electronics, not an optical instrument"},
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
			if g.match(p) {
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

// clauseWords are the words of a product phrase before any clause: up to a
// comma, semicolon or bracket, after the last "of" ("set of chairs"), and
// before the first clause word ("made in", "for", "that"); words with
// digits and unit words are dropped ("12V 100Ah").
func clauseWords(p string) []string {
	for _, sep := range []string{",", ";", "("} {
		if i := strings.Index(p, sep); i > 0 {
			p = p[:i]
		}
	}
	if i := strings.LastIndex(" "+p, " of "); i >= 0 {
		p = (" " + p)[i+len(" of "):]
	}
	var out []string
	for _, f := range strings.Fields(p) {
		if strings.ContainsAny(f, "0123456789") {
			continue
		}
		w := strings.Trim(tokenRe.FindString(f), "'-")
		if w == "" {
			continue
		}
		if cutWords[w] && len(out) > 0 {
			break
		}
		if unitWords[w] || cutWords[w] {
			continue
		}
		out = append(out, w)
	}
	return out
}

// productWord is the last word of the phrase's clause words, in the
// singular, that one of the vocabularies knows (a hyphenated word is also
// read by its last part: "semi-truck" -> "truck"); "" when none does.
func productWord(p string, vocabs ...map[string]bool) string {
	known := func(w string) bool {
		for _, v := range vocabs {
			if v[w] {
				return true
			}
		}
		return false
	}
	toks := clauseWords(p)
	for i := len(toks) - 1; i >= 0; i-- {
		w := singular(toks[i])
		if known(w) {
			return w
		}
		if j := strings.LastIndex(w, "-"); j >= 0 && known(singular(w[j+1:])) {
			return singular(w[j+1:])
		}
	}
	return ""
}

// headNoun is the last clause word, in the singular ("" when none).
func headNoun(p string) string {
	toks := clauseWords(p)
	if len(toks) == 0 {
		return ""
	}
	return singular(toks[len(toks)-1])
}

var tokenRe = regexp.MustCompile(`[a-z][a-z'-]*`)

func singular(w string) string {
	switch {
	case w == "lens" || w == "glass":
		return w
	case strings.HasSuffix(w, "ies") && len(w) > 4:
		return w[:len(w)-3] + "y"
	case strings.HasSuffix(w, "ches") || strings.HasSuffix(w, "shes") || strings.HasSuffix(w, "xes") || strings.HasSuffix(w, "sses") || strings.HasSuffix(w, "lenses"):
		return w[:len(w)-2]
	case strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && len(w) > 3:
		return w[:len(w)-1]
	}
	return w
}

// petBefore reports whether a pet word comes right before the product word
// ("dog sofa", "pet bed"), not anywhere ("pet-friendly sofa").
func petBefore(p, product string) bool {
	toks := clauseWords(p)
	for i := 1; i < len(toks); i++ {
		if singular(toks[i]) == product && petWords[singular(toks[i-1])] {
			return true
		}
	}
	return false
}

// hasPetWord reports whether the whole phrase has a pet word on its own
// ("bed for dogs"; not "pet-friendly" or "dog-proof").
func hasPetWord(p string) bool {
	for _, f := range strings.Fields(p) {
		if petWords[singular(strings.Trim(f, ",;()'.!"))] {
			return true
		}
	}
	return false
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
