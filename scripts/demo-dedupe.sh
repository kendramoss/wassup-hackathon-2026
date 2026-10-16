#!/usr/bin/env bash
# Keeps one pinned case per demo on a server and removes the rest (D56).
#
#   ID_TOKEN=$(gcloud auth print-identity-token) ADMIN_TOKEN=... scripts/demo-dedupe.sh <API>           # dry run
#   ID_TOKEN=... ADMIN_TOKEN=... APPLY=1 scripts/demo-dedupe.sh <API>                                   # delete
#
# Groups pinned cases by demo (matched on the demo title as display name, the
# product name, or the full facts as first predicted) and keeps, per demo, the case that went furthest (plan > origin) and
# then the newest. Kept cases get the demo's title as display_name. Only
# pinned cases are touched; deleting them needs the admin token.
set -euo pipefail
API=${1:?usage: scripts/demo-dedupe.sh <API base URL>}
API=${API%/}
: "${ADMIN_TOKEN:?ADMIN_TOKEN is required}"
hdr=(-H "Content-Type: application/json" -H "X-Admin-Token: $ADMIN_TOKEN")
[ -n "${ID_TOKEN:-}" ] && hdr+=(-H "Authorization: Bearer $ID_TOKEN")
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
curl -sf "${hdr[@]}" "$API/v1/demos" >"$TMP/demos.json"
for id in $(python3 -c "import json;print(' '.join(d['id'] for d in json.load(open('$TMP/demos.json'))['demos']))"); do
	curl -sf "${hdr[@]}" "$API/v1/demos/$id" >"$TMP/demo-$id.json"
done
curl -sf "${hdr[@]}" "$API/v1/cases" >"$TMP/cases.json"
for id in $(python3 -c "import json;print(' '.join(c['id'] for c in json.load(open('$TMP/cases.json'))['cases'] if c['pinned']))"); do
	curl -sf "${hdr[@]}" "$API/v1/cases/$id" >"$TMP/case-$id.json"
done
python3 - "$TMP" >"$TMP/plan.txt" <<'PY'
import json, glob, os, sys
tmp = sys.argv[1]
demos = []
for p in glob.glob(f"{tmp}/demo-*.json"):
    d = json.load(open(p))
    pr = d["predict"]
    demos.append((d["title"], (pr.get("product") or {}).get("name", ""), (pr.get("facts") or "").strip()))
multi = ("Heat gun: compare Thailand, Vietnam and Malaysia", None, None)
rank = {"export": 3, "plan": 2, "origin": 1, "product": 0}
groups = {}
for p in glob.glob(f"{tmp}/case-*.json"):
    c = json.load(open(p))
    name = (c.get("product") or {}).get("name", "")
    facts = (c.get("original_facts") or c.get("facts") or "").strip()
    title = None
    if c.get("targets") or c.get("display_name") == multi[0]:
        title = multi[0]
    else:
        for t, pname, pfacts in demos:
            if c.get("display_name") == t or (pname and name == pname) or (pfacts and facts == pfacts):
                title = t
                break
    if title is None:
        print(f"UNMATCHED {c['id']} {c.get('display_name','')[:50]!r}", file=sys.stderr)
        continue
    groups.setdefault(title, []).append(c)
for title, cs in sorted(groups.items()):
    cs.sort(key=lambda c: (rank.get(c.get("step"), 0), c.get("updated_at", "")), reverse=True)
    keep, drop = cs[0], cs[1:]
    print(f"KEEP {keep['id']} {title}")
    for c in drop:
        print(f"DROP {c['id']} {title}")
PY
cat "$TMP/plan.txt"
if [ "${APPLY:-}" != 1 ]; then
	echo "dry run: set APPLY=1 to rename the kept cases and delete the others"
	exit 0
fi
while read -r action id title; do
	case $action in
	KEEP)
		body=$(python3 -c 'import json,sys;print(json.dumps({"display_name":sys.argv[1]}))' "$title")
		curl -sf -o /dev/null -X PATCH "${hdr[@]}" -d "$body" "$API/v1/cases/$id" && echo "renamed $id: $title" ;;
	DROP)
		code=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "${hdr[@]}" "$API/v1/cases/$id")
		echo "deleted $id ($code)" ;;
	esac
done <"$TMP/plan.txt"
