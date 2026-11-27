package httpapi

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffwatch"
)

// Tariff and trade-action watch (#917, D938, TARIFF_WATCH=on).
//
// A plan search stores, on the case, the duty basis it priced the money on:
// the HTS revision, the MFN rate, the Section 301 lists and duties, and
// that Section 232 is not modelled. cmd/tariffwatch re-checks those
// snapshots against a newer HTS revision's data and records, per case, what
// changed and the change in the annual duty figure. The route below lists the
// cases it found affected. Nothing here calls the HTS or a model.

const section232Note = "Section 232 duties are not part of the rate this case was priced with (the duty table does not model them): check before relying on the duty figure"

// dutyBasis is the snapshot of the duty basis a priced plan request used, or
// nil when there is nothing to snapshot (no HTS number, no duty rate). priced
// is the economics the planner was given: its rates are the table rates, live
// rates and user rates the search worked from.
func (a *api) dutyBasis(econ planner.Economics, p *domain.Product, asOf, now time.Time) *tariffwatch.Basis {
	if p == nil || len(econ.DutyRates) == 0 {
		return nil
	}
	code, err := duty.ParseHTS(p.HTS)
	if err != nil {
		return nil
	}
	b := &tariffwatch.Basis{HTS: code.Subheading, PricedAt: now.UTC(), AsOf: asOf.UTC().Format(time.DateOnly), AnnualImportUSD: econ.AnnualImportValueUSD,
		Section232: tariffwatch.Section232{Modelled: false, Note: section232Note}}
	if a.d.DutyAdd != nil {
		b.HTSRevision = a.d.DutyAdd.Revision()
	}
	line, hasLine := duty.Line{}, false
	if a.d.Duty != nil {
		line, hasLine = a.d.Duty.Line(p.HTS)
		if hasLine && b.HTSRevision == "" {
			b.HTSRevision = line.Revision
		}
	}
	fromTable, live := map[string]bool{}, map[string]bool{}
	for _, o := range econ.DutyRatesFromTable {
		fromTable[domain.DutyCountry(o)] = true
	}
	for _, o := range econ.DutyRatesLiveMFN {
		live[domain.DutyCountry(o)] = true
	}
	var mfn *float64
	if a.d.LiveMFN != nil {
		if m, ok, err := a.d.LiveMFN.Cached(p.HTS); ok && err == nil && m.Supported && m.Rate != nil {
			r := *m.Rate
			mfn = &r
		}
	}
	origins := make([]string, 0, len(econ.DutyRates))
	for o := range econ.DutyRates {
		origins = append(origins, o)
	}
	sort.Strings(origins)
	for _, o := range origins {
		n := domain.DutyCountry(o)
		ob := tariffwatch.OriginBasis{Origin: o, Rate: econ.DutyRates[o], Source: "user"}
		switch {
		case fromTable[n] && hasLine:
			ob.Source = "table"
			for _, r := range line.Rates {
				if domain.DutyCountry(r.Origin) != n {
					continue
				}
				for _, c := range r.Components {
					k := tariffwatch.Component{Kind: componentKind(c.Name), Name: c.Name, Heading: c.Heading, Rate: c.Rate}
					ob.Components = append(ob.Components, k)
					if k.Kind == tariffwatch.KindMFN {
						v := c.Rate
						ob.MFN = &v
					}
				}
			}
		case live[n] && mfn != nil:
			ob.Source = "live_mfn"
			ob.MFN = mfn
			ob.Components = append(ob.Components, tariffwatch.Component{Kind: tariffwatch.KindMFN, Name: "MFN (column 1 general)", Heading: code.Subheading, Rate: *mfn})
			for _, x := range econ.DutyAdditional[o] {
				ob.Components = append(ob.Components, tariffwatch.Component{Kind: tariffwatch.KindSection301, Name: x.Name, Heading: x.Heading, Rate: x.Rate})
			}
			if a.d.DutyAdd != nil {
				r := a.d.DutyAdd.ForAsOf(code.Subheading, o, *mfn, asOf)
				for _, x := range append(append([]duty.Exclusion{}, r.MayExclude...), r.Expired...) {
					ob.Exclusions = append(ob.Exclusions, tariffwatch.Exclusion{Heading: x.Heading, HTS: x.HTS, EndDate: x.EndDate, Status: x.Status})
				}
			}
		}
		sort.SliceStable(ob.Components, func(i, j int) bool { return ob.Components[i].Heading < ob.Components[j].Heading })
		b.Origins = append(b.Origins, ob)
	}
	return b
}

