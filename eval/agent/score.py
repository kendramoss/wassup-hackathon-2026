"""Scores the ADK agent evaluation runs (D450). No model calls, no network.

Reads eval/agent/products.jsonl and every eval/agent/results/*.jsonl, scores
each run with the rules pre-registered in docs/DECISIONS.md D450, and writes
eval/agent/summary.json and eval/agent/SUMMARY.md.

    python3 -I -B eval/agent/score.py .          # write
    python3 -I -B eval/agent/score.py . --check  # verify (make reproduce)

Checks on the set itself: every product's ruling is in the dev split of
eval/splits/splits_final.json, none is in a held-out split (final, final2,
pilot), and cbp_origin equals labels.jsonl's primary_origin.
"""
import glob
import json
import os
import re
import statistics
import sys

REPO = sys.argv[1]
CHECK = "--check" in sys.argv
R = lambda p: os.path.join(REPO, p)
OUT_JSON, OUT_MD = "eval/agent/summary.json", "eval/agent/SUMMARY.md"

OFFERED = {"extract_product", "predict_origin", "plan_production_moves", "export_ruling_request_draft", "suggest_part_hts", "get_ruling"}
ORDER = ["extract_product", "predict_origin", "plan_production_moves", "export_ruling_request_draft"]
DISCLAIMER = "Not a ruling — confirm with a licensed customs broker."
RULING = re.compile(r"\b(?:[HN]\d{6}|NY [A-Z]?\d{5,6}|HQ \d{6})\b")
ORIGIN_SENTENCE = re.compile(r"(predict|likely|expected|would)\w*[^.\n]{0,60}?\borigin\b|\borigin\b[^.\n]{0,20}\b(is|would be|will be|:)", re.I)
ALIASES = {"Vietnam": ["Viet Nam"], "South Korea": ["Korea"], "United States": ["U.S.A.", "USA", "United States of America"],
           "China": ["PRC", "People's Republic of China"], "Taiwan": [], "Thailand": ["Thai"], "Malaysia": [], "Cambodia": [],
           "Indonesia": [], "Mexico": [], "India": [], "Japan": [], "Germany": []}


def jl(path):
    return [json.loads(l) for l in open(path) if l.strip()]


def names(country):
    return [country] + ALIASES.get(country, [])


def mentions(text, country):
    return any(re.search(r"\b" + re.escape(n) + r"\b", text) for n in names(country))


def countries_in(text):
    return {c for c in ALIASES if mentions(text, c)}


def norm_rulings(text):
    return {m.replace("NY ", "N").replace("HQ ", "H") for m in RULING.findall(text)}


def check_set(products):
    splits = json.load(open(R("eval/splits/splits_final.json")))
    dev = set(splits["dev"])
    held = set(splits["final"]) | set(splits.get("pilot", []))
    f2 = R("eval/splits/splits_final2.json")
    if os.path.exists(f2):
        s2 = json.load(open(f2))
        for k, v in s2.items():
            if isinstance(v, list) and k not in ("source", "rule"):
                held |= {x for x in v if isinstance(x, str)}
    labels = {r["ruling"]: r for r in jl(R("eval/data/labels.jsonl"))}
    problems = []
    for p in products:
        if p["ruling"] not in dev:
            problems.append(f"{p['id']}: {p['ruling']} not in the dev split")
        if p["ruling"] in held:
            problems.append(f"{p['id']}: {p['ruling']} is in a held-out split")
        if labels.get(p["ruling"], {}).get("primary_origin") != p["cbp_origin"]:
            problems.append(f"{p['id']}: cbp_origin differs from labels.jsonl")
    return problems


