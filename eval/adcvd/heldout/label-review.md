# Label review of the held-out set (D943)

Done after the 45 labels were made and before any score was computed, as D940 did. Each outcome in `labels.jsonl` was compared with the closing sentence of its entry in `rulings.jsonl`.

- 44 outcomes agree with the entry's wording. No label was corrected, so the scored labels are the model's.
- 2025-02989-1: one product outside (roof tiles), one within (decorative board brick); `mixed`. Not scored.
- Entries 2024-17167-10 and 2023-27326-6 were read in full because their closing sentences are the end of a list of reasons; both agree (within; outside).
- `turns_on_origin` is true for 2024-12171-1 (Bahrain aluminum finished in Jordan), 2024-17167-1 (U.S. pipe processed in Mexico), 2024-17167-3 (Vietnamese chassis), 2024-17167-9 (Thai steel wheels) and 2024-26642-5 (Philippine pencils): 5 of the 44 scored.
