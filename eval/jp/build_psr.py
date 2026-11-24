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
    # Annex 3-A (introductory notes), read only for the ANNEX_3A notes below.
    "eu2.pdf": "f9281449008f2f7ea155590404ebb4351c0801dca5038d3ee9714d9f141e8f42",
    "uk2.pdf": "0fbc823b40513a44d5be2c377e803a84b7d1b4d7c4c2164738a698d511acfcca",
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
# notes (Indonesia was audited and has no scope notes; see docs/japan/indonesia-scope-notes.md). Every
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
# JP-EU EPA / JP-UK CEPA: Section and Chapter note cells of Annex 3-B (a "Section
# note:" or "Chapter note:" sentence inside the heading cell). Each needs a
# reviewed entry in NOTE_APPLIES; an empty list is the whole Section or Chapter
# the cell names ("within this Section", "this Chapter").
CELL_NOTE_PDFS = {"eu3.pdf", "uk3.pdf"}
CELL_NOTE = re.compile(r"(?:Section|Chapter) note:")
NOTE_APPLIES.update({
    ("eu3.pdf", 12, "Section note:", "V"): [], ("eu3.pdf", 14, "Section note:", "VI"): [],
    ("eu3.pdf", 24, "Section note:", "VII"): [], ("eu3.pdf", 30, "Section note:", "XI"): [],
    ("eu3.pdf", 38, "Chapter note:", "57"): [],
    ("uk3.pdf", 14, "Section note:", "V"): [], ("uk3.pdf", 16, "Section note:", "VI"): [],
    ("uk3.pdf", 26, "Section note:", "VII"): [], ("uk3.pdf", 32, "Section note:", "XI"): [],
    ("uk3.pdf", 40, "Chapter note:", "57"): [],
})

