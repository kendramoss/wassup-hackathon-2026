package brokerflow

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// Filing statuses, in the order a request moves through them. Withdrawn can
// follow any status before a ruling.
const (
	FilingPrepared     = "prepared"
	FilingSubmitted    = "submitted"
	FilingAcknowledged = "acknowledged"
	FilingRuling       = "ruling_issued"
	FilingWithdrawn    = "withdrawn"
)

var filingNext = map[string][]string{
	FilingPrepared:     {FilingSubmitted, FilingWithdrawn},
	FilingSubmitted:    {FilingAcknowledged, FilingRuling, FilingWithdrawn},
	FilingAcknowledged: {FilingRuling, FilingWithdrawn},
}

// NextStatuses lists where a filing can go from status.
func NextStatuses(status string) []string { return append([]string{}, filingNext[status]...) }

// CanMove reports whether a filing may go from one status to another.
func CanMove(from, to string) bool {
	for _, s := range filingNext[from] {
		if s == to {
			return true
		}
	}
	return false
}

var interests = map[string]bool{"importer": true, "exporter": true, "manufacturer": true, "agent": true, "other": true}

// CleanSubmitter trims the submitter and refuses bad values. Missing required
// fields are not an error here: the element check reports them.
func CleanSubmitter(s store.FilingSubmitter) (store.FilingSubmitter, error) {
	fields := []struct {
		p    *string
		name string
	}{
		{&s.RequesterName, "requester_name"}, {&s.RequesterAddress, "requester_address"},
		{&s.RequesterEmail, "requester_email"}, {&s.RequesterPhone, "requester_phone"},
		{&s.Interest, "interest"}, {&s.ImporterOfRecord, "importer_of_record"},
		{&s.PortOfEntry, "port_of_entry"}, {&s.Confidential, "confidential"},
		{&s.SignerName, "signer_name"}, {&s.SignerTitle, "signer_title"},
		{&s.CountryOfExportation, "country_of_exportation"}, {&s.FirstShipment, "first_shipment"},
		{&s.ProductModel, "product_model"}, {&s.ProductDescription, "product_description"},
		{&s.HTSReference, "hts_reference"}, {&s.Marking, "marking"}, {&s.LegalBasis, "legal_basis"},
		{&s.PriorRulingNote, "prior_ruling_note"}, {&s.PriorAdvice, "prior_advice"},
		{&s.Series, "series"}, {&s.Sample, "sample"},
	}
	for _, f := range fields {
		v, err := CleanText(*f.p, f.name, false)
		if err != nil {
			return s, err
		}
		if strings.Contains(v, "[FILL IN") || strings.Contains(v, "[[") {
			return s, fmt.Errorf("%w: %s may not contain placeholder brackets", ErrInvalid, f.name)
		}
		*f.p = v
	}
	if len(s.Manufacturers) > 20 {
		return s, fmt.Errorf("%w: manufacturers has more than 20 plants", ErrInvalid)
	}
	mf := make(map[string]string, len(s.Manufacturers))
	for k, v := range s.Manufacturers {
		k, kerr := CleanText(k, "manufacturers key", true)
		v, verr := CleanText(v, "manufacturers value", true)
		if kerr != nil || verr != nil || strings.Contains(v, "[FILL IN") {
			return s, fmt.Errorf("%w: each manufacturers entry needs a plant name and a non-placeholder value", ErrInvalid)
		}
		mf[k] = v
	}
	if len(mf) > 0 {
		s.Manufacturers = mf
	} else {
		s.Manufacturers = nil
	}
	s.Series = strings.ToLower(s.Series)
	if s.Series != "" && s.Series != "is" && s.Series != "is not" {
		return s, fmt.Errorf("%w: series must be \"is\" or \"is not\"", ErrInvalid)
	}
	s.Interest = strings.ToLower(s.Interest)
	if s.Interest != "" && !interests[s.Interest] {
		return s, fmt.Errorf("%w: interest must be importer, exporter, manufacturer, agent or other", ErrInvalid)
	}
	return s, nil
}

