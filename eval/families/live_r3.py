"""Live predict + plan checks for the D660 families, one fixture each (D661).

Reads the committed plan results (eval/results/plan-<case>-local-verifier-v2-d660-litellm.json,
`cd backend && go run ./cmd/eval plan -fixture testdata/families/<fixture>.json -tag d660`
through an OpenAI-compatible LiteLLM gateway to Gemini) and writes
eval/results/families-r3-live-checks.json. The hand notes below say whether the
ruling cited by the best shown plan supports the move, read by the agent. No model
calls. Development checks on fixtures from pool rulings, one run each; not an
evaluation.

    python3 -I -B eval/families/live_r3.py .          # write
    python3 -I -B eval/families/live_r3.py . --check  # verify
"""
import json
import os
import sys

OUT = "eval/results/families-r3-live-checks.json"
CASES = {
    "paper-napkins-2024": ("Indonesia", "reverse probe", "N340077 (tissue, napkins and towels converted from Indonesian parent rolls, held Indonesia) supports it: the parent roll decides."),
    "roll-agv-2023": ("China", "China-held", "N334276 (scissor lifts: two of three structural subassemblies made in Poland, held Poland) supports moving both frames, by analogy. Backtest: in N337718 CBP held South Korea with a Korean handling frame and roll tray in the upper frame; the plan's single working-frame move was supported at 0.90, below the 0.92 bar, so the shown plan moves more than CBP's case did."),
    "integrated-circuit-2023": ("South Korea", "reverse probe", "N333677 (a silicon pressure sensor whose die is made in the United States, held United States) supports front-end fabrication deciding origin; it is a sensor, not an integrated circuit: partial."),
    "golf-driver-heads-2025": ("Vietnam", "reverse probe", "N340536 (putter heads formed in Vietnam and finished in China, held Vietnam) supports it: the forming country decides."),
    "plush-dinosaur-2024": ("China", "China-held", "No plan shown. Backtest: CBP held Vietnam in scenario 2 of N343777, where the cutting and sewing move to Vietnam; the plan found that move (engine Vietnam) but the check gave 0.90, below the 0.92 bar, citing N342448 (a Part 102 textile ruling)."),
    "gas-heater-2025": ("China", "China-held", "N346005 (vent-free gas logs and a stove with a Chinese burner and log set, assembled in Vietnam, held China) supports the burner and sensor assemblies deciding origin."),
}


def build(root):
    runs = []
    for case, (cbp, kind, note) in CASES.items():
        rel = f"eval/results/plan-{case}-local-verifier-v2-d660-litellm.json"
        with open(os.path.join(root, rel)) as f:
            d = json.load(f)
        shown = [p for p in d["plans"] if p.get("shown")]
        best = shown[0] if shown else None
        base = d["baseline"]["prediction"]
        runs.append({
            "case_id": case, "family": d["family"], "target": d["target"], "setup": kind, "cbp_current_origin": cbp,
            "engine_current_origin": base["origin_country"], "engine_current_confidence": base["confidence"],
            "moves_searched": d["stats"]["moves_searched"], "shown_plans": len(shown),
            "best_plan": best["description"] if best else None,
            "best_supporting_ruling": best["verdict"]["supporting_ruling"] if best else None,
            "best_verifier_confidence": best["verdict"]["confidence"] if best else None,
            "outcome": "precedent-supported plan" if best else "send to a broker",
            "hand_note": note,
            "gateway_cost_usd": round(d["gateway_cost_usd"], 4), "cost_usd_promo": round(d["cost_usd_promo"], 4),
            "wall_seconds": round(d["stats"]["wall_seconds"], 1), "template_version": d["template_versions"][d["family"]],
            "result_file": rel,
        })
    walls = sorted(r["wall_seconds"] for r in runs)
    return {
        "what": "Live predict + plan checks, one fixture per D660 family, through an OpenAI-compatible LiteLLM gateway to Gemini; "
                "production engine and verifier defaults, verifier-v2, local corpus search, plan bar 0.92. Development checks on fixtures "
                "built from pool rulings, one run each, not an evaluation. Duty rates and costs are illustrative.",
        "precedent_supported": sum(1 for r in runs if r["shown_plans"]),
        "send_to_broker": sum(1 for r in runs if not r["shown_plans"]),
        "total_gateway_cost_usd": round(sum(r["gateway_cost_usd"] for r in runs), 4),
        "wall_seconds_median": round((walls[2] + walls[3]) / 2, 1), "wall_seconds_max": walls[-1],
        "runs": runs,
    }


if __name__ == "__main__":
    root = sys.argv[1]
    want = json.dumps(build(root), indent=1) + "\n"
    path = os.path.join(root, OUT)
    if "--check" in sys.argv:
        with open(path) as f:
            if f.read() != want:
                sys.exit(OUT + " is stale: run eval/families/live_r3.py")
        print(OUT, "ok")
    else:
        with open(path, "w") as f:
            f.write(want)
        print("wrote", OUT)
