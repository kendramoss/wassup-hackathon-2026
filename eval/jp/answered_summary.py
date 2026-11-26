"""Japan after one round of question-card answers: D921 analysis.

Usage: python3 -I -B eval/jp/answered_summary.py <repo> [--check]

For each answered split, every case keeps its base-run row unless the
answered run re-checked it, in which case the re-checked row counts. Reports
criterion match before and after with the exact McNemar test on the changed
cases, per agreement, wrong approvals (approved under a criterion that is not
the published one) and, for dev_neg, negatives approved. Writes
eval/jp/d922-answered-summary.json.
"""
import json, math, os, sys

RUNS = "eval/jp/runs/"
PAIRS = {
    "dev_eval": ("jp-dev_eval-gemini-4-argon-jp-engine-v1-litellm-d909-dev",
                 "jp-dev_eval_answered-gemini-4-argon-jp-engine-v1-litellm-d921-dev"),
    "fresh_1126": ("jp-fresh_1126-gemini-4-argon-jp-engine-v1-litellm-d919-fresh",
                   "jp-fresh_1126_answered-gemini-4-argon-jp-engine-v1-litellm-d921-fresh"),
    "dev_neg": ("jp-dev_neg-gemini-4-argon-jp-engine-v1-litellm-d913-neg-v1",
                "jp-dev_neg_answered-gemini-4-argon-jp-engine-v1-litellm-d921-neg"),
}
OUT = "eval/jp/d922-answered-summary.json"


def rows(repo, name):
    return {r["id"]: r for r in (json.loads(l) for l in open(os.path.join(repo, RUNS + name + ".jsonl"), encoding="utf-8"))}


def jp(r):
    return ((r.get("prediction") or {}).get("japan_preference") or {})


def right(r):
    l, p = r["label"], jp(r)
    gold = l.get("criteria") or ([l["criterion"]] if l.get("criterion") else [])
    return p.get("outcome") == "qualifies" and p.get("criterion") in gold


def wrong_approval(r):
    return jp(r).get("outcome") == "qualifies" and not right(r)


def mcnemar(b, c):
    n = b + c
    if n == 0:
        return 1.0
    k = min(b, c)
    return round(min(1.0, 2 * sum(math.comb(n, j) for j in range(k + 1)) / 2 ** n), 6)


def wilson(k, n):
    if n == 0:
        return [0.0, 0.0]
    z, p = 1.959964, k / n
    d = 1 + z * z / n
    c = (p + z * z / (2 * n)) / d
    h = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / d
    return [round(c - h, 4), round(c + h, 4)]


def summarise(repo):
    out = {}
    for split, (base_name, ans_name) in PAIRS.items():
        base, ans = rows(repo, base_name), rows(repo, ans_name)
        after = {i: ans.get(i, r) for i, r in base.items()}
        s = {"base_run": base_name, "answered_run": ans_name, "cases": len(base), "rechecked": len(ans),
             "cost_usd": round(sum(r.get("cost_usd") or 0 for r in ans.values()), 4)}
        if split == "dev_neg":
            s["approved_before"] = sum(jp(r).get("outcome") == "qualifies" for r in base.values())
            s["approved_after"] = sum(jp(r).get("outcome") == "qualifies" for r in after.values())
            s["undetermined_after"] = sum(jp(r).get("outcome") == "undetermined" for r in after.values())
        else:
            gained = sorted(i for i in base if not right(base[i]) and right(after[i]))
            lost = sorted(i for i in base if right(base[i]) and not right(after[i]))
            kb, ka = sum(map(right, base.values())), sum(map(right, after.values()))
            s.update({"criterion_before": kb, "criterion_after": ka, "net": ka - kb, "gained": len(gained), "lost": len(lost),
                      "mcnemar_p": mcnemar(len(gained), len(lost)), "ci_after": wilson(ka, len(base)),
                      "wrong_approvals_before": sum(map(wrong_approval, base.values())),
                      "wrong_approvals_after": sum(map(wrong_approval, after.values())),
                      "undetermined_after": sum(jp(r).get("outcome") == "undetermined" for r in after.values()),
                      "declined_after": sum(jp(r).get("outcome") == "does_not_qualify" for r in after.values())})
            by = {}
            for i, r in base.items():
                a = r["label"]["agreement"]
                b = by.setdefault(a, {"n": 0, "before": 0, "after": 0})
                b["n"] += 1
                b["before"] += right(r)
                b["after"] += right(after[i])
            s["by_agreement"] = dict(sorted(by.items()))
        out[split] = s
    return out


def main():
    if len(sys.argv) < 2 or len(sys.argv) > 3 or (len(sys.argv) == 3 and sys.argv[2] != "--check"):
        sys.exit(__doc__.split("\n\n")[1])
    repo = sys.argv[1]
    text = json.dumps(summarise(repo), indent=1, sort_keys=True) + "\n"
    path = os.path.join(repo, OUT)
    if len(sys.argv) == 3:
        if open(path, encoding="utf-8").read() != text:
            sys.exit("answered_summary: %s is out of date" % OUT)
        print("answered summary ok")
        return
    open(path, "w", encoding="utf-8").write(text)
    print(text)


if __name__ == "__main__":
    main()
