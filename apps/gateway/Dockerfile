FROM golang:1.25-alpine AS build

WORKDIR /src/apps/gateway

COPY apps/gateway/go.mod apps/gateway/go.sum ./
RUN GOWORK=off go mod download

COPY apps/gateway ./

RUN GOWORK=off CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/gateway ./cmd/gateway

FROM alpine:3.21

RUN apk add --no-cache ca-certificates wget && adduser -D -u 10001 app
USER app

COPY --from=build /out/gateway /app/gateway

ENTRYPOINT ["/app/gateway"]
