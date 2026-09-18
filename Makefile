.PHONY: test race build fmt vet

test:
	go test ./...

race:
	go test -race ./...

build:
	go build -o bin/nebuladb$(shell go env GOEXE) ./cmd/nebuladb
	go build -o bin/nebulactl$(shell go env GOEXE) ./cmd/nebulactl

fmt:
	go fmt ./...

vet:
	go vet ./...
