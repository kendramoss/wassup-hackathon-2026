import assert from "node:assert/strict";
import { test } from "node:test";
import { MEDIA_MAX_BYTES, mediaBanner, routeFor, scaledSize, tooLarge } from "../src/lib/media-upload.ts";

const f = (name, type = "", size = 1000) => ({ name, type, size });

test("one CSV or Excel file keeps the BOM import", () => {
  assert.deepEqual(routeFor([f("bom.csv", "text/csv")]), { route: "bom" });
  assert.deepEqual(routeFor([f("bom.xlsx")]), { route: "bom" });
});

test("one PDF tries the table first; images go to the media endpoint", () => {
  assert.deepEqual(routeFor([f("scan.pdf", "application/pdf")]), { route: "pdf" });
  assert.deepEqual(routeFor([f("IMG_0001.JPG", "image/jpeg")]), { route: "media" });
  assert.deepEqual(routeFor([f("shot.webp")]), { route: "media" });
  assert.deepEqual(routeFor([f("photo.png", "image/png"), f("spec.pdf", "application/pdf")]), { route: "media" });
});

test("mixes and too many files are refused before upload", () => {
  assert.ok("error" in routeFor([]));
  assert.match(routeFor([f("bom.csv"), f("photo.png", "image/png")]).error, /on its own/);
  assert.match(routeFor([1, 2, 3, 4, 5].map((i) => f(`p${i}.png`, "image/png"))).error, /at most 4/);
  assert.match(routeFor([f("x.gif", "image/gif"), f("y.png", "image/png")]).error, /PNG, JPEG or WebP/);
  assert.match(routeFor([f("IMG_1.HEIC", "image/heic")]).error, /PNG, JPEG or WebP/);
});

test("the size check matches the API's 8 MB", () => {
  assert.equal(tooLarge([f("a.png", "", MEDIA_MAX_BYTES)]), null);
  assert.match(tooLarge([f("a.png", "", MEDIA_MAX_BYTES), f("b.png", "", 1)]), /limit is 8 MB/);
});

test("large photos are scaled to 3072 px on the longest side, keeping the aspect", () => {
  assert.deepEqual(scaledSize(4032, 3024, 5_000_000), { width: 3072, height: 2304 });
  assert.deepEqual(scaledSize(3000, 8000, 1_000_000), { width: 1152, height: 3072 });
  assert.equal(scaledSize(1280, 900, 400_000), null);
  assert.equal(scaledSize(3000, 2000, 6_000_000), null, "already small enough on the longest side");
  assert.equal(scaledSize(0, 0, 10), null);
});

test("the banner says a simulated draft is not the user's parts", () => {
  assert.match(mediaBanner(true), /did not read your file/);
  assert.match(mediaBanner(false), /Check every part/);
});
