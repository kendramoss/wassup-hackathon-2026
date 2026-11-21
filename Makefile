# Unde Labs. Deploys are handled by .github/workflows/deploy.yml, not here.
GO_DIR := backend
STATICCHECK ?= $(shell command -v staticcheck 2>/dev/null || echo $(HOME)/go/bin/staticcheck)

.PHONY: demo-seed test lint fmt run smoke reproduce verify-data eval-dev eval-test eval-report eval-slice prompt-snapshot eval-summary bundle-corpus upload-corpus

test: ## unit tests, no network
	cd $(GO_DIR) && go test ./...

lint: ## gofmt, go vet, staticcheck
	@cd $(GO_DIR) && out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	cd $(GO_DIR) && go vet ./...
	cd $(GO_DIR) && $(STATICCHECK) ./...

fmt:
	cd $(GO_DIR) && gofmt -w .

run: ## local server on :8080
	cd $(GO_DIR) && ADMIN_TOKEN=$${ADMIN_TOKEN:-local-dev-admin-token} go run ./cmd/server

smoke: ## one live Vertex call (Application Default Credentials and GOOGLE_CLOUD_PROJECT)
	cd $(GO_DIR) && go run ./cmd/server -smoke

reproduce: ## recompute every published number from committed files: no model calls, no credentials
	$(MAKE) verify-data
	cd $(GO_DIR) && go run ./cmd/claimscheck
	python3 -I eval/charts/build_evaluation.py . --check
	python3 -I eval/charts/build_trust_data.py . --check
	python3 -I eval/charts/build_accuracy.py . --check
	python3 -I eval/charts/build_repeatability.py . --check
	python3 -I eval/planner/summarize.py . --check
	python3 -I eval/charts/build_adjudicator.py . --check
	python3 -I eval/planner/summarize_addendum.py . --check

verify-data: ## check committed eval data against eval/SHA256SUMS and the bundled corpus against it
	cd eval && shasum -a 256 -c SHA256SUMS
	cmp eval/data/labels.jsonl backend/data/labels.jsonl
	cmp eval/data/labels_usmca.jsonl backend/data/labels_usmca.jsonl
	@if [ -d data/corpus/facts ]; then diff -rq data/corpus/facts backend/data/facts --exclude=split_meta.jsonl --exclude=.generated_manifest.json --exclude=.bundle_manifest.json && echo "bundled facts match data/corpus/facts"; fi

bundle-corpus: ## copy labels and the built facts corpus into backend/data (baked into the image); removes only bundled files whose source is gone (manifest-owned)
	mkdir -p backend/data/facts
	cp eval/data/labels.jsonl backend/data/labels.jsonl
	cp eval/data/labels_usmca.jsonl backend/data/labels_usmca.jsonl
	cd backend && go run ./cmd/corpus bundle -from ../data/corpus/facts -to data/facts

# legacy name kept: the GCS bucket exists under this name (renaming needs a migration, D-rename).
CORPUS_BUCKET ?= gs://wassup-hackathon-2026-origin-proof-corpus
upload-corpus: ## copy the bundled corpus (labels + facts) to the corpus bucket (docs/SETUP-GCP.md)
	gcloud storage rsync --recursive --delete-unmatched-destination-objects backend/data $(CORPUS_BUCKET)/bundled

eval-dev: ## engine on the dev split (469; resumes, so a finished run just re-prints its summary)
	cd $(GO_DIR) && go run ./cmd/eval run -split dev

eval-test: ## held-out split (377): run once per frozen config; never tune on it
	cd $(GO_DIR) && go run ./cmd/eval run -split final -confirm-final

eval-report: ## reprint the recorded proxy runs (the *-litellm.jsonl inputs stay as history; no model calls)
	cd $(GO_DIR) && go run ./cmd/eval report -in ../eval/results/dev-gemini-4-argon-prec-engine-v1-litellm.jsonl -scope ../eval/results/scope-dev-router-v2.jsonl
	cd $(GO_DIR) && go run ./cmd/eval report -in ../eval/results/final-gemini-4-argon-prec-engine-v1-litellm.jsonl -scope ../eval/results/scope-final-router-v2.jsonl

eval-slice: ## regression slice (D72): 30 dev rulings vs the committed baseline (~60 model calls; also .github/workflows/eval-slice.yml)
	cd $(GO_DIR) && EVAL_SLICE=on go run ./cmd/eval slice

prompt-snapshot: ## rewrite backend/internal/promptsnap/snapshot.json after bumping a prompt version (D72)
	cd $(GO_DIR) && go test ./internal/promptsnap -update

eval-summary: ## regenerate the numbers served by GET /v1/eval/summary (embedded in the server)
	cd $(GO_DIR) && go run ./cmd/eval summary

demo-seed: ## prepare a running server (make run) for recording: every demo case, plans, scenarios, watch, ask, country comparison; checks verdicts repeat (docs/DEMO.md)
	cd $(GO_DIR) && ADMIN_TOKEN=$${ADMIN_TOKEN:-local-dev-admin-token} go run ./cmd/demo seed -out ../eval/results/demo-seed.json