# Annex 3-A (separate PDF) quoted verbatim, one note per source page chunk. Each
# entry is (page, label, start, end, scope, key, applies). start and end are the
# first and last words of the quoted chunk in the normalised page text (None: the
# page edge, without the page footer and the closing rule line). Only the
# provisions that define a term of the Annex 3-B rules or change how a rule
# qualifies are quoted; Note 1 paragraphs 1, 2 and 4, Note 2 paragraph 4 (the
# meaning of Chapter, heading, Section and subheading) are left out.
# See docs/japan/annex-3a-notes.md.
_N5 = [["25", "40"]]  # Note 5: "processes referred to in Sections V to VII"
_W5 = [["5106", "5110"], ["5204", "5207"]]  # Note 7 paragraph 5, from its own wording
_W8 = [["61", "63"]]  # Note 8 paragraph 2, from its own wording
_S, _XI = ("schedule", ""), ("section", "XI")
_EU_FORMULA = " MaxNOM(%) = (VNM / EXW) × 100 RVC(%) = ((FOB − VNM) / FOB) × 100"
_UK_FORMULA = " (a) MaxNOM(%) = (VNM / EXW) × 100 and (b) RVC(%) = ((FOB − VNM) / FOB) × 100"
# What the PDF text layer gives for the two typeset fractions; checked so a
# changed layer fails the build. The linear form above is the one transcription.
_EU_FORMULA_PDF = "VNM MaxNOM(%) = ×100 EXW FOB−VNM RVC(%) = ×100 FOB"
_UK_FORMULA_PDF = "(a) VNM MaxNOM(%) 100 EXW = × and (b) FOB VNM RVC(%) 100 FOB − = ×"
ANNEX_3A = {
    "eu3.pdf": {
        "pdf": "eu2.pdf", "label": "JP-EU EPA Annex 3-A",
        "notes": [
            (1, "Note 1 paragraph 3", "3. Reference to weight", "packaging.", *_S, []),
            (2, "Note 2 paragraphs 1 to 3", "Note 2", "all of the requirements.", *_S, []),
            (3, "Note 2 paragraph 5", "5. For the purposes of product specific rules of origin", "individually or jointly.", *_S, []),
            (4, "Note 3", "Note 3", "inherent nature.", *_S, []),
            (5, "Note 4", "Note 4", None, *_S, []),
            (6, "Note 4", None, None, *_S, []),
            (7, "Note 4", None, None, *_S, []),
            (8, "Note 4", None, ("formulas apply:", _EU_FORMULA, _EU_FORMULA_PDF), *_S, []),
            (9, "Note 5", "Note 5", None, *_S, _N5),
            (10, "Note 5", None, None, *_S, _N5),
            (11, "Note 5", '(e) "isomer separation"', "existing impurities.", *_S, _N5),
            (11, "Note 6", "Note 6", None, *_XI, []),
            (12, "Note 6", None, None, *_XI, []),
            (13, "Note 7", None, None, *_XI, []),
            (14, "Note 7", None, None, *_XI, []),
            (15, "Note 7", None, None, *_XI, []),
            (16, "Note 7", None, None, *_XI, []),
            (17, "Note 7 paragraph 4", "4. Notwithstanding", "exceed 10 per cent.", *_XI, []),
            (17, "Note 7 paragraph 5", "5. For a product", "weight of the product.", *_XI, _W5),
            (17, "Note 8 paragraph 1", "Note 8", None, *_XI, []),
            (18, "Note 8 paragraph 2", "2. Non-originating", "slide-fasteners normally contain textiles.", *_XI, _W8),
            (18, "Note 8 paragraph 3", "3. Where a requirement", None, *_XI, []),
        ],
    },
    "uk3.pdf": {
        "pdf": "uk2.pdf", "label": "JP-UK CEPA Annex 3-A",
        "notes": [
            (1, "Note 1 paragraph 3", "3. Reference to weight", "packaging.", *_S, []),
            (1, "Note 2 paragraph 1", "Note 2", None, *_S, []),
            (2, "Note 2 paragraphs 2 and 3", "2. Each product", "all of the requirements.", *_S, []),
            (2, "Note 2 paragraph 5", "5. For the purposes of product specific rules of origin", None, *_S, []),
            (3, "Note 2 paragraph 5", None, "in subheading) of the Harmonized System.", *_S, []),
            (3, "Note 3", "Note 3", None, *_S, []),
            (4, "Note 3 paragraph 4", "4. For greater certainty", "European Union.", *_S, []),
            (4, "Note 4", "Note 4", None, *_S, []),
            (5, "Note 4", None, None, *_S, []),
            (6, "Note 4", None, ("apply respectively:", _UK_FORMULA, _UK_FORMULA_PDF), *_S, []),
            (7, "Note 5", "Note 5", None, *_S, _N5),
            (8, "Note 5", None, None, *_S, _N5),
            (9, "Note 6", "Note 6", None, *_XI, []),
            (10, "Note 7", "Note 7", None, *_XI, []),
            (11, "Note 7", None, None, *_XI, []),
            (12, "Note 7", None, None, *_XI, []),
            (13, "Note 7 paragraph 5", "5. For a product", "weight of the product.", *_XI, _W5),
            (13, "Note 8 paragraph 1", "Note 8", "of the product.", *_XI, []),
            (13, "Note 8 paragraph 2", "2. For the purposes", "for that product.", *_XI, _W8),
            (13, "Note 8 paragraph 3", "3. Where a requirement", None, *_XI, []),
        ],
    },
}
PAGE_FOOTER = re.compile(r"\s*(?:EU/JP/Annex 3-A/en \d+)?\s*$")
CLOSING_RULE = re.compile(r"\s*_{4,}\s*$")


