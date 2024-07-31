.DEFAULT_GOAL := all

all: test-unit build

build:
	go build -o bin/ ./...

test-unit:
	go test \
		-race \
		-shuffle on \
		./...

.PHONY: all build test-unit
