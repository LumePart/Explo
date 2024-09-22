Explo - Discover Weekly for selfhosted music systems

Explo is a program for that aims to offer an alternative to Spotify's "Discover Weekly". It automates music discovery by downloading recommended songs based on your listening history. Using ListenBrainz as a discovery source and Youtube for downloading tracks.

Explo has 2 discovery modes, the preferred (and default) one gets songs from a playlist made by ListenBrainz, second one gets them through ListenBrainz API (weekly recommendations are quite poor). they are toggeable via the .env file
Features

    Compatible with MPD and Subsonic-API systems.
    Automatically gets and downloads music recommendations.
    Adds metadata (title, artist, album) to the downloaded tracks
    Creates "Discover Weekly" playlist with downloaded songs.
    By default keeps past Discover Weekly playlists

Getting Started
Prerequisites

    MPD (Music Player Daemon) or a Subsonic-API compatible system (e.g., Navidrome, Airsonic).
    ffmpeg installed on server
    YouTube Data API key.
    Scrobbling to ListenBrainz setup

Installation

    Download the latest release (make sure it can be executed)
    Make an "local.env" file in the same directory and fill it (refer to sample.env for options)
    Add a Cron job that executes Explo weekly

crontab -e

Insert this to the last line to execute Explo every tuesday at 00:15 (ListenBrainz updates its discovery db at monday)

15 0 * * 2 cd /path/to/explo && ./explo-amd64-linux

PS! If using playlist discovery, don't run the program more than once per day (eats up youtube API credits). For testing, change LISTENBRAINZ_DISCOVERY variable to a random value

Note for MPD users: Comment out subsonic variables in local.env, and make sure you define PLAYLIST_DIR
Contributing

Contributions are always welcome! If you have any suggestions, bug reports, or feature requests, please open an issue or submit a pull request.
