.PHONY: build build-lsp test test-update install clean fmt fmt-glsp examples examples-tour examples-api

BIN     := glisp
BIN_LSP := glisp-lsp

build:
	go build -o $(BIN) ./cmd/glisp

build-lsp:
	go build -o $(BIN_LSP) ./cmd/glisp-lsp

test:
	go test ./...

test-update:
	go test ./internal/transpiler/... -update

install:
	go install ./cmd/glisp
	go install ./cmd/glisp-lsp

clean:
	rm -f $(BIN) $(BIN_LSP)
	rm -f examples/tour/tour examples/api/api examples/shapes/shapes
	find . -name "*.go.out" -delete

fmt:
	gofmt -w .

fmt-glsp: build
	find examples -name '*.glsp' | xargs ./$(BIN) fmt

examples: examples-tour examples-api examples-shapes

examples-tour: build
	./$(BIN) build -o examples/tour/tour examples/tour/main.glsp

examples-api: build
	./$(BIN) build -o examples/api/api examples/api/main.glsp

examples-shapes: build
	./$(BIN) build -o examples/shapes/shapes examples/shapes/main.glsp
