"""Scores the ADK agent evaluation runs (D450, D510). No model calls, no network.

Reads eval/agent/products.jsonl and every eval/agent/results/*.jsonl, scores
each run with the rules pre-registered in docs/DECISIONS.md D450, and again
with D510's rules (D450 plus the D470 changes), and writes
eval/agent/summary.json and eval/agent/SUMMARY.md. Runs whose meta file names
a series (v2) are also compared run against run, product by product.

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
# D510 rule 7: with no plan shown, a sentence of the answer (outside code
# blocks) claims a move changes origin when it names the target country in one
# of these shapes and has no negation.
CLAIM = [
    r"\b(make|makes|making|establish|establishes|establishing|become|becomes|confer|confers|conferring|shift|shifts|shifting|change|changes|changing|render|renders)\b[^.\n]{0,80}\b{T}\b[^.\n]{0,40}\borigin\b",
    r"\b{T}\b[^.\n]{0,40}\b(would|will|could|can)\s+(then\s+)?(become|be)\s+(the\s+)?(new\s+)?(country\s+of\s+)?origin\b",
    r"\borigin\b[^.\n]{0,40}\b(would|will|could|can)\s+(then\s+)?(become|be|change\s+to|shift\s+to|move\s+to)\s+{T}\b",
]
NEGATION = re.compile(r"\b(no|not|cannot|can't|won't|none|nothing|neither|nor|unsupported|stands)\b", re.I)
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
        # The rule counts calls. Only a call that succeeded makes a case: a06
        # in D451 was refused once by input validation, then called again
        # (judge panel round 10, N3), so the label says which it was.
        made = sum(1 for c in calls if c["tool"] == "predict_origin" and not err(c))
        issues.append("predict_origin called more than once (" + ("a second case" if made > 1 else "an earlier call was refused; no second case") + ")")
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
            # plan_id (D470) is the move key under its new name.
            if a.get("move_key") is None and a.get("plan_index") is None and a.get("plan_id") is None:
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

    # D510: D450 plus the D470 changes.
    v2_order = list(issues)
    exports = [c for c in calls if c["tool"] == "export_ruling_request_draft"]
    for a, b in zip(exports, exports[1:]):
        if not err(a):
            v2_order.append("export repeated after a draft that worked")
            break
    v2_args = [i for i in arg_issues if i != "export without move_key or plan_index"]
    ids = [i for c in plans_ok for i in ((c.get("fields") or {}).get("plan_ids") or []) if i]
    for c in exports:
        a = c["args"]
        sel = [k for k in ("plan_id", "plan_index", "move_key") if a.get(k) is not None]
        if shown == 0 and sel:
            v2_args.append("export names a plan when the search showed none")
        elif shown > 0 and not sel:
            v2_args.append("export without plan_id, plan_index or move_key after a search that showed a plan")
        if shown > 0 and a.get("plan_id") is not None and ids and a["plan_id"] not in ids:
            v2_args.append("export plan_id is not one the search returned")
        if shown > 0 and isinstance(a.get("plan_index"), int) and not 0 <= a["plan_index"] < shown:
            v2_args.append("export plan_index out of range")
    v2_args_ok = not v2_args and any(c["tool"] == "plan_production_moves" for c in calls)
    no_plan_claims = None
    if plans_ok and shown == 0:
        prose = re.sub(r"```.*?(```|$)", " ", final, flags=re.S)
        pats = [re.compile(p.replace("{T}", "(?:" + "|".join(re.escape(n) for n in names(tgt)) + ")"), re.I) for p in CLAIM]
        no_plan_claims = [m.strip() for m in re.split(r"(?<=[.!?\n])", prose)
                          if any(p.search(m) for p in pats) and not NEGATION.search(m)]
    passed_v2 = (bool(final) and not row.get("error") and not v2_order and v2_args_ok and origin_ok is True and cites
                 and not invented and disclaimer and not no_plan_claims)
    gold = None
    if origin:
        gold = {"cbp": prod["cbp_origin"], "predicted": origin, "agree": origin.lower() == prod["cbp_origin"].lower(),
                "source_ruling_in_precedents": prod["ruling"] in (pf.get("precedents") or []) or prod["ruling"] in tool_rulings,
                "source_ruling_in_answer": prod["ruling"] in said}
    return {
        "id": row["id"], "run": row["run"], "model": row["model"], "mcp": row["mcp"],
        "tools": seq, "order_ok": order_ok, "order_issues": issues, "args_ok": args_ok, "arg_issues": sorted(set(arg_issues)),
        "target_plant_added": plant_added, "predicted": origin, "origin_consistent": origin_ok,
        "cites_tool_ruling": cites, "invented_rulings": invented, "disclaimer": disclaimer, "disclaimer_lenient": disclaimer_lenient,
        "completed": bool(final) and not row.get("error"), "error": row.get("error"),
        "plans_shown": shown, "tool_errors": [c["tool"] for c in calls if err(c)],
        "wall_s": row["wall_s"], "cost_usd": row.get("cost_usd", 0.0), "model_calls": row.get("model_calls", 0),
        "gold": gold, "pass": passed,
        "v2": {"order_ok": not v2_order, "order_issues": v2_order, "args_ok": v2_args_ok, "arg_issues": sorted(set(v2_args)),
               "no_plan_claims": no_plan_claims, "pass": passed_v2,
               "plan_ids": ids, "export_plan_ids": [c["args"].get("plan_id") for c in exports]},
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
        "d510": {
            "order_ok": c(lambda s: s["v2"]["order_ok"]),
            "args_ok": c(lambda s: s["v2"]["args_ok"]),
            "no_plan_honest": [c(lambda s: s["v2"]["no_plan_claims"] == []), c(lambda s: s["v2"]["no_plan_claims"] is not None)],
            "runs_with_plans_shown": c(lambda s: s["plans_shown"] > 0),
            "pass_all": c(lambda s: s["v2"]["pass"]),
        },
        "gold_agree": [sum(1 for s in gold_n if s["gold"]["agree"]), len(gold_n)],
        "gold_source_in_precedents": sum(1 for s in gold_n if s["gold"]["source_ruling_in_precedents"]),
        "gold_source_in_answer": sum(1 for s in gold_n if s["gold"]["source_ruling_in_answer"]),
        "wall_s": {"median": round(statistics.median(walls), 1), "max": walls[-1], "min": walls[0]} if walls else None,
        "cost_usd": {"total": round(sum(costs), 4), "per_run_mean": round(sum(costs) / n, 4) if n else 0,
                     "per_run_max": round(max(costs), 4) if costs else 0},
    }


def render_md(summary):
    out = ["# ADK agent evaluation: scores", "",
           "Written by `python3 -I -B eval/agent/score.py .` from `eval/agent/results/*.jsonl` (rules: docs/DECISIONS.md D450; D510 adds the D470 changes). Do not edit by hand.", ""]
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
                f"| predict_origin agrees with CBP (not an accuracy figure, see D450) | {pct(*a['gold_agree'])}; the scored product's own ruling was among the tools' rulings in {a['gold_source_in_precedents']} and named in the answer in {a['gold_source_in_answer']} |",
                f"| Wall time per run, median (min to max) | {a['wall_s']['median']} s ({a['wall_s']['min']} to {a['wall_s']['max']} s) |" if a["wall_s"] else "| Wall time | none |",
                f"| Agent model cost (gateway-reported) | ${a['cost_usd']['total']:.4f} total, ${a['cost_usd']['per_run_mean']:.4f} mean per run, ${a['cost_usd']['per_run_max']:.4f} max |",
                f"| D510 rules: tool order correct | {pct(a['d510']['order_ok'], a['runs'])} |",
                f"| D510 rules: arguments valid | {pct(a['d510']['args_ok'], a['runs'])} |",
                f"| D510 rules: no move claimed after a search with no plan | {pct(*a['d510']['no_plan_honest'])} |",
                f"| D510 rules: passed every check | {pct(a['d510']['pass_all'], a['runs'])} |",
                "", "| Product | Tools called | Order | Args | Origin said | Cites | Invented | Disclaimer | CBP / predicted | Time | Notes |",
                "|---|---|---|---|---|---|---|---|---|---|---|"]
        for s in block["scored"]:
            yn = lambda v: "n/a" if v is None else ("yes" if v else "no")
            g = s["gold"]
            gold = f"{g['cbp']} / {g['predicted']}" if g else f"/ {s['predicted'] or 'none'}"
            v2 = s["v2"]
            extra = [i for i in v2["order_issues"] + v2["arg_issues"] if i not in s["order_issues"] + s["arg_issues"]]
            extra += ["D510: claims a move changes origin after no plan"] if v2["no_plan_claims"] else []
            extra += [] if v2["pass"] == s["pass"] else [f"D510 pass: {yn(v2['pass'])}"]
            notes = "; ".join(s["order_issues"] + s["arg_issues"] + extra + ([f"tool errors: {', '.join(s['tool_errors'])}"] if s["tool_errors"] else []) + ([s["error"]] if s["error"] else []))
            out.append(f"| {s['id']} | {' > '.join(t.replace('_', ' ') for t in s['tools'])} | {yn(s['order_ok'])} | {yn(s['args_ok'])} | {yn(s['origin_consistent'])} | {yn(s['cites_tool_ruling'])} | {', '.join(s['invented_rulings']) or 'none'} | {yn(s['disclaimer'])} | {gold} | {s['wall_s']} s | {notes or ''} |")
        out.append("")
    for name, ser in (summary.get("series") or {}).items():
        out += render_series(name, ser)
    return "\n".join(out)


def series(runs, metas):
    """Groups runs whose meta names a series (D510) and compares them."""
    out = {}
    for label, meta in metas.items():
        if meta.get("series"):
            out.setdefault(meta["series"], {"runs": [], "baseline": meta.get("baseline")})["runs"].append((meta.get("rep", 0), label))
    res = {}
    for name, g in out.items():
        labels = [l for _, l in sorted(g["runs"])]
        by = [{s["id"]: s for s in runs[l]["scored"]} for l in labels]
        ids = sorted(set.intersection(*[set(b) for b in by])) if by else []
        base = {s["id"]: s for s in runs[g["baseline"]]["scored"]} if g["baseline"] in runs else {}
        same = lambda f: sum(1 for i in ids if len({json.dumps(f(b[i])) for b in by}) == 1)
        rows = []
        for i in ids:
            rows.append({"id": i,
                         "v1_d450": base[i]["pass"] if i in base else None, "v1_d510": base[i]["v2"]["pass"] if i in base else None,
                         "runs_d510": [b[i]["v2"]["pass"] for b in by],
                         "predicted": [b[i]["predicted"] for b in by], "plans_shown": [b[i]["plans_shown"] for b in by],
                         "export_plan_ids": [b[i]["v2"]["export_plan_ids"] for b in by],
                         "issues": [b[i]["v2"]["order_issues"] + b[i]["v2"]["arg_issues"] + (["claims a move after no plan"] if b[i]["v2"]["no_plan_claims"] else []) + ([f"tool errors: {', '.join(b[i]['tool_errors'])}"] if b[i]["tool_errors"] else []) + ([b[i]["error"]] if b[i]["error"] else []) for b in by]})
        pooled = [b[i] for b in by for i in ids]
        res[name] = {
            "runs": labels, "baseline": g["baseline"], "products": len(ids),
            "pass_d510_per_run": [sum(1 for i in ids if b[i]["v2"]["pass"]) for b in by],
            "pass_d450_per_run": [sum(1 for i in ids if b[i]["pass"]) for b in by],
            "pass_d510_pooled": [sum(1 for s in pooled if s["v2"]["pass"]), len(pooled)],
            "agree": {
                "pass_d510": same(lambda s: s["v2"]["pass"]),
                "both_pass_d510": sum(1 for i in ids if all(b[i]["v2"]["pass"] for b in by)),
                "both_fail_d510": sum(1 for i in ids if not any(b[i]["v2"]["pass"] for b in by)),
                "predicted_origin": same(lambda s: s["predicted"]),
                "plan_found": same(lambda s: s["plans_shown"] > 0),
                "plans_shown_count": same(lambda s: s["plans_shown"]),
                "export_plan_id": same(lambda s: s["v2"]["export_plan_ids"]),
                "required_tool_order": same(lambda s: [t for t in s["tools"] if t in ORDER]),
                "full_tool_sequence": same(lambda s: s["tools"]),
            },
            "baseline_d450": sum(1 for s in base.values() if s["pass"]) if base else None,
            "baseline_d510": sum(1 for s in base.values() if s["v2"]["pass"]) if base else None,
            "rows": rows,
        }
    return res


def render_series(name, g):
    yn = lambda v: "n/a" if v is None else ("pass" if v else "fail")
    n, k = g["products"], len(g["runs"])
    ag = g["agree"]
    out = [f"## Series {name}: {k} runs per product, compared (D510)", "",
           f"Runs: {', '.join(g['runs'])}. Baseline: {g['baseline']} (one run per product). Pass is under D510 rules unless the row says D450.", "",
           "| Measure | Result |", "|---|---|",
           f"| Baseline passed every check, D450 rules as published | {pct(g['baseline_d450'], n)} |",
           f"| Baseline passed every check, rescored under D510 rules | {pct(g['baseline_d510'], n)} |"]
    for j, (lab, p, p4) in enumerate(zip(g["runs"], g["pass_d510_per_run"], g["pass_d450_per_run"]), 1):
        out.append(f"| Run {j} ({lab}) passed every check | {pct(p, n)} (D450 rules: {p4}/{n}) |")
    out += [f"| All runs pooled | {pct(*g['pass_d510_pooled'])} |",
            f"| Same pass or fail in every run | {pct(ag['pass_d510'], n)} ({ag['both_pass_d510']} pass in all, {ag['both_fail_d510']} fail in all) |",
            f"| Same predicted origin in every run | {pct(ag['predicted_origin'], n)} |",
            f"| Same plan found or not found in every run | {pct(ag['plan_found'], n)} |",
            f"| Same number of plans shown in every run | {pct(ag['plans_shown_count'], n)} |",
            f"| Same plan drafted (export plan_id) in every run | {pct(ag['export_plan_id'], n)} |",
            f"| Same order of the four required tools in every run | {pct(ag['required_tool_order'], n)} |",
            f"| Same full tool sequence in every run | {pct(ag['full_tool_sequence'], n)} |",
            "", "| Product | v1 (D450) | v1 (D510) | " + " | ".join(f"Run {j}" for j in range(1, k + 1)) + " | Predicted | Plans shown | Drafted plan_id | Issues |",
            "|---|---|---|" + "---|" * k + "---|---|---|---|"]
    for r in g["rows"]:
        iss = " / ".join(f"run {j}: {'; '.join(x)}" for j, x in enumerate(r["issues"], 1) if x)
        pid = " / ".join(", ".join(str(x) for x in e) or "current setup" for e in r["export_plan_ids"])
        out.append(f"| {r['id']} | {yn(r['v1_d450'])} | {yn(r['v1_d510'])} | " + " | ".join(yn(x) for x in r["runs_d510"])
                   + f" | {' / '.join(str(x) for x in r['predicted'])} | {' / '.join(str(x) for x in r['plans_shown'])} | {pid} | {iss} |")
    out.append("")
    return out


def main():
    products = {p["id"]: p for p in jl(R("eval/agent/products.jsonl"))}
    problems = check_set(products.values())
    if problems:
        sys.exit("eval set problems:\n" + "\n".join(problems))
    runs, metas = {}, {}
    for path in sorted(glob.glob(R("eval/agent/results/*.jsonl"))):
        label = os.path.basename(path)[:-len(".jsonl")]
        scored = [score_run(r, products[r["id"]]) for r in jl(path)]
        scored.sort(key=lambda s: s["id"])
        meta = path[:-len(".jsonl")] + ".meta.json"
        metas[label] = json.load(open(meta)) if os.path.exists(meta) else {}
        note = metas[label].get("note", "")
        runs[label] = {"note": note, "file": os.path.relpath(path, REPO), "aggregate": aggregate(scored), "scored": scored}
    summary = {"rules": "docs/DECISIONS.md D450; D510", "products": len(products), "runs": runs}
    ser = series(runs, metas)
    if ser:
        summary["series"] = ser
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
