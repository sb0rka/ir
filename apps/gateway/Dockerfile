## Compose supplies packages/core via additional_contexts.
## Build: docker build -f apps/gateway/Dockerfile --build-context core=../sb0rka/packages/core .
FROM golang:1.25-alpine AS build

WORKDIR /src

COPY --from=core / ./community-core
COPY packages/common ./packages/common
COPY apps/gateway/go.mod apps/gateway/go.sum* ./apps/gateway/

WORKDIR /src/apps/gateway
RUN go mod edit -replace=github.com/sb0rka/sb0rka/packages/core=/src/community-core \
 && GOWORK=off go mod download

# Copy sources without overwriting the edited go.mod (replace would be lost).
COPY apps/gateway/api ./api
COPY apps/gateway/cmd ./cmd
COPY apps/gateway/internal ./internal
COPY apps/gateway/tests ./tests

RUN CGO_ENABLED=0 GOOS=linux GOWORK=off go build -trimpath -ldflags="-s -w" \
    -o /out/gateway ./cmd/gateway

FROM alpine:3.21

RUN apk add --no-cache ca-certificates wget && adduser -D -u 10001 app
USER app

COPY --from=build /out/gateway /app/gateway

ENTRYPOINT ["/app/gateway"]
