FROM golang:1.25-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /glisp ./cmd/glisp

FROM scratch
COPY --from=build /glisp /glisp
ENTRYPOINT ["/glisp"]
