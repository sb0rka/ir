FROM golang:1.27.3-alpine AS build

ARG BINARY=ir-api

WORKDIR /src

COPY packages/common/go.mod ./packages/common/
COPY packages/contract/go.mod packages/contract/go.sum* ./packages/contract/
COPY apps/investigations/go.mod apps/investigations/go.sum* ./apps/investigations/

WORKDIR /src/apps/investigations
RUN GOWORK=off go mod download

WORKDIR /src
COPY packages/common ./packages/common
COPY packages/contract ./packages/contract

COPY apps/investigations/cmd ./apps/investigations/cmd
COPY apps/investigations/internal ./apps/investigations/internal

WORKDIR /src/apps/investigations
RUN CGO_ENABLED=0 GOOS=linux GOWORK=off go build -trimpath -ldflags="-s -w" \
    -o /out/service ./cmd/${BINARY}

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata wget && adduser -D -u 10001 app
USER app

COPY --from=build /out/service /app/service

ENTRYPOINT ["/app/service"]
