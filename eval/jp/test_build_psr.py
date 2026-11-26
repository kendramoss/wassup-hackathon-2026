"""Unit tests for the pure helpers of build_psr.py (no PDFs needed).

Run from this directory: python3 -m unittest test_build_psr
"""
import os
import unittest

try:
    import build_psr as b
except ImportError as error:  # pdfplumber missing
    raise unittest.SkipTest(str(error))


class StatedEdition(unittest.TestCase):
    """The declared HS edition is an explicit typed table, never read from prose."""

    def test_every_source_has_an_explicit_known_edition(self):
        self.assertEqual(set(b.STATED_EDITIONS), {s[0] for s in b.SOURCES})
        for filename, edition in b.STATED_EDITIONS.items():
            self.assertIn(edition, (2002, 2007, 2012, 2017, 2022), filename)
            self.assertIsInstance(edition, int)

    def test_citation_edition_matches_the_table(self):
        for filename, agreement, annex, hs, start, end in b.SOURCES:
            entry = b.source_entry(filename, agreement, annex, hs, start, end)
            self.assertEqual(entry["stated_edition"], b.STATED_EDITIONS[filename])
            self.assertIn(f"(HS {entry['stated_edition']})", entry["citation"])

    def test_wording_cannot_change_the_edition(self):
        entry = b.source_entry("eu3.pdf", "JP-EU EPA", "Annex 3-B", "2017", "2019-02-01", "")
        self.assertEqual(entry["stated_edition"], 2017)
        self.assertEqual(b.source_entry("eu3.pdf", "X validated against HS 2022", "A", "2017", "", "")["stated_edition"], 2017)

    def test_editions_without_a_statement_are_listed(self):
        self.assertEqual(b.NOT_STATED, ("JP-US TA", "GSP"))


class FakePage:
    def __init__(self, x0s):
        self.lines = [{"text": "3901.10 x", "x0": x} for x in x0s]

    def extract_text_lines(self):
        return self.lines

    def flush_cache(self):
        pass


class FakePdf:
    def __init__(self, *pages):
        self.pages = [FakePage(p) for p in pages]


class Helpers(unittest.TestCase):
    def test_code_range_pads_to_equal_width(self):
        self.assertEqual(b.code_range("39.01-39.02"), ("3901", "3902"))
        self.assertEqual(b.code_range("3901.10-39.02"), ("390110", "390299"))
        with self.assertRaises(ValueError):
            b.code_range("39.02-39.01")

    def test_dangling(self):
        for text in ("heading 22.07 for any", "from heading 50.07,", "that the non-", "through"):
            self.assertTrue(b.DANGLING.search(text), text)
        for text in ("heading 22.07.", "CC", "not less than 40 percent."):
            self.assertFalse(b.DANGLING.search(text), text)

    def test_numbered_note(self):
        self.assertTrue(b.NUMBERED_NOTE.match("Note 2: For the purposes of"))
        self.assertFalse(b.NUMBERED_NOTE.match("Note: For the purposes of"))

    def test_code_column_edge_and_slack(self):
        pdf = FakePdf([85, 85, 230], [84, 86, 230, 230])
        limit, beside = b.code_column(pdf)
        self.assertEqual(limit, 85 + b.CODE_COLUMN_SLACK)
        self.assertFalse(beside)

    def test_code_column_requires_a_dominant_edge(self):
        with self.assertRaises(ValueError):
            b.code_column(FakePdf([85, 120, 160, 200]))
        with self.assertRaises(ValueError):
            b.code_column(FakePdf([300, 300]))

    def test_numbered_notes_need_an_applicability_entry(self):
        with self.assertRaises(ValueError):
            b.scope_numbered_notes("malaysia2.pdf", [{"text": "Note 9: x", "page": 1}])
        notes = [{"text": "Note 1: x", "page": 35}]
        b.scope_numbered_notes("malaysia2.pdf", notes)
        self.assertEqual(notes[0]["applies"], [["50", "55"], ["60", "60"]])

    def test_thailand_notes_need_a_reviewed_scope(self):
        with self.assertRaises(ValueError):
            b.scope_numbered_notes("thailand2-2.pdf", [{"text": "Note: x", "page": 4, "key": "04"}])
        notes = [{"text": "Note: x", "page": 7, "key": "18"},
                 {"text": "Note: y", "page": 29, "key": "71"}]
        b.scope_numbered_notes("thailand2-2.pdf", notes)
        self.assertEqual(notes[0]["applies"], [["180320", "180320"], ["1805", "1805"]])
        self.assertNotIn("applies", notes[1])  # "this chapter": whole chapter

    def test_withheld_narrowing_tables(self):
        self.assertEqual([b.code_range(c) for c in b.EU_NARROW["70.071 -70.09"]["blocked"]],
                         [("700711", "700711"), ("700721", "700721")])
        self.assertEqual(b.EU_FOOTNOTES["70.071 -70.09"], "70.07-70.09")
        ranges = [b.code_range(code) for k in b.UK_FOOTNOTES for code, _ in b.UK_FOOTNOTES[k]]
        self.assertEqual(ranges, sorted(ranges))
        self.assertEqual(b.code_range("87.09-87.11"), ("8709", "8711"))
        (lines,) = b.SWISS_SPLITS.values()
        swiss = [b.code_range(code) for code, _ in lines]
        self.assertEqual(swiss, [("3501", "3501"), ("350211", "350219"), ("350220", "350499"), ("3505", "3505")])