// Fill puts the submitter's answers into the placeholders of an
// export.CFR177 letter. Placeholders it has no answer for stay. The
// replacements are tied to the generated wording: a test fails if it changes.
func Fill(letter string, s store.FilingSubmitter) string {
	rep := func(old, new string) {
		if new != "" {
			letter = strings.ReplaceAll(letter, old, new)
		}
	}
	contact := strings.Join(nonEmpty(s.RequesterName, s.RequesterAddress, s.RequesterEmail, s.RequesterPhone), ", ")
	rep("[FILL IN: requester's legal name]", s.RequesterName)
	if s.RequesterName != "" && s.RequesterAddress != "" {
		rep("[FILL IN: legal name, address, email and telephone]", contact)
	}
	rep("[FILL IN: importer, exporter or other interest]", s.Interest)
	rep("[FILL IN: name, address and importer number, or \"the requester\"]", s.ImporterOfRecord)
	if s.PortOfEntry != "" {
		rep("[FILL IN: the port or ports where the merchandise will be entered, if known]", s.PortOfEntry)
	} else {
		rep("[FILL IN: the port or ports where the merchandise will be entered, if known]", "Not yet known")
	}
	if s.NoPriorConsideration {
		letter = strings.ReplaceAll(letter, " [FILL IN: confirm, or name the office or court, the date and the reference].", "")
	}
	switch c := strings.TrimSpace(s.Confidential); {
	case strings.EqualFold(c, "none"):
		letter = keepConfidential(letter, false, "")
	case c != "":
		letter = keepConfidential(letter, true, c)
	}
	rep("[FILL IN: country of exportation]", s.CountryOfExportation)
	if s.FirstShipment != "" {
		rep("[FILL IN: expected date of the first shipment].", "Expected first shipment: "+strings.TrimSuffix(s.FirstShipment, ".")+".")
	}
	rep("[FILL IN: commercial name and model number]", s.ProductModel)
	rep("[FILL IN: what the article is, how it works and its chief use in the United States]", s.ProductDescription)
	if s.HTSReference != "" {
		rep("(classification is not requested): [FILL IN]", "(classification is not requested): "+s.HTSReference)
	}
	rep("[FILL IN: wording, method and location; attach illustrations]", s.Marking)
	rep("[FILL IN: the legal basis, with counsel]", s.LegalBasis)
	rep("[FILL IN: whether it was issued to the requester, and how this transaction differs from it]", s.PriorRulingNote)
	rep("[FILL IN: none, or from whom and the advice given]", s.PriorAdvice)
	rep("[FILL IN: is / is not]", s.Series)
	rep("[FILL IN: sent separately, or not sent; if it should be returned, how]", s.Sample)
	letter = fillManufacturers(letter, s.Manufacturers)
	if s.SignerName != "" {
		rep("[FILL IN: signature]", "/s/ "+s.SignerName)
		rep("[FILL IN: name of signer]", s.SignerName)
		rep("[FILL IN: title]", s.SignerTitle)
		rep("[FILL IN: company]", s.RequesterName)
		rep("[FILL IN: address]", s.RequesterAddress)
		rep("[FILL IN: email and telephone]", strings.Join(nonEmpty(s.RequesterEmail, s.RequesterPhone), ", "))
		rep("[FILL IN: the requester or its agent].", strings.TrimSuffix(s.RequesterName, ".")+".")
	}
	return letter
}

// fillManufacturers fills each "- Manufacturer, <plant> (<country>): [FILL IN
// ...]" line from the plant's entry.
func fillManufacturers(letter string, m map[string]string) string {
	const ph = "[FILL IN: legal name, address and manufacturer identification (MID) code]"
	if len(m) == 0 {
		return letter
	}
	lines := strings.Split(letter, "\n")
	for i, l := range lines {
		if !strings.HasPrefix(l, "- Manufacturer, ") || !strings.Contains(l, ph) {
			continue
		}
		name := strings.TrimPrefix(l, "- Manufacturer, ")
		if k := strings.Index(name, "): "); k >= 0 {
			name = name[:k]
		}
		if j := strings.LastIndex(name, " ("); j >= 0 {
			name = name[:j]
		}
		if v, ok := m[name]; ok {
			lines[i] = strings.Replace(l, ph, v, 1)
		}
	}
	return strings.Join(lines, "\n")
}

