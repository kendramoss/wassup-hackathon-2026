"""Unit tests for the pure helpers of build_psr.py (no PDFs needed).

Run from this directory: python3 -m unittest test_build_psr
"""
import os
import unittest

try:
    import build_psr as b
except ImportError as error:  # pdfplumber missing
    raise unittest.SkipTest(str(error))


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
        self.assertEqual(len(self.notes), 1 + 1 + 17 + 6 + 1 + 1)
        header = [n for n in self.notes if n["page"] == 11]
        self.assertEqual(header[0]["text"], "Note: Goods which satisfy the applicable rules set out in column 3 or column 4 below shall be considered as originating goods of a Party.")
        self.assertEqual(sorted({n["scope"] for n in self.notes}), ["schedule"])
        self.assertTrue(all(n["page"] for n in self.notes))
        self.assertTrue(by)


if __name__ == "__main__":
    unittest.main()
