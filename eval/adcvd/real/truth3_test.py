"""Tests of the pair rule of truth3.py (D959): python3 -I -B -m unittest eval/adcvd/real/truth3_test.py (from the repository root, with the directory on the path) or run this file."""
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from truth3 import pair_verdict  # noqa: E402


class PairRule(unittest.TestCase):
    def test_nodissent(self):
        r = "nodissent"
        self.assertEqual(pair_verdict(["in", "in", "in"], r), "in")
        self.assertEqual(pair_verdict(["in", "unclear", "in"], r), "in")
        self.assertEqual(pair_verdict(["in", "in", "out"], r), "contested")
        self.assertEqual(pair_verdict(["out", "unclear", "out"], r), "out")
        self.assertEqual(pair_verdict(["out", "out", "in"], r), "contested")
        self.assertEqual(pair_verdict(["in", "unclear", "unclear"], r), "contested")
        self.assertEqual(pair_verdict(["unclear", "unclear", "unclear"], r), "contested")
        self.assertEqual(pair_verdict(["in", "out", "unclear"], r), "contested")
        self.assertEqual(pair_verdict(["in", "in", "missing"], r), "in")
        self.assertEqual(pair_verdict(["out", "missing", "missing"], r), "contested")

    def test_all(self):
        self.assertEqual(pair_verdict(["in", "in", "in"], "all"), "in")
        self.assertEqual(pair_verdict(["in", "unclear", "in"], "all"), "contested")
        self.assertEqual(pair_verdict(["out", "out", "out"], "all"), "out")


if __name__ == "__main__":
    unittest.main()
