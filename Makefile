.PHONY: all build build-lsp test test-update install clean fmt fmt-glsp examples examples-clean

BIN     := glisp
BIN_LSP := glisp-lsp

# Add build as the first prerequisite of examples (defined in Makefile.examples).
examples: build

include Makefile.examples

all: build build-lsp examples

build:
	go build -o $(BIN) ./cmd/glisp

build-lsp:
	go build -o $(BIN_LSP) ./cmd/glisp-lsp

test:
	go test $(shell go list ./... | grep -v golisp/examples)

test-update:
	go test ./internal/transpiler/... -update

install:
	go install ./cmd/glisp
	go install ./cmd/glisp-lsp

clean: examples-clean
	rm -f $(BIN) $(BIN_LSP)
	find . -name "*.go.out" -delete

fmt:
	gofmt -w .

fmt-glsp: build
	find examples -name '*.glsp' | xargs ./$(BIN) fmt
