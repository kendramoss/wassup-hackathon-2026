"""Source-grounded image robustness study; scoring never calls the extractor."""
import argparse
import collections
import hashlib
import json
import math
import pathlib
import re
import statistics
import unicodedata

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parents[2]
ALIASES = {"cn": "china", "prc": "china", "th": "thailand", "vn": "vietnam", "kr": "south korea", "korea": "south korea", "republic of korea": "south korea", "id": "indonesia", "jp": "japan", "my": "malaysia", "mx": "mexico", "tw": "taiwan", "us": "united states", "usa": "united states"}
STYLES = ["table", "reordered", "list", "dense", "low_contrast", "skew", "scan", "jpeg", "missing_origin", "quantity_and_codes", "bilingual_headers", "process_footer"]
DEV_FAMILIES = {"pcr_analyzer_2023", "roll_agv_2023", "power_bank_2025"}


def dump(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, indent=2) + "\n"


def sha(data):
    return hashlib.sha256(data).hexdigest()


def norm(value):
    return " ".join(unicodedata.normalize("NFKC", value).casefold().split())


def country(value):
    value = norm(value)
    return ALIASES.get(value, value)


def score(case, result):
    if result.get("provider") != "litellm":
        raise ValueError("only measured approved-gateway outputs may be scored")
    if len(result.get("scores", [])) != 1:
        raise ValueError("one complete document result required")
    actual = result["scores"][0]
    if actual.get("simulated") is True:
        raise ValueError("simulated output cannot be measured evidence")
    expected = case["components"]
    known = [r for r in expected if r["country"]]
    unknown = [r for r in expected if not r["country"]]
    row = {"id": case["id"], "matched": False, "error": bool(actual.get("error")), "expected_names": len(expected), "expected_countries": len(known), "unknown_expected": len(unknown), "names_read": 0, "countries_right": 0, "unknown_withheld": 0, "unknown_documented": 0, "extra_rows": 0, "wrong_countries": 0, "cost_usd": actual.get("cost_usd_gateway", 0), "latency_ms": actual.get("latency_ms", 0)}
    if row["error"]:
        return row
    got = collections.defaultdict(list)
    for r in actual.get("read", []):
        got[norm(r["name"])].append(country(r["country"]))
    warnings = norm(" ".join(actual.get("warnings", [])))
    matched_outputs = 0
    for part in known:
        values = got.get(norm(part["name"]), [])
        if values:
            row["names_read"] += 1
            matched_outputs += 1
            if values[0] == country(part["country"]):
                row["countries_right"] += 1
            else:
                row["wrong_countries"] += 1
    for part in unknown:
        name = norm(part["name"])
        if not got.get(name):
            row["unknown_withheld"] += 1
        else:
            row["wrong_countries"] += 1
        if name in warnings and "without saying where" in warnings:
            row["unknown_documented"] += 1
            row["names_read"] += 1
    row["extra_rows"] = sum(map(len, got.values())) - matched_outputs
    row["matched"] = (row["names_read"] == len(expected) and row["countries_right"] == len(known) and row["unknown_withheld"] == len(unknown) and row["extra_rows"] == 0)
    return row


def validate_output(case, result, candidate, dataset_sha, prompt_version="extract-image-v2"):
    if result.get("provider") != "litellm" or result.get("candidate_revision") != candidate or result.get("git_revision") != candidate:
        raise ValueError("output candidate/provider mismatch")
    if result.get("image_sha256") != case["image_sha256"] or result.get("dataset_sha256") != dataset_sha:
        raise ValueError("output image/dataset hash mismatch")
    if len(result.get("scores", [])) != 1 or result["scores"][0].get("image") != case["image"]:
        raise ValueError("output document identity mismatch")
    row = result["scores"][0]
    if result.get("model") != "gemini-4-argon" or row.get("model_id") != "gemini-4-argon" or row.get("prompt_version") != prompt_version or row.get("simulated") is not False:
        raise ValueError("output model/prompt/simulation mismatch")
    cost = row.get("cost_usd_gateway", 0)
    if not isinstance(cost, (int, float)) or not math.isfinite(cost) or cost < 0:
        raise ValueError("invalid output cost")
    if abs(result["summary"]["cost_usd_gateway"] - cost) > 1e-9:
        raise ValueError("output cost totals disagree")


