"""Near-duplicate precedents for the D919 fresh rulings.

Usage: python3 -I -B eval/jp/fresh_twins.py <repo> [--check]

Many rulings published after the 22 Nov snapshot are repeat requests for goods
already ruled on. For each fresh ruling this records every earlier ruling in
data/jp/rulings.jsonl whose Japanese product, inputs, process and reasoning
text has a difflib ratio of at least THRESHOLD. Reads no label.
"""
import difflib, json, os, sys

THRESHOLD = 0.85


def text(r):
    return r["product_ja"] + r["inputs_ja"] + r["process_ja"] + r["reasoning_ja"]


def build(old, fresh):
    out = {}
    for f in fresh:
        t = text(f)
        twins = []
        for o in old:
            x = text(o)
            sm = difflib.SequenceMatcher(None, t, x, autojunk=False)
            if sm.real_quick_ratio() < THRESHOLD or sm.quick_ratio() < THRESHOLD:
                continue
            r = sm.ratio()
            if r >= THRESHOLD:
                twins.append({"id": o["id"], "ratio": round(r, 3)})
        twins.sort(key=lambda d: (-d["ratio"], d["id"]))
        out[f["id"]] = twins
    return {"rule": "D919: earlier rulings with difflib ratio >= %.2f on product+inputs+process+reasoning (Japanese)" % THRESHOLD,
            "with_twin": sum(1 for v in out.values() if v), "fresh": len(out), "twins": out}


def main():
    repo = sys.argv[1]
    load = lambda p: [json.loads(l) for l in open(os.path.join(repo, p), encoding="utf-8")]
    out = json.dumps(build(load("data/jp/rulings.jsonl"), load("data/jp/fresh-1126/rulings.jsonl")), indent=1, sort_keys=True) + "\n"
    p = os.path.join(repo, "data/jp/fresh-1126/twins.json")
    if "--check" in sys.argv:
        if open(p, encoding="utf-8").read() != out:
            sys.exit(p + " differs from fresh_twins.py")
        print("jp fresh twins ok")
        return
    open(p, "w", encoding="utf-8").write(out)
    d = json.loads(out)
    print("fresh", d["fresh"], "with a twin", d["with_twin"])


if __name__ == "__main__":
    main()
