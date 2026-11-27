"""Checks that screen-prompts.json (read by the Go scope-first check, D959) holds the prompt label.py ran the name screen with: python3 -I -B eval/adcvd/real/prompts_test.py"""
import json
import os
import sys
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
sys.path.insert(0, os.path.dirname(HERE))
import label  # noqa: E402


class ScreenPrompt(unittest.TestCase):
    def test_same_text(self):
        d = json.load(open(os.path.join(HERE, "screen-prompts.json")))
        self.assertEqual(d["version"], label.SCREEN_VERSION)
        self.assertEqual(d["system"], label.SCREEN_SYSTEM)
        self.assertEqual(d["user"], label.SCREEN_USER)


if __name__ == "__main__":
    unittest.main()
