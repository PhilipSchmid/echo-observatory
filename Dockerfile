# Build stage
FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:fa3d596898d9f126af7c01d14ee7597bab3f39cccf392ef36b69fe77cd681f18 AS builder

# Define ARGs to specify the target platform and build metadata
ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH
ARG BUILD_DATE
ARG VCS_REF
ARG VERSION=dev

# Set the working directory inside the container
WORKDIR /app

RUN apk add --no-cache ca-certificates

# Copy the module checksums before downloading dependencies
COPY go.mod go.sum ./
# Download dependencies
RUN go mod download

# Copy the source code into the container
COPY . .

# Compile the application to a binary with all dependencies included
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w -X github.com/PhilipSchmid/echo-observatory/v2/internal/ui.version=${VERSION}" -a -o /echo-observatory ./cmd/echo-observatory

# Non-root runtime image.
FROM scratch AS final

# Re-declare ARGs in the final stage to use them
ARG BUILD_DATE
ARG VCS_REF

# Add metadata to the image using opencontainers labels
LABEL org.opencontainers.image.title="Echo Observatory" \
      org.opencontainers.image.description="Multi-protocol network diagnostics for HTTP, TLS, WebSocket, TCP, gRPC, and HTTP/3 connections." \
      org.opencontainers.image.url="https://github.com/philipschmid/echo-observatory" \
      org.opencontainers.image.source="https://github.com/philipschmid/echo-observatory" \
      org.opencontainers.image.vendor="philipschmid" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.revision="$VCS_REF" \
      org.opencontainers.image.created="$BUILD_DATE"

# Copy the compiled binary from the builder stage
COPY --from=builder /echo-observatory /echo-observatory
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

USER 65532:65532

# Set the binary as the entrypoint of the container
ENTRYPOINT ["/echo-observatory"]

# Internal listeners; deployment mappings expose HTTP 80 and HTTPS/HTTP3 443.
EXPOSE 8080/tcp 8443/tcp 4433/udp 9090/tcp 50051/tcp 3000/tcp
