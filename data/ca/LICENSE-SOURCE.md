# Source and licence: CBSA rulings

`rulings.jsonl` holds the origin advance rulings and marking national customs
rulings the Canada Border Services Agency publishes, with each applicant's
consent, under "Rulings issued by the CBSA"
(https://ccp-pcc.cbsa-asfc.cloud-nuage.canada.ca/en/national-rulings).
Retrieved 26 Nov 2026 by `eval/ca/fetch.py`; parsed by `eval/ca/parse.py`.

Source: Canada Border Services Agency. Reproduced under the Canada.ca Terms
and Conditions (https://www.canada.ca/en/transparency/terms.html): non-commercial
reproduction is allowed with attribution and a link to the source; commercial
reproduction needs the CBSA's permission or a separate licence. This
reproduction has not been produced in affiliation with, or with the
endorsement of, the Government of Canada.

The `holding` field is our one-line summary of each decision, not CBSA text.
`facts` and `reasoning` are shortened excerpts; each row's `url` opens the
full ruling.
