# Makefile for Cascade

# ANSI color codes
BLUE := \033[1;34m
GREEN := \033[1;32m
YELLOW := \033[1;33m
RED := \033[1;31m
CYAN := \033[1;36m
RESET := \033[0m

# Variables
PROJECT_NAME := "Cascade"
CODE_NAME := "cascade"
MODULE_PATH := github.com/geomyidia/cascade
BIN_DIR := ./bin
MODE := debug
VERSION := $(shell cat project/VERSION 2>/dev/null || echo "unknown")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
GIT_SUMMARY := $(shell git describe --tags --dirty --always 2>/dev/null || echo "untagged")
BUILD_DATE := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
GO_VERSION := $(shell go version 2>/dev/null | awk '{print $$3}' || echo "unknown")

# Fully-qualified import path for the project (build-metadata) package
# targeted by ldflags.
VERSION_PKG := $(MODULE_PATH)/project

# Coverage
COVERAGE_FILE := coverage.out
COVERAGE_HTML := coverage.html
COVERAGE_THRESHOLD := 90

# List of binaries to build and install (matches subdirectories under cmd/)
BINARIES := cascade

# ldflags for version injection — populates the project package vars at link time.
# No inner quoting: -X values never contain spaces (commit hashes, refs, ISO-8601 timestamps),
# so the surrounding shell double-quotes in the recipe are sufficient.
LDFLAGS_VERSION := -X $(VERSION_PKG).Version=$(VERSION) \
                   -X $(VERSION_PKG).GitCommit=$(GIT_COMMIT) \
                   -X $(VERSION_PKG).GitBranch=$(GIT_BRANCH) \
                   -X $(VERSION_PKG).GitSummary=$(GIT_SUMMARY) \
                   -X $(VERSION_PKG).BuildDate=$(BUILD_DATE)

# Release builds strip debug info and trim local paths for reproducibility
LDFLAGS_RELEASE := -s -w $(LDFLAGS_VERSION)
GOFLAGS_RELEASE := -trimpath -ldflags "$(LDFLAGS_RELEASE)"
GOFLAGS_DEBUG   := -ldflags "$(LDFLAGS_VERSION)"

# Pick build flags by MODE; recursive (=) so build-release's MODE override is honored.
GO_BUILD_FLAGS = $(if $(filter release,$(MODE)),$(GOFLAGS_RELEASE),$(GOFLAGS_DEBUG))

# Git remotes to push to
GIT_REMOTES := macpro github codeberg
REMOTE_macpro := ssh://macpro.local:23231/geomyidia/$(CODE_NAME).git
REMOTE_github := git@github.com:geomyidia/$(CODE_NAME).git
REMOTE_codeberg := ssh://git@codeberg.org/geomyidia/$(CODE_NAME).git


# Default target
.DEFAULT_GOAL := help

# Help target
.PHONY: help
help:
	@echo ""
	@echo "$(CYAN)╔══════════════════════════════════════════════════════════╗$(RESET)"
	@echo "$(CYAN)║$(RESET) $(BLUE)$(PROJECT_NAME) Build System$(RESET)                                     $(CYAN)║$(RESET)"
	@echo "$(CYAN)╚══════════════════════════════════════════════════════════╝$(RESET)"
	@echo ""
	@echo "$(GREEN)Building:$(RESET)"
	@echo "  $(YELLOW)make build$(RESET)            - Build all binaries ($(BINARIES))"
	@echo "  $(YELLOW)make build-release$(RESET)    - Build optimized release binaries"
	@echo "  $(YELLOW)make build MODE=release$(RESET) - Build with custom mode"
	@echo "  $(YELLOW)make install$(RESET)          - go install all binaries to GOBIN"
	@echo ""
	@echo "$(GREEN)Testing & Quality:$(RESET)"
	@echo "  $(YELLOW)make test$(RESET)             - Run all tests with -race"
	@echo "  $(YELLOW)make lint$(RESET)             - Run gofmt + go vet + golangci-lint"
	@echo "  $(YELLOW)make format$(RESET)           - Format all code with gofmt + goimports"
	@echo "  $(YELLOW)make vet$(RESET)              - Run go vet"
	@echo "  $(YELLOW)make coverage$(RESET)         - Generate test coverage report"
	@echo "  $(YELLOW)make coverage-html$(RESET)    - Generate HTML coverage report"
	@echo "  $(YELLOW)make coverage-check$(RESET)   - Verify coverage meets $(COVERAGE_THRESHOLD)% threshold"
	@echo "  $(YELLOW)make docs$(RESET)             - Serve godoc locally via pkgsite"
	@echo "  $(YELLOW)make check$(RESET)            - Build + lint + test"
	@echo "  $(YELLOW)make check-all$(RESET)        - Build + lint + test + coverage gate"
	@echo ""
	@echo "$(GREEN)Dependencies:$(RESET)"
	@echo "  $(YELLOW)make check-deps$(RESET)       - Check for outdated dependencies"
	@echo "  $(YELLOW)make deps$(RESET)             - Update dependencies + go mod tidy"
	@echo "  $(YELLOW)make tidy$(RESET)             - Run go mod tidy"
	@echo ""
	@echo "$(GREEN)Cleaning:$(RESET)"
	@echo "  $(YELLOW)make clean$(RESET)            - Clean bin directory + coverage artifacts"
	@echo "  $(YELLOW)make clean-all$(RESET)        - Full clean (also go clean -cache -testcache)"
	@echo ""
	@echo "$(GREEN)Releasing:$(RESET)"
	@echo "  $(YELLOW)make release-dry-run$(RESET)  - Verify module is ready to tag (tidy + lint + test + build)"
	@echo "  $(YELLOW)make release VERSION=v0.1.0$(RESET) - Tag a release and push tag to all remotes"
	@echo ""
	@echo "$(GREEN)Utilities:$(RESET)"
	@echo "  $(YELLOW)make push$(RESET)             - Push to all configured remotes"
	@echo "  $(YELLOW)make remotes$(RESET)          - Configure git remotes"
	@echo "  $(YELLOW)make tracked-files$(RESET)    - Save list of tracked files"
	@echo ""
	@echo "$(GREEN)Information:$(RESET)"
	@echo "  $(YELLOW)make info$(RESET)             - Show build information"
	@echo "  $(YELLOW)make check-tools$(RESET)      - Verify required tools are installed"
	@echo ""
	@echo "$(CYAN)Current status:$(RESET) Branch: $(GIT_BRANCH) | Commit: $(GIT_COMMIT) | Tag: $(GIT_SUMMARY)"
	@echo ""

# Info target
.PHONY: info
info:
	@echo ""
	@echo "$(CYAN)╔══════════════════════════════════════════════════════════╗$(RESET)"
	@echo "$(CYAN)║$(RESET)  $(BLUE)Build Information$(RESET)                                       $(CYAN)║$(RESET)"
	@echo "$(CYAN)╚══════════════════════════════════════════════════════════╝$(RESET)"
	@echo ""
	@echo "$(GREEN)Project:$(RESET)"
	@echo "  Name:           $(PROJECT_NAME)"
	@echo "  Module:         $(MODULE_PATH)"
	@echo "  Build Mode:     $(MODE)"
	@echo "  Version:        $(VERSION)"
	@echo "  Build Date:     $(BUILD_DATE)"
	@echo ""
	@echo "$(GREEN)Paths:$(RESET)"
	@echo "  Binary Dir:     $(BIN_DIR)/"
	@echo "  Workspace:      $$(pwd)"
	@echo "  GOPATH:         $$(go env GOPATH 2>/dev/null || echo 'unknown')"
	@echo "  GOBIN:          $$(go env GOBIN 2>/dev/null || echo '<GOPATH>/bin')"
	@echo ""
	@echo "$(GREEN)Git:$(RESET)"
	@echo "  Branch:         $(GIT_BRANCH)"
	@echo "  Commit:         $(GIT_COMMIT)"
	@echo "  Summary:        $(GIT_SUMMARY)"
	@echo ""
	@echo "$(GREEN)Tools:$(RESET)"
	@echo "  Go:             $(GO_VERSION)"
	@echo "  gofmt:          $$(command -v gofmt >/dev/null 2>&1 && echo 'found' || echo 'not found')"
	@echo "  goimports:      $$(command -v goimports >/dev/null 2>&1 && echo 'found' || echo 'not found')"
	@echo "  golangci-lint:  $$(golangci-lint --version 2>/dev/null | head -1 || echo 'not found')"
	@echo ""
	@echo "$(GREEN)Binaries:$(RESET)"
	@for bin in $(BINARIES); do \
		if [ -f $(BIN_DIR)/$$bin ]; then \
			echo "  $$bin:          $(GREEN)✓ installed$(RESET)"; \
		else \
			echo "  $$bin:          $(RED)✗ not built$(RESET)"; \
		fi; \
	done
	@echo ""

# Check tools target
.PHONY: check-tools
check-tools:
	@echo "$(BLUE)Checking for required tools...$(RESET)"
	@command -v go >/dev/null 2>&1 && echo "$(GREEN)✓ go found ($(GO_VERSION))$(RESET)" || echo "$(RED)✗ go not found$(RESET)"
	@command -v gofmt >/dev/null 2>&1 && echo "$(GREEN)✓ gofmt found$(RESET)" || echo "$(RED)✗ gofmt not found$(RESET)"
	@command -v goimports >/dev/null 2>&1 && echo "$(GREEN)✓ goimports found$(RESET)" || echo "$(YELLOW)⊙ goimports not found (install: go install golang.org/x/tools/cmd/goimports@latest)$(RESET)"
	@command -v golangci-lint >/dev/null 2>&1 && echo "$(GREEN)✓ golangci-lint found$(RESET)" || echo "$(YELLOW)⊙ golangci-lint not found (install: https://golangci-lint.run/usage/install/)$(RESET)"
	@command -v pkgsite >/dev/null 2>&1 && echo "$(GREEN)✓ pkgsite found$(RESET)" || echo "$(YELLOW)⊙ pkgsite not found (install: go install golang.org/x/pkgsite/cmd/pkgsite@latest)$(RESET)"
	@command -v git >/dev/null 2>&1 && echo "$(GREEN)✓ git found$(RESET)" || echo "$(RED)✗ git not found$(RESET)"
	@test -f go.mod && echo "$(GREEN)✓ go.mod found$(RESET)" || echo "$(RED)✗ go.mod not found$(RESET)"

# Build directory creation
$(BIN_DIR):
	@echo "$(BLUE)Creating bin directory...$(RESET)"
	@mkdir -p $(BIN_DIR)
	@echo "$(GREEN)✓ Directory created$(RESET)"

# Build targets
.PHONY: build
build: clean $(BIN_DIR)
	@echo "$(BLUE)Building $(PROJECT_NAME) in $(MODE) mode...$(RESET)"
	@for bin in $(BINARIES); do \
		echo "$(CYAN)• Compiling cmd/$$bin...$(RESET)"; \
		if [ ! -d ./cmd/$$bin ]; then \
			echo "  $(YELLOW)⚠$(RESET) ./cmd/$$bin does not exist, skipping"; \
			continue; \
		fi; \
		go build $(GO_BUILD_FLAGS) -o $(BIN_DIR)/$$bin ./cmd/$$bin && \
			echo "  $(GREEN)✓$(RESET) $$bin" || \
			{ echo "  $(RED)✗$(RESET) $$bin failed to build"; exit 1; }; \
	done
	@echo "$(GREEN)✓ Build complete$(RESET)"
	@echo "$(CYAN)→ Binaries available in $(BIN_DIR)/$(RESET)"

.PHONY: build-release
build-release: MODE = release
build-release: clean $(BIN_DIR)
	@echo "$(BLUE)Building $(PROJECT_NAME) in release mode...$(RESET)"
	@for bin in $(BINARIES); do \
		echo "$(CYAN)• Compiling cmd/$$bin (optimized)...$(RESET)"; \
		if [ ! -d ./cmd/$$bin ]; then \
			echo "  $(YELLOW)⚠$(RESET) ./cmd/$$bin does not exist, skipping"; \
			continue; \
		fi; \
		go build $(GOFLAGS_RELEASE) -o $(BIN_DIR)/$$bin ./cmd/$$bin && \
			echo "  $(GREEN)✓$(RESET) $$bin (size: $$(du -h $(BIN_DIR)/$$bin | cut -f1))" || \
			{ echo "  $(RED)✗$(RESET) $$bin failed to build"; exit 1; }; \
	done
	@echo "$(GREEN)✓ Release build complete$(RESET)"
	@echo "$(CYAN)→ Optimized binaries in $(BIN_DIR)/$(RESET)"

.PHONY: install
install:
	@echo "$(BLUE)Installing binaries to $$(go env GOBIN || echo $$(go env GOPATH)/bin)...$(RESET)"
	@for bin in $(BINARIES); do \
		if [ ! -d ./cmd/$$bin ]; then \
			echo "  $(YELLOW)⚠$(RESET) ./cmd/$$bin does not exist, skipping"; \
			continue; \
		fi; \
		go install $(GOFLAGS_DEBUG) ./cmd/$$bin && \
			echo "  $(GREEN)✓$(RESET) installed $$bin" || \
			{ echo "  $(RED)✗$(RESET) failed to install $$bin"; exit 1; }; \
	done

# Cleaning targets
.PHONY: clean
clean:
	@echo "$(BLUE)Cleaning bin directory and coverage artifacts...$(RESET)"
	@rm -rf $(BIN_DIR) $(COVERAGE_FILE) $(COVERAGE_HTML)
	@echo "$(GREEN)✓ Clean complete$(RESET)"

.PHONY: clean-all
clean-all: clean
	@echo "$(BLUE)Performing full go clean...$(RESET)"
	@go clean -cache -testcache ./...
	@echo "$(GREEN)✓ Full clean complete$(RESET)"

# Testing & Quality targets
.PHONY: test
test:
	@echo "$(BLUE)Running tests...$(RESET)"
	@echo "$(CYAN)• go test -race -count=1 ./...$(RESET)"
	@go test -race -count=1 ./...
	@echo "$(GREEN)✓ All tests passed$(RESET)"

.PHONY: vet
vet:
	@echo "$(BLUE)Running go vet...$(RESET)"
	@go vet ./...
	@echo "$(GREEN)✓ go vet passed$(RESET)"

.PHONY: lint
lint:
	@echo "$(BLUE)Running linter checks...$(RESET)"
	@echo "$(CYAN)• Checking code formatting (gofmt)...$(RESET)"
	@unformatted="$$(gofmt -l . 2>/dev/null | grep -v '^vendor/' || true)"; \
	if [ -n "$$unformatted" ]; then \
		echo "$(RED)✗ The following files are not gofmt-clean:$(RESET)"; \
		echo "$$unformatted"; \
		echo "$(YELLOW)→ Run 'make format' to fix$(RESET)"; \
		exit 1; \
	fi
	@echo "$(GREEN)✓ Format check passed$(RESET)"
	@echo "$(CYAN)• Running go vet...$(RESET)"
	@go vet ./...
	@echo "$(GREEN)✓ go vet passed$(RESET)"
	@echo "$(CYAN)• Running golangci-lint...$(RESET)"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
		echo "$(GREEN)✓ golangci-lint passed$(RESET)"; \
	else \
		echo "$(YELLOW)⊙ golangci-lint not installed, skipping$(RESET)"; \
		echo "$(YELLOW)→ Install: https://golangci-lint.run/usage/install/$(RESET)"; \
	fi

.PHONY: format
format:
	@echo "$(BLUE)Formatting code...$(RESET)"
	@echo "$(CYAN)• Running gofmt -w on all files...$(RESET)"
	@gofmt -w .
	@if command -v goimports >/dev/null 2>&1; then \
		echo "$(CYAN)• Running goimports -w on all files...$(RESET)"; \
		goimports -w -local $(MODULE_PATH) .; \
	else \
		echo "$(YELLOW)⊙ goimports not installed, skipping$(RESET)"; \
		echo "$(YELLOW)→ Install: go install golang.org/x/tools/cmd/goimports@latest$(RESET)"; \
	fi
	@echo "$(GREEN)✓ Code formatted$(RESET)"

.PHONY: coverage
coverage:
	@echo "$(BLUE)Generating test coverage report...$(RESET)"
	@echo "$(CYAN)• go test -covermode=atomic -coverprofile=$(COVERAGE_FILE) ./...$(RESET)"
	@go test -covermode=atomic -coverprofile=$(COVERAGE_FILE) ./...
	@echo "$(GREEN)✓ Coverage profile written to $(COVERAGE_FILE)$(RESET)"
	@echo "$(CYAN)• Per-package summary:$(RESET)"
	@go tool cover -func=$(COVERAGE_FILE) | tail -20
	@echo "$(YELLOW)→ For detailed HTML report, run: make coverage-html$(RESET)"

.PHONY: coverage-html
coverage-html: coverage
	@echo "$(BLUE)Generating HTML coverage report...$(RESET)"
	@go tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "$(GREEN)✓ HTML coverage report generated$(RESET)"
	@echo "$(CYAN)→ Report: $(COVERAGE_HTML)$(RESET)"
	@echo "$(CYAN)→ Open:   file://$$(pwd)/$(COVERAGE_HTML)$(RESET)"

.PHONY: coverage-check
coverage-check: coverage
	@echo "$(BLUE)Checking coverage threshold ($(COVERAGE_THRESHOLD)%)...$(RESET)"
	@COVERAGE=$$(go tool cover -func=$(COVERAGE_FILE) | awk '/^total:/ {gsub(/%/,"",$$NF); print $$NF}'); \
	echo "$(CYAN)• Coverage: $${COVERAGE}%$(RESET)"; \
	if [ "$$(echo "$${COVERAGE} < $(COVERAGE_THRESHOLD)" | bc -l)" -eq 1 ]; then \
		echo "$(RED)✗ Coverage $${COVERAGE}% is below $(COVERAGE_THRESHOLD)% threshold$(RESET)"; \
		exit 1; \
	else \
		echo "$(GREEN)✓ Coverage $${COVERAGE}% meets threshold$(RESET)"; \
	fi

# Common checks
.PHONY: common-checks
common-checks: check-deps lint build

# Combined check targets
.PHONY: check
check: common-checks test
	@echo ""
	@echo "$(GREEN)✓ All checks passed (build + lint + test)$(RESET)"
	@echo ""

.PHONY: check-all
check-all: common-checks coverage-check
	@echo ""
	@echo "$(GREEN)✓ Full validation complete (build + lint + test + coverage gate)$(RESET)"
	@echo ""

.PHONY: check-deps
check-deps:
	@echo "$(BLUE)Checking for outdated dependencies...$(RESET)"
	@OUTPUT=$$(go list -u -m -mod=mod all 2>/dev/null | grep -E '\[' || true); \
	if [ -n "$$OUTPUT" ]; then \
		echo "$$OUTPUT"; \
		echo ""; \
		echo "$(YELLOW)⚠ Dependency updates available$(RESET)"; \
		echo "$(YELLOW)→ Run 'make deps' to update and commit the updated go.mod/go.sum$(RESET)"; \
		exit 1; \
	else \
		echo "$(GREEN)✓ All dependencies up to date$(RESET)"; \
	fi

.PHONY: deps
deps:
	@echo "$(BLUE)Updating dependencies...$(RESET)"
	@go get -u ./...
	@go mod tidy
	@echo "$(GREEN)✓ Dependencies upgraded; go.mod/go.sum updated$(RESET)"

.PHONY: tidy
tidy:
	@echo "$(BLUE)Running go mod tidy...$(RESET)"
	@go mod tidy
	@echo "$(GREEN)✓ Module tidied$(RESET)"

.PHONY: docs
docs:
	@echo "$(BLUE)Serving godoc locally...$(RESET)"
	@if ! command -v pkgsite >/dev/null 2>&1; then \
		echo "$(YELLOW)→ Installing pkgsite...$(RESET)"; \
		go install golang.org/x/pkgsite/cmd/pkgsite@latest; \
	fi
	@echo "$(CYAN)→ Browse: http://localhost:8080/$(MODULE_PATH)$(RESET)"
	@pkgsite -open .

# Utility targets
.PHONY: tracked-files
tracked-files: $(BIN_DIR)
	@echo "$(BLUE)Saving tracked files list...$(RESET)"
	@git ls-files > $(BIN_DIR)/git-tracked-files.txt
	@echo "$(GREEN)✓ Tracked files saved to $(BIN_DIR)/git-tracked-files.txt$(RESET)"
	@echo "$(CYAN)• Total files: $$(wc -l < $(BIN_DIR)/git-tracked-files.txt)$(RESET)"

.PHONY: remotes
remotes:
	@echo "$(BLUE)Configuring git remotes...$(RESET)"
	@for remote in $(GIT_REMOTES); do \
		case $$remote in \
			macpro)   url="$(REMOTE_macpro)"   ;; \
			github)   url="$(REMOTE_github)"   ;; \
			codeberg) url="$(REMOTE_codeberg)" ;; \
		esac; \
		if git remote get-url $$remote >/dev/null 2>&1; then \
			echo "  $(YELLOW)⊙$(RESET) $$remote already exists ($$url)"; \
		else \
			git remote add $$remote $$url; \
			echo "  $(GREEN)✓$(RESET) Added $$remote → $$url"; \
		fi; \
	done
	@echo "$(GREEN)✓ Remotes configured$(RESET)"

.PHONY: push
push:
	@echo "$(BLUE)Pushing changes...$(RESET)"
	@for remote in $(GIT_REMOTES); do \
		echo "$(CYAN)• $$remote:$(RESET)"; \
		git push $$remote main && git push $$remote --tags; \
		echo "$(GREEN)✓ Pushed$(RESET)"; \
	done

# Releasing — Go modules publish via git tags consumed by the Go module proxy.
.PHONY: release-dry-run
release-dry-run:
	@echo ""
	@echo "$(CYAN)╔══════════════════════════════════════════════════════════╗$(RESET)"
	@echo "$(CYAN)║$(RESET) $(BLUE)Dry Run: Releasing $(PROJECT_NAME)$(RESET)                            $(CYAN)║$(RESET)"
	@echo "$(CYAN)╚══════════════════════════════════════════════════════════╝$(RESET)"
	@echo ""
	@echo "$(BLUE)Verifying module is ready for release...$(RESET)"
	@echo "$(CYAN)• Checking go.mod is tidy...$(RESET)"
	@go mod tidy
	@if ! git diff --quiet go.mod go.sum 2>/dev/null; then \
		echo "$(RED)✗ go.mod/go.sum changed after 'go mod tidy' — commit before releasing$(RESET)"; \
		git --no-pager diff go.mod go.sum; \
		exit 1; \
	fi
	@echo "$(GREEN)✓ Module is tidy$(RESET)"
	@echo "$(CYAN)• Running lint...$(RESET)"
	@$(MAKE) --no-print-directory lint
	@echo "$(CYAN)• Running tests...$(RESET)"
	@$(MAKE) --no-print-directory test
	@echo "$(CYAN)• Verifying release build...$(RESET)"
	@$(MAKE) --no-print-directory build-release
	@echo ""
	@echo "$(GREEN)✓ Module ready for release!$(RESET)"
	@echo "$(CYAN)→ Run 'make release VERSION=vX.Y.Z' to tag and push$(RESET)"
	@echo ""

.PHONY: release
release:
	@if [ -z "$(VERSION)" ]; then \
		echo "$(RED)Error: VERSION variable not set$(RESET)"; \
		echo "Usage: make release VERSION=v0.1.0"; \
		exit 1; \
	fi
	@case "$(VERSION)" in \
		v[0-9]*.[0-9]*.[0-9]*) ;; \
		*) echo "$(RED)Error: VERSION must be semver-prefixed with 'v' (e.g. v0.1.0)$(RESET)"; exit 1 ;; \
	esac
	@if git rev-parse "$(VERSION)" >/dev/null 2>&1; then \
		echo "$(RED)Error: tag $(VERSION) already exists$(RESET)"; \
		exit 1; \
	fi
	@echo ""
	@echo "$(CYAN)╔══════════════════════════════════════════════════════════╗$(RESET)"
	@echo "$(CYAN)║$(RESET) $(BLUE)Releasing $(PROJECT_NAME) $(VERSION)$(RESET)                           $(CYAN)║$(RESET)"
	@echo "$(CYAN)╚══════════════════════════════════════════════════════════╝$(RESET)"
	@echo ""
	@echo "$(YELLOW)⚠ This will tag $(VERSION) and push the tag to:$(RESET)"
	@for remote in $(GIT_REMOTES); do echo "    - $$remote"; done
	@echo "$(YELLOW)⚠ Module proxies (proxy.golang.org) will index the tag automatically$(RESET)"
	@echo ""
	@read -p "Continue? [y/N] " -n 1 -r; \
	echo; \
	if [[ ! $$REPLY =~ ^[Yy]$$ ]]; then \
		echo "$(RED)✗ Aborted$(RESET)"; \
		exit 1; \
	fi
	@$(MAKE) --no-print-directory release-dry-run
	@echo "$(BLUE)Creating annotated tag $(VERSION)...$(RESET)"
	@git tag -a "$(VERSION)" -m "Release $(VERSION)"
	@echo "$(GREEN)✓ Tagged $(VERSION)$(RESET)"
	@for remote in $(GIT_REMOTES); do \
		echo "$(CYAN)• Pushing tag to $$remote...$(RESET)"; \
		git push $$remote "$(VERSION)" && \
			echo "  $(GREEN)✓$(RESET) pushed" || \
			{ echo "  $(RED)✗$(RESET) push to $$remote failed"; exit 1; }; \
	done
	@echo ""
	@echo "$(GREEN)✓ Released $(VERSION)$(RESET)"
	@echo "$(CYAN)→ Consumers can install with: go install $(MODULE_PATH)/cmd/cascade@$(VERSION)$(RESET)"
	@echo ""
