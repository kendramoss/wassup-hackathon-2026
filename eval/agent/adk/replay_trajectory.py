"""Scores recorded agent runs with ADK's own tool-trajectory metric (D511).

No model, no network: it rebuilds each recorded run's tool calls as an ADK
Invocation and asks google-adk's TrajectoryEvaluator, configured by
test_config.json (IN_ORDER, arguments ignored), to compare it with the
expected trajectory in unde_agent.evalset.json. It needs google-adk
(examples/adk-agent/requirements.txt), so it is not in make reproduce.

    python eval/agent/adk/replay_trajectory.py .          # write replay.json
    python eval/agent/adk/replay_trajectory.py . --check  # verify
"""
import glob
import json
import os
import sys

from google.adk.evaluation.eval_case import IntermediateData, Invocation
from google.adk.evaluation.eval_config import EvalConfig
from google.adk.evaluation.eval_metrics import EvalMetric
from google.adk.evaluation.eval_set import EvalSet
from google.adk.evaluation.trajectory_evaluator import TrajectoryEvaluator
from google.genai import types

REPO = sys.argv[1]
HERE = os.path.join(REPO, "eval", "agent", "adk")
OUT = os.path.join(HERE, "replay.json")
LIVE = ["live-gemini-3.8-flash-undelabs", "live-v2-run1", "live-v2-run2"]


def main():
    es = EvalSet.model_validate_json(open(os.path.join(HERE, "unde_agent.evalset.json")).read())
    cfg = EvalConfig.model_validate_json(open(os.path.join(HERE, "test_config.json")).read())
    metric = "tool_trajectory_avg_score"
    ev = TrajectoryEvaluator(eval_metric=EvalMetric(metric_name=metric, criterion=cfg.criteria[metric]))
    expected = {c.eval_id.split("_")[0]: c.conversation[0] for c in es.eval_cases}
    out = {"metric": metric, "criterion": cfg.criteria[metric].model_dump(mode="json"), "runs": {}}
    for label in LIVE:
        path = os.path.join(REPO, "eval", "agent", "results", label + ".jsonl")
        if not os.path.exists(path):
            continue
        scores = {}
        for line in open(path):
            row = json.loads(line)
            exp = expected[row["id"]]
            act = Invocation(user_content=exp.user_content, intermediate_data=IntermediateData(
                tool_uses=[types.FunctionCall(name=c["tool"], args={}) for c in row["calls"]]))
            scores[row["id"]] = ev.evaluate_invocations([act], [exp]).overall_score
        scores = dict(sorted(scores.items()))
        out["runs"][label] = {"scores": scores, "passed": sum(1 for v in scores.values() if v >= 1.0), "cases": len(scores)}
    text = json.dumps(out, indent=1) + "\n"
    if "--check" in sys.argv:
        if not os.path.exists(OUT) or open(OUT).read() != text:
            sys.exit("replay.json differs from a fresh computation")
        print("ADK trajectory replay matches replay.json")
        return
    open(OUT, "w").write(text)
    print(json.dumps({k: f"{v['passed']}/{v['cases']}" for k, v in out["runs"].items()}))


if __name__ == "__main__":
    main()
