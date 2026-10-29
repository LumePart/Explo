# Multi-User Instance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One Explo instance serves several people, each with their own ListenBrainz account and Plex account, managed by one admin in the web UI.

**Architecture:** A `users.json` store managed through a new Users API. A `--user <id>` flag overlays one user's details on the env config; `--user all` loops over enabled users as sequential child processes. Playlist caches and custom playlists become per user. Frontend adds a Users tab and small additions to Home, Settings and the wizard.

**Tech Stack:** Go (stdlib `net/http`, `slog`), React + Vite + Tailwind in `src/web/frontend`.

**Spec:** `docs/superpowers/specs/2026-10-28-multi-user-design.md`. **Depends on** the shared-playlists work (`docs/superpowers/plans/2026-10-28-shared-playlists.md`): this branch is stacked on it and uses its `PLAYLIST_SHARE_WITH` / `PLAYLIST_OWNER_NAME` config and `SharePlaylist`.

## Global Constraints

- Plex only. Single-user installs (no `users.json`, or an empty one) behave exactly as today: same command lines, same cache paths, same env keys.
- `users.json` lives in `WebDataDir`, mode 0600, written atomically.
- User `id`: lowercase slug of the display name, unique, immutable, validated against `^[a-z0-9][a-z0-9-]{0,31}$` before it is ever used in a path.
- Tokens are never returned by any API in full (return `token_set: bool`), never logged, and added to `src/logging/redact.go`.
- `--user all` rules: `users.json` absent or empty runs the single-user path. Unreadable or corrupt runs nothing and errors. Users exist but none enabled runs nothing with a warning. A `custom-*` playlist runs only for its owner. One user failing never stops the others; overall exit code is non-zero if any user failed.
- Schedules, playlist types, download and matching settings stay global. Blocked artists per user is out of scope; the global `ARTIST_BLACKLIST` still applies.
- No new run locking.
- Pre-merge checks per task: `go vet ./...`, `golangci-lint run ./...`, `go test -race ./...` (sandbox may block localhost sockets; record that instead of claiming a pass). Frontend: `npm run build` in `src/web/frontend`.

## Review Focus

- `users.json` corrupt or unreadable: runs and API refuse rather than falling back to the env account. Pinned in Task 4.
- Ids like `../x`, `A`, empty, 33 characters, or a duplicate slug (`Kelly` and `kelly!`): rejected, never reach a path. Pinned in Task 1.
- Tokens: absent from every API response and from log output. Pinned in Task 3.
- Deleting a user who owns custom playlists, and deleting the last user (instance returns to single-user mode). Pinned in Tasks 3 and 6.
- A user with no token or a Plex user missing on the server: skipped with a warning while the others still run. Pinned in Task 5.

## File Structure

- Create `src/web/backend/users/` (`users.go` model + store + validation, `handlers.go` API) with tests.
- Modify `src/config/flags.go`, `src/config/config.go`: `--user` flag and overlay (`src/config/user_overlay.go` new).
- Create `src/main/all_users.go`: the `--user all` loop.
- Modify `src/main/main.go`, `src/web/backend/playlist/*` (cache paths, custom `user_id`), `src/web/backend/run/*` (manual run user), `src/web/backend/routes.go`, `src/web/backend/server.go`, `src/web/backend/settings/handlers.go`, `src/logging/redact.go`, `docker/start.sh`.
- Frontend: `src/web/frontend/src/lib/api.js`, new `components/Users.jsx`, modify `components/Settings.jsx`, `components/ui/ImportModal.jsx`, `components/ui/PlaylistCard.jsx` (tag only), `components/Wizard.jsx`.

---

### Task 1: User model, validation, and file store

**Files:**
- Create: `src/web/backend/users/users.go`
- Test: `src/web/backend/users/users_test.go`

**Interfaces:**
- Produces:
  - `type User struct { ID, Name, ListenBrainzUser, ListenBrainzToken, PlexUser string; Enabled bool }` with JSON tags `id,name,listenbrainz_user,listenbrainz_token,plex_user,enabled`
  - `var IDRe = regexp.MustCompile("^[a-z0-9][a-z0-9-]{0,31}$")`
  - `func Slug(name string) string`
  - `func Load(dataDir string) ([]User, error)` — missing file returns `(nil, nil)`; unreadable or invalid JSON returns an error
  - `func Save(dataDir string, users []User) error` — atomic, 0600
  - `func Validate(u User, existing []User, editing bool) error` — name, LB user, Plex user non-empty; id valid; id unique unless editing
  - `func Enabled(users []User) []User`