def score_run(row, prod):
    calls = row["calls"]
    seq = [c["tool"] for c in calls]
    err = lambda c: bool(c.get("error"))
    first = {t: seq.index(t) for t in ORDER if t in seq}
    pred = next((c for c in calls if c["tool"] == "predict_origin" and not err(c)), None)
    pf = (pred or {}).get("fields") or {}
    plans_ok = [c for c in calls if c["tool"] == "plan_production_moves" and not err(c)]
    shown = max([(c.get("fields") or {}).get("plans", 0) for c in plans_ok] or [0])

    # Tool order (D450 rule 1).
    required = ORDER[:3] + (ORDER[3:] if shown > 0 else [])
    issues = []
    for t in required:
        if t not in seq:
            issues.append(f"missing {t}")
    pos = [first[t] for t in ORDER if t in first]
    if pos != sorted(pos):
        issues.append("required tools out of order")
    for c in calls:
        if c["tool"] not in OFFERED:
            issues.append(f"forbidden tool {c['tool']}")
    if seq.count("predict_origin") > 1:
        issues.append("predict_origin called more than once (a second case)")
    if "suggest_part_hts" in seq and ("extract_product" not in first or seq.index("suggest_part_hts") < first["extract_product"]):
        issues.append("suggest_part_hts before extract_product")
    plan_calls = [c for c in calls if c["tool"] == "plan_production_moves"]
    for a, b in zip(plan_calls, plan_calls[1:]):
        if not err(a) or a["args"].get("idempotency_key") != b["args"].get("idempotency_key"):
            issues.append("plan_production_moves repeated without a failed call and the same key")
    order_ok = not issues

    # Arguments (D450 rule 2).
    arg_issues = []
    fam, tgt = prod["family"], prod["target"]
    for c in calls:
        a = c["args"]
        if c["tool"] == "extract_product":
            if a.get("facts_chars", 0) < 20:
                arg_issues.append("extract without the description")
            if a.get("family") != fam:
                arg_issues.append("extract without the named family")
        elif c["tool"] == "predict_origin":
            if not ((a.get("product") or {}).get("summary") or {}).get("components"):
                arg_issues.append("predict without a structured product")
            if a.get("family") != fam:
                arg_issues.append("predict without the named family")
        elif c["tool"] == "plan_production_moves":
            if str(a.get("target_country", "")).strip().lower() != tgt.lower():
                arg_issues.append(f"plan target {a.get('target_country')!r}, expected {tgt}")
            if not str(a.get("idempotency_key", "")).strip():
                arg_issues.append("plan without an idempotency key")
            if pf.get("case_id") and a.get("case_id") != pf["case_id"]:
                arg_issues.append("plan on another case")
        elif c["tool"] == "export_ruling_request_draft":
            if a.get("move_key") is None and a.get("plan_index") is None:
                arg_issues.append("export without move_key or plan_index")
            if pf.get("case_id") and a.get("case_id") != pf["case_id"]:
                arg_issues.append("export on another case")
        if c["tool"] in ("plan_production_moves", "export_ruling_request_draft") and not c.get("access_key_matches"):
            arg_issues.append(f"{c['tool']} without predict_origin's case_access_key")
    args_ok = not arg_issues and any(c["tool"] == "plan_production_moves" for c in calls)

    # Target plant added when the description names none (reported, D450 rule 2b).
    plant_added = None
    if not prod["target_plant_named"]:
        plants = [((c["args"].get("product") or {}).get("summary") or {}).get("plants") or [] for c in calls if c["tool"] == "predict_origin"]
        plant_added = any(str(p.get("country", "")).lower() == tgt.lower() for ps in plants for p in ps)

    # Final answer (D450 rules 3-5).
    final = row.get("final") or ""
    origin = pf.get("origin")
    origin_ok = None
    if origin:
        sent = next((m for m in re.split(r"(?<=[.\n])", final) if ORIGIN_SENTENCE.search(m) and countries_in(m)), None)
        origin_ok = mentions(sent, origin) if sent else mentions(final, origin)
    tool_rulings = set().union(*[set(c.get("rulings") or []) for c in calls]) if calls else set()
    said = norm_rulings(final)
    cites = bool(said & tool_rulings)
    invented = sorted(said - tool_rulings)
    disclaimer = DISCLAIMER in final
    disclaimer_lenient = re.sub(r"\s*[-–—]+\s*", " - ", DISCLAIMER) in re.sub(r"\s*[-–—]+\s*", " - ", final)

    passed = bool(final) and not row.get("error") and order_ok and args_ok and origin_ok is True and cites and not invented and disclaimer
    gold = None
    if origin:
        gold = {"cbp": prod["cbp_origin"], "predicted": origin, "agree": origin.lower() == prod["cbp_origin"].lower(),
                "source_ruling_in_precedents": prod["ruling"] in (pf.get("precedents") or []) or prod["ruling"] in tool_rulings}
    return {
        "id": row["id"], "run": row["run"], "model": row["model"], "mcp": row["mcp"],
        "tools": seq, "order_ok": order_ok, "order_issues": issues, "args_ok": args_ok, "arg_issues": sorted(set(arg_issues)),
        "target_plant_added": plant_added, "predicted": origin, "origin_consistent": origin_ok,
        "cites_tool_ruling": cites, "invented_rulings": invented, "disclaimer": disclaimer, "disclaimer_lenient": disclaimer_lenient,
        "completed": bool(final) and not row.get("error"), "error": row.get("error"),
        "plans_shown": shown, "tool_errors": [c["tool"] for c in calls if err(c)],
        "wall_s": row["wall_s"], "cost_usd": row.get("cost_usd", 0.0), "model_calls": row.get("model_calls", 0),
        "gold": gold, "pass": passed,
    }


