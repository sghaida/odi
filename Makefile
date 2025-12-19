SHELL := /bin/bash

# -------- Paths --------
BIN_DIR      := $(CURDIR)/bin
COVERPROFILE := coverage.out
LCOV_REPORT  := coverage.lcov

CODACY_GETSH := https://coverage.codacy.com/get.sh

# Ensure local bin is used first
export PATH := $(BIN_DIR):$(PATH)

.PHONY: help tidy fmt vet test coverage tools coverage-lcov codacy-coverage clean

help:
	@echo "Targets:"
	@echo "  tidy            - go mod tidy"
	@echo "  fmt             - gofmt ./..."
	@echo "  vet             - go vet ./..."
	@echo "  test            - go test ./..."
	@echo "  coverage        - generate Go coverprofile ($(COVERPROFILE))"
	@echo "  tools           - install local tools into ./bin"
	@echo "  coverage-lcov   - convert $(COVERPROFILE) -> $(LCOV_REPORT) (LCOV)"
	@echo "  codacy-coverage - upload LCOV to Codacy (requires CODACY_PROJECT_TOKEN)"
	@echo "  clean           - remove coverage artifacts and ./bin tools"

tidy:
	go mod tidy

fmt:
	gofmt -w $$(go list -f '{{.Dir}}' ./... | tr '\n' ' ')

vet:
	go vet ./...

test:
	go test ./...

coverage:
	go test ./... -covermode=atomic -coverpkg=./... -coverprofile=$(COVERPROFILE)
	@test -s $(COVERPROFILE) || (echo "ERROR: $(COVERPROFILE) is empty"; exit 1)
	@echo "OK: wrote $(COVERPROFILE)"

tools:
	@mkdir -p "$(BIN_DIR)"
	@echo "Installing gcov2lcov into $(BIN_DIR)..."
	@GOBIN="$(BIN_DIR)" go install github.com/jandelgado/gcov2lcov@latest
	@echo "OK: installed $(BIN_DIR)/gcov2lcov"

coverage-lcov: tools coverage
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
