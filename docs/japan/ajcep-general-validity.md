# AJCEP general fallback requires a real HS 2017 code

The AJCEP general criteria (Articles 23 to 35 extract) used to be returned for any code with no product specific row, including codes that do not exist in HS 2017 (for example an HS 2022 code, or a typo). Now the general fallback is returned only for a real HS 2017 code. Any other code returns `invalid_code` and the case goes to review.

## Behaviour

Order inside `LookupDetailed` for AJCEP in the HS 2017 window (2023-01-01 up to, not including, 2027-02-15):

1. Product specific rows, `too_broad`, `withheld` come first, unchanged. A chapter-wide row (for example chapter 01) therefore still returns `psr` even for a code absent from the list.
2. Reserved chapters 00, 77, 98 and 99 return `invalid_code`.
3. Not a real HS 2017 code returns `invalid_code`. This comes before `excluded_general`, so a non-existent chapter 50 to 63 code reports `invalid_code`, not `excluded_general`. A real chapter 50 to 63 code without a row stays `excluded_general`; the shipped schedule has a row for every real textile code, so today that is only reached on a modified schedule in a test.
4. Otherwise `general`.
5. The correlation step still runs only on `no_match`. When a reviewed correlation exists for the exact agreement and six-digit code, the invalid code is left as `no_match`, so the correlation can return `mapped` or `edition_hint` before `invalid_code` would apply. No AJCEP correlation ships today; a test injects one to prove the order. Correlations never rescue a reserved chapter.

Code lengths: the lookup uses four or six digits. A six-digit code must be in the list. A four-digit heading is valid if some listed subheading starts with it (a heading present in HS 2017 keeps today's behaviour; a heading that does not exist is `invalid_code`). Nine-digit national codes are cut to six first. The list is HS 2017 only, so the HS 2022 AJCEP window and other agreements are unchanged.

## Source record

- File: `backend/internal/jprules/data/hs2017_codes.txt`, one six-digit code per line, sorted, 5,387 lines, codes only (no descriptions or hierarchy).
- sha256 of the file: `516da3067b86077fe0508822e70076ea7a0c6721f7bb853c6fc524030dc4eb73` (pinned in a test and in the `jpeval diagnose` candidate hashes).
- Source: UN Comtrade reference file for HS 2017 (H5), https://comtradeapi.un.org/files/v1/app/reference/H5.json, retrieved 2026-11-24, 1,632,156 bytes, sha256 `079987eba4e5e1f60e63368e2aa8d7ac68c29fa6f3a889466dbd5276272f2431`. It has 5,388 six-digit rows; the pseudo-code 999999 ("unspecified") is dropped, leaving 5,387.

## Cross-checks (all done at extraction)

| Source | Six-digit codes | Result |
|---|---|---|
| UNSD Economic Classifications workbook (H5, level 6) | 5,388 | identical to the UN file |
| UNSD HS 2017 to HS 2012 table | 5,388 | identical |
| UNSD CPC 2.1 to HS 2017 table | 5,387 | all but 999999 |
| UNSD HS 2017 to SITC 4 table | 5,386 | lacks 710820 and 711890 |
| UNSD HS 2022 to HS 2017 table, HS 2017 column | 5,387 | all but 999999, no extras |
| Japan Customs tariff schedule as of 2021-04-01 (HS 2017 edition) | 5,386 | lacks 271600 and 999999, no extras |

The gaps are reporting or national omissions in those sources, not evidence against the list. The WCO correlation PDFs were not machine-parsed; that the UNSD HS 2022 table equals the WCO PDF row for row is unverified.

Completeness against this repository: a test checks that every six-digit start and end of every schedule whose citation says "(HS 2017)" is in the list, and that two- and four-digit starts and ends have a listed code under them. Other tests check the count, the format (six digits, sorted, unique, no 999999) and the file hash.

## License caveat

The UN Comtrade license agreement (https://comtradeplus.un.org/en/license-agreement, section 5) says material may be copyrighted by the United Nations and that copying, downloading, redistribution and publication are prohibited without prior written permission, except as permitted by law. I found no open licence for the H5 file. The World Customs Organization owns the HS nomenclature text; I found no WCO statement permitting embedding (unverified). The embedded file is only a list of bare numeric identifiers: no descriptions, no hierarchy and no UN structure. That is a set of facts and a low copyright risk, but this is not a legal opinion. If the owners object, remove the file and regenerate it from another source. Do not add descriptions.

## Dev diagnosis, before and after

`jpeval diagnose -split dev` (2,973 cases, offline, no model):

| | before | after |
|---|---|---|
| eligible total | 2,673 | 2,672 |
| psr / general / mapped | 2,572 / 16 / 85 | 2,572 / 15 / 85 |
| abstained | 300 | 301 |
| AJCEP reasons | general 16, psr 557 | general 15, invalid_code 1, psr 557 |

The one change is a case with code 390729. Subheading 3907.29 is not in the HS 2017 list. Before, it got the general rules for a code that does not exist in the HS 2017 schedule. Now it is `invalid_code` and shows in `unmatched_codes`. There is no reviewed AJCEP correlation for it; adding one needs its own review and is not part of this change. `dev_eval` after the change: 400 cases, AJCEP general 1, psr 55.
