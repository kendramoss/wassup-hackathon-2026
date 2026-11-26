#!/usr/bin/env bash
# Real-Vertex smoke against a backend revision that serves no traffic yet (#276, #845).
# deploy.yml runs it between `gcloud run deploy --no-traffic --tag candidate` and
# the traffic shift: a non-zero exit leaves production on the previous revision.
#
# Usage: backend-candidate-smoke.sh CANDIDATE_URL ID_TOKEN [EXPECTED_FAMILY]
# Inputs are synthetic. Bad requests are rejected before any model call.
# The token is never printed. Results go to stdout and, in Actions, the job summary.
set -euo pipefail

url=${1:?candidate URL}
token=${2:?identity token}
expected=${3:-motorised_tools_small_appliances}
summary=${GITHUB_STEP_SUMMARY:-/dev/null}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fail=0

post() { # name body -> sets code, out
  local n=$1
  code=$(curl -sS -o "$tmp/$n" -w '%{http_code}' --max-time 120 \
    -H "Authorization: Bearer $token" -H 'Content-Type: application/json' \
    --data-binary "$2" "$url/v1/products/family-check") || code=000
  out=$(cat "$tmp/$n" 2>/dev/null || true)
}
field() { jq -r "$1 // empty" <<<"$out" 2>/dev/null || true; }
row() { # case result http detail
  printf '%s: %s (HTTP %s) %s\n' "$1" "$2" "$3" "$4"
  printf '| %s | %s | %s | %s |\n' "$1" "$2" "$3" "$4" >>"$summary"
}

{
  echo "### Backend candidate smoke (real Vertex)"
  echo
  echo "Candidate: \`$url\`"
  echo
  echo '| Case | Result | HTTP | Detail |'
  echo '| --- | --- | ---: | --- |'
} >>"$summary"

post supported '{"text":"hair dryer with a motor, heating coil and moulded plastic housing"}'
fam=$(field .result.family); prov=$(field .llm_provider)
if [ "$code" = 200 ] && [ "$prov" = vertex ] && [ "$fam" = "$expected" ]; then
  row "supported (hair dryer)" PASS "$code" "family=$fam provider=$prov"
else
  row "supported (hair dryer)" FAIL "$code" "family=${fam:-?} provider=${prov:-?}, want $expected/vertex"; fail=1
fi

post unsupported '{"text":"synthetic sailing yacht hull made of fibreglass"}'
fam=$(field .result.family); prov=$(field .llm_provider)
if [ "$code" = 200 ] && [ "$prov" = vertex ] && [ "$fam" = none ]; then
  row "unsupported (yacht hull)" PASS "$code" "family=none provider=$prov"
else
  row "unsupported (yacht hull)" FAIL "$code" "family=${fam:-?} provider=${prov:-?}, want none/vertex"; fail=1
fi

long=$(printf 'a%.0s' $(seq 1 2001))
i=0
for body in '{"text":"   "}' 'text=hair dryer' '{"text":"hair dryer","family":"led_lights"}' "{\"text\":\"$long\"}"; do
  i=$((i + 1))
  case $i in 1) name="blank text" ;; 2) name="not JSON" ;; 3) name="unknown field" ;; 4) name="2001 characters" ;; esac
  post "bad$i" "$body"
  c=$(field .code)
  if [ "$code" = 400 ] && [ "$c" = invalid_input ]; then
    row "malformed ($name)" PASS "$code" "code=$c"
  else
    row "malformed ($name)" FAIL "$code" "code=${c:-?}, want invalid_input"; fail=1
  fi
done

echo >>"$summary"
if [ "$fail" -ne 0 ]; then
  echo "**FAIL**: traffic was not shifted; production stays on the previous revision." >>"$summary"
  echo "::error::candidate smoke failed; traffic not shifted"
  exit 1
fi
echo "**PASS**: candidate may take traffic." >>"$summary"
