# Middleware — control-plane image.
# Copyright 2026 The Middleware Authors.

FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/middleware ./cmd/middleware \
 && CGO_ENABLED=0 go build -o /out/migrate ./cmd/migrate

FROM alpine:3.21

RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/middleware /app/middleware
COPY --from=build /out/migrate /app/migrate
COPY config.example.yaml /app/config.example.yaml
COPY deploy/middleware-entrypoint.sh /app/entrypoint.sh
RUN chmod 0755 /app/entrypoint.sh

EXPOSE 8080 9090
ENTRYPOINT ["/app/entrypoint.sh"]
