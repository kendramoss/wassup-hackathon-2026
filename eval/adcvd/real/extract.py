"""Everyday-import rulings for the realistic AD/CVD test (D945): one CBP New York ruling in, one product description out.

Usage (standard library only; gateway, LITELLM_* in the environment):
    python3 -I -B eval/adcvd/real/extract.py run RAW.jsonl OUT.jsonl [--workers N] [--limit N]

RAW.jsonl holds CROSS rulings fetched by number (id, date, tariffs, text). The text a model sees has every paragraph that mentions
antidumping, countervailing duties or AD/CVD removed first, so the description cannot carry CBP's own warning. CBP's warning is
read from the full text by rule, with no model: a paragraph that says the merchandise may be (or is) subject to an AD or CVD
order, naming a case number or a product; generic boilerplate (Section 301 notes, substantial-transformation law) is ignored.
One call per ruling at temperature 0; rows already in OUT.jsonl are kept.
"""
import argparse
import concurrent.futures as cf
import json
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))
from rulings import chat  # noqa: E402

PROMPT_VERSION = "adcvd-real-extract-v1"
MODEL = "gemini-3.8-flash"
SYSTEM = ("You read one U.S. Customs and Border Protection (CBP) ruling. Answer with a JSON object with exactly these keys: "
          "\"product_ruling\", \"origin\" and \"description\".\n"
          "\"product_ruling\": true if the ruling classifies, or decides the country of origin of, one physical product (or a defined set of physical products) that is imported for sale or use; false for rulings about services, software, a revocation or modification of another ruling, or no product.\n"
          "\"origin\": the country where the ruled product is made, in plain English as the ruling states it (for example \"China\", \"India\", \"Vietnam\", \"Taiwan\", \"South Korea\", \"Turkey\"); \"unknown\" if the ruling does not state it; \"multiple\" if it rules on products of several countries or the origin is split between countries. Do not use the importer's address, the port of entry or the country of a processor of a part unless the ruling says that is where the finished product is made.\n"
          "\"description\": what an importer would write on a product page or a commercial invoice line for the product: what it is, its main material, form, use and size or model, in one or two plain sentences. Leave out the tariff classification, any duty rate, any trade remedy, any mention of antidumping, countervailing or Section 301 duties, CBP's legal reasoning and the requestor.")
USER = "Ruling:\n{text}"

AD = re.compile(r"anti-?dumping|countervailing|\bAD/CVD\b|\bAD\s*/\s*CVD\b", re.I)
GENERIC = re.compile(r"9903\.88|section 301|Note 20|substantial transformation analysis|safeguard|may be subject to .{0,30}(duties|measures) (under|pursuant)", re.I)
CASE = re.compile(r"\b[AC]-\d{3}-\d{3}\b")
NOTE = re.compile(r"(may|might|would|could|will|does|do|is|are)(\s+also)?\s+(be\s+)?subject(\s+to)?|fall[s]?\s+within|covered\s+by|(is|are)\s+subject", re.I)
NEG = re.compile(r"(not|no)\s+(be\s+)?(subject\s+to|covered\s+by|within\s+the\s+scope)|is\s+not\s+subject|are\s+not\s+subject", re.I)


def paragraphs(text):
    return [p for p in re.split(r"\n\s*\n|\r\r+", text.replace("\r\n", "\n")) if p.strip()]


def split_text(text):
    """(text without AD/CVD paragraphs, the AD/CVD paragraphs)."""
    keep, ad = [], []
    for p in paragraphs(text):
        (ad if AD.search(p) else keep).append(p)
    return "\n\n".join(keep), ad


def note_of(ad_pars):
    """CBP's own warning: the AD/CVD paragraph that says the merchandise may be subject to an order. Generic boilerplate is ignored."""
    notes, neg = [], []
    for p in ad_pars:
        s = re.sub(r"\s+", " ", p).strip()
        if "Written decisions regarding the scope of AD/CVD orders" in s or "Enforcement and Compliance" in s:
            # the sentence(s) before the standard "Written decisions..." sentence name the product or case
            head = s.split("Written decisions regarding")[0].strip()
            notes.append(head or s)
            continue
        if GENERIC.search(s) and not CASE.search(s):
            continue
        if NEG.search(s) and AD.search(s):
            neg.append(s[:600])
        elif NOTE.search(s) and (CASE.search(s) or AD.search(s)):
            notes.append(s[:600])
    return notes, neg


