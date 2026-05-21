FROM node:20-alpine AS web-builder
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.25-alpine AS server-builder
WORKDIR /src/server
RUN apk add --no-cache ca-certificates git
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/hostdeck ./cmd/hostdeck

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata wget \
  && addgroup -S -g 10001 hostdeck \
  && adduser -S -D -H -u 10001 -G hostdeck hostdeck \
  && mkdir -p /app/logs \
  && chown -R hostdeck:hostdeck /app
WORKDIR /app
COPY --from=server-builder /out/hostdeck /usr/local/bin/hostdeck
COPY --from=web-builder /src/web/dist /app/web/dist
COPY server/config/config.example.yaml /app/config.example.yaml

EXPOSE 18080
USER hostdeck
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD wget -qO- http://127.0.0.1:18080/api/healthz >/dev/null || exit 1

ENTRYPOINT ["hostdeck"]
CMD ["--config", "/app/config.yaml"]
