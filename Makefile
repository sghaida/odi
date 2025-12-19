SHELL := /bin/bash

# ---------- Local tools ----------
BIN_DIR := $(CURDIR)/bin
export PATH := $(BIN_DIR):$(PATH)

# ---------- Coverage artifacts ----------
COVERPROFILE := coverage.out
LCOV_REPORT  := coverage.lcov

# ---------- Codacy reporter ----------
CODACY_GETSH := https://coverage.codacy.com/get.sh

.PHONY: help tidy fmt fmt-check tidy-check vet lint lint-fix test \
        coverage tools-coverage coverage-lcov codacy-coverage clean

help:
	@echo "Targets:"
	@echo "  tidy           - go mod tidy"
	@echo "  tidy-check     - ensure go.mod/go.sum are tidy (CI-friendly)"
	@echo "  fmt            - gofmt all packages"
	@echo "  fmt-check      - ensure formatting is clean (CI-friendly)"
	@echo "  vet            - go vet ./..."
	@echo "  lint           - golangci-lint run (requires golangci-lint installed or via CI action)"
	@echo "  lint-fix       - golangci-lint run --fix (where supported)"
	@echo "  test           - go test ./..."
	@echo "  coverage       - go test with coverprofile ($(COVERPROFILE))"
	@echo "  coverage-lcov  - convert $(COVERPROFILE) -> $(LCOV_REPORT) (LCOV)"
	@echo "  codacy-coverage- upload $(LCOV_REPORT) to Codacy (requires CODACY_PROJECT_TOKEN)"
	@echo "  clean          - remove coverage artifacts and local tools"

# -------- Module hygiene --------
tidy:
	go mod tidy

tidy-check:
	@tmpdir="$$(mktemp -d)"; \
	cp go.mod go.sum "$$tmpdir/" 2>/dev/null || true; \
	go mod tidy; \
	if ! git diff --quiet -- go.mod go.sum; then \
	  echo "ERROR: go.mod/go.sum not tidy. Run 'go mod tidy' and commit changes."; \
	  git --no-pager diff -- go.mod go.sum; \
	  rm -rf "$$tmpdir"; \
	  exit 1; \
	fi; \
	rm -rf "$$tmpdir"

# -------- Formatting --------
fmt:
	gofmt -w $$(go list -f '{{.Dir}}' ./... | tr '\n' ' ')

fmt-check:
	@out="$$(gofmt -l $$(go list -f '{{.Dir}}' ./... | tr '\n' ' '))"; \
	if [ -n "$$out" ]; then \
	  echo "ERROR: gofmt needed on:"; \
	  echo "$$out"; \
	  exit 1; \
	fi

# -------- Analysis / lint --------
vet:
	go vet ./...

# Note: We intentionally do NOT install golangci-lint via `go install` here,
# because that can break in CI due to toolchain/x/tools compatibility.
# - In GitHub Actions, use golangci/golangci-lint-action.
# - Locally, install it once (brew install golangci-lint) or download a release.
lint:
	golangci-lint run --timeout=5m

lint-fix:
	golangci-lint run --timeout=5m --fix

# -------- Tests --------
test:
	go test ./...

# -------- Coverage --------
coverage:
	@PKGS=$$(go list ./... | grep -vE '/examples/|/mocks/|/mock[^/]*'); \
	echo "Covering packages:"; \
	echo "$$PKGS"; \
	go test $$PKGS \
	  -covermode=atomic \
	  -coverpkg=$$(echo $$PKGS | tr ' ' ',') \
	  -coverprofile=$(COVERPROFILE)
	@test -s $(COVERPROFILE) || (echo "ERROR: $(COVERPROFILE) is empty"; exit 1)


# Install ONLY the coverage converter (gcov2lcov) locally into ./bin (no sudo).
tools-coverage:
	@mkdir -p "$(BIN_DIR)"
	@echo "Installing gcov2lcov into $(BIN_DIR)..."
	@GOBIN="$(BIN_DIR)" go install github.com/jandelgado/gcov2lcov@latest
	@echo "OK: installed $(BIN_DIR)/gcov2lcov"

coverage-lcov: tools-coverage coverage
	@echo "Converting $(COVERPROFILE) -> $(LCOV_REPORT)..."
	@$(BIN_DIR)/gcov2lcov -infile "$(COVERPROFILE)" -outfile "$(LCOV_REPORT)"
	@test -s $(LCOV_REPORT) || (echo "ERROR: $(LCOV_REPORT) is empty"; exit 1)
	@echo "OK: wrote $(LCOV_REPORT)"

codacy-coverage: coverage-lcov
	@test -n "$$CODACY_PROJECT_TOKEN" || (echo "ERROR: CODACY_PROJECT_TOKEN is not set"; exit 1)
	@bash <(curl -fsSL $(CODACY_GETSH)) report \
		--language Go \
		--coverage-reports $(LCOV_REPORT)

clean:
	@rm -f $(COVERPROFILE) $(LCOV_REPORT)
	@rm -rf "$(BIN_DIR)"
	@echo "OK: cleaned"
