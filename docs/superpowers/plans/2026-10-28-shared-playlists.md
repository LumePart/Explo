# Shared Plex Playlists Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** After each run, copy the owner's Plex playlist into the accounts of the users on a share list, titled `<Owner>'s <PlaylistName>`.

**Architecture:** An optional `PlaylistSharer` interface (like `ArtworkUploader`) implemented only by Plex. `Plex.SharePlaylist` switches to each recipient with the existing admin `SwitchUser`, creates the copy first, then deletes the recipient's old copy of the same exact title. Pure helpers (title, share-list parsing) live in a new `src/client/share.go`.

**Tech Stack:** Go, `net/http/httptest` for Plex tests, cleanenv struct tags for config.

**Spec:** `docs/superpowers/specs/2026-10-28-shared-playlists-design.md`

## Global Constraints

- Env vars: `PLAYLIST_SHARE_WITH` (comma-separated Plex usernames, empty disables) and `PLAYLIST_OWNER_NAME` (defaults to `Creds.User`).
- Copy title is exactly `<OwnerName>'s <ClientCfg.PlaylistName>` with a straight apostrophe.
- Feature unconfigured means behaviour identical to today.
- A sharing problem must never fail the run, block the owner's playlist, or block other recipients.
- Create the new copy before deleting the old one. Delete only an exact title match in the recipient's account.
- Plex only. No custom-playlist artwork on copies.
- Sharing needs admin credentials (`ADMIN_API_KEY`, or `ADMIN_SYSTEM_USERNAME` + `ADMIN_SYSTEM_PASSWORD`); without them log one warning and stop.
- Pre-merge checks: `go vet ./...`, `golangci-lint run ./...`, `go test -race ./...`. Some sandboxes block localhost sockets; record that instead of claiming a pass.

## Review Focus

- Recipient owns an unrelated playlist with a similar title (e.g. `Gabe's Weekly Exploration (old)`): it must survive. Pinned in Task 4.
- Share list `"kelly, ,Kelly,gabe"` where `gabe` is the instance's own user: blanks, case-duplicates and self dropped. Pinned in Task 2.
- Owner name empty or containing an apostrophe (`O'Brien`): title still sensible. Pinned in Task 2.
- Share list set but backend is not Plex (or Plex without admin credentials): one warning, no error, no request. Pinned in Task 4.
- One recipient's create fails or is unknown: the others still get their copy, and the failed one keeps its old copy. Pinned in Task 4.

## File Structure

- Create `src/client/share.go`: `ShareTitle`, `ParseShareList`.
- Create `src/client/share_test.go`, `src/client/plex_share_test.go`.
- Modify `src/client/plex.go`: extract create helper, add `SharePlaylist`, user-switch seam.
- Modify `src/client/client.go`: `PlaylistSharer` interface, `Client.SharePlaylist`.
- Modify `src/config/config.go`: two `ClientConfig` fields.
- Modify `src/main/main.go`: call sharing after owner's playlist.
- Modify `src/web/backend/defs/defs.go`, both `sample.env` files (`sample.env`, `src/web/sample.env`).
- Modify `src/web/backend/settings/handlers.go` (admin key bug, Task 1).

---

### Task 1: Fix wizard saving the admin API key under the wrong name

Found while planning: `HandleWizardStep2` writes `ADMIN_SYSTEM_APIKEY` but config reads `ADMIN_API_KEY`, so an admin key saved through the web UI is ignored and sharing could never work with it.

**Files:**
- Modify: `src/web/backend/settings/handlers.go:495`
- Test: `src/web/backend/settings/wizard_test.go` (follow its existing style; add a test there)

**Interfaces:**
- Produces: wizard step 2 persists the admin key as `ADMIN_API_KEY`.

- [ ] **Step 1: Write failing test `TestHandleWizardStep2_WritesAdminAPIKeyUnderConfigName`**: POST step-2 body with `admin_api_key: "k123"`; assert the saved env contains `ADMIN_API_KEY=k123` and does not contain `ADMIN_SYSTEM_APIKEY`.
- [ ] **Step 2: Run** `go test ./src/web/backend/settings/ -run AdminAPIKey -v`; expect FAIL.
- [ ] **Step 3: Change the key** in the `updates` map at `handlers.go:495` to `"ADMIN_API_KEY"`.
- [ ] **Step 4: Run the test and the whole package**; expect PASS.
- [ ] **Step 5: Commit** `fix: save wizard admin API key as ADMIN_API_KEY`.

### Task 2: Config fields and pure share helpers

**Files:**
- Modify: `src/config/config.go` (`ClientConfig`, near `PublicPlaylist`)
- Create: `src/client/share.go`
- Test: `src/client/share_test.go`

