# Multi-User Instance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One Explo instance serves several people, each with their own ListenBrainz account and Plex account, managed by one admin in the web UI.

**Architecture:** A `users.json` store managed through a new Users API. A `--user <id>` flag overlays one user's details on the env config; `--user all` loops over enabled users as sequential child processes. Playlist caches and custom playlists become per user. Frontend adds a Users tab and small additions to Home, Settings and the wizard.

**Tech Stack:** Go (stdlib `net/http`, `slog`), React + Vite + Tailwind in `src/web/frontend`.

**Spec:** `docs/superpowers/specs/2026-10-28-multi-user-design.md`. **Depends on** the shared-playlists work (`docs/superpowers/plans/2026-10-28-shared-playlists.md`): this branch is stacked on it and uses its `PLAYLIST_SHARE_WITH` / `PLAYLIST_OWNER_NAME` config and `SharePlaylist`.

## Global Constraints

- Plex only. Single-user installs (no `users.json`, or an empty one) behave exactly as today: same command lines, same cache paths, same env keys.
- `users.json` lives in `WebDataDir`, mode 0600, written atomically.
- User `id`: slug of the display name (algorithm pinned in Task 1), unique, immutable, validated against `^[a-z0-9][a-z0-9-]{0,31}$` before it is ever used in a path.
- Cache layout: `<WebDataDir>/cache/users/<id>/<type>.json`. The `users/` level keeps ids from colliding with `cache/covers`, `cache/playlist_artwork`, or playlist-type files.
- Token is optional when creating or editing a user and required to run: a user with an empty token is skipped with a warning, and that skip does not count toward the non-zero exit code.
- `users.json` has one writer, the web server process, through `users.Store`. Explo child processes and config loading only read it.
- Plex playlists are never created, deleted, or modified by managing users in the UI.
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
- A user with no ListenBrainz token: skipped with a warning while the others still run, exit code unaffected. Pinned in Task 5. (A Plex user missing on the server surfaces through the existing "failed to switch user" warning in `CreatePlaylist`; this plan adds no separate lookup or test for it.)

## File Structure

- Create `src/web/backend/users/` (`users.go` model, `Store`, validation; `handlers.go` API) with tests. The package imports only the standard library, so `config` and `main` can import it without a cycle.
- Modify `src/config/flags.go`, `src/config/config.go`: `--user` flag and overlay (`src/config/user_overlay.go` new).
- Create `src/main/all_users.go`: the `--user all` loop.
- Modify `src/main/main.go`, `src/web/backend/playlist/*` (cache paths, custom `user_id`), `src/web/backend/run/*` (manual run user), `src/web/backend/routes.go`, `src/web/backend/server.go`, `src/web/backend/settings/handlers.go`, `src/logging/redact.go`, `docker/start.sh`.
- Frontend: `src/web/frontend/src/lib/api.js`, `lib/listenbrainz.js`, new `components/Users.jsx`, modify `components/Settings.jsx`, `components/ui/ImportModal.jsx`, `components/ui/PlaylistCard.jsx` (tag only), `components/Wizard.jsx`.
- Prototype reference (throwaway, not shipped): `docs/superpowers/prototypes/multiuser-demo.html`.
- Docs: `README.md`.

---

### Task 1: User model, validation, and file store

**Files:**
- Create: `src/web/backend/users/users.go`
- Test: `src/web/backend/users/users_test.go`

**Interfaces:**
- Produces:
  - `type User struct { ID, Name, ListenBrainzUser, ListenBrainzToken, PlexUser string; Enabled bool }` with JSON tags `id,name,listenbrainz_user,listenbrainz_token,plex_user,enabled`
  - `var IDRe = regexp.MustCompile("^[a-z0-9][a-z0-9-]{0,31}$")`
  - `func Slug(name string) string`: lowercase; keep `a-z0-9`; map spaces, underscores and hyphens to `-`; drop everything else (including non-ASCII); collapse repeated `-`; trim leading and trailing `-`; truncate to 32 characters and trim a trailing `-`. So `Slug("A")` is `"a"` (valid) while a raw id `"A"` fails `IDRe`.
  - `func Load(dataDir string) ([]User, error)`: read-only; missing file returns `(nil, nil)`; unreadable or invalid JSON returns an error. Order is file order.
  - `type Store struct{ /* dir + mutex */ }`, `func NewStore(dataDir string) *Store`, `func (s *Store) Update(fn func([]User) ([]User, error)) error`: the only writer; holds the mutex across load, `fn`, atomic save (temp file in same dir, rename, mode 0600).
  - `func Validate(u User, existing []User, editing bool) error`: name, ListenBrainz user, Plex user non-empty; `u.ID` matches `IDRe`; id not already in `existing` unless `editing`. Token is not required.
  - `func Enabled(users []User) []User`: preserves file order.

