FROM --platform=$BUILDPLATFORM node:20-alpine AS ui-builder
ARG VERSION=dev
WORKDIR /app/src/web/frontend
COPY src/web/frontend/package*.json ./
RUN npm ci
COPY src/web/frontend/ ./
RUN VITE_VERSION=${VERSION} npm run build

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

# Set the working directory
WORKDIR /app

# Copy the Go source code into the container
COPY ./ .

# Copy the built React frontend into the embed path
COPY --from=ui-builder /app/src/web/dist ./src/web/dist

# Build the Go binary based on the target architecture
ARG TARGETARCH
ARG VERSION=dev
RUN GOOS=linux GOARCH=$TARGETARCH go build -ldflags "-X explo/src/config.Version=${VERSION}" -o explo ./src/main/

FROM python:3.12-alpine

# Install runtime deps: libc compat, ffmpeg, yt-dlp, tzdata, shadow for user management, su-exec for user switching
RUN apk add --no-cache \
    libc6-compat \
    ffmpeg \
    yt-dlp \
    tzdata \
    shadow \
    su-exec

# SpotiFLAC python module version to install (FLAC download source). Pulled from the
# project's matching GitHub version-branch archive, since 1.2.1 (which restored the
# upstream endpoint registry) is not on PyPI yet. Override at build time, e.g.:
#   --build-arg SPOTIFLAC_VERSION=1.2.2
ARG SPOTIFLAC_VERSION=1.2.1

# Install ytmusicapi (youtube fallback search) and SpotiFLAC (FLAC download source)
RUN pip install --no-cache-dir ytmusicapi \
    "https://github.com/ShuShuzinhuu/SpotiFLAC-Module-Version/archive/${SPOTIFLAC_VERSION}.tar.gz"

# Set working directory
WORKDIR /opt/explo/

# Copy entrypoint, binary, python helpers
COPY ./docker/start.sh /start.sh
COPY --from=builder /app/explo .
COPY src/downloader/youtube_music/search_ytmusic.py .
COPY src/downloader/spotiflac/spotiflac_dl.py .


RUN chmod +x /start.sh ./explo


ENV WEB_ADDR=":7288"

EXPOSE 7288

CMD ["/start.sh"]