def quote_chunk(text, start, end):
    """The verbatim chunk of a normalised page between two anchors."""
    text = CLOSING_RULE.sub("", PAGE_FOOTER.sub("", text))
    tail = ""
    if isinstance(end, tuple):  # (last words, linear formula, expected PDF layer)
        end, formula, layer = end
        i = text.index(end) + len(end)
        if text[i:].strip() != layer:
            raise ValueError(f"formula text layer changed: {text[i:]!r}")
        text, tail = text[:i], formula
    i = 0 if start is None else text.index(start)
    if start is not None and text.count(start) != 1:
        raise ValueError(f"start {start!r} is not unique on its page")
    j = len(text) if end is None else text.index(end, i) + len(end)
    return text[i:j] + tail


def annex_3a_notes(path, filename):
    spec = ANNEX_3A[filename]
    notes = []
    with pdfplumber.open(path) as pdf:
        for page_number, label, start, end, scope, key, applies in spec["notes"]:
            page = pdf.pages[page_number - 1]
            note = {
                "scope": scope, "key": key,
                "text": quote_chunk(normalize(page.extract_text()), start, end),
                "page": page_number,
                "source_url": BASE + spec["pdf"],
                "citation": f"{spec['label']}, {label}",
            }
            if applies:
                note["applies"] = applies
            notes.append(note)
            page.close()
    return notes

# General Notes of the text-layer Annexes quoted verbatim from their own PDF
# (docs/japan/general-notes-embedded.md). Each entry is (page, label, start, end,
# applies): start and end are the first and last words of the quoted chunk in the
# normalised page text, so a page footer or a table row after the paragraph is
# never in it. Scope is schedule for all of them; applies is read from the
# paragraph's own wording (an empty list means the whole annex). Only provisions
# that define a term of the rows or change how a rule qualifies are quoted; the
# HS edition statements, the digit-prefix meanings of section, chapter, heading
# and subheading, the "adjacent rule" paragraphs and the CPTPP paragraphs 1, 2, 3(a),
# 3(b), 4 and 5 are cut (5: the dagger rows are withheld; see general-notes-embedded.md). Provisions that refer to the main Agreement (Articles) are quoted
# as the Annex words them and do not supply that text.
_WEIGHT = "unless otherwise specified in the Harmonized System;"
_ITA_END = "to subheading 8541.10 through 8542.90 from any other subheading."
# Indonesia (i)(ii), "For the purposes of headings 50.07, 51.11 through 51.13, ..."
_INDONESIA_TRANSPORT = _headings(
    "5007", "5111-5113", "5204-5212", "5309-5311", "5407", "5408", "5508-5516",
    "5604-5609", "5902", "5910", "57-58", "60-63")
