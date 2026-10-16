#!/usr/bin/env bash
# Launch a Vertex AI supervised tuning job for the origin engine (#38, D38).
# Costs money: training is billed per token x epochs. Not run by CI.
#
#   BASE_MODEL=<a Gemini model Vertex lists as tunable> scripts/tune-vertex.sh
#
# Before running: check the Vertex AI tuning docs for which Gemini models
# support supervised tuning and the price per training token. Do not assume
# gemini-4-argon is tunable.
# Inputs: data/tuning/{train,tune_check}.jsonl from `eval tunedata`.
set -euo pipefail
: "${BASE_MODEL:?set BASE_MODEL to a tunable Gemini model ID}"
PROJECT=${PROJECT:-wassup-hackathon-2026}
REGION=${REGION:-us-central1} # tuning jobs are regional, unlike the global serving endpoint
EPOCHS=${EPOCHS:-3}
BUCKET=${BUCKET:-gs://$PROJECT-origin-proof-corpus}
NAME=${NAME:-origin-engine-cbp-reasoning-$(date +%Y%m%d-%H%M)}

gcloud storage cp data/tuning/train.jsonl data/tuning/tune_check.jsonl "$BUCKET/tuning/$NAME/"

curl -sS -X POST \
  -H "Authorization: Bearer $(gcloud auth print-access-token)" \
  -H "Content-Type: application/json" \
  "https://$REGION-aiplatform.googleapis.com/v1/projects/$PROJECT/locations/$REGION/tuningJobs" \
  -d @- <<JSON
{
  "baseModel": "$BASE_MODEL",
  "tunedModelDisplayName": "$NAME",
  "supervisedTuningSpec": {
    "trainingDatasetUri": "$BUCKET/tuning/$NAME/train.jsonl",
    "validationDatasetUri": "$BUCKET/tuning/$NAME/tune_check.jsonl",
    "hyperParameters": {"epochCount": $EPOCHS}
  }
}
JSON
echo
echo "When the job succeeds, point ENGINE_MODEL at the tuned model's endpoint and compare on dev:"
echo "  eval run -split dev  (then)  eval compare engine-v1 vs tuned"
