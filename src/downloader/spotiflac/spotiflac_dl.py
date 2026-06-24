#!/usr/bin/env python3
"""Bundled wrapper that downloads a single track via the SpotiFLAC module.

Explo invokes this as a subprocess (mirroring how youtube_music/search_ytmusic.py
shells out to ytmusicapi). It receives the track metadata as a JSON object on
argv[1] (or stdin), tries each configured source/provider in priority order, and
prints a single JSON result line to stdout:

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

    meta = TrackMetadata(
        id=str(p.get("id") or p.get("isrc") or "explo"),
        title=p.get("title", ""),
        artists=p.get("artists", ""),
        album=p.get("album", ""),
        album_artist=p.get("album_artist") or p.get("artists", ""),
        isrc=p.get("isrc") or "",
        track_number=int(p.get("track_number") or 0),
        duration_ms=int(p.get("duration_ms") or 0),
        cover_url=p.get("cover_url") or "",
    )

    output_dir = p["output_dir"]
    os.makedirs(output_dir, exist_ok=True)

    sources = p.get("sources") or ["deezer", "tidal", "qobuz", "amazon"]
    quality = p.get("quality") or ""
    filename_format = p.get("filename_format") or "{title} - {artist}"
    qobuz_token = p.get("qobuz_token") or ""
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

        kwargs = {"allow_fallback": True, "filename_format": filename_format}
        if quality:
            kwargs["quality"] = quality
        if qobuz_token:
            kwargs["qobuz_token"] = qobuz_token

        try:
            # download_track (<=1.2.0) became the async download_track_async (>=1.2.1)
            if hasattr(provider, "download_track"):
                res = provider.download_track(meta, output_dir, **kwargs)
            elif hasattr(provider, "download_track_async"):
                res = asyncio.run(provider.download_track_async(meta, output_dir, **kwargs))
            else:
                errors.append(f"{name}: provider exposes no download method")
                continue
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
