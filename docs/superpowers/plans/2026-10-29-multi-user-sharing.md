# Multi-user Playlist Sharing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** After a user's run creates their Plex playlist, copy it into the account of each user on that user's own share list, titled `<Display name>'s <PlaylistName>`.

**Architecture:** `users.User` gains `ShareWith` (user IDs). `ApplyUser` resolves those IDs to Plex usernames into the existing `ClientCfg.ShareWith`. Plex implements an optional `PlaylistSharer` interface (like `ArtworkUploader`); per recipient it switches to that account with the existing admin `SwitchUser`, creates the copy first, then deletes the old exact-title copy. The Users tab gets a "Share with" checkbox list.

**Tech Stack:** Go (`net/http/httptest` for Plex tests), React (Vite) frontend.

**Spec:** `docs/superpowers/specs/2026-10-29-multi-user-sharing-design.md` (also read `2026-10-28-shared-playlists-design.md` for copy semantics).

## Global Constraints

- Copy title is exactly `<OwnerName>'s <PlaylistName>`, straight apostrophe; `OwnerName` is `users.User.Name`.
- Default share list is empty. Empty list, or non-Plex backend, behaves exactly as today.
- Sharing problems never fail the run, block the owner's playlist, or block other recipients. Warn only (no `notify`, no exit).
- Create the new copy before deleting the old one. Delete only a recipient playlist whose title is an exact match.
- Every recipient request uses that recipient's token, never the instance token.
- Copies are never re-shared; only the owner's own run produces them.
- Plex only. No artwork on copies. No change to the owner-side replace/duplicate bug.
- Sharing needs admin credentials; without them log one warning, make no requests.
- Pre-merge: `go vet ./...`, `go test -race ./...`. Some sandboxes block localhost sockets; record that instead of claiming a pass.
- Already done, do not redo: the wizard saves the admin key as `ADMIN_API_KEY` (`src/web/backend/settings/handlers.go:537`); `ClientConfig.ShareWith`/`OwnerName` exist (`src/config/config.go:84-85`); `SwitchUser` supports the owner account.

## Review Focus

- A user toggled enabled/disabled in the UI keeps their share list (the toggle PUT must not wipe it). Pinned in Tasks 1 and 6.
- Share list names a user who is later deleted, disabled, or has no Plex name: dropped from recipients, no error. Pinned in Tasks 1 and 2.
- Recipient owns `Gabe's Weekly-Exploration (old)` and `Mix`: both survive. Pinned in Task 4.
- Recipient is the Plex server owner (not in shared_servers): still receives the copy. Pinned in Task 4.
- One recipient's create fails or is unknown: others still get theirs, and the failed one keeps its old copy. Pinned in Task 4.

## File Structure

- Modify `src/web/backend/users/users.go`, `handlers.go`: field, validation, API.
- Modify `src/config/user_overlay.go`: resolve share IDs.
- Create `src/client/share.go` (`ShareTitle`), `src/client/plex_share_test.go`, `src/client/share_test.go`.
- Modify `src/client/plex.go`: `createPlaylistAs`, `SharePlaylist`, user-switch seam.
- Modify `src/client/client.go`: `PlaylistSharer`, `Client.SharePlaylist`.
- Modify `src/main/main.go`: call after owner playlist.
- Modify `src/web/frontend/src/components/Users.jsx`.

---

### Task 1: `ShareWith` on users and the users API

**Files:**
- Modify: `src/web/backend/users/users.go` (struct at line 17, `Validate`)
- Modify: `src/web/backend/users/handlers.go`
- Test: `src/web/backend/users/handlers_test.go`, `users_test.go` (follow existing style; `newAPI`, `do`, `m`, `list` helpers exist)

**Interfaces:**
- Produces:
  - `User.ShareWith []string` with tag `json:"share_with"`; a stored file without the key loads as empty.
  - `userResponse.ShareWith []string` (`share_with`, never null: empty list serializes as `[]`).
  - `userRequest.ShareWith *[]string`: absent or null keeps the stored list on update; present replaces it. On create, absent means empty. (Builder-facing call: pointer so a PUT that omits it cannot wipe the list.)
  - `Validate(u, existing, editing)` additionally rejects a `ShareWith` entry that is the user's own ID or not an ID in `existing` (message names the entry); duplicates are collapsed, not rejected.
  - On delete, the deleted ID is removed from every remaining user's `ShareWith` within the same `store.Update`.