- [ ] **Step 1: Write failing tests**:
  - `TestSlug`: `"Kelly W."`→`kelly-w`; `"  "`→`""`; `"Ünï"`→`n` (non-ASCII dropped; empty result must fail validation).
  - `TestValidate_RejectsBadIDs`: `../x`, `A`, `""`, 33 chars, `-a`; and duplicate id on create (`Kelly` vs `kelly!`).
  - `TestSaveLoad_RoundTripAnd0600`: save two users, load equal, file mode `0600`, no temp file left.
  - `TestLoad_MissingIsEmptyCorruptIsError`: absent → `nil, nil`; `{not json` → non-nil error.
- [ ] **Step 2: Run** `go test ./src/web/backend/users/ -v`; expect FAIL (undefined).
- [ ] **Step 3: Implement** following the atomic-write approach in `src/web/backend/playlist/custom_playlists.go` (temp file in same dir, rename).
- [ ] **Step 4: Run tests**; expect PASS.
- [ ] **Step 5: Commit** `feat: add multi-user store and validation`.

### Task 2: Per-user cache layout

**Files:**
- Modify: `src/web/backend/playlist/playlist.go` (`WritePlaylistCache`, `writePreliminaryCache`, `writePrefetchCache`, `writeTrackCache` and their cache-path joins)
- Create: `src/web/backend/playlist/cachepath.go`
- Test: `src/web/backend/playlist/cachepath_test.go`

**Interfaces:**
- Produces:
  - `func CacheDir(dataDir, userID string) string` — `dataDir/cache` when `userID == ""`, else `dataDir/cache/users/<userID>`; panics are not acceptable: an id failing `users.IDRe` returns `dataDir/cache/users/_invalid` (never a traversal path).
  - `func RemoveUserCache(dataDir, userID string) error`
  - `WritePlaylistCache(dataDir, userID, playlist string, tracks []*models.Track, added map[string]bool)` — new `userID` parameter; every existing caller passes `""` until Task 5.

- [ ] **Step 1: Write failing tests**: `TestCacheDir_EmptyUserIsLegacyPath`; `TestCacheDir_UserPath`; `TestCacheDir_TraversalIDNeverEscapes` (`../../etc`, `a/b`, `..` all stay under `dataDir/cache/users`); `TestWritePlaylistCache_WritesUnderUserDir`; `TestRemoveUserCache_RemovesOnlyThatUser`.
- [ ] **Step 2: Run** `go test ./src/web/backend/playlist/ -run 'CacheDir|UserCache|WritePlaylistCache' -v`; expect FAIL.
- [ ] **Step 3: Implement** `CacheDir`/`RemoveUserCache`, thread `userID` through the cache writers, update all callers (`src/main/main.go` and playlist handlers) to pass `""`. Covers stay in the shared `cache/covers`.
- [ ] **Step 4: Run** `go build ./... && go test ./src/web/backend/playlist/...`; expect PASS, including existing tests.
- [ ] **Step 5: Commit** `refactor: make playlist cache paths per user`.

### Task 3: Users API

**Files:**
- Create: `src/web/backend/users/handlers.go`
- Modify: `src/web/backend/routes.go` (add `registerUserRoutes`), `src/web/backend/server.go` (construct the handler), `src/logging/redact.go`
- Test: `src/web/backend/users/handlers_test.go`, `src/logging/redact_test.go`

**Interfaces:**
- Consumes: Task 1 store, Task 2 `playlist.RemoveUserCache`.
- Produces routes (all behind `s.auth`): `GET /api/ui/users`, `POST /api/ui/users`, `PUT /api/ui/users/{id}`, `DELETE /api/ui/users/{id}`. Response user: `{id,name,listenbrainz_user,token_set,plex_user,enabled}`. `PUT` token field empty means keep. `DELETE` removes the record and that user's cache dir only.
- Handler holds a mutex around read-modify-write of `users.json`.

