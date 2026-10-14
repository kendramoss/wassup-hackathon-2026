# Origin Proof. Deploys are handled by .github/workflows/deploy.yml, not here.
GO_DIR := backend
STATICCHECK ?= $(shell command -v staticcheck 2>/dev/null || echo $(HOME)/go/bin/staticcheck)

.PHONY: test lint fmt run smoke verify-data

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
