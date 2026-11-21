# Import from an image or scanned document

Status: built. Tested offline with a fake model. Accuracy on real photos and scans: not yet measured.

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
- PDFs: pages are counted without rendering, including pages inside compressed object streams (decompression is capped at 4 MB). More than 10 pages across all PDFs, an encrypted PDF, or a PDF whose pages cannot be counted is refused.
- In the browser, a photo over 3 MB or over 4,096 pixels on its longest side is re-encoded as a JPEG with its longest side at 3,072 pixels before upload (`frontend/src/lib/media-upload.ts`).

## Data handling

Follows `docs/design/data-handling.md`. The files are read into memory (not `ParseMultipartForm`, which spills large files to disk), sent to the model in one call, and the handler zeroes its buffers when the request ends (the encoded copy the model client sends is left to the garbage collector). They are not stored, cached or logged. `llm.Message.Parts` is never serialised, so prompt snapshots and any JSON dump of a request leave the bytes out. The model call logs sizes and token counts only, as before. Tests check that no log line holds the file's bytes, its name or the parts read from it, and that the new route logs nothing the production policy masks.

## Model and prompt

The extractor uses `ENGINE_MODEL` (default `gemini-4-argon`), the same model and client as the text draft, at temperature 0 with the fixed seed and a JSON schema. The prompt `extract-image-v1` tells the model to transcribe what is printed, to leave a country empty when none is printed, to mark rows it is unsure of, and to treat text in the files as data, not instructions. It is fingerprinted in `backend/internal/promptsnap/snapshot.json`.

## How it was tested

- Backend unit tests with `llm.Fake`: type sniffing (an HTML file named `.png` is refused), size, file-count and page limits, a 60,000 x 60,000 PNG header refused, PDF page counting with and without object streams, mapping of the answer to the draft (file and page citations checked, unknown files dropped), the simulated label, and the no-content-logging checks (`backend/internal/media`, `backend/internal/extract/media_test.go`, `backend/internal/httpapi/media_test.go`).
- Frontend: `node:test` for routing a picked file, the size check and the downscale arithmetic (`frontend/scripts/media-upload.test.mjs`).
- By hand, in fake mode: uploading a PNG and an image-only PDF at 1280 and 390 pixels wide. Both land in Parts and plants with the simulated note.
- Five synthetic test images: `eval/media/render_boms.py` draws four committed BOM CSVs as tables, plus one blurred, rotated copy that stands in for a phone photo (`eval/media/images`). `go run ./cmd/mediacheck` (from `backend/`) runs them through the extractor and scores each expected part's name and country. With `LLM_PROVIDER=fake` this checks the wiring only: the rows are the fixed simulated example.

## Accuracy

Not yet measured. No live Gemini run was made for this change: there were no Vertex credentials where it was built. `LLM_PROVIDER=vertex go run ./cmd/mediacheck -out <file>` is the planned first check. Even then, five synthetic renderings would be an illustrative check, not a measure of accuracy on real photos, handwriting or poor scans. Until a measured figure exists, treat every drafted row as unverified, which the page already asks the user to do.
