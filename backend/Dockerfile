# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Release provenance (set by CI; local builds keep the defaults -> "dev").
# RELEASE_JSON_B64 is base64 so multi-line notes survive as one build arg; it
# replaces the `{}` placeholder that internal/buildinfo embeds via go:embed.
# Declared after `go mod download` so the module cache layer stays warm.
ARG VERSION=
ARG GIT_SHA=
ARG BUILD_TIME=
ARG RELEASE_JSON_B64=

RUN if [ -n "${RELEASE_JSON_B64}" ]; then \
      printf '%s' "${RELEASE_JSON_B64}" | base64 -d > internal/buildinfo/release.json; \
    fi && \
    BI=github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/buildinfo && \
    LDFLAGS="-s -w -X ${BI}.Version=${VERSION} -X ${BI}.GitSHA=${GIT_SHA} -X ${BI}.BuildTime=${BUILD_TIME}" && \
    for cmd in server worker release-record create-super-admin; do \
      CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
      go build -trimpath -ldflags="${LDFLAGS}" -o "/out/${cmd}" "./cmd/${cmd}" || exit 1; \
    done

FROM migrate/migrate:v4.19.1 AS migrate

FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 10001 appuser

WORKDIR /app

COPY --from=migrate /usr/local/bin/migrate /usr/local/bin/migrate
COPY --from=builder /out/server /app/server
COPY --from=builder /out/worker /app/worker
COPY --from=builder /out/release-record /app/release-record
COPY --from=builder /out/create-super-admin /app/create-super-admin
COPY migrations /app/migrations
COPY deploy/docker/entrypoint.sh /app/entrypoint.sh

RUN chmod +x /app/entrypoint.sh \
    && chown -R appuser:appuser /app

USER appuser

EXPOSE 8080

ENV APP_HTTP_ADDR=:8080
ENV APP_ROLE=server
ENV AUTO_MIGRATE=true

ENTRYPOINT ["/app/entrypoint.sh"]
