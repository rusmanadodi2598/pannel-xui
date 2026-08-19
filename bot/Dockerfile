# syntax=docker/dockerfile:1

# ---- Build stage ----
FROM golang:1.26.5-alpine AS build
WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w -X main.version=$(git describe --tags --always 2>/dev/null || echo dev)" \
    -o /out/bot ./cmd/bot

# ---- Runtime stage ----
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/bot /app/bot
COPY seed/ /app/seed/

EXPOSE 8443
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8443/health || exit 1

ENTRYPOINT ["/app/bot"]