- [ ] **Step 1: Write failing tests**
  - `TestUsersAPI_ShareWithRoundTrip`: create `Kelly` and `Gabe`, update Gabe with `share_with: ["kelly"]`; `list` returns Gabe's `share_with == ["kelly"]` and Kelly's `== []`.
  - `TestUsersAPI_UpdateWithoutShareWithKeepsIt`: after the above, PUT Gabe with the body of `m("Gabe")` (no `share_with` key); list still shows `["kelly"]`.
  - `TestUsersAPI_ShareWithRejectsSelfAndUnknown`: PUT Gabe with `["gabe"]` and with `["nobody"]`; both return 400 and the stored list is unchanged.
  - `TestUsersAPI_DeleteRemovesFromShareLists`: delete Kelly; Gabe's `share_with == []`.
  - `TestLoad_MissingShareWithIsEmpty`: users.json written without the key loads with `len(ShareWith) == 0`.
- [ ] **Step 2: Run** `go test ./src/web/backend/users/ -v`; expect FAIL.
- [ ] **Step 3: Implement** the field, response/request changes, validation (sorted-stable de-dupe keeping first order), and delete cleanup as specified in Interfaces.
- [ ] **Step 4: Run** `go test ./src/web/backend/users/ ./src/web/...`; expect PASS.
- [ ] **Step 5: Commit** `feat: per-user share list in the users store and API`.

### Task 2: Resolve share lists in `ApplyUser`

**Files:**
- Modify: `src/config/user_overlay.go:29-42`
- Test: `src/config/user_overlay_test.go` (extend the existing tests)

**Interfaces:**
- Consumes: `users.User.ShareWith` (Task 1).
- Produces: `cfg.ClientCfg.ShareWith` = Plex usernames of the target's `ShareWith` IDs, in list order, skipping IDs that are unknown, disabled, equal to the target, or whose `PlexUser` is empty, and skipping case-insensitive duplicate Plex names or a name equal to the target's own `PlexUser`. `OwnerName` remains `target.Name`.

- [ ] **Step 1: Write failing tests**
  - `TestApplyUser_ShareWithResolvesIDs`: users gabe (`ShareWith: ["kelly","ghost"]`), kelly enabled `PlexUser: "kellyp"`; applying gabe gives `ShareWith == ["kellyp"]`.
  - `TestApplyUser_ShareWithDefaultsEmpty`: a user with no `ShareWith` gets an empty list even when other enabled users exist.
  - `TestApplyUser_ShareWithSkipsDisabledAndSelf`: disabled kelly and a self ID are dropped.
  - `TestApplyUser_ShareWithDropsSamePlexName`: gabe sharing with a user whose `PlexUser` equals gabe's (any case) yields an empty list.
- [ ] **Step 2: Run** `go test ./src/config/ -run ApplyUser -v`; expect FAIL on the new tests.
- [ ] **Step 3: Replace** the "everyone else" loop with the resolution above; update the comment.
- [ ] **Step 4: Run** `go test ./src/config/`; expect PASS.
- [ ] **Step 5: Commit** `feat: resolve per-user share lists instead of sharing with everyone`.

### Task 3: Copy-title helper and playlist-create extraction

**Files:**
- Create: `src/client/share.go`, `src/client/share_test.go`
- Modify: `src/client/plex.go:554-609`
- Test: `src/client/plex_share_test.go`

**Interfaces:**
- Produces:
  - `func ShareTitle(owner, playlistName string) string` → `<owner>'s <playlistName>`; empty/blank owner returns `playlistName` unchanged.
  - `func (c *Plex) createPlaylistAs(userClient *Plex, title string, tracks []*models.Track) (string, error)` — POSTs the playlist as `userClient` (its headers, URL, `machineID`, `c.LibraryID`), adds tracks via `userClient.addtoPlaylist`, returns the new rating key; sets `userClient.Cfg.PlaylistID`. `CreatePlaylist` calls it with `userClient.Cfg.PlaylistName` and still sets `c.Cfg.PlaylistID`.