class TablePage:
    def __init__(self, *rows):
        self.rows = rows

    def extract_tables(self):
        return [[list(r) for r in self.rows]]

    def close(self):
        pass


class TablePdf:
    def __init__(self, *pages):
        self.pages = [TablePage(*rows) for rows in pages]


class UkHeaderRows(unittest.TestCase):
    """Empty-cell heading rows never copy the previous row's rule."""

    PREV = ("70.01-70.05", "CTH; MaxNOM 50 % (EXW); or RVC 55 % (FOB).")

    def run_uk(self, *pages):
        return b.uk(TablePdf(*pages))

    def test_empty_heading_creates_branch_rows_not_a_copy(self):
        rows, _ = self.run_uk([
            ("Chapter 70", "Glass"), self.PREV, ("70.06", ""),
            ("- Coated plates", "Rule A."), ("- Others", "Rule B."),
            ("70.07", "Rule C.")])
        self.assertEqual([(r["start"], r["end"], r["text"], r.get("description")) for r in rows], [
            ("7001", "7005", self.PREV[1], None),
            ("7006", "7006", "Rule A.", "- Coated plates"),
            ("7006", "7006", "Rule B.", "- Others"),
            ("7007", "7007", "Rule C.", None)])

    def test_branches_follow_across_a_page_break(self):
        rows, _ = self.run_uk(
            [("Chapter 59", "x"), ("59.05", ""), ("- A", "Rule A.")],
            [("Column 1 Harmonized System classification (2017)", "Column 2"), ("- Others", "Rule B.")])
        self.assertEqual([(r["start"], r["page"], r["description"]) for r in rows],
                         [("5905", 1, "- A"), ("5905", 2, "- Others")])

    def test_heading_only_range_keeps_its_codes(self):
        rows, _ = self.run_uk([("Chapter 61", "x"), ("61.01-61.17", ""), ("- Others", "R")])
        self.assertEqual([(r["start"], r["end"]) for r in rows], [("6101", "6117")])

    def test_heading_ends_at_the_next_row(self):
        rows, _ = self.run_uk([
            ("Chapter 62", "x"), ("62.02", ""), ("- Others", "R1"), ("62.03", "R2"),
            ("62.04", ""), ("- Others", "R3")])
        self.assertEqual([(r["start"], r["text"]) for r in rows],
                         [("6202", "R1"), ("6203", "R2"), ("6204", "R3")])

    def test_own_cells_with_identical_text_stay_separate_rows(self):
        rows, _ = self.run_uk([("Chapter 62", "x"), ("62.03", "Same."), ("62.05", "Same.")])
        self.assertEqual([r["text"] for r in rows], ["Same.", "Same."])

    def test_one_footnoted_cell_for_several_codes_is_split_with_its_own_text(self):
        rows, _ = self.run_uk([("Chapter 87", "x"), ("87.085-87.11", "CTH.")])
        self.assertEqual([(r["start"], r["end"], r["text"], r.get("blocked", False)) for r in rows],
                         [("8708", "8708", "CTH.", True), ("8709", "8711", "CTH.", False)])

    def test_chapter_level_sub_lines_stay_with_the_chapter(self):
        rows, _ = self.run_uk([("Chapter 3", "x"), ("- Tuna", "R1"), ("- Others", "R2")])
        self.assertEqual([(r["start"], r["end"]) for r in rows], [("03", "03"), ("03", "03")])

    def test_continuation_text_joins_the_branch_row(self):
        rows, _ = self.run_uk([("Chapter 70", "x"), ("70.06", ""), ("- A", "Part one."), ("", "Part two.")])
        self.assertEqual(rows[0]["text"], "Part one. Part two.")

    def test_heading_without_branches_fails_the_build(self):
        with self.assertRaises(ValueError):
            self.run_uk([("Chapter 70", "x"), ("70.06", ""), ("70.07", "R")])
        with self.assertRaises(ValueError):
            self.run_uk([("Chapter 70", "x"), ("70.06", "")])

    def test_branch_without_rule_fails_the_build(self):
        with self.assertRaises(ValueError):
            self.run_uk([("Chapter 70", "x"), ("70.06", ""), ("- A", "")])

    def test_branch_without_a_heading_or_chapter_row_fails_the_build(self):
        with self.assertRaises(ValueError):
            self.run_uk([("Chapter 70", "x"), ("70.01", "R"), ("- A", "R2")])


