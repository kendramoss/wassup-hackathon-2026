"""Extract the Japan Customs treaty schedules used by the Japan engine.

Pass the directory containing the four PDFs named in SOURCES. The source
URLs and SHA-256 digests are checked and stored with the generated table.
Requires pdfplumber 0.11.10.
"""
import argparse
import hashlib
import json
from pathlib import Path
import re

import pdfplumber

BASE = "https://www.customs.go.jp/roo/english/text/"
SOURCES = (
    ("eu3.pdf", "JP-EU EPA", "Annex 3-B", "2017", "2019-02-01", ""),
    ("asean2-1.pdf", "AJCEP", "Annex 2", "2017", "2023-01-01", "2027-02-15"),
    ("tpp5.pdf", "CPTPP", "Annex 4-A", "2012", "2018-12-30", ""),
    ("tpp2.pdf", "CPTPP", "Annex 3-D", "2012", "2018-12-30", ""),
)
DIGESTS = {
    "eu3.pdf": "cb7f029ad6679a70796ba45f619e1a623d8e9304ed041069a4b007cdc9633704",
    "asean2-1.pdf": "00b7e42a64fd0fb7dad809ba3b3d487ecfd84ded5e103eea229fe973de3e127d",
    "tpp2.pdf": "c80acd1e0c458f130cc6d6602ce18192626e801623947fffc9d1ed18020873b9",
    "tpp5.pdf": "56eb27080e455e792131760cb5d0805556de8b73039a4b5f4a5b7485740785d1",
}
CODE = r"(?:\d{2}|\d{2}\.\d{2}|\d{4}\.\d{2})"
RANGE = re.compile(rf"({CODE})(?:\s*[-–]\s*({CODE}))?")
CHAPTER = re.compile(r"Chapter (\d+)\b", re.I)
SECTION = re.compile(r"Section [IVX]+\b", re.I)
NOTE = re.compile(r"(Section|Chapter|Heading|Subheading) Note\b", re.I)
# PDF footnote superscripts appear as regular digits. The relevant rules
# refer to a separate vehicle appendix or other legal text. Keep their
# ranges blocked instead of presenting the base rule on its own.
EU_FOOTNOTES = {
    "70.071 -70.09": "70.07-70.09",
    "84.07-84.081": "84.07-84.08",
    "87.01 -87.071": "87.01-87.07",
    "87.082": "87.08",
}
# Split merged cells. These four are headings / note continuations and
# contain no tariff line or independent product-specific requirement.
NOTE_CONTINUATIONS = {
    ("tpp2.pdf", 46, "point and the vapour"),
    ("tpp5.pdf", 10, "of the change in tariff classification"),
    ("tpp5.pdf", 13, "Women’s or girls’ kimono:"),
}
TITLE_CONTINUATIONS = {
    ("tpp2.pdf", 55, "WAXES, PREPARED WAXES"),
    ("tpp2.pdf", 72, "JEWELLERY; COIN"),
    ("tpp2.pdf", 95, "ELECTRICAL EQUIPMENT"),
}


def normalize(text):
    return " ".join((text or "").replace("\u00ad", "").replace("-\n", "-").split())


def join(text, addition):
    return " ".join(x for x in (text, addition) if x)


def code_range(key):
    match = RANGE.fullmatch(key)
    if not match:
        raise ValueError(f"unrecognised schedule classification {key!r}")
    start = match[1].replace(".", "")
    end = (match[2] or match[1]).replace(".", "")
    if len(start) > len(end):
        end += "9" * (len(start) - len(end))
    if len(end) > len(start):
        start += "0" * (len(end) - len(start))
    if start > end:
        raise ValueError(f"reversed schedule range {key!r}")
    return start, end


