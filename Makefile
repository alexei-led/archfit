## archfit — architecture drift feedback for CI and AI agents
## Usage: make [target]

BINARY         := archfit
CMD            := ./cmd/archfit
BIN_DIR        := .bin
MODULE         := github.com/alexei-led/archfit/v3
ARCHFIT_CONFIG := .archfit.yaml
ARCHFIT_REPORT := .archfit-cache/reports/archfit-report.txt

VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT    := $(shell sh scripts/build-meta.sh commit)
DATE      := $(shell sh scripts/build-meta.sh date)

LDFLAGS   := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

RAW_TEST_PKGS := $(filter-out test,$(MAKECMDGOALS))
ifeq ($(firstword $(MAKECMDGOALS)),test)
TEST_PKGS := $(if $(RAW_TEST_PKGS),$(foreach p,$(RAW_TEST_PKGS),$(if $(filter ./% /%,$(p)),$(p),./$(p))),./...)
endif

GOLANGCI_LINT_VERSION := v2.15.0
MOQ_VERSION           := v0.8.1
JSONSCHEMA_VERSION    := 4.26.0
ACTIONLINT_VERSION    := v1.7.12
ZIZMOR_VERSION        := 1.31.1
# goimports ships with golang.org/x/tools; install the version in go.mod so the
# formatter cannot drift from the toolchain the code builds with.
GOIMPORTS_VERSION     := $(shell go list -m -f '{{.Version}}' golang.org/x/tools 2>/dev/null)

.DEFAULT_GOAL := help

## help: show this help message
.PHONY: help
help: ## show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

## all: fmt lint test arch-lint
.PHONY: all
all: fmt lint test arch-lint ## fmt + lint + architecture change gate

## build: compile the archfit binary
.PHONY: build
build: ## compile the archfit binary
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(CMD)

## test: run tests with race detector and coverage
# The SCIP reader test uses ARCHFIT_TOOL_PYTHON when set, as CI does. Otherwise it
# runs where the extractor falls back (internal/extract/scip/scip_strength.go): an
# isolated uv Python 3.14 with the reader pins from tools/python/requirements.in.
SCIP_READER_DEPS   = $(shell grep -E '^(protobuf|grpcio-tools)==' tools/python/requirements.in)
SCIP_READER_PYTHON = $(if $(ARCHFIT_TOOL_PYTHON),"$$ARCHFIT_TOOL_PYTHON",uv run --no-config --config-file /dev/null --no-env-file --no-project --isolated --python 3.14 $(addprefix --with ,$(SCIP_READER_DEPS)) python)

.PHONY: test
test: ## run tests with race detector and coverage
	go test -race -coverprofile=coverage.out $(if $(TEST_PKGS),$(TEST_PKGS),./...)
	$(SCIP_READER_PYTHON) -I internal/extract/scip/scip_reader_test.py
	uv run --with jsonschema==$(JSONSCHEMA_VERSION) python -m unittest discover -s scripts/eval -p '*_test.py'
	bash scripts/tests/cli_exit_contract_test.sh

## language-contracts: run the bounded real-tool acceptance gate
.PHONY: language-contracts
language-contracts: build dotnet-roslyn ## validate Go, TypeScript, JavaScript, Python, Rust, and .NET with real producers
	uv run --with jsonschema==$(JSONSCHEMA_VERSION) python scripts/eval/language_contracts.py --archfit $(BIN_DIR)/$(BINARY)

## dotnet-roslyn: build the .NET type-graph helper; point ARCHFIT_DOTNET_ROSLYN at its directory
DOTNET_ROSLYN_SRC := tools/dotnet/archfit-roslyn
DOTNET_ROSLYN_DIR := $(BIN_DIR)/dotnet-roslyn
DOTNET            := DOTNET_CLI_TELEMETRY_OPTOUT=1 DOTNET_NOLOGO=1 dotnet

.PHONY: dotnet-roslyn dotnet-roslyn-test
dotnet-roslyn: ## build the Roslyn helper into .bin/dotnet-roslyn (restores in locked mode)
	$(DOTNET) publish $(DOTNET_ROSLYN_SRC)/src -c Release -o $(DOTNET_ROSLYN_DIR)

dotnet-roslyn-test: ## run the Roslyn helper xUnit tests (loads internal/testdata/dotnet from a temp copy)
	$(DOTNET) test $(DOTNET_ROSLYN_SRC)/tests

.PHONY: tool-locks image-contracts
tool-locks: ## validate the canonical analyzer pins and dependency locks
	python3 scripts/provision_tools.py check
	python3 -m unittest discover -s scripts/tests -p '*_test.py'