class AustraliaColumn4(unittest.TestCase):
    """Column 4 (alternative rules of chapters 27-40) is kept with column 3."""

    def test_row_text_labels_both_columns(self):
        self.assertEqual(
            b.australia_text("CTH", "CR or D"),
            "Rule (column 3): CTH; alternative (column 4): CR or D")
        self.assertEqual(b.australia_text("CTH", ""), "CTH")
        self.assertEqual(b.australia_columns("Rule (column 3): CTH or QVC 40; alternative (column 4): CR, P, SM or IS"),
                         ("CTH or QVC 40", "CR, P, SM or IS"))
        self.assertEqual(b.australia_columns("CC"), ("CC", ""))

    def table_pdf(self):
        hdr = ("Tariff Item Number (HS 2012)", "Description of Goods", "Product Specific Rules", None)
        return TablePdf([
            hdr,
            ("Note: Goods which satisfy the applicable rules set out in column 3 or column 4 below shall be considered as originating goods of a Party.", None, None, None),
            ("1", "2", "3", "4"),
            ("Chapter 27", "MINERAL FUELS", "", ""),
            ("27.01", "Coal", "CTH", "CR"),
            ("27.02", "Lignite", "CTH, or a change", "CR or\nD"),
            ("", "", "from any other good.", ""),
            ("Chapter 36", "EXPLOSIVES", "CTH or QVC 40", "CR"),
            ("Chapter 41", "HIDES", "CC", ""),
        ])

    def test_parser_joins_continuation_into_column_3_and_keeps_column_4(self):
        rows, notes = b.australia(self.table_pdf())
        self.assertEqual([(r["start"], r["text"]) for r in rows], [
            ("2701", "Rule (column 3): CTH; alternative (column 4): CR"),
            ("2702", "Rule (column 3): CTH, or a change from any other good.; alternative (column 4): CR or D"),
            ("36", "Rule (column 3): CTH or QVC 40; alternative (column 4): CR"),
            ("41", "CC")])
        header = [n for n in notes if n["page"] == 1 and n["text"].startswith("Note: Goods which satisfy")]
        self.assertEqual(len(header), 1)
        self.assertEqual(header[0]["scope"], "schedule")
        self.assertEqual(header[0]["applies"], [["27", "40"]])

    def test_a_filled_column_4_without_a_row_fails_the_build(self):
        pdf = TablePdf([("Chapter 27", "x", "", ""), ("", "", "", "CR")])
        with self.assertRaises(ValueError):
            b.australia(pdf)

    def test_usage_ranges_merge_runs_and_stop_at_unused_rows(self):
        rows = [{"start": s, "end": e} for s, e in
                [("2801", "2801"), ("2802", "2802"), ("2803", "2803"), ("280410", "280419"), ("2805", "2805")]]
        uses = [True, True, False, True, True]
        got = b.usage_ranges(rows, lambda r: uses[rows.index(r)])
        self.assertEqual(got, [["280100", "280299"], ["280410", "280599"]])

    def test_usage_ranges_reject_a_run_that_covers_an_unused_row(self):
        rows = [{"start": "2801", "end": "2801"}, {"start": "280110", "end": "280110"}, {"start": "2802", "end": "2802"}]
        with self.assertRaises(ValueError):
            b.usage_ranges(rows, lambda r: r is not rows[1])

    def test_page_text_joins_hyphenated_lines_and_drops_the_page_number(self):
        class P:
            def __init__(self, t):
                self.t = t

            def extract_text(self):
                return self.t

        text = b.annex_text(P("the non-\noriginating material (Rules of Origin -\nCalculation) shall\n768"))
        self.assertEqual(text, "the non-originating material (Rules of Origin - Calculation) shall")

    def test_between_is_verbatim_and_requires_both_markers_once(self):
        text = "a (d) “QVC 40” means x. (e) “(1)” denotes y"
        self.assertEqual(b.between(text, "(d) “QVC", "(e) “(1)”"), "(d) “QVC 40” means x.")
        with self.assertRaises(ValueError):
            b.between(text, "(z)", "(e)")
        with self.assertRaises(ValueError):
            b.between("(d) (d) (e)", "(d)", "(e)")


