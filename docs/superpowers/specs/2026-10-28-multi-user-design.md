# Multi-user instance — design

Status: draft, awaiting owner approval. Date: 2026-10-28.
Builds on: `2026-10-28-shared-playlists-design.md` (implement that first).

## Intent

Today one Explo instance serves one person: a single ListenBrainz account and
a single Plex user, configured through env vars. Gabe and Kelly currently
need two instances. This feature lets one instance serve several people, each
with their own ListenBrainz account and their own Plex account. Each person's
playlist is generated from their own listening and lands in their own Plex
account. A single admin manages everyone from the web UI.

## Goals

- An admin can add, edit, disable, and remove users in the web UI.
- A user is: display name, ListenBrainz username and token, Plex username,
  enabled flag.
- A scheduled or manual run generates each enabled user's playlist from that
  user's ListenBrainz data and creates it in that user's Plex account.
- Downloads go into one shared library. A track wanted by two users is
  downloaded once (the second user's existence check finds it).
- With two or more enabled users, each user's playlist is also copied to every
  other enabled user, titled `<DisplayName>'s <PlaylistName>` (the shared
  playlists feature, with the share list computed instead of typed).
- One user failing (bad token, missing Plex user, empty recommendations) never
  stops the other users' runs.
- Existing single-user installs keep working with no action.

## Non-goals (v1)

- Per-user blocked artists. The existing global `ARTIST_BLACKLIST` keeps
  working and applies to everyone. Per-user blocking is a later feature.
- Per-user schedules, playlist types, or filters. Schedules, enabled
  playlist types, download settings, and matching settings stay global.
- Per-user logins to the web UI. There is one admin.
- Non-Plex backends. Multi-user is Plex-only in v1; other backends keep the
  single-user path.
- Per-user download libraries or quality settings.

## Anti-goals

- A run for one user must never create, delete, or modify another user's own
  playlists, other than the titled copies defined by shared playlists.
- Tokens and Plex credentials must not leak into logs, error messages, API
  responses, or the playlist cache.
- Turning the feature on must not change anything for an install with one
  user.

## How it works today (relevant facts)

- A run is a separate process: the scheduler or manual run spawns
  `explo -c <env> -p <playlist-type>` (`src/web/backend/run/manual_run.go`,
  `src/web/backend/jobs`, `src/web/backend/playlist/jobs.go`). Config is loaded
  from env into `config.Config`. The ListenBrainz user and token come from
  `LISTENBRAINZ_USER` / `LISTENBRAINZ_USER_TOKEN`; the Plex user from
  `SYSTEM_USERNAME`.
- Plex multi-user plumbing exists: with admin credentials set, the client
  switches to a named shared user (`SwitchUser`).
- Playlist cache files live at `<WebDataDir>/cache/<playlist-type>.json` and
  cover caches under `cache/covers`. Custom playlists are stored in
  `custom-playlists.json` with per-id cache files.

## Design

### User store

A new file `<WebDataDir>/users.json`, mode 0600, written atomically (same
approach as `custom-playlists.json` and the env writer). Each user:

```json
{
  "id": "gabe",
  "name": "Gabe",
  "listenbrainz_user": "...",
  "listenbrainz_token": "...",
  "plex_user": "...",
  "enabled": true
}
```

`id` is a lowercase slug generated from the display name, unique, immutable
after creation, and validated against a strict regex before it is ever used in
a path (same discipline as `defs.CustomIDRe`). A new package
`src/web/backend/users` owns load, save, validation, and CRUD handlers, in the
style of `src/web/backend/playlist/custom_playlists.go`.

Tokens are never returned by the API in full (return a "set / not set" flag and
accept a new value on update). They are added to the log redaction in
`src/logging/redact.go`.

### Running per user

- New flag `--user <id>` on the Explo binary. When given, config loading
  overlays that user's `listenbrainz_user`, `listenbrainz_token`, `plex_user`
  (as `Creds.User`) and display name (as `OwnerName`) on top of the env
  config, then runs exactly the existing single-user flow.
- Without `--user`, behaviour is exactly today's, which is the single-user
  path and the path non-Plex installs use.
- The scheduler and manual run, when the user store has at least one enabled
  user and the system is Plex, start one `explo --user <id>` process per
  enabled user, one after another (not in parallel: shared library, shared
  downloader limits, shared Plex scan). A non-zero exit for one user is logged
  and the loop continues.
- Playlist cache files become per user when `--user` is used:
  `cache/<user-id>/<playlist-type>.json`. The playlist view in the web UI
  reads the cache for the user being viewed. Single-user installs keep the
  current path.
- Covers stay shared (they are keyed by release, not by user).

### Shared playlists integration

When `--user` is used, the share list is every other enabled user's
`plex_user`, computed by the scheduler and passed to the process (new flag or
env override; builder's choice), and `OwnerName` is the display name. The typed
`PLAYLIST_SHARE_WITH` is ignored in that mode.

### Upgrade path

On first start with an empty `users.json` and a configured
`LISTENBRAINZ_USER`, Explo does nothing new: the single-user path continues to
work unchanged. The admin creating the first user in the UI is what switches
the instance to multi-user. The UI may offer a one-click "import current
settings as the first user" button; the owner said they will probably set up
from scratch, so this is optional polish, not required.

### Web UI

- A "Users" page, admin-only like every other page: list, add, edit, enable or
  disable, delete. Delete asks for confirmation and removes only the user
  record and that user's cache directory, never anything in Plex.
- A user picker on the playlist view when more than one user exists.
- "Run now" offers "all users" and a single user.
- The settings UI and defs mechanism in `src/web/backend/defs` stays for global
  settings; users are a separate resource and a separate API.

### Validation and errors

- Add/edit validates: non-empty name, unique id, non-empty ListenBrainz user,
  non-empty Plex user. Optionally verify the Plex user exists on the server and
  the ListenBrainz token with a lightweight call, and show the result as a
  warning (builder's call whether this is a save-time check or a separate
  "test" button).
- Missing admin credentials with users configured: the Users page shows a
  clear banner, and runs log one error and fall back to nothing rather than to
  the wrong account.

## Testing

- User store: round-trip, atomic write, id validation (path-traversal strings
  like `../x`, empty, uppercase), duplicate ids, 0600 permissions.
- Config overlay: `--user` overlays only the four fields and leaves the rest of
  the env config untouched; unknown id is a clear error.
- Scheduler loop: two users, the first process fails, the second still runs;
  disabled users are skipped; zero enabled users falls back to the single-user
  path.
- Cache paths: per-user paths used with `--user`, old path without; ids cannot
  escape the cache directory.
- API: tokens never appear in any response or log line.
- Single-user regression: with no `users.json`, command lines and cache paths
  are unchanged.
- Manual: two real Plex users, two ListenBrainz accounts, one run, verify each
  account's own playlist plus the cross-copies.

## Left to the builder

- Package and file split, API route shapes, how the computed share list reaches
  the child process.
- Whether credential verification is on save or a separate button.
- Exact UI layout, following the existing settings pages.

## Open questions for the owner

1. Custom playlists (imported from Spotify, Apple Music and so on): my default
   is that each belongs to the user who created it, and the Users/playlist UI
   shows them under that user. The alternative is leaving them global,
   attached to the first user. Which do you want?
2. Should "all users" runs be allowed to run overlapping with a manual
   single-user run, or should the instance refuse a second run while one is
   active? (Default: refuse, as long as that is how runs behave today. The
   builder confirms.)
3. Is Plex-only for v1 acceptable?
