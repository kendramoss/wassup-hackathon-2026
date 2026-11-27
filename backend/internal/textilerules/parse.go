package textilerules

import (
	"fmt"
	"regexp"
	"strings"
)

// Facts only the (e)(2) alternatives read.
const (
	Fact6117_10      = "is_6117_10"
	FactNot6117Other = "not_6117_10_knit_or_parts"
)

var (
	reEnum     = regexp.MustCompile(`^\((\d+)\)\s*(?:\(([a-z])\)\s*)?`)
	reOrIf     = regexp.MustCompile(`;\s*or,?\s+(?:If )`)
	reSub      = regexp.MustCompile(`(?:^|[:;,]\s*(?:or,?\s+)?)\(([a-d])\)\s*`)
	reSeq      = regexp.MustCompile(`(?i)\bIf the country of origin cannot be determined under (?:paragraph )?\(?(\d+|[a-z])\)?(?:\(([a-z])\))?(?: above| of this entry)?(?: of this section)?,\s*`)
	reAnchor   = regexp.MustCompile(`(?i)\ba change\b|\bthe country of origin\b`)
	reNotKTS   = regexp.MustCompile(`(?i)\b(?:is )?not knit to shape\b`)
	reKTS      = regexp.MustCompile(`(?i)\b(?:is )?knit to shape\b`)
	reNoParts  = regexp.MustCompile(`(?i)\bdoes not consist of two or more (?:component parts|components)\b`)
	reParts    = regexp.MustCompile(`(?i)\bconsists? of two or more (?:component parts|components)\b`)
	reFilament = regexp.MustCompile(`(?i)\b(?:the textile component )?(?:is )?of (?:continuous )?filaments(?:, including strips,)?`)
	reStaple   = regexp.MustCompile(`(?i)\b(?:the textile component )?(?:is )?of staple fibers\b`)
	reWool     = regexp.MustCompile(`(?i)\bExcept for [a-z ]+? of wool or of fine animal hair(?: treated on the back or affixed by any means to a backing of any material)?`)
	reE2       = regexp.MustCompile(`(?i)\bexcept for goods of (?:sub)?headings? [\d. ]+(?:through [\d.]+)? provided for in paragraph \(e\)\(2\) of this section`)
	reOfThose  = regexp.MustCompile(`(?i)\ba change of those (?:filaments|fibers)`)
	reTarget   = regexp.MustCompile(`(?i)\ba change (?:of those (?:filaments|fibers)(?:, including strips,)? )?(?:to|from) (.+?) (?:from|to|by) `)
	rePlain    = regexp.MustCompile(`(?i)^(?:an assembled good of )?(?:(?:sub)?headings? )?[\d.]+(?: through [\d.]+)?$`)
	rePlainTo  = regexp.MustCompile(`(?i)^(?:greige fabric|finished fabric) of (?:sub)?headings? [\d.]+(?: through [\d.]+)?$`)

	reDyePrint = regexp.MustCompile(`(?i)by both dyeing and printing when accompanied by two or more`)
	reAssembly = regexp.MustCompile(`(?i)(?:wholly assembled in a single country|the country of origin is the place of assembly)`)
	reKTSProc  = regexp.MustCompile(`(?i)knit to shape components are knit in a single country`)
	reProcess  = regexp.MustCompile(`(?i)result of (?:a |an )?(spinning|extrusion|fabric[- ]making|garnetting)(?: process)?`)
	reFabricO  = regexp.MustCompile(`(?i)in which the (?:woven )?fabric (?:comprising|component of) the good was formed|in which the fabric was produced by a fabric-making process`)
	reExtruded = regexp.MustCompile(`(?i)in which those filaments, including strips, were extruded`)
	reSpun     = regexp.MustCompile(`(?i)in which those fibers were spun into yarns`)
	reEmbroid  = regexp.MustCompile(`(?i)in which the embroidery was performed`)
	rePack     = regexp.MustCompile(`(?i)last collected and packaged for shipment`)
	reFromCl   = regexp.MustCompile(`(?i)\bfrom (any other chapter|any other heading|any heading outside that group|any heading|any other subheading|any other good of subheading [\d.]+ or from any other subheading|unassembled components)`)
	reExceptF  = regexp.MustCompile(`(?i)\bexcept from ([^;]*?)(?:,? and provided|,? provided|;|\.$|$)`)
	reCOO      = regexp.MustCompile(`(?i)the country of origin of (?:a|an) (.+?) (?:classifiable|of heading|of subheading)`)
	reToC4     = regexp.MustCompile(`(?i)determined by application of § 102\.21\(c\)\(4\)`)
	reTailArm  = regexp.MustCompile(`(?i)provided that the good does not contain armor or accessories of nontextile material`)
	reConn     = regexp.MustCompile(`(?i)\b(?:if|for|and|the good|is|it)\b|[,;:.]`)
)

