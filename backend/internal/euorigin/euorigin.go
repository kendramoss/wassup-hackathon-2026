// Package euorigin is the EU's non-preferential origin rules check
// (D760-D766): Article 60 of the Union Customs Code (Regulation (EU) No
// 952/2013) and Articles 31 to 36 of Delegated Regulation (EU) 2015/2446,
// with the list rules of its Annex 22-01, applied by package nporigin.
//
// These rules are common to the EU. Member State customs authorities
// administer them: they issue Binding Origin Information (BOI) and check
// declarations. Nothing here reads a BOI decision or EBTI; there is no
// accuracy figure, because no origin decisions with their facts are
// published to test against.
package euorigin

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/nporigin"
)

// TableVersion is the version of rules_eu.json. Bump it when the rows, the
// parse or the source change.
const TableVersion = "eu-da2015-2446-annex22-01-v1"

// Consolidation is the EUR-Lex consolidated version the table was built
// from, and CELEX its identifier.
const (
	CELEX         = "02015R2446-20261101"
	Consolidation = "2026-11-01"
	UCCCELEX      = "02013R0952-20221212"
)

// General is how the EU text frames the list (Art. 32 to 34 DA, Art. 60
// UCC), as package nporigin applies it.
var General = nporigin.General{
	Market:     "eu",
	ListName:   "Annex 22-01",
	RowRef:     "Delegated Regulation (EU) 2015/2446, Annex 22-01, ",
	Listed:     "Art. 32 DA",
	Residual:   "Annex 22-01, chapter residual rule",
	Mixture:    "Annex 22-01, chapter residual rule applicable to mixtures",
	Tolerance:  "Annex 22-01, introductory note 2.5",
	ChapterAlt: "Annex 22-01, introductory note 2.6",
	General:    "Art. 60(2) UCC (Regulation (EU) No 952/2013); Art. 32 DA",
	Judgement:  "This good has no list rule in Annex 22-01, so its origin is the country of its last substantial, economically justified processing (Art. 60(2) UCC). That is a judgement on the facts; it is not decided by code",
	// Art. 34 DA, last paragraph (and Art. 33 for uneconomic work).
	MinimalFallback: true,
	MinimalRef:      "Art. 34 DA",
	MinimalOps:      "the work in %s is more than the minimal operations of Art. 34 DA (for example packing, sorting, simple assembly or labelling)",
	Neutral:         "packing, and neutral elements such as energy, plant and tools, are left out of the materials (Art. 36 DA)",
	Uneconomic:      "the work was not done to avoid EU measures (Art. 33 DA)",
	Administration:  "These are common EU rules. Each Member State's customs authority applies them, issues Binding Origin Information and checks declarations.",
}

//go:embed rules_eu.json
var rulesJSON []byte

var (
	once  sync.Once
	rules *nporigin.Rules
	err0  error
)

// Rules returns the committed EU table (rules_eu.json) with the EU framing.
func Rules() (*nporigin.Rules, error) {
	once.Do(func() {
		var t nporigin.Table
		if err := json.Unmarshal(rulesJSON, &t); err != nil {
			err0 = fmt.Errorf("euorigin: rules_eu.json: %w", err)
			return
		}
		if t.Market != "eu" || t.Version != TableVersion {
			err0 = fmt.Errorf("euorigin: rules_eu.json is %s %s, want eu %s", t.Market, t.Version, TableVersion)
			return
		}
		if err := correctHS2017(&t); err != nil {
			err0 = err
			return
		}
		rules = nporigin.New(t, General)
	})
	return rules, err0
}

// hs2017 are two rows the 2018 amendment (Delegated Regulation (EU)
// 2018/1063, marker M3) wrote with HS 2017 subheadings that the HS 2022
// update of Annex 22-01 did not renumber (D767). HS 2022 split 8525 80 into
// 8525 81 to 8525 89 and replaced 8548 90 with 8548 00. Read as written,
// they would match no HS 2022 code. The row text stays verbatim; only the
// codes the row covers are read in HS 2022.
var hs2017 = map[string][]nporigin.Range{
	"852580x": {{Lo: "852581", Hi: "852589"}},
	"854890x": {{Lo: "854800", Hi: "854800"}},
}

func correctHS2017(t *nporigin.Table) error {
	n := 0
	for ci := range t.Chapters {
		for ri := range t.Chapters[ci].Rows {
			r := &t.Chapters[ci].Rows[ri]
			if rs, ok := hs2017[r.Key]; ok {
				r.Applies = rs
				n++
			}
		}
	}
	if n != len(hs2017) {
		return fmt.Errorf("euorigin: %d of %d HS 2017 rows found", n, len(hs2017))
	}
	return nil
}
