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

// The declared edition of an Annex is the typed `stated_edition`; citation prose is only cross-checked.
function derive(schedules) {
  const editions = {};
  const unstated = new Set();
  for (const s of schedules) {
    assert.ok("stated_edition" in s, `${s.agreement}: stated_edition missing`);
    if (s.stated_edition === null) unstated.add(s.agreement);
    else {
      assert.ok([2002, 2007, 2012, 2017, 2022].includes(s.stated_edition), s.agreement);
      editions[s.agreement] ??= String(s.stated_edition);
    }
  }
  return { editions, unstated };
}

function crossCheck(s) {
  if (s.stated_edition === null) return /not stated/.test(s.citation) && !/\(HS [0-9]{4}\)/.test(s.citation);
  return s.citation.includes(`(HS ${s.stated_edition})`);
}

test("the selector’s HS editions match the embedded schedules", async () => {
  const schedules = JSON.parse(await readFile(new URL("../../backend/internal/jprules/data/schedules.json", import.meta.url), "utf8"));
  const { editions, unstated } = derive(schedules);
  // A schedule that states no HS edition must be listed explicitly, never given a guessed year.
  assert.deepEqual([...unstated].sort(), [...japanNoEditionAgreements].sort());
  // The citation must agree with the typed field, so a source change forces a conscious UI decision.
  for (const s of schedules) assert.ok(crossCheck(s), `${s.agreement}: ${s.citation}`);
  for (const s of schedules.filter((x) => x.stated_edition === null)) assert.ok(japanNoEditionAgreements.includes(s.agreement), s.agreement);
  const gsp = schedules.find((x) => x.agreement === "GSP");
  assert.equal(gsp.from, "2023-04-01");
  assert.equal(gsp.hs_edition, 2022);
  // The validation list (2022) and the declared edition (none) are different things.
  assert.equal(gsp.stated_edition, null);
  assert.deepEqual(japanTariffEditions, { ...editions, RCEP: "2022" });
});

test("the derived edition does not change with citation wording", async () => {
  const schedules = JSON.parse(await readFile(new URL("../../backend/internal/jprules/data/schedules.json", import.meta.url), "utf8"));
  const base = derive(schedules);
  const reworded = schedules.map((s, i) => ({
    ...s,
    citation: [`validated against HS 2022; ${s.citation}`, `${s.citation}, HS 2017 in prose`, `HS 2012 ${s.citation}`][i % 3],
  }));
  assert.deepEqual(derive(reworded), base);
  // A schedule whose citation disagrees with its typed edition is caught.
  const mismatch = { ...schedules[0], stated_edition: 2012 };
  assert.ok(!crossCheck(mismatch));
  assert.ok(!crossCheck({ ...schedules.find((x) => x.agreement === "GSP"), citation: "GSP (HS 2022)" }));
  assert.ok(!crossCheck({ ...schedules.find((x) => x.agreement === "GSP"), citation: "GSP" }));
});

test("the selector offers Japan–U.S. and GSP without inventing an HS edition", () => {
  assert.deepEqual(japanNoEditionAgreements, ["JP-US TA", "GSP"]);
  assert.ok(supportedJapanAgreements.includes("JP-US TA"));
  assert.equal(japanTariffEditions["JP-US TA"], undefined);
  assert.equal(japanAgreements["JP-US TA"], "Japan–U.S. agreement");
  assert.ok(supportedJapanAgreements.includes("GSP"));
  assert.equal(japanTariffEditions.GSP, undefined);
  assert.equal(japanAgreements.GSP, "Generalized System of Preferences (GSP)");
  assert.equal(supportedJapanAgreements.indexOf("GSP"), supportedJapanAgreements.indexOf("JP-US TA") + 1);
  assert.deepEqual(supportedJapanAgreements.slice(0, 6), ["JP-EU EPA", "CPTPP", "AJCEP", "RCEP", "JP-US TA", "GSP"]);
});