// cellParser reads the cells of one row in order. A header-only alternative
// ("(2) For yarns, twine, cord, and braid:") or the words before the first
// "(a)" apply to the lettered alternatives that follow, even in the next
// cell.
type cellParser struct {
	id, header string
	n          int // alternatives read so far, for unnumbered ones
}

var reLetter = regexp.MustCompile(`^\(([a-d])\)\s*`)

// parse reads the text of a table cell into its alternatives. A cell holds
// one alternative, or several joined by "; or If".
func (p *cellParser) parse(text string) []Alt {
	text = strings.TrimSpace(text)
	chunks := []string{text}
	if loc := reOrIf.FindStringIndex(text); loc != nil {
		chunks = []string{strings.TrimSpace(text[:loc[0]]) + ";", strings.TrimSpace(text[loc[1]-3:])}
	}
	var out []Alt
	for _, c := range chunks {
		out = append(out, p.chunk(c)...)
	}
	for i := range out {
		if out[i].ID == "" {
			out[i].ID = fmt.Sprint(p.n + i + 1)
		}
	}
	p.n += len(out)
	return out
}

func (p *cellParser) chunk(text string) []Alt {
	id, header := "", ""
	if m := reEnum.FindStringSubmatch(text); m != nil {
		id = m[1]
		if m[2] != "" {
			id += "(" + m[2] + ")"
		}
		text = strings.TrimSpace(text[len(m[0]):])
		p.id, p.header = id, ""
	} else if m := reLetter.FindStringSubmatch(text); m != nil && p.id != "" {
		id, header = p.id+"("+m[1]+")", p.header
		text = strings.TrimSpace(text[len(m[0]):])
		return []Alt{parseOne(id, text, header)}
	}
	// Words before an inline "(a)" are the header of the lettered alternatives.
	if loc := reSub.FindStringSubmatchIndex(text); loc != nil && loc[0] > 0 && strings.HasSuffix(strings.TrimSpace(text[:loc[0]+1]), ":") {
		header = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(text[:loc[0]+1]), ":"))
		p.header = header
		subs := reSub.FindAllStringSubmatchIndex(text, -1)
		var out []Alt
		for n, sm := range subs {
			end := len(text)
			if n+1 < len(subs) {
				end = subs[n+1][0]
			}
			out = append(out, parseOne(id+"("+text[sm[2]:sm[3]]+")", strings.TrimSpace(text[sm[1]:end]), header))
		}
		return out
	}
	// A header with no outcome of its own.
	if strings.HasSuffix(text, ":") {
		p.header = strings.TrimSpace(strings.TrimRight(text, ":"))
		return nil
	}
	return []Alt{parseOne(id, text, "")}
}

