package clarify

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/familycheck"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

// FamilyVersion names the family questions below (#770, D810). They are
// fixed text, chosen without a model: a change to a question, its document
// or when it is asked needs a new version.
const FamilyVersion = "clarify-family-v1"

// Family intake statuses.
const (
	// FamilyMissing: at least one question is open.
	FamilyMissing = "missing_information"
	// FamilyComplete: every question that fits the product is known from
	// the parts list or answered.
	FamilyComplete = "complete"
	// FamilyNotModelled: no question can be asked (no template, a family
	// with no questions, the guard, another market). No answer controls.
	FamilyNotModelled = "not_modelled"
)

// Question kinds.
const (
	KindCountry = "country"
	KindYesNo   = "yes_no"
)

// Question states.
const (
	StateMissing  = "missing"
	StateAnswered = "answered"
	StateKnown    = "known"
)

// familyQuestion is one fixed question. Groups are the template groups whose
// work it asks about; the rulings it shows are those groups' citations, read
// from the template, so a question never cites a ruling the template does
// not.
type familyQuestion struct {
	id, kind, text, evidence string
	groups                   []string
	// when: asked only if the product text matches (nil: always). unless:
	// never asked if it matches.
	when, unless *regexp.Regexp
	// knownWhen (yes/no questions): the parts list's operations already
	// say it, each pattern matching some operation.
	knownWhen []*regexp.Regexp
}

func re(s string) *regexp.Regexp { return regexp.MustCompile(`(?i)` + s) }

var (
	batteryText = re(`batter|\bcells?\b|power ?bank|power station|jump starter|accumulator|\bbms\b`)
	machineText = re(`machine|robot|detector|inspection|injection|moulding|molding|press\b|cnc|laser|printer|conveyor|packaging|filling|cutter|lathe|mill\b|welder`)
	pumpText    = re(`pump|compressor|motor|fan\b|blower`)
	panelText   = re(`table|desk|cabinet|shelf|shelving|bookcase|wardrobe|dresser|sideboard|nightstand|panel|\btop\b|tabletop|board|mdf|particleboard|wood`)
	softText    = re(`sofa|couch|chair|recliner|ottoman|mattress|\bbed\b|upholster|foam|cushion|futon|headboard|bench`)
	kitText     = re(`table|desk|cabinet|shelf|bookcase|wardrobe|dresser|knock-?down|flat-?pack|unassembled`)
	metalText   = re(`needle|cannula|drill|\bbits?\b|blank|forceps|scalpel|instrument|burr|\btips?\b|syringe`)
	mouldText   = re(`syringe|barrel|tub(e|ing)|drain|catheter|chamber|bulb|infusion|transfusion|\bset\b`)
	sensorText  = re(`monitor|sensor|meter|oximeter|thermometer|electrode|electronic|display|gauge`)
	lensText    = re(`lens|optic|scope|binocular|telescope|monocular|magnifier|\bwindow\b|prism|loupe`)
	cameraText  = re(`camera|webcam|dashcam|image sensor|\bccm\b|sensor module`)
	displayText = re(`display|screen|\bpanel\b|lcd|oled|touch`)
)

