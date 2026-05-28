.PHONY: build test test-update install clean fmt examples examples-hello examples-webserver

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
	rm -f examples/hello/hello examples/webserver/webserver
	find . -name "*.go.out" -delete

fmt:
	gofmt -w .

examples: examples-hello examples-webserver

examples-hello: build
	./$(BIN) build -o examples/hello/hello examples/hello/main.glsp

examples-webserver: build
	./$(BIN) build -o examples/webserver/webserver examples/webserver/main.glsp
