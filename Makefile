.PHONY: lint mut
lint:
	golangci-lint run -E whitespace -E wsl -E wastedassign -E unconvert -E tparallel -E thelper -E stylecheck -E prealloc \
	-E predeclared -E nlreturn -E misspell -E makezero -E lll -E importas -E ifshort -E gosec -E  gofmt -E goconst \
	-E forcetypeassert -E dogsled -E dupl -E errname -E errorlint -E nolintlint --timeout 2m

deps:
	go get github.com/JekaMas/go-mutesting/cmd/go-mutesting@v1.0.0
	go install github.com/JekaMas/go-mutesting/...
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./build/bin v1.50.0

mut:
	MUTATION_TEST=on go-mutesting --blacklist=".github/mut_blacklist" --config=".github/mut_config.yml" ./...
	@echo MSI: `jq '.stats.msi' report.json`
