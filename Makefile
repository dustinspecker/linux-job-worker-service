.DEFAULT_GOAL := all

all: lint proto test-unit build certs

build:
	go build -o bin/ ./...

certs:
	./scripts/make-certificates.sh

lint:
	golangci-lint run \
		--disable cyclop \
		--disable depguard \
		--disable execinquery \
		--disable exhaustruct \
		--disable funlen \
		--disable gocognit \
		--disable godox \
		--disable gomnd \
		--disable gosec \
		--disable lll \
		--disable mnd \
		--disable stylecheck \
		--enable-all \
		./...

proto:
	protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_out=. \
		--go-grpc_opt=paths=source_relative \
		internal/proto/job_worker_service.proto

test-integration: build
	go test \
		-race \
		-shuffle on \
		-test.v \
		./test/integration/...

test-unit:
	go test \
		-race \
		-shuffle on \
		$(shell go list ./... | grep --invert integration)

.PHONY: all build certs lint proto test-integration test-unit
