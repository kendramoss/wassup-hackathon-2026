"""D862-D865 (#777, #779): check the battery packs template and furniture version 3 against their records.

Every group/outcome pairing a template has that main's did not (b4-before-citations.json) is in battery_packs or
furniture, cites a ruling D862 kept (b4-screen.json) or one of the six battery rulings the power supplies template
already cited (D821), has its held-origin sentence in backend/data/labels.jsonl or labels_cited.jsonl, and is
recorded in b4-citations.json with exactly where it is cited. No template cites a D862 development case.

    python3 -I -B eval/families/depth_b4.py . check
"""
import json
import os
import sys

NEW = {"battery_packs": "battery", "furniture": "furniture"}
CORPUS_BATTERY = {"N342506", "N346317", "N346403", "N347172", "N347193", "N348871"}


def jsonl(p):
    with open(p) as f:
        return [json.loads(l) for l in f if l.strip()]


def check(root):
    E = lambda *p: os.path.join(root, *p)
    lab = {l["ruling"]: l for p in ("backend/data/labels.jsonl", "backend/data/labels_cited.jsonl")
           for l in jsonl(E(p))}
    with open(E("eval/families/b4-before-citations.json")) as f:
        before = json.load(f)["templates"]
    with open(E("eval/families/b4-citations.json")) as f:
        rec = {(c["template"], c["ruling"]): c for c in json.load(f)["citations"]}
    with open(E("eval/families/b4-screen.json")) as f:
        screen = json.load(f)
    pool = {s: {r for r, v in screen[s].items() if v["status"] == "kept"} for s in ("battery", "furniture")}
    pool["battery"] |= CORPUS_BATTERY
    dev = {r for s in ("battery", "furniture") for r, v in screen[s].items() if v["status"] == "development"}
    want, cited = {}, set()
    d = E("backend/internal/planner/templates")
    for n in sorted(os.listdir(d)):
        with open(os.path.join(d, n)) as f:
            t = json.load(f)
        old = set(before.get(t["family"], {}).get("pairs", []))
        for g in t["groups"]:
            p = g.get("precedent") or {}
            for k in ("confers_origin", "not_enough"):
                for r in p.get(k) or []:
                    cited.add(r)
                    if g["id"] + ":" + k + ":" + r in old:
                        continue
                    if t["family"] not in NEW:
                        sys.exit(f"{t['family']}: new pairing {g['id']}:{k}:{r} in a template D863 does not change")
                    want.setdefault((t["family"], r), []).append(g["id"] + ":" + k)
    if set(want) != set(rec):
        sys.exit("b4-citations.json records %s, the templates add %s" % (sorted(rec), sorted(want)))
    for (fam, r), where in want.items():
        c = rec[(fam, r)]
        if r not in pool[NEW[fam]] or not lab.get(r, {}).get("evidence") or sorted(c["cited_in"]) != sorted(where) \
                or c["held_sentence"] != lab[r]["evidence"]:
            sys.exit(f"{fam} {r}: not in the D862 pool, no held-origin sentence, or its record is stale")
    if dev & cited:
        sys.exit("a template cites a D862 development case: %s" % sorted(dev & cited))
    print(len(want), "new pairings checked")


if __name__ == "__main__":
    if sys.argv[2] != "check":
        sys.exit("usage: depth_b4.py <root> check")
    check(sys.argv[1])
