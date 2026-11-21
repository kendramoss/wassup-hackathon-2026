"""Render a second, less uniform set of synthetic BOM images (D168).

    python3 -I eval/media/render_varied.py    # from the repo root; needs Pillow, fontTools, brotli

Writes eval/media/images-varied/<name>.{png,jpg} and manifest.json. Every
image is drawn from part and country pairs already committed in the repo, so
the expected answer is known exactly:

- backend/testdata/heatgun_bom.csv, backend/testdata/lidar_bom.csv,
  frontend/public/samples/bom-template.csv, frontend/public/samples/heatgun-bom.csv
  (component rows only; plant and operation rows are not parts);
- the "actual_change" part lists of three demo fixtures in
  backend/internal/demos/fixtures (gas grill, 2026 heat gun, LED tub light):
  parts listed as moved to the new country carry that country, parts still
  from China carry China.

The styles vary what the first set (render_boms.py) kept fixed: layout (a
table, a bulleted spec list, reordered columns with part numbers and
quantities, a dense small-print grid), font (Paper Mono from the repo in
three weights, Pillow's bundled sans, a serif if the system has one; the
manifest records which), and capture (a rotated and shaded phone-photo style
saved as JPEG, a low-resolution grey scan with speckle, a page turned
sideways). Long names wrap instead of being cut. Randomness uses a fixed
seed. All images are synthetic renderings of illustrative BOMs, not customer
documents.
"""
import csv
import io
import json
import os
import random

from fontTools.ttLib import TTFont
from PIL import Image, ImageDraw, ImageFilter, ImageFont, ImageOps

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
OUT = os.path.join(ROOT, "eval", "media", "images-varied")
FIXTURES = os.path.join(ROOT, "backend", "internal", "demos", "fixtures")
SEED = 20261121
SERIFS = ["/System/Library/Fonts/Supplemental/Times New Roman.ttf",
          "/usr/share/fonts/truetype/dejavu/DejaVuSerif.ttf"]


def csv_parts(path):
    with open(os.path.join(ROOT, path), newline="") as f:
        out = []
        for r in csv.DictReader(f):
            r = {k.strip().lower(): (v or "").strip() for k, v in r.items()}
            if r.get("type", "component") == "component":
                out.append({"name": r["name"], "country": r["country"],
                            "part_no": r.get("part no.", ""), "qty": r.get("qty", "1") or "1"})
        return out


def fixture_parts(file):
    with open(os.path.join(FIXTURES, file)) as f:
        ch = json.load(f)["plan"]["actual_change"]
    moved = next(v for k, v in ch.items() if k.startswith("moved_to_"))
    rows = [{"name": n, "country": ch["moved_to"]} for n in moved]
    rows += [{"name": n, "country": "China"} for n in ch["still_from_china"]]
    for i, r in enumerate(rows):
        r["part_no"], r["qty"] = f"P-{100 + i}", "1"
    return rows


SOURCES = {
    "heatgun": ("backend/testdata/heatgun_bom.csv", lambda: csv_parts("backend/testdata/heatgun_bom.csv")),
    "lidar": ("backend/testdata/lidar_bom.csv", lambda: csv_parts("backend/testdata/lidar_bom.csv")),
    "template": ("frontend/public/samples/bom-template.csv", lambda: csv_parts("frontend/public/samples/bom-template.csv")),
    "supplier": ("frontend/public/samples/heatgun-bom.csv", lambda: csv_parts("frontend/public/samples/heatgun-bom.csv")),
    "grill": ("backend/internal/demos/fixtures/gas_grill_2023.json", lambda: fixture_parts("gas_grill_2023.json")),
    "heatgun2026": ("backend/internal/demos/fixtures/heatgun_2022.json", lambda: fixture_parts("heatgun_2022.json")),
    "tublight": ("backend/internal/demos/fixtures/tublight_2025.json", lambda: fixture_parts("tublight_2025.json")),
}


def paper_mono(weight, size):
    t = TTFont(os.path.join(ROOT, "frontend", "src", "app", "fonts", f"PaperMono-{weight}.woff2"))
    t.flavor = None
    buf = io.BytesIO()
    t.save(buf)
    buf.seek(0)
    return ImageFont.truetype(buf, size)


def font(kind, size):
    if kind.startswith("paper-mono-"):
        return paper_mono(kind[len("paper-mono-"):].capitalize(), size), kind
    if kind == "serif":
        for p in SERIFS:
            if os.path.exists(p):
                return ImageFont.truetype(p, size), "serif (" + os.path.basename(p) + ")"
    return ImageFont.load_default(size=size), "pillow-default-sans"


def wrap(text, f, width):
    words, lines, cur = text.split(), [], ""
    for w in words:
        t = (cur + " " + w).strip()
        if f.getlength(t) <= width or not cur:
            cur = t
        else:
            lines.append(cur)
            cur = w
    return lines + [cur] if cur else lines


