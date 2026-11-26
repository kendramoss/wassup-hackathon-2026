"""Tests for refresh.py. Run: python3 -I -B eval/jp/test_refresh.py"""
import json, os, sys, tempfile, unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import refresh as r  # noqa: E402


def ruling(i, **kw):
    base = {"id": i, "date": "2026-11-20", "agreement": "RCEP", "hs": "8501.10", "product_ja": "モーター",
            "reasoning_ja": "上記原材料は全て非原産材料として扱う。", "answer_ja": "原産品と認められる。"}
    base.update(kw)
    return base


class Refresh(unittest.TestCase):
    def test_compare(self):
        new, gone = r.compare([ruling("3"), ruling("1")], {"1", "2"})
        self.assertEqual([x["id"] for x in new], ["3"])
        self.assertEqual(gone, ["2"])

    def test_check(self):
        self.assertEqual(r.check(ruling("1")), [])
        self.assertIn("missing reasoning_ja", r.check(ruling("1", reasoning_ja="")))
        self.assertIn("date is not YYYY-MM-DD", r.check(ruling("1", date="2026/11/20")))

    def test_report(self):
        new = [ruling("3"), ruling("4", agreement="CPTPP", date="2026-11-25"), ruling("5", hs="")]
        rep = r.report("2026-12-01", [ruling("1")] + new, new, ["2"], {"list_total": 4})
        self.assertEqual((rep["new"], rep["valid"], rep["withdrawn_from_list"]), (3, 2, 1))
        self.assertEqual(rep["agreements"], {"CPTPP": 1, "RCEP": 1})
        self.assertEqual((rep["date_min"], rep["date_max"]), ("2026-11-20", "2026-11-25"))
        self.assertFalse(rep["items"][2]["valid"])

    def test_known_ids_reads_fresh_dirs(self):
        with tempfile.TemporaryDirectory() as repo:
            os.makedirs(os.path.join(repo, "data/jp/fresh-1126"))
            for p, ids in (("data/jp/rulings.jsonl", ["1"]), ("data/jp/fresh-1126/rulings.jsonl", ["2"])):
                with open(os.path.join(repo, p), "w") as f:
                    f.writelines(json.dumps({"id": i}) + "\n" for i in ids)
            self.assertEqual(r.known_ids(repo), {"1", "2"})


if __name__ == "__main__":
    unittest.main()
