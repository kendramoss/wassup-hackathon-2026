# HS edition correlation for the Japan lookup

Some Japan Customs advance rulings carry a six-digit code from a later Harmonized System (HS) edition than the one a treaty schedule uses. The schedule has no row for such a code, so the lookup used to return `no_match` and the case went to review. This change adds a small, reviewed list of correlations in `backend/internal/jprules/data/correlations.json`.

## What it does

The list applies only when a schedule covers the case date, the lookup would return `no_match`, and the list has an entry for that exact agreement and code. Any other outcome is untouched, including a matched rule, `too_broad`, `withheld`, `invalid_code`, `excluded_general` and the AJCEP general rules. The edition is never guessed from the case date. Each entry is for a code that does not exist in the schedule's edition.

- `mapped`: every parent code returns the same schedule row. The evidence is that row, unchanged and with its own citation, behind an Unde Labs note (no URL, not treaty text). The note names the requested code, the parent codes, the schedule edition and the WCO table page. It says the correlation is administrative and partial ("ex"), not a rule for the requested code, and that the requested code stays the declared tariff code. The case then goes to the model like any other case with a rule.
- `edition_hint`: the case goes to review with a short reason naming the parent codes. No rule text is shown as evidence.
- If the parents of a `mapped` entry do not return identical rows, the lookup downgrades to `edition_hint`.

## Entries

| Agreement | Code | Schedule edition | Parents | Result |
|---|---|---|---|---|
| JP-EU EPA | 2404.11 | HS 2017 | ex 2403.91, ex 2403.99 (one row, PDF page 12) | mapped |
| CPTPP | 2202.99 | HS 2012 | ex 2202.90 (one row, PDF page 40) | mapped |
| CPTPP | 3824.99 | HS 2012 | ex 3824.90 (range row 3824.10 to 3824.90, PDF page 56) | mapped |
| JP-Malaysia EPA | 3824.99 | HS 2002 | ex 3824.20, 3824.71, 3824.79, 3824.90 (range row 3809.91 to 3824.90, PDF page 28) | mapped |
| CPTPP | 0307.43 | HS 2012 | ex 0307.49 and ex 0307.99, with different rules | edition_hint |

Not mapped: 2404.12, 2404.19, 2404.91, 2404.92 and 2404.99 stay `no_match`. Their WCO sources include a different chapter (ex 3824.99, ex 2106.90), so one schedule row does not follow.

## Data repair

The Japan-Malaysia schedule split the rule on PDF page 28 for 3809.91 to 3824.90 into two rows, one truncated after "3809.91 through". It is now one row with the full text of the page, and a test pins it.

The same truncation pattern is widespread in other rows. Many JP-Malaysia rows (for example 0901.11, 1513.11 and 1602.xx) and some JP-Thailand rows are split into a first row that ends mid-sentence and a continuation row. None of them were changed here; they are tracked as separate work.

## Sources

Retrieved 2026-11-25. Page numbers are PDF page indexes. The WCO tables are Secretariat guidance, not binding classification decisions.

| Source | SHA-256 | Pages used |
|---|---|---|
| WCO HS 2022 to HS 2017 Table I | `e6aa91163b993bee6d7bf77c2a5e01914c6a9794829f9164491442ec2b1c1361` | 6 (2404.11), 5 (2202.99), 11 (3824.99) |
| WCO HS 2017 to HS 2012 Table I (rev 1) | `0c143ddcd4f670f6d2310afe16cdc4d324bec4c735ca3a016c29e9cda53c0f6a` | 9 (2202.99), 15 (3824.99), 8 (0307.43) |
| WCO HS 2012 to HS 2007 Table I (rev 3) | `a811946b3240aa8389c9f77258640acb0d0ed74e61529a39c56a829a0bbd6cd8` | 20 (3824.90) |
| WCO HS 2007 to HS 2002 Table I (rev 5) | `bef4b83f07b72f638ef78b115fcb1872a9f17134c00e64f4677cfc0611c7c6f4` | 13 (3824.90) |
| JP-EU EPA Annex 3-B (`eu3.pdf`) | `cb7f029ad6679a70796ba45f619e1a623d8e9304ed041069a4b007cdc9633704` | 12 |
| CPTPP Annex 3-D (`tpp2.pdf`) | `c80acd1e0c458f130cc6d6602ce18192626e801623947fffc9d1ed18020873b9` | 15, 16, 40, 56 |
| JP-Malaysia Annex 2 (`malaysia2.pdf`) | `10ad097b32caa1de4a2d04a6ecfa67ae118a32da85e135ba3e797402d3e329d3` | 1 (HS 2002 basis), 28 |

The URLs are in `correlations.json` and in `schedules.json`.

## What is not claimed

- Japan has published no later-edition annex for JP-EU (HS 2017) or CPTPP (HS 2012) in the sources checked. The New Zealand HS 2022 text of CPTPP Annex 3-D is New Zealand's certified instrument, not Japan's rule, and is not used.
- No claim that a mapped rule is "the" rule for the requested code, or that a good qualifies. The WCO "ex" link means only part of the old subheading went to the new one.
- Whether a Japanese Ministry of Foreign Affairs revised Malaysia annex supersedes the 2002 edition is unverified (the page was not reachable).
- Corrigenda to the 2012 to 2017 and 2007 correlation tables were not read in full.

## Dev-split rule availability

`go run ./cmd/jpeval diagnose -split dev` and `-split dev_eval`, run before and after the change. These count whether a rule source can be shown before any model call. They are not accuracy, and no label was read.

| | dev (2973 cases) before | dev after | dev_eval (400 cases, subset of dev) before | dev_eval after |
|---|---|---|---|---|
| Eligible total | 2589 | 2674 | 366 | 367 |
| psr | 2573 | 2573 | 365 | 365 |
| general | 16 | 16 | 1 | 1 |
| mapped | 0 | 85 | 0 | 1 |
| Abstained | 384 | 299 | 34 | 33 |
| JP-EU 2404.11 | no_match 80 | mapped 80 | 0 | 0 |
| CPTPP 2202.99, 3824.99 | no_match 4 | mapped 4 | 0 | 0 |
| CPTPP 0307.43 | no_match 14 | edition_hint 14 | no_match 2 | edition_hint 2 |
| Malaysia 3824.99 | no_match 1 | mapped 1 | no_match 1 | mapped 1 |

Remaining misses in the dev split are 7007.29 under JP-EU (fixed since: it now returns the rule, see [withheld-rows-narrowing.md](withheld-rows-narrowing.md)), the 0307.43 hints, and agreements without a schedule.