- Reuse `testConfig`/`recordingServer` patterns from `delete_playlist_test.go`; the fake server for this file must answer `POST /playlists` with `{"MediaContainer":{"Metadata":[{"ratingKey":"99"}]}}`.

- [ ] **Step 1: Write tests**
  - `TestShareTitle`: `("Gabe","Weekly-Exploration")` → `Gabe's Weekly-Exploration`; `("O'Brien","X")` → `O'Brien's X`; `("  ","X")` → `X`; `("","X")` → `X`.
  - `TestPlexCreatePlaylist_PostsTitleAndAddsTracks` (characterization, no `AdminClient`): tracks with IDs `/library/metadata/1` and `/library/metadata/2`; assert one `POST /playlists` with `title=Weekly Exploration`, two `PUT /playlists/99/items`, and `Cfg.PlaylistID == "99"`.
- [ ] **Step 2: Run** the characterization test against current code; expect PASS. Run `TestShareTitle`; expect FAIL (undefined).
- [ ] **Step 3: Implement** `ShareTitle` and extract `createPlaylistAs`; owner path behavior unchanged.
- [ ] **Step 4: Run** `go test ./src/client/...`; expect PASS.
- [ ] **Step 5: Commit** `refactor: extract Plex playlist creation, add share title helper`.

### Task 4: `PlaylistSharer` and `Plex.SharePlaylist`

**Files:**
- Modify: `src/client/client.go` (interface next to `ArtworkUploader`; `Client.SharePlaylist`)
- Modify: `src/client/plex.go`
- Test: `src/client/plex_share_test.go`

**Interfaces:**
- Consumes: `ShareTitle`, `createPlaylistAs`, `PlexPlaylist` response type, `ClientConfig.ShareWith/OwnerName`, `ReplacePlaylist` (see `Config.ReplacePlaylist` as read in `main.go`).
- Produces:
  - `type PlaylistSharer interface { SharePlaylist(tracks []*models.Track, owner string, users []string, replace bool) error }`
  - `func (c *Client) SharePlaylist(tracks []*models.Track) error` — no-op nil when `Cfg.ShareWith` is empty; one `slog.Warn` and nil when `c.API` is not a `PlaylistSharer`. `replace` comes from a new `Client` field `Replace bool` set in `NewClient` from `cfg.ReplacePlaylist`.
  - `Plex.userSwitcher func(username string) (*Plex, error)` — test seam; `switchTo(name)` uses it when set, else sets `c.AdminClient.machineID = c.machineID` and calls `c.AdminClient.SwitchUser(name)`.
  - `func (c *Plex) SharePlaylist(tracks []*models.Track, owner string, users []string, replace bool) error`:
    - `AdminClient == nil` and no seam → one warning, nil, zero requests.
    - Filters to tracks with non-empty `ID`; none → log, nil.
    - Per recipient: `switchTo`; if `replace`, `GET /playlists` with the recipient's headers and remember the key of the exact-title match; `createPlaylistAs(recipient, title, tracks)`; set the summary (`PUT /playlists/<key>?summary=...` with `Cfg.PlaylistDescr`, recipient headers); only then `DELETE /playlists/<oldKey>`. Failures `slog.Warn` per recipient, are collected, returned via `errors.Join` (message names the recipient), and never stop the loop.