def pct(k, n):
    return f"{k}/{n}" + (f" ({100 * k / n:.0f}%)" if n else "")


def aggregate(scored):
    n = len(scored)
    c = lambda f: sum(1 for s in scored if f(s))
    walls = sorted(s["wall_s"] for s in scored)
    costs = [s["cost_usd"] for s in scored]
    origin_n = [s for s in scored if s["origin_consistent"] is not None]
    gold_n = [s for s in scored if s["gold"]]
    plant_n = [s for s in scored if s["target_plant_added"] is not None]
    return {
        "runs": n,
        "completed": c(lambda s: s["completed"]),
        "order_ok": c(lambda s: s["order_ok"]),
        "args_ok": c(lambda s: s["args_ok"]),
        "origin_consistent": [sum(1 for s in origin_n if s["origin_consistent"]), len(origin_n)],
        "cites_tool_ruling": c(lambda s: s["cites_tool_ruling"]),
        "runs_with_invented_rulings": c(lambda s: s["invented_rulings"]),
        "disclaimer": c(lambda s: s["disclaimer"]),
        "disclaimer_lenient": c(lambda s: s["disclaimer_lenient"]),
        "target_plant_added": [sum(1 for s in plant_n if s["target_plant_added"]), len(plant_n)],
        "pass_all": c(lambda s: s["pass"]),
        "gold_agree": [sum(1 for s in gold_n if s["gold"]["agree"]), len(gold_n)],
        "gold_source_in_precedents": sum(1 for s in gold_n if s["gold"]["source_ruling_in_precedents"]),
        "wall_s": {"median": statistics.median(walls), "max": walls[-1], "min": walls[0]} if walls else None,
        "cost_usd": {"total": round(sum(costs), 4), "per_run_mean": round(sum(costs) / n, 4) if n else 0,
                     "per_run_max": round(max(costs), 4) if costs else 0},
    }


