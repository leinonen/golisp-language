.PHONY: all build build-lsp test test-update install clean fmt fmt-glsp examples examples-clean dist

BIN     := glisp
BIN_LSP := glisp-lsp

# Version stamped into the binaries. Defaults to a git-derived tag; the release
# workflow overrides it with the pushed tag.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X golisp/internal/version.Version=$(VERSION)

# Platforms built by `make dist`.
DIST_PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

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

# dist cross-compiles release archives for every DIST_PLATFORMS target into
# dist/, mirroring the GitHub release workflow so a release can be built locally.
dist:
	@rm -rf dist && mkdir -p dist
	@for p in $(DIST_PLATFORMS); do \
		goos=$${p%/*}; goarch=$${p#*/}; \
		name="glisp_$(VERSION)_$${goos}_$${goarch}"; stage="dist/$${name}"; \
		ext=""; [ "$${goos}" = windows ] && ext=".exe"; \
		echo "building $${name}"; \
		mkdir -p "$${stage}"; \
		GOOS=$${goos} GOARCH=$${goarch} CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o "$${stage}/$(BIN)$${ext}" ./cmd/glisp || exit 1; \
		GOOS=$${goos} GOARCH=$${goarch} CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o "$${stage}/$(BIN_LSP)$${ext}" ./cmd/glisp-lsp || exit 1; \
		cp README.md LICENSE "$${stage}/"; \
		if [ "$${goos}" = windows ]; then (cd dist && zip -qr "$${name}.zip" "$${name}"); else tar -C dist -czf "dist/$${name}.tar.gz" "$${name}"; fi; \
		rm -rf "$${stage}"; \
	done
	@cd dist && sha256sum *.tar.gz *.zip > SHA256SUMS 2>/dev/null || true
	@echo "dist artifacts:" && ls -1 dist

clean: examples-clean
	rm -f $(BIN) $(BIN_LSP)
	rm -rf dist
	find . -name "*.go.out" -delete

fmt:
	gofmt -w .

fmt-glsp: build
	find examples -name '*.glsp' | xargs ./$(BIN) fmt
