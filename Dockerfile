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

# yt-dlp is used to extract YouTube captions. YouTube changes often, so the
# latest release is fetched at build time.
ARG TARGETARCH
RUN case "$TARGETARCH" in \
        amd64) asset=yt-dlp_linux ;; \
        arm64) asset=yt-dlp_linux_aarch64 ;; \
        *) echo "unsupported arch: $TARGETARCH" >&2; exit 1 ;; \
    esac && \
    apt-get update && \
    apt-get install -y --no-install-recommends curl && \
    curl -fsSL -o /usr/local/bin/yt-dlp \
        "https://github.com/yt-dlp/yt-dlp/releases/latest/download/$asset" && \
    chmod 755 /usr/local/bin/yt-dlp && \
    apt-get purge -y curl && apt-get autoremove -y && \
    rm -rf /var/lib/apt/lists/*

COPY --from=build /markdowner /usr/local/bin/markdowner

WORKDIR /data
USER markdowner
ENTRYPOINT ["markdowner"]