- [ ] **Step 1: Write failing tests** (httptest, temp `dataDir`):
  - `TestUsersAPI_CreateListUpdateDelete`.
  - `TestUsersAPI_TokenNeverReturned`: create with token `secret123`; raw bodies of GET/POST/PUT contain `token_set` but never `secret123`.
  - `TestUsersAPI_PutEmptyTokenKeepsToken`.
  - `TestUsersAPI_ValidationErrors`: 400 for bad id source name, duplicate, missing fields.
  - `TestUsersAPI_CorruptFileIs500NotEmpty`: corrupt `users.json` returns 500, never an empty list.
  - `TestUsersAPI_DeleteLastUser`: afterwards `GET` returns `[]`.
  - `TestRedact_ListenBrainzToken`: a log line containing `LISTENBRAINZ_USER_TOKEN=abc` is redacted (follow the existing patterns in `redact_test.go`).
- [ ] **Step 2: Run** `go test ./src/web/backend/users/ ./src/logging/ -v`; expect FAIL.
- [ ] **Step 3: Implement** handlers, routes, server wiring, and the redaction pattern.
- [ ] **Step 4: Run** `go test -race ./src/web/... ./src/logging/...`; expect PASS.
- [ ] **Step 5: Commit** `feat: add users API`.

### Task 4: `--user` flag and config overlay

**Files:**
- Modify: `src/config/flags.go` (flag `--user`, default empty; stored in `cfg.Flags.User`)
- Create: `src/config/user_overlay.go`
- Test: `src/config/user_overlay_test.go`

