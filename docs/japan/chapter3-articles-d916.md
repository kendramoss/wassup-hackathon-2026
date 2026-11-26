# Chapter 3 text added in D916: EU and UK Articles 3.7 to 3.9 and 3.13 to 3.15, CPTPP Chapter 3, AJCEP Articles 24, 29 and 30

D910 left 128 of 400 development approvals unanswered or wrong. Read from the model's
own reasons, most JP-EU, CPTPP and AJCEP refusals cite provisions that were not in the
request: how fungible or self-produced materials count, accumulation, de minimis and
its exceptions, packaging, sets and neutral elements. This change supplies that text.
The `jp-engine-v1` prompt is unchanged; only the evidence in the request changes, as in
D910.

## Sources

| File | URL | Retrieved | SHA-256 | Pages |
|---|---|---|---|---|
| eu1.pdf | https://www.customs.go.jp/roo/english/text/eu1.pdf | 2026-11-25 (pinned before) | a71de5d6… (unchanged) | 40 |
| uk1.pdf | https://www.customs.go.jp/roo/english/text/uk1.pdf | 2026-11-25 (pinned before) | 277066a8… (unchanged) | 28 |
| tpp1.pdf (CPTPP Chapter 3 and Annexes 3-A to 3-C) | https://www.customs.go.jp/roo/english/text/tpp1.pdf | 2026-11-26 | da772daad07ef203f3326c18337579423e67f7ea200e0697ba8c484b569810d0 | 31 |
| asean1.pdf (AJCEP Chapter 3) | already in `data/ajcep_general.json` | earlier | unchanged | |

`tpp1.pdf` is pinned in `DIGESTS` of `eval/jp/build_psr.py`. The EU, UK and CPTPP Annex
3-D entries of `schedules.json` are regenerated from the pinned PDFs by the same script
(`CHAPTER_3`); every other entry is left byte for byte. Each quote is one chunk of one
PDF page, anchored by its first and last words.

## What is added

| Agreement | Provision | PDF pages | Scope |
|---|---|---|---|
| JP-EU EPA | Article 3.7 (unit of qualification), 3.8 (accounting segregation), 3.9 (sets) | 12 to 14 | schedule |
| JP-EU EPA | Article 3.13 (neutral elements), 3.14 (packing for shipment), 3.15 (packaging for retail sale) | 18, 19 | schedule |
| JP-UK CEPA | Articles 3.7 to 3.9 and 3.13 to 3.15, the same text | 11, 12, 14 to 16 | schedule |
| CPTPP (Annex 3-D schedule) | Article 3.2 (originating goods), 3.6 (materials used in production, including self-produced materials), 3.7 (value of materials), 3.10 (accumulation), 3.11 (de minimis), 3.12 (fungible goods), 3.13 (accessories), 3.14 and 3.15 (packaging and packing), 3.16 (indirect materials), 3.17 (sets) | 2, 3, 6, 7, 12 to 15 | schedule |
| CPTPP (Annex 3-D schedule) | Annex 3-C, exceptions to de minimis, and its footnote 7 | 30, 31 | rows of headings 04.01 to 04.06, 15.07, 15.08, 15.12, 15.14, 19.01, 20.08, 20.09, 21.05, 21.06, 22.02 and 23.09, the goods it names |
| AJCEP (HS 2017 rule hits) | Article 24 (originating goods), 29 (accumulation), 30 (non-qualifying operations), already verified in `ajcep_general.json` and now returned after a matched Annex 2 row (`psr_provisions`) | asean1.pdf | rule hits |

Each agreement keeps one Unde Labs note that ranks the quoted text under the listed rule
and names what is still not supplied: EU and UK Articles 3.10 and 3.11 (non-alteration
and returning products, about transport after export), Annex 3-C, Appendix 3-B-1 and the
procedural articles; CPTPP Articles 3.1, 3.3 to 3.5, 3.8, 3.9, 3.18 to 3.32 and Annexes
3-A and 3-B; AJCEP Article 31 and Annex 4. The CPTPP Annex 4-A (textiles) schedule is
unchanged: Article 3.11 paragraph 4 sends textiles to Chapter 4.

## Effect on the development requests (offline, no model)

`jpeval plan -split dev_eval`: 396 eligible requests, as before. Request bytes 9,279,221 →
11,172,654 (+20.4%); largest request 36,612 → 41,521; `requests_sha256` 2d5b8b49… →
dabd515a…. Requests that gain new evidence: JP-EU EPA 181, CPTPP 82, AJCEP 55, JP-UK
CEPA 1. `dev_neg`: 79 requests, 2,018,900 bytes, `requests_sha256` 5c6a7f77…; gains EU
14, CPTPP 36, AJCEP 18, UK 1. `jpeval diagnose -split dev_eval` counts are identical
before and after.

## Checks

`go test ./internal/jprules` (chapter3_test.go covers the EU, UK and CPTPP notes, their
pages, anchors, scopes and caveats; psrprovisions_test.go the AJCEP list; golden hashes
re-pinned for EU, UK, CPTPP and AJCEP only) and `JP_PDF_DIR=<pdfs> python3 -m unittest
test_build_psr` (verbatim page chunks against the pinned PDFs).