// familyQuestions are the questions per family: only the five families
// deepened in #772 (D782). Each names the document that settles it.
var familyQuestions = map[string][]familyQuestion{
	"power_supplies_chargers": {
		{id: "cells_made", kind: KindCountry, text: "Where were the battery cells made?",
			evidence: "Bill of materials with the origin of each part, or the cell maker's certificate of origin",
			groups:   []string{"battery_cells"}, when: batteryText},
		// Pack work, not cell making: N347193 held that welding cells into a
		// pack and fitting its BMS was not enough; formation is a cell step
		// (battery_cells, N347172) and is asked by cells_made (D817).
		{id: "pack_work", kind: KindYesNo, text: "Is the pack's work only welding the cells and fitting the BMS?",
			evidence: "Production routing or traveller for pack assembly",
			groups:   []string{"final_assembly"}, when: batteryText},
		{id: "bms_made", kind: KindCountry, text: "Where was the battery management board (BMS) made?",
			evidence: "Bill of materials with the origin of each board, or the board maker's certificate of origin",
			groups:   []string{"pcba_manufacturing"}, when: batteryText},
		{id: "pcba_made", kind: KindCountry, text: "Where was the main circuit board populated and soldered?",
			evidence: "Bill of materials with the origin of each board, or the board maker's certificate of origin",
			groups:   []string{"pcba_manufacturing"}, unless: batteryText},
		{id: "module_made", kind: KindCountry, text: "Where was the power conversion or charging module made?",
			evidence: "Bill of materials with the origin of each module, or the supplier's certificate of origin",
			groups:   []string{"power_charging_modules"}, unless: batteryText},
	},
	"motors_pumps_compressors": {
		{id: "working_unit_made", kind: KindCountry, text: "Where was the machine's working unit or main subassembly made?",
			evidence: "Bill of materials with the origin of each subassembly, or the supplier's certificate of origin",
			groups:   []string{"machine_working_unit"}, when: machineText},
		{id: "frame_made", kind: KindCountry, text: "Where were the frame, conveyors or tooling made?",
			evidence: "Bill of materials with the origin of each subassembly, or the fabricator's invoice",
			groups:   []string{"machine_frame_tooling"}, when: machineText},
		{id: "motor_made", kind: KindCountry, text: "Where were the motor's stator and rotor made?",
			evidence: "Production routing or traveller for the motor, or the motor maker's certificate of origin",
			groups:   []string{"motor_stator_rotor"}, when: pumpText},
		{id: "rotary_made", kind: KindCountry, text: "Where were the impeller, pistons or compressor mechanism made?",
			evidence: "Bill of materials with origins, or a process sheet for the machining",
			groups:   []string{"fluid_rotary_mechanism"}, when: re(`pump|compressor`)},
		{id: "housing_made", kind: KindCountry, text: "Where was the housing or machined block made?",
			evidence: "Process sheet for the machining, or the foundry's certificate of origin",
			groups:   []string{"housing_and_chassis"}, when: pumpText},
	},
	"furniture": {
		{id: "panels_cut", kind: KindCountry, text: "Where were the panels or tops cut and edged?",
			evidence: "Production routing or traveller showing where panels were cut and edge-banded",
			groups:   []string{"tabletop_fabrication"}, when: panelText},
		{id: "frame_made", kind: KindCountry, text: "Where were the frame, legs or base made?",
			evidence: "Bill of materials with the origin of each part, or the supplier's certificate of origin",
			groups:   []string{"frame_metal_fabrication"}},
		{id: "upholstered", kind: KindCountry, text: "Where were the foam and covers cut, sewn and upholstered?",
			evidence: "Process sheet for upholstery, or the supplier's certificate of origin for the covers and foam",
			groups:   []string{"foam_textile_production"}, when: softText},
		{id: "knock_down", kind: KindYesNo, text: "Is it sold unassembled, the tops packed with the bases as a kit?",
			evidence: "Packing list or product specification",
			groups:   []string{"final_assembly"}, when: kitText,
			knownWhen: []*regexp.Regexp{re(`knock-?down|unassembled|flat-?pack|as a kit`)}},
	},
	"medical_devices": {
		{id: "blanks_made", kind: KindCountry, text: "Where were the needle tubing or metal blanks made?",
			evidence: "Supplier's certificate of origin for the tubing or blanks",
			groups:   []string{"metal_cannula_forging"}, when: metalText},
		{id: "moulded_made", kind: KindCountry, text: "Where were the barrels, chambers or tubing moulded or extruded?",
			evidence: "Bill of materials with origins, or the moulder's certificate of origin",
			groups:   []string{"molded_barrels_chambers"}, when: mouldText},
		{id: "sensors_made", kind: KindCountry, text: "Where were the circuit board and sensors made?",
			evidence: "Bill of materials with the origin of each board and sensor",
			groups:   []string{"pcba_and_sensors"}, when: sensorText},
		{id: "sterile_only", kind: KindYesNo, text: "Is the work in the last country only sterilisation and packing?",
			evidence: "Process sheet for the last country's work",
			groups:   []string{"sterile_packaging"}},
	},
	"display_camera_modules": {
		{id: "lens_ground", kind: KindCountry, text: "Where were the lenses ground or milled to shape?",
			evidence: "Process sheet for lens grinding, or the lens maker's certificate of origin",
			groups:   []string{"optical_lens_shaping"}, when: lensText},
		{id: "polish_only", kind: KindYesNo, text: "Is the work in the last country only polishing or coating the lenses?",
			evidence: "Process sheet for the last country's work",
			groups:   []string{"lens_polish_coat"}, when: lensText},
		{id: "sensor_made", kind: KindCountry, text: "Where was the camera board with its image sensor made?",
			evidence: "Bill of materials with origins, or the board maker's certificate of origin",
			groups:   []string{"camera_pcba_smt"}, when: cameraText},
		{id: "panel_made", kind: KindCountry, text: "Where was the display cell (glass, thin-film transistors, liquid crystal) made?",
			evidence: "Supplier's certificate of origin for the display cell",
			groups:   []string{"display_cell_fab"}, when: displayText},
	},
}

