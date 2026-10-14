#!/usr/bin/env bash
# One-time setup so GitHub Actions (.github/workflows/deploy.yml) can deploy to
# Cloud Run without JSON keys, via Workload Identity Federation.
# Safe to re-run: existing resources are skipped, bindings are idempotent.
#
# Only pushes to main of kendramoss/wassup-hackathon-2026 can get a token, and
# the deployer can build and deploy but not change who may call a service.
set -euo pipefail

PROJECT=wassup-hackathon-2026
PROJECT_NUMBER=88903435351
REPO_ID=1417155350 # GitHub repo ID: stays the same if the repo is renamed
DEPLOYER=github-deployer@$PROJECT.iam.gserviceaccount.com
POOL=projects/$PROJECT_NUMBER/locations/global/workloadIdentityPools/github

gcloud services enable sts.googleapis.com iamcredentials.googleapis.com --project "$PROJECT"

gcloud iam service-accounts describe "$DEPLOYER" --project "$PROJECT" >/dev/null 2>&1 ||
  gcloud iam service-accounts create github-deployer --project "$PROJECT" \
    --display-name="GitHub Actions deployer"

gcloud iam workload-identity-pools describe github --location=global --project "$PROJECT" >/dev/null 2>&1 ||
  gcloud iam workload-identity-pools create github --location=global --project "$PROJECT" \
    --display-name="GitHub Actions"

gcloud iam workload-identity-pools providers describe github-actions --location=global \
  --workload-identity-pool=github --project "$PROJECT" >/dev/null 2>&1 ||
  gcloud iam workload-identity-pools providers create-oidc github-actions --location=global \
    --workload-identity-pool=github --project "$PROJECT" \
    --display-name="GitHub Actions OIDC" \
    --issuer-uri="https://token.actions.githubusercontent.com" \
    --attribute-mapping="google.subject=assertion.sub,attribute.repository_id=assertion.repository_id,attribute.ref=assertion.ref" \
    --attribute-condition="assertion.repository_id=='$REPO_ID' && assertion.ref=='refs/heads/main'"

# Workflows in this repo may act as the deployer.
gcloud iam service-accounts add-iam-policy-binding "$DEPLOYER" --project "$PROJECT" --quiet \
  --role=roles/iam.workloadIdentityUser \
  --member="principalSet://iam.googleapis.com/$POOL/attribute.repository_id/$REPO_ID" >/dev/null

# Build from source and deploy (no setIamPolicy: can't make the backend public).
for role in roles/run.sourceDeveloper roles/serviceusage.serviceUsageConsumer; do
  gcloud projects add-iam-policy-binding "$PROJECT" --quiet --condition=None \
    --member="serviceAccount:$DEPLOYER" --role="$role" >/dev/null
done

# Deploy services that run as these accounts, and start builds that run as the
# default compute account.
for sa in agent-backend@$PROJECT.iam.gserviceaccount.com \
          frontend@$PROJECT.iam.gserviceaccount.com \
          $PROJECT_NUMBER-compute@developer.gserviceaccount.com; do
  gcloud iam service-accounts add-iam-policy-binding "$sa" --project "$PROJECT" --quiet \
    --member="serviceAccount:$DEPLOYER" --role=roles/iam.serviceAccountUser >/dev/null
done

echo "Done. GitHub Actions can now deploy as $DEPLOYER."