@unittest.skipUnless(os.environ.get("JP_PDF_DIR"), "set JP_PDF_DIR to the directory holding australia2.pdf")
class AustraliaPdfTruth(unittest.TestCase):
    """Pinned to the official PDF (digest checked): every column 4 is kept."""

    @classmethod
    def setUpClass(cls):
        import hashlib
        import pdfplumber
        path = os.path.join(os.environ["JP_PDF_DIR"], "australia2.pdf")
        with open(path, "rb") as f:
            assert hashlib.sha256(f.read()).hexdigest() == b.DIGESTS["australia2.pdf"]
        cls.pdf = pdfplumber.open(path)
        cls.rows, cls.notes = b.australia_schedule(cls.pdf)
        cls.truth = []
        for number, page in enumerate(cls.pdf.pages, 1):
            for table in page.extract_tables():
                for raw in table:
                    if len(raw) > 3 and raw[3] and raw[3].strip() and b.normalize(raw[0]) != "1":
                        cls.truth.append((number, b.normalize(raw[0]), b.normalize(raw[2]), b.normalize(raw[3])))

    def test_every_pdf_column_4_is_in_a_row_of_that_page(self):
        self.assertEqual(len(self.truth), 530)
        for number, code, column3, column4 in self.truth:
            key = b.CHAPTER.match(code) and f"{int(b.CHAPTER.match(code)[1]):02}" or b.code_range(code)[0]
            matching = [r for r in self.rows if r["start"] == key and r["page"] == number
                        and b.australia_columns(r["text"])[1] == column4]
            self.assertTrue(matching, (number, code, column4))
            self.assertTrue(b.australia_columns(matching[0]["text"])[0].startswith(column3), (code,))

    def test_no_other_row_has_a_column_4_label(self):
        labelled = [r for r in self.rows if b.AUSTRALIA_COLUMN_4 in r["text"]]
        self.assertEqual(len(labelled), 530)

    def test_notes_are_verbatim_and_page_pinned(self):
        by = {n["text"][:30]: n for n in self.notes}
        qvc = [n for n in self.notes if "QVC 40" in n["text"] and n["text"].startswith("5.")][0]
        self.assertEqual(qvc["page"], 2)
        self.assertTrue(qvc["text"].endswith("shall apply."))
        self.assertEqual(len(self.notes), 1 + 1 + 1 + 17 + 6 + 1 + 1)
        header = [n for n in self.notes if n["page"] == 11]
        self.assertEqual(header[0]["text"], "Note: Goods which satisfy the applicable rules set out in column 3 or column 4 below shall be considered as originating goods of a Party.")
        self.assertEqual(sorted({n["scope"] for n in self.notes}), ["schedule"])
        self.assertTrue(all(n["page"] for n in self.notes))
        self.assertTrue(by)


