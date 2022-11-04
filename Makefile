.PHONY: lint mut
lint:
	golangci-lint run --config ./.golangci.yml

install-deps:
	go get github.com/JekaMas/go-mutesting/cmd/go-mutesting@v1.1.1
	go install github.com/JekaMas/go-mutesting/...
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./build/bin v1.50.0

mut:
	MUTATION_TEST=on go-mutesting --blacklist=".github/mut_blacklist" --config=".github/mut_config.yml" ./...
	@echo MSI: `jq '.stats.msi' report.json`
