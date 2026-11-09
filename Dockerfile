FROM node:22-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

FROM golang:1.25-alpine AS backend
WORKDIR /app/backend
COPY backend/ ./
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/leadhelper ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && addgroup -S -g 10001 leadhelper && adduser -S -D -H -u 10001 -G leadhelper leadhelper && mkdir -p /app/data && chown leadhelper:leadhelper /app/data
WORKDIR /app
COPY --from=backend /out/leadhelper /app/leadhelper
COPY --from=frontend /app/frontend/dist /app/web
ENV HTTP_ADDR=:8080 DATA_FILE=/app/data/leadhelper.json STATIC_DIR=/app/web TZ=Europe/Moscow
USER leadhelper
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 CMD wget -q -O - http://127.0.0.1:8080/api/health || exit 1
ENTRYPOINT ["/app/leadhelper"]