class Annex3A(unittest.TestCase):
    """Annex 3-A quoting and the Annex 3-B Section/Chapter note cells."""

    def test_quote_chunk_is_verbatim_between_anchors_without_footer(self):
        page = "Note 3 Application 1. One. 2. Two. EU/JP/Annex 3-A/en 4"
        self.assertEqual(b.quote_chunk(page, "Note 3", None), "Note 3 Application 1. One. 2. Two.")
        self.assertEqual(b.quote_chunk(page, "1. One.", "2. Two."), "1. One. 2. Two.")
        self.assertEqual(b.quote_chunk("Text. ________________ EU/JP/Annex 3-A/en 18", None, None), "Text.")

    def test_quote_chunk_needs_a_unique_start(self):
        with self.assertRaises(ValueError):
            b.quote_chunk("a b a b", "a", None)
        with self.assertRaises(ValueError):
            b.quote_chunk("a b", "zzz", None)

    def test_formula_page_must_keep_its_text_layer(self):
        end = ("apply:", " F = x", "layer")
        self.assertEqual(b.quote_chunk("1. apply: layer", None, end), "1. apply: F = x")
        with self.assertRaises(ValueError):
            b.quote_chunk("1. apply: other layer", None, end)

    def test_every_annex_note_is_page_pinned_and_scoped(self):
        for filename, spec in b.ANNEX_3A.items():
            self.assertIn(spec["pdf"], b.DIGESTS)
            self.assertIn(filename, b.DIGESTS)
            for page, label, start, end, scope, key, applies in spec["notes"]:
                self.assertGreaterEqual(page, 1)
                self.assertTrue(label.startswith("Note "))
                self.assertIn((scope, key), (("schedule", ""), ("section", "XI")))
                for lo, hi in applies:
                    self.assertTrue(lo.isdigit() and len(lo) == len(hi) and lo <= hi)
            pages = [n[0] for n in spec["notes"]]
            self.assertEqual(pages, sorted(pages))

    def test_cell_notes_are_captured_with_a_reviewed_scope(self):
        pdf = TablePdf([
            ("SECTION V", "MINERAL PRODUCTS Section note: See Note 5 of Annex 3-A."),
            ("Chapter 57", "Carpets Chapter note: Jute may be a backing."),
            ("57.01", "Rule.")])
        for filename, page in (("eu3.pdf", 12), ("uk3.pdf", 14)):
            notes = []
            b.cell_note(notes, filename, "section", "V", "MINERAL PRODUCTS Section note: See.", page)
            self.assertEqual(notes, [{"scope": "section", "key": "V", "text": "Section note: See.", "page": page}])
            b.scope_numbered_notes(filename, notes)
            self.assertNotIn("applies", notes[0])
        rows, notes = b.uk(pdf)
        self.assertEqual([(n["scope"], n["key"], n["text"][:14]) for n in notes],
                         [("section", "V", "Section note: "), ("chapter", "57", "Chapter note: ")])

    def test_cell_note_without_an_applicability_entry_fails_the_build(self):
        with self.assertRaises(ValueError):
            b.scope_numbered_notes("eu3.pdf", [{"text": "Section note: x", "page": 13, "key": "V"}])

    def test_other_pdfs_do_not_take_cell_notes(self):
        notes = []
        b.cell_note(notes, "tpp2.pdf", "section", "V", "x Section note: y", 1)
        self.assertEqual(notes, [])


class GeneralNotes(unittest.TestCase):
    """General Notes of the text-layer Annexes (build_psr.GENERAL_NOTES)."""

    TARGETS = {
        "thailand2-2.pdf": 7, "indonesia2-2.pdf": 13, "malaysia2.pdf": 4, "philippine2.pdf": 5,
        "mexico2.pdf": 5, "chile2.pdf": 4, "vietnam2.pdf": 3, "switzerland3.pdf": 2,
        "tpp2.pdf": 1, "tpp5.pdf": 1,
    }

    def test_quote_general_is_verbatim_between_unique_anchors(self):
        page = "Intro (a) one; (b) two; (c) three. 7"
        self.assertEqual(b.quote_general(page, "(b) two;", "two;"), "(b) two;")
        self.assertEqual(b.quote_general(page, "(a) one;", "three."), "(a) one; (b) two; (c) three.")

    def test_quote_general_rejects_ambiguous_or_missing_anchors(self):
        with self.assertRaises(ValueError):
            b.quote_general("x; y; x; y;", "x;", "y;")  # start twice
        with self.assertRaises(ValueError):
            b.quote_general("a b c b", "a", "b")  # end twice after the start
        with self.assertRaises(ValueError):
            b.quote_general("a b", "a", "zzz")
        with self.assertRaises(ValueError):
            b.quote_general("end a", "a", "end")  # end before the start

    def test_every_target_has_a_pinned_reviewed_spec(self):
        self.assertEqual({f: len(v["notes"]) for f, v in b.GENERAL_NOTES.items()}, self.TARGETS)
        for filename, spec in b.GENERAL_NOTES.items():
            self.assertIn(filename, b.DIGESTS)
            self.assertTrue(spec["label"].endswith(("General Notes", "General Interpretative Notes", "Annex 2", "Introductory Notes")))
            keys = [(page, label) for page, label, *_ in spec["notes"]]
            self.assertEqual(len(keys), len(set(keys)), "labels are unique per page")
            pages = [n[0] for n in spec["notes"]]
            self.assertEqual(pages, sorted(pages))
            for page, label, start, end, applies in spec["notes"]:
                self.assertGreaterEqual(page, 1)
                self.assertTrue(start and end and label)
                for lo, hi in applies:
                    self.assertTrue(lo.isdigit() and len(lo) == len(hi) and 2 <= len(lo) <= 6 and lo <= hi)

    def test_general_notes_are_not_attached_to_other_pdfs(self):
        for filename in b.ANNEX_3A:
            self.assertNotIn(filename, b.GENERAL_NOTES)
        self.assertNotIn("australia2.pdf", b.GENERAL_NOTES)


