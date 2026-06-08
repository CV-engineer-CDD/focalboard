# Card task IDs and linux-loong64 plugin build

This fork adds a board-scoped task ID and a plugin-wide global task ID to every
Boards card so cards can be referenced with short stable values such as `#1`
inside one board and `G-1` across boards.

## Version notes

- `7.11.0-taskid-boardonly-linux-loong64`: board-scoped `#N` only; kept as a
  fallback package.
- `7.11.1-taskid-linux-loong64`: added plugin-wide `G-N`, read-only `Global ID`
  property display, and a manifest version bump for Mattermost upgrades.
- `7.11.2-taskid-linux-loong64`: fixed timestamp-shaped global IDs such as
  `G-1780519393936` by initializing from the highest valid existing `G-N`.
- `7.11.3-taskid-linux-loong64`: soft-deleted cards no longer reserve IDs;
  deleting the highest active ID lets the next card reuse it, and restore
  reassigns IDs when needed to avoid active-card conflicts.
- `7.11.4-taskid-linux-loong64`: added the board header `Deleted cards` dialog
  so users can restore deleted cards from the UI.
- `7.11.5-taskid-linux-loong64`: added confirmed permanent deletion from the
  `Deleted cards` dialog. The purge endpoint only accepts already-deleted cards
  and removes their active/history records so they no longer appear in the
  restore list.
- `7.11.6-taskid-linux-loong64`: changed delete semantics so soft and permanent
  deletes do not release IDs, fixed deleted-card title/time display, moved the
  global ID to card surfaces, and shows the board ID in card details.
- `7.11.7-taskid-linux-loong64`: fixed remaining restore and permanent-delete
  paths that could lower reserved counters in some cases.
- `7.11.8-taskid-linux-loong64`: fixed permanent-delete websocket placeholders
  reappearing as string/Untitled entries in Deleted cards.

## Card task ID behavior

- New cards receive their `taskId` on the server.
- The value is scoped to one board and uses the next available numeric ID in
  `#N` format.
- Client-provided `taskId` values are ignored during card creation.
- Duplicated cards receive a fresh `taskId`; the duplicate does not reuse the
  source card ID.
- Existing cards that do not have a numeric `taskId` are backfilled before card
  reads, board block reads, card creation, and card duplication.
- Backfill keeps the first occurrence of each existing numeric value unchanged,
  finds the highest current number, then assigns missing IDs in stable card
  order: `createAt`, then block ID.
- Duplicate numeric legacy values are resolved deterministically: the earliest
  card keeps the existing number and later duplicates are assigned new numbers
  after the current maximum.
- Invalid legacy values, including internal card UUIDs previously stored in
  `taskId`, are treated as missing and replaced with `#N`.
- Card task ID assignment is serialized per board inside the plugin process, so
  concurrent card creates and duplicates in the same board do not receive the
  same number.
- Soft-deleted and permanently deleted cards keep their IDs reserved for the
  lifetime of the board history. Deleting the highest ID does not let the next
  card reuse that number.
- Permanent deletion removes the card and its restore history from `Deleted
  cards`, but it does not put the card's `taskId` back into a reusable pool.
- If a deleted card is restored and a legacy conflict is detected, the restored
  card is assigned a new non-conflicting `taskId`.

Backfill writes the generated field with the `system` user. This avoids exposing
internal UUIDs as user-facing references and makes old cards usable with the same
short ID format as new cards.

The uniqueness guarantee is per board. In a multi-instance deployment where more
than one plugin process writes to the same database at the same time, a database
constraint or transactional sequence would be required for a strict cross-process
guarantee because `taskId` is stored inside the block fields JSON.

## UI display

The global task ID is shown in the main card surfaces, formatted as `#N` instead
of `G-N` for compact display:

- kanban cards
- table rows
- gallery cards
- calendar events
- card detail header

The card detail properties show the board-scoped value as a read-only `Board ID`
property, for example `471`.

The UI falls back to the board ID and then to the internal card ID only if the
server returns a card that still has no `globalTaskId`.

## Global task ID behavior

