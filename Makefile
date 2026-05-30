.PHONY: all build build-lsp test test-update install clean fmt fmt-glsp examples examples-clean examples-tour examples-api examples-shapes examples-multifile examples-httpclient

BIN     := glisp
BIN_LSP := glisp-lsp

all: build build-lsp examples

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
	rm -f examples/tour/tour examples/api/api examples/shapes/shapes examples/multifile/multifile examples/httpclient/httpclient
	rm -f examples/multifile/glisp_runtime.go examples/multifile/main.go examples/multifile/helpers.go
	find . -name "*.go.out" -delete

fmt:
	gofmt -w .

fmt-glsp: build
	find examples -name '*.glsp' | xargs ./$(BIN) fmt

examples-clean:
	rm -f examples/tour/tour examples/api/api examples/shapes/shapes examples/multifile/multifile examples/httpclient/httpclient
	rm -f examples/multifile/glisp_runtime.go examples/multifile/main.go examples/multifile/helpers.go

examples: examples-clean examples-tour examples-api examples-shapes examples-multifile examples-httpclient

examples-tour: build
	./$(BIN) build -o examples/tour/tour examples/tour/main.glsp

examples-api: build
	./$(BIN) build -o examples/api/api examples/api/main.glsp

examples-shapes: build
	./$(BIN) build -o examples/shapes/shapes examples/shapes/main.glsp

examples-multifile: build
	./$(BIN) build -o examples/multifile/multifile examples/multifile/

examples-httpclient: build
	./$(BIN) build -o examples/httpclient/httpclient examples/httpclient/main.glsp
