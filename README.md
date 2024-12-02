# Explo - Discover Weekly for Self-Hosted Music Systems

**Explo** is an alternative to Spotify's "Discover Weekly". It automates music discovery by downloading recommended tracks based on your listening history. Using [ListenBrainz](https://listenbrainz.org/) for recommendations and Youtube for downloading.

Explo offers two discovery modes:

1. Playlist Discovery (default): Retrieves songs from a ListenBrainz-generated playlist.
2. API Discovery: Uses the ListenBrainz API for recommendations (Note: API recommendations don't update often).

## Features

- Supports **Jellyfin**, **MPD** and **Subsonic-API-based systems**.
- Automatically fetches recommendations and downloads the tracks.
- Adds metadata (title, artist, album) to the downloaded files.
- Creates a "Discover Weekly" playlist with the latest songs.
- Keeps past playlists by default for easy access.

## Getting Started

### Prerequisites

- A self-hosted music system like Jellyfin, MPD, or any Subsonic-API compatible system (e.g., Navidrome, Airsonic).
- A [YouTube Data API](https://developers.google.com/youtube/v3/getting-started) key.
- [ListenBrainz scrobbling](https://listenbrainz.org/add-data/) set up

### Installation

#### Docker

1. Download [docker-compose.yaml](https://github.com/LumePart/Explo/blob/main/docker-compose.yaml) file to your system and configure volume mappings
2. Make a ``local.env`` file in the directory defined in docker-compose and configure it ([refer to sample.env](https://github.com/LumePart/Explo/blob/main/sample.env) for options)
3. Launch the container with `docker compose up -d`

#### Binary

Make sure ffmpeg and yt-dlp are installed on the system and accessible via $PATH. Alternatively, you can specify their paths in the local.env file.

1. Download the [latest release](https://github.com/LumePart/Explo/releases/latest) and ensure it's executable
2. Make a ``local.env`` file in the same directory as the binary and configure it ([refer to sample.env](https://github.com/LumePart/Explo/blob/main/sample.env) for options)
3. Add a Cron job to run Explo weekly:
```bash
crontab -e
```
Insert this to the last line to execute Explo every tuesday at 00:15 (ListenBrainz updates its discovery database on Mondays)
```bash
15 0 * * 2 cd /path/to/explo && ./explo-linux-amd64
```
**PS!** To test if everything is correct change ``LISTENBRAINZ_DISCOVERY`` to ``test`` and run the program manually

## Acknowledgements

Explo uses the following 3rd-party libraries:

- [ffmpeg-go](https://github.com/u2takey/ffmpeg-go): A Go wrapper for FFmpeg.

- [goutubedl](github.com/wader/goutubedl): A Go wrapper for yt-dlp.

- [godotenv](https://github.com/joho/godotenv): A library for loading configuration from .env files.

## Contributing

Contributions are always welcome! If you have any suggestions, bug reports, or feature requests, please open an issue or submit a pull request.