- [ ] **Step 1: Write failing tests**:
  - `TestSlug`: `"Kelly W."`→`kelly-w`; `"  "`→`""`; `"Ünï"`→`n`; `"A"`→`a`; `"a__b  c"`→`a-b-c`; 40 `x` characters → 32 `x`.
  - `TestValidate_RejectsBadIDs`: `../x`, `A`, `""`, 33 characters, `-a` rejected; empty `Slug` result (name `"!!!"`) rejected; duplicate id on create (`Kelly` then `kelly!`) rejected; same id allowed when `editing`.
  - `TestValidate_TokenOptional`: empty token passes.
  - `TestStoreUpdate_RoundTripAnd0600`: two updates, `Load` equal and in order, file mode `0600`, no temp file left.
  - `TestStoreUpdate_ConcurrentUpdatesAllSurvive` (race detector on).
  - `TestLoad_MissingIsEmptyCorruptIsError`.
- [ ] **Step 2: Run** `go test ./src/web/backend/users/ -v`; expect FAIL (undefined).
- [ ] **Step 3: Implement** following the atomic-write approach in `src/web/backend/playlist/custom_playlists.go`.
- [ ] **Step 4: Run tests with `-race`**; expect PASS.
- [ ] **Step 5: Commit** `feat: add multi-user store and validation`.

### Task 2: Per-user cache layout

Reads stay on the legacy path until Task 7, so between this task and Task 7 anything written for a user is not yet shown by the UI. That is acceptable: no user mode is reachable from the UI until Task 8.

**Files:**
- Modify: `src/web/backend/playlist/playlist.go` (`WritePlaylistCache`, `writePreliminaryCache`, `writePrefetchCache`, `writeTrackCache` and their cache-path joins)
- Create: `src/web/backend/playlist/cachepath.go`
- Test: `src/web/backend/playlist/cachepath_test.go`

**Interfaces:**
- Produces:
  - `func CacheDir(dataDir, userID string) (string, error)`: `dataDir/cache` when `userID == ""`; otherwise `dataDir/cache/users/<userID>`; an id failing `users.IDRe` returns an error and no path.
  - `func RemoveUserCache(dataDir, userID string) error`: removes only that user's directory; invalid id is an error; missing directory is not.
  - `WritePlaylistCache(dataDir, userID, playlist string, tracks []*models.Track, added map[string]bool)`: new `userID` parameter; every existing caller passes `""` until Task 5. A `CacheDir` error is logged and nothing is written.

- [ ] **Step 1: Write failing tests**: `TestCacheDir_EmptyUserIsLegacyPath`; `TestCacheDir_UserPath`; `TestCacheDir_InvalidIDErrors` (`../../etc`, `a/b`, `..`, `A`, 33 characters all error and return an empty path); `TestWritePlaylistCache_WritesUnderUserDir`; `TestRemoveUserCache_RemovesOnlyThatUser` (a sibling user and `cache/covers` survive).
- [ ] **Step 2: Run** `go test ./src/web/backend/playlist/ -run 'CacheDir|UserCache|WritePlaylistCache' -v`; expect FAIL.
- [ ] **Step 3: Implement** `CacheDir`/`RemoveUserCache`, thread `userID` through the cache writers, update all callers (`src/main/main.go` and playlist handlers) to pass `""`. Covers stay in the shared `cache/covers`.
- [ ] **Step 4: Run** `go build ./... && go test ./src/web/backend/playlist/...`; expect PASS, including existing tests.
- [ ] **Step 5: Commit** `refactor: make playlist cache paths per user`.

### Task 3: Users API

**Files:**
- Create: `src/web/backend/users/handlers.go`
- Modify: `src/web/backend/routes.go` (add `registerUserRoutes`), `src/web/backend/server.go` (construct the handler with a `*users.Store`), `src/logging/redact.go`
- Test: `src/web/backend/users/handlers_test.go`, `src/logging/redact_test.go`

