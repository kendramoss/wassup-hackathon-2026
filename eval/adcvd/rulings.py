"""Ground truth for the AD/CVD exposure check: Commerce scope rulings.

Usage (standard library only):
    python3 -I -B eval/adcvd/rulings.py entries --after 2025-01-01 --out eval/adcvd/rulings.jsonl [--cache DIR]
    python3 -I -B eval/adcvd/rulings.py label eval/adcvd/rulings.jsonl eval/adcvd/labels.jsonl      (gateway, LITELLM_* in the environment)
    python3 -I -B eval/adcvd/rulings.py check eval/adcvd/rulings.jsonl

"entries" reads every Federal Register "Notice of Scope Rulings" (Commerce,
quarterly, 19 CFR 351.225(o)) and keeps each final scope ruling made on or
after --after: the order's case numbers, its product and country, the
requestor and the ruling text, which ends with the date of the ruling. No
model is called. "label" asks the gateway model for each ruling's outcome and
a neutral description of the product, then for the HTS code of that
description alone (the prompts are in prompts.json, committed with D940 before
any call). "check" re-parses the entries file against the notices' cached
text and fails on any difference.
"""
import argparse
import datetime as dt
import hashlib
import html
import json
import os
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

API = "https://www.federalregister.gov/api/v1/documents.json?"
UA = {"User-Agent": "Mozilla/5.0 (origin-proof adcvd rulings)"}
MONTH = "(?:January|February|March|April|May|June|July|August|September|October|November|December)"
DATE = re.compile(r"(" + MONTH + r"\s+\d{1,2},\s+20\d\d)\.?")
CASES = r"[ACE]-\d{3}-\d{3}"
ENTRY = re.compile(r"\n(?=" + CASES + r"(?: and " + CASES + r")*:)")


def get(url, cache, key):
    p = os.path.join(cache, key)
    if os.path.exists(p):
        return open(p, "rb").read()
    for i in range(5):
        try:
            b = urllib.request.urlopen(urllib.request.Request(url, headers=UA), timeout=90).read()
            os.makedirs(cache, exist_ok=True)
            open(p, "wb").write(b)
            return b
        except Exception:
            time.sleep(2 * (i + 1))
    raise RuntimeError("fetch failed: " + url)


def notices(cache, after):
    out, page = [], 1
    while True:
        q = {"conditions[term]": '"Notice of Scope Rulings"', "conditions[agencies][]": "international-trade-administration",
             "conditions[type][]": "NOTICE", "conditions[publication_date][gte]": after, "per_page": 100, "page": page, "order": "newest",
             "fields[]": ["title", "document_number", "publication_date"]}
        d = json.loads(get(API + urllib.parse.urlencode(q, doseq=True, safe="[]"), cache, f"sr-{after}-p{page}.json"))
        out += [x for x in d.get("results", []) if x["title"] == "Notice of Scope Rulings"]
        if page >= d.get("total_pages", 1):
            return sorted(out, key=lambda x: x["publication_date"])
        page += 1


def text_of(doc, cache):
    url = f"https://www.govinfo.gov/content/pkg/FR-{doc['publication_date']}/html/{doc['document_number']}.htm"
    raw = get(url, cache, f"doc-{doc['document_number']}.txt")
    return html.unescape(re.sub(r"<[^>]+>", "", raw.decode("utf8", "replace"))), hashlib.sha256(raw).hexdigest()


def parse(text):
    """[(cases, title, requestor, body, ruling_date)] of the final scope rulings in a notice."""
    # notices from 2025 head the section "Final Scope Rulings"; earlier ones "Scope Rulings Made" and add
    # sections of preliminary rulings and of circumvention determinations after it, which are not final scope rulings
    m = re.search(r"^(?:Final )?Scope Rulings Made", text, re.M)
    if "Final Scope Rulings" in text:
        start = text.index("Final Scope Rulings")
    elif m:
        start = m.start()
    else:
        return []
    body = text[start:].split("Notification to Interested Parties")[0]
    # the notices of 2017-2018 end the list with an invitation to comment, which would trail the last entry's date
    body = re.split(r"\n\s*Interested parties are invited", body)[0]
    if "Final Scope Rulings" not in text:
        cut = re.search(r"^(?:Preliminary Scope Rulings? Made|Preliminary Scope Ruling/|Circumvention Determinations? Made|Scope Ruling/Circumvention|Final Scope Ruling/Circumvention)", body, re.M)
        if cut:
            body = body[:cut.start()]
    body = re.sub(r"\n\[\[Page \d+\]\]\n", "\n", body)
    out = []
    for e in ENTRY.split(body)[1:]:
        flat = re.sub(r"\s+", " ", e).strip()
        head = re.match(r"^((?:" + CASES + r")(?: and " + CASES + r")*): (.+?) Request(?:or|er)s?: (.+)$", flat)
        # the ruling's date ends the entry; only a country heading may follow it
        dates = [m for m in DATE.finditer(flat) if len(flat[m.end():].strip()) < 70]
        if not head or not dates:
            out.append((None, flat[:80], None, flat, None))
            continue
        last = dates[0]
        cases = re.findall(CASES, head.group(1))
        cases = [re.sub(r"^E-", "C-", c) for c in cases]
        title, rest = head.group(2), flat[head.start(3):last.end()]
        requestor = re.split(r"\.\s", rest, maxsplit=1)[0]
        d = dt.datetime.strptime(re.sub(r"\s+", " ", last.group(1)), "%B %d, %Y").date().isoformat()
        out.append((cases, title, requestor, rest[:last.end() - head.start(3)], d))
    return out


