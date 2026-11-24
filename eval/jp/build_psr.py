"""Extract the Japan Customs treaty schedules used by the Japan engine.

Pass the directory containing the source PDFs named in SOURCES. The source
URLs and SHA-256 digests are checked and stored with the generated table.
Requires pdfplumber 0.11.10.
"""
import argparse
import hashlib
import json
from collections import Counter
from pathlib import Path
import re

import pdfplumber

BASE = "https://www.customs.go.jp/roo/english/text/"
SOURCES = (
    ("eu3.pdf", "JP-EU EPA", "Annex 3-B", "2017", "2019-02-01", ""),
    ("asean2-1.pdf", "AJCEP", "Annex 2", "2017", "2023-01-01", "2027-02-15"),
    ("tpp5.pdf", "CPTPP", "Annex 4-A", "2012", "2018-12-30", ""),
    ("tpp2.pdf", "CPTPP", "Annex 3-D", "2012", "2018-12-30", ""),
    ("asean2-2.pdf", "AJCEP", "Annex 2", "2022", "2027-02-15", ""),
    ("india2.pdf", "JP-India CEPA", "Annex 2", "2007", "2011-08-01", ""),
    ("thailand2-2.pdf", "JP-Thailand EPA", "Annex 2", "2017", "2023-01-01", ""),
    ("indonesia2-2.pdf", "JP-Indonesia EPA", "Annex 2", "2017", "2024-02-05", ""),
    ("australia2.pdf", "JP-Australia EPA", "Annex 2", "2012", "2015-01-15", ""),
    ("uk3.pdf", "JP-UK CEPA", "Annex 3-B", "2017", "2021-01-01", ""),
    ("vietnam2.pdf", "JP-Vietnam EPA", "Annex 2", "2007", "2009-10-01", ""),
    ("malaysia2.pdf", "JP-Malaysia EPA", "Annex 2", "2002", "2006-07-13", ""),
    ("mexico2.pdf", "JP-Mexico EPA", "Annex 4", "2002", "2005-04-01", ""),
    ("philippine2.pdf", "JP-Philippines EPA", "Annex 2", "2002", "2008-12-11", ""),
    ("switzerland3.pdf", "JP-Switzerland EPA", "Appendix 1 to Annex II", "2007", "2009-09-01", ""),
    ("chile2.pdf", "JP-Chile EPA", "Annex 2", "2002", "2007-09-03", ""),
)
DIGESTS = {
    "eu3.pdf": "cb7f029ad6679a70796ba45f619e1a623d8e9304ed041069a4b007cdc9633704",
    "asean2-1.pdf": "00b7e42a64fd0fb7dad809ba3b3d487ecfd84ded5e103eea229fe973de3e127d",
    "tpp2.pdf": "c80acd1e0c458f130cc6d6602ce18192626e801623947fffc9d1ed18020873b9",
    "tpp5.pdf": "56eb27080e455e792131760cb5d0805556de8b73039a4b5f4a5b7485740785d1",
    "asean2-2.pdf": "4ec0fffea58f4d261d1388ae7284dce8a3e7bef891148144fd7c9c53d32d2cb0",
    "india2.pdf": "4011ed6be153ec99ce9b199bb314c0f0fa69204677f7150bef649bf55a0b39e9",
    "thailand2-2.pdf": "f87a52b564a4e6b7c18207c6a0cf6c6cbb244f81f1e6c79204f03f3437db9fbc",
    "indonesia2-2.pdf": "b82294d088899ad39d6047320a2e88a4af30d0a6efb4ca2283443ca84464f151",
    "australia2.pdf": "9699b69dbc784d76a7461735d5944b967642cbbca504b17f79cc3669caa7f768",
    "uk3.pdf": "7caabd2e36f02f6794cff41501ffa4ba339a6603b4c5c462bc4bedc7e371f9eb",
    "vietnam2.pdf": "12e578de5573c093379d02813ca5215eefaa3cebf8e837bacfdbe1f38d019a4f",
    "malaysia2.pdf": "10ad097b32caa1de4a2d04a6ecfa67ae118a32da85e135ba3e797402d3e329d3",
    "mexico2.pdf": "cd746343339b4f1ee9ca892b3874cf68c34266b8c4686a493042b43caac952f0",
    "philippine2.pdf": "5ec10d077976947d214a723e8e50827c93aefe465e0199f26916b9327b08ca2e",
    "switzerland3.pdf": "ba550092a9ad93cba330b298a1e88064d060a324e271e944a1435b2503e807f5",
    "chile2.pdf": "f8078cebd61348669aa4f7b380b93d30832d4bd2d58091823b073456788d2acc",
}
URLS = {"indonesia2-2.pdf": "https://www.customs.go.jp/roo/text/indonesia2-2.pdf"}
TEXT_PDFS = {
    "chile2.pdf", "india2.pdf", "malaysia2.pdf", "mexico2.pdf", "philippine2.pdf",
}
FIVE_COLUMN_PDFS = {"asean2-2.pdf", "vietnam2.pdf"}
SIX_COLUMN_PDFS = {"indonesia2-2.pdf", "thailand2-2.pdf"}

