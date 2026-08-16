# ---- frontend build ----
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci || npm install
COPY web/ ./
RUN npm run build

# ---- backend build ----
FROM golang:1.26-alpine AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist /src/web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/pleumcloud ./cmd/web

# ---- runtime ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 1000 pleum     && mkdir -p /data && chown pleum:pleum /data
USER pleum
VOLUME /data
ENV PLEUMCLOUD_DATA=/data     PLEUMCLOUD_BIND=0.0.0.0
EXPOSE 7777
COPY --from=go /out/pleumcloud /usr/local/bin/pleumcloud
ENTRYPOINT ["pleumcloud"]