**Interfaces:**
- Consumes: Task 1 `Store`/`Validate`/`Slug`, Task 2 `playlist.RemoveUserCache`.
- Produces routes (all behind `s.auth`): `GET /api/ui/users`, `POST /api/ui/users`, `PUT /api/ui/users/{id}`, `DELETE /api/ui/users/{id}`. Response user: `{id,name,listenbrainz_user,token_set,plex_user,enabled}`.
  - `POST`: body has no `id`; the server derives it with `Slug(name)`. A collision returns 400 `"a user with that id already exists"` (no auto-suffix), shown inline by the UI (`Kelly` then `kelly!` hits this).
  - `PUT`: the path id is authoritative and immutable; an `id` in the body is ignored. Name and other fields can change. An empty `listenbrainz_token` keeps the stored token.
  - `DELETE`: removes the user record and that user's cache directory only. It never touches Plex. Handling of custom playlists they owned is added in Task 6, which extends this handler and its test; until then deleting a user leaves any custom playlists untouched.
  - Every write goes through `Store.Update`.

- [ ] **Step 1: Write failing tests** (httptest, temp `dataDir`):
  - `TestUsersAPI_CreateListUpdateDelete`.
  - `TestUsersAPI_TokenNeverReturned`: create with token `secret123`; raw bodies of GET/POST/PUT contain `token_set` but never `secret123`.
  - `TestUsersAPI_PutEmptyTokenKeepsToken`; `TestUsersAPI_PutIgnoresBodyID`.
  - `TestUsersAPI_ValidationErrors`: 400 for empty names, missing fields, unslugable name, duplicate slug (`Kelly` then `kelly!`); empty token is accepted.
  - `TestUsersAPI_CorruptFileIs500NotEmpty`.
  - `TestUsersAPI_DeleteRemovesCacheDirNotOthers`.
  - `TestUsersAPI_DeleteLastUser`: afterwards `GET` returns `[]`.
  - `TestRedact_ListenBrainzToken`: a log line containing `LISTENBRAINZ_USER_TOKEN=abc` is redacted (follow `redact_test.go` patterns).
- [ ] **Step 2: Run** `go test ./src/web/backend/users/ ./src/logging/ -v`; expect FAIL.
- [ ] **Step 3: Implement** handlers, routes, server wiring, and the redaction pattern.
- [ ] **Step 4: Run** `go test -race ./src/web/... ./src/logging/...`; expect PASS.
- [ ] **Step 5: Commit** `feat: add users API`.

### Task 4: `--user` flag and config overlay

**Files:**
- Modify: `src/config/flags.go` (flag `--user`, default empty, stored in `cfg.Flags.User`)
- Create: `src/config/user_overlay.go`
- Test: `src/config/user_overlay_test.go`

**Interfaces:**
- Consumes: `users.User` (the `users` package imports only the standard library, so `config` importing it creates no cycle), shared-playlists `ClientConfig.ShareWith` / `OwnerName`.
- Produces: `func (cfg *Config) ApplyUser(id string, all []users.User) error`: a pure overlay with no disk I/O; the caller loads users with `users.Load`. An unknown id, or a user that is not enabled, returns an error and leaves `cfg` untouched. Otherwise it sets:
  - `DiscoveryCfg.Listenbrainz.User` and `.UserToken`
  - `ClientCfg.Creds.User`
  - `ClientCfg.OwnerName` (display name)
  - `ClientCfg.ShareWith`: the other enabled users' `PlexUser` in file order; the typed `PLAYLIST_SHARE_WITH` is ignored
  - `cfg.UserID` (new field, used for cache paths)
  - and nothing else.
- Startup in `src/main/main.go`: when `Flags.User` is set and is not `all`, call `users.Load(cfg.ServerCfg.WebDataDir)` (error or empty list is fatal for `--user <id>`), then `ApplyUser`.

- [ ] **Step 1: Write failing tests** (pure, in-memory slices):
  - `TestApplyUser_OverlaysOnlyUserFields`: other env-derived fields unchanged.
  - `TestApplyUser_ComputesShareList`: three users, one disabled → share list is the other enabled user's Plex user only; typed share list ignored.
  - `TestApplyUser_UnknownAndDisabledErrorLeaveCfgUntouched`.