**Interfaces:**
- Consumes: `users.Load`, shared-playlists `ClientConfig.ShareWith` / `OwnerName`.
- Produces: `func (cfg *Config) ApplyUser(id string) error` — loads users from `cfg.ServerCfg.WebDataDir`; unknown id, corrupt file, or disabled user returns an error. Sets `DiscoveryCfg.Listenbrainz.User` and `.UserToken`, `ClientCfg.Creds.User`, `ClientCfg.OwnerName` (display name), `ClientCfg.ShareWith` (the other enabled users' `PlexUser`, ignoring `PLAYLIST_SHARE_WITH`), and `cfg.UserID` (new field, used for cache paths). Touches nothing else.
- To avoid an import cycle with `users` (which may import `config`), put the file format struct in a small shared package or have `ApplyUser` take `[]users.User`; builder's call, tests below define the behaviour.

- [ ] **Step 1: Write failing tests**:
  - `TestApplyUser_OverlaysOnlyUserFields`: other env-derived fields unchanged.
  - `TestApplyUser_ComputesShareList`: three users, one disabled → share list is the other *enabled* user's Plex user only; typed `PLAYLIST_SHARE_WITH` ignored.
  - `TestApplyUser_UnknownDisabledAndCorrupt`: each returns an error and leaves cfg untouched.
- [ ] **Step 2: Run** `go test ./src/config/ -run ApplyUser -v`; expect FAIL.
- [ ] **Step 3: Implement** the flag, `Flags.User`, `cfg.UserID`, and `ApplyUser`; call it from startup in `src/main/main.go` right after config load when `Flags.User` is set and not `all`.
- [ ] **Step 4: Run** `go test ./src/config/... && go build ./...`; expect PASS.
- [ ] **Step 5: Commit** `feat: add --user flag with config overlay`.

### Task 5: `--user all` loop

**Files:**
- Create: `src/main/all_users.go`
- Modify: `src/main/main.go` (dispatch when `Flags.User == "all"`; pass `cfg.UserID` to `WritePlaylistCache`)
- Test: `src/main/all_users_test.go`

**Interfaces:**
- Consumes: Task 1 `Load`/`Enabled`, Task 4 flag.
- Produces: `func runAllUsers(dataDir, playlist string, args []string, run func(args []string) error) error` — applies the Global Constraints rules for `--user all`. `run` is injectable; production runs `exec.Command(os.Args[0], args...)` with stdout/stderr inherited and `--user <id>` replacing `--user all`. Returns an aggregate error naming failed users; `nil` when every enabled user succeeded or when the single-user fallback ran.
- Custom playlists: when `playlist` starts with `custom-`, look up its `user_id` (Task 7) and run only that user; before Task 7 lands, unowned custom playlists run for the first enabled user.

- [ ] **Step 1: Write failing tests** with a fake `run` recording args:
  - `TestRunAllUsers_NoUsersFileRunsSingleUser`: fake called once with no `--user`.
  - `TestRunAllUsers_RunsEnabledInOrderSkipsDisabled`.
  - `TestRunAllUsers_OneFailureContinuesAndReports`: first user's fake errors; second still runs; returned error names the first.
  - `TestRunAllUsers_CorruptFileRunsNothing`.
  - `TestRunAllUsers_AllDisabledRunsNothing`.
  - `TestRunAllUsers_MissingTokenUserSkippedWithWarning`: a user with an empty ListenBrainz token is not run.
- [ ] **Step 2: Run** `go test ./src/main/ -run RunAllUsers -v`; expect FAIL.
- [ ] **Step 3: Implement** `runAllUsers` and the dispatch; add `user` to the `slog` default attributes when `--user <id>` is active so log lines carry it.
- [ ] **Step 4: Run** `go build ./... && go test -race ./src/main/...`; expect PASS.
- [ ] **Step 5: Commit** `feat: add --user all multi-user run loop`.

### Task 6: Custom playlists belong to a user

**Files:**
- Modify: `src/web/backend/playlist/custom_playlists.go` (`CustomPlaylist.UserID` with JSON `user_id,omitempty`), `src/web/backend/playlist/handlers.go` (import requires `user_id` when users exist; new `POST /api/ui/custom-playlists/{id}/user` to reassign), `src/web/backend/routes.go`, `src/main/all_users.go` (lookup)
- Test: `src/web/backend/playlist/custom_playlists_user_test.go`

**Interfaces:**
- Consumes: Task 1 `Load`.
- Produces: `func ownerOf(dataDir, playlistID string) (userID string)`; on first multi-user load, entries without `user_id` are treated as belonging to the first user (assigned lazily on next save, no migration job).

- [ ] **Step 1: Write failing tests**: `TestImport_RequiresUserWhenUsersExist` (400 without it; 200 with a valid id; unknown id 400); `TestImport_NoUsersFileStillWorksWithoutUser`; `TestReassign_ChangesOwner`; `TestOwnerOf_LegacyEntryBelongsToFirstUser`; `TestDeleteUser_ReassignsOrphanedPlaylistsToFirstRemainingUser` (extends the Task 3 delete path, which now calls into this).
- [ ] **Step 2: Run**; expect FAIL.
- [ ] **Step 3: Implement**; wire `runAllUsers` to use `ownerOf`; update Task 3's delete to reassign.
- [ ] **Step 4: Run** `go test -race ./src/web/backend/... ./src/main/...`; expect PASS.
- [ ] **Step 5: Commit** `feat: assign custom playlists to a user`.

### Task 7: Manual run, cron, and playlist API accept a user

**Files:**
- Modify: `src/web/backend/run/handlers.go` + `manual_run.go` (optional `user` in the run request → `--user`), `src/web/backend/playlist/handlers.go` (`HandleGetPlaylist`, `HandlePrefetchCovers` accept optional `user`, validated with `users.IDRe`, using `CacheDir`), `docker/start.sh` (append `--user all` to per-playlist and legacy cron flags), `docker/explo-run_test.sh` if it asserts command lines
- Test: `src/web/backend/run/*_test.go`, `src/web/backend/playlist/*_test.go`

**Interfaces:**
- Produces: `POST /api/ui/run` body gains `user` (`""`, `"all"`, or an id); `""` keeps today's args exactly. `GET /api/ui/playlists?type=…&user=<id>` reads that user's cache; invalid id → 400; missing `user` → legacy path.

- [ ] **Step 1: Write failing tests**: `TestRunArgs_NoUserUnchanged`; `TestRunArgs_UserAndAll`; `TestRun_InvalidUserRejected` (`../x` → 400, no process started); `TestGetPlaylist_UserCacheAndInvalidUser`; `TestPrefetch_UsesThatUsersListenBrainzName`.
- [ ] **Step 2: Run**; expect FAIL.
- [ ] **Step 3: Implement**; keep the single manual-run guard as is. In `start.sh`, append the flag so lines read `... ./explo --config "$_cfg" $flags --user all`.
- [ ] **Step 4: Run** `go test -race ./src/web/... && sh docker/explo-run_test.sh` (if runnable); expect PASS.
- [ ] **Step 5: Commit** `feat: pass user through manual run, cron, and playlist API`.

### Task 8: Frontend — Users tab

**Files:**
- Create: `src/web/frontend/src/components/Users.jsx`
- Modify: `src/web/frontend/src/lib/api.js` (`fetchUsers, createUser, updateUser, deleteUser`), `src/web/frontend/src/components/Settings.jsx` (tab button between Home and Settings; render `<Users />`)

**Interfaces:**
- Consumes: Task 3 routes. Design reference: the approved demo at `scratchpad/multiuser-demo.html` (Users tab and modal), restyled with the app's existing classes (`Button`, `TextField`, `Toggle`, surface cards).

- [ ] **Step 1: Implement** list cards (name, id badge, ListenBrainz user, "token set", Plex user, "copies shared with…" line), enable toggle, Add/Edit modal (token write-only, "leave blank to keep"), delete confirmation, missing-admin-credentials banner (read from existing config state), and an empty-state note.
- [ ] **Step 2: Run** `npm run build` in `src/web/frontend`; expect success.
- [ ] **Step 3: Manual check** against a running backend: add, edit, disable, delete a user; API responses contain no token.
- [ ] **Step 4: Rebuild `src/web/dist` only if git history shows `dist` is committed with frontend changes** (check `git log --stat -- src/web/dist | head`); otherwise leave it.
- [ ] **Step 5: Commit** `feat: add Users tab`.

### Task 9: Frontend — Home, Settings, wizard, logs

**Files:**
- Modify: `src/web/frontend/src/components/Settings.jsx` (Home: "Viewing" pills beside the Scheduled Playlists title, User select in Manual Run, user passed to `startRun` and to `fetchPlaylistTracks`; Settings: "Managed in Users" note; Logs: user filter), `src/web/frontend/src/components/ui/PlaylistCard.jsx` (custom cards accept a `tag` override for the owner name), `src/web/frontend/src/components/ui/ImportModal.jsx` (required "For user" select when users exist), `src/web/frontend/src/components/Wizard.jsx` (read-only "Managed in Users" for LB user/token and Plex user when users exist), `src/web/frontend/src/lib/api.js`, `src/web/frontend/src/lib/listenbrainz.js` (user-aware cache key)

**Interfaces:**
- Consumes: Task 3 users list, Task 6 custom playlist `user_id` and reassign route, Task 7 run/playlist `user` parameters. Every addition renders only when at least one user exists, so single-user Home is byte-for-byte today's layout.

- [ ] **Step 1: Implement the Home additions, Import modal field, and Reassign action (card menu).**
- [ ] **Step 2: Implement the Settings, Wizard, and Logs changes.** Log lines carry the user from the `user` attribute added in Task 5.
- [ ] **Step 3: Run** `npm run build`; expect success.
- [ ] **Step 4: Manual check** with zero users (UI identical to today) and with two users (switcher changes the track list; Manual Run "All users" starts a run; import without a user is refused).
- [ ] **Step 5: Commit** `feat: multi-user controls on Home, Settings, wizard, and logs`.

### Task 10: Pre-fill first user, final verification

**Files:**
- Modify: `src/web/frontend/src/components/Users.jsx` (pre-fill the first user's form from existing `LISTENBRAINZ_USER`, `SYSTEM_USERNAME`, token shown as "set"), `README.md` or the wiki-adjacent doc this repo keeps (one short section on users)

- [ ] **Step 1: Implement the pre-fill** and a one-paragraph docs note.
- [ ] **Step 2: Run** `go vet ./... && golangci-lint run ./... && go test -race ./... && (cd src/web/frontend && npm run build)`; expect no issues. Report any skipped tests.
- [ ] **Step 3: Manual test on the real server**: two users (two real Plex accounts, two ListenBrainz accounts, admin credentials set). Run "All users" once. Confirm each account has its own playlist plus the other's titled copy, tracks play, the second run replaces instead of duplicating, and a wrong token for one user does not stop the other.
- [ ] **Step 4: Commit** `feat: pre-fill first user and document multi-user`.
