"""Tests for summarize.py. Run: python3 -I -B eval/rescore/test_summarize.py"""
import json, os, sys, tempfile, unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import summarize as s  # noqa: E402


class Summarize(unittest.TestCase):
    def test_wilson(self):
        self.assertEqual(s.wilson(77, 83), [0.8511, 0.9665])  # D154's interval
        self.assertIsNone(s.wilson(0, 0))

    def _repo(self, monthly, runlog):
        repo = tempfile.mkdtemp()
        os.makedirs(os.path.join(repo, "eval/rescore/2026-12"))
        os.makedirs(os.path.join(repo, "eval/results"))
        json.dump({"monthly": monthly, "date_min": None, "date_max": None},
                  open(os.path.join(repo, "eval/rescore/2026-12/splits.json"), "w"))
        with open(os.path.join(repo, "eval/results/RUNLOG.jsonl"), "w") as f:
            f.writelines(json.dumps(r) + "\n" for r in runlog)
        return repo

    def test_empty_month(self):
        out = s.build(self._repo([], []), "2026-12")
        self.assertEqual(out["cbp"]["cases"], 0)
        self.assertIsNone(out["cbp"]["accuracy"])
        self.assertEqual(out["japan"]["cases"], 0)

    def test_scored_month(self):
        run = {"split": "monthly", "run": "monthly-gemini-4-argon-prec-engine-v1-litellm-router-rescore-2026-12", "correct": 9, "cases": 10}
        other = {"split": "monthly", "run": "x-rescore-2026-11", "correct": 1, "cases": 1}
        out = s.build(self._repo(["N1"] * 10, [run, other]), "2026-12")
        self.assertEqual((out["cbp"]["correct"], out["cbp"]["cases"], out["cbp"]["accuracy"]), (9, 10, 0.9))

    def test_needs_one_run(self):
        with self.assertRaises(SystemExit):
            s.build(self._repo(["N1"], []), "2026-12")


if __name__ == "__main__":
    unittest.main()
