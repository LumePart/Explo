# Multi-user playlist sharing — design

Status: draft, awaiting owner approval. Date: 2026-10-29.
Supersedes the share-list source in `2026-10-28-shared-playlists-design.md`
(env var) and the "share with everyone" default in
`2026-10-28-multi-user-design.md`. Everything else in the shared-playlists
design (copy semantics, create-then-delete, failure isolation) still applies.

## Intent

One Explo instance now runs several users (`--user <id>` / `--user all`).
Each user's weekly playlists exist only in that user's Plex account. Users
want to see each other's playlists, clearly labelled, without Plex's missing
playlist sharing: a playlist belongs to exactly one account, so sharing means
creating a **copy** in each recipient's account.

| Owner | Owner sees | Recipient sees |
|---|---|---|
| Gabe | `Weekly-Exploration` | `Gabe's Weekly-Exploration` |

## Decisions (made by the owner)

- Separate copy per recipient, titled `<Display name>'s <PlaylistName>`.
- Per-user choice: each user has their own "share with" list of other users.
- Default share list is empty (opt-in). Existing users behave as today.
- Replace in step: each run replaces the recipient's previous copy of that
  playlist; no history accumulates.
- Copies are made during the owner's own run, from the tracks already matched
  there (no extra downloads). A lone `--user gabe` run shares too.
- Share lists live in `users.json` and are edited in the Users tab.
- The duplicate-playlist bug (owner replace deletes only the first title
  match) is a **separate change**, not part of this work.

## Goals

- After a user's run creates their playlist, every user on their share list
  has an up-to-date copy.
- Owner's own playlist and its behaviour are unchanged.
- A user with an empty share list, or a non-Plex backend, behaves exactly as
  today.
- Share lists are managed in the Users tab and stored per user.

## Non-goals

- Non-Plex backends. Live sync. Recipients editing the source. Per-playlist
  share lists (one list per user). Inviting users to the Plex server.
- Fixing the owner-side duplicate/replace bug.
- Artwork on copies.

## Anti-goals

- A sharing problem must never fail the run, block the owner's playlist, or
  block other recipients.
- A recipient is never left without a copy because a refresh failed midway:
  create the new copy first, delete the old one only after.
- Never delete or rename a playlist the recipient made themselves. Only
  delete a recipient playlist whose title is exactly the copy title.
- Copies are never re-shared; only an owner's own run produces copies.

## Design

**Data.** `users.User` gains `ShareWith []string` (user IDs, not Plex names;
IDs are stable if a Plex name changes). Missing field loads as empty. Display
name is the existing `Name`.

**Config overlay.** `ApplyUser` stops sharing with every enabled user. It
resolves the target's `ShareWith` IDs to recipient Plex usernames, dropping
IDs that are unknown, disabled, self, or have no `PlexUser`, and sets
`ClientCfg.ShareWith` (Plex names) and `ClientCfg.OwnerName` (= `Name`).
Typed `PLAYLIST_SHARE_WITH` stays ignored in multi-user mode.

**Client.** Per the existing plan: optional `PlaylistSharer` interface
implemented by Plex; `Plex.SharePlaylist(tracks, owner, users, replace)`;
pure helpers `ShareTitle`, `ParseShareList` in `src/client/share.go`; copy
creation uses an extracted `createPlaylistAs` helper. Per recipient:
switch to the recipient (owner account supported via `isOwnerAccount`),
find the exact-title playlist, create the copy, then delete the old one
(when replace is on). Recipient operations must use the recipient's token
(`playlistHeaders`-style scoping), never the instance token. Errors are
logged per recipient and joined; the loop never stops early. No admin
credentials: one warning, no requests.

**Run.** In `main.go`, after the owner's `CreatePlaylist` succeeds and artwork
upload, call `client.SharePlaylist(tracks)`; a failure is a warning only
(not `notify`, no exit).

**Users API and UI.** Users create/update accepts and returns `share_with`
(array of IDs). Validation: IDs must exist, not be self; unknown IDs are
rejected with 400. Deleting a user removes their ID from every other user's
`share_with`. The Users tab edit form gets a multi-select of the other users
(checkboxes), empty by default. Docs and sample env drop the env-var story
for multi-user.

**Admin key bug.** The wizard saves the admin key as `ADMIN_SYSTEM_APIKEY`
but config reads `ADMIN_API_KEY` (Task 1 of the existing plan). Verify it is
still present and fix it, since sharing needs admin credentials.

## Existing copies and edge cases

- A user whose Plex name changes: `ShareWith` uses IDs so recipients stay
  correct; old copies titled with the old display name are left alone (they
  don't match the new exact title). Accepted limitation.
- Display name containing an apostrophe or empty: title stays sensible
  (`ShareTitle` handles it, as in the existing plan).
- Recipient cannot see the Explo library: copy creation yields an empty or
  failing playlist; log a warning for that recipient.
- `--user all`: users run sequentially, so a recipient may briefly hold last
  week's copy until the owner's turn; acceptable.

## Left to the builder

Exact JSON/UI layout of the multi-select, test structure, and how the
recipient-token scoping is factored, provided the anti-goals hold.

## Verification

`go vet ./...`, `go test -race ./...`, plus a manual run on the Unraid test
instance: set Gabe to share with Kelly, run `--user gabe`, confirm
`Gabe's Weekly-Exploration` appears only in Kelly's account, a second run
replaces it (one copy), and Kelly's unrelated playlists survive.