def table(title, rows, cols, widths, f, fb, line, grid=False):
    """A table; each cell wraps inside its column."""
    pad = 24
    heads = [c[0] for c in cols]
    cells = [[wrap(str(r[k]), f, w - 12) for (_, k), w in zip(cols, widths)] for r in rows]
    heights = [max(len(c) for c in row) * line + 8 for row in cells]
    w = sum(widths) + 2 * pad
    h = 80 + line + 12 + sum(heights) + pad
    img = Image.new("RGB", (w, h), "white")
    d = ImageDraw.Draw(img)
    d.text((pad, 20), title, fill="black", font=fb)
    y = 70
    x = pad
    for hd, cw in zip(heads, widths):
        d.text((x + 4, y), hd, fill="black", font=fb if grid else f)
        x += cw
    y += line + 6
    d.line((pad, y, w - pad, y), fill="black", width=2 if grid else 1)
    y += 6
    for row, rh in zip(cells, heights):
        x = pad
        for lines, cw in zip(row, widths):
            for i, ln in enumerate(lines):
                d.text((x + 4, y + i * line), ln, fill="black", font=f)
            if grid:
                d.line((x, y - 6, x, y + rh - 6), fill="#888888", width=1)
            x += cw
        y += rh
        if grid:
            d.line((pad, y - 6, w - pad, y - 6), fill="#888888", width=1)
    return img


def spec_list(title, rows, f, fb, width=900):
    """Not a table: one bulleted line per part, "name: made in country"."""
    pad, line = 30, int(f.size * 1.5)
    lines = []
    for r in rows:
        ws = wrap(f"- {r['name']}: made in {r['country']}", f, width - 2 * pad - 20)
        lines += [ws[0]] + ["  " + s for s in ws[1:]]
    img = Image.new("RGB", (width, 110 + line * len(lines) + pad), "white")
    d = ImageDraw.Draw(img)
    d.text((pad, 24), title, fill="black", font=fb)
    d.text((pad, 62), "Parts and where each is made:", fill="black", font=f)
    for i, ln in enumerate(lines):
        d.text((pad, 100 + i * line), ln, fill="black", font=f)
    return img


def phone_photo(img, rng, angle):
    """Rotated, shaded from one side, slightly blurred, JPEG."""
    img = img.rotate(angle, expand=True, fillcolor=(205, 200, 190), resample=Image.BICUBIC)
    w, h = img.size
    shade = Image.linear_gradient("L").rotate(90).resize((w, h))
    dark = Image.new("RGB", (w, h), (120, 115, 105))
    img = Image.composite(dark, img, shade.point(lambda v: int(v * 0.45)))
    img = img.filter(ImageFilter.GaussianBlur(1.1))
    border = Image.new("RGB", (w + 120, h + 160), (70, 62, 55))
    border.paste(img, (60 + rng.randint(-20, 20), 80 + rng.randint(-20, 20)))
    return border


