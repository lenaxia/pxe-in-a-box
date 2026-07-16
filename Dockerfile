# Multi-stage build: compile Go binaries, then create minimal runtime image

# ── Build stage ──────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -ldflags='-s -w' -o /pxe-gen ./cmd/pxe-gen && \
    CGO_ENABLED=0 go build -ldflags='-s -w' -o /pxe-in-a-box ./cmd/pxe-in-a-box

# ── Runtime stage ────────────────────────────────────────────────────
FROM alpine:3.20

# Install dnsmasq and utilities
RUN apk add --no-cache \
    dnsmasq \
    ca-certificates \
    wget \
    && rm -rf /var/cache/apk/*

# Copy compiled binaries from build stage
COPY --from=builder /pxe-gen /usr/local/bin/pxe-gen
COPY --from=builder /pxe-in-a-box /usr/local/bin/pxe-in-a-box

# Install matchbox binary (download pre-built from Poseidon)
ARG MATCHBOX_VERSION=v0.11.0
RUN wget -qO /usr/local/bin/matchbox \
    "https://github.com/poseidon/matchbox/releases/download/${MATCHBOX_VERSION}/matchbox-${MATCHBOX_VERSION}-linux-amd64" \
    && chmod +x /usr/local/bin/matchbox

# Copy iPXE chainload binaries (x86_64, baked into image)
COPY tftpboot/ /tftpboot/

# Volumes
VOLUME ["/config", "/assets"]

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q -O- http://localhost:8081/ || exit 1

# Entrypoint
ENTRYPOINT ["pxe-in-a-box"]
CMD ["--config-dir", "/config", "--assets-dir", "/assets"]
