test:
	go test -race ./...

lint:
	golangci-lint --config .golangci.yml run --out-format=github-actions ./...

.DEFAULT_GOAL := all
all: lint test