@unittest.skipUnless(os.environ.get("JP_PDF_DIR"), "set JP_PDF_DIR to the directory holding the Annex PDFs")
class GeneralNotesPdfTruth(unittest.TestCase):
    """Pinned to the official PDFs (digest checked): text, page, citation, source."""

    def test_every_note_is_a_verbatim_page_chunk_of_its_own_pdf(self):
        import hashlib
        for filename, count in GeneralNotes.TARGETS.items():
            path = os.path.join(os.environ["JP_PDF_DIR"], filename)
            with open(path, "rb") as f:
                self.assertEqual(hashlib.sha256(f.read()).hexdigest(), b.DIGESTS[filename], filename)
            notes = b.general_notes(path, filename)
            spec = b.GENERAL_NOTES[filename]
            self.assertEqual(len(notes), count, filename)
            for note, (page, label, start, end, applies) in zip(notes, spec["notes"]):
                self.assertTrue(note["text"].startswith(start) and note["text"].endswith(end), (filename, label))
                self.assertEqual((note["scope"], note["key"], note["page"]), ("schedule", "", page))
                self.assertEqual(note["source_url"], b.URLS.get(filename, b.BASE + filename))
                self.assertEqual(note["citation"], f"{spec['label']}, {label}")
                self.assertEqual(note.get("applies", []), [list(a) for a in applies])
                self.assertNotIn("  ", note["text"])


