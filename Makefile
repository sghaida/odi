SHELL := /bin/bash

# -------- Tools / Paths --------
COVERPROFILE := coverage.out
LCOV_REPORT  := coverage.lcov

GCOV2LCOV_URL := https://raw.githubusercontent.com/jandelgado/gcov2lcov/master/gcov2lcov
CODACY_GETSH  := https://coverage.codacy.com/get.sh

.PHONY: help tidy fmt vet test coverage coverage-lcov codacy-coverage clean tools-gcov2lcov

help:
	@echo "Targets:"
	@echo "  tidy            - go mod tidy"
	@echo "  fmt             - gofmt ./..."
	@echo "  vet             - go vet ./..."
	@echo "  test            - go test ./..."
	@echo "  coverage        - generate Go coverage profile ($(COVERPROFILE))"
	@echo "  coverage-lcov   - convert $(COVERPROFILE) -> $(LCOV_REPORT) (LCOV)"
	@echo "  codacy-coverage - upload LCOV to Codacy (requires CODACY_PROJECT_TOKEN)"
	@echo "  clean           - remove coverage artifacts"

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

# Installs gcov2lcov locally into ./bin (no sudo needed; works on macOS + Linux)
tools-gcov2lcov:
	@mkdir -p ./bin
	@curl -fsSL $(GCOV2LCOV_URL) -o ./bin/gcov2lcov
	@chmod +x ./bin/gcov2lcov
	@echo "OK: installed ./bin/gcov2lcov"

coverage-lcov: tools-gcov2lcov coverage
	@./bin/gcov2lcov $(COVERPROFILE) > $(LCOV_REPORT)
	@test -s $(LCOV_REPORT) || (echo "ERROR: $(LCOV_REPORT) is empty"; exit 1)
	@echo "OK: wrote $(LCOV_REPORT)"

# Upload to Codacy using the official reporter script.
# Usage:
#   CODACY_PROJECT_TOKEN=xxxxx make codacy-coverage
codacy-coverage: coverage-lcov
	@test -n "$$CODACY_PROJECT_TOKEN" || (echo "ERROR: CODACY_PROJECT_TOKEN is not set"; exit 1)
	@bash <(curl -fsSL $(CODACY_GETSH)) report \
		--language Go \
		--coverage-reports $(LCOV_REPORT)

clean:
	@rm -f $(COVERPROFILE) $(LCOV_REPORT)
	@echo "OK: cleaned coverage artifacts"