_CHILE_F = _headings("19", "200110-200891", "200892", "200899-200990", "21", "28-97")
GENERAL_NOTES = {
    "thailand2-2.pdf": {"label": "JP-Thailand EPA Annex 2 General Notes", "notes": [
        (1, "paragraphs (b) and (c)", "(b) a requirement of a change in tariff classification", _WEIGHT, []),
        (1, "paragraph (d)(v)", "(v) the term “QVC 40” means", "has been performed in a Party;", []),
        (2, "paragraphs (d)(vi) to (d)(ix)", "(vi) the term “CC” denotes", "as defined in paragraph 2 of Article 28;", []),
        (2, "paragraph (f)", "(f) specific percentages referred to in Article 30,", "10 per cent in value of the good; and",
         _headings("19-24", "28-97")),
        (3, "paragraph (f)(iii)", "(iii) in the case of a good provided for in chapters 50 through 63",
         "10 per cent by weight of the good; and", _headings("50-63")),
        (3, "Notes 1 and 2 to paragraph (f)", "Note 1: The term “value of non-originating materials”",
         "the value provided for in paragraph 5 of that Article.", []),
        (3, "paragraph (g)", "(g) a good which is covered by Attachment A or B",
         "classified in subheadings 8541.10 through 8542.90.", []),
    ]},
    "indonesia2-2.pdf": {"label": "JP-Indonesia EPA Annex 2 General Notes", "notes": [
        (1, "paragraph (b)", "(b) reference to weight in this Annex means dry weight", _WEIGHT, []),
        (1, "paragraphs (d), (e)(i) and (e)(ii)", "(d) where a specific rule of origin is defined using the criterion",
         "(i.e. a change in heading) of the Harmonized System;", []),
        (2, "paragraphs (e)(iii) and (e)(iv)", "(iii) the term “CTSH” denotes", "has been performed in a Party;", []),
        (2, "paragraph (g)", "(g) specific percentages referred to in Article 31,", "seven percent by weight of the good;",
         _headings("28-97")),
        (2, "Notes 1 and 2 to paragraph (g)", "Note 1: The term “value of non-originating materials”",
         "the value set out in paragraph 5 of that Article.", []),
        (2, "paragraph (h)(i)", "(h) For the purposes of chapters 28 through 40",
         "(C) the addition or elimination of water of crystallization;", _headings("28-40")),
        (3, "paragraphs (h)(ii) to (h)(iv)", "(ii) the term “purification” means", "of cellular or intercellular structures;",
         _headings("28-40")),
        (3, "paragraph (i)(i)", "(i) (i) For the purposes of chapters 50 through 55 and 60", "(2) antimelt finish;", _TEXTILE_1),
        (4, "paragraph (i)(i), items (3) to (25)", "(3) antimosquito finish;", "(25) milling;", _TEXTILE_1),
        (5, "paragraph (i)(i), items (26) to (48)", "(26) moare finishing;", "(48) wire raising;", _TEXTILE_1),
        (6, "paragraph (i)(ii)", "(ii) For the purposes of headings 50.07, 51.11 through 51.13,",
         "any other operation to preserve it in good condition;", _INDONESIA_TRANSPORT),
        (6, "paragraph (i)(iii)", "(iii) For the purposes of determining the origin of a good of chapters 61 through 63",
         "set out in the rule for that good;", _headings("61-63")),
        (6, "paragraph (j)", "(j) (i) a good which is covered by Attachment A or B", "8541.10 through 8542.90.", []),
    ]},
    "malaysia2.pdf": {"label": "JP-Malaysia EPA Annex 2 General Notes", "notes": [
        (1, "paragraph (b)", "(b) reference to weight in this Annex means dry weight", _WEIGHT, []),
        (1, "paragraph (e)", "(e) Specific percentages referred to in Article 30,", "7 percent by weight of the good;",
         _headings("28-97")),
        (2, "Notes 1 and 2 to paragraph (e)", "Note 1: the term “value of non-originating materials”",
         "the value set out in paragraph 5 of that Article.", []),
        (2, "paragraph (f)", "(f) (i) a good which is covered by Attachment A or B", _ITA_END, []),
    ]},
    "philippine2.pdf": {"label": "JP-Philippines EPA Annex 2 General Notes", "notes": [
        (1, "paragraphs (b) and (c)", "(b) reference to weight in this Annex means dry weight",
         "does not indicate priority of application;", []),
        (1, "paragraph (f), introduction", "(f) Specific percentages referred to in Article 31,", "are as follows:",
         _headings("28-97")),
        (2, "paragraphs (f)(i) and (f)(ii)", "(i) in the case of a good provided for in chapter 28 through 49",
         "7 percent by weight of the good;", _headings("28-97")),
        (2, "Notes 1 and 2 to paragraph (f)", "Note 1: The term “value of non-originating materials”",
         "the value set out in paragraph 5 of that Article.", []),
        (2, "paragraph (g)", "(g) (i) a good which is covered by Attachment A or B", _ITA_END, []),
    ]},
    "mexico2.pdf": {"label": "JP-Mexico EPA Annex 4 General Interpretative Notes", "notes": [
        (1, "paragraph (b)", "(b) a requirement of a change in tariff classification applies only to non-originating materials;",
         "applies only to non-originating materials;", []),
        (1, "paragraph (c)", "(c) reference to weight in the rules for goods provided for in chapter 1 through 24", _WEIGHT,
         _headings("01-24")),
        (1, "paragraph (f)", "(f) the goods classified on Chapter 61, 62 or 63 and exported from Mexico to Japan",
         "calculated on a F.O.B. basis:", _headings("61-63")),
        (2, "paragraphs (f)(i) to (f)(iii)", "(i) 61.01 – 61.17 A change to heading 61.01 through 61.17 from any other chapter;",
         "63.10 from any other chapter; and", _headings("61-63")),
        (2, "paragraph (g)", "(g) paragraph (f) above shall be implemented in accordance with",
         "management of such allocation.", _headings("61-63")),
    ]},
    "chile2.pdf": {"label": "JP-Chile EPA Annex 2 General Notes", "notes": [
        (1, "paragraphs (b) and (c)", "(b) reference to weight in this Annex means dry weight",
         "applies only to non-originating materials;", []),
        (1, "paragraph (d), Build-down and Build-up methods",
         "the term “Build-down method” means the method referred to in subparagraph 1(a) of Article 30;",
         "subparagraph 1(b) of Article 30;", []),
        (2, "paragraph (f)", "(f) specific percentages referred to in Article 32,", "7 percent by weight of the good.", _CHILE_F),
        (2, "Notes 1 and 2 to paragraph (f)", "Note 1: The term “value of non-originating materials”",
         "the value set out in paragraph 2 of that Article.", []),
    ]},
    "vietnam2.pdf": {"label": "JP-Vietnam EPA Annex 2", "notes": [
        (1, "paragraph 1", "1. For the purposes of the product specific rules set out in this Annex and the Attachment to Annex 3, the term:",
         "as defined in Article 25.", []),
        (1, "paragraph 3", "3. For the purposes of subparagraph 1(b) of Article 28, the following shall apply:",
         "10 percent of the FOB; and", _headings("090121-090122", "180310-180320", "180500", "210390")),
        (2, "paragraph 3(b)", "(b) in the case of a good classified under subheading 2103.90", "seven percent of the FOB.",
         _headings("210390")),
    ]},
    "switzerland3.pdf": {"label": "JP-Switzerland EPA Appendix 1 to Annex II Introductory Notes", "notes": [
        (1, "paragraph 1(a)", "1. For the purposes of the product specific rules set out in this Appendix:",
         "as defined in Article III of Annex II;", []),
        (2, "paragraph 1(b)", "(b) In determining the origin of a product classified under chapters 61 to 63,",
         "whether or not they contain textiles.", _headings("61-63")),
    ]},
    "tpp2.pdf": {"label": "CPTPP Annex 3-D Section A General Interpretative Notes", "notes": [
        (2, "paragraphs 3(c) to 3(g)", "(c) the requirement of a change in tariff classification shall apply only to non-originating materials;",
         "within the group.", []),
    ]},
    "tpp5.pdf": {"label": "CPTPP Annex 4-A Section A General Interpretative Notes", "notes": [
        (2, "paragraphs 3(c) to 3(g)", "(c) the requirement of a change in tariff classification shall apply only to non-originating materials;",
         "rules of origin set out in this Annex.", []),
    ]},
}


