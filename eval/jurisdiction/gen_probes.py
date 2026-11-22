"""Writes probes-model.jsonl: synthetic non-U.S. origin questions written by a
model, not by the detector's author (D680). Run once, before scoring.

    set -a; . <your gateway env>; set +a
    python3 -I eval/jurisdiction/gen_probes.py eval/jurisdiction/probes-model.jsonl

Needs LITELLM_BASE_URL and LITELLM_API_KEY (an OpenAI-compatible LiteLLM
gateway to Gemini). Prints counts and cost only."""
import json, os, sys, urllib.request

MODEL = "gemini-3.8-flash"
MARKETS = [("eu", "the European Union or one of its member states", 6), ("uk", "the United Kingdom", 4),
           ("ca", "Canada", 4), ("jp", "Japan", 3), ("kr", "South Korea", 3), ("au", "Australia", 3),
           ("mx", "Mexico", 3), ("other", "a country other than the United States, EU members, the UK, Canada, Japan, South Korea, Australia and Mexico (name it)", 4)]
PROMPT = """Write {n} short, varied questions a business might ask about the customs country of origin of a manufactured or processed product it imports into {market}.
Each question names the product, where its main parts or materials come from, and where it is processed or assembled. Make clear, in different ways, that the goods are imported into {market}: name the country or a city, the customs authority, a local law or tariff, a local currency, or a local sales channel. Vary the wording and length (20 to 70 words). Do not mention the United States as the destination.
Return JSON: {{"questions": ["...", ...]}}"""


def call(prompt):
    body = json.dumps({"model": MODEL, "messages": [{"role": "user", "content": prompt}],
                       "response_format": {"type": "json_object"}, "temperature": 1.0}).encode()
    req = urllib.request.Request(os.environ["LITELLM_BASE_URL"].rstrip("/") + "/chat/completions", data=body,
                                 headers={"Authorization": "Bearer " + os.environ["LITELLM_API_KEY"], "Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=120) as r:
        cost = float(r.headers.get("x-litellm-response-cost") or 0)
        out = json.load(r)
    return json.loads(out["choices"][0]["message"]["content"])["questions"], cost, out.get("usage", {})


def main(path):
    rows, total = [], 0.0
    for code, market, n in MARKETS:
        qs, cost, usage = call(PROMPT.format(n=n, market=market))
        total += cost
        for i, q in enumerate(qs[:n]):
            rows.append({"id": f"m-{code}-{i+1:02d}", "want": code, "text": q, "model": MODEL})
        print(code, len(qs[:n]), "cost", round(cost, 6), usage)
    with open(path, "w") as f:
        for r in rows:
            f.write(json.dumps(r, ensure_ascii=False) + "\n")
    print("rows", len(rows), "total_cost_usd", round(total, 6))


if __name__ == "__main__":
    main(sys.argv[1])
