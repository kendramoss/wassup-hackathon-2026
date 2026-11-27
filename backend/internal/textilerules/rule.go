package textilerules

// Table is the encoded 19 CFR 102.21: the scope list of (b)(5), the
// exclusions of (c)(3)(ii), paragraph (e)(1) row by row and paragraph (e)(2).
type Table struct {
	Source Source `json:"source"`
	// Scope is the (b)(5) list of codes beyond Chapters 50 through 63.
	Scope []string `json:"scope"`
	// Excl3ii is what (c)(3)(ii) takes out of the wholly-assembled rule:
	// fabrics of Chapter 59 and the listed headings and subheadings.
	Excl3ii []string `json:"c3ii_excluded"`
	Rows    []Row    `json:"rows"`
	E2      E2       `json:"e2"`
}

// Source says where the text came from.
type Source struct {
	ECFR string `json:"ecfr"`
	FR   string `json:"fr"`
	Note string `json:"note"`
}

// Row is one entry of Table 1 to paragraph (e)(1): a code or code range and
// its alternatives, in the order printed.
type Row struct {
	Code string `json:"code"`
	Lo   string `json:"lo"`
	Hi   string `json:"hi"`
	// Origin is where the row text is from: "eCFR", or the Federal Register
	// notice for an entry the eCFR does not carry.
	Origin string `json:"origin"`
	Alts   []Alt  `json:"alts"`
}

// Alt is one numbered alternative of a row.
type Alt struct {
	ID   string `json:"id"` // "1", "2", "1(a)"
	Text string `json:"text"`
	// After is the alternative that must have failed to give a country
	// first ("If the country of origin cannot be determined under (1)").
	After string `json:"after,omitempty"`
	Conds []Cond `json:"conds,omitempty"`
	Req   Req    `json:"req"`
}

// Fact names a fact about the good a condition reads.
const (
	FactKnitToShape  = "knit_to_shape"
	FactComponents   = "two_or_more_components"
	FactFibre        = "fibre_form"
	FactFabricOfWool = "fabric_of_wool"
	// FactNotE2 holds when paragraph (e)(2) does not take the good over from
	// this row (see E2).
	FactNotE2 = "not_e2"
)

// Cond is a condition of an alternative: Fact must equal Want, or the
// condition is Unmodeled (a words-only description the facts do not give).
type Cond struct {
	Fact      string `json:"fact,omitempty"`
	Want      string `json:"want,omitempty"` // "yes", "no", "filaments", "staple"
	Unmodeled string `json:"unmodeled,omitempty"`
}

// Process names. A requirement names the operation whose country is the
// country of origin.
const (
	ProcSpinning     = "spinning"
	ProcExtrusion    = "extrusion"
	ProcFabricMaking = "fabric_making"
	ProcGarnetting   = "garnetting"
	ProcAssembly     = "assembly"
	ProcKnitToShape  = "knit_to_shape"
	ProcDyePrint     = "dye_print_finish"
	ProcEmbroidery   = "embroidery"
	ProcPackaging    = "packaging"
)

// Kinds of requirement.
const (
	// ReqProcess: the country of origin is the country of Process.
	ReqProcess = "process"
	// ReqShift: a tariff shift with no process named; it needs the codes of
	// the foreign materials, which the facts do not carry.
	ReqShift = "shift"
	// ReqToC4: the row sends the good to (c)(4) ("For all other goods, the
	// country of origin ... will be determined by application of (c)(4)").
	ReqToC4 = "to_c4"
	// ReqOther: not read by code (a description or a count of waste).
	ReqOther = "other"
)

// Req is what an alternative requires.
type Req struct {
	Kind    string `json:"kind"`
	Process string `json:"process,omitempty"`
	// Last is set where the text says "a change of those filaments (fibers)
	// to heading ...": the filaments themselves become the good by the
	// process, so no later operation in another country may change them.
	Last bool `json:"last,omitempty"`
	// Shift is the tariff-shift clause the text states, kept for
	// ShiftAllows; the process decides the country (see the package doc).
	Shift *Shift `json:"shift,omitempty"`
}

// Shift is the "from ..." clause of a tariff shift.
type Shift struct {
	From   string   `json:"from"`             // "any other heading", "any heading outside that group", ...
	Except []string `json:"except,omitempty"` // ranges excluded, as printed
}

// E2 is paragraph (e)(2): goods whose rule changes unless they are of
// cotton, of wool or blends of 16 percent or more cotton.
type E2 struct {
	Text  string   `json:"text"`
	Codes []string `json:"codes"`
	Alts  []Alt    `json:"alts"`
}
