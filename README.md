# Explo - Discover Weekly for Subsonic compatible systems

**Explo** is a program for Subsonic-API compatible software, that aims to offer an alternative to Spotify's "Discover Weekly". It automates music discovery by downloading recommended songs based on your listening history. Using [ListenBrainz](https://listenbrainz.org/) as a discovery source and Youtube for downloading tracks.

Explo has 2 discovery modes, the preferred (and default) one gets songs from a playlist made by ListenBrainz, second one gets them through ListenBrainz API (weekly recommendations are quite poor). they are toggeable via the .env file

## Features

- Automatically gets and downloads music recommendations.
- Adds metadata (title, artist, album) to the downloaded tracks
- Creates "Discover Weekly" playlist with downloaded songs.
- By default keeps past Discover Weekly playlists
- Compatible with Subsonic-API systems.

## Getting Started

### Prerequisites

- Subsonic-API compatible system (e.g., Navidrome, Airsonic).
- ffmpeg installed on server
- [YouTube Data API](https://developers.google.com/youtube/v3/getting-started) key.
- [Scrobbling to ListenBrainz](https://listenbrainz.org/add-data/) setup

### Installation

1. Download the [latest release](https://github.com/LumePart/Explo/releases/latest) (make sure it can be executed)
2. Make an "local.env" file in the same directory and fill it ([refer to sample.env](https://github.com/LumePart/Explo/blob/main/sample.env) for options)
3. Add a Cron job that executes Explo weekly
```bash
crontab -e
```
Insert this to the last line to execute Explo every tuesday at 00:15 (ListenBrainz updates its discovery db at monday)
```bash
15 0 * * 2 cd /path/to/explo && ./explo-amd64-linux
```
**PS!** If using playlist discovery, don't run the program more than once per day (eats up youtube API credits). For testing, change LISTENBRAINZ_DISCOVERY variable to a random value

## Contributing

Contributions are always welcome! If you have any suggestions, bug reports, or feature requests, please open an issue or submit a pull request.
