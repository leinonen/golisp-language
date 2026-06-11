# Docker packaging

`glisp build` produces a statically-linked binary with no external dependencies, so it runs in a `scratch` image.

```dockerfile
# Build stage
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git && \
    go install github.com/leinonen/golisp-language/cmd/glisp@latest

WORKDIR /app
COPY . .

# Produces a statically-linked binary
RUN CGO_ENABLED=0 glisp build src/

# Runtime stage — zero OS overhead
FROM scratch
COPY --from=builder /app/src /app
ENTRYPOINT ["/app/src"]
```

Build and run:

```
docker build -t myapp .
docker run -p 3000:3000 myapp
```

The final image contains only your binary. Typical size: 8–15 MB.

## Multi-file projects

For a directory build (`glisp build dir/`) the output binary name matches the directory name:

```dockerfile
RUN CGO_ENABLED=0 glisp build api/
COPY --from=builder /app/api /app
```

## Health checks

`scratch` has no shell, so use the `HEALTHCHECK` exec form with your app's own endpoint:

```dockerfile
HEALTHCHECK --interval=10s --timeout=3s \
  CMD ["/app/src", "--healthz"]   # implement a /healthz flag in main
```

Or use a sidecar/external probe and skip the `HEALTHCHECK` entirely.
