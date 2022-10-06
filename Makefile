.PHONY: lint mut
lint:
	golangci-lint run -E whitespace -E wsl -E wastedassign -E unconvert -E tparallel -E thelper -E stylecheck -E prealloc \
	-E predeclared -E nlreturn -E misspell -E makezero -E lll -E importas -E ifshort -E gosec -E  gofmt -E goconst \
	-E forcetypeassert -E dogsled -E dupl -E errname -E errorlint -E nolintlint --timeout 2m

install:
	go install github.com/avito-tech/go-mutesting/...

mut:
	go-mutesting --blacklist=".github/mut_blacklist" --config=".github/mut_config.yml" ./...
	@echo MSI: `jq '.stats.msi' report.json`