def quote_general(text, start, end):
    """The verbatim chunk of a normalised page from a unique start to the end
    words, which must occur exactly once after the start."""
    if text.count(start) != 1:
        raise ValueError(f"start {start!r} is not unique on its page")
    i = text.index(start)
    if end not in text[i:] or text[i:].count(end) != 1:
        raise ValueError(f"end {end!r} does not occur exactly once after the start")
    return text[i:text.index(end, i) + len(end)]


def general_notes(path, filename):
    spec = GENERAL_NOTES[filename]
    notes = []
    with pdfplumber.open(path) as pdf:
        for page_number, label, start, end, applies in spec["notes"]:
            page = pdf.pages[page_number - 1]
            note = {
                "scope": "schedule", "key": "",
                "text": quote_general(normalize(page.extract_text()), start, end),
                "page": page_number,
                "source_url": URLS.get(filename, BASE + filename),
                "citation": f"{spec['label']}, {label}",
            }
            if applies:
                note["applies"] = applies
            notes.append(note)
            page.close()
    return notes


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


def cell_note(notes, filename, scope, key, cell, page_number):
    """A "Section note:" or "Chapter note:" sentence inside a heading cell."""
    m = CELL_NOTE.search(cell) if filename in CELL_NOTE_PDFS else None
    if m:
        add_note(notes, scope, key, cell[m.start():], page_number)


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
                    cell_note(notes, filename, "chapter", chapter, text, page_number)
                    continue
                if m := SECTION.match(classification):
                    section = m[1]
                    key = ""
                    branches = []
                    pending = None
                    target = None
                    cell_note(notes, filename, "section", section, text, page_number)
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


