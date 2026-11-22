# Third-party notices

Unde Labs bundles the following third-party assets and libraries. Their licence
texts are reproduced or linked below. The project's own source is under the
[MIT License](LICENSE); this file does not change the licences of the components
listed here.

## Bundled assets

### Paper Mono (font)

- Files: `frontend/src/app/fonts/PaperMono*.woff2`, `docs/design/keyframes/_shared/fonts/`
- Copyright 2025 The Paper-Mono.Git Project Authors (<https://github.com/paper-design/paper-mono.git>)
- Licence: **SIL Open Font License 1.1**
- Full text: `frontend/src/app/fonts/PaperMono-OFL.txt` and
  `docs/design/keyframes/_shared/fonts/PaperMono-OFL.txt`

### flag-icons (flag SVGs)

- Files: copied into `frontend/public/` by `frontend/scripts/copy-flags.mjs`
- Licence: **MIT** (`flag-icons` npm package, <https://github.com/lipis/flag-icons>)

### pdfjs-dist (PDF rendering in the browser)

- Package: `pdfjs-dist` (Apache-2.0), <https://github.com/mozilla/pdf.js>

### Japan Customs advance rulings on origin (data)

- Files: `data/jp/` (parsed rulings, English machine translations, splits, labels)
- Source: Japan Customs website, 事前教示回答事例（原産地関係）, <https://www.customs.go.jp/searchao/jaosv001.jsp>, retrieved 22 Nov 2026
- Licence: **Public Data License 1.0 (PDL1.0)**, compatible with CC BY 4.0; edited by Unde Labs (parsed, translated, labelled). Attribution and the edit notice: `data/jp/LICENSE-SOURCE.md`; licence text: `data/jp/LICENSE-PDL1.0.ja.txt`
- Not used in the deployed service yet (eval data only)

## Bundled libraries (npm)

The frontend declares these dependencies in `frontend/package.json`; they are
installed from npm and included in the built client bundle. All are used under
their respective licences:

| Package | Licence |
|---|---|
| next | MIT |
| react, react-dom | MIT |
| react-markdown | MIT |
| three | MIT |
| flag-icons | MIT |
| pdfjs-dist | Apache-2.0 |

Development-only tooling (`eslint`, `tailwindcss`, `openapi-typescript`) is MIT
licensed and is not shipped in the client bundle.

## Data

CBP rulings are works of the U.S. government and are in the **public domain**
in the United States (17 U.S.C. § 105). Source: <https://rulings.cbp.gov>.
Raw ruling texts are fetched, not redistributed. The labels, splits and facts
files derived from them are in this repository; see [docs/DATASHEET.md](docs/DATASHEET.md).

The RCEP rule table (`backend/internal/rcep/data/psr_hs2022.json`) is parsed
from the RCEP Agreement's Annex 3A product-specific rules in HS 2022, as
published by the RCEP Secretariat (<https://rcepsec.org/rules-of-origin/>)
and cross-checked against the Australian Border Force copy. It is the text of
a public international agreement; neither PDF states a reuse licence. The
PDFs are not redistributed; their URLs, retrieval date and SHA-256 are in
`eval/rcep/source.json`.

Canada's marking rules (`backend/internal/tariffshift/rules_ca.json`) are the
Schedule III rows of the Determination of Country of Origin for the Purpose of
Marking Goods (CUSMA Countries) Regulations, SOR/94-23, from the Justice Laws
website (<https://laws-lois.justice.gc.ca/eng/XML/SOR-94-23.xml>, retrieved 22
Nov 2026). Reproduced under the Reproduction of Federal Law Order (SI/97-5):
anyone may reproduce federal enactments without charge or permission, if the
copy is accurate and not presented as the official version. This copy is not
the official version. Headings are written 8473 for 84.73; two slips are
corrected (D720).

## Go dependencies

Backend dependencies are declared in `backend/go.mod` and fetched through the
Go module proxy at build time. Their individual licences are available in the
module cache (`go mod download -json` gives each module's directory) and are
not reproduced here.
