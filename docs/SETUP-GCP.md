# One-time GCP setup (run by a project owner)

Status (18 Oct): all steps are done. The corpus is uploaded (`make upload-corpus`). `agent-backend` has `roles/storage.objectViewer` on the corpus bucket, `roles/datastore.user` (granted by the project owner; it needs `setIamPolicy`, which Editor lacks) and `roles/aiplatform.user`, plus `roles/cloudbuild.builds.viewer` and `roles/run.viewer`.

Project `wassup-hackathon-2026`, region `us-central1`. The API runs as `agent-backend@`.

```bash
PROJECT=wassup-hackathon-2026
REGION=us-central1
SA=agent-backend@$PROJECT.iam.gserviceaccount.com

# 1. Enable Firestore, Cloud Storage and Vertex AI APIs
gcloud services enable firestore.googleapis.com storage.googleapis.com aiplatform.googleapis.com --project "$PROJECT"

# 2. Create the Firestore database (Native mode, default database)
gcloud firestore databases create --project "$PROJECT" --location "$REGION" --type firestore-native

# 3. Create the corpus bucket (uniform access, no public access)
# legacy name kept: the bucket was created under this name (D-rename).
gcloud storage buckets create "gs://$PROJECT-origin-proof-corpus" --project "$PROJECT" --location "$REGION" \
  --uniform-bucket-level-access --public-access-prevention

# 4. Let the API read/write Firestore and read the corpus bucket
gcloud projects add-iam-policy-binding "$PROJECT" --member "serviceAccount:$SA" --role roles/datastore.user --condition=None
gcloud storage buckets add-iam-policy-binding "gs://$PROJECT-origin-proof-corpus" --member "serviceAccount:$SA" --role roles/storage.objectViewer

# 5. Confirm the API can call Vertex AI (should list roles/aiplatform.user)
gcloud projects get-iam-policy "$PROJECT" --flatten "bindings[].members" \
  --filter "bindings.members:serviceAccount:$SA" --format "value(bindings.role)"
```

To give the team Vertex access for local runs and the official eval, each person needs `roles/aiplatform.user` on the project, then `gcloud auth application-default login`.
