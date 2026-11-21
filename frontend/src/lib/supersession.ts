// Revoked and modified rulings (D320), mirrored from
// backend/internal/domain/supersession.json: for each ruling, its earliest
// revocation or modification in the corpus. scripts/supersession.test.mjs
// fails when the two drift. The backend never cites such a ruling as support
// for a case dated on or after the change; the UI says so beside every link.
export type Supersession = { by: string; kind: "revoked" | "modified"; date: string };

export const superseded: Record<string, Supersession> = {
  "558009": { by: "H314649", kind: "modified", date: "2022-03-03" },
  "963233": { by: "H293881", kind: "modified", date: "2018-06-18" },
  "E87234": { by: "H243329", kind: "modified", date: "2015-12-28" },
  "F88926": { by: "H256782", kind: "modified", date: "2015-12-28" },
  "H265611": { by: "H293880", kind: "modified", date: "2018-06-19" },
  "H300226": { by: "H301619", kind: "modified", date: "2018-11-06" },
  "H303733": { by: "H304419", kind: "revoked", date: "2024-12-11" },
  "H303773": { by: "H304419", kind: "revoked", date: "2024-12-11" },
  "H303816": { by: "H304419", kind: "revoked", date: "2024-12-11" },
  "H82352": { by: "H256785", kind: "modified", date: "2015-12-28" },
  "H84143": { by: "H256783", kind: "modified", date: "2015-12-28" },
  "N047417": { by: "H266527", kind: "revoked", date: "2016-04-11" },
  "N057699": { by: "H072375", kind: "modified", date: "2015-11-10" },
  "N228118": { by: "H256781", kind: "modified", date: "2015-12-28" },
  "N260916": { by: "H270451", kind: "modified", date: "2017-01-04" },
  "N261615": { by: "H263571", kind: "modified", date: "2015-11-04" },
  "N263735": { by: "H263735", kind: "modified", date: "2015-05-06" },
  "N271090": { by: "H281296", kind: "modified", date: "2017-05-23" },
  "N284181": { by: "H288252", kind: "modified", date: "2017-09-11" },
  "N287930": { by: "H290258", kind: "modified", date: "2018-06-19" },
  "N299096": { by: "H300226", kind: "modified", date: "2018-09-13" },
  "N299944": { by: "H304419", kind: "revoked", date: "2024-12-11" },
  "N302378": { by: "H325607", kind: "revoked", date: "2025-08-29" },
  "N303338": { by: "H305371", kind: "modified", date: "2024-07-12" },
  "N306886": { by: "H312440", kind: "modified", date: "2020-12-03" },
  "N307465": { by: "N308442", kind: "modified", date: "2019-12-19" },
  "N322364": { by: "H323218", kind: "modified", date: "2025-12-16" },
  "N322681": { by: "H323218", kind: "revoked", date: "2025-12-16" },
  "N326445": { by: "H335139", kind: "modified", date: "2024-05-01" },
  "N331974": { by: "H325607", kind: "revoked", date: "2025-08-29" },
  "N344694": { by: "H344999", kind: "modified", date: "2026-05-06" },
  "N345891": { by: "H346161", kind: "revoked", date: "2025-05-05" },
  "R00721": { by: "H266527", kind: "revoked", date: "2016-04-11" },
  "R02589": { by: "H256784", kind: "modified", date: "2015-12-28" },
};

// The supersession of a ruling, if any. With asOf (the case date,
// YYYY-MM-DD or ISO), "inForce" says whether CBP had already changed it by
// then: only then is it barred as precedent for that case.
export function supersededAsOf(id: string | undefined, asOf?: string | null): (Supersession & { inForce: boolean }) | undefined {
  const s = id ? superseded[id] : undefined;
  if (!s) return undefined;
  const at = asOf ? asOf.slice(0, 10) : "";
  return { ...s, inForce: !at || s.date <= at };
}
