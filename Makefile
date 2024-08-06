.DEFAULT_GOAL := all

all: lint test-unit build

build:
	go build -o bin/ ./...

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

.PHONY: all build lint test-integration test-unit
