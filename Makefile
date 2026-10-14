# Origin Proof. Deploys are handled by .github/workflows/deploy.yml, not here.
GO_DIR := backend
STATICCHECK ?= $(shell command -v staticcheck 2>/dev/null || echo $(HOME)/go/bin/staticcheck)

.PHONY: test lint fmt run smoke verify-data eval-dev eval-test eval-report

test: ## unit tests, no network
	cd $(GO_DIR) && go test ./...

lint: ## gofmt, go vet, staticcheck
	@cd $(GO_DIR) && out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	cd $(GO_DIR) && go vet ./...
	cd $(GO_DIR) && $(STATICCHECK) ./...

fmt:
	cd $(GO_DIR) && gofmt -w .

run: ## local server on :8080
	cd $(GO_DIR) && go run ./cmd/server

smoke: ## one live engine-model call (needs LLM_PROVIDER and its credentials in the env)
	cd $(GO_DIR) && go run ./cmd/server -smoke

verify-data: ## check committed eval data against eval/SHA256SUMS
	cd eval && shasum -a 256 -c SHA256SUMS

eval-dev: ## engine on the dev split (469; resumes, so a finished run just re-prints its summary)
	cd $(GO_DIR) && go run ./cmd/eval run -split dev

eval-test: ## held-out split (377): run once per frozen config; never tune on it
	cd $(GO_DIR) && go run ./cmd/eval run -split final -confirm-final

eval-report: ## summaries of the committed runs, fast path joined with router-v2 labels (no model calls)
	cd $(GO_DIR) && go run ./cmd/eval report -in ../eval/results/dev-gemini-4-argon-prec-engine-v1-litellm.jsonl -scope ../eval/results/scope-dev-router-v2.jsonl
	cd $(GO_DIR) && go run ./cmd/eval report -in ../eval/results/final-gemini-4-argon-prec-engine-v1-litellm.jsonl -scope ../eval/results/scope-final-router-v2.jsonl
