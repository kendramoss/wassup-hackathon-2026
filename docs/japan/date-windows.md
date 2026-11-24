# Schedule date windows

The `Applied from` date of each embedded schedule filters case dates. It is the earliest start of the embedded edition, not always a legal in-force date. A treaty entry into force is not the validity of a particular HS schedule edition, so a date is changed only with a pinned official source for that exact edition. Audit retrieved 2026-11-25. Evidence levels: verified means read from the official Japan Customs pages for that edition; unverified means no primary source was pinned for the start date.

| Agreement | Embedded window | Evidence | Known gap |
|---|---|---|---|
| JP-EU EPA | 2019-02-01 onward | Customs index entry-into-force line (not re-read in this audit) | none known |
| AJCEP (HS 2017) | 2023-01-01 to 2027-02-15 | End verified (index "2027.2.14" and "2027.2.15~"); start unverified | See AJCEP below |
| AJCEP (HS 2022) | 2027-02-15 onward | Verified from the index; future-dated | none |
| CPTPP | 2018-12-30 onward | Japan entry into force; no later Japan annex on the index | Per-origin party dates are not modelled |
| JP-India CEPA | 2011-08-01 onward | Entry into force verified; edition is the 2011 original | Chapter 3 amended from 2022-04-04, amended text not found (below) |
| JP-Thailand EPA | 2023-01-01 onward | Start unverified (see Thailand below) | 2022 edition candidate, HS 2002 edition before |
| JP-Indonesia EPA | 2024-02-05 onward | Verified on the EN and JA index (old annex ends 2024-02-04) | Earlier HS 2002 annex not embedded |
| JP-Australia EPA | 2015-01-15 onward | Entry into force, not re-checked | none known |
| JP-UK CEPA | 2021-01-01 onward | Entry into force, not re-checked | none known |
| JP-Vietnam EPA | 2009-10-01 onward | Entry into force, not re-checked | none known |
| JP-Malaysia EPA | 2006-07-13 onward | Unverified: the revised Annex 2 page returned 403 | Edition cannot be confirmed |
| JP-Mexico EPA | 2005-04-01 onward | Unverified: protocol in force 2012-04-01, edition of the embedded Annex 4 unknown | Edition cannot be confirmed |
| JP-Philippines EPA | 2008-12-11 onward | Entry into force, not re-checked | none known |
| JP-Switzerland EPA | 2009-09-01 onward | Entry into force, not re-checked | none known |
| JP-Chile EPA | 2007-09-03 onward | Entry into force, not re-checked | none known |
| JP-US TA | 2020-01-01 onward | Entry into force, not re-checked | none known |
| GSP | 2023-04-01 onward | Deliberate pilot window | Earlier revisions extracted, not embedded (see `gsp-pilot.md`) |

RCEP is not a `schedules.json` entry. It reuses Annex 3A and starts at 2023-01-01 (`rcepStart`).

## Known gaps

Only one development ruling falls in any gap below (id 1231682, JP-Indonesia, HS 2402.10, dated 2023-12-21). Dev rulings start 2023-11-09, so early-window cases are almost absent from the dev data. That count says nothing about accuracy elsewhere.

- **AJCEP HS 2017 start.** 2023-01-01 is the earliest embedded start. No primary source for an official HS 2017 start is pinned. Unverified secondary summaries suggest the Joint Committee adopted it on 2022-08-24 with effect from 2023-03-01. Consequence: cases dated 2023-01-01 to 2023-02-28 may receive an HS 2017 rule (or the general criteria) where the HS 2002 annex might officially apply. The old HS 2002 annex is still hosted by Japan Customs and is not embedded; dates before 2023-01-01 return `no_dated_source`.
- **JP-Thailand EPA start.** The embedded Annex 2 (HS 2017, `thailand2-2.pdf`) starts 2023-01-01. The Japanese Customs index labels its Japanese edition (`thailand3-2.pdf`) "2022.1.1~". The two files were compared: same creation date (2021-06-30) and modification date (2021-08-20), every subheading read from the Japanese file (758) is also in the English file, and the English text states it is based on HS 2017. Search summaries (unverified, not primary) say the exchange of notes of 2021-07-05 took effect on 2022-01-01. The official notice itself could not be retrieved (Customs page not found, MOFA returned 403), so the date is not changed. If a primary notice is pinned, the start moves to 2022-01-01 and cases from 2022-01-01 to 2022-12-31 would change from `no_dated_source` to a rule. The earlier HS 2002 annex (`thailand2.pdf`) is not embedded. Thailand notes: [thailand-scope-notes.md](thailand-scope-notes.md).
- **JP-Indonesia before 2024-02-05.** An earlier HS 2002 Annex 2 (`indonesia2.pdf`) exists and is not embedded; those dates return `no_dated_source`. Scope notes: [indonesia-scope-notes.md](indonesia-scope-notes.md). That note covers the Annex 2 PDF only and says nothing about other treaty provisions.
- **JP-India chapter 3.** A Japan Customs notice says Annex 2 chapter 3 rows were amended from 2022-04-04. The embedded `india2.pdf` is the 2011 original and the amended text was not found, so chapter 3 codes dated from 2022-04-04 may carry the superseded rule. No amendment is added without source-checking every condition of it.
- **RCEP before 2023-01-01.** The original 2022 annex is an image-only PDF. `rcepStart` is deliberate and is not lowered.
- **AJCEP and Thailand HS 2002 editions.** Different HS edition, no reviewed correlation, not embedded.
- **GSP earlier windows.** Revisions from 2017-04-01, 2018-04-01 and 2022-01-01 were extracted and checked, not embedded.
- **Mexico and Malaysia.** Revised pages are unreachable, so the editions are unverifiable.
- **AJCEP HS 2022 from 2027-02-15.** Real, from the index, and future-dated; nothing today reaches it.

Source index pages (retrieved 2026-11-25): https://www.customs.go.jp/roo/english/text/index.htm (sha256 c0dba02637417ebbea1cf1f985e87a66b8fc20dbc0e151cc8b15055fd5ea766d) and https://www.customs.go.jp/roo/text/index.htm (sha256 516706f55e51fb82f7fc7bd22424f8718e177131b4a1955c65c860fbc9140edc). Thailand Japanese Annex 2, https://www.customs.go.jp/roo/text/thailand3-2.pdf, sha256 2472b2ece3b55962a58e1cbf273d68bca0783341ba8cfac91bd4d0c3c214dd6b. English `thailand2-2.pdf` sha256 f87a52b564a4e6b7c18207c6a0cf6c6cbb244f81f1e6c79204f03f3437db9fbc.
