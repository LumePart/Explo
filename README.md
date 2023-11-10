# Explo - Discover Weekly for Subsonic compatible systems

**Explo** is a simple program for Subsonic-API compatible software, that aims to offer an alternative to Spotify's "Discover Weekly". It automates music discovery by downloading recommended songs based on your listening history. Using [ListenBrainz](https://listenbrainz.org/) as a discovery source and Youtube for downloading tracks.

## Features

- Automatically gets and downloads music recommendations.
- Adds metadata (title, artist, album) to the downloaded tracks
- Creates "Discover Weekly" playlist with downloaded songs.
- Compatible with Subsonic-API systems.

## Getting Started

### Prerequisites

- Subsonic-API compatible system (e.g., Navidrome, Airsonic).
- ffmpeg installed on server
- [YouTube Data API](https://developers.google.com/youtube/v3/getting-started) key.
- [Scrobbling to ListenBrainz](https://listenbrainz.org/add-data/) setup

### Installation

1. Download the [latest release](https://github.com/LumePart/Explo/releases/latest)
2. Make an "local.env" file in the same directory and fill it ([refer to sample.env](https://github.com/LumePart/Explo/blob/main/sample.env) for options)
3. Add a Cron job that executes Explo weekly
```bash
crontab -e
```
Insert this to the last line to execute Explo every monday
```bash
0 0 * * 1 cd /path/to/explo && ./explo-amd64-linux
```

## Contributing

Contributions are always welcome! If you have any suggestions, bug reports, or feature requests, please open an issue or submit a pull request.
