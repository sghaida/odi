SHELL := /bin/bash

# -------- Paths --------
BIN_DIR      := $(CURDIR)/bin
COVERPROFILE := coverage.out
LCOV_REPORT  := coverage.lcov

CODACY_GETSH := https://coverage.codacy.com/get.sh

# Ensure local bin is used first
export PATH := $(BIN_DIR):$(PATH)

.PHONY: help tidy fmt vet lint test coverage tools coverage-lcov codacy-coverage clean

help:
	@echo "Targets:"
	@echo "  tidy            - go mod tidy"
	@echo "  fmt             - gofmt ./..."
	@echo "  vet             - go vet ./..."
	@echo "  lint            - golangci-lint run"
	@echo "  test            - go test ./..."
	@echo "  coverage        - generate Go coverprofile ($(COVERPROFILE))"
	@echo "  tools           - install local tools into ./bin"
	@echo "  coverage-lcov   - convert $(COVERPROFILE) -> $(LCOV_REPORT)"
	@echo "  codacy-coverage - upload LCOV to Codacy"
	@echo "  clean           - cleanup"

tidy:
	go mod tidy

fmt:
	gofmt -w $$(go list -f '{{.Dir}}' ./... | tr '\n' ' ')

vet:
	go vet ./...

lint: tools
	@echo "Running golangci-lint..."
	@$(BIN_DIR)/golangci-lint run

test:
	go test ./...

coverage:
	go test ./... -covermode=atomic -coverpkg=./... -coverprofile=$(COVERPROFILE)
	@test -s $(COVERPROFILE) || (echo "ERROR: $(COVERPROFILE) is empty"; exit 1)

tools:
	@mkdir -p "$(BIN_DIR)"
	@echo "Installing tools into $(BIN_DIR)..."
	@GOBIN="$(BIN_DIR)" go install github.com/jandelgado/gcov2lcov@latest
	@GOBIN="$(BIN_DIR)" go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.61.0
	@echo "OK: tools installed"

coverage-lcov: tools coverage
	@$(BIN_DIR)/gcov2lcov -infile "$(COVERPROFILE)" -outfile "$(LCOV_REPORT)"
	@test -s $(LCOV_REPORT) || (echo "ERROR: $(LCOV_REPORT) is empty"; exit 1)

codacy-coverage: coverage-lcov
	@test -n "$$CODACY_PROJECT_TOKEN" || (echo "CODACY_PROJECT_TOKEN not set"; exit 1)
	@bash <(curl -fsSL $(CODACY_GETSH)) report \
		--language Go \
		--coverage-reports $(LCOV_REPORT)

clean:
	@rm -rf "$(BIN_DIR)" $(COVERPROFILE) $(LCOV_REPORT)