// componentKind says what a researched-table component is by its name.
func componentKind(name string) string {
	switch {
	case strings.HasPrefix(name, "MFN"):
		return tariffwatch.KindMFN
	case strings.HasPrefix(name, "Section 301"):
		return tariffwatch.KindSection301
	case strings.HasPrefix(name, "Section 232"):
		return tariffwatch.KindSection232
	}
	return tariffwatch.KindOther
}

// snapshotBasis is the basis to store with a plan result, nil with the flag
// off.
func (a *api) snapshotBasis(req planner.Request, now time.Time) *tariffwatch.Basis {
	if !a.d.TariffWatch {
		return nil
	}
	return a.dutyBasis(req.Economics, &req.Product, a.asOf(), now)
}

// setDutyBasis stores a new snapshot on the case. The last check was of the
// old snapshot, so it goes; the next run of the command makes a new one.
func setDutyBasis(c *store.Case, b *tariffwatch.Basis) {
	if b == nil {
		return
	}
	c.DutyBasis, c.TariffWatch = b, nil
}

type tariffWatchRow struct {
	CaseID      string             `json:"case_id"`
	Name        string             `json:"name,omitempty"`
	HTS         string             `json:"hts,omitempty"`
	CheckedAt   time.Time          `json:"checked_at"`
	Result      tariffwatch.Result `json:"result"`
	ChangeCount int                `json:"change_count"`
}

// listTariffWatch is GET /v1/workspaces/{id}/tariff-watch: the workspace's
// cases that the latest check found affected by a newer HTS revision, with
// what changed and the change in the annual duty figure and the plan's duty
// effect. ?all=true lists every checked case. Any member may read it. A case
// that was never checked is not listed (the counts say how many were).
func (a *api) listTariffWatch(w http.ResponseWriter, r *http.Request) {
	id, ok := a.workspaceMember(w, r)
	if !ok {
		return
	}
	cs, err := store.ListWorkspace(r.Context(), a.d.Cases, id.workspace)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	all := r.URL.Query().Get("all") == "true"
	rows, checked, affected, total := []tariffWatchRow{}, 0, 0, 0
	for _, c := range cs {
		if IsHealthProbeID(c.ID) {
			continue
		}
		total++
		if c.TariffWatch == nil {
			continue
		}
		checked++
		if c.TariffWatch.Affected {
			affected++
		}
		if !c.TariffWatch.Affected && !all {
			continue
		}
		row := tariffWatchRow{CaseID: c.ID, CheckedAt: c.TariffWatch.CheckedAt, Result: *c.TariffWatch, ChangeCount: len(c.TariffWatch.Changes)}
		if c.Product != nil {
			row.Name, row.HTS = c.Product.Name, c.Product.HTS
		}
		if c.DisplayName != "" {
			row.Name = c.DisplayName
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		di, dj := abs(rowChange(rows[i])), abs(rowChange(rows[j]))
		if di != dj {
			return di > dj
		}
		return rows[i].CaseID < rows[j].CaseID
	})
	writeJSON(w, http.StatusOK, map[string]any{"workspace_id": id.workspace, "cases_in_workspace": total, "checked": checked, "affected": affected, "cases": rows})
}

// rowChange is the size a row is ranked by: the change in the annual duty
// figure, else in the plan's duty effect.
func rowChange(r tariffWatchRow) float64 {
	switch {
	case r.Result.Annual != nil:
		return r.Result.Annual.Change
	case r.Result.Plan != nil:
		return r.Result.Plan.Change
	}
	return 0
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