// IntakeFamilies lists the families that have questions, sorted.
func IntakeFamilies() []string {
	out := make([]string, 0, len(familyQuestions))
	for f := range familyQuestions {
		out = append(out, f)
	}
	slices.Sort(out)
	return out
}

// HasQuestions reports whether the family has questions.
func HasQuestions(family string) bool { _, ok := familyQuestions[family]; return ok }

// FamilyIntake is a case's family questions: what is missing, answered or
// already known, or why nothing can be asked.
type FamilyIntake struct {
	Version         string `json:"version"`
	Status          string `json:"status"`
	Family          string `json:"family,omitempty"`
	FamilyName      string `json:"family_name,omitempty"`
	TemplateVersion string `json:"template_version,omitempty"`
	// NotModelled says why no question is asked (status not_modelled).
	NotModelled *NotModelled     `json:"not_modelled,omitempty"`
	Questions   []FamilyQuestion `json:"questions"`
	// Dropped names saved answers not read because their question is no
	// longer asked (the product changed, or it is now on the parts list);
	// StaleAll says none was read (D812).
	Dropped  []string `json:"dropped,omitempty"`
	StaleAll bool     `json:"stale,omitempty"`
}

// NotModelled is why a case gets no family questions.
type NotModelled struct {
	// Code: no_product, no_family, unknown_family, no_questions (a family
	// with no questions yet), guard, no_match (no question fits), market.
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

// FamilyQuestion is one question as asked for a case.
type FamilyQuestion struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Question string `json:"question"`
	// Evidence is the document that settles it.
	Evidence string `json:"evidence"`
	State    string `json:"state"`
	// Groups are the template groups it asks about; ConfersOrigin and
	// NotEnough are those groups' cited rulings, from the template.
	Groups        []string `json:"groups"`
	ConfersOrigin []string `json:"confers_origin"`
	NotEnough     []string `json:"not_enough"`
	// Known is what the parts list already says (state known).
	Known  string               `json:"known,omitempty"`
	Answer *domain.FamilyAnswer `json:"answer,omitempty"`
}

// Open counts the missing questions.
func (in FamilyIntake) Open() int {
	n := 0
	for _, q := range in.Questions {
		if q.State == StateMissing {
			n++
		}
	}
	return n
}

// FamilyCase is what the intake reads from a case.
type FamilyCase struct {
	// US: the case asks the U.S. question. The templates cite CBP rulings,
	// so other markets get no family questions.
	US      bool
	Family  string
	Facts   string
	Product *domain.Product
	// Supplied are the saved answers as read (Stale already set).
	Supplied *domain.SuppliedFamilyFacts
}

func notModelled(code, reason string) FamilyIntake {
	return FamilyIntake{Version: FamilyVersion, Status: FamilyNotModelled, Questions: []FamilyQuestion{},
		NotModelled: &NotModelled{Code: code, Reason: reason}}
}