def entries(after, out, cache, before=None):
    rows, bad = [], []
    for doc in notices(cache, after):
        text, sha = text_of(doc, cache)
        for i, (cases, title, req, body, d) in enumerate(parse(text), 1):
            if cases is None:
                bad.append((doc["document_number"], title))
                continue
            mc = re.search(r"\b[Ff]rom (?:the )?(.+)$", title)
            rows.append({"id": f"{doc['document_number']}-{i}", "notice": doc["document_number"], "published": doc["publication_date"], "sha256": sha,
                         "url": f"https://www.federalregister.gov/d/{doc['document_number']}", "cases": cases, "title": title,
                         "order_country": mc.group(1).strip() if mc else None, "requestor": req, "text": body, "ruling_date": d})
    kept = [r for r in rows if r["ruling_date"] >= after and (before is None or r["ruling_date"] < before)]
    with open(out, "w") as f:
        for r in kept:
            f.write(json.dumps(r, ensure_ascii=False) + "\n")
    print(json.dumps({"notices": len({r["notice"] for r in rows}), "entries": len(rows), "kept_on_or_after": after, "kept": len(kept), "unparsed": bad}))


def check(path, cache):
    rows = [json.loads(l) for l in open(path)]
    bad = 0
    byn = {}
    for r in rows:
        byn.setdefault(r["notice"], []).append(r)
    for n, rs in byn.items():
        doc = {"document_number": n, "publication_date": rs[0]["published"]}
        text, sha = text_of(doc, cache)
        if sha != rs[0]["sha256"]:
            print("NOTICE CHANGED", n)
            bad += 1
        got = {(tuple(c), t, d) for c, t, _, _, d in parse(text) if c}
        for r in rs:
            if (tuple(r["cases"]), r["title"], r["ruling_date"]) not in got:
                print("ENTRY NOT REPRODUCED", r["id"])
                bad += 1
    print("entries", len(rows), "problems", bad)
    sys.exit(1 if bad else 0)


# ----------------------------------------------------------------- labels


def chat(model, system, user, base, key, retries=4):
    body = json.dumps({"model": model, "temperature": 0, "response_format": {"type": "json_object"},
                       "messages": [{"role": "system", "content": system}, {"role": "user", "content": user}]}).encode()
    for i in range(retries):
        try:
            req = urllib.request.Request(base + "/v1/chat/completions", data=body,
                                         headers={"Content-Type": "application/json", "Authorization": "Bearer " + key})
            d = json.load(urllib.request.urlopen(req, timeout=120))
            return json.loads(d["choices"][0]["message"]["content"]), d.get("usage", {})
        except urllib.error.HTTPError as e:
            if e.code in (400, 401, 403, 404):
                raise RuntimeError(f"gateway refused the call ({e.code})")
            time.sleep(3 * (i + 1))
        except Exception:
            time.sleep(3 * (i + 1))
    raise RuntimeError("gateway call failed")


def label(rulings, out):
    base, key = os.environ["LITELLM_BASE_URL"].rstrip("/"), os.environ["LITELLM_API_KEY"]
    pr = json.load(open(os.path.join(os.path.dirname(os.path.abspath(rulings)), "prompts.json")))
    rows = [json.loads(l) for l in open(rulings)]
    done = {json.loads(l)["id"] for l in open(out)} if os.path.exists(out) else set()
    tok = {"prompt": 0, "completion": 0}
    with open(out, "a") as f:
        for r in rows:
            if r["id"] in done:
                continue
            a, u1 = chat(pr["model"], pr["extract_system"], pr["extract_user"].format(text=r["text"]), base, key)
            b, u2 = chat(pr["model"], pr["hts_system"], pr["hts_user"].format(description=a.get("description", "")), base, key)
            for u in (u1, u2):
                tok["prompt"] += u.get("prompt_tokens", 0)
                tok["completion"] += u.get("completion_tokens", 0)
            f.write(json.dumps({"id": r["id"], "outcome": a.get("outcome"), "description": a.get("description"), "turns_on_origin": a.get("turns_on_origin"),
                                "hts": b.get("hts"), "hts_note": b.get("note"), "model": pr["model"], "prompt_version": pr["version"]}, ensure_ascii=False) + "\n")
            f.flush()
    print(json.dumps(tok))


if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    sub = ap.add_subparsers(dest="cmd", required=True)
    e = sub.add_parser("entries")
    e.add_argument("--after", required=True)
    e.add_argument("--before", default=None, help="keep only rulings dated before this day (default: no upper bound)")
    e.add_argument("--out", required=True)
    e.add_argument("--cache", default=os.path.expanduser("~/.cache/origin-proof-adcvd"))
    l = sub.add_parser("label")
    l.add_argument("rulings")
    l.add_argument("out")
    c = sub.add_parser("check")
    c.add_argument("rulings")
    c.add_argument("--cache", default=os.path.expanduser("~/.cache/origin-proof-adcvd"))
    a = ap.parse_args()
    if a.cmd == "entries":
        entries(a.after, a.out, a.cache, a.before)
    elif a.cmd == "label":
        label(a.rulings, a.out)
    else:
        check(a.rulings, a.cache)
