package jprules

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"
)

//go:embed data/correlations.json
var correlationsJSON []byte

// Correlation modes. A mapped entry lets the lookup show the schedule row of
// the parent codes; a hint entry only tells the reader which parents to check.
const (
	modeMap  = "map"
	modeHint = "hint"
)

var (
	sixDigits = regexp.MustCompile(`^[0-9]{6}$`)
	sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// correlationSource names the official WCO correlation table page that backs
// an entry.
type correlationSource struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Page   int    `json:"page"`
	Table  string `json:"table"`
}

// correlation is one reviewed link from a code of a later HS edition to the
// codes of the schedule's edition it descends from (a WCO "ex" source).
type correlation struct {
	Agreement        string              `json:"agreement"`
	Code             string              `json:"code"`
	RequestedEdition string              `json:"requested_edition"`
	ScheduleEdition  string              `json:"schedule_edition"`
	Parents          []string            `json:"parents"`
	Mode             string              `json:"mode"`
	Sources          []correlationSource `json:"sources"`
	Hint             string              `json:"hint"`
}

type correlationSet struct {
	entries []correlation
}

func (c *correlationSet) find(agreement, code string) (correlation, bool) {
	if c == nil {
		return correlation{}, false
	}
	for _, e := range c.entries {
		if e.Agreement == agreement && e.Code == code {
			return e, true
		}
	}
	return correlation{}, false
}

func parseCorrelations(data []byte, schedules map[string][]schedule) (*correlationSet, error) {
	var entries []correlation
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&entries); err != nil {
		return nil, fmt.Errorf("japan correlations: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("japan correlations: trailing data")
	}
	seen := map[string]bool{}
	for _, e := range entries {
		id := e.Agreement + " " + e.Code
		bad := func(why string) error { return fmt.Errorf("japan correlations: entry %q: %s", id, why) }
		switch {
		case len(schedules[e.Agreement]) == 0:
			return nil, bad("agreement has no schedule")
		case !sixDigits.MatchString(e.Code):
			return nil, bad("code must be six digits")
		case seen[id]:
			return nil, bad("duplicate entry")
		case e.RequestedEdition == "" || e.ScheduleEdition == "":
			return nil, bad("editions are required")
		case len(e.Parents) == 0:
			return nil, bad("parents are required")
		case len(e.Sources) == 0:
			return nil, bad("a source is required")
		}
		seen[id] = true
		for i, p := range e.Parents {
			if !sixDigits.MatchString(p) || p == e.Code || slices.Contains(e.Parents[:i], p) {
				return nil, bad("parents must be distinct six digit codes other than the code")
			}
		}
		for _, s := range e.Sources {
			if s.Name == "" || s.Table == "" || !strings.HasPrefix(s.URL, "https://") || !sha256Hex.MatchString(s.SHA256) || s.Page < 1 {
				return nil, bad("incomplete source")
			}
		}
		switch e.Mode {
		case modeMap:
			if e.Hint != "" {
				return nil, bad("a mapped entry carries no hint text")
			}
		case modeHint:
			if strings.TrimSpace(e.Hint) == "" {
				return nil, bad("a hint entry needs hint text")
			}
		default:
			return nil, bad("mode must be map or hint")
		}
	}
	return &correlationSet{entries: entries}, nil
}

// dotted writes a six digit code as 1234.56.
func dotted(code string) string { return code[:4] + "." + code[4:] }

func dottedList(codes []string) string {
	out := make([]string, len(codes))
	for i, c := range codes {
		out[i] = dotted(c)
	}
	switch len(out) {
	case 1:
		return out[0]
	case 2:
		return out[0] + " and " + out[1]
	}
	return strings.Join(out[:len(out)-1], ", ") + " and " + out[len(out)-1]
}

// correlate runs after a lookup that ended in no_match. It never replaces any
// other outcome. A mapped result needs every parent to return the same
// non-withheld rows; otherwise the entry downgrades to a hint.
func (t *Table) correlate(agreement string, res Result, before time.Time) (Result, error) {
	e, ok := t.corr.find(agreement, res.Digits)
	if !ok || res.Reason != ReasonNoMatch {
		return res, nil
	}
	hint := Result{Reason: ReasonEditionHint, Digits: res.Digits, Hint: e.Hint}
	if e.Mode == modeMap {
		var rows []Evidence
		same := true
		for i, p := range e.Parents {
			pr, err := t.lookup(agreement, p, before, false)
			if err != nil {
				return res, err
			}
			if pr.Reason != ReasonPSR || len(pr.Evidence) == 0 || i > 0 && !slices.Equal(rows, pr.Evidence) {
				same = false
				break
			}
			rows = pr.Evidence
		}
		if same {
			return t.mapped(e, res.Digits, rows), nil
		}
		hint.Hint = fmt.Sprintf("%s is not in the %s schedule, and its %s parents %s do not share one rule. A specialist needs to choose the rule by product.",
			dotted(e.Code), e.ScheduleEdition, e.ScheduleEdition, dottedList(e.Parents))
	}
	return hint, nil
}

func (t *Table) mapped(e correlation, digits string, rows []Evidence) Result {
	cites := make([]string, len(e.Sources))
	for i, s := range e.Sources {
		cites[i] = fmt.Sprintf("WCO %s, PDF page %d", s.Table, s.Page)
	}
	note := Evidence{
		Text: fmt.Sprintf("Unde Labs note (not treaty text): %s is not listed in this schedule, which uses %s codes. "+
			"The rule below is the one listed for %s, the %s code(s) that the WCO correlation table names as the source of %s. "+
			"This is an administrative correlation, not a rule for %s, and the WCO lists the source as partial (\"ex\"). "+
			"The original code %s remains the declared tariff code. Whether this rule fits the product still has to be checked.",
			dotted(e.Code), e.ScheduleEdition, dottedList(e.Parents), e.ScheduleEdition, dotted(e.Code), dotted(e.Code), dotted(e.Code)),
		Citation: strings.Join(cites, "; "),
	}
	ev := append([]Evidence{note}, rows...)
	return Result{Reason: ReasonMapped, Evidence: ev, Digits: digits, Mapped: &Mapped{
		From: e.Code, To: slices.Clone(e.Parents), RequestedEdition: e.RequestedEdition,
		ScheduleEdition: e.ScheduleEdition, SourceURL: e.Sources[0].URL,
	}}
}