- [ ] **Step 2: Run** `go test ./src/config/ -run ApplyUser -v`; expect FAIL.
- [ ] **Step 3: Implement** the flag, `Flags.User`, `cfg.UserID`, `ApplyUser`, and the startup call.
- [ ] **Step 4: Run** `go test ./src/config/... && go build ./...`; expect PASS.
- [ ] **Step 5: Commit** `feat: add --user flag with config overlay`.

### Task 5: `--user all` loop

**Files:**
- Create: `src/main/all_users.go`
- Modify: `src/main/main.go` (dispatch when `Flags.User == "all"`; pass `cfg.UserID` to `WritePlaylistCache`)
- Test: `src/main/all_users_test.go`

**Interfaces:**
- Consumes: Task 1 `Load`/`Enabled`, Task 4 flag.
- Produces:
  - `func rewriteUserArg(args []string, id string) []string`: removes every `--user <v>` and `--user=<v>` occurrence (repeated flags included) and appends one `--user <id>`; other args keep their order.
  - `type UserRunError struct{ Failed []string }` (`Error()` lists the failed ids, file order).
  - `func runAllUsers(dataDir, playlist string, args []string, run func(args []string) error) error`: applies the Global Constraints rules. Users run in file order (the run order is the file order). `run` is injectable; production runs `exec.Command(os.Args[0], args...)` with stdout/stderr inherited. Returns `*UserRunError` when any child run failed, `nil` otherwise (including the single-user fallback and token-less skips).
  - Custom playlists: when `playlist` starts with `custom-`, the owner comes from Task 6's `ownerOf`; until Task 6 lands, run for the first enabled user.

- [ ] **Step 1: Write failing tests** with a fake `run` recording args:
  - `TestRewriteUserArg`: `--user all`, `--user=all`, `--user a --user b`, and no `--user` all yield exactly one `--user <id>` with other args intact.
  - `TestRunAllUsers_NoUsersFileRunsSingleUser`: fake called once with no `--user`.
  - `TestRunAllUsers_RunsEnabledInFileOrderSkipsDisabled`.
  - `TestRunAllUsers_OneFailureContinuesAndReports`: first user's fake errors; second still runs; error names the first.
  - `TestRunAllUsers_CorruptFileRunsNothing`.
  - `TestRunAllUsers_AllDisabledRunsNothing`.
  - `TestRunAllUsers_TokenlessUserSkippedWithoutFailing`: not run, warning logged, returns `nil` when the others succeed.
- [ ] **Step 2: Run** `go test ./src/main/ -run 'RunAllUsers|RewriteUserArg' -v`; expect FAIL.
- [ ] **Step 3: Implement** and add the `user` attribute to the default `slog` attributes when `--user <id>` is active so log lines carry it.
- [ ] **Step 4: Run** `go build ./... && go test -race ./src/main/...`; expect PASS.
- [ ] **Step 5: Commit** `feat: add --user all multi-user run loop`.

### Task 6: Custom playlists belong to a user

**Files:**
- Modify: `src/web/backend/playlist/custom_playlists.go` (`CustomPlaylist.UserID` with JSON `user_id,omitempty`), `src/web/backend/playlist/handlers.go` (import requires `user_id` when users exist; new `POST /api/ui/custom-playlists/{id}/user` to reassign), `src/web/backend/routes.go`, `src/main/all_users.go` (lookup)
- Test: `src/web/backend/playlist/custom_playlists_user_test.go`

**Interfaces:**
- Consumes: Task 1 `Load`.
- Produces: `func ownerOf(dataDir, playlistID string) (userID string)`; entries without `user_id` belong to the first user in file order (assigned lazily on next save, no migration job). Deleting a user extends the Task 3 handler: their custom playlists are reassigned to the first remaining user; if none remains (last user deleted) their `user_id` is cleared and the instance is back in single-user mode, where `user_id` is ignored. Plex playlists are never touched.

