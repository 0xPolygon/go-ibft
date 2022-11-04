.PHONY: lint install-deps

FIRST_COMMIT ?= $(shell git rev-list --max-parents=0 HEAD)
lint:
	./build/bin/golangci-lint run --config ./.golangci.yml

lint-all:
	./build/bin/golangci-lint run --config ./.golangci.yml --new-from-rev=$(FIRST_COMMIT)

install-deps:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./build/bin v1.50.1