def schedule(pdf, filename):
    rows = []
    notes = []
    chapter = ""
    section = ""
    key = ""
    branches = []
    pending = None
    target = None
    merged = []

    def row(key, text, page, description="", blocked=False):
        start, end = code_range(key)
        item = {
            "start": start,
            "end": end,
            "page": page,
            "text": text,
        }
        if description:
            item["description"] = description
        if blocked:
            item["blocked"] = True
        rows.append(item)
        return item

    def note(scope, heading, text, page):
        item = {"scope": scope, "key": heading, "text": text, "page": page}
        notes.append(item)
        return item

    for page_number, page in enumerate(pdf.pages, 1):
        for table in page.extract_tables():
            for row_number, raw in enumerate(table):
                cells = [normalize(cell) for cell in raw]
                if filename == "asean2-1.pdf":
                    ch, heading, sub, description, text = cells
                    code = sub or heading or ch
                    if code.startswith("Tariff item number"):
                        continue
                    if m := CHAPTER.match(ch):
                        code = f"{int(m[1]):02}"
                    if SECTION.match(ch):
                        target = None
                        merged = []
                        continue
                    # The rule cell can span several classification rows. A
                    # None cell is inside that merged cell; an empty string is
                    # a distinct, empty cell. These merged cells begin on
                    # the previous page, so the opening rule is not repeated.
                    clipped = page_number in (25, 35) and row_number == 0
                    if (not code or clipped) and text:
                        if target is None:
                            raise ValueError(f"{filename}:{page_number} stray rule {text!r}")
                        for item in merged:
                            item["text"] = join(item["text"], text)
                    if not code:
                        continue
                    if raw[4] is None or clipped:
                        if target is None:
                            raise ValueError(f"{filename}:{page_number} missing merged rule {cells!r}")
                        merged.append(row(code, target["text"], target["page"], description))
                    elif text:
                        target = row(code, text, page_number, description)
                        merged = [target]
                    else:
                        target = None
                        merged = []
                    continue

                classification, text = cells
                if classification.startswith(("Column 1 ", "HS Classification")):
                    continue
                if m := CHAPTER.match(classification):
                    chapter = f"{int(m[1]):02}"
                    key = chapter
                    branches = []
                    pending = None
                    target = None
                    continue
                if SECTION.match(classification):
                    section = classification.split()[1]
                    key = ""
                    branches = []
                    pending = None
                    target = None
                    continue
                if m := NOTE.match(classification):
                    scope = m[1].lower()
                    # A heading note often precedes the heading it governs.
                    # Keep it with its chapter and let its quoted HS scope
                    # govern. Section notes travel with that HS section.
                    target = note("section" if scope == "section" else "chapter",
                                  section if scope == "section" else chapter,
                                  join(classification, text), page_number)
                    continue
                if any(filename == f and page_number == p and classification.startswith(prefix)
                       for f, p, prefix in NOTE_CONTINUATIONS):
                    if target is None or "scope" not in target:
                        raise ValueError(f"{filename}:{page_number} missing note before continuation")
                    target["text"] = join(target["text"], join(classification, text))
                    continue
                if any(filename == f and page_number == p and classification.startswith(prefix)
                       for f, p, prefix in TITLE_CONTINUATIONS):
                    continue
                if not classification:
                    if text:
                        if target is None:
                            raise ValueError(f"{filename}:{page_number} stray rule {text!r}")
                        target["text"] = join(target["text"], text)
                    continue
                if filename == "eu3.pdf" and classification.startswith("-"):
                    pieces = re.split(r" (?=-+ )", classification)
                    for piece in pieces:
                        depth = len(piece) - len(piece.lstrip("-"))
                        branches = branches[:depth - 1] + [piece[depth:].strip()]
                    if not key or not text:
                        raise ValueError(f"{filename}:{page_number} missing parent or rule {cells!r}")
                    target = row(key, text, page_number, " / ".join(branches))
                    pending = None
                    continue
                code = EU_FOOTNOTES.get(classification, classification).replace("†", "")
                blocked = code != classification or "appendix" in text.lower() and "short supply" in text.lower()
                if not RANGE.fullmatch(code):
                    raise ValueError(f"{filename}:{page_number} stray row {cells!r}")
                if pending:
                    raise ValueError(f"{filename}:{page_number} row {pending!r} has no rule")
                key = code
                branches = []
                if text:
                    target = row(code, text, page_number, blocked=blocked)
                    pending = None
                else:
                    pending = classification
                    target = None
        if filename == "asean2-1.pdf":
            # These notes occur below the last table and on the next page.
            if page_number == 49:
                text = page.extract_text().split("Notes to Section XI (chapters 50-63):", 1)[1]
                target = note("section", "XI", normalize(text), page_number)
            elif page_number == 50:
                target["text"] = join(target["text"], normalize(page.extract_text()))
        page.close()

    if pending:
        raise ValueError(f"{filename}: missing rule for {pending}")
    return rows, notes


def main():
    args = argparse.ArgumentParser(description=__doc__)
    args.add_argument("pdf_directory", type=Path)
    args.add_argument("destination", type=Path)
    args = args.parse_args()
    sources = []
    for filename, agreement, annex, hs, start, end in SOURCES:
        path = args.pdf_directory / filename
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        if digest != DIGESTS[filename]:
            raise ValueError(f"{filename}: source digest changed ({digest})")
        with pdfplumber.open(path) as pdf:
            rows, notes = schedule(pdf, filename)
        sources.append({
            "agreement": agreement,
            "citation": f"{agreement} {annex} (HS {hs})",
            "source_url": BASE + filename,
            "sha256": digest,
            "from": start,
            "until": end,
            "rows": rows,
            "notes": notes,
        })
    args.destination.write_text(
        json.dumps(sources, ensure_ascii=False, indent=1) + "\n",
        encoding="utf-8",
    )


if __name__ == "__main__":
    main()
