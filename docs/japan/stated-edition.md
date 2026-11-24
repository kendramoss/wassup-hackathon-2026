# Stated edition of a Japan schedule

Each entry of `backend/internal/jprules/data/schedules.json` has a nullable
`stated_edition`: the HS edition (2002, 2007, 2012, 2017 or 2022) that the
agreement's own Annex declares for its schedule. `null` means the agreement
states none. The null schedules are exactly the Japan-US Trade Agreement and
GSP.

It is not `hs_edition`. `hs_edition` names the code list (2017 or 2022) used
to gate default rows. The two can differ: GSP has `stated_edition` null and
`hs_edition` 2022.

## Where it is set

`eval/jp/build_psr.py` holds an explicit table, `STATED_EDITIONS`, keyed by
source file. The build writes the value into each entry and builds the
citation from it. The Japan-US and GSP entries are hand-maintained and carry
`null`.

## Checks at load

- A non-null value must be one of the five editions above.
- Consistency, not inference: the citation contains `(HS <edition>)` when the
  value is set. When it is null the citation says `not stated` and carries no
  `(HS <year>)`. A mismatch fails the loader.

The AJCEP general bundle matches its window on the typed value. Nothing at
runtime reads an edition out of citation text, so a phrase such as
"validated against HS 2022" cannot be taken as a declared edition.

## Verification and known gaps

- The 16 non-null values were independently checked against the Annex PDFs in
  review. The CPTPP, EU and UK editions are read from column or table headers
  of the source; the others come from a sentence in the Annex body. JP-US TA
  and GSP state no edition.
- RCEP's 2022 is still an untyped literal in the frontend leaf
  (`japan-preference.ts`) and no test guards it. This is a gap.
- The Python wording test is a guard only; it does not read any PDF.
- Loader caveat: an absent `stated_edition` key decodes as null. The data test
  guards key presence in `schedules.json`; the loader does not.
- Runtime lookups, the dev diagnosis and the evaluation plan are unchanged;
  only the data file hash differs.
