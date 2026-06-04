.PHONY: all build build-lsp test test-update install clean fmt fmt-glsp examples examples-clean examples-tour examples-api examples-shapes examples-multifile examples-httpclient examples-notes-api examples-concurrency examples-logparser

BIN     := glisp
BIN_LSP := glisp-lsp

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

clean:
	rm -f $(BIN) $(BIN_LSP)
	rm -f examples/tour/tour examples/api/api examples/shapes/shapes examples/multifile/multifile examples/httpclient/httpclient examples/data/data examples/notes-api/notes-api examples/concurrency/concurrency examples/logparser/logparser
	rm -f examples/multifile/glisp_runtime.go examples/multifile/main.go examples/multifile/helpers.go
	rm -f examples/notes-api/glisp_runtime.go examples/notes-api/main.go examples/notes-api/handlers.go examples/notes-api/db.go examples/notes-api/helpers.go
	find . -name "*.go.out" -delete

fmt:
	gofmt -w .

fmt-glsp: build
	find examples -name '*.glsp' | xargs ./$(BIN) fmt

examples-clean:
	rm -f examples/tour/tour examples/api/api examples/shapes/shapes examples/multifile/multifile examples/httpclient/httpclient examples/data/data examples/notes-api/notes-api examples/concurrency/concurrency examples/logparser/logparser
	rm -f examples/multifile/glisp_runtime.go examples/multifile/main.go examples/multifile/helpers.go
	rm -f examples/notes-api/glisp_runtime.go examples/notes-api/main.go examples/notes-api/handlers.go examples/notes-api/db.go examples/notes-api/helpers.go

examples: examples-clean examples-tour examples-api examples-shapes examples-multifile examples-httpclient examples-data examples-notes-api examples-concurrency examples-logparser

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

examples-data: build
	./$(BIN) build -o examples/data/data examples/data/main.glsp

examples-notes-api: build
	./$(BIN) build -o examples/notes-api/notes-api examples/notes-api/

examples-concurrency: build
	./$(BIN) build -o examples/concurrency/concurrency examples/concurrency/main.glsp

examples-logparser: build
	./$(BIN) build -o examples/logparser/logparser examples/logparser/main.glsp
