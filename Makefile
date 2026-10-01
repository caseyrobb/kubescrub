.PHONY: build test tidy kind-reset kind-test test-cover

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	go build -ldflags "-X github.com/caseyrobb/kubescrub/internal/apply.Version=$(VERSION)" -o bin/kubescrub ./cmd/kubescrub

test:
	go test ./...

test-cover:
	go test -coverprofile=coverage.out -count=1 ./...

tidy:
	go mod tidy

kind-reset:
	./scripts/kind.sh reset

kind-test:
	go test -tags=kind ./internal/app/...
