# Контекст сборки — корень монорепы: нужен packages/contract по replace-директиве.
FROM golang:1.25-alpine AS build

ARG BINARY=ir-api

WORKDIR /src

COPY go.work ./
COPY packages/contract/go.mod packages/contract/go.sum* ./packages/contract/
COPY apps/investigations/go.mod apps/investigations/go.sum* ./apps/investigations/
RUN go mod download all || true

COPY packages ./packages
COPY apps ./apps

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/service ./apps/investigations/cmd/${BINARY}

FROM alpine:3.21

RUN apk add --no-cache ca-certificates wget && adduser -D -u 10001 app
USER app

COPY --from=build /out/service /app/service

ENTRYPOINT ["/app/service"]
