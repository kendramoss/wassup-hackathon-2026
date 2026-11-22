"""English translation of the Japan Customs origin rulings, for retrieval and display only (D710, D711).

Usage: python3 -I -B eval/jp/translate.py <repo> [--limit N] [--ids a,b,...]

Model calls go to an OpenAI-compatible LiteLLM gateway to Gemini (LITELLM_BASE_URL,
LITELLM_API_KEY in the environment). Writes data/jp/translations-en.jsonl, one row
per ruling, resumable. The Japanese original stays in data/jp/rulings.jsonl and is
the only source of labels; a translation is never a label.
"""
import concurrent.futures as cf, json, os, sys, time, urllib.request

MODEL = "gemini-3.8-flash"
VERSION = "jp-translate-v1"
SYS = """You translate Japan Customs advance rulings on origin from Japanese into plain, faithful English for customs professionals.
Rules: translate every field completely; do not summarise, add, or drop facts. Keep tariff numbers, chapter/heading/subheading numbers, article and annex numbers, percentages and circled item numbers (①②...) exactly. Translate agreement names to their usual English names (日EU経済連携協定 = Japan-EU EPA; TPP11協定 = CPTPP; RCEP協定 = RCEP; 日米貿易協定 = Japan-US Trade Agreement; 日アセアン包括的経済連携協定 = AJCEP; 一般特恵 = GSP). Render rule terms literally (類 = chapter, 項 = heading, 号 = subheading; 品目別原産地規則 = product-specific rules of origin; 原産材料 = originating materials; 非原産材料 = non-originating materials). Keep quoted rule text in double quotes. An empty field stays empty.
Return only JSON with the same keys."""
KEYS = ["product", "inputs", "process", "reasoning", "articles", "answer", "other"]
SCHEMA = {"type": "object", "properties": {k: {"type": "string"} for k in KEYS}, "required": KEYS, "additionalProperties": False}


def source(r):
    inputs, process = r["inputs_ja"], r["process_ja"]
    if not inputs and not process:
        inputs = r["summary_ja"]
    return {"product": r["product_ja"], "inputs": inputs, "process": process, "reasoning": r["reasoning_ja"],
            "articles": r["articles_ja"], "answer": r["answer_ja"], "other": r["other_ja"]}


def one(r):
    base = os.environ["LITELLM_BASE_URL"].rstrip("/")
    key = os.environ["LITELLM_API_KEY"]
    body = {"model": MODEL, "temperature": 0, "seed": 20261017, "reasoning_effort": "none", "max_tokens": 6000,
            "messages": [{"role": "system", "content": SYS},
                         {"role": "user", "content": json.dumps(source(r), ensure_ascii=False)}],
            "response_format": {"type": "json_schema", "json_schema": {"name": "translation", "schema": SCHEMA}}}
    err = ""
    for a in range(5):
        t0 = time.time()
        try:
            req = urllib.request.Request(base + "/chat/completions", data=json.dumps(body).encode(),
                                         headers={"Authorization": "Bearer " + key, "Content-Type": "application/json"})
            with urllib.request.urlopen(req, timeout=180) as resp:
                cost = float(resp.headers.get("x-litellm-response-cost") or 0)
                d = json.load(resp)
            en = json.loads(d["choices"][0]["message"]["content"])
            u = d.get("usage") or {}
            return {"id": r["id"], "model": MODEL, "version": VERSION, "en": {k: en.get(k, "") for k in KEYS},
                    "cost_usd": cost, "tokens_in": u.get("prompt_tokens"), "tokens_out": u.get("completion_tokens"),
                    "latency_ms": int((time.time() - t0) * 1000)}
        except Exception as e:
            err = str(e)[:200].replace(base, "<gateway>")
            time.sleep(3 * (a + 1))
    return {"id": r["id"], "error": err, "cost_usd": 0}


def main():
    repo = sys.argv[1]
    rulings = [json.loads(l) for l in open(os.path.join(repo, "data/jp/rulings.jsonl"), encoding="utf-8")]
    outp = os.path.join(repo, "data/jp/translations-en.jsonl")
    done = {}
    if os.path.exists(outp):
        for l in open(outp, encoding="utf-8"):
            x = json.loads(l)
            if "error" not in x:
                done[x["id"]] = x
    todo = [r for r in rulings if r["id"] not in done]
    if "--ids" in sys.argv:
        want = set(sys.argv[sys.argv.index("--ids") + 1].split(","))
        todo = [r for r in todo if r["id"] in want]
    if "--limit" in sys.argv:
        todo = todo[: int(sys.argv[sys.argv.index("--limit") + 1])]
    new = []
    with cf.ThreadPoolExecutor(8) as ex:
        for o in ex.map(one, todo):
            new.append(o)
            if "error" not in o:
                done[o["id"]] = o
    order = [r["id"] for r in rulings]
    with open(outp, "w", encoding="utf-8") as f:
        for i in order:
            if i in done:
                f.write(json.dumps(done[i], ensure_ascii=False, sort_keys=True) + "\n")
    print("new", len(new), "errors", sum("error" in o for o in new), "cost_this_run",
          round(sum(o["cost_usd"] for o in new), 4), "rows", len(done))


if __name__ == "__main__":
    main()
