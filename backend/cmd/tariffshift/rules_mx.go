package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"slices"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

// cmdRulesMX rebuilds Mexico's non-preferential origin table (D914) from a
// plain-text export of the SNICE Word consolidation (textutil -convert txt).
// Neither file is committed; the table keeps each row's Spanish text.
func cmdRulesMX(args []string) error {
	fl := flag.NewFlagSet("rules-mx", flag.ExitOnError)
	src := fl.String("src", "", "text export of the SNICE consolidation of the Acuerdo (DOF 30-08-1994)")
	doc := fl.String("doc", "", "the downloaded Word file, for its SHA-256")
	retrieved := fl.String("retrieved", "", "date the Word file was downloaded (YYYY-MM-DD)")
	out := fl.String("out", "internal/tariffshift/rules_mx.json", "output file")
	_ = fl.Parse(args)
	if *src == "" || *doc == "" || *retrieved == "" {
		return fmt.Errorf("-src, -doc and -retrieved are required")
	}
	txt, err := os.ReadFile(*src)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(*doc)
	if err != nil {
		return err
	}
	rules, err := tariffshift.BuildMexicoRules(txt)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	var chapters []string
	for _, r := range rules {
		if !slices.Contains(chapters, r.Paragraph) {
			chapters = append(chapters, r.Paragraph)
		}
	}
	t := tariffshift.Table{
		Regime:  tariffshift.Mexico.ID,
		Version: tariffshift.MexicoTableVersion,
		Source: tariffshift.TableSource{
			Title:     "Acuerdo por el que se establecen las normas para la determinación del país de origen de mercancías importadas y las disposiciones para su certificación, en materia de cuotas compensatorias (DOF 30-08-1994), Anexo I, Apéndice de Reglas Específicas",
			URL:       "https://www.snice.gob.mx/~oracle/SNICE_DOCS/Importaci%C3%B3n-temporalImportaci%C3%B3n-definitiva_Vigente_Normatividad_Cupos-Importaci%C3%B3n-Regulaciones-y-Restricciones-No-Arancelarias_Acuerdo-Cuotas-WORD-Acuerdo-Certificaci%C3%B3n-Cuotas_20160801-19940830.doc",
			AsOf:      "2016-08-01",
			Retrieved: *retrieved,
			Note:      "Rule text from the SNICE consolidation (dated 1 August 2016; rows last amended DOF 16-10-2008). text_es keeps each row's Spanish text; text is a mechanical English rendering (tariffshift.MXRender), or the Spanish itself when any Spanish is left, which is then never encoded. Headings are written 2501 for 25.01. Deleted rows (Suprimida) are left out. Not the official version; the DOF text governs.",
			SHA256:    hex.EncodeToString(sum[:]),
			Licence:   "Diario Oficial de la Federación texts; cite the DOF. Legal texts are excluded from copyright protection (Ley Federal del Derecho de Autor, art. 14 VIII). No reuse terms were found for SNICE.",
		},
		Chapters: chapters,
		Rules:    rules,
	}
	if err := writeRules(*out, t); err != nil {
		return err
	}
	return printCoverage(t)
}
