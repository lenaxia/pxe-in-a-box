# syntax=docker/dockerfile:1

# ── Build stage: compile Go binaries ─────────────────────────────────
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETOS TARGETARCH

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags='-s -w' -o /pxe-gen ./cmd/pxe-gen && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags='-s -w' -o /pxe-in-a-box ./cmd/pxe-in-a-box

# ── Matchbox stage: download per-arch binary ─────────────────────────
FROM --platform=$BUILDPLATFORM alpine:3.20 AS matchbox

ARG TARGETARCH
ARG MATCHBOX_VERSION=v0.11.0

RUN apk add --no-cache curl tar && \
    arch="${TARGETARCH}" && \
    case "${TARGETARCH}" in \
      amd64) arch="amd64" ;; \
      arm64) arch="arm64" ;; \
      arm)   arch="arm" ;; \
    esac && \
    curl -sL \
      "https://github.com/poseidon/matchbox/releases/download/${MATCHBOX_VERSION}/matchbox-${MATCHBOX_VERSION}-linux-${arch}.tar.gz" \
      | tar xz --strip-components=1 -C /tmp && \
    cp /tmp/matchbox /matchbox && \
    chmod +x /matchbox

# ── iPXE stage: download chainload binaries ──────────────────────────
FROM --platform=$BUILDPLATFORM alpine:3.20 AS ipxe

RUN apk add --no-cache curl && \
    mkdir -p /tftpboot && \
    curl -sL -o /tftpboot/undionly.kpxe \
      "http://boot.ipxe.org/undionly.kpxe" && \
    curl -sL -o /tftpboot/ipxe.efi \
      "http://boot.ipxe.org/ipxe.efi"

# ── Runtime stage ────────────────────────────────────────────────────
FROM alpine:3.20

LABEL org.opencontainers.image.title="PXE-in-a-Box"
LABEL org.opencontainers.image.source="https://github.com/lenaxia/pxe-in-a-box"
LABEL org.opencontainers.image.licenses="MIT"

RUN apk add --no-cache \
    dnsmasq \
    ca-certificates \
    wget \
    && rm -rf /var/cache/apk/*

COPY --from=builder /pxe-gen /usr/local/bin/pxe-gen
COPY --from=builder /pxe-in-a-box /usr/local/bin/pxe-in-a-box
COPY --from=matchbox /matchbox /usr/local/bin/matchbox
COPY --from=ipxe /tftpboot/ /tftpboot/
COPY templates/ /default-templates/

VOLUME ["/config", "/assets"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q -O- http://localhost:8081/ || exit 1

ENTRYPOINT ["pxe-in-a-box"]
CMD ["--config-dir", "/config", "--assets-dir", "/assets"]
