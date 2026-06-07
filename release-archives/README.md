# Release Archives

This directory stores Mattermost Boards plugin tarballs that are worth keeping
for rollback or comparison during the linux-loong64 task ID work.

Package path format:

```text
release-archives/<package-name>.tar.gz
```

| Package | Purpose and feature level | Recommendation |
| --- | --- | --- |
| `focalboard-7.11.0-original-linux-loong64.tar.gz` | Upstream `v7.11.0` behavior with only linux-loong64 build/package adaptation. No task ID feature. | Use only to test rollback to original Boards behavior. |
| `focalboard-7.11.0-taskid-boardonly-linux-loong64.tar.gz` | First usable board-scoped `#N` build. No global ID. | Fallback when only per-board IDs are needed. |
| `focalboard-7.11.0-taskid-linux-loong64.tar.gz` | Early task ID build with board and global ID work before later versioned fixes. | Historical archive. Prefer newer builds. |
| `focalboard-7.11.1-taskid-linux-loong64.tar.gz` | Added plugin manifest version bump and read-only Global ID property display. | Historical archive. |
| `focalboard-7.11.2-taskid-linux-loong64.tar.gz` | Fixed timestamp-shaped global IDs such as `G-1780519393936`. | Historical archive. |
| `focalboard-7.11.3-taskid-linux-loong64.tar.gz` | Deleted highest IDs could be reused; restore reassigned conflicts. | Kept for history, not recommended for stable code references. |
| `focalboard-7.11.4-taskid-linux-loong64.tar.gz` | Added the Deleted cards dialog and restore UI. | Historical archive. |
| `focalboard-7.11.5-taskid-linux-loong64.tar.gz` | Added confirmed permanent delete from Deleted cards. | Historical archive. |
| `focalboard-7.11.6-taskid-linux-loong64.tar.gz` | External surfaces show global ID, card detail shows Board ID, and Deleted cards display is fixed. Some restore/permanent-delete paths could still lower counters. | Historical archive. |
| `focalboard-7.11.7-taskid-linux-loong64.tar.gz` | Fixes remaining restore and permanent-delete paths that could lower reserved counters, making the no-ID-reuse rule consistent. | Recommended custom build. |

The `.zst` copies were intentionally removed. The committed artifacts are the
Mattermost-uploadable `.tar.gz` plugin packages.
