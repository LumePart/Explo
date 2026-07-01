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

# Always install the LATEST SpotiFLAC (the FLAC download source — provides both the `spotiflac`
# CLI and the importable module used by spotiflac_dl.py). SpotiFLAC's reverse-engineered Deezer/
# Qobuz/etc. backends break often and are fixed upstream, so pinning goes stale fast. The ADD of
# PyPI's release metadata busts THIS layer's build cache whenever a new SpotiFLAC version ships, so
# a rebuild (`app.redeploy music`) re-resolves to the newest release; unchanged → fast cache hit.
# ytmusicapi = youtube fallback search. To pin instead, use `SpotiFLAC==<ver>` below.
ADD https://pypi.org/pypi/SpotiFLAC/json /tmp/spotiflac-pypi.json
RUN pip install --no-cache-dir --upgrade ytmusicapi SpotiFLAC && rm -f /tmp/spotiflac-pypi.json

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
