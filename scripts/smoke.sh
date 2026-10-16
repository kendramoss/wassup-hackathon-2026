#!/usr/bin/env bash
# Post-deploy smoke test of the backend API, through the real endpoints.
#
#   scripts/smoke.sh http://localhost:8080
#   ID_TOKEN=$(gcloud auth print-identity-token) ADMIN_TOKEN=... scripts/smoke.sh https://<backend>.run.app
#
# Checks /health, /health?deep=1 (case store write + model call), the demo
# catalog, a heat-gun predict, reading and listing the case, and one plan
# search (~30 s, makes model calls). The case it creates is deleted at the
# end unless KEEP=1. Exits non-zero at the first failure.
#
# ID_TOKEN:    Google identity token, for a private Cloud Run service.
# ADMIN_TOKEN: bypasses the rate limits (the script makes 3 model-calling requests).
# Needs curl and python3.
set -euo pipefail
API=${1:?usage: scripts/smoke.sh <API base URL>}
API=${API%/}
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

hdr=(-H "Content-Type: application/json")
[ -n "${ID_TOKEN:-}" ] && hdr+=(-H "Authorization: Bearer $ID_TOKEN")
[ -n "${ADMIN_TOKEN:-}" ] && hdr+=(-H "X-Admin-Token: $ADMIN_TOKEN")

# step NAME EXPECTED_STATUS METHOD PATH [BODY_FILE] -> response in $TMP/out
step() {
	local name=$1 want=$2 method=$3 path=$4 body=${5:-}
	local args=(-s -o "$TMP/out" -w "%{http_code} %{time_total}" -X "$method" "${hdr[@]}")
	[ -n "$body" ] && args+=(--data-binary "@$body")
	local res code secs
	res=$(curl "${args[@]}" "$API$path" || echo "000 0")
	code=${res%% *} secs=${res##* }
	if [ "$code" = "$want" ]; then
		printf 'ok    %-34s %s  %5.1f s\n' "$name" "$code" "$secs"
	else
		printf 'FAIL  %-34s %s (want %s)  %5.1f s\n' "$name" "$code" "$want" "$secs"
		head -c 600 "$TMP/out" 2>/dev/null; echo
		return 1
	fi
}
json() { python3 -c "import json,sys; d=json.load(open('$TMP/out')); print($1)"; }

step "GET /health" 200 GET /health
step "GET /health?deep=1" 200 GET "/health?deep=1" ||
	{ echo "deep health failed: fix the case store or model access before anything else"; exit 1; }
json "'      checks: ' + ', '.join(f\"{k} {v['latency_ms']} ms\" for k, v in d['checks'].items())"
step "GET /v1/demos/heatgun-2022" 200 GET /v1/demos/heatgun-2022
python3 -c "import json; d=json.load(open('$TMP/out')); json.dump(d['predict'], open('$TMP/predict.json','w')); json.dump(d['plan'], open('$TMP/plan.json','w'))"

step "POST /v1/cases/predict (heat gun)" 201 POST /v1/cases/predict "$TMP/predict.json"
ID=$(json "d['id']")
json "f\"      {d['id']}: {d['prediction']['origin_country']} {d['prediction']['confidence']:.2f} ({d['triage']['route']}), {d['llm_provider']} {d['model_ids']['engine']}\""
cleanup() {
	if [ "${KEEP:-}" != 1 ] && [ -n "${ID:-}" ]; then
		curl -s -o /dev/null -X DELETE "${hdr[@]}" "$API/v1/cases/$ID" || true
		echo "deleted $ID (KEEP=1 keeps it)"
	fi
	rm -rf "$TMP"
}
trap cleanup EXIT

step "GET /v1/cases/{id}" 200 GET "/v1/cases/$ID"
step "GET /v1/cases" 200 GET /v1/cases
json "'      listed: ' + str(any(c['id'] == '$ID' for c in d['cases']))"
step "POST /v1/cases/{id}/plan (~30 s)" 200 POST "/v1/cases/$ID/plan" "$TMP/plan.json"
json "(lambda p: f\"      {p['stats']['moves_searched']} moves, {len(p['plans'])} shown, {p['stats'].get('verifier_cached', 0)}/{p['stats']['verifier_runs']} checks cached\")(d['plan'])"

echo "all checks passed against $API"