// Intake works out a case's family questions. Deterministic; no model call.
func Intake(c FamilyCase, templates map[string]planner.Template) FamilyIntake {
	if !c.US {
		return notModelled("market", "Family questions follow CBP rulings, so they are asked on U.S. cases only.")
	}
	fam := strings.TrimSpace(c.Family)
	if fam == "" || fam == familycheck.None {
		return notModelled("no_family", "No product family was chosen, so there is no template to ask from.")
	}
	tm, ok := templates[fam]
	if !ok {
		return notModelled("unknown_family", "This product family has no template.")
	}
	qs, ok := familyQuestions[fam]
	if !ok {
		out := notModelled("no_questions", "No questions are written for "+tm.Name+" yet.")
		out.Family, out.FamilyName, out.TemplateVersion = fam, tm.Name, tm.Version
		return out
	}
	if c.Product == nil && strings.TrimSpace(c.Facts) == "" {
		return notModelled("no_product", "The case has no product to ask about.")
	}
	name := productName(c)
	if _, rule := familycheck.Guard("Product: "+name, familycheck.Result{Family: fam, Name: tm.Name, Confidence: 1}); rule != "" {
		out := notModelled("guard", "No questions for "+tm.Name+": "+guardReason("Product: "+name, fam, tm.Name)+".")
		out.Family, out.FamilyName, out.TemplateVersion = fam, tm.Name, tm.Version
		return out
	}
	text := productText(c)
	groups := map[string]planner.TemplateGroup{}
	for _, g := range tm.Groups {
		groups[g.ID] = g
	}
	sup := c.Supplied
	if sup != nil && sup.Stale {
		sup = nil
	}
	out := FamilyIntake{Version: FamilyVersion, Status: FamilyComplete, Family: fam, FamilyName: tm.Name, TemplateVersion: tm.Version,
		Questions: []FamilyQuestion{}, StaleAll: c.Supplied != nil && c.Supplied.Stale}
	asked := map[string]bool{}
	for _, q := range qs {
		if (q.when != nil && !q.when.MatchString(text)) || (q.unless != nil && q.unless.MatchString(text)) {
			continue
		}
		v := FamilyQuestion{ID: q.id, Kind: q.kind, Question: q.text, Evidence: q.evidence, State: StateMissing,
			Groups: q.groups, ConfersOrigin: []string{}, NotEnough: []string{}}
		for _, id := range q.groups {
			g, ok := groups[id]
			if !ok || g.Precedent == nil {
				continue
			}
			v.ConfersOrigin = addAll(v.ConfersOrigin, g.Precedent.ConfersOrigin)
			v.NotEnough = addAll(v.NotEnough, g.Precedent.NotEnough)
		}
		k, settled := known(q, c.Product)
		v.Known = k // what the parts list says, also beside an open question
		if settled {
			v.State = StateKnown
		} else if a, ok := suppliedAnswer(sup, q.id); ok {
			v.State, v.Answer = StateAnswered, &a
			asked[q.id] = true
		} else {
			asked[q.id] = true
		}
		out.Questions = append(out.Questions, v)
	}
	if len(out.Questions) == 0 {
		o := notModelled("no_match", "None of the questions for "+tm.Name+" fit this product.")
		o.Family, o.FamilyName, o.TemplateVersion = fam, tm.Name, tm.Version
		return o
	}
	if sup != nil {
		for id := range sup.Answers {
			if !asked[id] {
				out.Dropped = append(out.Dropped, id)
			}
		}
		slices.Sort(out.Dropped)
	}
	if out.Open() > 0 {
		out.Status = FamilyMissing
	}
	return out
}

// ValidateFamilyAnswers checks answers before they are saved: the intake must
// ask questions, every key must be one it asks now (not one the parts list
// already settles), and each value must fit its kind.
func ValidateFamilyAnswers(in FamilyIntake, answers map[string]domain.FamilyAnswer) error {
	if in.Status == FamilyNotModelled {
		return errorf("this case has no family questions: %s", in.NotModelled.Reason)
	}
	byID := map[string]FamilyQuestion{}
	for _, q := range in.Questions {
		byID[q.ID] = q
	}
	for id, a := range answers {
		q, ok := byID[id]
		switch {
		case !ok:
			return errorf("%q is not a question this case asks", id)
		case q.State == StateKnown:
			return errorf("%q is already on the parts list (%s); change it on the product", id, q.Known)
		case q.Kind == KindCountry && (a.Yes != nil || !validCountry(a.Country)):
			return errorf("%q needs a country from the list (not unknown)", id)
		case q.Kind == KindYesNo && (a.Yes == nil || a.Country != ""):
			return errorf("%q needs yes or no", id)
		}
	}
	return nil
}

