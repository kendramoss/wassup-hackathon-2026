# Japan GSP rule pilot (Japanese text, current window only)

The Japan engine now has product specific rule text for Japan's Generalized System of Preferences (agreement code `GSP`) for import dates from 2023-04-01. The text is the Appendix Table to Article 9 of the Ministerial Ordinance for Enforcement of the Temporary Tariff Measures Law (関税暫定措置法施行規則, e-Gov law id `344M50000040039`), with Articles 8 and 9 and the remarks (備考) of the table. The ordinance is published only in Japanese, so rows and notes are stored in Japanese and are not translated.

## Sources

All retrieved 2026-11-24 and 2026-11-25 from the e-Gov law API (<https://laws.e-gov.go.jp/>). Revision list: <https://laws.e-gov.go.jp/api/2/law_revisions/344M50000040039>, sha256 `d8d2ef34369552b22e89e01d407e283be14097b3ac498ac53bb2f4bd0ba7a115`.

| Revision id | In force from | sha256 of the XML |
|---|---|---|
| `344M50000040039_20170401_429M60000040035` | 2017-04-01 | `2921e5178b66106c1a7d864814a02c320bcf27a180e00a7ab0e6afb59deb130b` |
| `344M50000040039_20171008_428M60000040055` | 2017-10-08 | `172231ba1d950b51a19f888a24d1cf3a58f24fa5e3a7d84067a3188ba44dd993` |
| `344M50000040039_20180401_430M60000040009` | 2018-04-01 | `838ca18e644cf0546fbea45bdbac257e7f9862b4a740be01f283738811d1d829` |
| `344M50000040039_20181230_429M60000040001` | 2018-12-30 | `297920fb43841c207d552180b31b225805c9b93120750f7de917985f53697e03` |
| `344M50000040039_20190701_501M60000040008` | 2019-07-01 | `72cf75fc2bbb9070bf183e42b08b29e5fe1fa0766f8e20be5fc59209bb692c2a` |
| `344M50000040039_20220101_503M60000040070` | 2022-01-01 | `09799704098a563a731c1e30474d95c4bdf45c2fecc28701162c098a5190a442` |
| **`344M50000040039_20230401_505M60000040029`** (embedded) | 2023-04-01 | `8dfd625b110a8fd9699f6a17d583c17b0cb3a957388da398b7484392a8ddfb56` |
| `344M50000040039_20241215_505M60000040057` (current) | 2024-12-15 | `1faf66ba359030d31a2149b384d3e15ce2c04dced6b612c7795a0153f1b84af4` |

URL pattern of each XML: `https://laws.e-gov.go.jp/api/2/law_data/<revision id>?response_format=xml`. The schedule cites the law page of the embedded revision, <https://laws.e-gov.go.jp/law/344M50000040039/20230401_505M60000040029>.

The 2024-12-15 revision changed other parts of the ordinance. Articles 8 and 9, the remarks and every row of the table are identical to the 2023-04-01 revision (compared as parsed text), so one embedded window runs from 2023-04-01 with no end date.

## What is embedded

| Part | Count |
|---|---|
| Table entries of the Appendix Table | 244 (398 XML table rows; sub-items have an empty first cell and are folded into their entry) |
| Stored rows | 251 (13 two-digit chapter rows, 238 four-digit heading rows) |
| Notes quoted from the ordinance | 9: Article 8, Article 9 paragraphs 1 to 3, remarks 1 to 5 |
| Unde Labs caveat | 1 (no ref, no link) |

Eleven chapter entries name headings they do not cover (for example chapter 4 "except heading 04.10"). They are stored as ranges that skip those headings: 04.01 to 04.09, 18.03 to 18.05, 38.10 to 38.20, 38.24 to 38.27, 42.01 to 42.03, 54.01 to 54.03, 54.07 to 54.08, 57.01 to 57.03, 62.01 to 62.12, 63.01 to 63.07, 81.01 to 81.12, 90.02 to 90.29, 90.31 to 90.33 and 94.01 to 94.03. Heading 04.10 (edible insects, new in HS 2022), 18.01, 18.02 and 90.30 therefore have no row from the chapter entry.

Every stored row carries `"default": true`, the existing group-row flag: it applies only to codes in the embedded list of the schedule's HS edition (below). A row keeps the table's exact text; branches are joined with a line break, and a sub-item label such as `（ｉ）` stays in the text.

Rows and notes are located by `ref`, not by PDF page. A row ref reads `別表 row 37 (一八・〇六)`: the 1-based row of the table in the XML where its entry starts, and the table's own code. The evidence citation is the schedule citation, a comma and the ref, and the link is the e-Gov law page with no `#page=` fragment. The stored `page` is 1.

Notes are stored once for the schedule (scope `schedule`) instead of once per HS section. Article 9 paragraph 2 (chapters 50 to 63, the 10% weight tolerance) and remark 4 (section XI) apply only to chapters 50 to 63; remark 5 applies only to chapters 61 to 63. The Unde Labs caveat is the first evidence item (schedule field `caveat_first`), before the rows, so the reader sees what was not checked first. It also says that any different or relaxed rules for particular beneficiaries (for example least-developed countries) are not in this text and were not checked.

## HS edition and the HS 2022 code list

The ordinance names no HS edition; it points to the numbers of the Customs Tariff Law Annex in force. The citation says "HS edition not stated; codes validated against HS 2022". HS 2022 is inferred because the embedded window starts after the 2022-01-01 HS revision, which the table follows (it lists heading 24.04, an HS 2022 heading). This is an inference, not a statement of the ordinance.

`backend/internal/jprules/data/hs2022_codes.txt` lists the 5,612 real HS 2022 six-digit subheadings, one per line, sorted, codes only. sha256 `09b1a9a72ee8ffe52a15c173c233201d0d1db18af7f9c30684c5ac4c4379597c` (pinned in a test).

- Source: UN Comtrade reference file for HS 2022 (H6), <https://comtradeapi.un.org/files/v1/app/reference/H6.json>, retrieved 2026-11-24, 1,750,838 bytes, sha256 `e266823e1299d1307c5d7986eb738b83aa5e5bd9af675bf19bf372220660be15`. It has 5,613 six-digit rows; the pseudo-code 999999 ("unspecified") is dropped.
- Cross-check done at extraction: the UNSD HS 2022 to HS 2017 conversion table (`HS2022toHS2017ConversionAndCorrelationTables.xlsx`) has 5,613 distinct HS 2022 codes, which are exactly the 5,612 codes plus 999999: none missing, none extra. A test also checks that every HS 2022 code named by a reviewed correlation entry is in the list and every parent is in the HS 2017 list. The WCO correlation PDFs were not machine-parsed; that the UNSD table equals the WCO table row for row is unverified.
- Licence: the same caveat as the HS 2017 list (docs/japan/ajcep-general-validity.md): UN Comtrade terms restrict copying without permission, and I found no WCO statement permitting embedding. The file holds bare numeric identifiers only; this is not a legal opinion. If the owners object, remove the file and regenerate it from another source. Do not add descriptions.

Schema change (additive): a schedule may carry `hs_edition` (2017 or 2022). It selects which list gates the rows flagged `default`. Absent means 2017, so every earlier schedule behaves as before; a lookup over all 15.5 million combinations of 16 existing agreements, 6 dates and the 4 and 6 digit codes returned the same results before and after (hash of the full output compared). A code of a gated schedule that is absent from the selected list returns `not_in_schedule_edition`, so `0100.99` or `6499.99` never returns the chapter row. Other additive fields: `ref` on rows and notes, and note scope `schedule`. Load validation rejects an unsupported edition, a schedule that mixes `ref` and page numbering, a blank or padded ref, a `ref` row whose page is not 1, a source URL with a fragment, a schedule note with a key, and a caveat with a ref.

## Behaviour

- Windows. 2023-04-01 and later: rule text. Before 2023-04-01: `no_dated_source` (nothing is embedded for earlier windows).
- Goods not in the table (for example 0101.21, 2710.12, 8471.30, 04.10) return `no_match`. No default is invented: the ordinance's general rule (Article 9 paragraph 1, change of tariff heading) is quoted as a note whenever a row is returned, but is not returned as a rule row on its own.
- A test runs every one of the 5,612 HS 2022 codes: 2,766 return exactly one rule row (plus notes), 2,846 return `no_match`. No code absent from the list returns a rule (every six digit code from 000100 to 999999 not in the list was tried).
- The prompt and answer schema are unchanged. Sources shown to the user list the e-Gov law page once per cited item, never the caveat.
- The new-case selector is not changed: GSP stays out of `japan-preference.ts` until a later, separate step.

## Guard against a qualifying answer

GSP is reachable through the API even though the selector does not list it. Because the beneficiary country, Cabinet Order coverage, Form A and cumulation are not checked, `Predict` never returns `qualifies` for GSP. A model answer of `qualifies` is changed after the model call to `undetermined` (criterion `NONE`, origin cleared) with a review reason saying the rule may be met but a specialist must confirm those four points. The model's reasoning text and confidence are kept. `does_not_qualify` and `undetermined` stay as the model said, other agreements are unchanged, and the prompt is unchanged.

## What is and is not claimed

Claimed: the Japanese text of the rows and notes above is the text of the 2023-04-01 revision (checked against the XML by an extraction independent of the one that built the data, for all 251 rows and the nine notes), and a code is shown a row only when the code exists in HS 2022.

Not claimed, and stated in the caveat note: whether the exporting country is a GSP beneficiary; whether the good is covered by the preference under the Cabinet Order for Enforcement (including its appended table) on the import date; the certificate of origin (Form A); cumulation; direct consignment. A rule text reading is not an eligibility finding.

## Recorded gap: earlier windows

Not embedded (a ruling of this pilot): import dates before 2023-04-01 return `no_dated_source`. The ordinance has three earlier windows, each extracted and checked against its XML (no row or note mismatch) but not shipped:

| Window | Revisions | Change from the previous window (table entries) |
|---|---|---|
| 2017-04-01 to 2018-03-31 | 2017-04-01, 2017-10-08 | first window in the e-Gov history |
| 2018-04-01 to 2021-12-31 | 2018-04-01, 2018-12-30, 2019-07-01 | text of 18.06, 19.01, 21.01 and 21.06 changed |
| 2022-01-01 to 2023-03-31 | 2022-01-01 | 24.04 added; text of chapter 16, 20.09, 22.02, 59.11, chapter 94 and 96.17 changed (HS 2022 revision) |
| 2023-04-01 onward | 2023-04-01, 2024-12-15 | text of 44.18 changed; 2024-12-15 changes nothing in the embedded parts |

Table text is identical within a window (checked by hashing). The ordinance names no edition in any window; which HS edition applies to the first two windows is not verified here, and shipping them would need its own code-list check.

## Remaining gaps and honest notes

- Rows that cover only part of a heading. 33 entries read "of which ..." (のうち) in their description, so the rule is for those goods only and the rest of the heading falls under the general rule of Article 9 paragraph 1, which is not embedded as a row: 13.02, 15.04, 15.11, 21.01, 21.03, 22.02, 28.52, 29.05, 29.06, 29.34, 35.02, 36.06, 38.01, 38.06, 43.02, 44.07, 44.12, 44.18, 44.20, 46.01, 50.06, 53.08, 56.04, 65.05, 70.13, 75.08, 78.06, 79.07, 80.07, 96.01, 96.03, 96.08 and 96.14. A code in such a heading gets the row and the caveat says to read its description. The lookup cannot tell whether the product is within the described part.
- Other narrowing wording. Row descriptions and rule text carry restrictions such as 「固体のものに限る」 (limited to solid goods, for 17.01) that limit a row to part of a heading. Only the 33 "のうち" entries and the 9603.10 exclusion were listed above; other narrowing words were not enumerated, and each row must be read in full.
- Sub-heading exclusion. The 96.03 entry excludes sub-heading 9603.10 in its own text ("第九六〇三・一〇号に該当するものを除く"). The row is returned for 9603.10 too, with the full text; it is not marked withheld because the existing semantics do not fit a sub-heading exclusion inside an entry.
- Conditions of the form "only if X" inside an entry stay in the text for the model and the reader; they are not evaluated.
- The Cabinet Order (Arts 26 to 31 and its Appended Table 2) and Japan Customs' notices are not embedded. The list of beneficiary countries and the products covered by the preference on a date are not checked.
- No Japanese customs specialist has reviewed the text.
- No rebuild script is committed: the extraction used local helper scripts that read the e-Gov XML. The embedded data can be compared with the XML at the hashes above.

## Development counts (offline, no model)

`jpeval diagnose -split dev` (2,973 cases; 131 GSP cases, all dated 2023-04-01 or later): GSP before, 131 `unsupported_agreement`; after, 130 `psr` and 1 `no_match`. Eligible total 2,820 to 2,950; abstained 153 to 23. `dev_eval` (400 cases; 7 GSP): before 7 `unsupported_agreement`; after 7 `psr`; eligible 388 to 395, abstained 12 to 5. No other agreement changed. These counts show rule availability only, not accuracy.
