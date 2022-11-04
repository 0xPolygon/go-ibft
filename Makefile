.PHONY: lint install-deps

lint:
	./build/bin/golangci-lint run --config ./.golangci.yml

install-deps:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./build/bin v1.50.1
