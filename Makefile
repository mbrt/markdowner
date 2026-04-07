.PHONY: test unit-test lint format build build-docker

test: lint unit-test

unit-test:
	go test -timeout=30s ./...
	go test -timeout=60s -race ./...

lint:
	go tool revive -config revive.toml ./...
	@test -z "$$(gofmt -s -l .)" || (echo "Unformatted files:"; gofmt -s -l .; exit 1)
	@files=$$(go fix -json ./... 2>&1 | jq -r 'to_entries[] | select(.key | test("internal/proto") | not) | .value | .. | .filename? // empty' | sort -u); \
	if [ -n "$$files" ]; then echo "Files need go fix:"; echo "$$files"; exit 1; fi

format:
	go fmt ./...

build:
	go build .

build-docker:
	docker build -t markdowner:latest .