AUSTRALIA_COLUMN_3 = "Rule (column 3): "
AUSTRALIA_COLUMN_4 = "; alternative (column 4): "
# Chapters whose rows can carry a column 4 (General Notes paragraph 6 and the
# table header note speak of chapters 27 to 40).
AUSTRALIA_ALTERNATIVE_CHAPTERS = [["27", "40"]]
AUSTRALIA_NOTE_PAGES = 10  # General Notes, PDF pages 1-10; the table starts on page 11


def australia_text(column3, column4):
    """Row text keeping both columns, labelled as the PDF header relates them."""
    if not column4:
        return column3
    return f"{AUSTRALIA_COLUMN_3}{column3}{AUSTRALIA_COLUMN_4}{column4}"


def australia_columns(text):
    """Inverse of australia_text: (column 3, column 4)."""
    if text.startswith(AUSTRALIA_COLUMN_3) and AUSTRALIA_COLUMN_4 in text:
        column3, _, column4 = text[len(AUSTRALIA_COLUMN_3):].partition(AUSTRALIA_COLUMN_4)
        return column3, column4
    return text, ""


def padded_range(start, end):
    return start.ljust(6, "0"), end.ljust(6, "9")


def usage_ranges(rows, uses):
    """6-digit [start, end] ranges covering the rows that use a term.

    Consecutive rows that all use the term merge into one range; a range may
    never cover a row that does not use the term.
    """
    ranges = []
    run = None
    for row in rows:
        if not uses(row):
            run = None
            continue
        lo, hi = padded_range(row["start"], row["end"])
        if run is not None and lo > run[1]:
            run[1] = hi
        else:
            run = [lo, hi]
            ranges.append(run)
    for row in rows:
        if uses(row):
            continue
        lo, hi = padded_range(row["start"], row["end"])
        if any(a <= hi and lo <= z for a, z in ranges):
            raise ValueError(f"row {row['start']}-{row['end']} lies inside a usage range")
    return ranges


def annex_text(page):
    """Verbatim text of a General Notes page: lines joined, page number dropped.

    A line that ends in a hyphen glued to a letter ("non-" / "originating")
    is joined without a space; "Origin -" keeps its space.
    """
    lines = (page.extract_text() or "").split("\n")
    if lines and lines[-1].strip().isdigit():
        lines.pop()
    out = ""
    for line in lines:
        line = line.strip()
        if out.endswith("-") and not out.endswith(" -"):
            out += line
        else:
            out = f"{out} {line}" if out else line
    return " ".join(out.split())


def between(text, start, end):
    """The text from one marker up to, not including, the next; each marker is unique."""
    for marker in (start, end):
        if text.count(marker) != 1:
            raise ValueError(f"marker {marker!r} found {text.count(marker)} times")
    i, j = text.index(start), text.index(end)
    if j <= i:
        raise ValueError(f"marker {end!r} precedes {start!r}")
    return text[i:j].strip()


