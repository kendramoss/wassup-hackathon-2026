"""Run the unchanged production extractor through mediacheck, one immutable file per image."""
import argparse
import concurrent.futures
import hashlib
import json
import math
import os
import pathlib
import subprocess
import threading
import urllib.parse

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parents[2]


def redact(value):
    if isinstance(value, dict):
        return {k: redact(v) for k, v in value.items()}
    if isinstance(value, list):
        return [redact(v) for v in value]
    if isinstance(value, str):
        for key in ("LITELLM_API_KEY", "LITELLM_BASE_URL"):
            secret = os.environ.get(key, "")
            if secret:
                value = value.replace(secret, "[redacted]")
                if key.endswith("BASE_URL"):
                    host = urllib.parse.urlparse(secret).hostname
                    if host:
                        value = value.replace(host, "[redacted]")
    return value


def validate(dataset, candidate):
    cases = dataset["cases"]
    if len(candidate) != 40 or any(c not in "0123456789abcdef" for c in candidate):
        raise ValueError("full frozen candidate required")
    ids = [c["id"] for c in cases]
    if len(ids) != len(set(ids)):
        raise ValueError("duplicate case id")
    dev = {c["source_family"] for c in cases if c["split"] == "dev"}
    test = {c["source_family"] for c in cases if c["split"] == "test"}
    if dev & test or sum(c["split"] == "test" for c in cases) < 100:
        raise ValueError("reserved source families or count invalid")
    images, content = set(), {"dev": set(), "test": set()}
    for case in cases:
        image = case["image"]
        if pathlib.Path(image).name != image or image in images:
            raise ValueError("unsafe or repeated image")
        images.add(image)
        data = (HERE/"images"/image).read_bytes()
        if hashlib.sha256(data).hexdigest() != case["image_sha256"]:
            raise ValueError("image hash differs")
        names = [" ".join(p["name"].casefold().split()) for p in case["components"]]
        if len(names) != len(set(names)) or not names:
            raise ValueError("duplicate or empty expected component names")
        content[case["split"]].add(json.dumps(case["components"], sort_keys=True))
        source = case["source"]
        path = ROOT/source["path"]
        if not path.resolve().is_relative_to(ROOT) or hashlib.sha256(path.read_bytes()).hexdigest() != source["sha256"]:
            raise ValueError("source fixture changed")
    if content["dev"] & content["test"]:
        raise ValueError("development/test content duplicate")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--split", required=True, choices=["dev", "test"])
    parser.add_argument("--binary", required=True, type=pathlib.Path)
    parser.add_argument("--candidate", required=True)
    parser.add_argument("--max-usd", type=float, required=True)
    parser.add_argument("--workers", type=int, default=3, choices=[1,2,3])
    parser.add_argument("--resume", action="store_true")
    args = parser.parse_args()
    if os.environ.get("LLM_PROVIDER") != "litellm" or not all(os.environ.get(k) for k in ("LITELLM_API_KEY", "LITELLM_BASE_URL")):
        raise ValueError("approved gateway process settings required")
    if not math.isfinite(args.max_usd) or not 0 < args.max_usd <= 15:
        raise ValueError("run allowance must be positive and at most 15")
    if subprocess.check_output(["git","status","--porcelain","--untracked-files=no"],cwd=ROOT,text=True).strip():
        raise ValueError("tracked tree must be clean before model calls")
    current = subprocess.check_output(["git","rev-parse","HEAD"],cwd=ROOT,text=True).strip()
    if subprocess.run(["git","diff","--quiet",args.candidate,current,"--","backend"],cwd=ROOT).returncode:
        raise ValueError("candidate backend changed before scoring")
    dataset = json.loads((HERE/"cases.json").read_text())
    validate(dataset,args.candidate)
    if args.split == "test":
        freeze = json.loads((HERE/"freeze.json").read_text())
        if freeze["candidate_revision"] != args.candidate or freeze["dataset_sha256"] != hashlib.sha256((HERE/"cases.json").read_bytes()).hexdigest():
            raise ValueError("test freeze mismatch")
        committed = subprocess.check_output(["git","show","HEAD:eval/challenges/media/freeze.json"],cwd=ROOT)
        if committed != (HERE/"freeze.json").read_bytes():
            raise ValueError("test freeze must be committed")
    out = HERE/"results"/args.split
    out.mkdir(parents=True,exist_ok=True)
    selected = [c for c in dataset["cases"] if c["split"] == args.split]
    state = {"charged":0.0,"pending":0,"actual":0.0,"unreported":0,"blocked":0}
    lock = threading.Lock()
    def work(case):
        path = out/(case["id"]+".json")
        if path.exists():
            if not args.resume:
                raise ValueError("prior document result exists; refusing overwrite")
            data = json.loads(path.read_text())
            if data.get("candidate_revision") != args.candidate or data.get("image_sha256") != case["image_sha256"]:
                raise ValueError("resume provenance mismatch")
            return
        with lock:
            if state["charged"] + (state["pending"]+1)*.5 > args.max_usd:
                state["blocked"] += 1
                return
            state["pending"] += 1
        temp = out/(case["id"]+".pending")
        if temp.exists():
            raise ValueError("ambiguous prior invocation; inspect before retry")
        temp.touch(exist_ok=False)
        try:
            proc = subprocess.run([str(args.binary.resolve()),"-dir",str(HERE/"images"),"-only",case["image"],"-out",str(temp)],cwd=ROOT/"backend",stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=180)
            if proc.returncode != 0:
                raise RuntimeError("mediacheck process failed; private output retained only in memory")
            data = json.loads(temp.read_text())
            if data.get("git_revision") != args.candidate or data.get("provider") != "litellm" or len(data.get("scores",[])) != 1 or data["scores"][0]["image"] != case["image"]:
                raise ValueError("binary or output provenance mismatch")
            row = data["scores"][0]
            cost = row.get("cost_usd_gateway",0)
            if not isinstance(cost,(int,float)) or not math.isfinite(cost) or cost < 0:
                raise ValueError("invalid provider cost")
            data.update(candidate_revision=args.candidate,image_sha256=case["image_sha256"],dataset_sha256=hashlib.sha256((HERE/"cases.json").read_bytes()).hexdigest())
            data = redact(data)
            with path.open("x") as f:
                json.dump(data,f,ensure_ascii=False,sort_keys=True,indent=2); f.write("\n")
            temp.unlink()
            with lock:
                state["pending"] -= 1
                state["actual"] += cost
                state["charged"] += cost if cost > 0 else .5
                state["unreported"] += int(cost == 0)
            print(json.dumps({"id":case["id"],"error":bool(row.get("error")),"cost_usd":cost}),flush=True)
        except BaseException:
            with lock:
                state["pending"] -= 1
                state["charged"] += .5
            raise
    with concurrent.futures.ThreadPoolExecutor(max_workers=args.workers) as pool:
        list(pool.map(work, selected))
    print(json.dumps({"run":args.split,**state}),flush=True)
    if state["blocked"]:
        raise RuntimeError("run allowance reached; no unattempted case is reported as scored")


if __name__ == "__main__":
    main()
