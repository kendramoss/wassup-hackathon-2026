# Japan Customs advance rulings on origin: source and licence

## Attribution (PDL1.0 section 1.1)

出典：税関ホームページ「事前教示回答事例（原産地関係）」（https://www.customs.go.jp/searchao/jaosv001.jsp 及び各回答事例ページ https://www.customs.go.jp/tetsuzuki_search/origin/ ）、PDL1.0（https://www.digital.go.jp/resources/open_data/public_data_license_v1.0 ）（2026年11月22日に利用）

Source: Japan Customs website, "Advance rulings on origin" (事前教示回答事例（原産地関係）), search page and one page per ruling as above; licensed under the Public Data License 1.0 (PDL1.0); retrieved 22 Nov 2026.

## Edited by Unde Labs (PDL1.0 section 1.1, edited content)

「事前教示回答事例（原産地関係）」（税関）（https://www.customs.go.jp/searchao/jaosv001.jsp ）を加工して作成（Unde Labs）。

What Unde Labs did: parsed each ruling page into fields (`data/jp/rulings.jsonl`, Japanese text as published, whitespace trimmed), machine-translated the fields into English with a Gemini model (`data/jp/translations-en.jsonl`; a machine translation, not reviewed by Japan Customs or a licensed customs specialist), derived labels by fixed patterns (`data/jp/labels-*.jsonl`), and split the rulings for evaluation (`data/jp/splits.json`). None of this is an official Japan Customs product, and the edited data must not be presented as if Japan Customs made it. The Japanese ruling page is the authority; the English text is for search and reading only.

## Licence

- Japan Customs' terms of use (利用規約, https://www.customs.go.jp/kyotsu/rules.htm, read 22 Nov 2026): content on the site is copyright of the Ministry of Finance unless stated otherwise, and PDL1.0 applies unless a page says otherwise. The Japan Customs logos and the mascot are under separate rules; none is used here.
- PDL1.0 allows copying, distribution, translation and adaptation, including commercial use, with attribution and a notice of any editing. The Digital Agency states it is compatible with CC BY 4.0.
- PDL1.0 text as retrieved on 22 Nov 2026: `data/jp/LICENSE-PDL1.0.ja.txt` (Japanese; the official text). Section 1.6: the State accepts no responsibility for any use of the content, including edited content.
- Numerical data and simple tables are not protected by copyright (PDL1.0 section 1).

## What is in this directory

| File | What | Built by |
|---|---|---|
| `snapshot.json` | retrieval start and end (UTC), client name, search result count, and for every ruling its URL and the SHA-256 and size of the raw HTML page | `eval/jp/crawl.py`, `eval/jp/parse.py` |
| `rulings.jsonl` | one parsed ruling per line, Japanese as published | `eval/jp/parse.py` |
| `translations-en.jsonl` | English translation per ruling, with model, cost and latency | `eval/jp/translate.py` |
| `splits.json` | frozen development and held-out split (D710) | `eval/jp/split.py` |
| `labels-dev.jsonl` | criterion and stated origin for development rulings | `eval/jp/labels.py` |

The raw HTML (about 55 MB) is not committed; `snapshot.json` keeps its hashes. The site keeps roughly three years of rulings online, so later snapshots will differ.