QVC = "QVC 40"
CTC = "CTC"  # rows whose rule text (column 3 or 4) uses CC, CTH or CTSH
CTC_WORD = re.compile(r"\b(?:CC|CTH|CTSH)\b")
TEXTILE_CODES = [f"({n})" for n in range(1, 18)]
COLUMN_4_CODES = ("CR", "D", "P", "CPS", "SM", "IS")
CODE_WORD = re.compile(r"[A-Z]+")
PARA_5_LEAD = ("5. For the purposes of column 3", "(a) “CC” denotes")
PARA_6_LEAD = ("6. For the purposes of column 4", "(a) “CR” means")
_CODE_START = {"CR": "(a) “CR”", "D": "(b) “D”", "P": "(c) “P”", "CPS": "(d) “CPS”",
               "SM": "(e) “SM”", "IS": "(f) “IS”"}
_CODE_END = {"CR": "(b) “D”", "D": "(c) “P”", "P": "(d) “CPS”", "CPS": "(e) “SM”",
             "SM": "(f) “IS”", "IS": "7. In determining"}
_LETTERS = "efghijklmnopqrstu"
_TEXTILE_START = {f"({n})": f"({_LETTERS[n - 1]}) “({n})” denotes" for n in range(1, 18)}
_TEXTILE_END = {f"({n})": f"({_LETTERS[n]}) “({n + 1})” denotes" for n in range(1, 17)}
_TEXTILE_END["(17)"] = "6. For the purposes of column 4"


def row_uses(term, row):
    column3, column4 = australia_columns(row["text"])
    if term == CTC:
        return bool(CTC_WORD.search(column3) or CTC_WORD.search(column4))
    if term == QVC or term in TEXTILE_CODES:
        return term in column3
    return term in CODE_WORD.findall(column4)


def australia_general_notes(pages, rows):
    """Schedule-scope notes for the General Notes that define the rule codes.

    Each definition is its own verbatim note, pinned to the PDF page it starts
    on and attached only to the rows that use the term. Paragraphs 1-2 attach
    to chapters 27-40, paragraph 3 to rows using CC, CTH or CTSH (where column 4 exists) and paragraph 7 to chapters
    61-63, which its own wording names.
    """
    texts = [annex_text(page) for page in pages]
    full = " ".join(texts)
    ends = []
    total = 0
    for text in texts:
        total += len(text) + 1
        ends.append(total)

    def note(body, applies, lead=""):
        page = 1 + next(i for i, end in enumerate(ends) if full.index(body) < end)
        return {"scope": "schedule", "key": "", "text": f"{lead} {body}".strip(), "page": page,
                "applies": applies}

    out_para3 = note(between(full, "3. Where a specific rule of origin", "4. For the purposes of this Annex"),
                     usage_ranges(rows, lambda r: row_uses(CTC, r)))
    lead5 = between(full, *PARA_5_LEAD)
    lead6 = between(full, *PARA_6_LEAD)
    out = [note(between(full, "1. The specific rule or specific set", "3. Where a specific rule of origin"),
                AUSTRALIA_ALTERNATIVE_CHAPTERS), out_para3]
    out.append(note(between(full, "(d) “QVC 40” means", "(e) “(1)” denotes"),
                    usage_ranges(rows, lambda r: row_uses(QVC, r)), lead5))
    for code in TEXTILE_CODES:
        out.append(note(between(full, _TEXTILE_START[code], _TEXTILE_END[code]),
                        usage_ranges(rows, lambda r, c=code: row_uses(c, r)), lead5))
    for code in COLUMN_4_CODES:
        out.append(note(between(full, _CODE_START[code], _CODE_END[code]),
                        usage_ranges(rows, lambda r, c=code: row_uses(c, r)), lead6))
    out.append(note(between(full, "7. In determining", "8. This Annex is based"), [["61", "63"]]))
    return out


