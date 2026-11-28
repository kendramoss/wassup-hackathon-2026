#!/usr/bin/env python3
"""Builds the post-hoc supplement after the first reserved test run.

The reserved test cases that errored because their input was refused for a
dataset authoring reason are rebuilt with the refused field removed and scored
separately. They never enter the reserved test totals.
"""
import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
from oracle import evaluate  # noqa: E402
from rules import Tables  # noqa: E402
from generate import clean, normalize  # noqa: E402


def main(feature):
    tables = Tables()
    res = json.load(open(os.path.join(HERE, "results", feature + "-test.json")))
    test = json.load(open(os.path.join(HERE, feature + "-test.json")))
    by_id = {c["id"]: c for c in test["cases"]}
    out = []
    for r in res["results"]:
        if r["status"] != "error":
            continue
        c = json.loads(json.dumps(by_id[r["id"]]))
        inp = c["input"]
        inp.pop("method", None)
        row = tables.resolve(feature, "".join(ch for ch in inp["hs"] if ch.isdigit())[:6])
        exp = evaluate(feature, clean(inp), row.rule())
        e = {"outcome": exp["outcome"]}
        plain = {k: v for k, v in exp.get("alternative", {}).items() if k.startswith("RVC") and v != "undetermined"}
        if plain:
            e["alternative"] = plain
        if "accumulation" in exp:
            e["accumulation"] = exp["accumulation"]
        c["id"] = r["id"].replace("-test-", "-posthoc-")
        c["split"] = "posthoc"
        c["parent_id"] = r["id"]
        c["scenario_family"] = "authority-variants-corrected"
        c["variation_tags"] = sorted(set(c["variation_tags"]) | {"post-hoc"})
        c["rationale"] = "Post-hoc supplement: reserved case %s errored because its input carried method direct, which ACFTA refuses (its pre-registered parent input does too, D950); same facts without the field. %s" % (r["id"], c["rationale"])
        c["expect"] = e
        c["expected_abstention"] = e["outcome"] == "undetermined"
        out.append(c)
    doc = {k: test[k] for k in ("schema_version", "dataset_version", "evidence_kind", "feature", "sources")}
    doc.update(split="posthoc", about="Post-hoc supplement written after the first reserved test run; not part of the reserved totals. Labels from the unchanged oracle.", label_method=test["label_method"], case_count=len(out), cases=out)
    with open(os.path.join(HERE, feature + "-posthoc.json"), "w") as fh:
        json.dump(doc, fh, indent=1, ensure_ascii=False)
        fh.write("\n")
    print(feature, len(out))


if __name__ == "__main__":
    main(sys.argv[1])
