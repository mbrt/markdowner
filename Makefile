.PHONY: test unit-test lint format build build-docker

test: lint unit-test

unit-test:
	go test -timeout=30s ./...
	go test -timeout=60s -race ./...

lint:
	revive ./...
	@test -z "$$(gofmt -s -l .)" || (echo "Unformatted files:"; gofmt -s -l .; exit 1)

format:
	go fmt ./...

build:
	go build .

build-docker:
	docker build -t markdowner:latest .
