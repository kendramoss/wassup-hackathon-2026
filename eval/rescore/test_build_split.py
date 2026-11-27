"""Tests for build_split.py. Run: python3 -I -B eval/rescore/test_build_split.py"""
import json, os, sys, unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import build_split as b  # noqa: E402

ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..")


class Select(unittest.TestCase):
    def test_reproduces_final2(self):
        # The monthly rule is D150's: on the final2 labels it gives the frozen final2 split.
        labels = b.rows(os.path.join(ROOT, "eval/data/labels_final2.jsonl"))
        want = json.load(open(os.path.join(ROOT, "eval/splits/splits_final2.json")))
        keep, steps = b.select(labels, lambda i: os.path.exists(os.path.join(ROOT, "eval/data/facts_final2", i + ".txt")))
        self.assertEqual([r["ruling"] for r in keep], want["final2"])
        self.assertEqual(dict(steps), want["funnel"])

    def test_rule(self):
        L = lambda i, **k: {"ruling": i, "date": "2026-12-0" + i[-1], "regime": "substantial_transformation",
                            "multi_country": True, "primary_origin": "China", **k}
        labels = [L("N1"), L("N2", primary_origin=None), L("N3", primary_origin="China | Vietnam"),
                  L("N4", multi_country=False), L("N5", regime="usmca"), L("N6")]
        keep, steps = b.select(labels, lambda i: i != "N6")
        self.assertEqual([r["ruling"] for r in keep], ["N1"])
        self.assertEqual(steps["without_facts_file"], 1)
        self.assertEqual(steps["excluded_two_or_more_countries_held"], 1)


if __name__ == "__main__":
    unittest.main()
