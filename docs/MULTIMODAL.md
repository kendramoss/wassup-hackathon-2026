# Import from an image or scanned document

Status: built. Measured on 20 synthetic BOM images through an OpenAI-compatible LiteLLM gateway to Gemini (illustrative, n small; shown on the Accuracy page under "Import from a photo or scan"). Gateway and Vertex AI agree on the development set (same engine and model, exact McNemar p = 0.58, D151); these runs were not repeated on Vertex AI. Accuracy on real photos, scans and handwriting: not yet measured.

## What it does

On the product step, the Import BOM box now also takes a photo or screenshot of a bill of materials, a scanned BOM or spec-sheet PDF, or a product photo with its spec sheet. Gemini reads the files once and returns the same draft as "Paste a description": parts, countries, operations, plants and warnings. The draft opens in Parts and plants with a note that it was read from an image and must be checked. Nothing is predicted until the user continues from there. There is no path from an image straight to a prediction.

A PDF is first read in the browser as before (`frontend/src/lib/pdf-bom.ts`). If it has no table with name and country columns, which is the case for a scan, it is sent to the model instead of failing. On a phone, a "Take a photo" button opens the camera.

## API

`POST /v1/products/extract-media`, `multipart/form-data`:

- `file`, one to four times: PNG, JPEG, WebP or PDF.
- `family` (optional): the product family, for operation groups.
- `name` (optional): the product name; otherwise the printed name is used.

The answer is the `/v1/products/extract` draft plus:

- `source: "media"` and `files`: each file's sniffed type, size, and pixel size or page count. Never content or file names.
- per row `source_file`, `source_page` (1-based) and `unclear` (the model was not sure it read the row). `source_paragraph` is 0.
- `unreadable`: places the model could not read, with file, page and what. Each is also a plain-words line in `warnings`.
- `simulated: true` when the local fake model answered.
- `trace.prompt_version`: `extract-image-v1`.

It is a separate endpoint from `/v1/products/extract` (JSON text) so each has one content type, one size limit and one schema. An MCP tool can wrap it as is. Full schema: `docs/api/openapi.yaml`.

Errors: 400 for a damaged, empty, encrypted or oversized-in-pixels file, too many files or pages, or an unknown family; 413 when the files are too large together; 415 for a type that is not accepted or a body that is not multipart. Messages name a file by position only.

## Limits and checks

All in `backend/internal/media/media.go`:

- 1 to 4 files, 8 MB together. The web proxy allows 9 MB per request (`frontend/src/lib/backend.ts`), so the backend limit is the one users meet.
- The type comes from the file's first bytes. The file name and declared type are ignored.
- Images: the size is read from the header without decoding pixels. More than 12,000 pixels a side or 40 megapixels is refused, which stops decompression bombs before any large allocation.
- PDFs: pages are counted from the document's page tree (the trailer's `/Root` → `/Pages` → `/Kids`, using the latest revision of each object and decoding `#xx` name escapes), without rendering. Pages in compressed object streams are resolved with a 4 MB decompression cap. More than 10 pages across all PDFs, an encrypted PDF, or a PDF whose page tree cannot be resolved is refused; there is no raw-text fallback.
- In the browser, a photo over 3 MB or over 4,096 pixels on its longest side is re-encoded as a JPEG with its longest side at 3,072 pixels before upload (`frontend/src/lib/media-upload.ts`).

## Data handling

Follows `docs/design/data-handling.md`. The files are read into memory (not `ParseMultipartForm`, which spills large files to disk), sent to the model in one call, and the handler zeroes its buffers when the request ends (the encoded copy the model client sends is left to the garbage collector). They are not stored, cached or logged. `llm.Message.Parts` is never serialised, so prompt snapshots and any JSON dump of a request leave the bytes out. The model call logs sizes and token counts only, as before. Tests check that no log line holds the file's bytes, its name or the parts read from it, and that the new route logs nothing the production policy masks.

## Model and prompt

The extractor uses `ENGINE_MODEL` (default `gemini-4-argon`), the same model and client as the text draft, at temperature 0 with the fixed seed and a JSON schema. The prompt `extract-image-v1` tells the model to transcribe what is printed, to leave a country empty when none is printed, to mark rows it is unsure of, and to treat text in the files as data, not instructions. It is fingerprinted in `backend/internal/promptsnap/snapshot.json`.

## How it was tested

