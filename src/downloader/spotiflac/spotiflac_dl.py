#!/usr/bin/env python3
"""Bundled helper that downloads a single track via the SpotiFLAC module's
provider API, used by Explo's spotiflac downloader for tracks that have no
streaming URL (e.g. ListenBrainz discovery).

This mirrors how youtube_music/search_ytmusic.py shells out to ytmusicapi: it
resolves a track by ISRC (falling back to a title/artist text search inside each
provider) and downloads it from one of the configured FLAC sources in priority
order. The official `spotiflac` CLI handles URL-based imports (e.g. Spotify); this
helper handles the metadata-search path the CLI cannot do.

It receives the track metadata as a JSON object on argv[1] (or stdin) and prints a
single JSON result line to stdout:

    {"success": true, "file": "Song.flac", "path": "/abs/Song.flac",
     "provider": "deezer", "format": "flac"}

or, on failure:

    {"success": false, "error": "deezer: ...; tidal: ..."}

All library/diagnostic output is routed to stderr so stdout carries only the
final JSON line for the Go side to parse.
"""
import sys
import os
import json
import asyncio
import contextlib


def _read_payload():
    if len(sys.argv) > 1 and sys.argv[1] not in ("-", ""):
        return json.loads(sys.argv[1])
    return json.loads(sys.stdin.read())


def _run(p):
    try:
        from SpotiFLAC.providers import PROVIDER_REGISTRY
        from SpotiFLAC.core.models import TrackMetadata
    except Exception as e:  # noqa: BLE001 - report import failure to caller as JSON
        return {"success": False, "error": f"SpotiFLAC import failed: {e}"}

    artists = p.get("artists") or ""
    meta = TrackMetadata(
        id=str(p.get("id") or p.get("isrc") or "explo"),
        title=p.get("title", ""),
        artists=artists,
        album=p.get("album") or "",
        album_artist=p.get("album_artist") or artists,
        isrc=p.get("isrc") or "",
        track_number=int(p.get("track_number") or 0),
        duration_ms=int(p.get("duration_ms") or 0),
        cover_url=p.get("cover_url") or "",
    )

    output_dir = p["output_dir"]
    os.makedirs(output_dir, exist_ok=True)

    sources = p.get("sources") or ["deezer", "tidal", "qobuz", "amazon", "netease", "joox"]
    filename_format = p.get("filename_format") or "{title} - {artist}"
    timeout_s = int(p.get("timeout_s") or 0) or None

    errors = []
    for name in sources:
        cls = PROVIDER_REGISTRY.get(name)
        if cls is None:
            errors.append(f"{name}: unknown source")
            continue
        try:
            provider = cls(timeout_s=timeout_s) if timeout_s else cls()
        except TypeError:
            provider = cls()

        try:
            # Each provider matches by ISRC first, then a title/artist text search.
            # allow_fallback=False keeps this provider self-contained so our own
            # source loop controls the priority order.
            res = asyncio.run(
                provider.download_track_async(
                    meta,
                    output_dir,
                    filename_format=filename_format,
                    allow_fallback=False,
                )
            )
        except Exception as e:  # noqa: BLE001 - a failing provider must not abort the chain
            errors.append(f"{name}: {e}")
            continue

        if getattr(res, "success", False) and getattr(res, "file_path", None):
            return {
                "success": True,
                "file": os.path.basename(res.file_path),
                "path": res.file_path,
                "provider": res.provider,
                "format": getattr(res, "format", None) or "flac",
            }
        errors.append(f"{name}: {getattr(res, 'error', None) or 'no file downloaded'}")

    return {"success": False, "error": "; ".join(errors) or "all sources failed"}


def main():
    try:
        payload = _read_payload()
    except Exception as e:  # noqa: BLE001
        print(json.dumps({"success": False, "error": f"invalid payload: {e}"}))
        return 1

    # Keep stdout clean: library prints/progress and logging go to stderr; only
    # the final JSON (printed below, outside this block) reaches stdout.
    with contextlib.redirect_stdout(sys.stderr):
        outcome = _run(payload)

    print(json.dumps(outcome))
    return 0 if outcome.get("success") else 1


if __name__ == "__main__":
    sys.exit(main())