func parseOne(id, text, header string) Alt {
	a := Alt{ID: id, Text: strings.TrimSpace(strings.TrimRight(strings.TrimSpace(strings.Join(nonEmpty(header, text), " ")), ";"))}
	full := a.Text
	rest := text
	if m := reSeq.FindStringSubmatch(rest); m != nil {
		a.After = m[1]
		if m[2] != "" {
			a.After += "(" + m[2] + ")"
		}
		if len(m[1]) == 1 && m[1] >= "a" && m[1] <= "z" { // "under (a) above": a sibling of this alternative
			a.After = strings.TrimRight(id, "abcd()") + "(" + m[1] + ")"
		}
		rest = strings.Replace(rest, m[0], "", 1)
	}
	// Words before the outcome are conditions; the header's words come first.
	pre := header
	if loc := reAnchor.FindStringIndex(rest); loc != nil {
		pre += " " + rest[:loc[0]]
	}
	a.Conds = append(a.Conds, conds(pre)...)
	if reTailArm.MatchString(full) {
		a.Conds = append(a.Conds, Cond{Unmodeled: "goods without armor or accessories of nontextile material"})
	}
	a.Req = req(rest, &a)
	return a
}

func nonEmpty(s ...string) []string {
	var o []string
	for _, x := range s {
		if strings.TrimSpace(x) != "" {
			o = append(o, x)
		}
	}
	return o
}

// conds reads the words before an outcome: the facts they name, and any
// words left over (a description of the goods) as an unmodeled condition.
func conds(pre string) []Cond {
	var out []Cond
	take := func(re *regexp.Regexp, c Cond) {
		if re.MatchString(pre) {
			out = append(out, c)
			pre = re.ReplaceAllString(pre, " ")
		}
	}
	take(reE2, Cond{Fact: FactNotE2, Want: "yes"})
	take(reWool, Cond{Fact: FactFabricOfWool, Want: "no"})
	take(reNotKTS, Cond{Fact: FactKnitToShape, Want: "no"})
	take(reKTS, Cond{Fact: FactKnitToShape, Want: "yes"})
	take(reNoParts, Cond{Fact: FactComponents, Want: "no"})
	take(reParts, Cond{Fact: FactComponents, Want: "yes"})
	take(reFilament, Cond{Fact: FactFibre, Want: "filaments"})
	take(reStaple, Cond{Fact: FactFibre, Want: "staple"})
	left := strings.TrimSpace(reSpace.ReplaceAllString(reConn.ReplaceAllString(pre, " "), " "))
	if strings.ContainsAny(left, "abcdefghijklmnopqrstuvwxyz") {
		raw := strings.TrimSpace(strings.Trim(reSpace.ReplaceAllString(pre, " "), " ,:;"))
		out = append(out, Cond{Unmodeled: raw})
	}
	return out
}

func req(rest string, a *Alt) Req {
	var r Req
	switch {
	case reToC4.MatchString(rest):
		return Req{Kind: ReqToC4}
	case reDyePrint.MatchString(rest):
		r = Req{Kind: ReqProcess, Process: ProcDyePrint}
	case reAssembly.MatchString(rest):
		r = Req{Kind: ReqProcess, Process: ProcAssembly}
	case reKTSProc.MatchString(rest):
		r = Req{Kind: ReqProcess, Process: ProcKnitToShape}
	case reProcess.MatchString(rest):
		p := strings.ToLower(reProcess.FindStringSubmatch(rest)[1])
		switch {
		case strings.HasPrefix(p, "fabric"):
			r = Req{Kind: ReqProcess, Process: ProcFabricMaking}
		case p == "garnetting":
			r = Req{Kind: ReqProcess, Process: ProcGarnetting}
		default:
			r = Req{Kind: ReqProcess, Process: p}
		}
	case reFabricO.MatchString(rest):
		r = Req{Kind: ReqProcess, Process: ProcFabricMaking}
	case reExtruded.MatchString(rest):
		r = Req{Kind: ReqProcess, Process: ProcExtrusion}
	case reSpun.MatchString(rest):
		r = Req{Kind: ReqProcess, Process: ProcSpinning}
	case reEmbroid.MatchString(rest):
		r = Req{Kind: ReqProcess, Process: ProcEmbroidery}
	case rePack.MatchString(rest):
		r = Req{Kind: ReqProcess, Process: ProcPackaging}
	case reTarget.MatchString(rest) || reFromCl.MatchString(rest):
		r = Req{Kind: ReqShift}
	default:
		return Req{Kind: ReqOther}
	}
	if m := reCOO.FindStringSubmatch(rest); m != nil && strings.ToLower(m[1]) != "good" {
		a.Conds = append(a.Conds, Cond{Unmodeled: "goods: " + m[1]})
	}
	r.Last = reOfThose.MatchString(rest) && r.Kind == ReqProcess
	// A target that is not a plain code or range is a description of goods.
	if m := reTarget.FindStringSubmatch(rest); m != nil {
		t := strings.TrimSpace(m[1])
		if !rePlain.MatchString(t) && !rePlainTo.MatchString(t) && r.Process != ProcDyePrint {
			a.Conds = append(a.Conds, Cond{Unmodeled: "goods: " + t})
		}
	}
	if m := reFromCl.FindStringSubmatch(rest); m != nil {
		sh := &Shift{From: strings.ToLower(m[1])}
		if e := reExceptF.FindStringSubmatch(rest); e != nil {
			sh.Except = exceptList(e[1])
		}
		r.Shift = sh
	}
	return r
}

