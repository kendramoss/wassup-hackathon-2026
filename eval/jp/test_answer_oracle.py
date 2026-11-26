"""Tests for answer_oracle.py. Run: python3 -I -B eval/jp/test_answer_oracle.py"""
import os, sys, unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import answer_oracle as o  # noqa: E402

RULE = "非原産材料を使用してEUにおいて生産される関税率表第04.06項に分類される産品が、協定上の原産品と認められるためには、CTHを満たさなければならない。本品は完全生産品の規定を満たす。"


class Parse(unittest.TestCase):
    def test_all_non(self):
        self.assertEqual(o.parse("上記原材料は全て非原産材料として扱う。" + RULE, 3), {1: o.NON, 2: o.NON, 3: o.NON})

    def test_some_originating_rest_non(self):
        r = "照会者から提出された情報により、上記原材料のうち②及び④については日EU経済連携協定（以下「協定」という。）上のEU原産材料と認められる。その他の原材料については全て非原産材料として扱う。" + RULE
        self.assertEqual(o.parse(r, 4), {1: o.NON, 2: o.ORIG, 3: o.NON, 4: o.ORIG})

    def test_range_and_wholly_obtained(self):
        r = "上記原材料のうち①から③までについては日EU経済連携協定上のEUの完全生産品と認められる。その他の原材料については非原産材料として扱う。" + RULE
        self.assertEqual(o.parse(r, 4), {1: o.WO, 2: o.WO, 3: o.WO, 4: o.NON})

    def test_two_clauses_and_neutral(self):
        r = "上記原材料のうち①から③については非原産材料として扱い、④及び⑤については日EU経済連携協定（以下「協定」という。）上の中立的な要素と認められることから、原産品としての資格については決定する必要はない。" + RULE
        self.assertEqual(o.parse(r, 5), {1: o.NON, 2: o.NON, 3: o.NON, 4: o.NEUTRAL, 5: o.NEUTRAL})

    def test_other_in_same_sentence(self):
        r = "上記原材料のうち②については協定上のEU原産材料と認められ、その他の原材料については全て非原産材料として扱う。" + RULE
        self.assertEqual(o.parse(r, 3), {1: o.NON, 2: o.ORIG, 3: o.NON})

    def test_never_reads_the_rule(self):
        # The rule sentence mentions 完全生産品 and 原産品; only the status
        # sentence before it may be read.
        self.assertEqual(o.parse(RULE, 2), None)

    def test_unreadable_is_none(self):
        self.assertIsNone(o.parse("上記原材料のうち①については審査中である。" + RULE, 2))
        self.assertIsNone(o.parse("上記原材料のうち①については協定上の原産材料と認められる。" + RULE, 2))  # ② left unknown
        self.assertIsNone(o.parse("上記原材料のうち⑤については協定上の原産材料と認められる。" + RULE, 2))  # beyond the list


class Materials(unittest.TestCase):
    def test_nested_reference_stays_in_item(self):
        m = o.materials("① Hinge pin (7318.15) (raw materials of ①: iron bars (Chapter 72)), ② Plate (Chapter 73)")
        self.assertEqual(sorted(m), [1, 2])
        self.assertIn("iron bars", m[1])

    def test_answer_text(self):
        a = o.answer({1: o.WO, 2: o.NON}, {1: "① Raw milk (04.01)", 2: "② Salt (25.01)"})
        self.assertEqual(a, "These materials have no proof of origin: ② Salt (25.01). "
                            "These materials are wholly obtained, with proof of origin under the agreement: ① Raw milk (04.01).")
        self.assertEqual(o.answer({1: o.ORIG}, {1: "① A"}), "Every material has proof of origin. These materials have proof of origin under the agreement: ① A.")


if __name__ == "__main__":
    unittest.main()
