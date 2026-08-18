FROM golang:1.25-alpine AS build

ARG BINARY=ir-api

WORKDIR /src

COPY packages/contract/go.mod packages/contract/go.sum* ./packages/contract/
COPY apps/investigations/go.mod apps/investigations/go.sum* ./apps/investigations/
WORKDIR /src/apps/investigations
RUN GOWORK=off go mod download

WORKDIR /src
COPY packages ./packages
COPY apps/investigations ./apps/investigations

WORKDIR /src/apps/investigations
RUN CGO_ENABLED=0 GOOS=linux GOWORK=off go build -trimpath -ldflags="-s -w" \
    -o /out/service ./cmd/${BINARY}

FROM alpine:3.21

RUN apk add --no-cache ca-certificates wget && adduser -D -u 10001 app
USER app

COPY --from=build /out/service /app/service

ENTRYPOINT ["/app/service"]
