FROM golang:1.23-alpine AS builder
WORKDIR /build
# Copy dependency files first (better layer caching)
COPY go.mod go.sum ./
COPY vendor/ vendor/
# Copy source files
COPY *.go ./
COPY decoder/ decoder/
COPY main/ main/
COPY version ./
# Build the binary
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=v$(cat version)" -o ftdc_json main/ftdc_json.go

FROM alpine
LABEL maintainer="Ken Chen <ken.chen@simagix.com>"
RUN addgroup -S simagix && adduser -S simagix -G simagix
USER simagix
WORKDIR /home/simagix
COPY --from=builder /build/ftdc_json /ftdc_json
CMD ["/ftdc_json", "--latest", "3", "/diagnostic.data/"]