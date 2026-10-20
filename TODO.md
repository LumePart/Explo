# Explo — Bug Tracking TODO

> Persistent tracker for the code-review bug list. Keep this file updated across sessions — it is the single source of truth for what is open, in progress, and done.
> YouTube downloader items (`src/downloader/youtube.go` issues) were removed from this list at the owner's request and are intentionally not tracked here.

**Created:** 2026-10-19 · **Source:** Full-codebase bug review (6 subsystem reviewers + adversarial verification pass, all findings re-checked against source) · **Branch at review:** `fix/album-dedupe` (`46d8f00`)

## How to use this file across sessions

1. **Start of session:** Read this file. Pick the highest-severity `open` item, change its Status to `in-progress`, and note the session/date.
2. **Before fixing:** Re-check the cited `file:line` — line numbers drift as code changes. Confirm the bug still exists before editing.
3. **Fix:** Make the smallest fix at the root cause. Add or update a test where the area has committed tests nearby.
4. **Verify:** Run `go vet ./...` and the relevant package tests, e.g. `go test ./src/client/...`. Note: some packages need localhost sockets (`httptest`), which may be blocked in sandboxed environments — record that instead of claiming a pass you did not see.
5. **End of session:** Update the checkbox, Status, PR/commit, and Verification fields for anything you touched — even if unfinished. Leave a one-line Handoff note so the next session can resume without re-investigating.
6. **Do not delete issues.** Move fixed items to `done` with the commit/PR that fixed them. If an issue turns out to be invalid, mark it `wontfix` with a one-line reason.

Status values: `open` · `in-progress` · `blocked` · `done` · `wontfix`

## Progress

| Category | Total | Done | Open |
|---|---|---|---|
| App crashes | 5 | 5 | 0 |
| Data loss and corruption | 3 | 3 | 0 |
| Security | 2 | 2 | 0 |
| Playlists broken or wrong | 7 | 7 | 0 |
| Hangs and resource leaks | 4 | 4 | 0 |
| Wrong songs and metadata | 4 | 4 | 0 |
| Web UI and settings screens | 4 | 4 | 0 |
| Scheduling | 2 | 2 | 0 |
| **Total** | **31** | **31** | **0** |

Update this table whenever an issue changes status.

---

## App crashes