CODE = r"(?:\d{4}\.\d{2}|\d{2}\.\d{2}|\d{2})"
RANGE = re.compile(rf"({CODE})(?:\s*[-–—~]\s*({CODE}))?")
RANGE_IN_TEXT = re.compile(rf"({CODE})(?:\s*[-–—~]\s*({CODE}))?")
NUMBERED_NOTE = re.compile(r"Notes? \d+:")
# Where a numbered scope note applies, read from the note's own wording in the
# PDF (page given). A numbered note without an entry here fails the build
# rather than being attached to a whole chapter or section.
def _headings(*pieces):
    out = []
    for piece in pieces:
        start, _, end = piece.partition("-")
        out.append([start, end or start])
    return out


_TEXTILE_1 = _headings("50-55", "60")  # "Chapter 50 through 55 and 60"
NOTE_APPLIES = {
    ("malaysia2.pdf", 35, "Note 1:"): _TEXTILE_1,
    ("malaysia2.pdf", 36, "Note 2:"): _headings(
        "5007", "5106-5113", "5204-5212", "5309-5311", "5407-5408", "5508-5516",
        "5604-5609", "5701-5705", "5801-5811", "5902", "5910", "6001-6006",
        "6101-6117", "6201-6217", "6301-6310"),
    ("philippine2.pdf", 33, "Note 1:"): _TEXTILE_1,
    ("philippine2.pdf", 34, "Note 2:"): _headings(
        "5007", "5111-5113", "5204-5212", "5309-5311", "5407-5408", "5508-5516",
        "5604-5609", "5701-5705", "5801-5811", "5902-5911", "6001-6006",
        "6101-6117", "6201-6217", "6301-6308"),
    ("mexico2.pdf", 65, "Note 1:"): _headings("847149"),
    ("mexico2.pdf", 65, "Note 2:"): _headings("847330"),
}
# Six-column schedules whose code-less "Note:" cells are captured as scope
# notes (Indonesia has the same layout but is not regenerated here). Every
# note of these files needs an entry keyed (file, page, label, heading key)
# so the scope is a reviewed reading of the note's own wording; an empty list
# means the whole chapter or section named by the heading the note sits under.
SCOPE_NOTE_PDFS = {"thailand2-2.pdf"}
SCOPE_NOTE_START = re.compile(r"Note(?: \d+)?:")
_T = "thailand2-2.pdf"
NOTE_APPLIES.update({
    # "subheadings 0306.16, 0306.17, 0306.35, 0306.36 and 0306.95"
    (_T, 4, "Note:", "03"): _headings("030616", "030617", "030635", "030636", "030695"),
    (_T, 5, "Note:", "07"): _headings("071022", "071029"),  # "0710.22 and 0710.29"
    (_T, 6, "Note 1:", "16"): _headings("160414"),
    (_T, 6, "Note 2:", "16"): _headings("160413", "160415", "160420", "160521", "160529"),
    # "subheading 1803.20 and heading 18.05"
    (_T, 7, "Note:", "18"): _headings("180320", "1805"),
    (_T, 8, "Note:", "19"): _headings("190590"),
    # "2001.10 through 2005.60, 2005.80 through 2007.10, 2008.11, 2008.19,
    # 2008.40 through 2008.91, 2009.61, 2009.69, 2009.81 and 2009.89"
    (_T, 9, "Note:", "20"): _headings(
        "200110-200560", "200580-200710", "200811", "200819", "200840-200891",
        "200961", "200969", "200981", "200989"),
    (_T, 12, "Note:", "27"): [],  # "this chapter"
    (_T, 12, "Note:", "VI"): _headings("28-38"),  # "chapters 28 through 38"
    (_T, 20, "Note:", "VII"): _headings("39-40"),  # "chapters 39 and 40"
    (_T, 23, "Note:", "XI"): _TEXTILE_1,  # "chapters 50 through 55 and 60"
    (_T, 26, "Note 1:", "61"): [],  # "a good of this chapter"
    (_T, 27, "Note 2:", "61"): [],  # "this chapter"
    (_T, 27, "Note 1:", "62"): [],
    (_T, 27, "Note 2:", "62"): [],
    (_T, 28, "Note:", "63"): [],
    (_T, 29, "Note:", "71"): [],
    (_T, 29, "Note:", "72"): [],
    (_T, 30, "Note:", "73"): [],
    (_T, 35, "Note:", "87"): _headings("8702-8704"),  # "headings 87.02 through 87.04"
})
COLUMN_HEADER = re.compile(r"^(?:\(HS ?\d{4}\)|HS Classification|Product Specific Rule)", re.I)
# A rule that ends like this is cut short (end of a page-broken cell).
DANGLING = re.compile(r"(?:^|[\s,])(?:through|to|from|or|and|than|of|the|any)$|[,-]$", re.I)
CHAPTER = re.compile(r"Chapter (\d+)\b", re.I)
SECTION = re.compile(r"Section ([IVX]+)\b", re.I)
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
# A footnote that names only some subheadings of a range limits the block to
# those subheadings. The rest of the range keeps its rule and carries the
# footnote as a page-bearing note. Footnote text is quoted from eu3.pdf page 55.
EU_NARROW = {
    "70.071 -70.09": {
        "blocked": ("7007.11", "7007.21"),
        "note": ("70", "For the products of subheadings 7007.11 and 7007.21, see also Appendix 3-B-1.",
                 [["7007", "7009"]]),
    },
}
# Japan-UK: the PDF glues the footnote digit to the code ("87.011"), which the
# generic reader does not recognise, so these rows were dropped. Each entry
# lists (code range, withheld). The footnotes say "see also Appendix 3-B-1"
# for 70.07 (7007.11 and 7007.21 only), 84.07-84.08 and 87.01-87.08; every
# such range is withheld here, the rest of 87.08-87.11 is a normal row.
UK_FOOTNOTES = {
    "70.071 -70.09": (("70.07-70.09", True),),
    "84.07-84.081": (("84.07-84.08", True),),
    "87.011": (("87.01", True),),
    "87.02-87.052": (("87.02-87.05", True),),
    "87.063": (("87.06", True),),
    "87.074": (("87.07", True),),
    "87.085-87.11": (("87.08", True), ("87.09-87.11", False)),
}
# Japan-Switzerland: a cell that lists several codes is split into its source
# lines when the table pins them. Column (4) is empty in these lines, so the
# rule is column (3) alone. Keys are the code cell, values (code, rule) lines.
SWISS_SPLITS = {
    "35.01 3502.11 -3502.19 3502.20 -35.04 35.05": (
        ("35.01", "CC"),
        ("3502.11-3502.19", "CC except from chapter 4."),
        ("3502.20-35.04", "CTH"),
        ("35.05", "CC except from chapter 11."),
    ),
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
END_MARKERS = (
    "part 3", "section 3", "minimum data requirement", "operational procedures",
    "attachment b", "proofs of origin", "implementing procedures",
    # India: a process-requirement table for textiles, not product rules.
    "appendix to annex 2",
)


def normalize(text):
    return " ".join((text or "").replace("\u00ad", "").replace("\u2011", "-").replace("-\n", "-").split())


def join(text, addition):
    return " ".join(x for x in (text, addition) if x)


def clean_code(key):
    return (key or "").replace("\u00a0", " ").replace("\u2013", "-").replace("\u2014", "-").replace("~", "-")


def code_range(key):
    match = RANGE.fullmatch(clean_code(key).strip())
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


def add_row(rows, key, page, text, description="", blocked=False):
    start, end = code_range(key)
    item = {"start": start, "end": end, "page": page, "text": text}
    if description:
        item["description"] = description
    if blocked:
        item["blocked"] = True
    rows.append(item)
    return item


def add_note(notes, scope, heading, text, page):
    item = {"scope": scope, "key": heading, "text": text, "page": page}
    notes.append(item)
    return item


def schedule(pdf, filename):
    """Parser for the four original, narrow-table PDFs."""
    rows = []
    notes = []
    chapter = ""
    section = ""
    key = ""
    branches = []
    pending = None
    target = None
    merged = []

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
                        merged.append(add_row(rows, code, target["page"], target["text"], description))
                    elif text:
                        target = add_row(rows, code, page_number, text, description)
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
                if m := SECTION.match(classification):
                    section = m[1]
                    key = ""
                    branches = []
                    pending = None
                    target = None
                    continue
                if m := NOTE.match(classification):
                    scope = m[1].lower()
                    target = add_note(notes, "section" if scope == "section" else "chapter",
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
                    target = add_row(rows, key, page_number, text, " / ".join(branches))
                    pending = None
                    continue
                if filename == "eu3.pdf" and classification in EU_NARROW and text:
                    narrow = EU_NARROW[classification]
                    for blocked_code in narrow["blocked"]:
                        add_row(rows, blocked_code, page_number, text, blocked=True)
                    key = EU_FOOTNOTES[classification]
                    target = add_row(rows, key, page_number, text)
                    scope, note_text, applies = narrow["note"]
                    add_note(notes, "chapter", scope, note_text, page_number)["applies"] = applies
                    branches = []
                    continue
                code = EU_FOOTNOTES.get(classification, classification).replace("†", "")
                blocked = code != classification or "appendix" in text.lower() and "short supply" in text.lower()
                if not RANGE.fullmatch(clean_code(code)):
                    raise ValueError(f"{filename}:{page_number} stray row {cells!r}")
                if pending:
                    raise ValueError(f"{filename}:{page_number} row {pending!r} has no rule")
                key = code
                branches = []
                if text:
                    target = add_row(rows, code, page_number, text, blocked=blocked)
                    pending = None
                else:
                    pending = classification
                    target = None
        if filename == "asean2-1.pdf":
            if page_number == 49:
                text = page.extract_text().split("Notes to Section XI (chapters 50-63):", 1)[1]
                target = add_note(notes, "section", "XI", normalize(text), page_number)
            elif page_number == 50:
                target["text"] = join(target["text"], normalize(page.extract_text()))
        page.close()

    if pending:
        raise ValueError(f"{filename}: missing rule for {pending}")
    return rows, notes


def five_column(pdf, filename):
    """Parser for the HS 2022 AJCEP and Vietnam five-column tables."""
    rows = []
    notes = []
    target = None
    group = []
    for page_number, page in enumerate(pdf.pages, 1):
        first = True
        for table in page.extract_tables():
            for raw in table:
                cells = [normalize(cell) for cell in raw]
                if len(cells) < 5:
                    continue
                rule_missing = raw[4] is None
                if len(cells) > 5:
                    # Pages with extra empty columns: codes first, rule last,
                    # description in between.
                    rule_missing = raw[-1] is None
                    cells = [*cells[:3], " ".join(c for c in cells[3:-1] if c), cells[-1]]
                ch, heading, sub, description, text = cells[:5]
                code = sub or heading or ch
                if code.startswith(("Tariff item number", "Tariff Item Number")):
                    continue
                leading, first = first, False
                if m := CHAPTER.match(ch):
                    code = f"{int(m[1]):02}"
                if SECTION.match(ch):
                    target = None
                    group = []
                    continue
                if description.startswith("Note:") or code.startswith("Note:"):
                    note = join(code, join(description, text))
                    chapter_match = CHAPTER.match(ch)
                    section_match = SECTION.match(ch)
                    section = section_match[1] if section_match else ""
                    chapter = f"{int(chapter_match[1]):02}" if chapter_match else ""
                    key = section or chapter
                    if key:
                        target = add_note(notes, "section" if section else "chapter", key, note, page_number)
                        group = [target]
                    continue
                if not code:
                    if text and target is not None:
                        # A merged cell continues under every row that shares it.
                        for item in group:
                            item["text"] = join(item["text"], text)
                    continue
                if not RANGE.fullmatch(clean_code(code)):
                    continue
                if not text and rule_missing and target is not None:
                    target = add_row(rows, code, target["page"], target["text"], description)
                    group.append(target)
                elif text:
                    if leading and group and (DANGLING.search(group[0]["text"]) or text[0].islower()):
                        # A rule cell merged across a page break continues in
                        # the first row of the next page; every row sharing
                        # the cell carries the whole rule.
                        text = join(group[0]["text"], text)
                        for item in group:
                            item["text"] = text
                        target = add_row(rows, code, group[0]["page"], text, description)
                        group.append(target)
                    else:
                        target = add_row(rows, code, page_number, text, description)
                        group = [target]
                else:
                    target = None
                    group = []
        page.close()
    return rows, notes


def australia(pdf):
    """Parser for the Japan-Australia four-column table."""
    rows = []
    notes = []
    target = None
    for page_number, page in enumerate(pdf.pages, 1):
        for table in page.extract_tables():
            for raw in table:
                cells = [normalize(cell) for cell in raw]
                if len(cells) < 3:
                    continue
                code, description, text = cells[:3]
                if code.startswith(("Tariff Item", "Tariff item")) or code in ("1", "2", "3", "4"):
                    continue
                if m := CHAPTER.match(code):
                    key = f"{int(m[1]):02}"
                    if text:
                        target = add_row(rows, key, page_number, text)
                    else:
                        target = None
                    continue
                if SECTION.match(code):
                    target = None
                    continue
                if description.startswith("Note:") or code.startswith("Note:"):
                    continue
                if not RANGE.fullmatch(clean_code(code)):
                    if text and target is not None:
                        target["text"] = join(target["text"], text)
                    continue
                if text:
                    target = add_row(rows, code, page_number, text, description)
                elif raw[2] is None and target is not None:
                    target = add_row(rows, code, target["page"], target["text"], description)
                else:
                    target = None
        page.close()
    return rows, notes


def six_column(pdf, filename):
    """Parser for the HS 2017 Thailand and Indonesia tables."""
    rows = []
    notes = []
    target = None
    group = []
    chapter = ""
    section = ""
    in_note = False
    scope_notes = filename in SCOPE_NOTE_PDFS
    heading = ""  # "chapter" or "section": the last heading seen
    scope_note = None
    for page_number, page in enumerate(pdf.pages, 1):
        for table in page.extract_tables():
            for raw in table:
                cells = [normalize(cell) for cell in raw]
                codes = []
                for i, cell in enumerate(cells):
                    if cell.startswith("Chapter") and (m := CHAPTER.match(cell)):
                        chapter = f"{int(m[1]):02}"
                        heading = "chapter"
                        target = None
                        group = []
                        scope_note = None
                    elif cell.startswith("Section") and (m := SECTION.match(cell)):
                        section = m[1]
                        heading = "section"
                        target = None
                        group = []
                        scope_note = None
                    elif cell and RANGE.fullmatch(clean_code(cell)):
                        codes.append((i, cell))
                if not codes:
                    # Later physical lines of a multi-line rule cell have only
                    # one cell filled. The column varies by page layout, so
                    # notes and column headers are excluded instead.
                    filled = [cell for cell in cells if cell]
                    if scope_notes and len(filled) == 1:
                        # A scope note sits under a chapter or section
                        # heading and runs until the next rule row or heading.
                        if SCOPE_NOTE_START.match(filled[0]):
                            scope_note = add_note(
                                notes, heading,
                                chapter if heading == "chapter" else section,
                                filled[0], page_number)
                            target = None
                            group = []
                            continue
                        if scope_note is not None and not COLUMN_HEADER.match(filled[0]):
                            scope_note["text"] = join(scope_note["text"], filled[0])
                            continue
                    if len(filled) == 1 and target is not None:
                        if filled[0].startswith("Note:"):
                            in_note = True
                            target = None
                            group = []
                        elif not in_note and not COLUMN_HEADER.match(filled[0]):
                            for item in group:
                                item["text"] = join(item["text"], filled[0])
                    continue
                in_note = False
                scope_note = None
                index, code = codes[0]
                rule = ""
                description = ""
                for cell in cells[index + 1:]:
                    if not cell or cell in {c for _, c in codes}:
                        continue
                    if RANGE.fullmatch(clean_code(cell)):
                        continue
                    rule = cell
                    break
                copied = not rule and target is not None
                if copied:
                    rule = target["text"]
                if rule.startswith("Note:"):
                    key = section or chapter
                    if key:
                        add_note(notes, "section" if section else "chapter", key, rule, page_number)
                    target = None
                    group = []
                    continue
                if rule:
                    target = add_row(rows, code, page_number, rule, description)
                    group = group + [target] if copied else [target]
        page.close()
    return rows, notes


def switzerland(pdf):
    """Parser for the Switzerland appendix; multi-code cells fail closed."""
    rows = []
    notes = []
    for page_number, page in enumerate(pdf.pages, 1):
        for table in page.extract_tables():
            for raw in table:
                cells = [normalize(cell) for cell in raw]
                if len(cells) < 3:
                    continue
                chapter_cell, codes, rules = cells[:3]
                if codes.startswith(("Tariff classi", "Tariff classification")):
                    continue
                found = RANGE_IN_TEXT.findall(clean_code(codes))
                if not found:
                    continue
                if codes in SWISS_SPLITS:
                    lines = SWISS_SPLITS[codes]
                    if " ".join(rule for _, rule in lines) != rules or len(cells) > 3 and cells[3]:
                        raise ValueError(f"{filename}:{page_number} split {codes!r} does not match {rules!r}")
                    for code, rule in lines:
                        add_row(rows, code, page_number, rule)
                    continue
                start, end = code_range(found[0][0] or found[0][1])
                blocked = len(found) > 1 or "except for" in codes.lower()
                if len(found) > 1:
                    last = code_range(found[-1][1] or found[-1][0])[1]
                    start, end = min(start, last), max(end, last)
                text = join(rules, cells[3] if len(cells) > 3 else "")
                item = {"start": start, "end": end, "page": page_number, "text": text}
                if blocked:
                    item["blocked"] = True
                rows.append(item)
        page.close()
    return rows, notes


def uk(pdf):
    """Parser for the two-column Japan-UK annex.

    A code row with an empty rule cell is a heading: its rules are the "-"
    lines that follow, stored one row per line, keyed to the heading's codes
    with the line's label as description. A heading never takes the previous
    row's rule (the cell is empty in the PDF, not merged). The build fails on
    a heading without branch lines, a branch line without a rule, and a
    branch line that follows neither a heading nor a chapter title.
    """
    rows = []
    notes = []
    chapter = ""
    target = None
    heading = None  # (code cell, page) of an open empty-cell heading
    heading_branches = 0
    chapter_open = False  # a chapter title with no code row after it yet

    def close_heading():
        nonlocal heading
        if heading is not None and not heading_branches:
            raise ValueError(f"uk3.pdf:{heading[1]} heading {heading[0]!r} has no rule lines")
        heading = None

    for page_number, page in enumerate(pdf.pages, 1):
        for table in page.extract_tables():
            for raw in table:
                cells = [normalize(cell) for cell in raw]
                if len(cells) < 2:
                    continue
                classification, text = cells[:2]
                if classification.startswith(("Column 1", "Harmonized System", "(1)")):
                    continue
                if SECTION.match(classification):
                    close_heading()
                    target = None
                    chapter_open = False
                    continue
                if m := CHAPTER.match(classification):
                    close_heading()
                    chapter = f"{int(m[1]):02}"
                    target = None
                    chapter_open = True
                    continue
                if classification.startswith("Note:") or text.startswith("Note:"):
                    if chapter:
                        add_note(notes, "chapter", chapter, join(classification, text), page_number)
                    continue
                if classification in UK_FOOTNOTES and text:
                    close_heading()
                    chapter_open = False
                    for code, blocked in UK_FOOTNOTES[classification]:
                        target = add_row(rows, code, page_number, text, blocked=blocked)
                    continue
                if RANGE.fullmatch(clean_code(classification)):
                    close_heading()
                    chapter_open = False
                    if text:
                        target = add_row(rows, classification, page_number, text)
                    else:
                        heading, heading_branches, target = (classification, page_number), 0, None
                    continue
                if classification.startswith("-"):
                    if not text:
                        raise ValueError(f"uk3.pdf:{page_number} rule line {classification!r} has no rule")
                    if heading is not None:
                        heading_branches += 1
                        key = heading[0]
                    elif chapter_open:
                        key = chapter
                    else:
                        raise ValueError(f"uk3.pdf:{page_number} rule line {classification!r} follows no heading")
                    target = add_row(rows, key, page_number, text, classification)
                    continue
                if text and target is not None:
                    target["text"] = join(target["text"], text)
        page.close()
    close_heading()
    if any(not row["text"] for row in rows):
        raise ValueError("uk3.pdf: a row has no rule text")
    return rows, notes


CODE_COLUMN_SLACK = 15


def code_column(pdf):
    """Left edge of the classification column of a text schedule.

    A wrapped line of rule prose can start with a code-like token ("3901.20
    from ...", "35 percent"); only a line that starts in the classification
    column opens a new row. The edge is the most common left edge of the
    code-like lines in the whole document. The second result tells whether
    the rules sit beside the codes (Chile, Malaysia, Mexico, Philippines);
    in the stacked layout (India) rule text starts at the same left edge, so
    only a line holding nothing but the code opens a row.
    """
    edges = Counter()
    x0s = []
    for page in pdf.pages:
        for line in page.extract_text_lines():
            text = normalize(line["text"])
            if re.fullmatch(r"-?\s*\d{1,4}\s*-?", text):
                continue
            x0s.append(line["x0"])
            if RANGE_IN_TEXT.match(clean_code(text)):
                edges[5 * round(line["x0"] / 5)] += 1
        page.flush_cache()
    edge, count = edges.most_common(1)[0]
    if count < sum(edges.values()) / 2 or not 40 <= edge <= 150:
        raise ValueError(f"no dominant classification column: {sorted(edges.items())}")
    limit = edge + CODE_COLUMN_SLACK
    beside = sum(x0 > limit + CODE_COLUMN_SLACK for x0 in x0s)
    return limit, beside > len(x0s) / 2


def text_schedule(pdf, filename):
    """Parser for the two-column text-only schedules."""
    rows = []
    notes = []
    target = None
    note = None
    pending = None
    chapter = ""
    section = ""
    started = False
    code_limit, beside = code_column(pdf)
    for page_number, page in enumerate(pdf.pages, 1):
        for line in page.extract_text_lines():
            text = normalize(line["text"])
            low = text.lower()
            if not started:
                if re.match(r"^(part|section) 2\b", low, re.I) or CHAPTER.fullmatch(text):
                    started = True
                else:
                    continue
            if any(low.startswith(marker) for marker in END_MARKERS):
                return rows, notes
            if re.fullmatch(r"-?\s*\d{1,4}\s*-?", text):
                continue
            in_code_column = line["x0"] <= code_limit or not beside
            if in_code_column and text.startswith("Section") and (m := SECTION.match(text)):
                section = m[1]
                chapter = ""
                target = None
                note = None
                pending = None
                continue
            if in_code_column and text.startswith("Chapter") and (m := CHAPTER.match(text)):
                chapter = f"{int(m[1]):02}"
                target = None
                note = None
                pending = None
                continue
            if text.startswith("Note:") or text.startswith("Notes to"):
                note = add_note(notes, "section" if section else "chapter",
                                section or chapter, text, page_number)
                target = None
                pending = None
                continue
            if NUMBERED_NOTE.match(text) and (section or chapter):
                # "Note 1:", "Note 2:" scope notes. Under a chapter heading
                # they belong to that chapter, otherwise to the section.
                note = add_note(notes, "chapter" if chapter else "section",
                                chapter or section, text, page_number)
                target = None
                pending = None
                continue
            match = RANGE_IN_TEXT.match(clean_code(text))
            remainder = clean_code(text)[match.end():].strip(" .;:") if match else ""
            if match and (match[1] or match[2]) and in_code_column and (beside or not remainder):
                key = match[0].strip()
                if not remainder:
                    pending = (key, page_number)
                    target = None
                    note = None
                    continue
                target = add_row(rows, key, page_number, remainder)
                pending = None
                note = None
                continue
            if pending is not None:
                key, pending_page = pending
                target = add_row(rows, key, pending_page, text)
                pending = None
                note = None
                continue
            if text.startswith("-") and chapter:
                target = add_row(rows, chapter, page_number, text.lstrip("- "), text)
                continue
            if note is not None:
                note["text"] = join(note["text"], text)
            elif target is not None:
                target["text"] = join(target["text"], text)
        page.close()
    return rows, notes


def table_schedule(pdf, filename):
    if filename in FIVE_COLUMN_PDFS:
        return five_column(pdf, filename)
    if filename in SIX_COLUMN_PDFS:
        return six_column(pdf, filename)
    if filename == "australia2.pdf":
        return australia(pdf)
    if filename == "switzerland3.pdf":
        return switzerland(pdf)
    if filename == "uk3.pdf":
        return uk(pdf)
    raise ValueError(f"no parser for {filename}")


def scope_numbered_notes(filename, notes):
    for note in notes:
        if filename in SCOPE_NOTE_PDFS:
            label = SCOPE_NOTE_START.match(note["text"])[0]
            key = (filename, note["page"], label, note["key"])
        else:
            label = NUMBERED_NOTE.match(note["text"])
            if not label:
                continue
            key = (filename, note["page"], label[0])
        if key not in NOTE_APPLIES:
            raise ValueError(f"{key}: note has no applicability entry")
        if NOTE_APPLIES[key]:
            note["applies"] = NOTE_APPLIES[key]


def extract(path, filename):
    rows, notes = _extract(path, filename)
    scope_numbered_notes(filename, notes)
    return rows, notes


def _extract(path, filename):
    with pdfplumber.open(path) as pdf:
        if filename in TEXT_PDFS:
            return text_schedule(pdf, filename)
        if filename in {"eu3.pdf", "asean2-1.pdf", "tpp5.pdf", "tpp2.pdf"}:
            return schedule(pdf, filename)
        return table_schedule(pdf, filename)


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
        rows, notes = extract(path, filename)
        if not rows:
            raise ValueError(f"{filename}: no rules extracted")
        sources.append({
            "agreement": agreement,
            "citation": f"{agreement} {annex} (HS {hs})",
            "source_url": URLS.get(filename, BASE + filename),
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
