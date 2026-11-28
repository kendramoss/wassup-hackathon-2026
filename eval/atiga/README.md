# ATIGA conformance checks

These checks measure agreement with declared outcomes and component-rule checks. They do **not** measure real-world accuracy.

## Results

| Evidence | Passed | Meaning |
|---|---:|---|
| Authority worked examples | 7/7 (100%) | The original MITI examples 1–5 and Singapore Customs examples B/F, not new independent decisions |
| Constructed rule checks | 35/35 (100%) | Synthetic boundary and missing-fact cases, written with knowledge of the existing engine |

The groups are not pooled into an accuracy figure. No population interval is justified by these selected teaching examples and constructed cases.

## Reproduce

From `backend/`:

```sh
go run ./cmd/atigaeval > /tmp/atiga-results.json
cmp /tmp/atiga-results.json ../eval/atiga/evaluation-results.json
go test ./cmd/atigaeval ./internal/atiga
```

The command exits unsuccessfully for invalid evidence or any mismatched check, but emits the complete result report when scored cases fail. The Go tests also compare the report byte for byte, verify the original reference-file hash, and check that authority ids, inputs, sources and scored expectations still match the original seven. A synthetic case cannot silently enter the authority group.

## Frozen protocol

D963 and commit `7991b45a` freeze `evaluation-cases.json` before scoring. Candidate engine and rules: main `792f0fe9`, unchanged by this work. D964 records the result. `evaluation-results.json` retains every case, expected checks, actual result and failures. Deterministic reproductions do not add independent observations.

The authority group checks declared outcomes, applied rules, alternative-rule outcomes and accumulation flags. It does not score the original numeric RVC ranges; those remain exercised by the unchanged `internal/atiga/reference_test.go`. Singapore example B prints 36%, while its listed amounts compute to 36.67%; both fail RVC40. D927 and its known-deviation test preserve that discrepancy. For example B, this evaluation scores the **RVC40 component failure**, not a complete eligibility refusal.

## Added controls

The 35 synthetic cases cover direct/indirect RVC40 boundaries, CTSH, full and partial cumulation, minimal operations, absent facts, de minimis, textile process requirements and steel exclusions. Expectations come from the rule clauses identified in each case, not a new panel of independent customs decisions. Partial cumulation values are manual calculations under Article 30(2)/Annex 6: with FOB 100, a Vietnamese material worth 50 and another non-originating material worth 20, RVC is 30% before partial credit; a qualifying 20% material adds 10, reaching 40%.

The unknown-origin control deliberately has the material's value and code but no proof of originating status. Singapore Customs' handbook, page 6, includes materials whose origin cannot be determined among non-originating materials; Article 29(2)(b) addresses their valuation. Counting them as non-originating is conservative on the supplied facts, not proof that obtaining a declaration could never change the answer.

## Source search

The bounded search verified no additional eligible ATIGA decisions or worked outcomes. The locally archived December 2025 Singapore handbook was inspected, including its agreement labels. Its jam, handbag, chemical and de-minimis examples are for other agreements and were not relabelled as ATIGA. Archived ATIGA Annexes 5 and 6 state costing and partial-cumulation rules, not further worked outcomes. This does not establish that no ASEAN authority publishes decisions.

Source URLs and checksums are retained in the case files. Public sources include [MITI](https://www.miti.gov.my/index.php/pages/view/2071), the [Singapore Customs handbook](https://isomer-user-content.by.gov.sg/174/fc3ecb0b-d701-4d8f-9881-9d99f90f35a2/Handbook_on_ROO_for_PCO_Dec2025__clean_.pdf), and the [ASEAN Secretariat ATIGA materials](https://asean.org/asean-trade-in-goods-agreement-atiga/).

Specialist review is deferred, not a hackathon release gate. Nothing here claims that professional review occurred.