def aggregate(cases, rows):
    ids = [c["id"] for c in cases]
    got = [r["id"] for r in rows]
    if len(set(ids)) != len(ids) or len(set(got)) != len(got) or set(ids) != set(got):
        raise ValueError("missing, duplicate or unexpected document results")
    keys = ["expected_names", "expected_countries", "unknown_expected", "names_read", "countries_right", "unknown_withheld", "unknown_documented", "extra_rows", "wrong_countries"]
    totals = {key: sum(r[key] for r in rows) for key in keys}
    totals.update(cases=len(cases), matched=sum(r["matched"] for r in rows), errors=sum(r["error"] for r in rows))
    totals["mismatched"] = totals["cases"] - totals["matched"] - totals["errors"]
    return totals


def source_cases():
    out = []
    for path in sorted((ROOT / "backend/internal/demos/fixtures").glob("*.json")):
        raw = path.read_bytes()
        data = json.loads(raw)
        product = (data.get("predict") or {}).get("product") or {}
        components = product.get("components", [])
        if not components:
            continue
        source = {"path": str(path.relative_to(ROOT)), "sha256": sha(raw), "reference": data["source"]}
        split = "dev" if path.stem in DEV_FAMILIES else "test"
        for variant, style in enumerate(STYLES[:8] if split == "dev" else STYLES):
            rows = [{"name": r["name"], "country": r["made_in"]} for r in components]
            if variant % 3 == 1 and len(rows) > 2:
                rows = rows[:-1]
            if variant % 2:
                rows.reverse()
            if style == "missing_origin":
                rows[-1]["country"] = ""
            identity = sha((path.stem + ":" + style).encode())[:20]
            out.append({"id": identity, "image": identity + (".jpg" if style == "jpeg" else ".png"), "split": split, "source_family": path.stem, "source": source, "layout_family": style, "product": product["name"], "components": rows, "tags": [style, "source_derived_components", "not_a_new_legal_case"], "rationale": "Expected names and origins are the text rendered on this synthetic sheet, derived from the cited fixture. Missing-origin rows must be withheld from confirmed components and named in a missing-country warning."})
    return out