def facts_of(clean):
    """The part of a ruling that states the product's facts: after the salutation, before the classification analysis."""
    m = re.search(r"\bDear [^\n]*:?\s*\n", clean)
    body = clean[m.end():] if m else clean
    body = re.sub(r"\s+", " ", body).strip()
    cut = re.search(r"(The applicable subheading|The tariff classification|HOLDING:|Classification is subject to application of GRI|The merchandise is classifiable|is provided for in)", body)
    if cut and cut.start() > 300:
        body = body[:cut.start()]
    return body[:2500]


def parse(r):
    text = r["text"] or ""
    cat = re.search(r"CATEGORY:\s*([^\n\r]+)", text)
    tar = re.search(r"TARIFF NO\.?:\s*([0-9.\s,;and]+)", text)
    hts = re.findall(r"\d{4}\.\d{2}(?:\.\d{2,4})?", tar.group(1)) if tar else []
    clean, ad = split_text(text)
    notes, neg = note_of(ad)
    return {"id": r["n"], "date": r["date"], "category": (cat.group(1).strip() if cat else ""), "hts": hts[:1], "clean": clean,
            "note": " ".join(notes)[:900], "note_cases": sorted(set(CASE.findall(" ".join(notes)))), "negative_note": " ".join(neg)[:600],
            "mentions_adcvd": bool(ad), "facts": facts_of(clean)}


def run(raw, out, workers, limit, min_id="", max_id=""):
    base, key = os.environ["LITELLM_BASE_URL"].rstrip("/"), os.environ["LITELLM_API_KEY"]
    rows = []
    for l in open(raw):
        r = json.loads(l)
        if not r.get("text"):
            continue
        p = parse(r)
        if "classification" not in p["category"].lower() and "origin" not in p["category"].lower():
            continue
        if not p["hts"]:
            continue
        rows.append(p)
    rows.sort(key=lambda p: p["id"])
    rows = [p for p in rows if (not min_id or p["id"] >= min_id) and (not max_id or p["id"] <= max_id)]
    if limit:
        rows = rows[:limit]
    have = {json.loads(l)["id"] for l in open(out)} if os.path.exists(out) else set()
    todo = [p for p in rows if p["id"] not in have]
    tok = {"prompt": 0, "completion": 0, "calls": 0}

    def one(p):
        try:
            a, u = chat(MODEL, SYSTEM, USER.format(text=p["clean"][:12000]), base, key)
        except Exception as e:
            return None, {}
        d = {k: v for k, v in p.items() if k != "clean"}
        d.update({"product_ruling": bool(a.get("product_ruling")), "origin": str(a.get("origin", "")).strip(), "description": str(a.get("description", "")).strip(),
                  "prompt_version": PROMPT_VERSION, "model": MODEL})
        return d, u

    failed = 0
    with open(out, "a") as f, cf.ThreadPoolExecutor(workers) as ex:
        for d, u in ex.map(one, todo):
            if d is None:
                failed += 1
                continue
            f.write(json.dumps(d, ensure_ascii=False) + "\n")
            f.flush()
            tok["prompt"] += u.get("prompt_tokens", 0)
            tok["completion"] += u.get("completion_tokens", 0)
            tok["calls"] += 1
    print(json.dumps({"rulings_parsed": len(rows), "asked": len(todo), "failed": failed, **tok}))


if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    sub = ap.add_subparsers(dest="cmd", required=True)
    r = sub.add_parser("run")
    r.add_argument("raw")
    r.add_argument("out")
    r.add_argument("--workers", type=int, default=8)
    r.add_argument("--limit", type=int, default=0)
    r.add_argument("--min-id", default="")
    r.add_argument("--max-id", default="")
    a = ap.parse_args()
    run(a.raw, a.out, a.workers, a.limit, a.min_id, a.max_id)
