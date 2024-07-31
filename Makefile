.DEFAULT_GOAL := all

all: test-unit

test-unit:
	go test \
		-race \
		-shuffle on \
		./...

.PHONY: all test-unit
