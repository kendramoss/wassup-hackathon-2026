# Missing facts in the Japan review text

When a Japan preference check ends as "undetermined" and the case has a
structured product, the review message now adds up to three short things the
user can fill in, then "And N more." The model prompt is unchanged.

## What is checked
- No materials listed.
- A material with a blank "made in" (blank means unknown, not zero and not originating).
- No production steps listed.
- A production step whose plant is not found or has no country.

Repeated material names (ignoring case) are listed once. Names are shown as
entered, in any language. Free-text cases with no structured product get no
extra text. "Qualifies" and "does not qualify" results are untouched.

## What is not checked
- Values or percentages of materials (the case data has none).
- Whether a stated country is correct.
- Meaning of the free-text facts (for example negations are not parsed).
- Translation fidelity of non-English input; this is unmeasured.

This is guidance for completing the input. It makes no accuracy claim.
