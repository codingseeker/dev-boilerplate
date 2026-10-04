# syntax=docker/dockerfile:1

# Build stage: compile a static, stripped binary.
FROM golang:1.25-alpine AS builder

ARG VERSION=dev
ARG COMMIT=none
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Dependencies are resolved from go.mod first so the layer is cached.
COPY go.mod go.sum* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath \
      -ldflags "-s -w \
        -X github.com/codingseeker/go-starter/internal/buildinfo.Version=${VERSION} \
        -X github.com/codingseeker/go-starter/internal/buildinfo.Commit=${COMMIT}" \
      -o /out/go-starter ./cmd/app

# Runtime stage: minimal image, non-root user, no shell tooling required.
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -g 10001 -S app \
    && adduser -u 10001 -S -G app -H -s /sbin/nologin app

WORKDIR /app

COPY --from=builder /out/go-starter /app/go-starter

USER 10001:10001

ENV APP_ENV=production \
    HTTP_HOST=0.0.0.0 \
    HTTP_PORT=8080

EXPOSE 8080

ENTRYPOINT ["/app/go-starter"]
CMD ["serve"]