def australia(pdf):
    """Parser for the Japan-Australia four-column table.

    Column 3 holds the rule and column 4 the alternative rules of chapters 27
    to 40; the row text keeps both, labelled (see australia_text).
    """
    rows = []
    notes = []
    column4 = {}
    target = None
    for page_number, page in enumerate(pdf.pages, 1):
        for table in page.extract_tables():
            for raw in table:
                cells = [normalize(cell) for cell in raw]
                if len(cells) < 3:
                    continue
                code, description, text = cells[:3]
                alternative = cells[3] if len(cells) > 3 else ""
                if code.startswith(("Tariff Item", "Tariff item")) or code in ("1", "2", "3", "4"):
                    continue
                if code.startswith("Note:") and not description and not text:
                    notes.append({"scope": "schedule", "key": "", "text": code, "page": page_number,
                                  "applies": AUSTRALIA_ALTERNATIVE_CHAPTERS})
                    continue
                if m := CHAPTER.match(code):
                    key = f"{int(m[1]):02}"
                    if text:
                        target = add_row(rows, key, page_number, text)
                        if alternative:
                            column4[id(target)] = alternative
                    elif alternative:
                        raise ValueError(f"{code}: column 4 without a column 3 rule")
                    else:
                        target = None
                    continue
                if SECTION.match(code):
                    target = None
                    continue
                if description.startswith("Note:") or code.startswith("Note:"):
                    continue
                if not RANGE.fullmatch(clean_code(code)):
                    if alternative:
                        raise ValueError(f"{code!r}: column 4 {alternative!r} on a row without a code")
                    if text and target is not None:
                        target["text"] = join(target["text"], text)
                    continue
                if text:
                    target = add_row(rows, code, page_number, text, description)
                    if alternative:
                        column4[id(target)] = alternative
                elif raw[2] is None and target is not None:
                    target = add_row(rows, code, target["page"], target["text"], description)
                elif alternative:
                    raise ValueError(f"{code}: column 4 without a column 3 rule")
                else:
                    target = None
        page.close()
    for row in rows:
        if id(row) in column4:
            row["text"] = australia_text(row["text"], column4[id(row)])
    return rows, notes


def australia_schedule(pdf):
    """The table (australia) plus the General Notes (PDF pages 1-10) it needs."""
    rows, notes = australia(pdf)
    notes = australia_general_notes(pdf.pages[:AUSTRALIA_NOTE_PAGES], rows) + notes
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
                if m := SECTION.match(classification):
                    close_heading()
                    target = None
                    chapter_open = False
                    cell_note(notes, "uk3.pdf", "section", m[1], text, page_number)
                    continue
                if m := CHAPTER.match(classification):
                    close_heading()
                    chapter = f"{int(m[1]):02}"
                    target = None
                    chapter_open = True
                    cell_note(notes, "uk3.pdf", "chapter", chapter, text, page_number)
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
        return australia_schedule(pdf)
    if filename == "switzerland3.pdf":
        return switzerland(pdf)
    if filename == "uk3.pdf":
        return uk(pdf)
    raise ValueError(f"no parser for {filename}")


def scope_numbered_notes(filename, notes):
    for note in notes:
        if filename in CELL_NOTE_PDFS:
            label = CELL_NOTE.match(note["text"])
            if not label:  # a footnote note, scoped where it is added
                continue
            key = (filename, note["page"], label[0], note["key"])
        elif filename in SCOPE_NOTE_PDFS:
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


def annex_path(directory, name):
    path = directory / name
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    if digest != DIGESTS[name]:
        raise ValueError(f"{name}: source digest changed ({digest})")
    return path


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
        if filename in GENERAL_NOTES:
            notes += general_notes(path, filename)
        if filename in ANNEX_3A:
            notes += annex_3a_notes(annex_path(args.pdf_directory, ANNEX_3A[filename]["pdf"]), filename)
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
