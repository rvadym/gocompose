include Makefile.dist.val
-include Makefile.val

.DEFAULT_GOAL := help

.PHONY: help lint test test-cover mod-sync verify

help: ## Show available make targets
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-zA-Z0-9_.-]+:.*##/ { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# Go -------------------------------------------------------------------------

lint: ## Run go vet and other linters
	$(GO) vet ./...

test: ## Run tests
	$(GO) test $(GO_TEST_FLAGS) ./...

test-cover: ## Run tests and generate an HTML coverage report
	mkdir -p $(COVERAGE_DIR)
	$(GO) test $(GO_TEST_FLAGS) -coverprofile=$(COVERAGE_PROFILE) ./...
	$(GO) tool cover -html=$(COVERAGE_PROFILE) -o $(COVERAGE_HTML)

mod-sync: ## Download, tidy, and vendor Go modules
	$(GO) mod download
	$(GO) mod tidy
	$(GO) mod vendor

verify: mod-sync lint test ## Run tests
