"""Recolour the P key frames into each palette option, side by side.

    python docs/design/palettes/build.py            # frames only
    python docs/design/palettes/build.py --png      # also screenshot (headless Chrome)

Reads docs/design/keyframes/<frame>.html (never modified) and writes
docs/design/palettes/<option>/<frame>.html and <option>/png/<frame>.png.
Each frame's colours are mapped from the current P palette to the option's,
and the graph grid's linear-gradient is replaced by a flat SVG line pattern,
so no option uses a gradient anywhere.
"""
import os
import re
import subprocess
import sys
import urllib.parse

HERE = os.path.dirname(os.path.abspath(__file__))
KF = os.path.join(HERE, "..", "keyframes")
FRAMES = ["p-cases", "p-k2", "p-plan-q1", "p-compare", "p-draft"]

# Current P colours (DESIGN.md step 3) → role. Letter-only colours in p-draft
# and the guide/dimension blues are included so nothing keeps the old tint.
CURRENT = {
    "#f3f3ee": "paper", "#f9f9f5": "sheet", "#ebebe4": "wash", "#dde4f3": "grid",
    "#cfd3dc": "line", "#c9cfdc": "line", "#b9c6e6": "edge", "#c7d2ec": "edge",
    "#9fb2e3": "guide", "#b3c3ea": "guide", "#a8bcef": "shadow", "#7f97d6": "dimension",
    "#232320": "ink", "#121211": "ink", "#1b1a17": "ink", "#141413": "ink",
    "#5c5b55": "dim", "#3d3c37": "body", "#7f7d76": "faint",
    "#3a5fc4": "ok", "#e5ebfa": "okwash",
    "#965606": "ask", "#5a3a06": "ask", "#6b4a00": "ask",
    "#fbefdb": "askwash", "#fbf0dc": "askwash", "#fdf0c2": "askwash",
    "#e3be86": "askline", "#c98a28": "askline", "#d4902a": "askbar", "#efcd94": "askshadow",
    "#ffffff": "white", "#fff": "white", "#fffffb": "white", "#faf8f3": "white",
}

OPTIONS = {
    "a-drafting-white": {
        "title": "A · Drafting white",
        "paper": "#f6f7f9", "sheet": "#ffffff", "wash": "#eceef2", "grid": "#e3e8f2", "line": "#d5dae3",
        "edge": "#b9c6e6", "guide": "#a9bae2", "shadow": "#a8bcef", "dimension": "#4864b2",
        "ink": "#1c2028", "dim": "#555c69", "body": "#343944", "faint": "#7a808c",
        "ok": "#2f5bd3", "okwash": "#e8eefc",
        "ask": "#a8401a", "askwash": "#fcebe3", "askline": "#e8b29b", "askbar": "#c8603a", "askshadow": "#f0c6b4",
        "white": "#ffffff",
    },
    "b-blueprint": {
        "title": "B · Blueprint",
        "paper": "#edf2f9", "sheet": "#f9fbfe", "wash": "#e2e9f3", "grid": "#d3dff0", "line": "#c4d0e2",
        "edge": "#9fb4d8", "guide": "#8fa7d6", "shadow": "#86a3d8", "dimension": "#3c5aa8",
        "ink": "#13213a", "dim": "#47566f", "body": "#2b3a55", "faint": "#6e7c94",
        "ok": "#1d4fc4", "okwash": "#e2eafa",
        "ask": "#6a3fb0", "askwash": "#f0e9fb", "askline": "#c6b2ea", "askbar": "#8a63c9", "askshadow": "#d5c5f2",
        "white": "#ffffff",
    },
    "c-concrete-teal": {
        "title": "C · Concrete and teal",
        "paper": "#eeefef", "sheet": "#fafafa", "wash": "#e4e6e6", "grid": "#e0e2e2", "line": "#cfd3d3",
        "edge": "#aec4c0", "guide": "#9dbdb7", "shadow": "#97c0b9", "dimension": "#2f7268",
        "ink": "#1e2121", "dim": "#555b5b", "body": "#363b3b", "faint": "#7b8181",
        "ok": "#0d6b5f", "okwash": "#e2f1ee",
        "ask": "#a8401a", "askwash": "#fbe9e1", "askline": "#e6b19b", "askbar": "#c8603a", "askshadow": "#efc6b4",
        "white": "#ffffff",
    },
    "d-ink-electric": {
        "title": "D · Ink and electric blue",
        "paper": "#ffffff", "sheet": "#ffffff", "wash": "#f1f2f5", "grid": "#eceef3", "line": "#d9dce3",
        "edge": "#0c0c0e", "guide": "#9aa6c4", "shadow": "#0c0c0e", "dimension": "#0040e0",
        "ink": "#0c0c0e", "dim": "#4f525a", "body": "#26282d", "faint": "#7a7d85",
        "ok": "#0040e0", "okwash": "#e6edff",
        "ask": "#b83a00", "askwash": "#fff0e8", "askline": "#eba98a", "askbar": "#d65a1c", "askshadow": "#b83a00",
        "white": "#ffffff",
    },
    "e-graphite": {
        "title": "E · Graphite (dark)",
        "paper": "#14161a", "sheet": "#1b1e24", "wash": "#23272e", "grid": "#1f232a", "line": "#2e333c",
        "edge": "#3b4659", "guide": "#3d4a63", "shadow": "#33476f", "dimension": "#8fa6e0",
        "ink": "#e9ebef", "dim": "#a1a8b4", "body": "#cdd2da", "faint": "#7d8492",
        "ok": "#86a6ff", "okwash": "#1e2840",
        "ask": "#ff9a62", "askwash": "#33231a", "askline": "#7a4a30", "askbar": "#c8703f", "askshadow": "#5a3522",
        "white": "#1b1e24",
    },
}