// keepConfidential keeps one of the two section-10 statements.
func keepConfidential(letter string, withhold bool, what string) string {
	const pick = "[FILL IN: keep one of the two statements and delete the other.]\n\n"
	none := "- The requester does not request confidential treatment of any information in this request.\n"
	i := strings.Index(letter, pick)
	if i < 0 {
		return letter
	}
	rest := letter[i+len(pick):]
	j := strings.Index(rest, none)
	if j < 0 {
		return letter
	}
	after := rest[j+len(none):]
	k := strings.Index(after, "\n\n")
	if k < 0 {
		return letter
	}
	ask := after[:k+1]
	var keep string
	if withhold {
		keep = strings.Replace(ask, "[FILL IN: e.g. supplier names, costs]", what, 1)
	} else {
		keep = none
	}
	return letter[:i] + strings.TrimPrefix(keep, "- ") + after[k+1:]
}

func nonEmpty(xs ...string) []string {
	var out []string
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			out = append(out, x)
		}
	}
	return out
}

// element is one content 19 CFR 177.2 (and CBP's eRulings checklist for
// origin requests) requires, and the letter section that carries it.
type element struct {
	key, citation, label, section string
	required                      bool
}

var elements = []element{
	{"parties", "19 CFR 177.2(b)(1)", "Names and addresses of the interested parties", "1", true},
	{"port", "19 CFR 177.2(b)(1)", "Port of entry, if known", "2", false},
	{"transaction", "19 CFR 177.2(b)(1)", "Description of the transaction", "3", true},
	{"merchandise", "19 CFR 177.2(b)(1), (b)(3)", "Description of the merchandise", "4", true},
	{"origin_facts", "eRulings origin checklist", "Materials, their countries and the processing", "5", true},
	{"question", "19 CFR 177.2(b)(1)", "The question presented", "6", true},
	{"position", "19 CFR 177.2(b)(6)", "Requester's position and supporting authority", "7", true},
	{"prior", "19 CFR 177.2(b)(2)(ii)", "Statement on prior or pending consideration", "8", true},
	{"documents", "19 CFR 177.2(b)(3)-(4)", "Documents, illustrations and samples", "9", true},
	{"confidential", "19 CFR 177.2(b)(7)", "Confidential treatment requested or not", "10", true},
	{"signature", "19 CFR 177.1(c), 177.2(c)", "Declaration and signature of an authorized person", "11", true},
}

var (
	sectionHead = regexp.MustCompile(`(?m)^## (\d+)\. `)
	placeholder = regexp.MustCompile(`\[FILL IN[^\]]*\]`)
)

// optionalPlaceholder is a placeholder the letter itself marks as optional.
func optionalPlaceholder(p string) bool {
	for _, m := range []string{"if available", "if known", "or delete", "or none", "list, or none"} {
		if strings.Contains(p, m) {
			return true
		}
	}
	return false
}

// Check reports, for each required content, whether the letter has it: its
// section is there and holds no placeholder that is not marked optional.
func Check(letter string) []store.FilingElement {
	sections := map[string]string{}
	idx := sectionHead.FindAllStringSubmatchIndex(letter, -1)
	for i, m := range idx {
		end := len(letter)
		if i+1 < len(idx) {
			end = idx[i+1][0]
		}
		sections[letter[m[2]:m[3]]] = letter[m[0]:end]
	}
	out := make([]store.FilingElement, 0, len(elements))
	for _, e := range elements {
		fe := store.FilingElement{Key: e.key, Citation: e.citation, Label: e.label, Required: e.required, Status: "present"}
		body, ok := sections[e.section]
		if !ok {
			fe.Status, fe.Detail = "missing", "section "+e.section+" is not in the letter"
			out = append(out, fe)
			continue
		}
		var open []string
		for _, p := range placeholder.FindAllString(body, -1) {
			if !optionalPlaceholder(p) {
				open = append(open, strings.TrimSuffix(strings.TrimPrefix(p, "[FILL IN: "), "]"))
			}
		}
		if len(open) > 0 {
			fe.Status, fe.Detail = "missing", "still to fill: "+strings.Join(open, "; ")
		}
		out = append(out, fe)
	}
	if strings.Contains(firstLines(letter), "[FILL IN: date]") {
		out = append(out, store.FilingElement{Key: "date", Citation: "19 CFR 177.2", Label: "Date of the request", Required: false, Status: "missing", Detail: "eRulings dates the request on submission"})
	}
	return out
}

func firstLines(s string) string {
	if i := strings.Index(s, "\n## "); i >= 0 {
		return s[:i]
	}
	return s
}

// Ready reports whether every required element is present.
func Ready(es []store.FilingElement) bool {
	for _, e := range es {
		if e.Required && e.Status != "present" {
			return false
		}
	}
	return len(es) > 0
}
