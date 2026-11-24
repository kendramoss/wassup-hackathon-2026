# Japan model-answer validation

What the Japan engine checks in a model answer before it reaches a result. The
prompt, prompt version and request bytes are unchanged.

## Shipped
- Origin of a qualifying answer: exactly one country, or the EU. Japan (the
  importing country), regions and placeholders (Asia, ASEAN, Various, Not
  applicable), and several countries in one string (commas, "and", "or",
  slashes, semicolons) are rejected as an invalid answer. Accepted names are
  the shared country table plus Brunei and Myanmar, which the table lacks. A
  test checks that every party of the listed agreements is accepted.
  An "undetermined" answer is still an abstention, not an error.
- A blank deciding rule (spaces, control or invisible characters only) is
  rejected for a qualifying answer.
- Reasoning is cut to 2000 characters and the deciding rule to 200, ending in
  an ellipsis. Control and invisible format characters (NUL, bidi overrides,
  zero width marks) are removed; newline and tab stay.
- Ruling numbers the model was not shown are replaced with "an unverified
  reference" in reasoning and deciding rule. This covers "JP 1260484",
  "JP_1240271", "JP-1240271", lowercase, fullwidth letters and digits, and
  numbers written back to back. A real ruling id has seven digits, so "JP"
  followed by fewer than five digits is left alone. A match directly after a
  letter is part of another word and is left alone. Ids of the retrieved
  precedents stay.
- Case id and excluded rulings are read as "1240271" or "jp1240271" as well as
  "JP1240271" before retrieval.
- The retrieval query is cut to the same length the prompt shows.

## Known limit
GSP beneficiary countries outside the shared country table are rejected as an
origin until they are added. Not measured.

## Not shipped (prospective, change what the model sees)
- Irrelevant precedents: the query includes the agreement code, which every
  ruling of that agreement contains, so unrelated facts still return k
  precedents (measured: the code plus nonsense facts returns 5; the facts alone
  return 0).
- Duplicate precedents: 295 same-agreement groups share identical facts; 516 of
  3606 rulings, queried by their own facts, get an earlier identical-facts
  twin; 1120 of 17699 slots (6.3%) are such twins.