image-contracts: ## validate a built final image (set IMAGE, DOCKER_CONTEXT, IMAGE_CONTRACT_OUTPUT)
	python3 scripts/image_contracts.py --image "$(IMAGE)" --docker-context "$(DOCKER_CONTEXT)" --output-dir "$(IMAGE_CONTRACT_OUTPUT)"

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
## It accepts pass (0) and unknown (2, printed): a policy change on a branch is unknown until it
## lands, and the App's required check is the verdict authority. Only fail (1) and tool error (3) block.
.PHONY: archfit
archfit: build ## run archfit check against this repo's architecture policy
	@$(BIN_DIR)/$(BINARY) check; rc=$$?; \
	if [ $$rc -eq 0 ] || [ $$rc -eq 2 ]; then exit 0; fi; \
	exit $$rc

## arch-lint: run the architecture change gate
.PHONY: arch-lint
arch-lint: archfit ## run the architecture change gate

## archfit-report: write a repository architecture report
.PHONY: archfit-report
archfit-report: build ## write .archfit-cache/reports/archfit-report.txt for human review
	@mkdir -p $(dir $(ARCHFIT_REPORT))
	$(BIN_DIR)/$(BINARY) report > $(ARCHFIT_REPORT)
	@echo "archfit report written to $(ARCHFIT_REPORT)"

## lint: run golangci-lint
.PHONY: lint
lint: ## run golangci-lint
	golangci-lint run -c .golangci.yaml ./...

## workflows: lint GitHub Actions workflow files and test the release tag rule
.PHONY: workflows
workflows: ## lint .github/workflows with actionlint and zizmor; test the release tag rule
	go run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION) .github/workflows/*.yaml
	uvx zizmor==$(ZIZMOR_VERSION) --format plain .github/workflows
	bash scripts/tests/release_channel_test.sh

## docs: validate documentation and render Mermaid diagrams
.PHONY: docs
docs: ## validate links, Markdown, examples, and diagrams (install docs tools first)
	npm --prefix scripts/eval/docs test
	npm --prefix scripts/eval/docs run check

## fmt: format Go source with gofmt and goimports
.PHONY: fmt
fmt: ## format Go source with gofmt and goimports
	gofmt -s -w .
	goimports -w -local $(MODULE) .

## schema: regenerate the published JSON Schemas from the Go structs
.PHONY: schema
schema: ## regenerate archfit.schema.json + archfit.state.schema.json + archfit.agent-result.schema.json + archfit.result.schema.json (run when config, report, agent-result, or result structs change)
	ARCHFIT_UPDATE_SCHEMA=1 go test ./internal/configschema/ -run TestSchemaNoDrift -count=1
	go test ./internal/configschema/ -run TestSchemaNoDrift -count=1
	ARCHFIT_UPDATE_SCHEMA=1 go test ./internal/reportschema/ -run 'Test(State|AgentResult|Result)SchemaNoDrift' -count=1
	go test ./internal/reportschema/ -count=1

## mock: regenerate moq mocks via go generate
.PHONY: mock
mock: ## regenerate moq mocks via go generate
	go generate ./...

## setup-tools: install pinned development tools
.PHONY: setup-tools
setup-tools: ## install pinned development tools
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	go install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)
	go install github.com/matryer/moq@$(MOQ_VERSION)

## host-tools: install the locked Node and Python analyzer environments for host runs
# Host counterpart of scripts/provision-analyzers.sh for the Node and Python locks
# only: that script also compiles scip-go and cargo-modules into an image prefix.
# PATH and shell profiles stay with the user: the target only prints what to set.
HOST_TOOLS_DIR      := $(HOME)/.local/share/archfit-tools
TOOL_PYTHON_VERSION  = $(shell python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["versions"]["python"])' tools/manifest.json)

.PHONY: host-tools
host-tools: ## install locked depcruise and ARCHFIT_TOOL_PYTHON under HOST_TOOLS_DIR (default ~/.local/share/archfit-tools)
	mkdir -p "$(HOST_TOOLS_DIR)/node"
	cp tools/node/package.json tools/node/package-lock.json "$(HOST_TOOLS_DIR)/node/"
	npm ci --prefix "$(HOST_TOOLS_DIR)/node" --no-audit --no-fund
	uv venv --clear --python $(TOOL_PYTHON_VERSION) "$(HOST_TOOLS_DIR)/python"
	uv pip sync --python "$(HOST_TOOLS_DIR)/python/bin/python" --require-hashes --only-binary :all: tools/python/requirements.lock
	@echo "depcruise: $(HOST_TOOLS_DIR)/node/node_modules/.bin/depcruise"
	@echo "ARCHFIT_TOOL_PYTHON=$(HOST_TOOLS_DIR)/python/bin/python"

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
		--build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg DATE=$(DATE) \
		.

## docker-push: push multi-arch Docker image to GHCR (requires docker login ghcr.io)
.PHONY: docker-push
docker-push: ## push multi-arch image to ghcr.io (run: docker login ghcr.io first)
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		-t ghcr.io/alexei-led/$(BINARY):$(VERSION) \
		--build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg DATE=$(DATE) \
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
