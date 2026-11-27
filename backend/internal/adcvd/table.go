package adcvd

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

//go:embed data/orders.json
var ordersJSON []byte

var caseRE = regexp.MustCompile(`^[AC]-\d{3}-\d{3}$`)

// Source names the Federal Register notice a fact was read from.
type Source struct {
	DocumentNumber string `json:"document_number"`
	Date           string `json:"date"`
	Title          string `json:"title,omitempty"`
	URL            string `json:"url"`
	SHA256         string `json:"sha256,omitempty"`
}

// Order is one AD or CVD order in effect: a case number, the country and
// product it covers and the HTS numbers its scope lists.
type Order struct {
	Case      string `json:"case"`
	Type      string `json:"type"` // AD or CVD
	Country   string `json:"country"`
	Product   string `json:"product"`
	OrderDate string `json:"order_date,omitempty"`
	Status    string `json:"status"`
	// HTS are the numbers the scope text lists for the merchandise (four, six,
	// eight or ten digits, as Commerce prints them). HTSExcluded are numbers
	// named in sentences that exclude merchandise; they never match.
	HTS         []string `json:"hts"`
	HTSExcluded []string `json:"hts_excluded,omitempty"`
	// HTSText holds the verbatim scope paragraphs the numbers were read from.
	HTSText []string `json:"hts_text,omitempty"`
	// ScopeText is the written scope of the order as the same notice prints
	// it (section "Scope of the Order", capped), for reading a product
	// against the scope's words; empty when the notice carries none.
	ScopeText   string  `json:"scope_text,omitempty"`
	ScopeNotice *Source `json:"scope_notice"`
	ListedIn    *Source `json:"listed_in,omitempty"`
}

// Table is the set of orders at one snapshot date.
type Table struct {
	version, snapshot, fetched string
	orders                     []Order
	byHTS                      map[string][]int // listed code digits -> order indexes
	codes                      []string
	origin                     []string // canonical country of each order

	groupsOnce sync.Once
	groups     []group // AD and CVD orders of one product and country, built on first use by Candidates
}

type file struct {
	Version  string  `json:"version"`
	Snapshot string  `json:"snapshot"`
	Fetched  string  `json:"fetched"`
	Orders   []Order `json:"orders"`
}

// NewTable builds a table from orders, in the order given.
func NewTable(version, snapshot string, orders []Order) *Table {
	t := &Table{version: version, snapshot: snapshot, fetched: snapshot, orders: orders, byHTS: map[string][]int{}}
	for i, o := range orders {
		t.origin = append(t.origin, originCanon(o.Country))
		seen := map[string]bool{}
		for _, h := range o.HTS {
			d := digits(h)
			if d == "" || seen[d] {
				continue
			}
			seen[d] = true
			t.byHTS[d] = append(t.byHTS[d], i)
		}
	}
	for d := range t.byHTS {
		t.codes = append(t.codes, d)
	}
	sort.Strings(t.codes)
	return t
}

var (
	loadOnce sync.Once
	loaded   *Table
	loadErr  error
)

// Load returns the table embedded in the binary, parsed once.
func Load() (*Table, error) {
	loadOnce.Do(func() {
		var f file
		if err := json.Unmarshal(ordersJSON, &f); err != nil {
			loadErr = fmt.Errorf("adcvd: orders.json: %w", err)
			return
		}
		if len(f.Orders) == 0 || f.Snapshot == "" {
			loadErr = fmt.Errorf("adcvd: orders.json is empty")
			return
		}
		t := NewTable(f.Version, f.Snapshot, f.Orders)
		t.fetched = f.Fetched
		loaded = t
	})
	return loaded, loadErr
}

// LoadFile reads a table from a file in the format of data/orders.json: an
// earlier snapshot kept for evaluation, never served.
func LoadFile(path string) (*Table, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("adcvd: %s: %w", path, err)
	}
	if len(f.Orders) == 0 || f.Snapshot == "" {
		return nil, fmt.Errorf("adcvd: %s is empty", path)
	}
	t := NewTable(f.Version, f.Snapshot, f.Orders)
	t.fetched = f.Fetched
	return t, nil
}

// Orders returns the orders in the table.
func (t *Table) Orders() []Order { return t.orders }

// Snapshot is the date the table describes: notices published after it are
// not in it.
func (t *Table) Snapshot() string { return t.snapshot }

// Fetched is the date the notices were fetched.
func (t *Table) Fetched() string { return t.fetched }

// Version names the file format.
func (t *Table) Version() string { return t.version }

// digits returns the digits of an HTS code when it has 4, 6, 8 or 10 of
// them, else "".
func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == ' ':
		default:
			return ""
		}
	}
	d := b.String()
	switch len(d) {
	case 4, 6, 8, 10:
		return d
	}
	return ""
}

// htsMatch reports whether two codes agree at the shorter of their lengths:
// a listed ten-digit line needs the same ten digits from a ten-digit part,
// and a part given at heading or subheading level matches every listed line
// beneath it.
func htsMatch(part, listed string) bool {
	if part == "" || listed == "" {
		return false
	}
	n := len(part)
	if len(listed) < n {
		n = len(listed)
	}
	return part[:n] == listed[:n]
}

// aliases for Commerce's country names that domain.NormalizeCountry does not
// know. The keys are lower case with the leading formal words removed.
var commerceAlias = map[string]string{
	"lao people's democratic republic": "Laos", "laos": "Laos",
	"uae":                "United Arab Emirates",
	"korea, republic of": "South Korea",
	"rupublic of korea":  "South Korea", // as Commerce printed it in one monthly notice
	"russian federation": "Russia",
	"hong kong":          "Hong Kong",
}

var formal = []string{
	"the ", "socialist republic of ", "democratic socialist republic of ", "federative republic of ", "republic of the ", "republic of ",
	"kingdom of ", "sultanate of ", "state of ", "commonwealth of ", "united mexican states",
}

// originCanon is the name domain.NormalizeCountry gives a country after
// Commerce's formal wording ("Socialist Republic of Vietnam") is removed.
func originCanon(s string) string {
	c := strings.TrimSpace(s)
	l := strings.ToLower(c)
	if v, ok := commerceAlias[l]; ok {
		return v
	}
	if strings.HasPrefix(l, "united mexican states") {
		return "Mexico"
	}
	for changed := true; changed; {
		changed = false
		for _, p := range formal {
			if strings.HasPrefix(strings.ToLower(c), p) && !strings.HasPrefix(strings.ToLower(c), "the people's republic") && !strings.HasPrefix(strings.ToLower(c), "the netherlands") && !strings.HasPrefix(strings.ToLower(c), "the philippines") {
				c = strings.TrimSpace(c[len(p):])
				changed = true
			}
		}
	}
	if v, ok := commerceAlias[strings.ToLower(c)]; ok {
		return v
	}
	return domain.NormalizeCountry(c)
}

// originMatches reports whether one of the countries in a part's origin
// (several are written "A/B") is the order's country.
func originMatches(partOrigin, orderCountry string) bool {
	if strings.TrimSpace(partOrigin) == "" {
		return false
	}
	want := originCanon(orderCountry)
	for _, p := range domain.CountryParts(originCanon(partOrigin)) {
		if p != "" && p == want {
			return true
		}
	}
	return false
}
