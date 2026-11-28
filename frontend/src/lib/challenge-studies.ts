import bundle from "./challenge-data.json";
import { parseChallengeSummary } from "./challenge-evidence";

export const challengeStudies = bundle.sources.flatMap((source) => parseChallengeSummary(source.summary, source.sourcePath));
const ids = new Set<string>();
for (const study of challengeStudies) {
  if (ids.has(study.id)) throw new Error("A challenge capability has more than one selected result");
  ids.add(study.id);
}
