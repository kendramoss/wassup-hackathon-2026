// Command originrules builds the EU and UK non-preferential origin list
// tables (D760-D766) from their official sources and prints what each
// encodes.
//
//	go run ./cmd/originrules rules-eu -xhtml DA-02015R2446-20261101.xhtml -retrieved 2026-11-24
//	go run ./cmd/originrules rules-uk -docx ORD-v1.2.docx -retrieved 2026-11-24
//	go run ./cmd/originrules coverage            # coverage of the committed tables
//
// The sources are not committed. The tables keep the verbatim text of every
// row, and the package tests re-parse it. Sources:
//
//   - EU: the EUR-Lex consolidated text of Delegated Regulation (EU)
//     2015/2446, CELEX 02015R2446-20261101, fetched as XHTML from the
//     Publications Office (http://publications.europa.eu/resource/celex/02015R2446-20261101,
//     Accept: application/xhtml+xml, Accept-Language: eng).
//   - UK: HMRC, "Rules of Origin: Special Rules for Determining
//     Non-Preferential Origin", version 1.2 (.docx), from
//     https://www.gov.uk/government/publications/reference-document-for-the-customs-origin-of-chargeable-goods-eu-exit-regulations-2020
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/euorigin"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/nporigin"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/ukorigin"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: originrules rules-eu|rules-uk|coverage [flags]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "rules-eu":
		err = cmdRulesEU(os.Args[2:])
	case "rules-uk":
		err = cmdRulesUK(os.Args[2:])
	case "coverage":
		err = cmdCoverage()
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "originrules:", err)
		os.Exit(1)
	}
}

func cmdRulesEU(args []string) error {
	fl := flag.NewFlagSet("rules-eu", flag.ExitOnError)
	src := fl.String("xhtml", "", "consolidated XHTML of Delegated Regulation (EU) 2015/2446 ("+euorigin.CELEX+")")
	retrieved := fl.String("retrieved", "", "date the XHTML was downloaded (YYYY-MM-DD)")
	out := fl.String("out", "internal/euorigin/rules_eu.json", "output file")
	_ = fl.Parse(args)
	if *src == "" || *retrieved == "" {
		return fmt.Errorf("-xhtml and -retrieved are required")
	}
	b, err := os.ReadFile(*src)
	if err != nil {
		return err
	}
	blocks, err := euorigin.Blocks(b)
	if err != nil {
		return err
	}
	chs, err := nporigin.BuildChapters(blocks)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	t := nporigin.Table{
		Market:  "eu",
		Version: euorigin.TableVersion,
		Source: nporigin.Source{
			Title:      "Commission Delegated Regulation (EU) 2015/2446, Annex 22-01: Introductory notes and list of substantial processing or working operations conferring non-preferential origin",
			Instrument: "Regulation (EU) No 952/2013 (Union Customs Code) Art. 59-63; Delegated Regulation (EU) 2015/2446 Art. 31-36 and Annex 22-01",
			URL:        "https://eur-lex.europa.eu/legal-content/EN/TXT/?uri=CELEX:" + euorigin.CELEX,
			Version:    "CELEX " + euorigin.CELEX + " (consolidated text, " + euorigin.Consolidation + "); UCC CELEX " + euorigin.UCCCELEX,
			AsOf:       euorigin.Consolidation,
			Retrieved:  *retrieved,
			SHA256:     hex.EncodeToString(sum[:]),
			Licence:    "EUR-Lex content, reuse authorised provided the source is acknowledged (Commission Decision 2011/833/EU; EU-owned content under CC BY 4.0 per the Commission legal notice). Consolidated texts are documentation only and have no legal effect; only the Official Journal is authentic.",
			Note:       "Fetched as XHTML from the Publications Office (Cellar) for the CELEX above. Rows, descriptions and rule text verbatim; amendment markers (►M10, ▼B) dropped. Alternatives parsed by nporigin.ParseRule; an alternative with encoded=false, and every process rule, is decided only from a yes/no answer.",
		},
		Chapters: chs,
	}
	return write(*out, t)
}

func cmdRulesUK(args []string) error {
	fl := flag.NewFlagSet("rules-uk", flag.ExitOnError)
	src := fl.String("docx", "", "HMRC reference document v1.2 (.docx)")
	retrieved := fl.String("retrieved", "", "date the .docx was downloaded (YYYY-MM-DD)")
	out := fl.String("out", "internal/ukorigin/rules_uk.json", "output file")
	_ = fl.Parse(args)
	if *src == "" || *retrieved == "" {
		return fmt.Errorf("-docx and -retrieved are required")
	}
	b, err := os.ReadFile(*src)
	if err != nil {
		return err
	}
	blocks, err := ukorigin.Blocks(b)
	if err != nil {
		return err
	}
	chs, err := nporigin.BuildChapters(blocks)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	t := nporigin.Table{
		Market:  "uk",
		Version: ukorigin.TableVersion,
		Source: nporigin.Source{
			Title:      "HMRC, Rules of Origin: Special Rules for Determining Non-Preferential Origin, Part Four (Product Specific Rules)",
			Instrument: "Taxation (Cross-border Trade) Act 2018 s. 17; The Customs (Origin of Chargeable Goods) (EU Exit) Regulations 2020 (SI 2020/1433) regs 4-11; the reference document those Regulations give effect to",
			URL:        "https://assets.publishing.service.gov.uk/media/643d52d58b86bb0013f1b39b/230320_Non_Preferential_Rules_of_Origin_ORD_FINAL.docx",
			Version:    "Version " + ukorigin.DocVersion,
			AsOf:       "2023-05-10",
			Retrieved:  *retrieved,
			SHA256:     hex.EncodeToString(sum[:]),
			Licence:    "Open Government Licence v3.0 (GOV.UK: \"All content is available under the Open Government Licence v3.0, except where otherwise stated\"; the document states nothing else).",
			Note:       "Publication page https://www.gov.uk/government/publications/reference-document-for-the-customs-origin-of-chargeable-goods-eu-exit-regulations-2020 (last updated 10 Sep 2025; v1.2 is the current non-preferential document). Rows, descriptions and rule text verbatim. Alternatives parsed by nporigin.ParseRule; an alternative with encoded=false, and every process rule, is decided only from a yes/no answer.",
		},
		Chapters: chs,
	}
	return write(*out, t)
}

func write(out string, t nporigin.Table) error {
	b, err := json.MarshalIndent(t, "", " ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return printCoverage(t)
}

func cmdCoverage() error {
	for _, load := range []func() (*nporigin.Rules, error){euorigin.Rules, ukorigin.Rules} {
		r, err := load()
		if err != nil {
			return err
		}
		if err := printCoverage(r.Table()); err != nil {
			return err
		}
	}
	return nil
}

func printCoverage(t nporigin.Table) error {
	c, un := nporigin.CoverageOf(t)
	fmt.Printf("%s %s: chapters %d %v; rows %d (by code %d, with a process rule %d, with an unread rule %d); alternatives %d (by code %d, process %d, unread %d)\n",
		t.Market, t.Version, c.Chapters, t.ChapterCodes(), c.Rows, c.RowsByCode, c.RowsWithProcess, c.RowsUnread, c.Alternatives, c.ByCode, c.Process, c.Unread)
	for _, u := range un {
		fmt.Printf("  unread %s (%s): %s | %s\n", u.Row, u.Key, u.Reason, u.Text)
	}
	return nil
}
