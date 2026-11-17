# Levier — control-plane image.
# Copyright 2026 The Levier Authors.

FROM golang:1.26-alpine AS build

# Allow builds behind a module proxy mirror.
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-X github.com/fluffyTighnari/levier/internal/version.Version=$VERSION -X github.com/fluffyTighnari/levier/internal/version.Commit=$COMMIT -X github.com/fluffyTighnari/levier/internal/version.Built=$BUILD_TIME" \
      -o /out/levier ./cmd/levier \
 && CGO_ENABLED=0 go build -trimpath -o /out/migrate ./cmd/migrate

FROM alpine:3.21

RUN apk add --no-cache ca-certificates
WORKDIR /app
ENV PATH="/app:${PATH}"
COPY --from=build /out/levier /app/levier
COPY --from=build /out/migrate /app/migrate
COPY levier.example.yaml /app/levier.example.yaml
COPY deploy/levier-entrypoint.sh /app/entrypoint.sh
RUN chmod 0755 /app/entrypoint.sh

EXPOSE 8080 9090
ENTRYPOINT ["/app/entrypoint.sh"]
