.PHONY: lint lint-all build-dummy install-deps mut

FIRST_COMMIT ?= $(shell git rev-list --max-parents=0 HEAD)

lint:
	./build/bin/golangci-lint run --config ./.golangci.yml

builds-dummy:
	cd build && go build -o ./ibft1
	cd build && go build -o ./ibft2

lint-all:
	./build/bin/golangci-lint run --config ./.golangci.yml --new-from-rev=$(FIRST_COMMIT)

install-deps:
	go get github.com/JekaMas/go-mutesting/cmd/go-mutesting@v1.1.1
	go install github.com/JekaMas/go-mutesting/...
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./build/bin v1.50.1

mut:
	MUTATION_TEST=on go-mutesting --blacklist=".github/mut_blacklist" --config=".github/mut_config.yml" ./...
	@echo MSI: `jq '.stats.msi' report.json`
