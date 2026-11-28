import importlib.util
import pathlib
import unittest

HERE = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("study", HERE / "study.py")
study = importlib.util.module_from_spec(spec)
spec.loader.exec_module(study)


class Scoring(unittest.TestCase):
    def case(self):
        return {"id": "opaque", "components": [{"name": "motor", "country": "China"}, {"name": "cord", "country": ""}]}

    def result(self):
        return {"provider": "litellm", "scores": [{"read": [{"name": "motor", "country": "China"}], "warnings": ["The file mentions cord without saying where it is made; add it if it should count."], "simulated": False, "cost_usd_gateway": .01, "latency_ms": 100}]}

    def test_unknown_needs_withholding_and_missing_country_evidence(self):
        row = study.score(self.case(), self.result())
        self.assertTrue(row["matched"])
        self.assertEqual(row["names_read"], 2)
        self.assertEqual(row["unknown_withheld"], 1)
        result = self.result()
        result["scores"][0]["warnings"] = []
        self.assertFalse(study.score(self.case(), result)["matched"])

    def test_invented_country_and_duplicate_rows_cannot_pass(self):
        for extra in [{"name": "cord", "country": "Japan"}, {"name": "motor", "country": "China"}, {"name": "screw", "country": "China"}]:
            result = self.result()
            result["scores"][0]["read"].append(extra)
            self.assertFalse(study.score(self.case(), result)["matched"])

    def test_country_aliases_do_not_change_the_facts(self):
        result = self.result()
        result["scores"][0]["read"][0]["country"] = "CN"
        self.assertTrue(study.score(self.case(), result)["matched"])

    def test_error_is_not_a_successful_refusal(self):
        result = self.result()
        result["scores"][0]["error"] = "model error"
        row = study.score(self.case(), result)
        self.assertFalse(row["matched"])
        self.assertTrue(row["error"])
        self.assertEqual(row["unknown_withheld"], 0)

    def test_simulated_provider_cannot_be_scored_as_measured(self):
        for provider in ["fake", "vertex", ""]:
            result = self.result()
            result["provider"] = provider
            with self.assertRaises(ValueError):
                study.score(self.case(), result)

    def test_missing_or_duplicate_documents_invalidate_summary(self):
        case = self.case()
        row = study.score(case, self.result())
        with self.assertRaises(ValueError):
            study.aggregate([case], [])
        with self.assertRaises(ValueError):
            study.aggregate([case], [row, row])

    def test_name_substrings_are_not_matches(self):
        result = self.result()
        result["scores"][0]["read"][0]["name"] = "motor bracket"
        self.assertFalse(study.score(self.case(), result)["matched"])


class OutputProvenance(unittest.TestCase):
    def test_stored_rows_reject_wrong_image_candidate_dataset_and_cost(self):
        import json
        import hashlib
        import copy
        dataset_bytes = (HERE / "cases.json").read_bytes()
        case = next(c for c in json.loads(dataset_bytes)["cases"] if c["split"] == "test")
        result = json.loads((HERE / "results/test" / (case["id"] + ".json")).read_text())
        candidate = "dd158f8a94d3ec803519b5adc926e6fc3e4bd4e9"
        digest = hashlib.sha256(dataset_bytes).hexdigest()
        study.validate_output(case, result, candidate, digest)
        for mutate in [
            lambda r: r.update(image_sha256="0" * 64),
            lambda r: r.update(candidate_revision="0" * 40),
            lambda r: r.update(dataset_sha256="0" * 64),
            lambda r: r["scores"][0].update(image="wrong.png"),
            lambda r: r["scores"][0].update(cost_usd_gateway=-1),
            lambda r: r["scores"][0].update(prompt_version="wrong"),
            lambda r: r.update(model="wrong"),
            lambda r: r["scores"][0].update(simulated=True),
        ]:
            changed = copy.deepcopy(result)
            mutate(changed)
            with self.assertRaises(ValueError):
                study.validate_output(case, changed, candidate, digest)


class Integrity(unittest.TestCase):
    def runner(self):
        spec = importlib.util.spec_from_file_location("runner", HERE / "run.py")
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module

    def test_stored_images_sources_and_split_hashes_validate(self):
        import json
        dataset = json.loads((HERE / "cases.json").read_text())
        self.runner().validate(dataset, "a" * 40)
        self.assertEqual(sum(c["split"] == "test" for c in dataset["cases"]), 132)
        self.assertEqual(sum(c["split"] == "dev" for c in dataset["cases"]), 24)

    def test_changed_hash_or_duplicate_document_is_rejected(self):
        import json
        for mutate in [lambda d: d["cases"][0].update(image_sha256="0" * 64), lambda d: d["cases"].append(d["cases"][0])]:
            dataset = json.loads((HERE / "cases.json").read_text())
            mutate(dataset)
            with self.assertRaises(ValueError):
                self.runner().validate(dataset, "a" * 40)

    def test_shared_source_family_is_rejected(self):
        import json
        dataset = json.loads((HERE / "cases.json").read_text())
        dev = next(c for c in dataset["cases"] if c["split"] == "dev")
        test = next(c for c in dataset["cases"] if c["split"] == "test")
        test["source_family"] = dev["source_family"]
        with self.assertRaises(ValueError):
            self.runner().validate(dataset, "a" * 40)


if __name__ == "__main__":
    unittest.main()
