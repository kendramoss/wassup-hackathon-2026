"""Unit tests for the pure helpers of build_psr.py (no PDFs needed).

Run from this directory: python3 -m unittest test_build_psr
"""
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


if __name__ == "__main__":
    unittest.main()
