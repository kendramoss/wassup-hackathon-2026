# Japan model-answer validation

What the Japan engine checks in a model answer before it reaches a result. The
prompt, prompt version and request bytes are unchanged.

## Shipped
- Origin of a qualifying answer: exactly one country or territory from a
  closed list, or the EU. Japan (the importing country), regions, groups,
  placeholders, sentences and several countries in one string are all rejected
  as an invalid answer, because they are not on the list. Names are compared
  ignoring case, spacing, accents and fullwidth forms. Names that contain
  "and" (Trinidad and Tobago) match as whole names. Official spellings with a
  comma or brackets ("Korea, Republic of", "Lao PDR", "Hong Kong, China",
  "Myanmar (Burma)") are listed aliases and are returned as one name (South
  Korea, Laos, Hong Kong, Myanmar). Leading "the" and dots are ignored. The
  shared country helper's aliases (US, PRC, Holland, England, S. Korea) are
  tried as a second lookup, read only. UN long forms are returned as the short
  names the shared table and the eval labels use (Russia, Iran, Tanzania,
  Moldova, Bolivia, Venezuela, Syria, Palestine), so the eval comparison sees
  equal names. Kosovo, NZ, UAE, Vatican and Macedonia are project aliases. An "undetermined" answer is still an
  abstention, not an error.
  This is a well-formedness check on the answer only. It is not a treaty-party
  or origin-eligibility rule: a country that is not a party to the agreement
  is still accepted here.
  The list is the English country and area names of the UN Statistics
  Division M49 overview (https://unstats.un.org/unsd/methodology/m49/overview/,
  retrieved 2026-11-24, page sha256
  b9048114f6e7f2abda83bf03d4263c9d7cd1bd7230e3d0461025ee7839a7a1fb), the 248
  rows that carry ISO alpha-2 and alpha-3 codes, extracted to
  `countries_un_m49.tsv` (sha256
  7c8c33846a0e2dbf70acb4f36a41ff4477a55cdc59d4612b9756e9a652478662, pinned by
  a test), plus the shared country table and the listed aliases. Limits: the
  alias list is not exhaustive, so an uncommon alias is rejected until added; a
  country or territory added later, or a name written some other way, is
  rejected; this errors rather than guesses. Taiwan comes from the shared
  table because the UN list has no separate row. Tests cover every EU member
  and a sample of agreement parties; completeness against each agreement's
  party list is not claimed.
- A blank deciding rule (spaces, control or invisible characters only) is
  rejected for a qualifying answer.
- Reasoning is cut to 2000 characters and the deciding rule to 200, ending in
  an ellipsis. Control and invisible format characters (NUL, bidi overrides,
  zero width space, BOM) are removed; newline, tab and the zero width joiners
  U+200C and U+200D stay.
- Ruling numbers the model was not shown are replaced with "an unverified
  reference" in reasoning and deciding rule. This covers "JP 1260484",
  "JP_1240271", "JP-1240271", lowercase, fullwidth letters and digits, and
  numbers written back to back. A real ruling id has seven digits, so "JP"
  followed by fewer than five digits is left alone, and with a space, dash or
  underscore only seven digits count ("JP 20250101" stays). A match directly
  after an ASCII letter is part of another word and is left alone; after
  Japanese text it is masked. Ids of the retrieved
  precedents stay.
- Case id and excluded rulings are read as "1240271" or "jp1240271" as well as
  "JP1240271" before retrieval.
- The retrieval query is cut to the same length the prompt shows.

## Not shipped (prospective, change what the model sees)
- Irrelevant precedents: the query includes the agreement code, which every
  ruling of that agreement contains, so unrelated facts still return k
  precedents (measured: the code plus nonsense facts returns 5; the facts alone
  return 0).
- Duplicate precedents: 295 same-agreement groups share identical facts; 516 of
  3606 rulings, queried by their own facts, get an earlier identical-facts
  twin; 1120 of 17699 slots (6.3%) are such twins.
