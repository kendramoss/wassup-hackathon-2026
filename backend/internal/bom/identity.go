package bom

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Supplier identity is separate from the display slug.
//
// The identity of a plant is its name, NFKC-normalised, lower-cased, with
// invisible format characters dropped and runs of white space collapsed.
// Punctuation is kept: "A-B" and "A B" are different suppliers; "a-b" and
// " A-B " are the same one.
//
// The plant ID is a readable slug of the name (ASCII letters and digits,
// "_" between runs; the long-standing rule). A slug shared by two different
// identities in one BOM, or an empty slug (a name with no ASCII letters or
// digits), gets a suffix: "_" plus the start of the SHA-256 of the identity.
// Every identity in a colliding group is suffixed, so the result depends on
// the set of names in the BOM and never on row order or which row came first.
// An identity whose slug is unique keeps the plain slug, so existing IDs of
// unambiguous names do not change.

var nonID = regexp.MustCompile(`[^a-z0-9]+`)

// plantID is the display slug of a name: lower-case ASCII letters and digits
// joined by "_", or "plant" when nothing remains. It is NOT an identity.
func plantID(name string) string {
	id := strings.Trim(nonID.ReplaceAllString(strings.ToLower(name), "_"), "_")
	if id == "" {
		id = "plant"
	}
	return id
}

// PlantIdentity returns the normalised identity of a plant name. Two names
// are the same supplier exactly when their identities are equal.
func PlantIdentity(name string) string {
	s := norm.NFKC.String(name)
	s = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// hashedID is the collision-safe form of a name's ID: the slug, "_" and n hex
// digits of the SHA-256 of the identity.
func hashedID(identity string, n int) string {
	sum := sha256.Sum256([]byte(identity))
	return plantID(identity) + "_" + hex.EncodeToString(sum[:])[:n]
}

// assignIDs gives each distinct identity among names its ID. The result is a
// function of the set of identities only.
func assignIDs(names []string) map[string]string {
	byIdentity := map[string]string{} // identity -> plain slug ("" when empty)
	for _, n := range names {
		id := PlantIdentity(n)
		byIdentity[id] = strings.Trim(nonID.ReplaceAllString(id, "_"), "_")
	}
	perSlug := map[string]int{}
	for _, s := range byIdentity {
		perSlug[s]++
	}
	out := map[string]string{}
	var hashed []string
	for id, s := range byIdentity {
		if s != "" && perSlug[s] == 1 {
			out[id] = s
		} else {
			hashed = append(hashed, id)
		}
	}
	sort.Strings(hashed)
	for _, n := range []int{8, 16, 32, 64} {
		taken := map[string]bool{}
		for _, v := range out {
			taken[v] = true
		}
		cand := map[string]string{}
		ok := true
		for _, id := range hashed {
			c := hashedID(id, n)
			if taken[c] {
				ok = false
				break
			}
			taken[c] = true
			cand[id] = c
		}
		if ok || n == 64 {
			for id, c := range cand {
				out[id] = c
			}
			break
		}
	}
	return out
}

// IDFor returns the ID the BOM parser gives name among the plant names in
// names (a name that is not in names is resolved as if it were added to them).
func IDFor(name string, names []string) string {
	return assignIDs(append(append([]string(nil), names...), name))[PlantIdentity(name)]
}

// PlantIDs is the mapping from plant names to the plant IDs Parse gives them
// for one sheet. Build it from the same rows Parse reads (NewPlantIDs) and a
// money or other per-plant parser gets exactly the IDs of the product.
type PlantIDs struct{ byIdentity map[string]string }

// rowPlantName is the plant name of a sheet row, as Parse reads it: the plant
// cell; for a "plant" row without one, the row name; else the canonical country.
func rowPlantName(kind, name, plant, country string) string {
	if plant == "" && kind == "plant" {
		plant = name
	}
	if plant == "" {
		plant = domain.NormalizeCountry(country)
	}
	return plant
}

// NewPlantIDs resolves the plant IDs of rows (header first) as Parse does:
// every row with a name and a country contributes its plant name.
func NewPlantIDs(rows [][]string) PlantIDs {
	for len(rows) > 0 && blank(rows[0]) {
		rows = rows[1:]
	}
	if len(rows) < 2 {
		return PlantIDs{byIdentity: map[string]string{}}
	}
	col, err := columns(rows[0])
	if err != nil {
		return PlantIDs{byIdentity: map[string]string{}}
	}
	var names []string
	for _, row := range rows[1:] {
		if blank(row) {
			continue
		}
		get := func(k string) string {
			if i, ok := col[k]; ok && i < len(row) {
				return strings.TrimSpace(row[i])
			}
			return ""
		}
		name, country := get("name"), domain.NormalizeCountry(get("country"))
		if name == "" || country == "" {
			continue
		}
		names = append(names, rowPlantName(strings.ToLower(get("type")), name, get("plant"), country))
	}
	return PlantIDs{byIdentity: assignIDs(names)}
}

// ID returns the plant ID for a row's plant cell and country cell: the plant
// name when given, else the canonical country as the plant name (the
// fallback Parse uses). Equal for the product parser and any money lookup.
func (p PlantIDs) ID(plant, country string) string {
	name := rowPlantName("", "", strings.TrimSpace(plant), country)
	if id, ok := p.byIdentity[PlantIdentity(name)]; ok {
		return id
	}
	return IDFor(name, nil)
}
