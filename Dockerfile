# 1. The Svelte interface
FROM node:22-alpine AS front
WORKDIR /app
COPY web/package.json web/package-lock.json* ./
RUN npm install
COPY web/ ./
RUN npm run build

# 2. The Go binary, with the interface embedded in it
FROM golang:1.26-alpine AS back
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=front /app/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/gordi ./cmd/gordi

# 3. Final image
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata su-exec
COPY --from=back /out/gordi /usr/local/bin/gordi
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
VOLUME ["/input", "/output", "/config"]
EXPOSE 7373
ENTRYPOINT ["docker-entrypoint.sh"]
