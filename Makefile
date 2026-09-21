# The tracked launcher lives at plugin/bin/skim; the compiled binary sits beside
# it as skim-bin so a build never overwrites the file the hooks depend on.
BIN := plugin/bin/skim-bin

.PHONY: build test lint fmt check smoke bench

build:
	go build -o $(BIN) ./cmd/skim
test:
	go test ./...
lint:
	go vet ./...
fmt:
	gofmt -l .

# Everything CI-able, no API calls.
check: build lint test
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }
	@echo "check: clean"

# Real-API smoke test. The entire fake-claude test suite was green while the
# product silently did nothing in production (Haiku wraps its JSON in a markdown
# fence, so every digest failed to parse and degraded open). Only a real call
# catches that class of bug — run this before any release.
# Costs a few cents and draws on your API/subscription limits.
smoke: build
	@command -v claude >/dev/null || { echo "smoke: 'claude' not on PATH"; exit 1; }
	@./scripts/smoke.sh

# Price-aware interception benchmark: does digesting this file actually cost
# less than letting the session model read it? Makes one real worker call.
#   make bench FILE=path/to/file [MODEL=opus-5|sonnet-5|haiku-4.5] [TURNS=10]
FILE  ?=
MODEL ?= opus-5
TURNS ?= 10
bench:
	@test -n "$(FILE)" || { echo "usage: make bench FILE=<path> [MODEL=opus-5] [TURNS=10]"; exit 2; }
	@python3 scripts/bench-digest.py "$(FILE)" --session-model "$(MODEL)" --turns "$(TURNS)"
