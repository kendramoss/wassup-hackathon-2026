// Package ukorigin is the UK's non-preferential origin rules check
// (D760-D766): section 17 of the Taxation (Cross-border Trade) Act 2018,
// the Customs (Origin of Chargeable Goods) (EU Exit) Regulations 2020 (SI
// 2020/1433) and the product-specific rules in Part Four of HMRC's "Rules
// of Origin: Special Rules for Determining Non-Preferential Origin"
// (version 1.2, 12 April 2023), applied by package nporigin.
//
// HMRC publishes no origin rulings (Advance Origin Rulings are not
// published), so there is no accuracy figure.
package ukorigin

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/nporigin"
)

// TableVersion is the version of rules_uk.json. Bump it when the rows, the
// parse or the source change.
const TableVersion = "uk-ord-v1.2-part4-v1"

// DocVersion is the HMRC reference document version the table was read from.
const DocVersion = "1.2 (12 April 2023, in effect from 10 May 2023)"

// General is how the UK text frames Part Four (SI 2020/1433 regs 4 to 11,
// TCTA 2018 s. 17), as package nporigin applies it.
var General = nporigin.General{
	Market:     "uk",
	ListName:   "Part Four",
	RowRef:     "Rules of Origin reference document v1.2, Part Four, ",
	Listed:     "SI 2020/1433 reg. 11",
	Residual:   "Part Four, chapter residual rule",
	Mixture:    "Part Four, chapter residual rule applicable to mixtures",
	Tolerance:  "Part Four, introductory notes (10% tolerance)",
	ChapterAlt: "Part Four, introductory notes (chapter primary rules)",
	Wholly:     "TCTA 2018 s. 17(2); SI 2020/1433 reg. 8; Part One",
	General:    "Taxation (Cross-border Trade) Act 2018 s. 17(3)",
	Judgement:  "This good has no product-specific rule in Part Four, so its origin is the country of its last substantial, economically justified processing (Taxation (Cross-border Trade) Act 2018 s. 17(3)). That is a judgement on the facts; it is not decided by code",
	// The UK text gives no "major portion" rule for minimal operations on
	// goods outside Part Four (D764): reg. 9 only says they are not an
	// important stage of manufacture.
	MinimalFallback:        false,
	MinimalRef:             "SI 2020/1433 reg. 9; Part Two",
	MinimalJudgement:       "the last work was only operations that are not an important stage of manufacture (SI 2020/1433 reg. 9, Part Two), so it does not confer origin. Where the good last underwent substantial processing is a judgement on the facts; it is not decided by code",
	MinimalOps:             "the work in %s is more than the operations Part Two says are not an important stage of manufacture (for example packing, sorting, simple assembly or labelling)",
	Uneconomic:             "the work was not done mainly to change origin and so reduce duty or avoid trade restrictions (SI 2020/1433 reg. 4)",
	MaxValueNonOriginating: true,
	Administration:         "HMRC applies these rules for goods imported into the UK. HMRC does not publish its Advance Origin Rulings.",
}

//go:embed rules_uk.json
var rulesJSON []byte

var (
	once  sync.Once
	rules *nporigin.Rules
	err0  error
)

// Rules returns the committed UK table (rules_uk.json) with the UK framing.
func Rules() (*nporigin.Rules, error) {
	once.Do(func() {
		var t nporigin.Table
		if err := json.Unmarshal(rulesJSON, &t); err != nil {
			err0 = fmt.Errorf("ukorigin: rules_uk.json: %w", err)
			return
		}
		if t.Market != "uk" || t.Version != TableVersion {
			err0 = fmt.Errorf("ukorigin: rules_uk.json is %s %s, want uk %s", t.Market, t.Version, TableVersion)
			return
		}
		rules = nporigin.New(t, General)
	})
	return rules, err0
}
