package tariffshift

// Condition names a fact the rule text depends on that an HTS code alone
// cannot settle. An exception or source with a condition is "conditional":
// the evaluator decides it only when the input states that fact.
type Condition string

const (
	// CondSimpleAssembly: the exception applies when the change results
	// from a simple assembly (102.1(o)).
	CondSimpleAssembly Condition = "simple_assembly"
	// CondNotSimpleAssembly: the source is allowed only when the change is
	// not the result of a simple assembly.
	CondNotSimpleAssembly Condition = "not_simple_assembly"
	// CondUnlessBending: the exception applies unless the change involves
	// bending to shape (the steel-tube exceptions in chapter 84).
	CondUnlessBending Condition = "unless_bending_to_shape"
	// CondGRI2a: the exception applies when the change is pursuant to General
	// Rule of Interpretation 2(a) (102.18(a)).
	CondGRI2a Condition = "gri_2a"
	// CondNotGRI2a: the source is allowed only when the change is not
	// pursuant to General Rule of Interpretation 2(a) ("or from subheading
	// 8716.90 except when that change is pursuant to ...", #550).
	CondNotGRI2a Condition = "not_gri_2a"
)

// Exception is one "except from ..." clause of an alternative.
type Exception struct {
	// To narrows the exception to a change to these codes ("except for a
	// change to subheading 8507.80 from ..."). Empty: the whole alternative.
	To        []Range   `json:"to,omitempty"`
	From      []Range   `json:"from"`
	Condition Condition `json:"condition,omitempty"`
	// Described: the clause names goods by description ("parts of water-jet
	// cutting machines of heading 8466"), so a code in the range may or may
	// not be that good.
	Described bool   `json:"described,omitempty"`
	Text      string `json:"text"`
}

// Source is an allowed source listed by code ("or from heading 8473", or a
// rule that only says "from subheading 7321.90").
type Source struct {
	From []Range `json:"from"`
	// SameAsGood, when set, is the level (6 or 4) of a source written as
	// "any other good of that subheading" (Canada, SOR/94-23 s. 12(2)): a
	// material in the good's own subheading or heading. From is then empty.
	SameAsGood int       `json:"same_as_good,omitempty"`
	Condition  Condition `json:"condition,omitempty"`
	Described  bool      `json:"described,omitempty"`
	Text       string    `json:"text"`
}

// Alternative is one "A change to ... from ..." sentence. A rule's
// alternatives are joined by "or": a material that satisfies any applicable
// alternative undergoes the shift.
type Alternative struct {
	Text string `json:"text"`
	// Encoded is false when the parser could not turn the sentence into the
	// fields below without guessing (most often a change described by goods,
	// not codes). The evaluator never decides on an alternative that is not
	// encoded; it reports it as needing a person.
	Encoded bool   `json:"encoded"`
	Reason  string `json:"reason,omitempty"`
	// To is the HTS range the sentence changes to.
	To []Range `json:"to,omitempty"`
	// From is the level of the "from any other ..." clause: 2 (chapter),
	// 4 (heading) or 6 (subheading); 0 when the sentence only lists
	// sources.
	From int `json:"from,omitempty"`
	// Group is how a source inside the target range counts: "outside" (a
	// source anywhere in the target range at the From level is not a
	// shift), "including" (only the good's own code at that level is not),
	// "within" (only sources inside the range, other than the good's own
	// code, count). GroupImplied marks "outside" read from a range rule
	// that says neither.
	Group        string `json:"group,omitempty"`
	GroupImplied bool   `json:"group_implied,omitempty"`
	// Also lists sources allowed besides the From clause.
	Also   []Source    `json:"also,omitempty"`
	Except []Exception `json:"except,omitempty"`
	// NotSimpleAssembly: the whole alternative holds only when the change is
	// not the result of a simple assembly. NotGRI2a: only when it is not
	// pursuant to GRI 2(a).
	NotSimpleAssembly bool `json:"not_simple_assembly,omitempty"`
	NotGRI2a          bool `json:"not_gri_2a,omitempty"`
}

// Rule is one row of the 19 CFR 102.20 table: the HTS range in the left
// column and the verbatim text in the right, split into alternatives.
type Rule struct {
	// ID cites the row: "19 CFR 102.20(o), 8473".
	ID string `json:"id"`
	// Paragraph is the 102.20 paragraph letter of the section ("o" for
	// Section XVI); Section is its heading.
	Paragraph string `json:"paragraph"`
	Section   string `json:"section"`
	HTS       string `json:"hts"`
	Applies   Range  `json:"applies"`
	Text      string `json:"text"`
	// TextES is the row's original Spanish text (Mexico, D914); Text is
	// then its mechanical English rendering, or the Spanish itself when the
	// row does not translate cleanly.
	TextES       string        `json:"text_es,omitempty"`
	Alternatives []Alternative `json:"alternatives"`
	// Notes are table notes of the same paragraph that govern this rule
	// (verbatim); a barring note is also an exception on each alternative.
	Notes []string `json:"notes,omitempty"`
	// FallbackNote: a note replaces 102.11(b)-(d) when a material fails the
	// shift, so the evaluator does not apply 102.11(b) to this rule.
	FallbackNote bool `json:"fallback_note,omitempty"`
	// Textile lists the codes a chapter note sends to 102.21 (the textile
	// rules, not encoded): a good in one is out of scope.
	Textile []Range `json:"textile,omitempty"`
	// NoteOverrides: a note on this rule can set the origin by a process
	// the codes cannot show, "notwithstanding the tariff shift rules"
	// (SOR/94-23 s. 4(1)(d)), so a decided answer assumes it does not.
	NoteOverrides bool `json:"note_overrides,omitempty"`
}

// Table is the committed rule data (rules.json, rules_ca.json).
type Table struct {
	// Regime names the general rules the rows are applied through (D721):
	// empty for 19 CFR Part 102, "ca-sor-94-23" for Canada's SOR/94-23.
	Regime string `json:"regime,omitempty"`
	// Version is the table's version, bumped when its rows or parse change.
	Version  string      `json:"version,omitempty"`
	Source   TableSource `json:"source"`
	Chapters []string    `json:"chapters"`
	Rules    []Rule      `json:"rules"`
}

// TableSource says where the rule text came from.
type TableSource struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	AsOf      string `json:"as_of"`
	Retrieved string `json:"retrieved"`
	Note      string `json:"note"`
	// SHA256 of the source file the table was built from, and its
	// licence terms, when the source is not public domain.
	SHA256  string `json:"sha256,omitempty"`
	Licence string `json:"licence,omitempty"`
}

// ScopeChapters are the HTS chapters encoded. D360: Section XVI (84, 85)
// and the chapters of the goods in the 14 planner families (73 grills and
// stoves, 82 hand tools, 87 vehicle parts, 90 medical and optical devices,
// 94 furniture and lamps). D560 (correcting D460): the other chapters with
// at least four goods among the development Part 102 rulings and the
// labelled rulings dated before 2024 (39 plastics, 44 wood, 48 paper, 76
// aluminium, 83 miscellaneous base metal), plus five chapters D460 picked
// from a pool that read held-out rulings, kept as regulation text (15 fats
// and oils, 21 food preparations, 40 rubber, 71 jewellery, 95 toys, games
// and sports goods). Other chapters are out of scope.
var ScopeChapters = []string{"15", "21", "39", "40", "44", "48", "71", "73", "76", "82", "83", "84", "85", "87", "90", "94", "95"}
