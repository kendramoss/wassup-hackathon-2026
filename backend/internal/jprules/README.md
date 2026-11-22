# Product-specific rules for import into Japan

The Japan engine reads the text of the applicable rule alongside earlier Japan Customs rulings. The table does not transpose tariff codes or check tariff eligibility, country membership, or treaty requirements outside its sources. A buyer needs the tariff code under the HS edition named by the treaty.

| Agreement | Source | HS edition |
|---|---|---|
| Japan–EU EPA | [Annex 3-B](https://www.customs.go.jp/roo/english/text/eu3.pdf) | 2017 |
| Japan–ASEAN (AJCEP) | [Annex 2](https://www.customs.go.jp/roo/english/text/asean2-1.pdf), the edition listed through 14 February 2027 | 2017 |
| CPTPP | [Annex 3-D](https://www.customs.go.jp/roo/english/text/tpp2.pdf) and [Annex 4-A for textiles and apparel](https://www.customs.go.jp/roo/english/text/tpp5.pdf) | 2012 |
| RCEP | [Annex 3A](https://rcepsec.org/rules-of-origin/), reused from the calculator | 2022 |

The lookup starts on 1 January 2023 for RCEP's HS 2022 table. AJCEP uses that same starting date for this implementation, with no claim that 1 January was the legal start of the published edition. A case outside the supported dates or without an applicable rule goes to review. [Japan Customs legal texts](https://www.customs.go.jp/roo/english/text/index.htm)

Treaty notes and provisions outside the embedded sources can change the answer. Where a listed row names separate provisions and the row is withheld, no text is returned. Examples include the vehicle rules in CPTPP's Appendix 1 to Annex 3-D. The EU's introductory notes, the CPTPP short supply list, and AJCEP's rules outside Annex 2 are separate legal texts. Japanese customs specialist review is tracked in issue #665. [Japan Customs legal texts](https://www.customs.go.jp/roo/english/text/index.htm)

### Agreements awaiting a rule source

The corpus also contains Japan–U.S., GSP, Japan–India, Japan–Thailand, Japan–Indonesia, Japan–Australia, Japan–UK, Japan–Vietnam, Japan–Malaysia, Japan–Mexico, Japan–Philippines, Japan–Switzerland, and Japan–Chile cases. This version has no embedded schedule for these agreements and preference systems. Non-preferential origin is outside the Japan assessment.

## Source and rebuild

Source is the English treaty text as published by the Ministry of Finance on the Japan Customs site, retrieved 23 November 2026. Unde Labs extracted and normalized the tables and selected notes. The result is an edited extract and should be read against the linked treaty text. The original PDFs are not redistributed here. [Japan Customs terms](https://www.customs.go.jp/kyotsu/rules.htm) · [Public Data License 1.0](https://www.digital.go.jp/resources/open_data/public_data_license_v1.0)

To rebuild with `pdfplumber` 0.11.10, download the four Japan Customs PDFs above into a directory. The script verifies their SHA-256 digests before reading and stores each source URL and hash in the table.

```sh
python3 -I -B eval/jp/build_psr.py <pdf-directory> backend/internal/jprules/data/schedules.json
```
