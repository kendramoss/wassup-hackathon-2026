import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

const { JapanPreference } = await import("../src/components/op/japan-preference.tsx");
const { japanAgreements, japanNoEditionAgreements, japanTariffEditions, supportedJapanAgreements, validJapanHS } = await import("../src/lib/japan-preference.ts");

const c = {
  id: "case_f987ac",
  jurisdiction: "jp",
  prediction: {
    origin_country: "Vietnam",
    reasoning: "The input classification changes.",
    japan_preference: {
      agreement: "RCEP",
      hs: "6403.99",
      outcome: "qualifies",
      criterion: "CTH",
      deciding_rule: "Change to the finished good from another heading.",
      sources: [{ citation: "Annex 3A", url: "https://rcepsec.org/rules-of-origin/" }],
      precedent_sources: [{ citation: "JP1234567", url: "https://www.customs.go.jp/ruling.htm" }],
    },
  },
};

test("Japan’s result shows preferential scope and the caveats before the closed fold", () => {
  const m = html(JapanPreference, { c });
  const [visible, details] = m.split("<details");
  assert.match(text(visible), /Trade preference in Japan.*Likely qualifies.*RCEP.*Likely origin Vietnam/s);
  assert.match(text(visible), /Not a ruling\. Confirm with a Japanese customs specialist\./);
  assert.match(text(visible), /published rulings used for testing all approved preferential treatment/);
  assert.doesNotMatch(visible, /CBP|USMCA|held-out|6403\.99|JP1234567/);
  assert.match(details, /<summary[^>]*>.*Details.*Change of tariff heading.*Annex 3A.*Earlier Japan Customs rulings.*JP1234567/s);
  assert.doesNotMatch(details, /\bopen=""/);
});

test("a negative or undetermined result shows no origin even if the case carries one", () => {
  for (const [outcome, label] of [["does_not_qualify", "Likely doesn’t qualify"], ["undetermined", "Needs more evidence"]]) {
    const prediction = { ...c.prediction, japan_preference: { ...c.prediction.japan_preference, outcome } };
    const m = text(html(JapanPreference, { c: { ...c, prediction } }));
    assert.ok(m.includes(label));
    assert.doesNotMatch(m, /Likely origin|Vietnam/);
  }
});

test("the Japan form uses the preferential agreement codes and accepts Japanese tariff code formats", async () => {
  const corpus = await readFile(new URL("../../backend/internal/jpcorpus/jpcorpus.go", import.meta.url), "utf8");
  const names = corpus.split("var AgreementNames = map[string]string{")[1].split("}")[0];
  const codes = [...names.matchAll(/^\s*"([^"]+)":/gm)].map((m) => m[1]).filter((code) => code !== "Non-preferential");
  assert.deepEqual(Object.keys(japanAgreements), codes);
  for (const value of ["6403", "6403.99", "6403.99.000", "6403.99-000", " 6403 99 "]) assert.ok(validJapanHS(value), value);
  for (const value of ["", "640", "6403.9", "6403.99.00", "6403.99.000A"]) assert.ok(!validJapanHS(value), value);
});

test("the selector’s HS editions match the embedded schedules", async () => {
  const schedules = JSON.parse(await readFile(new URL("../../backend/internal/jprules/data/schedules.json", import.meta.url), "utf8"));
  const editions = {};
  const unstated = new Set();
  for (const s of schedules) {
    const edition = s.citation.match(/HS ([0-9]{4})/)?.[1];
    if (edition) editions[s.agreement] ??= edition;
    else unstated.add(s.agreement);
  }
  // A schedule that states no HS edition must be listed explicitly, never given a guessed year.
  assert.deepEqual([...unstated].sort(), [...japanNoEditionAgreements].sort());
  assert.deepEqual(japanTariffEditions, { ...editions, RCEP: "2022" });
});

test("the selector offers Japan–U.S. without inventing an HS edition and keeps GSP out", () => {
  assert.deepEqual(japanNoEditionAgreements, ["JP-US TA"]);
  assert.ok(supportedJapanAgreements.includes("JP-US TA"));
  assert.equal(japanTariffEditions["JP-US TA"], undefined);
  assert.equal(japanAgreements["JP-US TA"], "Japan–U.S. agreement");
  assert.ok(!supportedJapanAgreements.includes("GSP"));
  assert.deepEqual(supportedJapanAgreements, Object.keys(japanAgreements).filter((code) => code !== "GSP"));
  assert.deepEqual(supportedJapanAgreements.slice(0, 5), ["JP-EU EPA", "CPTPP", "AJCEP", "RCEP", "JP-US TA"]);
});
