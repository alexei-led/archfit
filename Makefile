## archfit — architecture drift feedback for CI and AI agents
## Usage: make [target]

BINARY         := archfit
CMD            := ./cmd/archfit
BIN_DIR        := .bin
MODULE         := github.com/alexei-led/archfit
ARCHFIT_CONFIG := .archfit.yaml
ARCHFIT_REPORT := docs/reports/archfit-report.md

VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT    := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE      := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS   := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

RAW_TEST_PKGS := $(filter-out test,$(MAKECMDGOALS))
ifeq ($(firstword $(MAKECMDGOALS)),test)
TEST_PKGS := $(if $(RAW_TEST_PKGS),$(foreach p,$(RAW_TEST_PKGS),$(if $(filter ./% /%,$(p)),$(p),./$(p))),./...)
endif

GOLANGCI_LINT_VERSION := v2.1.6
MOQ_VERSION           := v0.4.0

.DEFAULT_GOAL := help

## help: show this help message
.PHONY: help
help: ## show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

## all: fmt lint test archfit
.PHONY: all
all: fmt lint test archfit ## fmt + lint + test + architecture drift gate

## build: compile the archfit binary
.PHONY: build
build: ## compile the archfit binary
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(CMD)

## test: run tests with race detector and coverage
.PHONY: test
test: ## run tests with race detector and coverage
	go test -race -coverprofile=coverage.out $(if $(TEST_PKGS),$(TEST_PKGS),./...)
	python3 internal/extract/scip/scip_reader_test.py
	bash scripts/tests/cli_exit_contract_test.sh

ifeq ($(firstword $(MAKECMDGOALS)),test)
.PHONY: $(RAW_TEST_PKGS)
$(RAW_TEST_PKGS):
	@:
endif

## test-fast: run tests with race detector, skipping slow subprocess integration tests (-short)
.PHONY: test-fast
test-fast: ## run tests with race + -short (skips real-sg subprocess integration tests for inner-loop speed)
	go test -race -short ./...

## test-coverage: open HTML coverage report
.PHONY: test-coverage
test-coverage: test ## open HTML coverage report
	go tool cover -html=coverage.out

## archfit: run architecture drift gate on this repo
.PHONY: archfit
archfit: build ## run archfit check against this repo's architecture policy
	@$(BIN_DIR)/$(BINARY) check --config $(ARCHFIT_CONFIG) --require-tools; rc=$$?; \
	if [ $$rc -eq 0 ] || [ $$rc -eq 2 ]; then exit 0; fi; \
	exit $$rc

## arch-lint: architecture drift linter — fails on any blocking architecture
## violation (forbidden dependency, layer inversion, god-struct ceiling). Alias
## for the archfit dogfood gate; wired into the pre-push hook so drift is caught
## before it reaches CI. `make all` runs this too.
.PHONY: arch-lint
arch-lint: archfit ## architecture drift linter (alias for the archfit dogfood gate)

## archfit-report: write a Markdown architecture audit report
.PHONY: archfit-report
archfit-report: build ## write docs/reports/archfit-report.md for human review
	@mkdir -p $(dir $(ARCHFIT_REPORT))
	$(BIN_DIR)/$(BINARY) analyze --markdown --config $(ARCHFIT_CONFIG) > $(ARCHFIT_REPORT)
	@echo "archfit report written to $(ARCHFIT_REPORT)"

## lint: run golangci-lint
.PHONY: lint
lint: ## run golangci-lint
	golangci-lint run -c .golangci.yaml ./...

## fmt: format Go source with gofmt and goimports
.PHONY: fmt
fmt: ## format Go source with gofmt and goimports
	gofmt -s -w .
	goimports -w -local $(MODULE) .

## schema: regenerate archfit.schema.json from internal/config structs
.PHONY: schema
schema: ## regenerate archfit.schema.json (run when config structs change)
	ARCHFIT_UPDATE_SCHEMA=1 go test ./internal/configschema/ -run TestSchemaNoDrift -count=1
	go test ./internal/configschema/ -run TestSchemaNoDrift -count=1

## mock: regenerate moq mocks via go generate
.PHONY: mock
mock: ## regenerate moq mocks via go generate
	go generate ./...

## setup-tools: install pinned development tools
.PHONY: setup-tools
setup-tools: ## install pinned development tools
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/matryer/moq@$(MOQ_VERSION)

## release: cross-compile for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 into dist/
.PHONY: release
release: ## cross-compile release binaries into dist/ with SHA256SUMS
	@mkdir -p dist
	@for platform in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do \
		os=$$(echo $$platform | cut -d/ -f1); \
		arch=$$(echo $$platform | cut -d/ -f2); \
		out=dist/$(BINARY)-$(VERSION)-$$os-$$arch; \
		echo "building $$out"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build \
			-trimpath \
			-ldflags "-s -w $(LDFLAGS)" \
			-o $$out \
			$(CMD); \
	done
	@cd dist && \
		if command -v sha256sum >/dev/null 2>&1; then \
			sha256sum * > SHA256SUMS; \
		else \
			shasum -a 256 * > SHA256SUMS; \
		fi
	@echo "release binaries written to dist/"

## docker-build: build multi-arch Docker image (requires docker buildx)
.PHONY: docker-build
docker-build: ## build multi-arch Docker image for linux/amd64 and linux/arm64
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		-t ghcr.io/alexei-led/$(BINARY):$(VERSION) \
		.

## docker-push: push multi-arch Docker image to GHCR (requires docker login ghcr.io)
.PHONY: docker-push
docker-push: ## push multi-arch image to ghcr.io (run: docker login ghcr.io first)
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		-t ghcr.io/alexei-led/$(BINARY):$(VERSION) \
		--push \
		.

## docker-run: smoke-test the published image with --help
.PHONY: docker-run
docker-run: ## run archfit --help from the GHCR image (smoke test)
	docker run --rm ghcr.io/alexei-led/$(BINARY):$(VERSION) --help

## build-calibrate: compile the calibrate scorer-comparison binary
.PHONY: build-calibrate
build-calibrate: ## compile the calibrate dev tool binary to .bin/calibrate
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -o $(BIN_DIR)/calibrate ./cmd/calibrate

## calibrate: compare AdditiveScorer vs MultiplicativeScorer on archfit (informational dev tool)
.PHONY: calibrate
calibrate: build-calibrate ## compare scorers on archfit; emits calibration-report.json (informational only)
	@bash scripts/calibrate.sh .

## bench-gate: cold vs warm fact-cache gate timing on this repo (reported number, not a CI assert)
.PHONY: bench-gate
bench-gate: build ## time archfit check cold (fact cache cleared) then warm
	@sh scripts/bench-gate.sh

## clean: remove build artefacts
.PHONY: clean
clean: ## remove build artefacts
	rm -rf $(BIN_DIR) dist/ coverage.out

## version: print build version info
.PHONY: version
version: ## print build version info
	@echo "version: $(VERSION)  commit: $(COMMIT)  date: $(DATE)"
