#!/usr/bin/env bash
# #421: the rate-limit fix only holds while the backend is not directly
# callable. Fail loudly if the IAM policy is opened or Google Frontend stops
# refusing an unauthenticated request from the internet.
set -euo pipefail

PROJECT="${PROJECT:-${GOOGLE_CLOUD_PROJECT:-wassup-hackathon-2026}}"
REGION="${REGION:-us-central1}"
SERVICE="${SERVICE:-backend}"

iam="$(gcloud run services get-iam-policy "$SERVICE" \
  --project "$PROJECT" --region "$REGION" --format=json)"
if printf '%s' "$iam" | grep -Eq '"member"[[:space:]]*:[[:space:]]*"allUsers"'; then
  echo "FAIL: $SERVICE IAM policy grants allUsers; never add allow-unauthenticated" >&2
  exit 1
fi

url="$(gcloud run services describe "$SERVICE" \
  --project "$PROJECT" --region "$REGION" --format='value(status.url)')"
code="$(curl -s -o /dev/null -w '%{http_code}' "$url/health" || true)"
if [ "$code" != "403" ]; then
  echo "FAIL: direct $url/health returned $code, want 403 from Google Frontend" >&2
  exit 1
fi

echo "PASS: $SERVICE is IAM-private (direct /health = 403, no allUsers)"
