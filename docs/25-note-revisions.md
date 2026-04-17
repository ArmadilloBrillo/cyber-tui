# Feature 25 — Note Revisions

Adds a revision history view to the Journal screen, allowing users to browse all past revisions of a private note and preview their content.

---

## Overview

In the journal note list, pressing `h` on the selected note opens the revision history view for that note. The view shows all revisions returned by the API, newest first. Pressing `enter` on a revision fetches and displays the full content of that revision. Pressing `esc` navigates back through the stack.

---

## Keyboard Shortcuts

| Context | Key | Action |
|---|---|---|
| Note list | `h` | Load revision history for selected note |
| Revision list | `j` / `↓` | Next revision |
| Revision list | `k` / `↑` | Previous revision |
| Revision list | `enter` | Preview revision content |
| Revision list | `esc` | Back to note list |
| Revision preview | `j` / `↓` | Scroll content down |
| Revision preview | `k` / `↑` | Scroll content up |
| Revision preview | `esc` | Back to revision list |

---

## States

```
note list → (h) → revision list → (enter) → revision preview
                       ↑                           |
                       └──────────── (esc) ────────┘
         ↑
         └──── (esc from revision list)
```

---

## New API Methods

| Method | Endpoint |
|---|---|
| `GetNote(noteID)` | `GET /v1/notes/:id` |
| `GetNoteRevision(noteID, revision)` | `GET /v1/notes/:id?revision=N` |
| `GetNoteRevisions(noteID, cursor)` | `GET /v1/notes/:id/revisions?limit=20` |

---

## New Message Types

```go
// Journal → App
LoadNoteRevisionsMsg { NoteID string }
LoadNoteRevisionMsg  { NoteID string; RevisionNumber int }
```

---

## New Model Type

```go
// model.NoteRevision represents a single historical revision of a note.
type NoteRevision struct {
    RevisionNumber int
    Content        string
    Topics         []string
    CreatedAt      time.Time
}
```

---

## New JournalModel Methods

| Method | Purpose |
|---|---|
| `SetRevisions(noteID, revisions, cursor)` | Enter revisions mode with loaded data |
| `SetRevisionPreview(note)` | Show a specific revision's content |

---

## JournalModel State Changes

New fields added to `JournalModel`:

```go
revisionsMode   bool                // true when viewing revision history
revisions       []model.NoteRevision
revisionsCursor string
revisionsNoteID string
revSelectedIdx  int
revPreview      *model.Note         // non-nil while previewing a specific revision
```

---

## Known Limitations

- Note revision pagination (`GetNoteRevisions` cursor) is implemented in the API client but the UI currently loads only the first page (20 revisions). Pagination support can be added as a follow-on.
- `PATCH /v1/notes/:id` (creating new revisions) returns a server-side 500 error. The client code is correct; see `docs/00-api-backlog.md` for status.