**Interfaces:**
- Produces:
  - `ClientConfig.ShareWith []string` with tag `env:"PLAYLIST_SHARE_WITH"`
  - `ClientConfig.OwnerName string` with tag `env:"PLAYLIST_OWNER_NAME"`
  - `func ShareTitle(owner, fallbackOwner, playlistName string) string`
  - `func ParseShareList(entries []string, self string) []string`

- [ ] **Step 1: Write failing tests** in `share_test.go`:
  - `TestShareTitle`: `("Gabe","", "Weekly Exploration")` → `Gabe's Weekly Exploration`; `("", "gabe_plex", "Weekly Exploration")` → `gabe_plex's Weekly Exploration`; `("O'Brien","", "X")` → `O'Brien's X`; both owners empty → `Shared X` is NOT used: return `Weekly Exploration` unchanged-prefix-less title `Shared Weekly Exploration`? No: assert it returns `playlistName` with no prefix when no owner is known.
  - `TestParseShareList`: `[]string{"kelly"," ","Kelly","gabe"," Sam "}` with self `"Gabe"` → `[]string{"kelly","Sam"}` (trimmed, blanks dropped, case-insensitive dedupe keeping first spelling, self dropped case-insensitively, order kept).
- [ ] **Step 2: Run** `go test ./src/client/ -run 'ShareTitle|ParseShareList' -v`; expect FAIL (undefined).
- [ ] **Step 3: Add the config fields and implement both functions** in `share.go`.
- [ ] **Step 4: Run tests**; expect PASS. Also `go build ./...`.
- [ ] **Step 5: Commit** `feat: add share-list config and title helpers`.

### Task 3: Extract the playlist-create helper from `Plex.CreatePlaylist`

Behaviour of the owner's path must not change.

**Files:**
- Modify: `src/client/plex.go:501-556`
- Test: `src/client/plex_share_test.go`

**Interfaces:**
- Consumes: `testConfig`, `recordingServer` patterns from `delete_playlist_test.go` (reuse or copy the helper; extend the fake server so `POST /playlists` returns `{"MediaContainer":{"Metadata":[{"ratingKey":"99"}]}}`).
- Produces: `func (c *Plex) createPlaylistAs(userClient *Plex, title string, tracks []*models.Track) (string, error)` returning the new playlist's rating key. `CreatePlaylist` calls it with `userClient.Cfg.PlaylistName`.

- [ ] **Step 1: Write characterization test `TestPlexCreatePlaylist_PostsTitleAndAddsTracks`** against the current code: no `AdminClient`, set `machineID` and `LibraryID`; two tracks with IDs `/library/metadata/1` and `/library/metadata/2`; assert one `POST /playlists` with `title=Weekly Exploration` and two `PUT /playlists/99/items`, and `Cfg.PlaylistID == "99"`.
- [ ] **Step 2: Run it**; expect PASS (it pins current behaviour).
- [ ] **Step 3: Extract `createPlaylistAs`**; `CreatePlaylist` keeps its user-switching and sets `c.Cfg.PlaylistID` from the returned key.
- [ ] **Step 4: Run the new test and `go test ./src/client/...`**; expect PASS.
- [ ] **Step 5: Commit** `refactor: extract Plex playlist creation helper`.

### Task 4: `PlaylistSharer` and `Plex.SharePlaylist`

**Files:**
- Modify: `src/client/client.go` (interface next to `ArtworkUploader`, plus `Client.SharePlaylist`)
- Modify: `src/client/plex.go`
- Test: `src/client/plex_share_test.go`

**Interfaces:**
- Consumes: `ShareTitle`, `ParseShareList`, `createPlaylistAs`.
- Produces:
  - `type PlaylistSharer interface { SharePlaylist(tracks []*models.Track, owner string, users []string, replace bool) error }`
  - `func (c *Client) SharePlaylist(tracks []*models.Track) error` — reads `ShareWith`/`OwnerName`/`ReplacePlaylist` from the client's config, type-asserts `c.API` to `PlaylistSharer`; no-op when the list is empty; one warning and nil when the backend is not a sharer.
  - On `Plex`: field `userSwitcher func(username string) (*Plex, error)`; helper `switchTo(name)` uses it when set, else `c.AdminClient.SwitchUser`. Tests inject it so no plex.tv call is made.
  - `func (c *Plex) SharePlaylist(...) error` — per recipient: switch user; if `replace`, `GET /playlists` as the recipient and remember the rating key of the exact-title match; `createPlaylistAs`; only then `DELETE` the remembered key; apply `PublicPlaylist` and description as the owner path does. Skips tracks with empty `ID`; if none have IDs, log and return nil. Per-recipient failures are logged with `slog.Warn` and collected into a returned `errors.Join`; they never stop the loop. `c.AdminClient == nil`: one warning, return nil, no requests.

