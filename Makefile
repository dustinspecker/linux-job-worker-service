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
		--disable godox \
		--disable gomnd \
		--disable gosec \
		--disable lll \
		--disable mnd \
		--enable-all \
		./...

test-unit:
	go test \
		-race \
		-shuffle on \
		./...

.PHONY: all build lint test-unit