def render(case):
    from PIL import Image, ImageDraw, ImageFilter, ImageFont, ImageOps
    import random
    style = case["layout_family"]
    size = 18 if style == "dense" else 23
    font = ImageFont.load_default(size=size)
    titlefont = ImageFont.load_default(size=25)
    width, pad = 1100, 32
    first_width = 780
    rows = case["components"]
    line = size + 9
    def wrap(text, limit):
        result, current = [], ""
        for word in text.split():
            trial = (current + " " + word).strip()
            if current and font.getlength(trial) > limit:
                result.append(current)
                current = word
            else:
                current = trial
        return result + [current]
    cells = [wrap(r["name"], first_width - 45) for r in rows]
    height = 175 + sum((len(v) + 1) * line for v in cells) + (90 if style in ("missing_origin", "process_footer") else 0)
    img = Image.new("RGB", (width, height), "white")
    draw = ImageDraw.Draw(img)
    ink = (100, 100, 100) if style == "low_contrast" else (20, 20, 20)
    heading = "Illustrative bill of materials"
    draw.text((pad, 20), heading, font=titlefont, fill=ink)
    draw.text((pad, 59), "Synthetic document; not a customs decision", font=font, fill=ink)
    left, right = ("Origin", "Component") if style == "reordered" else ("Component", "Country of origin")
    if style == "bilingual_headers":
        left, right = "Composant / Part", "Origine / Origin"
    draw.text((pad, 105), left, font=font, fill=ink)
    draw.text((first_width, 105), right, font=font, fill=ink)
    y = 148
    aliases = {"China": "CN", "Thailand": "TH", "Vietnam": "VN", "South Korea": "KR", "Japan": "JP", "Indonesia": "ID", "Malaysia": "MY", "Mexico": "MX", "Taiwan": "TW", "United States": "US"}
    for n, (part, lines) in enumerate(zip(rows, cells)):
        text_country = part["country"]
        if style == "quantity_and_codes":
            text_country = aliases.get(text_country, text_country)
            lines = wrap(f"{n+1:03d}  Qty {n+2}  |  {part['name']}", first_width-45)
        if style == "list":
            lines = wrap(part["name"] + ": made in " + text_country, width - pad*2)
            draw.multiline_text((pad, y), "\n".join(lines), font=font, fill=ink, spacing=9)
        elif style == "reordered":
            draw.text((pad, y), text_country, font=font, fill=ink)
            lines = wrap(part["name"], width-360)
            draw.multiline_text((330, y), "\n".join(lines), font=font, fill=ink, spacing=9)
        else:
            draw.multiline_text((pad, y), "\n".join(lines), font=font, fill=ink, spacing=9)
            draw.text((first_width, y), text_country, font=font, fill=ink)
        y += (len(lines) + 1) * line
        draw.line((pad, y-10, width-pad, y-10), fill=(190,190,190))
    if style == "missing_origin":
        draw.text((pad, y+6), "Supplier address: Tokyo, Japan (not a statement of part origin)", font=font, fill=ink)
    if style == "process_footer":
        draw.text((pad, y+6), "Process: final assembly and testing in Japan", font=font, fill=ink)
    if style == "skew":
        img = img.rotate(2.1, expand=True, fillcolor=(215,210,205))
        img = img.filter(ImageFilter.GaussianBlur(.4))
    if style == "scan":
        img = ImageOps.grayscale(img)
        img = img.resize((int(img.width*.65),int(img.height*.65)))
        rng = random.Random(int(case["id"],16))
        for _ in range(img.width*img.height//400):
            img.putpixel((rng.randrange(img.width),rng.randrange(img.height)),rng.choice([0,180,255]))
    return img


def generate(check=False):
    import PIL
    cases = source_cases()
    images = HERE / "images"
    if not check:
        images.mkdir(exist_ok=True)
    import io
    for case in cases:
        image = render(case)
        buffer = io.BytesIO()
        if case["image"].endswith(".jpg"):
            image.convert("RGB").save(buffer, format="JPEG", quality=68)
        else:
            image.save(buffer, format="PNG")
        data = buffer.getvalue()
        case["image_sha256"] = sha(data)
        path = images / case["image"]
        if check:
            if not path.is_file() or path.read_bytes() != data:
                raise ValueError("rendered image differs: " + case["id"])
        else:
            path.write_bytes(data)
    dataset = {"schema_version":1,"evidence_kind":"source_grounded_synthetic","renderer":{"pillow":PIL.__version__,"font":"Pillow bundled default"},"cases":cases}
    manifest = [{"image":c["image"],"source":c["source"]["path"],"style":c["layout_family"],"components":c["components"]} for c in cases]
    for path, value in [(HERE/"cases.json", dataset),(images/"manifest.json",manifest)]:
        data = dump(value)
        if check:
            if path.read_text() != data:
                raise ValueError("manifest differs")
        else:
            path.write_text(data)
    print({split:sum(c["split"]==split for c in cases) for split in ("dev","test")})


def summarize(check=False):
    dataset_path = HERE / "cases.json"
    dataset = json.loads(dataset_path.read_text())
    groups = {}
    allrows = []
    freeze = json.loads((HERE/"freeze.json").read_text())
    digest = sha(dataset_path.read_bytes())
    for run in ("dev-baseline", "dev", "test"):
        split = "dev" if run == "dev-baseline" else run
        cases = [c for c in dataset["cases"] if c["split"] == split]
        expected_candidate = freeze["development_selection"]["baseline_candidate"] if run == "dev-baseline" else freeze["candidate_revision"]
        directory = HERE / "results" / run
        if {p.stem for p in directory.glob("*.json")} != {c["id"] for c in cases}:
            raise ValueError("run has missing or extra document files")
        rows = []
        for c in cases:
            result = json.loads((directory / (c["id"] + ".json")).read_text())
            validate_output(c, result, expected_candidate, digest, "extract-image-v1" if run == "dev-baseline" else freeze["prompt"])
            rows.append(score(c, result))
        groups[run] = aggregate(cases, rows)
        allrows += rows
    test = [c for c in dataset["cases"] if c["split"]=="test"]
    test_ids = {c["id"] for c in test}
    latencies = sorted(r["latency_ms"] for r in allrows if r["id"] in test_ids and not r["error"])
    metrics = groups["test"]
    f = {"id":"media_extraction","label":"BOM image extraction","candidate_revision":freeze["candidate_revision"],"dataset_sha256":sha(dataset_path.read_bytes()),"source_families":len({c["source_family"] for c in test}),"development":groups["dev"],"test":metrics,"llm_provider":"litellm","cost_usd":sum(r["cost_usd"] for r in allrows),"latency_p50_ms":statistics.median(latencies) if latencies else 0,"latency_p95_ms":latencies[max(0,math.ceil(.95*len(latencies))-1)] if latencies else 0,"layout_families":len({c["layout_family"] for c in test}),"limitations":["Synthetic renderings of ruling-derived illustrative fixtures, not real photos, handwriting or independent customs cases. The original primary rulings were not newly reread for this image study.","Related documents share source components and layouts. Source families are disjoint across development and reserved test; eight rendering styles are shared, four are test-only.","Names use exact case/whitespace-normalized matching, not a model judge. Country aliases use a fixed map. Semantically equivalent paraphrases can therefore fail.","Whole-document agreement requires all expected confirmed parts and countries, no extra confirmed parts, and a warning plus no invented country for every missing-origin part. The production API withholds missing-country rows from confirmed components.","The language variation changes column headers only; the component names remain as in the English source fixtures."]}
    test_rows = {r["id"]: r for r in allrows if r["id"] in test_ids}
    size_groups = {}
    for name, low, high in [("one_or_two_parts", 1, 2), ("three_to_eleven_parts", 3, 11), ("twelve_plus_parts", 12, 10000)]:
        selected_cases = [c for c in test if low <= len(c["components"]) <= high]
        size_groups[name] = {"cases": len(selected_cases), "matched": sum(test_rows[c["id"]]["matched"] for c in selected_cases), "errors": sum(test_rows[c["id"]]["error"] for c in selected_cases)}
    f["document_sizes"] = size_groups
    small, large = size_groups["one_or_two_parts"], size_groups["twelve_plus_parts"]
    f["public_note"] = f'{small["cases"]}/{len(test)} sheets have only 1–2 parts; {large["matched"]}/{large["cases"]} sheets with 12+ parts matched.'
    f["limitations"].append("The missing-origin footer explicitly says the supplier address is not a statement of part origin. Withholding11/11 is on a self-labelled distractor in one layout, not an unlabelled-address challenge.")
    f["limitations"].append("The bundled font can leave a visible glyph gap before the closing parenthesis. The strict scorer still treats the model's inserted space as a mismatch, not a legal or wrong-origin error.")
    f["development_baseline"] = groups["dev-baseline"]
    f["model_calls"] = len(allrows)
    f["test_model_calls"] = len(test)
    f["cost_usd"] = round(f["cost_usd"], 6)
    f["test_cost_usd"] = round(sum(r["cost_usd"] for r in allrows if r["id"] in test_ids), 6)
    f["development_cost_usd"] = round(f["cost_usd"] - f["test_cost_usd"], 6)
    f["cost_unreported_calls"] = sum(r["cost_usd"] == 0 for r in allrows)
    f["latency_scope"] = "reserved documents, non-error calls"
    f["limitations"].append("All three reserved mismatches are an extra space before a closing parenthesis in the PCBA name; the three extracted rows are unmatched names, not independently established invented parts. Strict first scores remain unchanged.")
    out = dump({"schema_version":1,"evidence_kind":"source_grounded_synthetic","features":[f]})
    if check:
        if (HERE/"summary.json").read_text()!=out:
            raise ValueError("summary differs")
    else:
        (HERE/"summary.json").write_text(out)
        (HERE/"scored-test.json").write_text(dump([r for r in allrows if r["id"] in {c["id"] for c in test}]))
    print(dump(groups))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("command",choices=["generate","summarize"])
    parser.add_argument("--check",action="store_true")
    args = parser.parse_args()
    {"generate":generate,"summarize":summarize}[args.command](args.check)
