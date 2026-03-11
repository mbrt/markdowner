FROM golang:1.25-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /markdowner .

FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        git \
        chromium \
        ca-certificates \
        fonts-liberation && \
    rm -rf /var/lib/apt/lists/*

RUN groupadd -r markdowner && useradd -r -g markdowner -s /sbin/nologin markdowner
RUN mkdir -p /data && chown markdowner:markdowner /data

COPY --from=build /markdowner /usr/local/bin/markdowner

WORKDIR /data
USER markdowner
ENTRYPOINT ["markdowner"]
