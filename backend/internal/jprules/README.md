# Product-specific rules for import into Japan

The Japan engine reads the text of the applicable rule alongside earlier Japan Customs rulings. The table does not transpose tariff codes, apart from the reviewed correlations described in [docs/japan/hs-edition-correlation.md](../../../docs/japan/hs-edition-correlation.md), or check tariff eligibility, country membership, or treaty requirements outside its sources. A buyer needs the tariff code under the HS edition named by the treaty.

| Agreement | Source | HS edition | Applied from |
|---|---|---|---|
| Japan–EU EPA | [Annex 3-B](https://www.customs.go.jp/roo/english/text/eu3.pdf) | 2017 | 2019-02-01 |
| Japan–ASEAN (AJCEP) | [Annex 2](https://www.customs.go.jp/roo/english/text/asean2-1.pdf), the edition listed through 14 February 2027 | 2017 | 2023-01-01 |
| Japan–ASEAN (AJCEP) | [Annex 2](https://www.customs.go.jp/roo/english/text/asean2-2.pdf), the edition listed from 15 February 2027 | 2022 | 2027-02-15 |
| CPTPP | [Annex 3-D](https://www.customs.go.jp/roo/english/text/tpp2.pdf) and [Annex 4-A for textiles and apparel](https://www.customs.go.jp/roo/english/text/tpp5.pdf) | 2012 | 2018-12-30 |
| RCEP | [Annex 3A](https://rcepsec.org/rules-of-origin/), reused from the calculator | 2022 | 2023-01-01 |
| Japan–India CEPA | [Annex 2](https://www.customs.go.jp/roo/english/text/india2.pdf) | 2007 | 2011-08-01 |
| Japan–Thailand EPA | [Annex 2](https://www.customs.go.jp/roo/english/text/thailand2-2.pdf) | 2017 | 2023-01-01 |
| Japan–Indonesia EPA | [Annex 2](https://www.customs.go.jp/roo/text/indonesia2-2.pdf) | 2017 | 2024-02-05 |
| Japan–Australia EPA | [Annex 2](https://www.customs.go.jp/roo/english/text/australia2.pdf) | 2012 | 2015-01-15 |
| Japan–UK CEPA | [Annex 3-B](https://www.customs.go.jp/roo/english/text/uk3.pdf) | 2017 | 2021-01-01 |
| Japan–Vietnam EPA | [Annex 2](https://www.customs.go.jp/roo/english/text/vietnam2.pdf) | 2007 | 2009-10-01 |
| Japan–Malaysia EPA | [Annex 2](https://www.customs.go.jp/roo/english/text/malaysia2.pdf) | 2002 | 2006-07-13 |
| Japan–Mexico EPA | [Annex 4](https://www.customs.go.jp/roo/english/text/mexico2.pdf) | 2002 | 2005-04-01 |
| Japan–Philippines EPA | [Annex 2](https://www.customs.go.jp/roo/english/text/philippine2.pdf) | 2002 | 2008-12-11 |
| Japan–Switzerland EPA | [Appendix 1 to Annex II](https://www.customs.go.jp/roo/english/text/switzerland3.pdf) | 2007 | 2009-09-01 |
| Japan–Chile EPA | [Annex 2](https://www.customs.go.jp/roo/english/text/chile2.pdf) | 2002 | 2007-09-03 |

The `Applied from` date filters case dates, not a legal opinion. RCEP starts on 1 January 2023. The AJCEP start dates follow the Japan Customs index. The Japan–Indonesia start is the first day of the published HS 2017 table; the Japan–Thailand and original AJCEP dates are the implementation starts used by this offline check. A case outside the supported dates or without an applicable rule goes to review. [Japan Customs legal texts](https://www.customs.go.jp/roo/english/text/index.htm)

Treaty notes and provisions outside the embedded sources can change the answer. Where a listed row names separate provisions and the row is withheld, no text is returned. Examples include the vehicle rules in CPTPP's Appendix 1 to Annex 3-D and the analogous vehicle appendices for Japan–EU and Japan–UK. The EU introductory notes, CPTPP short supply list, and rules outside the embedded annexes are separate legal texts. Multi-rule cells in the Japan–Switzerland appendix are withheld rather than guessed. Japanese customs specialist review is tracked in issue #665. [Japan Customs legal texts](https://www.customs.go.jp/roo/english/text/index.htm)

### AJCEP general originating criteria

The AJCEP Annex 2 schedule lists product-specific rules selectively, so many headings have no row. For those codes the main agreement's general criteria apply (Article 26(1): a regional value content of at least 40% with final production in the Party, or a change of tariff heading for every non-originating material, at the exporter's choice). `LookupDetailed` returns an edited extract of those provisions only when all of these hold:

- the agreement is AJCEP and the case date is on or after 2023-01-01 and before 2027-02-15, the window of the HS 2017 Annex 2 embedded here (a test ties the window to that schedule);
- the HS 2017 schedule has no matching row, no withheld row and no more specific row that overlaps the code;
- the code has valid syntax, is not in a reserved or invalid chapter (00, 77, 98, 99), and is not in chapters 50 to 63, where the Section XI notes need separate treatment. A matched Annex 2 rule keeps its own evidence unchanged, and nothing is added to it.

`Lookup` never returns the general extract. `LookupDetailed` also says why no text was returned (`malformed`, `unsupported_agreement`, `no_dated_source`, `too_broad`, `withheld`, `invalid_code`, `no_match`, `excluded_general`) or which kind was returned (`psr`, `general`). Other agreements, the HS 2022 AJCEP schedule and RCEP get no general extract.

`data/ajcep_general.json` holds the extract with exact PDF pages and the SHA-256 of each source: [Chapter 3 excerpt](https://www.customs.go.jp/roo/english/text/asean1.pdf) (Articles 23 to 30 and 32 to 35, including the definitions and the wholly obtained list), [Annex 3](https://www.customs.go.jp/roo/english/text/asean3.pdf) and the Annex 2 introduction. Articles 32 and 33 are included because packaging and accessories change the value and tariff-change tests. Entries marked "Unde Labs note (not treaty text)" are caveats, not treaty wording; they carry no page or link and are not listed as sources:

- Annex 3 covers only the status of information-technology materials; the ITA attachment lists are not embedded, so that status cannot be assumed.
- Direct consignment (Article 31) and certification (Article 36 and Annex 4) are not verified.
- The extract is not a claim that a good qualifies or receives duty-free treatment.
- The general criteria do not validate the tariff code or convert between HS editions.

`python3 -I -B eval/jp/verify_ajcep_general.py <pdf-directory>` checks the three PDFs against the stored digests.

### Agreements awaiting a rule source

The corpus also contains Japan–U.S. Trade Agreement and GSP cases. The Japan Customs legal-text index used for this table has no product-specific rule schedule for either preference system, so they stay undetermined with a review reason. Non-preferential origin is outside the Japan assessment.

## Source and rebuild

Source is the English treaty text as published by the Ministry of Finance on the Japan Customs site, retrieved 23 November 2026. Unde Labs extracted and normalized the tables and selected notes. The result is an edited extract and should be read against the linked treaty text. The original PDFs are not redistributed here. [Japan Customs terms](https://www.customs.go.jp/kyotsu/rules.htm) · [Public Data License 1.0](https://www.digital.go.jp/resources/open_data/public_data_license_v1.0)

To rebuild with `pdfplumber` 0.11.10, download the PDFs named in `eval/jp/build_psr.py` from the links above into one directory. The script verifies each SHA-256 digest before reading and stores each source URL and hash in the table.

```sh
python3 -B eval/jp/build_psr.py <pdf-directory> backend/internal/jprules/data/schedules.json
```
