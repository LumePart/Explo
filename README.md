# Explo - Music Discovery for Self-Hosted Music Systems

**Explo** bridges the gap between music discovery and self-hosted music systems. Its main function is to act as a self-hosted alternative to Spotify’s *Discover Weekly*, automating music discovery based on your listening history.

Explo uses the [ListenBrainz](https://listenbrainz.org/) recommendation engine to retrieve personalized tracks and downloads them directly into your music library.

---

## Features

- Fetch personalized playlists from ListenBrainz (controlled by flags):
  - Weekly Exploration
  - Weekly Jams
  - Daily Jams
- Download tracks from YouTube, Soulseek, or both
- Add metadata (title, artist, album) to YouTube downloads
- Create playlists in your music system
- Keep previous playlists for later listening
---

## Documentation

See the [Wiki Home](https://github.com/LumePart/Explo/wiki) for an overview of supported systems and next steps.

Or jump directly to:

- [Getting Started](https://github.com/LumePart/Explo/wiki/2.-Getting-Started) – Installation and setup guide  
- [Configuration Parameters](https://github.com/LumePart/Explo/wiki/3.-Configuration-Parameters) – Environment variable and flag reference  
- [System Notes](https://github.com/LumePart/Explo/wiki/4.-System-Notes) – Known issues and system-specific tips  
- [FAQ](https://github.com/LumePart/Explo/wiki/6.-FAQ) – Common questions

## Acknowledgements

Explo uses the following 3rd-party libraries:

- [ffmpeg-go](https://github.com/u2takey/ffmpeg-go): Go wrapper for FFmpeg

- [goutubedl](https://github.com/wader/goutubedl): Go wrapper for yt-dlp

- [godotenv](https://github.com/joho/godotenv): Load configuration from `.env` files

- [ytmusicapi](https://github.com/sigma67/ytmusicapi): Unofficial Youtube Music API

## Contributing

Contributions are always welcome! If you have any suggestions, bug reports, or feature requests, please open an issue or submit a pull request.