- [ ] **Step 1: Write failing tests**: `TestImport_RequiresUserWhenUsersExist` (400 without it; 200 with a valid id; unknown id 400); `TestImport_NoUsersFileStillWorksWithoutUser`; `TestReassign_ChangesOwner`; `TestOwnerOf_LegacyEntryBelongsToFirstUser`; `TestDeleteUser_ReassignsOrphanedPlaylistsToFirstRemainingUser` and `TestDeleteLastUser_ClearsOwnerAndKeepsPlaylists` (these extend the Task 3 delete handler; its Task 3 test still passes unchanged because it has no custom playlists).
- [ ] **Step 2: Run**; expect FAIL.
- [ ] **Step 3: Implement**; wire `runAllUsers` to use `ownerOf`; update Task 3's delete to reassign.
- [ ] **Step 4: Run** `go test -race ./src/web/backend/... ./src/main/...`; expect PASS.
- [ ] **Step 5: Commit** `feat: assign custom playlists to a user`.

### Task 7: Manual run, cron, and playlist API accept a user

**Files:**
- Modify: `src/web/backend/run/handlers.go` + `manual_run.go` (optional `user` in the run request → `--user`), `src/web/backend/playlist/handlers.go` (`HandleGetPlaylist`, `HandlePrefetchCovers` accept optional `user`, validated with `users.IDRe`, using `CacheDir`), `docker/start.sh` (append `--user all` at all three explo invocations in the cron/start section: the legacy cron line (line 69), the per-playlist cron `cmd` (line 94), and the `EXECUTE_ON_START` run (line 108); not line 41, which starts the web server), `docker/explo-run_test.sh` if it asserts command lines
- Test: `src/web/backend/run/*_test.go`, `src/web/backend/playlist/*_test.go`

**Interfaces:**
- Produces: `POST /api/ui/run` body gains `user` (`""`, `"all"`, or an id); `""` keeps today's args exactly. `GET /api/ui/playlists?type=…&user=<id>` reads that user's cache; invalid id → 400; missing `user` → legacy path.

- [ ] **Step 1: Write failing tests**: `TestRunArgs_NoUserUnchanged`; `TestRunArgs_UserAndAll`; `TestRun_InvalidUserRejected` (`../x` → 400, no process started); `TestGetPlaylist_UserCacheAndInvalidUser`; `TestPrefetch_UsesThatUsersListenBrainzName`; and a shell assertion in `docker/explo-run_test.sh` style (or a new small test) that the generated lines at all three invocations contain `--user all`.
- [ ] **Step 2: Run**; expect FAIL.
- [ ] **Step 3: Implement**; keep the single manual-run guard as is. In `start.sh`, add `--user all` at the three invocations: `./explo --config \"$_cfg\" --user all` (legacy), `./explo --config \"$_cfg\" $flags --user all` (per-playlist), and `./explo --config "$_cfg" $START_FLAGS --user all` (start). Mind the escaped quotes in the echoed cron lines. Because `--user all` falls back to the single-user path when there is no `users.json`, this is safe for installs that never create users.
- [ ] **Step 4: Run** `go test -race ./src/web/... && sh docker/explo-run_test.sh` (if runnable); expect PASS.
- [ ] **Step 5: Commit** `feat: pass user through manual run, cron, and playlist API`.

### Task 8: Frontend — Users tab

**Files:**
- Create: `src/web/frontend/src/components/Users.jsx`
- Modify: `src/web/frontend/src/lib/api.js` (`fetchUsers, createUser, updateUser, deleteUser`), `src/web/frontend/src/components/Settings.jsx` (tab button between Home and Settings; render `<Users />`)

**Interfaces:**
- Consumes: Task 3 routes. Design reference: the approved prototype at `docs/superpowers/prototypes/multiuser-demo.html` (Users tab and modal), restyled with the app's existing classes (`Button`, `TextField`, `Toggle`, surface cards).

- [ ] **Step 1: Implement** list cards (name, id badge, ListenBrainz user, "token set", Plex user, "copies shared with…" line), enable toggle, Add/Edit modal (token write-only, "leave blank to keep"), delete confirmation, missing-admin-credentials banner (read from existing config state), and an empty-state note.
- [ ] **Step 2: Run** `npm run build` in `src/web/frontend`; expect success.
- [ ] **Step 3: Manual check** against a running backend: add, edit, disable, delete a user; API responses contain no token.
- [ ] **Step 4: Rebuild `src/web/dist` only if git history shows `dist` is committed with frontend changes** (check `git log --stat -- src/web/dist | head`); otherwise leave it.
- [ ] **Step 5: Commit** `feat: add Users tab`.

### Task 9: Frontend — Home user switcher and Manual Run user