- [ ] **Step 1: Write failing tests** (each uses a fake server per recipient and an injected `userSwitcher`):
  - `TestSharePlaylist_CreatesCopyThenDeletesOld`: recipient has `Gabe's Weekly Exploration` (key `7`); assert request order is POST create, then DELETE `/playlists/7`.
  - `TestSharePlaylist_KeepsUnrelatedPlaylists`: recipient also has `Gabe's Weekly Exploration (old)` (key `8`) and `Mix` (key `9`); assert only key `7` is deleted.
  - `TestSharePlaylist_ReplaceOffDeletesNothing`: `replace=false` sends no DELETE and no `GET /playlists`.
  - `TestSharePlaylist_CreateFailureKeepsOldAndContinues`: first recipient's POST returns 500, second succeeds; assert no DELETE for the first, second gets its copy, returned error mentions the first user.
  - `TestSharePlaylist_UnknownUserSkipped`: switcher errors for one name; others still processed.
  - `TestSharePlaylist_NoAdminClient`: `AdminClient == nil` and no switcher; zero requests, nil error.
  - `TestClientSharePlaylist_NonSharerBackend`: a stub `APIClient` without `SharePlaylist` and non-empty `ShareWith`; returns nil, no panic.
- [ ] **Step 2: Run** `go test ./src/client/ -run 'SharePlaylist' -v`; expect FAIL (undefined).
- [ ] **Step 3: Implement** the interface, `Client.SharePlaylist`, the switch seam, and `Plex.SharePlaylist`.
- [ ] **Step 4: Run** `go test -race ./src/client/...`; expect PASS.
- [ ] **Step 5: Commit** `feat: share Plex playlists with other users`.

### Task 5: Wire sharing into the run

**Files:**
- Modify: `src/main/main.go` (inside the `else` after `playlist created successfully`, after `uploadCustomPlaylistArtwork`)

**Interfaces:**
- Consumes: `(*client.Client).SharePlaylist(tracks []*models.Track) error`.

- [ ] **Step 1: Call** `client.SharePlaylist(tracks)` there; on error `slog.Warn(err.Error())` (no `notify`, no exit). It must not run when the owner's `CreatePlaylist` failed.
- [ ] **Step 2: Run** `go build ./... && go vet ./...`; expect clean (no committed tests cover `src/main`, as with BUG-34).
- [ ] **Step 3: Commit** `feat: share playlist after creation`.

### Task 6: Settings UI and sample env

**Files:**
- Modify: `src/web/backend/defs/defs.go` (new defs after `PUBLIC_PLAYLIST` at ~line 85; add both keys to `AllConfigKeys` ~line 193)
- Modify: `sample.env`, `src/web/sample.env`
- Test: an existing defs/settings test if one asserts `AllConfigKeys` coverage; otherwise a small test that both keys appear in `AllConfigKeys`.

**Interfaces:**
- Produces: two settings entries, `Section: "system"`, `VisibleWhen: {Field: "EXPLO_SYSTEM", Eq: "plex"}`. Labels: "Share playlists with (Plex usernames)", "Your name on shared playlists". Hints: comma-separated usernames, needs admin credentials; name used as the title prefix.

- [ ] **Step 1: Write failing test** that both keys are in `AllConfigKeys` and have a defs entry; run, expect FAIL.
- [ ] **Step 2: Add the defs, keys, and commented `sample.env` entries** (`# PLAYLIST_SHARE_WITH=`, `# PLAYLIST_OWNER_NAME=`) to both sample files.
- [ ] **Step 3: Check whether the settings form is generated from defs.** If yes, no frontend change. If a field needs frontend code, add it in `src/web/frontend`, and rebuild `src/web/dist` only if git history shows `dist` is committed alongside frontend changes.
- [ ] **Step 4: Run** `go test ./src/web/...`; expect PASS.
- [ ] **Step 5: Commit** `feat: add share-playlist settings`.

### Task 7: Final verification

- [ ] **Step 1:** `go vet ./... && golangci-lint run ./... && go test -race ./...`; expect no issues. Report any failures, and any tests skipped because localhost sockets were blocked.
- [ ] **Step 2: Manual test on the real server** (cannot be automated): with admin credentials set, set `PLAYLIST_SHARE_WITH` to the other user and run once with `--replace`-style config. Confirm the owner sees `Weekly Exploration`, the recipient sees `<Owner>'s Weekly Exploration`, tracks play, and a second run replaces rather than duplicates. Repeat from Kelly's instance.
- [ ] **Step 3:** Update `TODO.md` is not needed (this is a feature, not a tracked bug).
