# Build stage
FROM --platform=$BUILDPLATFORM golang:1.25 AS builder
ARG TARGETOS TARGETARCH
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /init-spock ./cmd/init-spock

# Runtime stage
FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /init-spock /init-spock
USER 65532:65532
ENTRYPOINT ["/init-spock"]
