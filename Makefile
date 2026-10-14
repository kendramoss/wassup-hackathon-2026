# Origin Proof. Deploys are handled by .github/workflows/deploy.yml, not here.
GO_DIR := backend
STATICCHECK ?= $(shell command -v staticcheck 2>/dev/null || echo $(HOME)/go/bin/staticcheck)

.PHONY: test lint fmt run smoke

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
