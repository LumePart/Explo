# Shared Plex playlists — design

Status: draft, awaiting owner approval. Date: 2026-10-28.

## Intent

Two people (Gabe and Kelly) each run their own Explo instance against the
same Plex server. Each wants to see the other's weekly playlist in their own
Plex account, clearly labelled with whose it is:

| Playlist owner | Owner sees | Other user sees |
|---|---|---|
| Gabe | `Weekly Exploration` | `Gabe's Weekly Exploration` |
| Kelly | `Weekly Exploration` | `Kelly's Weekly Exploration` |

Plex has no real playlist sharing: a playlist belongs to exactly one account.
Sharing therefore means creating a **copy** of the playlist in each
recipient's account, titled with the owner's name.

## Goals

- After a run, every user on the instance's share list has a copy of the
  playlist in their Plex account, titled `<Owner>'s <PlaylistName>`.
- The owner's own playlist is unchanged (name and behaviour).
- With the feature unconfigured, behaviour is byte-for-byte what it is today.
- Each instance works alone. The two instances never talk to each other.

## Non-goals

- Jellyfin, Emby, Subsonic, MPD. Only Plex.
- Live sync between the owner's playlist and the copies. Copies are replaced
  on the next run, not edited in place.
- Letting recipients edit the owner's playlist.
- Per-playlist share lists. One list per instance.
- Inviting users to the Plex server or granting library access.

## Anti-goals (a failure even if it "works")

- A sharing problem must never fail the run, block the owner's own playlist,
  or block other recipients.
- A recipient must never be left with no copy because a refresh failed halfway
  (same principle as BUG-34).
- Never delete or rename a playlist the recipient made themselves.

## Constraints and existing code

- `src/client/plex.go` already switches to another Plex user:
  `AdminClient.SwitchUser(username)` finds the user in the server's shared
  users and returns a client using that user's access token. It requires
  admin credentials (`AdminCreds`: user+password or API key). Playlist
  creation already uses this path for the instance's own user
  (`ensureUserClient`).
- Track IDs on `models.Track` come from the shared Explo library, so the same
  IDs are valid in every user's account provided that user can see the
  library.
- `main.go` flow today: download, then `DeletePlaylist` (if
  `ReplacePlaylist`), then `CreatePlaylist`, then artwork upload. Sharing
  happens after the owner's `CreatePlaylist` succeeds.
- `client.APIClient` is implemented by every backend. Optional capabilities
  use a separate interface (see `ArtworkUploader` in `src/client/client.go`).
  Sharing follows that pattern so other backends need no changes.

## Design

### Settings (`src/config/config.go`, `ClientConfig`)

| Env var | Type | Default | Meaning |
|---|---|---|---|
| `PLAYLIST_SHARE_WITH` | comma-separated string | empty | Plex usernames to receive a copy. Empty disables the feature. |
| `PLAYLIST_OWNER_NAME` | string | the instance's Plex user (`Creds.User`) | Name used in the copy's title prefix. |

Matching of share-list entries to Plex users reuses `findSharedUser`
(username, email, display name, or ID, case-insensitive). Entries are trimmed;
blanks and duplicates are dropped; an entry that resolves to the instance's own
user is skipped (no copy of your own playlist in your own account).

### Title

`<OwnerName>'s <ClientCfg.PlaylistName>`, e.g. `Gabe's Weekly Exploration`.
The suffix is the same name the owner's playlist gets, so week-stamped names
(`PLAYLISTNAME_FORMAT`, when not replacing) carry through. Use a straight
apostrophe. The prefix is built by one small pure function so it is unit
tested without Plex.

### New optional interface (`src/client/client.go`)

```go
// PlaylistSharer is implemented by backends that can copy a playlist into
// other users' accounts.
type PlaylistSharer interface {
	SharePlaylist(tracks []*models.Track, owner string, users []string, replace bool) error
}
```

`Client` gets a thin `SharePlaylist` that type-asserts `c.API` to
`PlaylistSharer`. If the backend does not implement it and the share list is
non-empty, log one warning and return nil.

### Plex implementation (`src/client/plex.go`)

`SharePlaylist` loops over recipients. For each one, independently:

1. `AdminClient.SwitchUser(name)`. If `AdminClient` is nil (no admin
   credentials) log a single warning explaining that sharing needs admin
   credentials and stop. If the user is not found, warn and continue to the
   next recipient.
2. Build the copy title (above).
3. If `replace` is true, look up the recipient's playlists (`GET /playlists`
   with their token) and note the rating key of the one whose title matches
   the copy title exactly. Only an exact title match is ever touched, so the
   recipient's own playlists are safe.
4. Create the new copy (same calls as `CreatePlaylist`: create, then add
   tracks) using the recipient's client.
5. Only after the new copy exists, delete the old one found in step 3. Create
   before delete means a failed refresh leaves the old copy in place. If
   deleting fails, warn; the recipient briefly has two copies, which is
   better than none.
6. If `replace` is false, skip steps 3 and 5. The title already differs per
   week, so copies accumulate exactly as the owner's playlists do.
7. Apply `PublicPlaylist` and the playlist description the same way the
   owner's playlist does (description keeps "Created for <ListenBrainz user>").

Refactor note: `CreatePlaylist` currently mixes "switch user" and "create".
Extract the create-and-fill part into a helper that takes a ready user client
and a title, and call it from both `CreatePlaylist` and `SharePlaylist`. The
owner's code path must behave identically after the refactor.

Sleeps: honour the existing `Sleep`/rate settings between recipients if the
current create path uses them.

### Wiring (`src/main/main.go`)

After the owner's `CreatePlaylist` succeeds and artwork is uploaded:

```go
if len(cfg.ClientCfg.ShareWith) > 0 {
	if err := client.SharePlaylist(tracks, cfg.ClientCfg.OwnerName, cfg.ClientCfg.ShareWith, cfg.ReplacePlaylist); err != nil {
		slog.Warn(err.Error())
	}
}
```

If the owner's create failed, sharing is skipped (nothing to share). Errors
from individual recipients are logged inside `SharePlaylist` and aggregated;
the run exit status is unchanged.

Artwork: custom-playlist artwork is uploaded for the owner only in this
version. Recipients' copies get no custom artwork. (Builder's call to extend
later.)

### Web UI settings

- Register both keys in `src/web/backend/defs/defs.go` next to
  `PUBLIC_PLAYLIST`, in the Plex-relevant group, with labels "Share
  playlists with (Plex usernames)" and "Your name on shared playlists".
- Allow them in the settings read/write path in
  `src/web/backend/settings/handlers.go`.
- Frontend source is in `src/web/frontend`; rebuild `src/web/dist` only if
  that is how this repo normally ships UI changes (check git history for how
  `dist` is updated).
- Document both vars in `src/web/sample.env`.

### Logging

- Info per recipient on success: owner, recipient, track count.
- Warn per recipient on failure with the reason.
- One warn if the backend cannot share or admin credentials are missing.

## Testing

Unit (no network):
- Title builder: normal, empty owner falls back to Plex user, name containing
  an apostrophe, week-stamped playlist name.
- Share-list parsing: trims, drops blanks and duplicates, case handling, and
  skipping the instance's own user.

Plex client with `httptest` (note: some sandboxes block localhost sockets;
record that rather than claiming a pass):
- Replace on: new copy created before old one deleted; only the exact-title
  playlist is deleted; the recipient's unrelated playlist survives.
- Create fails for one recipient: others still get theirs, old copy kept.
- Unknown recipient: warning, others proceed.
- No `AdminClient`: single warning, no requests made.
- Replace off: nothing deleted.

Regression: owner path unchanged. Existing Plex tests still pass; `go vet`
and `golangci-lint` clean.

Manual (the one thing tests cannot cover): run against the real server with
both users; confirm each account shows the expected titles and the tracks
play for the recipient.

## Left to the builder

- Exact helper/function names and file split.
- Whether the settings UI needs a frontend rebuild (see above).
- Error aggregation shape returned from `SharePlaylist`.

## Open questions for the owner

1. Do recipients have access to the Explo library in Plex? If not, copies
   will appear empty. (Assumed yes.)
2. Does each instance already have admin credentials configured? Sharing
   cannot work without them. (Assumed yes.)
3. Should custom-playlist artwork also go on the copies? (Default: no.)