- [x] **BUG-01 — Session race crashes the app** · Severity: High
  - Location: `src/web/backend/auth/session.go:141`
  - User impact: Using the UI in multiple tabs or clicking fast can crash the whole app.
  - Detail: `Session.Get/Put/Delete` access `s.data`/`s.id` with no lock (only `Migrate` locks). Concurrent requests sharing one session cause a fatal `concurrent map writes` panic.
  - Fix direction: Lock all session access (guard `Get`/`Put`/`Delete` and `writeCookieIfNecessary`'s read of `session.id`).
  - Status: done · PR/commit: local branch `fix/bug-01-session-race` (c85aaaf, unpushed) · Verification: new -race regression test passes; golangci-lint 0 issues
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

- [x] **BUG-02 — Lidarr album cache race crashes the app** · Severity: High
  - Location: `src/downloader/lidarr.go:353` (also `:519`, panic at `:527`)
  - User impact: Fetching several albums at once can crash the app instead of downloading them.
  - Detail: `AlbumCache` map is read without `CacheMu` while `cacheAlbum` writes it under lock from concurrent goroutines. At `:519` a nil cache entry is then dereferenced at `:527`.
  - Fix direction: `RLock` the reads and add a nil guard before use.
  - Status: done · PR/commit: local branch `fix/bug-02-lidarr-cache-race` (88bac9b, unpushed) · Verification: new -race regression test passes; golangci-lint 0 issues
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

- [x] **BUG-03 — Empty artist list panics the run** · Severity: High
  - Location: `src/discovery/listenbrainz.go:393`
  - User impact: One unusual track from ListenBrainz can crash the whole run so no playlist gets made.
  - Detail: `recArtists[0]` is indexed unconditionally; ListenBrainz metadata with an empty/missing artists array panics.
  - Fix direction: Guard `len(recArtists)` and degrade gracefully (skip or fall back to a joined/empty artist).
  - Status: done · PR/commit: local branch `fix/bug-03-empty-artist-panic` (1b16855, unpushed) · Verification: new unit tests pass; full discovery package passes; golangci-lint 0 issues
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

- [x] **BUG-04 — Config typo panics instead of erroring** · Severity: High
  - Location: `src/discovery/discovery.go:21`, dereferenced at `src/main/main.go:198`
  - User impact: One typo in your config crashes the app with no hint about what you typed wrong.
  - Detail: `NewDiscoverer` returns nil for any `DISCOVERY_SERVICE` other than `listenbrainz`, and no config validation catches it.
  - Fix direction: Validate the value in config loading, or return an error from `NewDiscoverer` and fail fast with the bad value in the message.
  - Status: done · PR/commit: local branch `fix/bug-04-config-typo-panic` (8c7fbf1, unpushed) · Verification: new unit tests pass; go vet ./... clean; golangci-lint 0 issues
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

- [x] **BUG-05 — Plex library panic** · Severity: Medium
  - Location: `src/client/plex.go:381`
  - User impact: Plex hiccups while creating a library can crash the run instead of showing an error.
  - Detail: `AddLibrary()` indexes `MediaContainer.Library[0]` with no length check when Plex returns an empty container.
  - Fix direction: Length-check and return a descriptive error.
  - Status: done · PR/commit: local branch `fix/bug-05-plex-library-panic` (1d957aa, unpushed) · Verification: new unit tests pass; go vet + golangci-lint clean
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

## Data loss and corruption

- [x] **BUG-06 — Custom playlists can be wiped by a race** · Severity: High
  - Location: `src/web/backend/playlist/custom_playlists.go:107`
  - User impact: Adding or removing playlists at the wrong moment can lose or wipe all your custom playlists.
  - Detail: `saveCustomPlaylists` does an unlocked, non-atomic `os.WriteFile` while import/delete/refresh/cron all do unsynchronized load-modify-save. A reader mid-truncate gets a parse error, the loader returns nil, and the next save wipes `custom-playlists.json`.
  - Fix direction: Add a mutex around load-modify-save and write via temp file + rename.
  - Status: done · PR/commit: local branch `fix/bug-06-custom-playlists-race` (51474f7, unpushed) · Verification: new -race regression tests pass; golangci-lint 0 issues
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

- [x] **BUG-07 — File move can erase an existing song** · Severity: Medium
  - Location: `src/downloader/downloader.go:400` (see also `src/downloader/slskd_album.go:347`)
  - User impact: Moving files can silently erase an existing song in your library.
  - Detail: `moveFile` truncates the destination with `os.Create` before copying, with no same-file or existing-file check. If `src == dst` (or a sibling name collides with a library file), the source/existing file is zeroed while the track is still reported present.
  - Fix direction: Refuse or skip when source and destination are the same file or the destination exists.
  - Status: done · PR/commit: local branch `fix/bug-07-file-move-erases-song` (8b4d1c3, unpushed) · Verification: new unit tests pass; golangci-lint 0 issues
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

- [x] **BUG-08 — Settings saves can be lost or corrupt the file** · Severity: Medium
  - Location: `src/web/backend/settings/settings.go:124`
  - User impact: Saving settings in quick succession can silently undo some of your changes, and a crash mid-save can corrupt the file.
  - Detail: `UpdateEnvKeys` does an unsynchronized read-modify-write with a truncating `os.WriteFile`; concurrent handlers lose each other's updates, and a crash mid-write leaves a truncated `.env`.
  - Fix direction: Lock the read-modify-write and write via temp file + rename. Related: BUG-06 (same pattern), BUG-10 (same file).
  - Status: done · PR/commit: local branch `fix/bug-08-settings-save-race` (95d8455, unpushed) · Verification: new -race regression test + full settings package pass; golangci-lint 0 issues
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

## Security

- [x] **BUG-09 — Settings value can inject extra config keys** · Severity: High
  - Location: `src/web/backend/settings/settings.go:134`
  - User impact: A value containing a line break can silently overwrite your other settings on the next start.
  - Detail: `formatEnvValue` quotes only on `"$#?'`, never newline/CR, and `UpdateEnvKeys` writes `key=value` raw — a value like `x\nAPI_KEY=pwned` injects arbitrary extra `.env` lines.
  - Fix direction: Reject or escape CR/LF in keys and values before writing.
  - Status: done · PR/commit: local branch `fix/bug-09-env-injection` (3c0cd50, unpushed) · Verification: new unit tests pass; full settings package passes; golangci-lint 0 issues
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

- [x] **BUG-10 — Admin API key shown in plain text** · Severity: Low
  - Location: `src/web/frontend/src/lib/utils.js:5` (rendered at `src/web/frontend/src/components/Settings.jsx:888`)
  - User impact: An admin API key shows in plain text on the settings screen where it should be hidden.
  - Detail: `SENSITIVE_KEY_RE` matches `api_key` but not `APIKEY` (no underscore), so keys such as `ADMIN_SYSTEM_APIKEY` are not masked.
  - Fix direction: Add `apikey` to the sensitive-key pattern and audit for other unmasked key spellings.
  - Status: done · PR/commit: local branch `fix/bug-10-sensitive-key-mask` (65d43d2, unpushed) · Verification: Node probe over 15 sensitive + 5 plain spellings ALL OK; no JS test runner in repo so no committed test
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

## Playlists broken or wrong

- [x] **BUG-11 — Playlists gain songs that never arrived** · Severity: High
  - Location: `src/downloader/monitor.go:135` (error only logged at `:140`)
  - User impact: Playlists can include songs that never actually landed in your library, and they never get retried.
  - Detail: The track is marked `Present`, counted as success, and cleaned up *before* `MoveDownload` runs; if the move fails, the playlist still keeps the track.
  - Fix direction: Set `Present` and count success only after `MoveDownload` succeeds.
  - Status: done · PR/commit: local branch `fix/bug-11-move-before-present` (1287887, unpushed) · Verification: new stub tests pass; golangci-lint 0 issues
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

- [x] **BUG-12 — One missing song can block a whole Subsonic playlist** · Severity: High
  - Location: `src/client/subsonic.go:238`
  - User impact: One song that could not be found can stop the entire playlist from being created in Navidrome/Subsonic.
  - Detail: `CreatePlaylist` appends `&songId=<id>` for every track without checking `track.Present` or an empty ID (unlike the Jellyfin/Emby formatters), so an absent track sends `&songId=` and the server can reject the whole create.
  - Fix direction: Skip tracks that are not present or have an empty ID.
  - Status: done · PR/commit: local branch `fix/bug-12-subsonic-missing-song` (a19fa89, unpushed) · Verification: new unit tests pass; golangci-lint 0 issues
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

- [x] **BUG-13 — Jellyfin playlist creation fails without a username** · Severity: Medium
  - Location: `src/client/jellyfin.go:236`
  - User impact: Without a username set, playlist creation in Jellyfin always fails.
  - Detail: With `SYSTEM_USERNAME` unset, `CreatePlaylist` sends the API key (`resolveAPIKey()`) as the Jellyfin `UserId`, which the server rejects with 400/404.
  - Fix direction: Resolve the real user ID from the API, or omit the user where the API allows it; fail with a clear message otherwise.
  - Status: done · PR/commit: local branch `fix/bug-13-jellyfin-userid` (39bff7b, unpushed) · Verification: new table tests pass; golangci-lint 0 issues
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

- [x] **BUG-14 — “Replace playlist” duplicates playlists on Emby** · Severity: Medium
  - Location: `src/client/emby.go:241`
  - User impact: Choosing “replace playlist” on Emby quietly piles up duplicate playlists instead of replacing.
  - Detail: `DeletePlaylist()` is a commented-out no-op returning nil, so callers believe the old playlist was deleted before creating the new one.
  - Fix direction: Implement the Emby delete call, or make replace explicitly delete-then-create.
  - Status: done · PR/commit: local branch `fix/bug-14-emby-delete-noop` (020c66d, unpushed) · Verification: go vet + golangci-lint clean; httptest delete tests committed — run on CI (sandbox blocks localhost binds)
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

- [x] **BUG-15 — Playlist names with quotes break Jellyfin/Emby** · Severity: Medium
  - Location: `src/client/jellyfin.go:243`, `src/client/emby.go:227`
  - User impact: Playlist names containing quotes break creation with a server error.
  - Detail: Playlist name/description are interpolated unescaped into JSON built with `fmt`, so a quote, backslash, or newline produces invalid JSON and the server returns 400.
  - Fix direction: Build the bodies with `json.Marshal` instead of string formatting.
  - Status: done · PR/commit: local branch `fix/bug-15-playlist-json-escape` (c27927f, unpushed) · Verification: new round-trip test passes; golangci-lint 0 issues
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

- [x] **BUG-16 — MPD playlists double on every re-run** · Severity: Medium
  - Location: `src/client/mpd.go:67`
  - User impact: Re-running the same playlist in MPD adds every song again, doubling it each time.
  - Detail: The `.m3u` is opened with `O_APPEND`, never truncated, and the handle is never closed.
  - Fix direction: Open with `O_TRUNC` (create fresh like the other clients) and close the file.
  - Status: done · PR/commit: local branch `fix/bug-16-mpd-append` (decf51d, unpushed) · Verification: new unit test passes; golangci-lint 0 issues
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

- [x] **BUG-17 — Playlist card Run overwrites the wrong playlist** · Severity: Medium
  - Location: `src/web/frontend/src/components/Settings.jsx:401`
  - User impact: Running a playlist from its card can overwrite a different playlist than the one you clicked.
  - Detail: The tracklist Run handler passes `replacePlaylists[playlist]` (the manual-run selector's state) instead of the opened tracklist's own flag.
  - Fix direction: Use `openTracklist` when looking up the replace flag.
  - Status: done · PR/commit: local branch `fix/bug-17-tracklist-replace-flag` (7045dcb, unpushed) · Verification: vite build passes; no JS test runner in repo so no committed test
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

## Hangs and resource leaks

- [x] **BUG-18 — A trickling download hangs the run forever** · Severity: Medium
  - Location: `src/downloader/monitor.go:161` (deadline check at `:167`)
  - User impact: One very slow download can hang the whole run forever instead of timing out.
  - Detail: The progress branch `continue`s before the `MaxDuration` check, so any download that moves at least a byte per poll never times out.
  - Fix direction: Check the deadline before the progress `continue`, on every poll.
  - Status: done · PR/commit: branch `feat/download-provider-fallback` (this commit) · Verification: `go vet ./...` clean; `go test ./src/downloader/ -run TestMonitorDownloads` passes including `-race`; new regression tests `TestMonitorDownloads_TricklingTrackHitsMaxDuration`, `TestMonitorDownloads_FallbackAfterAbandonsUnresolvedTracks`, `TestMonitorDownloads_FallbackAfterZeroWaitsForCompletion`. Full downloader package still fails only on pre-existing sandbox `httptest: bind: operation not permitted` (confirmed identical on untouched `dev`).
  - Handoff: Fixed together with the provider-fallback port: the progress branch no longer bypasses `MaxDuration`, and the new `SLSKD_FALLBACK_AFTER`/`LIDARR_FALLBACK_AFTER` whole-provider budget (default 0 = disabled) abandons unresolved tracks (with `Cleanup`) so the next `DOWNLOAD_SERVICES` provider gets a turn. `dfaaedb`'s separate `SLSKD_MONITOR_MAX_RUNTIME` knob was not ported — its intent is now covered by the fixed `MaxDuration` plus `FallbackAfter`, avoiding two near-identical caps.

- [x] **BUG-19 — One long log line wedges all future runs** · Severity: Medium
  - Location: `src/web/backend/run/events.go:48` (wait at `:65`)
  - User impact: One very long log line can wedge a run so every later run is refused as “already running”.
  - Detail: `collectRunOutput` uses `bufio.Scanner` with the default 64KB line limit; an over-long line stops draining the pipe, the child blocks on write, and `cmd.Wait` never returns.
  - Fix direction: Enlarge the scanner buffer and keep draining even after a scan error.
  - Status: done · PR/commit: local branch `fix/bug-19-long-log-line` (63d5cf3, unpushed) · Verification: new unit tests + full run package pass; golangci-lint 0 issues
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

- [x] **BUG-20 — Stalled or huge remote downloads can hang or OOM the app** · Severity: Medium
  - Location: `src/util/http.go:94` (also `:106`, cover path at `:137`)
  - User impact: One stalled or oversized download can hang the app or eat all its memory.
  - Detail: `http.Get`/default client with no timeout, plus unbounded `io.ReadAll`, on remote responses (same pattern in playlist/apple-music fetching).
  - Fix direction: Use a timeout client and cap reads with `io.LimitReader`.
  - Status: done · PR/commit: local branch `fix/bug-20-http-timeouts` (8ac653e, unpushed) · Verification: new util unit tests pass; playlist package has no tests; golangci-lint 0 issues
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

- [x] **BUG-21 — Rejected runs leak file handles** · Severity: Low
  - Location: `src/web/backend/run/manual_run.go:95`
  - User impact: Repeatedly trying to start a run while one is running slowly leaks system resources until the app can fail.
  - Detail: The already-running branch closes `pw` a second time instead of closing the log file `lf` opened earlier, leaking one descriptor per rejected attempt.
  - Fix direction: Close `lf` on that branch.
  - Status: done · PR/commit: local branch `fix/bug-21-rejected-run-leak` (6f831aa, unpushed) · Verification: new FD-count regression test + full run package pass; golangci-lint 0 issues
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

## Wrong songs and metadata

- [x] **BUG-22 — Every song tagged as track 1, disc 1** · Severity: Medium
  - Location: `src/discovery/listenbrainz.go:542`
  - User impact: Every song gets tagged as “track 1” of disc 1, scrambling album ordering in your player.
  - Detail: Enrichment always takes `Media[0].Tracks[0]`, so track number, disc number, and release-track ID come from the first track of the first medium for every song.
  - Fix direction: Select the track matching the recording/release track being enriched.
  - Status: done · PR/commit: local branch `fix/bug-22-track-number` (ab7c991, unpushed) · Verification: new unit tests + full discovery package pass; golangci-lint 0 issues
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

- [x] **BUG-23 — Playlist order is shuffled between runs** · Severity: Medium
  - Location: `src/discovery/listenbrainz.go:357`
  - User impact: Your playlist order changes randomly between runs instead of following the recommendation ranking.
  - Detail: `getTracks` iterates the `Recordings` map, losing the recommendation score order from the API response.
  - Fix direction: Iterate the ordered API slice, not the map.
  - Status: done · PR/commit: local branch `fix/bug-23-playlist-order` (8228f08, unpushed) · Verification: new unit tests + full discovery package pass; golangci-lint 0 issues
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

- [x] **BUG-24 — Bad metadata entries for some file types** · Severity: Medium
  - Location: `src/util/metadata.go:45` (tag sets at `:99`, `:162`)
  - User impact: Some songs fail tagging or get garbled tags depending on file type.
  - Detail: Tag helpers check the value but not an empty key, and several tag sets leave fields empty — producing `=value` ffmpeg metadata entries for formats like m4a/wav and ape-family files.
  - Fix direction: Skip empty keys (and empty values) when building ffmpeg metadata args.
  - Status: done · PR/commit: local branch `fix/bug-24-metadata-empty-keys` (ae5f305, unpushed) · Verification: new unit tests + full util package pass; golangci-lint 0 issues
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

- [x] **BUG-25 — Failed cover art can fail the song** · Severity: Medium
  - Location: `src/util/http.go:136` (caller at `src/downloader/youtube.go:259`)
  - User impact: When cover art fails to download the song can fail too, instead of just skipping the artwork.
  - Detail: `DownloadCover` returns a destination path even when the fetch or write failed, so callers embed a cover from a file that does not exist and the track save fails.
  - Fix direction: Return an error (or empty path) on failure and let callers proceed without cover art.
  - Status: done · PR/commit: local branch `fix/bug-25-cover-failure` (5da4767, unpushed) · Verification: new failure-path unit test passes; golangci-lint 0 issues
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

## Web UI and settings screens

- [x] **BUG-26 — Buttons break after a restart or a week left open (CSRF)** · Severity: Medium
  - Location: `src/web/frontend/src/lib/api.js:1` (backend check: `src/web/backend/auth/middleware.go:119`)
  - User impact: After a restart or a week left open, every button that changes something fails until you reload the page.
  - Detail: The frontend caches the CSRF token forever in a module variable, but the backend mints a new token per session. Sessions are in-memory with a 7-day expiry, so any restart or expiry strands the open page with a stale token and every POST/DELETE returns `403 CSRF token mismatch`. Login/logout do not rotate the token, so they are not a trigger.
  - Fix direction: In `apiFetch`, on 403 clear the cached token, re-fetch `/api/ui/csrf`, and retry the request once.
  - Status: done · PR/commit: local branch `fix/bug-26-csrf-retry` (52f6bce, unpushed) · Verification: mocked-fetch Node probe ALL OK + vite build; no JS test runner in repo so no committed test
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

- [x] **BUG-27 — Server hiccup leaves a permanent blank page** · Severity: Medium
  - Location: `src/web/frontend/src/App.jsx:17`, `src/web/frontend/src/lib/api.js:54`
  - User impact: If the server hiccups while the page loads you get a permanent blank page instead of an error or login screen.
  - Detail: Startup fetches have no `catch` and `fetchConfig` does not check `res.ok`, so a 500/network failure throws and the view never leaves its initial state.
  - Fix direction: Check `res.ok`, catch startup errors, and render an error/retry state.
  - Status: done · PR/commit: local branch `fix/bug-27-startup-blank-page` (76d5ea6, unpushed) · Verification: vite build passes; no JS test runner in repo so no committed test
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

- [x] **BUG-28 — Path-template settings page can never load** · Severity: Medium
  - Location: `src/web/backend/routes.go:61`
  - User impact: The path-template settings screen simply returns “not found” and can never be used.
  - Detail: The pattern `api/ui/path-templates` lacks a leading `/`, so `ServeMux` parses `api` as a host pattern and normal requests 404.
  - Fix direction: Register `/api/ui/path-templates`.
  - Status: done · PR/commit: local branch `fix/bug-28-path-templates-route` (77a2b56, unpushed) · Verification: new mux-pattern test passes; golangci-lint 0 issues
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

- [x] **BUG-29 — Wizard fields look editable but edits are ignored** · Severity: Medium
  - Location: `src/web/frontend/src/components/Wizard.jsx:171` (backend values: `src/web/backend/settings/handlers.go:40`)
  - User impact: Fields that should be locked by your Docker config look editable, but your edits are silently ignored.
  - Detail: Step 2 checks `envSources[key] === '.env'`, but the backend only emits `'env'` or `'file'`, so the lock never engages.
  - Fix direction: Compare against `'env'` (as Steps 1 and 3 already do).
  - Status: done · PR/commit: local branch `fix/bug-29-wizard-env-lock` (65e2c31, unpushed) · Verification: vite build passes; no JS test runner in repo so no committed test
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

## Scheduling

- [x] **BUG-30 — Legacy schedule setting disables all scheduling** · Severity: High
  - Location: `docker/start.sh:66`
  - User impact: If you set the old schedule option, none of your schedules run and music just never arrives.
  - Detail: The legacy `CRON_SCHEDULE` branch runs `crond -f` in the foreground, blocking the later per-playlist schedules, `EXECUTE_ON_START`, and the final `crond` invocation.
  - Fix direction: Don't run `crond` in the foreground in that branch; register the cron entry and let the normal startup path start the daemon.
  - Status: done · PR/commit: local branch `fix/bug-30-legacy-cron-schedule` (526fb4b, unpushed) · Verification: sh -n clean + registration simulation OK; shell script, no committed test
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

- [x] **BUG-31 — Weekly playlists start refreshing daily** · Severity: Medium
  - Location: `src/web/backend/playlist/jobs.go:45`
  - User impact: A playlist set to refresh weekly starts refreshing every day once it is old enough.
  - Detail: The cron closure gates on the `LastFetched` value captured at registration and never reloads it before the check (the fresh load happens only after the gate), so once a playlist is past its refresh interval it refreshes on every daily tick.
  - Fix direction: Reload the playlist state before evaluating the refresh gate.
  - Status: done · PR/commit: local branch `fix/bug-31-weekly-refresh` (872957f, unpushed) · Verification: new table tests pass; golangci-lint 0 issues
  - Handoff: fixed locally, not pushed and no PR opened yet — review the branch, then push + open PR per issue

---

## Session log

Record one line per working session so progress survives across sessions.

| Date | Session / author | Issues touched | Outcome / handoff |
|---|---|---|---|
| 2026-10-19 | Bug review | BUG-01 – BUG-31 created | Tracker created; all items open and unverified for fixes. |
| 2026-10-19 | Per-issue local fixes | BUG-01–17, BUG-19–31 fixed | All 30 remaining bugs fixed, each on its own local fix/bug-* branch with its own commit (see BUG entries). Nothing pushed; branches await review, then push + one PR per issue. |
| 2026-10-19 | Provider fallback port | BUG-18 done | Ported `FallbackAfter` from stale `b7122e3` onto `dev` (branch `feat/download-provider-fallback`, PR #25) and fixed BUG-18 deadline bypass. `feat/lidarr`/`feat/provider-fallback` remote branches deleted after the port landed on the new branch. |