- [ ] **Step 1: Write failing tests** (fake server per recipient, injected `userSwitcher`):
  - `TestSharePlaylist_CreatesCopyThenDeletesOld`: recipient has `Gabe's Weekly-Exploration` (key `7`); request order is `POST /playlists`, then `DELETE /playlists/7`.
  - `TestSharePlaylist_KeepsUnrelatedPlaylists`: recipient also has `Gabe's Weekly-Exploration (old)` (key `8`) and `Mix` (key `9`); only `7` is deleted.
  - `TestSharePlaylist_ReplaceOffDeletesNothing`: `replace=false` sends no `GET /playlists` and no DELETE.
  - `TestSharePlaylist_CreateFailureKeepsOldAndContinues`: first recipient's POST returns 500, second succeeds; no DELETE for the first, second gets its copy, returned error contains the first user's name.
  - `TestSharePlaylist_UnknownUserSkipped`: switcher errors for one name; the others are processed.
  - `TestSharePlaylist_UsesRecipientToken`: the switcher returns clients whose `X-Plex-Token` header differs per recipient; every request to that recipient's server carries its own token.
  - `TestSharePlaylist_OwnerAccountRecipient`: switcher returns a copy of the admin client (as `SwitchUser` does for the owner account); copy is created normally.
  - `TestSharePlaylist_NoAdminClient`: zero requests, nil error.
  - `TestClientSharePlaylist_NonSharerBackend`: stub `APIClient` without `SharePlaylist` and non-empty `ShareWith`; returns nil, no panic.
  - `TestClientSharePlaylist_EmptyListIsNoOp`.
- [ ] **Step 2: Run** `go test ./src/client/ -run SharePlaylist -v`; expect FAIL (undefined).
- [ ] **Step 3: Implement** the interface, `Client.SharePlaylist`, the seam and `Plex.SharePlaylist` as specified.
- [ ] **Step 4: Run** `go test -race ./src/client/...`; expect PASS.
- [ ] **Step 5: Commit** `feat: share Plex playlists with the users on a share list`.

### Task 5: Wire sharing into the run

**Files:**
- Modify: `src/main/main.go:288-293` (the `else` after `playlist created successfully`)

**Interfaces:**
- Consumes: `(*client.Client).SharePlaylist(tracks []*models.Track) error`.

- [ ] **Step 1: Call** `client.SharePlaylist(tracks)` after `uploadCustomPlaylistArtwork`; on error `slog.Warn(err.Error())`. It must not run when the owner's `CreatePlaylist` failed.
- [ ] **Step 2: Run** `go build ./... && go vet ./...`; expect clean (`src/main` has no committed tests; as with BUG-34).
- [ ] **Step 3: Commit** `feat: share the playlist after creating it`.

### Task 6: "Share with" in the Users tab

**Files:**
- Modify: `src/web/frontend/src/components/Users.jsx` (`UserForm` ~line 35, `toggleEnabled` ~line 192)
- Modify: `src/web/frontend/src/lib/api.js` only if `createUser`/`updateUser` need no change (they pass the body through; confirm).

**Interfaces:**
- Consumes: `share_with` on user list items and request bodies (Task 1).

- [ ] **Step 1: Add** `UserForm` state `shareWith` initialised from `user?.share_with ?? []`, a "Share my playlists with" checkbox list of the other users (excluding the edited user; hide the field when there are no other users), and include `share_with: shareWith` in the save body. `UserForm` needs the full user list passed in as a prop from the parent.
- [ ] **Step 2: Fix `toggleEnabled`** to include `share_with: u.share_with` in its PUT body.
- [ ] **Step 3: Run** `npm run build` in `src/web/frontend`; expect success. `src/web/dist` is not committed; do not add it.
- [ ] **Step 4: Manual check** (`npm run dev` against a running backend, or note that it could not be run): add two users, tick one, save, reopen to see it persisted, toggle enabled and confirm it persists.
- [ ] **Step 5: Commit** `feat: share-with selector in the Users tab`.

### Task 7: Final verification

- [ ] **Step 1:** `go vet ./... && go test -race ./...`; expect no failures. Report failures and any tests skipped for blocked localhost sockets.
- [ ] **Step 2: Manual test on the Unraid instance** (cannot be automated; needs the dev image rebuilt): in the Users tab set Gabe to share with Kelly, run `--user gabe`. Confirm `Gabe's Weekly-Exploration` appears only in Kelly's account, a second run leaves exactly one copy, Kelly's own playlists survive, and Gabe's account has no extra playlist. Then run `--user all`.
