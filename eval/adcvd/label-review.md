# Label review (D940)

Each of the 38 outcomes in `labels.jsonl` was compared with the closing sentence of its entry in `rulings.jsonl`.

- 36 outcomes agree with the entry's wording. No label was corrected, so the scored labels are the model's.
- 2025-16099-2 and 2025-16099-5: the entries say Commerce rescinded the scope inquiry and state no finding. The model's `mixed` follows the prompt's rule for an entry that does not state one result. Not scored.
- 2025-16099-1: one product within and one outside; `mixed`. Not scored.
- Preliminary results (2025-07356-8, 2025-16099-9, 2026-01455-7, 2026-01455-8) count as the result stated, as D940 says before the run.
- Entry 2026-04942-3 lists C-750-118, which is the notice's own typo for C-570-118; the case number is kept as printed.
- `turns_on_origin` agrees with the entry in every case read: true for the Mexico sugar and mattress, the Thailand wheels and lawn mowers and the xanthan gum products; false otherwise.
