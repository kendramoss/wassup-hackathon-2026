# JP-India CEPA Annex 2, chapter 3 amendment (from 2022-04-04)

The embedded JP-India CEPA schedule now has two windows. The first is the 2011 Annex 2 (`india2.pdf`), ending 2022-04-03. The second starts 2022-04-04 and differs only in the chapter 3 rows.

## Sources

| Source | Retrieved | sha256 |
|---|---|---|
| Japan Customs notice dated 2022-03-23, "日インド包括的経済連携協定の品目別規則の一部改正について", <https://www.customs.go.jp/roo/information/indiakaisei.html> (Shift-JIS page) | 2026-11-25 | `598631d6e349c0c7ea8ff2b43cef2755b26f12051f44576e45bd511548c560b2` |
| Ministry of Foreign Affairs press release dated 2022-03-19, "日・インド包括的経済連携協定実施取極及び附属書二の改正", <https://www.mofa.go.jp/mofaj/press/release/press3_000766.html>, read from the Wayback Machine copy of 2023-04-04 (the live MOFA page returned 403) | 2026-11-25 | `18b31d5a980b10769aae60e257dc60d9390b2b8f7758fa72414017fb1df7c909` |
| Embedded 2011 Annex 2, <https://www.customs.go.jp/roo/english/text/india2.pdf> | earlier | `4011ed6be153ec99ce9b199bb314c0f0fa69204677f7150bef649bf55a0b39e9` |

A second download of the Customs notice later on 2026-11-25 returned "404 Not Found", so the hash above is of the first copy, which was kept. The MOFA copy was downloaded again and has the same hash.

## What the notices say

- Effective date: the Customs notice says that from 2022-04-04 part of Annex 2 (product specific rules) of the Japan-India CEPA is amended, and that the agreement text and the other annexes are unchanged ("注：上記を除く、協定本文及び附属書については従前と変わらない"). The MOFA release has no date of effect; the 2022-04-04 date comes from the Customs notice only.
- Old row (chapter 3, 03.01-03.07): wholly obtained in the Party.
- New rows, quoted by both notices (MOFA writes the codes in kanji numerals):
  1. 0301.10-0304.92: wholly obtained in the Party.
  2. 0304.99: 第０３０４．９９号の産品への他の類の材料からの変更 (a change from materials of any other chapter).
  3. 0305.10-0307.99: wholly obtained in the Party.
- MOFA says the change concerns the product specific rule for fish paste (surimi).
- Transition clause (Customs notice): certificates of origin issued by Indian authorities under the old rule are accepted at import during their validity (one year from issue), because they plainly meet the amended rule.

In the embedded 2011 PDF, chapter 3 has exactly one row (page 2): 03.01-03.07, "Manufacture in which all the materials used are wholly obtained." No other chapter is named in either notice.

## What is modelled

- Old entry: `until` is 2022-04-04. The `until` date is exclusive, so 2022-04-03 returns the old row and 2022-04-04 the new rows (tests pin both days).
- New entry from 2022-04-04 with the same citation as the original, "JP-India CEPA Annex 2 (HS 2007)". All rows outside chapter 3 are byte-identical to the 2011 rows and keep their PDF page citations, so lookups outside chapter 3 return exactly what they did before. Only the chapter 3 rows say "as amended": their row citation reads "JP-India CEPA Annex 2 (HS 2007) as amended, effective 2022-04-04 (Japan Customs notice of 2022-03-23, ...)".
- Chapter 3 rows are quoted in Japanese from the Customs notice. The notice gives no English text, so none is added. Each row carries its own source (row fields `source_url` and `citation`, with a `ref` such as `【改正後】 row 1 (０３０１．１０－０３０４．９２)` and page 1). Evidence for such a row is cited "<citation>, <ref>" and links to the notice. This is additive: no schedule used these row fields before.
- The first new row is stored as two rows with the same text, 0301-0303 and 030410-030492, and the third as 0305-0307. The code ranges are equal to the notice's rows; the split keeps four-digit headings 0301, 0302, 0303, 0305, 0306 and 0307 answerable. Heading 0304 mixes rules, so a four-digit 0304 query returns `too_broad`.
- An Unde Labs caveat note (chapter 3 scope, applies to 0301-0307) states that rows outside chapter 3 are the 2011 rows, that no consolidated amended text or English text was found, and where the chapter 3 rows come from.
- The transition clause is a chapter 3 note (applies to 0301-0307), verbatim Japanese, with the same source. It concerns certificates, not the rule.

## What is not modelled

- No consolidated amended Annex 2 (Japanese or English) and no Indian-side notice (DGFT or Commerce) was found; English versions on the Customs page link only the 2011 texts. Rows other than chapter 3 are assumed unchanged on the strength of the notice's statement.
- HS 2017 codes 0304.93 to 0304.97 do not exist in HS 2007 and lie between the notice's ranges 0304.10-0304.92 and 0304.99. They return an `edition_hint` from 2022-04-04 (before, the old 03.01-03.07 row answered them). The WCO tables show they descend from ex 0304.99 of HS 2007 (see [hs-edition-correlation.md](hs-edition-correlation.md)); the hint quotes the amended 0304.99 rule (change from any other chapter) without choosing it. A four-digit 0304 query returns `too_broad` from 2022-04-04 because the heading now has different rules.
- The certificate transition is not modelled: no check of a certificate's issue date or the one-year validity is made.
- Whether the amendment entered into force on 2022-04-04 on the Indian side is not checked.
- Ruling dates in the development data for this agreement start 2023-11-09 and none concerns chapter 3 (24 India development rulings), so this change does not move development results.

## Codes whose answer changes on 2022-04-04

Basis: every chapter 3 code of the embedded HS 2017 list (224 six-digit codes, 8 four-digit headings), looked up for JP-India CEPA on 2022-04-03 and on 2022-04-04. Nothing outside chapter 3 differs, checked with one hash per agreement, date and chapter against `origin/main` for 6 dates and all agreements.

| Six-digit codes | Count | Change |
|---|---|---|
| Same rule, now the Japanese notice text and its citation | 210 | wholly obtained in the old and the new rows |
| 0304.99 | 1 | wholly obtained becomes change of chapter |
| 0304.93 to 0304.97 | 5 | rule becomes `edition_hint` |
| No row before or after (0308 and related) | 8 | unchanged `no_match` |

Four-digit headings: 6 keep a rule (0301, 0302, 0303, 0305, 0306, 0307), 0304 goes from a rule to `too_broad`, 0308 stays `no_match`.
