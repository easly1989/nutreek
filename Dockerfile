# syntax=docker/dockerfile:1.7

# --- Stage 1: Tailwind CSS bundle --------------------------------------------
# Pinned a v3.4.17 (vedi Makefile per la stessa ragione: v4 ha cambiato
# CLI/config, l'input.css/tailwind.config.js sono v3 syntax).
FROM alpine:3.20 AS tailwind-builder
RUN apk add --no-cache curl ca-certificates
WORKDIR /src
ARG TARGETARCH
ARG TAILWIND_VERSION=v3.4.17
RUN case "$TARGETARCH" in \
      amd64) TW_ARCH=x64 ;; \
      arm64) TW_ARCH=arm64 ;; \
      *) echo "unsupported arch: $TARGETARCH"; exit 1 ;; \
    esac && \
    curl -sSL -o /usr/local/bin/tailwindcss \
      "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/tailwindcss-linux-${TW_ARCH}" && \
    chmod +x /usr/local/bin/tailwindcss
COPY web/tailwind ./web/tailwind
COPY internal/web/templates ./internal/web/templates
COPY internal/app ./internal/app
RUN mkdir -p internal/web/static/css && \
    tailwindcss \
      -c ./web/tailwind/tailwind.config.js \
      -i ./web/tailwind/input.css \
      -o ./internal/web/static/css/tailwind.css \
      --minify

# --- Stage 2: Go build -------------------------------------------------------
FROM golang:1.25-alpine AS go-builder
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Override del CSS dal tailwind-builder: la sorgente locale eventualmente
# in context e' gia' stata esclusa da .dockerignore, quindi questa COPY
# e' l'unica fonte autoritativa di tailwind.css per l'embed Go.
COPY --from=tailwind-builder /src/internal/web/static/css/tailwind.css ./internal/web/static/css/tailwind.css
ARG VERSION=dev
RUN go build -ldflags="-s -w -X main.version=${VERSION}" -o /out/nutreek ./cmd/nutreek

# --- Stage 3: runtime --------------------------------------------------------
FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=go-builder /out/nutreek /app/nutreek
ENV NUTREEK_ADDR=:8080 \
    NUTREEK_DB_PATH=/data/nutreek.db \
    NUTREEK_ENV=prod
EXPOSE 8080
USER nonroot:nonroot
VOLUME ["/data"]
ENTRYPOINT ["/app/nutreek"]