// buildE2 encodes paragraph (e)(2) from its text: the goods it names, then
// its alternatives (i), (ii), (iii)(A) and (iii)(B).
func buildE2(x string) (E2, error) {
	i := strings.Index(x, "(2) For goods of HTSUS headings 6213")
	if i < 0 {
		return E2{}, fmt.Errorf("textilerules: (e)(2) not found")
	}
	end := strings.Index(x[i:], "[T.D.")
	if end < 0 {
		end = len(x) - i
	}
	text := clean(x[i : i+end])
	e := E2{Text: text}
	intro := text[:strings.Index(text, ", except for goods classified")]
	intro = strings.NewReplacer(" and ", ",").Replace(intro)
	e.Codes = append(e.Codes, reCodeTok.FindAllString(intro, -1)...)
	if len(e.Codes) != 16 {
		return e, fmt.Errorf("textilerules: (e)(2) names %d codes, want 16", len(e.Codes))
	}
	mk := func(id, after, quote string, cs []Cond, p string) Alt {
		return Alt{ID: id, After: after, Text: quote, Conds: cs, Req: Req{Kind: ReqProcess, Process: p}}
	}
	e.Alts = []Alt{
		mk("i", "", "The country of origin of the good is the country, territory, or insular possession in which the fabric comprising the good was both dyed and printed when accompanied by two or more of the following finishing operations: bleaching, shrinking, fulling, napping, decating, permanent stiffening, weighting, permanent embossing, or moireing",
			nil, ProcDyePrint),
		mk("ii", "i", "except for goods of HTSUS subheading 6117.10 that are knit to shape or consist of two or more component parts, the country of origin is the country, territory, or insular possession in which the fabric comprising the good was formed by a fabric-making process",
			[]Cond{{Fact: FactNot6117Other, Want: "yes"}}, ProcFabricMaking),
		mk("iii(A)", "i", "If the good is knit to shape, the country of origin of the good is the country, territory, or insular possession in which a change to HTSUS subheading 6117.10 from yarn occurs, provided that the knit to shape components are knit in a single country, territory, or insular possession",
			[]Cond{{Fact: Fact6117_10, Want: "yes"}, {Fact: FactKnitToShape, Want: "yes"}}, ProcKnitToShape),
		mk("iii(B)", "i", "If the good is not knit to shape and consists of two or more component parts, the country of origin of the good is the country, territory, or insular possession in which a change to an assembled good of HTSUS subheading 6117.10 from unassembled components occurs, provided that the change is the result of the good being wholly assembled in a single country, territory, or insular possession",
			[]Cond{{Fact: Fact6117_10, Want: "yes"}, {Fact: FactKnitToShape, Want: "no"}, {Fact: FactComponents, Want: "yes"}}, ProcAssembly),
	}
	// The encoded wording must be in the regulation's text.
	for _, a := range e.Alts {
		if !strings.Contains(text, a.Text) {
			return e, fmt.Errorf("textilerules: (e)(2) alternative %s is not in the regulation text", a.ID)
		}
	}
	return e, nil
}