type intakeError string

func (e intakeError) Error() string { return string(e) }

func errorf(format string, a ...any) error {
	return intakeError(fmt.Sprintf(format, a...))
}

// validCountry: a country of the picker's table (domain.KnownCountry), never
// a word that says it is not known.
func validCountry(s string) bool {
	if unstated.MatchString(s) {
		return false
	}
	_, ok := domain.KnownCountry(s)
	return ok
}

func suppliedAnswer(s *domain.SuppliedFamilyFacts, id string) (domain.FamilyAnswer, bool) {
	if s == nil {
		return domain.FamilyAnswer{}, false
	}
	a, ok := s.Answers[id]
	return a, ok
}

// productName is the line the guard reads: the product's name, else the
// first line of the facts.
func productName(c FamilyCase) string {
	if c.Product != nil && strings.TrimSpace(c.Product.Name) != "" {
		return c.Product.Name
	}
	line, _, _ := strings.Cut(strings.TrimSpace(c.Facts), "\n")
	return line
}

// productText is what "when" reads: the product's name, description and
// part names, and the start of the facts.
func productText(c FamilyCase) string {
	var b strings.Builder
	if p := c.Product; p != nil {
		b.WriteString(p.Name + "\n" + p.Description + "\n")
		for _, comp := range p.Components {
			b.WriteString(comp.Name + "\n")
		}
	}
	f := c.Facts
	if r := []rune(f); len(r) > 2000 {
		f = string(r[:2000])
	}
	b.WriteString(f)
	return b.String()
}

// guardReason is the guard's reason without its "Not planned as ..." lead.
func guardReason(text, fam, name string) string {
	r, _ := familycheck.Guard(text, familycheck.Result{Family: fam, Name: name, Confidence: 1})
	_, why, ok := strings.Cut(strings.TrimSuffix(r.Reason, "."), ": ")
	if !ok {
		return "the product reads as something else"
	}
	return why
}

// known is what the parts list says about a question, and whether that
// settles it. A yes/no question is settled when every knownWhen pattern
// matches an operation. A country question is settled only when every part
// and step in its groups names a country: one line blank or "Unknown" keeps
// it open, with the countries that are known shown beside it (D817).
func known(q familyQuestion, p *domain.Product) (string, bool) {
	if p == nil {
		return "", false
	}
	if q.kind == KindYesNo {
		if len(q.knownWhen) == 0 {
			return "", false
		}
		for _, k := range q.knownWhen {
			hit := false
			for _, op := range p.Operations {
				if k.MatchString(op.Name) {
					hit = true
					break
				}
			}
			if !hit {
				return "", false
			}
		}
		return "the parts list names these steps", true
	}
	var cs []string
	lines, open := 0, 0
	add := func(c string) {
		lines++
		if unstated.MatchString(c) {
			open++
			return
		}
		if c = domain.NormalizeCountry(c); !slices.Contains(cs, c) {
			cs = append(cs, c)
		}
	}
	for _, comp := range p.Components {
		if slices.Contains(q.groups, comp.Group) {
			add(comp.MadeIn)
		}
	}
	for _, op := range p.Operations {
		if slices.Contains(q.groups, op.Group) {
			country := ""
			if pl, ok := p.PlantByID(op.Plant); ok {
				country = pl.Country
			}
			add(country)
		}
	}
	if len(cs) == 0 {
		return "", false
	}
	if open > 0 {
		return strings.Join(cs, ", ") + fmt.Sprintf("; %d of %d lines name no country", open, lines), false
	}
	return strings.Join(cs, ", ") + ", from the parts list", true
}

// unstated is a "made in" that names no country: the parts list needs one,
// so a requester who does not know writes "Unknown", "TBD" or the like.
var unstated = regexp.MustCompile(`(?i)^\s*(unknown|not known|unsure|tbd|tba|n/?a|none|various|mixed|\?+|-+|)\s*$`)

func addAll(dst, src []string) []string {
	for _, s := range src {
		if !slices.Contains(dst, s) {
			dst = append(dst, s)
		}
	}
	return dst
}
