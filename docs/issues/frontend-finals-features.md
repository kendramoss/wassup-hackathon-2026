# Frontend work for the finals backend features (D61–D72)

The backend for each item below is on `main` (via `feat/finals-integration`). Every feature is off by default behind a server flag, so each frontend change must also work with the flag off: the endpoint returns 404, or the field is absent. Build each one on its own branch off `main`. Check it against a local backend with the flag on (`LLM_PROVIDER=fake`, or the LiteLLM backend), and with the flag off. Every change must pass `npm run lint` and `npm run build`. The API contract is `docs/api/openapi.yaml`.

Streaming predict (D71) already has its frontend part (the product step's stage list). Visitor-only cases (D58) is `docs/issues/frontend-visitor-id.md`. Teams (D68) is `docs/issues/frontend-teams.md`.

---

## 1. Regime results on the Result screen (D61 USMCA, D62 pharmaceutical) — small

**Backend:** with `USMCA_ENGINE=on` / `PHARMA_ENGINE=on`, cases the router labels `usmca_part102` or `pharma_api` get a prediction instead of "out of scope".
- Triage reasons start with "USMCA / Part 102 rules" or "pharmaceutical rules (active ingredient)".
- `prediction.trace.prompt_version` is `usmca-v1` or `pharma-v1`.

**Frontend:**
- On the Result screen, show which rules decided the case, the way textile does: a small chip such as "Pharmaceutical rules" or "USMCA rules (19 CFR 102)".
- Hide the plan search for these regimes. Moves are not modelled for them, the same as for textile.
- **Accuracy page:** add two cards to "Beyond substantial transformation", from `GET /v1/eval/summary` items once they're added (pharma held-out 89.3%, 100% at ≥ 0.95; USMCA held-out 78.1%, 100% at ≥ 0.95). If the items aren't in the summary yet, add them in `cmd/eval summary` the same way the textile items are (D60).

**Done when:** the gaiter-style check works for each regime. A pharma facts paste shows the chip, and so does a Canada/Mexico tariff-shift paste. With the flags off, both still show "out of scope" as today.

## 2. HTS suggestion on the product step (D64) — medium

**Backend:** `POST /v1/products/hts-suggest` with `{facts}` or `{product}` returns up to 3 candidate 8-digit subheadings, each with supporting rulings (ID, date, subject), a reason, `has_duty_rates`, and the note "suggestion only — confirm the classification with a licensed broker". It returns 404 when `HTS_SUGGEST` is off and is rate-limited (light).

**Frontend:**
- On the product step, next to the HTS field: a "Suggest from earlier rulings" button. Hide it if the endpoint 404s.
- Show the 3 candidates with their rulings as links to rulings.cbp.gov, and the broker note.
- Clicking a candidate fills `product.hts`. A candidate with `has_duty_rates` shows "sourced duty rates available".
- Never pre-fill silently; the user must pick.

**Done when:** a heat-gun paste suggests codes with ruling links, picking one fills the HTS field, and with the flag off the button is absent.

## 3. Live MFN rates in the money strip (D65) — small

**Backend:** with `DUTY_LIVE_MFN=on`, `GET /v1/duty-rates?hts=` and the plan's priced economics also cover HTS lines not in the researched table, using a live MFN rate from the USITC tariff schedule. Those come marked as live MFN only, with `source`, `fetched_at`, and a note that additional duties (Section 301, forced-labor, 232) are not included.

**Frontend:**
- In the Scenarios money strip and the duty-rate breakdown, show live MFN rates with a distinct label ("MFN only — additional duties not included") and the fetch date.
- Never present a live MFN rate as the full duty.
- Researched rates keep today's look.

**Done when:** a case with an HTS outside the researched table (e.g. 8516.79.00) shows the MFN rate with the warning, and the heat gun shows the researched rates as today.

## 4. Ruling-request format choice on the Draft page (D66) — small

**Backend:** `POST /v1/cases/{id}/export` accepts `"format": "cfr177"`, which returns a 19 CFR 177.2 binding-ruling request letter with `[FILL IN: …]` placeholders. It returns 400 when `EXPORT_CFR177` is off. Preview and the pinned no-write rule still apply.

**Frontend:**
- On the Draft page: a format choice, "Short draft" (default) or "Binding ruling request (19 CFR 177)". Hide it if a cfr177 preview returns 400.
- Highlight the `[FILL IN: …]` placeholders in the rendered letter.
- Download and Print keep working, and still record only on those actions.

**Done when:** the heat-gun demo shows both formats in preview without writing to the pinned case.

## 5. Upload a new BOM version (D67) — medium

**Backend:** `POST /v1/cases/{id}/bom-version` takes a multipart file (CSV or xlsx, the same limits as import). It returns `{changes, old_origin, new_origin, new_confidence, origin_changed, triage}`. With `apply: true` it updates the case and records a `bom_updated` event; it's 403 on pinned demos and 404 on others' cases. It returns 404 when `BOM_VERSIONS` is off.

**Frontend:**
- On the Product step of a case created from a BOM: "Upload a new version".
- Show the change list (component added/removed/country changed/plant moved) and the origin before → after, with a clear "origin changes" banner when `origin_changed`.
- "Apply to this case" sends `apply: true`. Hide the apply action on pinned demos.

**Done when:** uploading `backend/testdata/heatgun_bom.csv` with the motor's country changed shows that one change and the new origin, and apply updates the case and its history.

## 6. API keys admin (D69) — optional, admin only

**Backend:** `POST/GET /v1/admin/api-keys` and `DELETE /v1/admin/api-keys/{id}` need the admin token, which the browser never has. Broker clients call the API directly with `X-API-Key`.

**Frontend:** nothing in the public UI. If anything, a short "API" page that documents `X-API-Key` usage and quotas from `openapi.yaml`, with no key management in the browser.

---

**Not frontend work:** calibration by family (D63, left off; negative result), the shared rate limiter (D70), and regression eval in CI (D72).
