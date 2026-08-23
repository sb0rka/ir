FROM golang:1.25-alpine AS build

WORKDIR /src

COPY packages/common ./packages/common
COPY apps/gateway/go.mod apps/gateway/go.sum* ./apps/gateway/
WORKDIR /src/apps/gateway
RUN GOWORK=off go mod download

WORKDIR /src
COPY apps/gateway ./apps/gateway

WORKDIR /src/apps/gateway
RUN CGO_ENABLED=0 GOOS=linux GOWORK=off go build -trimpath -ldflags="-s -w" \
    -o /out/gateway ./cmd/gateway

FROM alpine:3.21

RUN apk add --no-cache ca-certificates wget && adduser -D -u 10001 app
USER app

COPY --from=build /out/gateway /app/gateway

ENTRYPOINT ["/app/gateway"]
