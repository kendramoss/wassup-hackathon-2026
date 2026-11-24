// Package nporigin is a deterministic check of a non-preferential origin
// list: the EU's Annex 22-01 to Delegated Regulation (EU) 2015/2446 and the
// UK's "Rules of Origin: Special Rules for Determining Non-Preferential
// Origin", Part Four (D760-D766). Both lists give primary rules for some
// goods only, by HS heading or subheading, with chapter residual rules
// behind them. Every other good falls to the general test (the last
// substantial, economically justified processing), which this package never
// decides: it says a person has to make that judgement.
//
// It does not classify goods. It needs the HS code of the good and of each
// material, and reads the rule text as published. A rule written as a
// process ("Manufacture from yarn", "Assembly of semi-conductor products")
// is decided only from a yes/no answer the caller gives; it is never
// guessed. No model is called.
package nporigin

import "github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"

// Range is an inclusive span of HS codes at one level (2, 4 or 6 digits).
type Range = tariffshift.Range

// Kinds of an Alternative.
const (
	// KindChange is a change in tariff classification (CC, CTH, CTSH, CTHS,
	// CTSHS), decided from the material codes.
	KindChange = "change"
	// KindChangeFrom: every non-originating material comes from the listed
	// codes (and the stated work was done).
	KindChangeFrom = "change_from"
	// KindMaxValue: the value of the non-originating materials does not
	// exceed a share of the ex-works price.
	KindMaxValue = "max_value"
	// KindProcess is a rule written as work done or a stage reached. Only a
	// yes/no answer decides it.
	KindProcess = "process"
)

// Exception is one "except from ..." clause of a change rule.
type Exception struct {
	From []Range `json:"from"`
	// Described: the clause names goods by description inside the range
	// ("split heading ex 7228 (a)", "cold-rolled products of heading 7209"),
	// so a material in the range may or may not be one of them.
	Described bool `json:"described,omitempty"`
	// Condition: the exception applies only when the change results from the
	// work this text names ("when this change results from cutting or
	// bending"). Answered by the key Alternative.Key + ".x<n>".
	Condition string `json:"condition,omitempty"`
	Text      string `json:"text"`
}

// Alternative is one primary rule of a row. A row's alternatives are joined
// by "or": the row is met when any is.
type Alternative struct {
	// Key names the alternative in answers and guidance ("8473x.2").
	Key  string `json:"key"`
	Text string `json:"text"`
	Kind string `json:"kind"`
	// Encoded is false when the parser could not read the text into the
	// fields below without guessing. An alternative that is not encoded is
	// treated as a process: decided only from an answer.
	Encoded bool   `json:"encoded"`
	Reason  string `json:"reason,omitempty"`

	// Change rules. Level is 2 (CC), 4 (CTH, CTHS) or 6 (CTSH, CTSHS); Split
	// marks CTHS and CTSHS (a change between splits of the same heading or
	// subheading also counts, which codes alone cannot show).
	Level  int         `json:"level,omitempty"`
	Split  bool        `json:"split,omitempty"`
	Except []Exception `json:"except,omitempty"`
	// From lists the only sources a "change from" rule allows ("change from
	// headings 7213 to 7215, provided the material has been cold-formed").
	From []Range `json:"from,omitempty"`
	// Condition is work the whole alternative requires besides the change
	// ("with the exclusion of assembly of uppers ..."). Answered by the key
	// Key + ".c".
	Condition string `json:"condition,omitempty"`

	// MaxValue rules: the largest share (0 to 1) of the ex-works price.
	MaxShare float64 `json:"max_share,omitempty"`

	// Identifies: a process rule that names the country of origin itself
	// ("The origin of the goods shall be the country in which the animal was
	// fattened ..."). When it is not met where the good was made, the origin
	// is another country the rule names, not a residual answer.
	Identifies bool `json:"identifies,omitempty"`
}

// Row is one line of the list: the code column, the description and the
// primary rule, split into alternatives.
type Row struct {
	// Key is a short stable name for the row ("8527", "ex8473",
	// "ex820239a"). The caller sends it back to pick a row.
	Key string `json:"key"`
	// Code is the code column as published ("ex 8202 39 (a)").
	Code string `json:"code"`
	// Applies are the HS ranges the code column covers.
	Applies []Range `json:"applies"`
	// Ex: the rule covers only the goods the description names, not the
	// whole heading or subheading. Split is the letter of a split heading or
	// subheading ("a"), set by description too.
	Ex          bool          `json:"ex,omitempty"`
	Split       string        `json:"split,omitempty"`
	Description string        `json:"description"`
	Text        string        `json:"text"`
	Alts        []Alternative `json:"alternatives"`
}

// Chapter is one chapter of the list with its notes and residual rules.
type Chapter struct {
	Chapter string `json:"chapter"`
	Title   string `json:"title"`
	// Preamble is the chapter text before the table, paragraph by
	// paragraph, as published (definitions, notes, chapter primary rules,
	// residual rules).
	Preamble []string `json:"preamble"`
	// Residual is the basis of the chapter residual rule: "value" or
	// "weight" ("" when the chapter has none).
	Residual     string `json:"residual,omitempty"`
	ResidualText string `json:"residual_text,omitempty"`
	// Mixture is the chapter residual rule applicable to mixtures, when the
	// chapter has one.
	Mixture string `json:"mixture,omitempty"`
	// ChapterRules are chapter primary rules (for example goods produced
	// from blanks). They can be applied as an alternative to every row of
	// the chapter (introductory note 2.6), and are read only from an answer
	// (key "ch<NN>.r").
	ChapterRules []string `json:"chapter_rules,omitempty"`
	// Fallback is a chapter note that names the origin when a primary rule
	// is not met (chapter 2: where the animals were fattened longest), so
	// the residual rule is not reached by code.
	Fallback string `json:"fallback,omitempty"`
	// Notes are the other chapter notes and definitions; a decided answer
	// assumes none of them excludes the change.
	Notes []string `json:"notes,omitempty"`
	Rows  []Row    `json:"rows"`
}

// Source says where a table came from.
type Source struct {
	Title string `json:"title"`
	// Instrument is the legal act and part the list sits in.
	Instrument string `json:"instrument"`
	URL        string `json:"url"`
	// Version names the text version: a CELEX consolidation, or a document
	// version and date.
	Version   string `json:"version"`
	AsOf      string `json:"as_of"`
	Retrieved string `json:"retrieved"`
	SHA256    string `json:"sha256"`
	Licence   string `json:"licence"`
	Note      string `json:"note"`
}

// Table is the committed rule data of one market.
type Table struct {
	Market   string    `json:"market"`
	Version  string    `json:"version"`
	Source   Source    `json:"source"`
	Chapters []Chapter `json:"chapters"`
}

// ChapterCodes lists the chapters the table encodes, in order.
func (t Table) ChapterCodes() []string {
	out := make([]string, 0, len(t.Chapters))
	for _, c := range t.Chapters {
		out = append(out, c.Chapter)
	}
	return out
}
