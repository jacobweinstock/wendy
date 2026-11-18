.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ {printf "  %-16s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## Build bin/wendy
	CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o bin/wendy ./cmd/wendy

.PHONY: test
test: ## Run the unit tests
	go test -race ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

# Keep in sync with the go:generate directive in internal/ui/web/pages.go.
TEMPL_VERSION := v0.3.1118
TOOLS_DIR := $(CURDIR)/out/tools
TEMPL_BIN := $(TOOLS_DIR)/templ-$(TEMPL_VERSION)

$(TEMPL_BIN):
	mkdir -p $(TOOLS_DIR)
	GOBIN=$(TOOLS_DIR) go install github.com/a-h/templ/cmd/templ@$(TEMPL_VERSION)
	mv $(TOOLS_DIR)/templ $@

.PHONY: generate
generate: $(TEMPL_BIN) ## Regenerate the web UI templates
	cd internal/ui/web && $(TEMPL_BIN) generate

# The generated templates are committed so that go install works without templ.
.PHONY: verify-generate
verify-generate: generate ## Fail if generated files are out of date
	git diff --exit-code -- '*_templ.go'

.PHONY: snapshot
snapshot: ## Build release archives locally, without publishing
	goreleaser release --snapshot --clean

.PHONY: clean
clean: ## Remove build output
	rm -rf bin dist

.PHONY: clean-tools
clean-tools: ## Remove downloaded tools
	rm -rf out

############## Linting ##############
GOLANGCI_LINT_VERSION := v2.15.0
LINT_ARCH := $(shell uname -m)
LINT_ROOT := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
GOLANGCI_LINT_BIN := $(LINT_ROOT)/out/linters/golangci-lint-$(GOLANGCI_LINT_VERSION)-$(LINT_ARCH)

$(GOLANGCI_LINT_BIN):
	mkdir -p $(LINT_ROOT)/out/linters
	rm -rf $(LINT_ROOT)/out/linters/golangci-lint-*
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(LINT_ROOT)/out/linters $(GOLANGCI_LINT_VERSION)
	mv $(LINT_ROOT)/out/linters/golangci-lint $@

.PHONY: lint
lint: $(GOLANGCI_LINT_BIN) ## Run golangci-lint
	$(GOLANGCI_LINT_BIN) run

.PHONY: lint-fix
lint-fix: $(GOLANGCI_LINT_BIN) ## Run golangci-lint and apply its fixes
	$(GOLANGCI_LINT_BIN) run --fix
