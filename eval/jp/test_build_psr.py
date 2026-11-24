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


if __name__ == "__main__":
    unittest.main()
