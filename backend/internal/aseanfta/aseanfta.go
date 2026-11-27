package aseanfta

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/atiga"
)

//go:embed data/akfta_psr_hs2022.json
var akftaJSON []byte

//go:embed data/acfta_psr_hs2017.json
var acftaJSON []byte

// Agreement names an agreement this package reads.
type Agreement string

const (
	AKFTA Agreement = "akfta"
	ACFTA Agreement = "acfta"
)

var (
	once   sync.Once
	tables map[Agreement]*atiga.Table
)

func load() {
	once.Do(func() {
		tables = map[Agreement]*atiga.Table{}
		for a, src := range map[Agreement]struct {
			p *atiga.Profile
			b []byte
		}{AKFTA: {AKFTAProfile, akftaJSON}, ACFTA: {ACFTAProfile, acftaJSON}} {
			t, err := atiga.ParseFor(src.p, src.b)
			if err != nil {
				panic(fmt.Sprintf("aseanfta %s: %v", a, err))
			}
			tables[a] = t
		}
	})
}

// TableFor is the embedded rule table of an agreement.
func TableFor(a Agreement) *atiga.Table {
	load()
	return tables[a]
}

// Check decides preferential origin under the agreement.
func Check(a Agreement, in atiga.Input) (atiga.Result, error) {
	t := TableFor(a)
	if t == nil {
		return atiga.Result{}, fmt.Errorf("aseanfta: unknown agreement %q", a)
	}
	return t.Check(in)
}
