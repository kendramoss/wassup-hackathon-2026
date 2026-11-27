"""Extract USMCA-preference test cases from CBP rulings that apply HTSUS General Note 11.

Usage: python3 -I -B eval/usmca/extract_cases.py <repo> <raw_dir> <split> [workers]

<raw_dir> holds the CROSS ruling JSON (data/corpus/raw, not committed). The
candidate rulings are those whose text cites "General Note 11" or "GN 11"
(eval/usmca/candidates.json, written by --candidates). split is dev (dated
before 2026-01-01) or held_out (2026-01-01 on).

One model call per ruling (LiteLLM gateway, OpenAI-compatible; LITELLM_BASE_URL
and LITELLM_API_KEY in the environment) returns, for each product the ruling
decides: CBP's classification of the good, the materials with their codes,
countries and values as the ruling states them, and CBP's answer on USMCA
eligibility. The facts and the label are separate fields; the rule engine
only ever reads the facts. Writes eval/usmca/cases-<split>.jsonl.
"""
import concurrent.futures as cf
import json
import os
import re
import sys
import time
import urllib.request

MODEL = "gemini-3.8-flash"
VERSION = "usmca-cases-v1"
CUTOFF = "2026-01-01"

SYS = """You read a U.S. Customs and Border Protection (CBP) ruling that decides whether imported goods qualify for USMCA preferential treatment under HTSUS General Note 11.
For every distinct product whose USMCA eligibility the ruling decides, return:
- name: the product's name.
- good_hs: the HTSUS code CBP classifies the product in, digits only (8 or 10 digits if given, else 6 or 4).
- materials: every material or component the ruling lists for that product, each with name; hs (digits only, the code the ruling gives that material, "" if none given); country (country of origin or production as stated, "" if not stated); stated_originating ("yes" if the ruling states the material is originating under USMCA, "no" if it states it is non-originating, "" otherwise); value (the material's value as a plain decimal string if the ruling states it, else "").
- materials_list_complete: true only if the ruling presents the list as all the materials used.
- transaction_value, net_cost, vnm: plain decimal strings if the ruling states them for the product, else "".
- produced_in: the country or countries where the product is made.
- question: "usmca_eligibility" if the ruling decides whether this product qualifies for USMCA preference, else "other".
- cbp_answer: "qualifies" or "does_not_qualify" if the ruling answers that for this product, else "unclear".
- cbp_basis: a short quotation (at most 30 words) of the sentence giving CBP's answer, "" if none.
Copy codes and numbers exactly; never compute or infer a code or value that the ruling does not state. Return only JSON."""

MAT = {"type": "object", "properties": {k: {"type": "string"} for k in ["name", "hs", "country", "stated_originating", "value"]},
       "required": ["name", "hs", "country", "stated_originating", "value"], "additionalProperties": False}
PROD_KEYS = ["name", "good_hs", "transaction_value", "net_cost", "vnm", "produced_in", "question", "cbp_answer", "cbp_basis"]
PROD = {"type": "object",
        "properties": {**{k: {"type": "string"} for k in PROD_KEYS}, "materials": {"type": "array", "items": MAT},
                       "materials_list_complete": {"type": "boolean"}},
        "required": PROD_KEYS + ["materials", "materials_list_complete"], "additionalProperties": False}
SCHEMA = {"type": "object", "properties": {"products": {"type": "array", "items": PROD}}, "required": ["products"],
          "additionalProperties": False}

CITE = re.compile(r"General Note 11|GN 11", re.I)


def candidates(raw):
    out = []
    for f in sorted(os.listdir(raw)):
        if not f.endswith(".json"):
            continue
        d = json.load(open(os.path.join(raw, f)))
        t = d.get("text") or ""
        if CITE.search(t):
            out.append({"ruling": d["rulingNumber"], "date": (d.get("rulingDate") or "")[:10]})
    return out


def one(raw, r):
    base = os.environ["LITELLM_BASE_URL"].rstrip("/")
    key = os.environ["LITELLM_API_KEY"]
    text = json.load(open(os.path.join(raw, r["ruling"] + ".json")))["text"]
    body = {"model": MODEL, "temperature": 0, "seed": 20261127, "reasoning_effort": "none", "max_tokens": 8000,
            "messages": [{"role": "system", "content": SYS}, {"role": "user", "content": text[:60000]}],
            "response_format": {"type": "json_schema", "json_schema": {"name": "usmca_cases", "schema": SCHEMA}}}
    err = ""
    for a in range(4):
        try:
            req = urllib.request.Request(base + "/chat/completions", data=json.dumps(body).encode(),
                                         headers={"Authorization": "Bearer " + key, "Content-Type": "application/json"})
            with urllib.request.urlopen(req, timeout=240) as resp:
                cost = float(resp.headers.get("x-litellm-response-cost") or 0)
                d = json.load(resp)
            out = json.loads(d["choices"][0]["message"]["content"])
            return {"ruling": r["ruling"], "date": r["date"], "model": MODEL, "version": VERSION,
                    "products": out["products"], "cost_usd": cost}
        except Exception as e:
            err = str(e)[:200].replace(base, "<gateway>")
            time.sleep(4 * (a + 1))
    return {"ruling": r["ruling"], "date": r["date"], "error": err, "cost_usd": 0}


def main():
    repo, raw = sys.argv[1], sys.argv[2]
    if sys.argv[3] == "--candidates":
        json.dump(candidates(raw), open(f"{repo}/eval/usmca/candidates.json", "w"), indent=0)
        return
    split = sys.argv[3]
    workers = int(sys.argv[4]) if len(sys.argv) > 4 else 4
    cands = json.load(open(f"{repo}/eval/usmca/candidates.json"))
    want = [c for c in cands if (c["date"] < CUTOFF) == (split == "dev")]
    path = f"{repo}/eval/usmca/cases-{split}.jsonl"
    done = {}
    if os.path.exists(path):
        for l in open(path):
            o = json.loads(l)
            if "error" not in o:
                done[o["ruling"]] = o
    todo = [c for c in want if c["ruling"] not in done]
    with cf.ThreadPoolExecutor(workers) as ex:
        new = list(ex.map(lambda c: one(raw, c), todo))
    for o in new:
        if "error" not in o:
            done[o["ruling"]] = o
    with open(path, "w") as f:
        for c in want:
            if c["ruling"] in done:
                f.write(json.dumps(done[c["ruling"]], ensure_ascii=False) + "\n")
    print(split, "wanted", len(want), "new", len(new), "errors", sum("error" in o for o in new),
          "cost_this_run", round(sum(o["cost_usd"] for o in new), 4))


if __name__ == "__main__":
    main()
