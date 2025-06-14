FROM golang:1.23-alpine AS builder

# Set the working directory
WORKDIR /app

# Copy the Go source code into the container
COPY ./ .

# Build the Go binary based on the target architecture
ARG TARGETARCH
RUN GOOS=linux GOARCH=$TARGETARCH go build -o explo ./src/main/

FROM alpine

RUN apk add --no-cache libc6-compat ffmpeg yt-dlp

WORKDIR /opt/explo/
COPY ./docker/start.sh /start.sh
COPY --from=builder /app/explo .
RUN chmod +x /start.sh ./explo

# Can be defined from compose as well 
ENV CRON_SCHEDULE="15 0 * * 2"

CMD ["/start.sh"]