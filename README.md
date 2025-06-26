# Explo - Discover Weekly for Self-Hosted Music Systems

**Explo** bridges the gap between music discovery and self-hosted music systems. It serves as a self-hosted alternative to Spotify’s *Discover Weekly*, automating music discovery based on your listening history.

Explo uses the [ListenBrainz](https://listenbrainz.org/) recommendation engine to retrieve personalized tracks and downloads them directly into your music library.

---

## Features

- Fetches weekly music recommendations based on your listening history
- Downloads tracks using YouTube, Soulseek (or both!)
- Adds metadata (title, artist, album) to each file (youtube downloads)
- Creates a “Discover Weekly” playlist in your music system
- Keeps previous playlists for later listening

---

## Documentation

See the [Wiki Home](https://github.com/LumePart/Explo/wiki) for an overview of supported systems and next steps.

Or jump directly to:

- [Getting Started](https://github.com/LumePart/Explo/wiki/2.-Getting-Started) – Installation and setup guide  
- [Configuration Parameters](https://github.com/LumePart/Explo/wiki/3.-Configuration-Parameters) – Environment variable reference  
- [System Notes](https://github.com/LumePart/Explo/wiki/4.-System-Notes) – Known issues and system-specific tips  
- [FAQ](https://github.com/LumePart/Explo/wiki/6.-FAQ) – Common questions

## Acknowledgements

Explo uses the following 3rd-party libraries:

- [ffmpeg-go](https://github.com/u2takey/ffmpeg-go): A Go wrapper for FFmpeg

- [goutubedl](https://github.com/wader/goutubedl): A Go wrapper for yt-dlp

- [godotenv](https://github.com/joho/godotenv): A library for loading configuration from .env files

## Contributing

Contributions are always welcome! If you have any suggestions, bug reports, or feature requests, please open an issue or submit a pull request.
