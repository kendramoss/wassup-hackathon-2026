import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('verifier_summary', Path(__file__).with_name('summarize.py'))
summary = importlib.util.module_from_spec(spec)
spec.loader.exec_module(summary)


class IncompleteControlScoring(unittest.TestCase):
    def test_error_is_not_a_correct_withhold(self):
        cases = [
            {'id': 'a', 'expected': {'changes_origin': None}},
            {'id': 'b', 'expected': {'changes_origin': None}},
            {'id': 'c', 'expected': {'changes_origin': None}},
            {'id': 'd', 'expected': {'changes_origin': True}},
        ]
        rows = [
            {'id': 'a', 'shown': False},
            {'id': 'b', 'shown': False, 'error': 'model_error'},
            {'id': 'c', 'shown': True},
            {'id': 'd', 'shown': True},
        ]
        self.assertEqual(summary.incomplete_control_score(cases, rows), {'num': 1, 'den': 3})

    def test_every_incomplete_case_must_have_a_result(self):
        cases = [{'id': 'a', 'expected': {'changes_origin': None}}]
        with self.assertRaises(ValueError):
            summary.incomplete_control_score(cases, [])


if __name__ == '__main__':
    unittest.main()
