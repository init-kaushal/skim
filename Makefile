BIN := plugin/bin/skim

.PHONY: build test lint
build:
	go build -o $(BIN) ./cmd/skim
test:
	go test ./...
lint:
	go vet ./...
