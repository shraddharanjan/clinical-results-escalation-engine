# syntax=docker/dockerfile:1

FROM golang:1.26.5-alpine AS builder

WORKDIR /app

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./

RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations

ARG SERVICE=api

RUN case "$SERVICE" in \
      api|worker|scheduler) ;; \
      *) echo "Unsupported SERVICE: $SERVICE" && exit 1 ;; \
    esac \
    && CGO_ENABLED=0 \
       GOOS=linux \
       go build \
         -trimpath \
         -ldflags="-s -w" \
         -o /out/service \
         "./cmd/${SERVICE}"

FROM alpine:3.21

RUN apk add --no-cache ca-certificates \
    && addgroup -S appgroup \
    && adduser -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /out/service /app/service
COPY --from=builder /app/migrations /app/migrations

USER appuser

ENTRYPOINT ["/app/service"]