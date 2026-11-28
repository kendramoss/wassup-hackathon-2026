export type ChallengeCounts = { cases: number; matched: number; errors: number; mismatched: number };
export type ChallengeFeature = {
  id: string;
  label: string;
  candidate_revision: string;
  dataset_sha256: string;
  source_families: number;
  development: Omit<ChallengeCounts, "mismatched">;
  test: ChallengeCounts;
  cost_usd: number;
  latency_p50_ms: number;
  latency_p95_ms: number;
  limitations: string[];
  sourcePath: string;
  public_note?: string;
};

function object(value: unknown): Record<string, unknown> {
  if (value === null || typeof value !== "object" || Array.isArray(value)) throw new Error("Invalid challenge evidence object");
  return value as Record<string, unknown>;
}
function integer(value: unknown, name: string): number {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0) throw new Error(`Invalid challenge ${name}`);
  return value;
}
function finite(value: unknown, name: string): number {
  if (typeof value !== "number" || !Number.isFinite(value) || value < 0) throw new Error(`Invalid challenge ${name}`);
  return value;
}
function nonempty(value: unknown, name: string): string {
  if (typeof value !== "string" || !value.trim()) throw new Error(`Missing challenge ${name}`);
  return value;
}

export function parseChallengeSummary(input: unknown, sourcePath: string): ChallengeFeature[] {
  if (!/^eval\/challenges\/[a-zA-Z0-9_/-]+\.json$/.test(sourcePath) || sourcePath.includes("..")) throw new Error("Invalid challenge source path");
  const doc = object(input);
  if (doc.schema_version !== 1 || doc.evidence_kind !== "source_grounded_synthetic" || !Array.isArray(doc.features)) {
    throw new Error("Challenge evidence must keep its synthetic label and schema");
  }
  const seen = new Set<string>();
  return doc.features.map((raw): ChallengeFeature => {
    const f = object(raw);
    if (f.simulated === true || f.llm_provider === "fake" || f.llm_provider === "vertex") {
      throw new Error("Challenge results must be measured with the approved execution provider");
    }
    const id = nonempty(f.id, "id");
    if (!/^[a-z][a-z0-9_-]*$/.test(id) || seen.has(id)) throw new Error("Invalid or duplicate challenge feature");
    seen.add(id);
    const candidate = nonempty(f.candidate_revision, "candidate revision");
    const dataset = nonempty(f.dataset_sha256, "dataset hash");
    if (!/^[0-9a-f]{40}$/.test(candidate) || !/^[0-9a-f]{64}$/.test(dataset)) throw new Error("Challenge candidate and labels must be frozen");
    const d = object(f.development), t = object(f.test);
    const development = {
      cases: integer(d.cases, "development cases"), matched: integer(d.matched, "development matches"), errors: integer(d.errors, "development errors"),
    };
    if (development.matched + development.errors > development.cases) throw new Error("Inflated development numerator");
    const test = {
      cases: integer(t.cases, "test cases"), matched: integer(t.matched, "test matches"),
      errors: integer(t.errors, "test errors"), mismatched: integer(t.mismatched, "test mismatches"),
    };
    if (test.cases < 100 || test.matched + test.mismatched + test.errors !== test.cases) {
      throw new Error("Challenge headline needs 100 cases and a complete denominator");
    }
    if (!Array.isArray(f.limitations) || f.limitations.length === 0 || !f.limitations.every((s) => typeof s === "string" && s.trim())) {
      throw new Error("Challenge limitations must stay with its results");
    }
    const errorNote = f.error_note === undefined ? [] : [nonempty(f.error_note, "error qualification")];
    const publicNote = f.public_note === undefined ? undefined : nonempty(f.public_note, "public qualification");
    if (f.source_isolation_issue === true && !publicNote) throw new Error("Source isolation qualification cannot be hidden");
    const sourceFamilies = integer(f.source_families, "source families");
    if (sourceFamilies < 1) throw new Error("Challenge source groups are missing");
    return {
      id, label: nonempty(f.label, "label"), candidate_revision: candidate, dataset_sha256: dataset,
      source_families: sourceFamilies, development, test,
      cost_usd: finite(f.cost_usd, "cost"), latency_p50_ms: finite(f.latency_p50_ms, "median latency"),
      latency_p95_ms: finite(f.latency_p95_ms, "tail latency"), limitations: [...f.limitations as string[], ...errorNote], sourcePath, public_note: publicNote,
    };
  });
}

export function challengePercent(feature: ChallengeFeature): string {
  return `${(100 * feature.test.matched / feature.test.cases).toFixed(1).replace(/\.0$/, "")}%`;
}
