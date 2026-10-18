## Summary

Origin Proof cannot produce official numbers or deploy until a project owner sets up GCP for `wassup-hackathon-2026` (region `us-central1`, API service account `agent-backend@`). All development numbers so far go through a LiteLLM proxy and are labelled "proxy, development only".

## What this blocks

- **Official eval numbers:** the README results table shows "pending the Vertex AI re-run" for the engine and the fast path. Needs `make eval-test` and `make eval-summary` with `LLM_PROVIDER=vertex`.
- **Vertex smoke test:** `make smoke` with `LLM_PROVIDER=vertex`.
- **Deploy:** `deploy.yml` sets `LLM_PROVIDER=vertex`. The LiteLLM provider refuses to start on Cloud Run.
- **Case persistence on Cloud Run** (D25 in `docs/DECISIONS.md`): cases are JSON files on local disk, which Cloud Run loses on restart. Needs a Firestore implementation of `store.Cases`.
- **Corpus on the service:** the ruling corpus must be readable by the deployed API (bucket below).

## Owner steps (exact commands in `docs/SETUP-GCP.md`)

- [ ] Enable Firestore, Cloud Storage and Vertex AI APIs
- [ ] Create the Firestore database (Native mode, `us-central1`)
- [ ] Create the corpus bucket `gs://wassup-hackathon-2026-origin-proof-corpus` (uniform access, public access prevention)
- [ ] Give `agent-backend@` `roles/datastore.user` on the project and `roles/storage.objectViewer` on the bucket
- [ ] Confirm `agent-backend@` has `roles/aiplatform.user`
- [ ] Give each team member who runs evals `roles/aiplatform.user`; they then run `gcloud auth application-default login`

## Team steps after access

- [ ] `LLM_PROVIDER=vertex GOOGLE_CLOUD_PROJECT=wassup-hackathon-2026 GOOGLE_CLOUD_LOCATION=global make smoke`
- [ ] Official held-out run: `make eval-test`, then `make eval-summary`; update the README results table
- [ ] Firestore `store.Cases` implementation, selected when running on Cloud Run
- [ ] Upload the corpus to the bucket and point the service at it
- [ ] Merge the stacked PRs and deploy

## Done when

The README shows Vertex numbers for the engine and the fast path, and the deployed API keeps cases across a restart.