- New cards also receive a `globalTaskId` in `G-N` format.
- The value is stored in `fields.globalTaskId`; the card detail view now uses
  the separate board-scoped `taskId` as the read-only `Board ID` property.
- Global IDs are assigned from the persisted `system_settings` key
  `focalboard_card_global_task_id_max`.
- Existing cards are migrated lazily for the board currently being opened or
  written. Opening one board does not scan cards from every other board.
- If the persisted counter does not exist yet, the server scans existing active
  cards and deleted-card history once to initialize it from the highest valid
  `G-N`.
- Timestamp-shaped IDs accidentally generated by earlier custom builds, such as
  `G-1780519393936`, are treated as invalid and reassigned into the normal
  sequence when that board is migrated.
- Soft-deleted and permanently deleted cards keep their `globalTaskId`
  reserved. This intentionally leaves gaps so old code comments, commits, build
  logs, and board references cannot point to a later unrelated card.
- The board header menu includes a `Deleted cards` dialog for restoring deleted
  cards in the current board or permanently deleting them after confirmation.

## linux-loong64 build notes

The Mattermost plugin manifest is limited to the `linux-loong64` server
executable:

```json
"executables": {
  "linux-loong64": "server/dist/plugin-linux-loong64"
}
```

The plugin server build uses:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=loong64 go build -trimpath -o dist/plugin-linux-loong64
```

For this target, SQLite migration driver imports are excluded from the
Focalboard store package on `linux/loong64`. The plugin build also uses newer
`modernc.org/sqlite` and `modernc.org/libc` versions so the Mattermost server
SQLite driver dependency can compile for `loong64`.

## Verification commands

Run server behavior tests:

```bash
cd server
GOCACHE=/tmp/focalboard-gocache GOMODCACHE=/tmp/focalboard-gomodcache go test ./model ./app
```

Run the main webapp check:

```bash
cd webapp
npm run check
```

Build the web assets and plugin:

```bash
cd webapp
npm run pack

cd ../mattermost-plugin/webapp
npm run build

cd ..
make bundle
```

Verify the final plugin package contains only the `linux-loong64` executable:

```bash
tar -tzf mattermost-plugin/dist/focalboard-7.11.8.tar.gz | grep plugin-linux
tar -xOzf mattermost-plugin/dist/focalboard-7.11.8.tar.gz focalboard/plugin.json
file mattermost-plugin/server/dist/plugin-linux-loong64
```

## Archived packages

Rollback and comparison packages are committed under `release-archives/`.

| Package path | Feature level |
| --- | --- |
| `release-archives/focalboard-7.11.0-original-linux-loong64.tar.gz` | Upstream `v7.11.0` behavior with only linux-loong64 build/package adaptation. |
| `release-archives/focalboard-7.11.0-taskid-boardonly-linux-loong64.tar.gz` | Board-scoped `#N` only. |
| `release-archives/focalboard-7.11.0-taskid-linux-loong64.tar.gz` | Early board/global ID build. |
| `release-archives/focalboard-7.11.1-taskid-linux-loong64.tar.gz` | Manifest version bump and Global ID property display. |
| `release-archives/focalboard-7.11.2-taskid-linux-loong64.tar.gz` | Fixed timestamp-shaped global IDs. |
| `release-archives/focalboard-7.11.3-taskid-linux-loong64.tar.gz` | Deleted highest IDs could be reused; historical archive. |
| `release-archives/focalboard-7.11.4-taskid-linux-loong64.tar.gz` | Deleted cards restore dialog. |
| `release-archives/focalboard-7.11.5-taskid-linux-loong64.tar.gz` | Permanent delete from Deleted cards. |
| `release-archives/focalboard-7.11.6-taskid-linux-loong64.tar.gz` | Global ID display and Deleted cards fixes; historical archive because some counter-lowering paths remained. |
| `release-archives/focalboard-7.11.7-taskid-linux-loong64.tar.gz` | Fixes restore and permanent-delete counter lowering. |
| `release-archives/focalboard-7.11.8-taskid-linux-loong64.tar.gz` | Current preferred build; fixes permanent-delete websocket placeholders reappearing in Deleted cards. |
