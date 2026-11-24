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
	Wholly:     "Art. 60(1) UCC; Art. 31 DA",
	General:    "Art. 60(2) UCC (Regulation (EU) No 952/2013); Art. 32 DA",
	Judgement:  "This good has no list rule in Annex 22-01, so its origin is the country of its last substantial, economically justified processing (Art. 60(2) UCC). That is a judgement on the facts; it is not decided by code",
	// Art. 34 DA, last paragraph (and Art. 33 for uneconomic work).
	MinimalFallback: true,
	MinimalRef:      "Art. 34 DA",
	MinimalOps:      "the work in %s is more than the minimal operations of Art. 34 DA (for example packing, sorting, simple assembly or labelling)",
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
		rules = nporigin.New(t, General)
	})
	return rules, err0
}
