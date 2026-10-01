.PHONY: build test tidy release-check kind-reset kind-test

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	go build -ldflags "-X github.com/caseyrobb/kubescrub/internal/apply.Version=$(VERSION)" -o bin/kubescrub ./cmd/kubescrub

test:
	go test ./...

tidy:
	go mod tidy

release-check:
	go test ./...
	go build -o bin/kubescrub ./cmd/kubescrub

kind-reset:
	./scripts/kind.sh reset

kind-test:
	go test -tags=kind ./internal/app/...