def low_res_scan(img, rng):
    """Grey, downscaled to about 45%, speckled, slightly skewed."""
    img = ImageOps.grayscale(img).rotate(0.7, expand=True, fillcolor=255, resample=Image.BICUBIC)
    img = img.resize((int(img.width * 0.45), int(img.height * 0.45)), Image.BILINEAR)
    px = img.load()
    for _ in range(img.width * img.height // 120):
        x, y = rng.randrange(img.width), rng.randrange(img.height)
        px[x, y] = rng.choice((0, 90, 255))
    return img


def save(img, name, fmt):
    path = os.path.join(OUT, name)
    if fmt == "jpg":
        img.convert("RGB").save(path, quality=60 if "scan" in name else 75, optimize=True)
    else:
        img.save(path, optimize=True)
    return name


def main():
    rng = random.Random(SEED)
    os.makedirs(OUT, exist_ok=True)
    manifest = []

    def add(name, src, rows, style, fontname, img, fmt="png"):
        image = save(img, f"{name}.{fmt}", fmt)
        manifest.append({"image": image, "source": SOURCES[src][0], "style": style, "font": fontname,
                         "components": [{"name": r["name"], "country": r["country"]} for r in rows]})

    std = [("Part", "name"), ("Country of origin", "country")]

    # heatgun: bulleted list in Paper Mono; low-resolution scan of a table.
    rows = SOURCES["heatgun"][1]()
    f, fn = font("paper-mono-regular", 17)
    add("heatgun-list", "heatgun", rows, "bulleted spec list", fn,
        spec_list("Heat gun parts list (illustrative)", rows, f, font("paper-mono-semibold", 22)[0]))
    f, fn = font("pillow-default", 18)
    add("heatgun-scan", "heatgun", rows, "low-resolution grey scan of a table", fn,
        low_res_scan(table("Heat gun BOM (illustrative)", rows, std, [620, 240], f, font("pillow-default", 22)[0], 26), rng), "jpg")

    # lidar: wrapped long names in a serif table; phone photo.
    rows = SOURCES["lidar"][1]()
    f, fn = font("serif", 19)
    img = table("LiDAR module bill of materials (illustrative)", rows, std, [700, 220], f, font("serif", 24)[0], 26)
    add("lidar-serif", "lidar", rows, "serif table, long names wrapped", fn, img)
    add("lidar-photo", "lidar", rows, "phone-photo style: rotated 6 degrees, shaded, blurred, JPEG", fn,
        phone_photo(img, rng, 6), "jpg")

    # template: page turned sideways.
    rows = SOURCES["template"][1]()
    f, fn = font("paper-mono-light", 20)
    add("template-sideways", "template", rows, "table on a page turned 90 degrees", fn,
        table("BOM template (illustrative)", rows, std, [420, 240], f, font("paper-mono-medium", 22)[0], 28).rotate(90, expand=True))

    # supplier BOM: columns reordered with part numbers and quantities; phone photo of it.
    rows = SOURCES["supplier"][1]()
    cols = [("Origin", "country"), ("Part no.", "part_no"), ("Qty", "qty"), ("Description", "name")]
    f, fn = font("paper-mono-regular", 16)
    img = table("Supplier BOM HG-1 (illustrative)", rows, cols, [150, 140, 60, 600], f, font("paper-mono-semibold", 20)[0], 24, grid=True)
    add("supplier-reordered", "supplier", rows, "grid table, country first, part numbers and quantities", fn, img)
    add("supplier-photo", "supplier", rows, "phone-photo style: rotated -4 degrees, shaded, blurred, JPEG", fn,
        phone_photo(img, rng, -4), "jpg")

    # grill: table, list, scan.
    rows = SOURCES["grill"][1]()
    f, fn = font("paper-mono-light", 18)
    add("grill-table", "grill", rows, "plain table", fn,
        table("Gas grill parts (illustrative)", rows, std, [520, 240], f, font("paper-mono-medium", 22)[0], 26))
    f, fn = font("serif", 20)
    add("grill-list", "grill", rows, "bulleted spec list", fn,
        spec_list("Gas grill: where the parts are made (illustrative)", rows, f, font("serif", 24)[0], 820))
    f, fn = font("pillow-default", 18)
    add("grill-scan", "grill", rows, "low-resolution grey scan of a table", fn,
        low_res_scan(table("Gas grill parts (illustrative)", rows, std, [520, 240], f, font("pillow-default", 22)[0], 26), rng), "jpg")

    # heat gun 2026: shuffled grid, dense small print, phone photo.
    rows = SOURCES["heatgun2026"][1]()
    shuffled = rows[:]
    rng.shuffle(shuffled)
    cols = [("Part no.", "part_no"), ("Component", "name"), ("Made in", "country")]
    f, fn = font("pillow-default", 18)
    img = table("Heat gun, 2026 setup (illustrative)", shuffled, cols, [140, 520, 200], f, font("pillow-default", 22)[0], 26, grid=True)
    add("heatgun2026-grid", "heatgun2026", shuffled, "grid table, rows in shuffled order", fn, img)
    f, fn = font("paper-mono-regular", 11)
    add("heatgun2026-small", "heatgun2026", shuffled, "dense small print (11 px)", fn,
        table("Heat gun, 2026 setup (illustrative)", shuffled, cols, [90, 330, 120], f, font("paper-mono-semibold", 13)[0], 15, grid=True))
    add("heatgun2026-photo", "heatgun2026", shuffled, "phone-photo style: rotated 9 degrees, shaded, blurred, JPEG", "pillow-default-sans",
        phone_photo(img, rng, 9), "jpg")

    # tub light: list and scan.
    rows = SOURCES["tublight"][1]()
    f, fn = font("paper-mono-regular", 18)
    add("tublight-list", "tublight", rows, "bulleted spec list", fn,
        spec_list("LED tub light components (illustrative)", rows, f, font("paper-mono-semibold", 22)[0], 960))
    f, fn = font("serif", 19)
    add("tublight-scan", "tublight", rows, "low-resolution grey scan of a table", fn,
        low_res_scan(table("LED tub light BOM (illustrative)", rows, std, [700, 220], f, font("serif", 24)[0], 26), rng), "jpg")

    with open(os.path.join(OUT, "manifest.json"), "w") as fh:
        json.dump(manifest, fh, indent=1)
        fh.write("\n")
    print(len(manifest), "images,", sum(len(m["components"]) for m in manifest), "expected parts")


if __name__ == "__main__":
    main()
