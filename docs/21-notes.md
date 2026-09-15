# Feature 21 — Journal (Notes)

Private notes accessible only to the logged-in user. Notes support multiple revisions; editing a note creates a new revision rather than overwriting the original.

---

## Tab Position

Journal is the third tab (key `3`). Tabs shifted right by one:

| Key | Screen |
|-----|--------|
| `1` | Feed |
| `2` | Notifications |
| `3` | **Journal** |
| `4` | Bookmarks |
| `5` | Topics |
| `6` | Profile |
| `7` | Settings |

---

## Screen Layout

**List mode** (default):
```
┌─────────────────────────────────────────────────┐
│  feed  notifications  [journal]  bookmarks  ...  │  ← tab bar
├─────────────────────────────────────────────────┤
│                                                  │
│  ┌── first line of note content  ·  date ──────┐ │
│  │  #topic1 #topic2                             │ │
│  └──────────────────────────────────────────────┘ │
│                                                  │
│  ┌──── another note ·  2 days ago ─────────────┐ │
│  └──────────────────────────────────────────────┘ │
│                                                  │
│                                                  │
├─────────────────────────────────────────────────┤
│  n · new note   enter · edit   d · delete        │  ← status bar
└─────────────────────────────────────────────────┘
```

**Edit mode** (compose open):
```
┌──────── viewport (note list, shrunk) ───────────┐
│  ...                                             │
└──────────────────────────────────────────────────┘
┌──────── compose box ────────────────────────────┐
│  editing note                                    │
│  ▌                                               │
└──────────────────────────────────────────────────┘
┌──────── topics input ───────────────────────────┐
│  add topics  (journal, idea, …  max 3)           │
└──────────────────────────────────────────────────┘
  Ctrl+S · save   Ctrl+P · publish   Tab · topics   Esc · cancel
```

**Confirmation prompt** (publish or delete):
```
┌──────── confirmation ───────────────────────────┐
│  Publish note as post?  [y]es  [n]o / esc        │
└──────────────────────────────────────────────────┘
```

---

## Keyboard Shortcuts

### List mode

| Key | Action |
|-----|--------|
| `j` / `↓` | Next note (triggers pagination at bottom) |
| `k` / `↑` | Previous note |
| `enter` | Open selected note in edit mode |
| `n` | New blank note |
| `d` | Delete selected note (shows confirmation) |

### Confirmation overlay

| Key | Action |
|-----|--------|
| `y` | Confirm (publish or delete) |
| `n` / `esc` | Cancel |

### Edit mode

| Key | Action |
|-----|--------|
| `tab` | Toggle focus between compose and topics input |
| `ctrl+s` | Save note (create if new, update if existing) |
| `ctrl+p` | Publish note as a post (shows confirmation) |
| `enter` | Insert paragraph break |
| `esc` | Cancel edit — discard unsaved changes |

---

## Publish Flow

Pressing `ctrl+p` in edit mode asks for confirmation:

```
Publish note as post?  [y]es  [n]o / esc
```

On confirmation (`y`), the note content and topics are submitted to `POST /v1/posts`. The note itself is **not** deleted — it remains in the journal. This lets you keep a private draft alongside the published post.

The editor stays open while the publish is in flight (`ctrl+p`/`ctrl+s`/`esc` inert, hint shows `… publishing`). On success it closes; on a non-401 failure (e.g. a rate limit) it stays open with your text intact and shows a banner, so you can retry or `ctrl+s` to save it as a note. See `docs/51-compose-to-journal.md`.

---

## Pagination

Journal uses cursor-based pagination identical to the feed:
- 20 notes per page (API default)
- Next page loads automatically when `j` / `↓` reaches the last note and a cursor is available
- `— end of journal —` is shown when all notes are loaded

---

## API Endpoints

| Method | Path | Used for |
|--------|------|---------|
| `GET` | `/v1/notes?limit=20[&cursor=...]` | List notes (paginated) |
| `POST` | `/v1/notes` | Create note |
| `PATCH` | `/v1/notes/:id` | Update note (creates new revision) |
| `DELETE` | `/v1/notes/:id` | Soft-delete note |
| `POST` | `/v1/posts` | Publish note as post |

---

## Data Model

```go
type Note struct {
    ID             string
    AuthorID       string
    Content        string
    Topics         []string  // max 3; sent on create/update, not in list API response
    RevisionNumber int       // increments on each update
    Deleted        bool
    CreatedAt      time.Time
}
```

---

## Files

| File | Role |
|------|------|
| `internal/model/types.go` | `Note` struct |
| `internal/api/interface.go` | `GetNotes`, `CreateNote`, `UpdateNote`, `DeleteNote` |
| `internal/api/client.go` | HTTP implementation + wire types |
| `internal/api/mock.go` | Mock data (3 seed notes) + mock implementations |
| `internal/api/client_test.go` | HTTP client tests for all 4 Note methods |
| `internal/api/mock_test.go` | Mock client tests for all 4 Note methods |
| `internal/ui/screens/journal.go` | `JournalModel` — full screen with list/edit/confirm states |
| `internal/ui/screens/shared.go` | `LoadMoreJournalMsg`, `SubmitSaveNoteMsg`, `SubmitPublishNoteMsg`, `SubmitDeleteNoteMsg` |
| `internal/ui/app.go` | `screenJournal`, tab wiring, `handleJournal`, all Note commands |