def render_md(summary):
    out = ["# ADK agent evaluation: scores", "",
           "Written by `python3 -I -B eval/agent/score.py .` from `eval/agent/results/*.jsonl` (rules: docs/DECISIONS.md D450). Do not edit by hand.", ""]
    for label, block in summary["runs"].items():
        a = block["aggregate"]
        out += [f"## {label}", "", f"{block['note']}", "",
                "| Measure | Result |", "|---|---|",
                f"| Runs finished with an answer | {pct(a['completed'], a['runs'])} |",
                f"| Tool order correct | {pct(a['order_ok'], a['runs'])} |",
                f"| Arguments valid | {pct(a['args_ok'], a['runs'])} |",
                f"| Final answer states predict_origin's origin | {pct(*a['origin_consistent'])} |",
                f"| Cites a ruling the tools returned | {pct(a['cites_tool_ruling'], a['runs'])} |",
                f"| Runs naming a ruling no tool returned | {pct(a['runs_with_invented_rulings'], a['runs'])} |",
                f"| \"Not a ruling\" line, word for word | {pct(a['disclaimer'], a['runs'])} |",
                f"| Target plant added when none was named | {pct(*a['target_plant_added'])} |",
                f"| Passed every check | {pct(a['pass_all'], a['runs'])} |",
                f"| predict_origin agrees with CBP (not an accuracy figure, see D450) | {pct(*a['gold_agree'])}; source ruling among the tools' rulings in {a['gold_source_in_precedents']} |",
                f"| Wall time per run, median (min to max) | {a['wall_s']['median']} s ({a['wall_s']['min']} to {a['wall_s']['max']} s) |" if a["wall_s"] else "| Wall time | none |",
                f"| Agent model cost (gateway-reported) | ${a['cost_usd']['total']:.4f} total, ${a['cost_usd']['per_run_mean']:.4f} mean per run, ${a['cost_usd']['per_run_max']:.4f} max |",
                "", "| Product | Tools called | Order | Args | Origin said | Cites | Invented | Disclaimer | CBP / predicted | Time | Notes |",
                "|---|---|---|---|---|---|---|---|---|---|---|"]
        for s in block["scored"]:
            yn = lambda v: "n/a" if v is None else ("yes" if v else "no")
            g = s["gold"]
            gold = f"{g['cbp']} / {g['predicted']}" if g else f"/ {s['predicted'] or 'none'}"
            notes = "; ".join(s["order_issues"] + s["arg_issues"] + ([f"tool errors: {', '.join(s['tool_errors'])}"] if s["tool_errors"] else []) + ([s["error"]] if s["error"] else []))
            out.append(f"| {s['id']} | {' > '.join(t.replace('_', ' ') for t in s['tools'])} | {yn(s['order_ok'])} | {yn(s['args_ok'])} | {yn(s['origin_consistent'])} | {yn(s['cites_tool_ruling'])} | {', '.join(s['invented_rulings']) or 'none'} | {yn(s['disclaimer'])} | {gold} | {s['wall_s']} s | {notes or ''} |")
        out.append("")
    return "\n".join(out)


def main():
    products = {p["id"]: p for p in jl(R("eval/agent/products.jsonl"))}
    problems = check_set(products.values())
    if problems:
        sys.exit("eval set problems:\n" + "\n".join(problems))
    runs = {}
    for path in sorted(glob.glob(R("eval/agent/results/*.jsonl"))):
        label = os.path.basename(path)[:-len(".jsonl")]
        scored = [score_run(r, products[r["id"]]) for r in jl(path)]
        scored.sort(key=lambda s: s["id"])
        meta = path[:-len(".jsonl")] + ".meta.json"
        note = json.load(open(meta))["note"] if os.path.exists(meta) else ""
        runs[label] = {"note": note, "file": os.path.relpath(path, REPO), "aggregate": aggregate(scored), "scored": scored}
    summary = {"rules": "docs/DECISIONS.md D450", "products": len(products), "runs": runs}
    js = json.dumps(summary, indent=1, ensure_ascii=False) + "\n"
    md = render_md(summary) + "\n"
    if CHECK:
        bad = [p for p, s in ((OUT_JSON, js), (OUT_MD, md)) if not os.path.exists(R(p)) or open(R(p)).read() != s]
        if bad:
            sys.exit(f"{', '.join(bad)} differ from a fresh computation; re-run without --check")
        print(f"agent eval: {len(runs)} run(s) over {len(products)} products match {OUT_JSON}")
        return
    open(R(OUT_JSON), "w").write(js)
    open(R(OUT_MD), "w").write(md)
    print(md)


if __name__ == "__main__":
    main()