HEX = re.compile(r"#(?:[0-9a-fA-F]{6}|[0-9a-fA-F]{3})\b")
DATA = re.compile(r"data:[^\"')]+")
GRID = "background-image:linear-gradient(var(--grid) 1px,transparent 1px),linear-gradient(90deg,var(--grid) 1px,transparent 1px)"


def grid_pattern(colour: str) -> str:
    svg = f"<svg xmlns='http://www.w3.org/2000/svg' width='20' height='20'><path d='M0 .5H20M.5 0V20' stroke='{colour}' fill='none'/></svg>"
    return f'background-image:url("data:image/svg+xml,{urllib.parse.quote(svg)}")'


def recolour(html: str, palette: dict) -> str:
    # Leave inlined fonts and flags alone; recolour only the CSS and markup.
    parts, last, out = [], 0, []
    for m in DATA.finditer(html):
        parts.append((html[last:m.start()], m.group(0)))
        last = m.end()
    parts.append((html[last:], ""))
    for text, data in parts:
        text = HEX.sub(lambda h: palette.get(CURRENT.get(h.group(0).lower(), ""), h.group(0)), text)
        out.append(text + data)
    html = "".join(out).replace(GRID, grid_pattern(palette["grid"]))
    if palette["paper"] < "#888888":  # dark option: flags and form controls on a dark page
        html = html.replace("rgba(0,0,0,.12)", "rgba(255,255,255,.18)").replace("<head>", '<head><meta name="color-scheme" content="dark">', 1)
    return html


def chrome() -> str:
    for p in [r"C:\Program Files\Google\Chrome\Application\chrome.exe", "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "google-chrome", "chromium"]:
        if os.path.exists(p) or not os.path.isabs(p):
            return p
    raise SystemExit("Chrome not found")


def main():
    png = "--png" in sys.argv
    for key, pal in OPTIONS.items():
        os.makedirs(os.path.join(HERE, key, "png"), exist_ok=True)
        for f in FRAMES:
            src = open(os.path.join(KF, f + ".html"), encoding="utf-8").read()
            dst = os.path.join(HERE, key, f + ".html")
            open(dst, "w", encoding="utf-8", newline="\n").write(recolour(src, pal))
            if png:
                subprocess.run([chrome(), "--headless=new", "--disable-gpu", "--hide-scrollbars", "--window-size=1440,900",
                                f"--screenshot={os.path.join(HERE, key, 'png', f + '.png')}", "file:///" + dst.replace(os.sep, "/")],
                               check=True, capture_output=True)
        print(key, "done")


if __name__ == "__main__":
    main()
