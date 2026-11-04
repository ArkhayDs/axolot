# syntax=docker/dockerfile:1
# Builds the front-end, embeds it in a static Go binary, ships both in a minimal image.
FROM node:24-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /web/dist ./web/dist
RUN CGO_ENABLED=0 go build -ldflags '-s -w' -o /out/axolot ./cmd/axolot

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
COPY --from=build /out/axolot /usr/local/bin/axolot
USER 1000
ENTRYPOINT ["axolot"]