**Files:**
- Modify: `src/web/frontend/src/components/Settings.jsx` (Home: "Viewing" pills beside the Scheduled Playlists title; User select next to Playlist in Manual Run, passed to `startRun`; the tracklist and card cover fetches pass the viewed user), `src/web/frontend/src/lib/api.js` (`startRun(playlist, dlmode, replace, user)`), `src/web/frontend/src/lib/listenbrainz.js`

**Interfaces:**
- Consumes: Task 3 users list, Task 7 run/playlist `user` parameters.
- Produces: `fetchPlaylistTracks(playlistType, { force, user })` and `clearPlaylistCache(playlistType, user)`; the in-memory cache key becomes `` `${user || ''}:${playlistType}` `` and requests add `&user=<id>` only when `user` is set. `startRun` sends `user` only when set.
- With no users, the page makes the same requests and renders the same output as today: no visible changes.

- [ ] **Step 1: Implement** the `listenbrainz.js`/`api.js` changes, then the pills and Manual Run select (both render only when at least one user exists).
- [ ] **Step 2: Run** `npm run build`; expect success. The repo has no JS test runner, so there is no committed test; verification is the build plus Step 3.
- [ ] **Step 3: Manual check** with zero users (Home unchanged, network requests have no `user` param) and with two (switching pills changes the track list; Manual Run "All users" starts a run).
- [ ] **Step 4: Commit** `feat: add user switcher and run user to Home`.

### Task 10: Frontend — custom playlists per user

**Files:**
- Modify: `src/web/frontend/src/components/ui/PlaylistCard.jsx` (accept a `tag` override for the label slot), `src/web/frontend/src/components/ui/ImportModal.jsx` (required "For user" select when users exist), `src/web/frontend/src/components/Settings.jsx` (custom playlists filtered to the viewed user; owner name passed as `tag`; Reassign in the card menu), `src/web/frontend/src/lib/api.js` (import sends `user_id`; `reassignCustomPlaylist(id, userId)`)

**Interfaces:**
- Consumes: Task 6 routes and `user_id`. With no users, the import form and cards are unchanged.

- [ ] **Step 1: Implement** the three changes.
- [ ] **Step 2: Run** `npm run build`; expect success.
- [ ] **Step 3: Manual check**: import without a user is refused; a new playlist shows under the chosen user only; Reassign moves it.
- [ ] **Step 4: Commit** `feat: assign custom playlists to users in the UI`.

### Task 11: Frontend — Settings, wizard, and logs

**Files:**
- Modify: `src/web/frontend/src/components/Settings.jsx` (Settings tab shows "Managed in Users" in place of the ListenBrainz fields when users exist; Logs tab user filter and `[Name]` prefix), `src/web/frontend/src/components/Wizard.jsx` (step 1 ListenBrainz username and token, and step 2 Plex user, render read-only "Managed in Users" when users exist; everything else stays editable)

**Interfaces:**
- Consumes: Task 3 users list; the `user` attribute added to log lines in Task 5. With no users, all three screens are unchanged.

- [ ] **Step 1: Implement** the three changes.
- [ ] **Step 2: Run** `npm run build`; expect success.
- [ ] **Step 3: Manual check** with zero and two users, including re-running the wizard with users present.
- [ ] **Step 4: Commit** `feat: show multi-user state in Settings, wizard, and logs`.

### Task 12: Pre-fill first user, docs, final verification

**Files:**
- Modify: `src/web/frontend/src/components/Users.jsx` (pre-fill the first user's form from existing `LISTENBRAINZ_USER`, `SYSTEM_USERNAME`, token shown as "set"), `README.md` (add a short "Multiple users" section before "Documentation": what it is, that users are added in the Users tab, that Plex admin credentials are required, and that single-user setups are unaffected)

- [ ] **Step 1: Implement the pre-fill** and a one-paragraph docs note.
- [ ] **Step 2: Run** `go vet ./... && golangci-lint run ./... && go test -race ./... && (cd src/web/frontend && npm run build)`; expect no issues. Report any skipped tests.
- [ ] **Step 3: Manual test on the real server**: two users (two real Plex accounts, two ListenBrainz accounts, admin credentials set). Run "All users" once. Confirm each account has its own playlist plus the other's titled copy, tracks play, the second run replaces instead of duplicating, and a wrong token for one user does not stop the other.
- [ ] **Step 4: Commit** `feat: pre-fill first user and document multi-user`.