- Backend unit tests with `llm.Fake`: type sniffing (an HTML file named `.png` is refused), size, file-count and page limits, a 60,000 x 60,000 PNG header refused, PDF page counting with and without object streams, mapping of the answer to the draft (file and page citations checked, unknown files dropped), the simulated label, and the no-content-logging checks (`backend/internal/media`, `backend/internal/extract/media_test.go`, `backend/internal/httpapi/media_test.go`).
- Frontend: `node:test` for routing a picked file, the size check and the downscale arithmetic (`frontend/scripts/media-upload.test.mjs`).
- By hand, in fake mode: uploading a PNG and an image-only PDF at 1280 and 390 pixels wide. Both land in Parts and plants with the simulated note.
- Five synthetic test images (set 1): `eval/media/render_boms.py` draws four committed BOM CSVs as tables, plus one blurred, rotated copy that stands in for a phone photo (`eval/media/images`).
- Fifteen more (set 2, D168): `eval/media/render_varied.py` draws part and country pairs already in the repo (the four BOM CSVs and the part lists of three demo fixtures: gas grill, 2026 heat gun, LED tub light) in other layouts, fonts and captures: bulleted spec lists, a grid table with the country first plus part numbers and quantities, shuffled rows, 11 px small print, a page turned sideways, three phone-photo style JPEGs (rotated 4 to 9 degrees, shaded, blurred), and three low-resolution grey scans with speckle (`eval/media/images-varied`). Long names wrap instead of being cut, and plant rows are not counted as parts.
- `go run ./cmd/mediacheck` (from `backend/`) runs a folder through the extractor and scores each expected part: was its name read (exact text, ignoring case and spacing), and its country. With `LLM_PROVIDER=fake` this checks the wiring only: the rows are the fixed simulated example. With `LLM_PROVIDER=litellm` it calls the gateway (offline only; the server refuses that provider, D167).

## Accuracy

Illustrative. Synthetic renderings of illustrative BOMs, one run each, Gemini 4 Argon with the production prompt `extract-image-v1`, through an OpenAI-compatible LiteLLM gateway to Gemini, 21 Nov 2026 (D167, D168). Not Vertex AI, not real documents. Parts inside one image are not independent, so no interval is given.

| Set | Images | Parts expected | Names read (exact) | Countries right | Images with every part | Extra rows | Gateway cost |
|---|---:|---:|---|---|---:|---:|---:|
| 1, plain tables (`eval/media/images`) | 5 | 48 | 45 (93.8%) | 45 (93.8%) | 4 | 1 | $0.079 |
| 2, varied (`eval/media/images-varied`) | 15 | 122 | 106 (86.9%) | 106 (86.9%) | 11 | 34 | $0.173 |
| Both | 20 | 170 | 151 (88.8%) | 151 (88.8%) | 15 | 35 | $0.252 |

Rows: `eval/results/media-v1-gemini-4-argon-litellm.json`, `eval/results/media-varied-gemini-4-argon-litellm.json`; each run is a line in `eval/results/RUNLOG.jsonl`.

What the misses are. Every part whose name was matched had the right country (151 of 151). No call failed. The 19 misses are all names that did not match the expected text exactly; the rows the model returned for them are in the two `*-diagnostic-*.json` files (the same images run again to keep the rows; the gateway answered from its cache, so the answers are the same and cost nothing):

- 13: the grid table with part numbers (`supplier-reordered.png`). The model wrote each name with its part number, such as "main housing (HG-1001)". Every country was right. The phone photo of the same table scored 13 of 13.
- 3: one row that lists several parts in one cell ("lenses, laser emitters and detectors, ... and all other components", China). In `lidar-serif.png` and `lidar-photo.jpg` the model split it into 9 or 10 rows, each China. In set 1 (`lidar.png`) the image itself cut the name short with "…", so the full text was not on the page.
- 1: "LED printed circuit board assembly (PCBA), populated with a South Korean LED" (Thailand) was read as two rows: the PCBA, Thailand, and the LED, South Korea. Both match what the page says.
- 2: set 1's manifest counts the two plant rows of the LiDAR BOM ("Vietnam SMT plant", "China suppliers") as parts. The extractor puts plants in a separate list, so these can never match. Set 2 leaves plant rows out.

So on these images the misses come from how the check matches names, plus one cut-off name and two plant rows in the test data, not from misread text. The low-resolution scans (about 380 to 450 pixels wide), the 11 px print, the sideways page and the photos were read in full. That says the model reads clean synthetic print well. It says nothing about real phone photos, handwriting, stamps, multi-page supplier PDFs or poor real scans, which are not measured. Treat every drafted row as unverified, which the page already asks the user to do.

Not changed because of this check: the prompt, the matching rule and the expected rows. The diagnostic re-runs are not scores.
