.PHONY: build test test-update install clean fmt examples examples-tour examples-api

BIN := glisp

build:
	go build -o $(BIN) ./cmd/glisp

test:
	go test ./...

test-update:
	go test ./internal/transpiler/... -update

install:
	go install ./cmd/glisp

clean:
	rm -f $(BIN)
	rm -f examples/tour/tour examples/api/api
	find . -name "*.go.out" -delete

fmt:
	gofmt -w .

examples: examples-tour examples-api

examples-tour: build
	./$(BIN) build -o examples/tour/tour examples/tour/main.glsp

examples-api: build
	./$(BIN) build -o examples/api/api examples/api/main.glsp