class Chapter3Articles(unittest.TestCase):
    """Chapter 3 articles of the JP-EU EPA, JP-UK CEPA and CPTPP."""

    def test_sources_are_pinned_and_paired_with_their_schedule(self):
        self.assertEqual(b.DIGESTS["eu1.pdf"][:8], "a71de5d6")
        self.assertEqual(b.DIGESTS["uk1.pdf"][:8], "277066a8")
        self.assertEqual(b.DIGESTS["tpp1.pdf"][:8], "da772daa")
        self.assertEqual({k: v["pdf"] for k, v in b.CHAPTER_3.items()},
                         {"eu3.pdf": "eu1.pdf", "uk3.pdf": "uk1.pdf", "tpp2.pdf": "tpp1.pdf"})

    def test_only_listed_articles_and_sorted_pages(self):
        for filename, spec in b.CHAPTER_3.items():
            pages = [n[0] for n in spec["notes"]]
            self.assertEqual(pages, sorted(pages))
            cptpp = filename == "tpp2.pdf"
            for page, label, start, end, wholly in spec["notes"]:
                if cptpp:
                    # Articles 3.2, 3.6, 3.7, 3.10 to 3.17 and Annex 3-C (D916)
                    self.assertRegex(label, r"^(Articles? 3\.(2|6|7|1[0-7])\b|Annex 3-C)", label)
                    self.assertTrue(wholly is False or label.startswith("Annex 3-C"), label)
                    continue
                self.assertTrue(label.startswith(("Article 3.", "Articles 3.")), label)
                # Articles 3.1(e), (f), (h), (i), 3.2 to 3.9 and 3.12 to 3.15 (3.7 to 3.9 and 3.13 to 3.15 since D916)
                self.assertRegex(label, r"^Articles? 3\.(1\(|2|3|4|5|6|7|8|9|12|13|14|15)\b")
                self.assertIsInstance(wholly, bool)
                for anchor in (start, end):
                    if anchor:
                        self.assertNotRegex(anchor, r"ARTICLE 3\.(1[0-1]|1[6-9]|2\d)\b")
            # the Article 3.3 and 3.6(3) chunks are the only ones scoped to wholly obtained rows
            for page, label, start, end, wholly in spec["notes"]:
                if not cptpp:
                    self.assertEqual(wholly, label.startswith("Article 3.3") or label == "Article 3.6 paragraph 3", label)

    def test_wholly_obtained_ranges_join_only_touching_equal_width_ranges(self):
        rows = [{"start": s, "end": e, "text": t} for s, e, t in [
            ("0101", "0106", "All animals of Chapter 1 are wholly obtained."),
            ("0107", "0108", "x is wholly obtained"),
            ("0109", "0109", "CC"),
            ("0110", "0110", "wholly obtained"),
            ("090210", "090220", "wholly obtained"),
            ("090221", "090221", "wholly obtained"),
            ("03", "03", "wholly obtained"), ("03", "03", "All fish are wholly obtained."),
            ("04", "04", "CC"),
        ]]
        self.assertEqual(b.wholly_obtained_ranges(rows), [["0101", "0108"], ["0110", "0110"], ["03", "03"], ["090210", "090221"]])

    def test_caveat_is_an_unde_labs_note_with_the_required_statements(self):
        for filename, spec in b.CHAPTER_3.items():
            text = b.chapter_3_caveat(spec)
            self.assertTrue(text.startswith("Unde Labs note (not treaty text)"))
            if filename == "tpp2.pdf":
                for frag in ("Articles 3.2, 3.6, 3.7, 3.10 to 3.17 and Annex 3-C", "Annex 3-D product-specific rule",
                             "do not replace the listed rule", "do not themselves establish origin", "3.18 to 3.32",
                             "NOT supplied", "not verified", "undetermined"):
                    self.assertIn(frag, text, filename)
                continue
            if filename == "uk3.pdf":
                self.assertIn("Article 3.5 paragraphs 10 to 12", text)
            for frag in ("do not replace the listed rule", "do not themselves establish origin", "Article 3.2 paragraph 1(b)",
                         "only partly quoted", "Note 1 paragraphs 1, 2 and 4", "Note 2 paragraph 4",
                         "Articles 3.10 and 3.11", "Annex 3-C", "Appendix 3-B-1", spec["procedural"],
                         "NOT supplied", "not verified", "undetermined"):
                self.assertIn(frag, text, filename)

    def test_page_footer_of_chapter_3_is_cut_from_a_page_edge(self):
        self.assertEqual(b.quote_chunk("A text. EU/JP/en 61", "A", None), "A text.")
        self.assertEqual(b.quote_chunk("A text. UK/JP/en 7", "A", None), "A text.")


@unittest.skipUnless(os.environ.get("JP_PDF_DIR"), "set JP_PDF_DIR to the directory holding eu1.pdf, uk1.pdf and tpp1.pdf")
class Chapter3PdfTruth(unittest.TestCase):
    """Pinned to the official PDFs (digest checked): verbatim page chunks."""

    def test_every_note_is_a_verbatim_page_chunk(self):
        import hashlib
        for filename, spec in b.CHAPTER_3.items():
            path = os.path.join(os.environ["JP_PDF_DIR"], spec["pdf"])
            with open(path, "rb") as f:
                self.assertEqual(hashlib.sha256(f.read()).hexdigest(), b.DIGESTS[spec["pdf"]])
            rows = [{"start": f"{n:04d}", "end": f"{n:04d}", "text": "wholly obtained"} for n in range(101, 123, 2)]
            notes = b.chapter_3_notes(path, filename, rows)
            self.assertEqual(len(notes), len(spec["notes"]) + 1)
            self.assertEqual((notes[0]["page"], notes[0]["scope"]), (0, "schedule"))
            for note, (page, label, start, end, wholly) in zip(notes[1:], spec["notes"]):
                self.assertEqual((note["scope"], note["page"], note["source_url"]), ("schedule", page, b.BASE + spec["pdf"]))
                self.assertEqual(note["citation"], f"{spec['label']}, {label}")
                self.assertEqual("applies" in note, bool(wholly))
                self.assertNotIn("  ", note["text"])
                self.assertNotRegex(note["text"], r"(EU|UK)/JP/en|Working Group on Wine|ARTICLE 4\.1|3-\d? ?\d$")


if __name__ == "__main__":
    unittest.main()
