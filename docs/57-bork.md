# 57 - Bork

## Overview

Bork rewrites outgoing text in the style of the Swedish Chef before it is
sent. It is applied entirely on the client: the server only ever receives the
rewritten text, as an ordinary message or post.

There are three entry points:

| Where | How |
|-------|-----|
| CIRC (inside a room) | `/bork <text>` |
| C-Mail (inside a conversation) | `/bork <text>` |
| New post or post edit | the `[ ] bork` checkbox on the topics row of the compose panel |

Replies have no bork checkbox. The reply compose box is a bare text area with
no checkbox row.

The transform itself is a pure function, `bork.Bork(string) string`, in
`internal/bork`. It is deterministic: the same input always gives the same
output.

---

## Rules

The text is split on spaces. Each word keeps its leading and trailing
punctuation and has these rules applied to the letters in between:

| Rule | Example |
|------|---------|
| the becomes zee | `the` -> `zee` |
| ending tion becomes shun | `nation` -> `nashun` |
| ending en becomes ee | `seven` -> `sefee` |
| ending e (words longer than 2 letters) becomes e-a | `have` -> `hafe-a` |
| leading e becomes i | `every` -> `ifery` |
| an -> un, au -> oo, ow -> oo | `Man` -> `Mun`, `know` -> `knoo` |
| w -> v, v -> f, f -> ff, o -> u, u -> oo | `world` -> `vurld`, `you` -> `yuoo`, `fix` -> `ffix` |

The letter substitutions run in a single pass, so a `w` that becomes `v` is not
then turned into `f`. A word with a leading capital keeps it (`The` -> `Zee`),
and a word in all capitals stays in capitals (`NATION` -> `NASHUN`). Letters
outside a-z are left as they are, and a capital first letter that is not ASCII
is preserved (`Uber` with an umlaut keeps its capital).

After the last line of prose, ` Bork Bork Bork!` is appended.

### Text that is left alone

- words starting with `@` (mentions)
- words containing `://` (links)
- inline code between backticks, including a span that covers several words on
  one line
- fenced code blocks (lines from one ``` line to the next)
- lines that are empty or hold only spaces

Inline code is tracked per line by counting backticks in each word, so an
unbalanced backtick leaves the rest of that line untouched.

If a message holds no prose at all (only a fenced block, or only spaces), it is
sent unchanged and no trailer is added.

---

## Slash command

`/bork` is registered in `baseSlashCommands` (`internal/ui/screens/chatrooms.go`),
which CIRC and C-Mail share, so the known-command check accepts it in both.
`borkCommand` in the same file does the work for both Enter handlers:

- `/bork the nation` sends `zee nashun Bork Bork Bork!`
- the command name is case-insensitive (`/BORK` works)
- `/bork` with no text shows the local notice `*** /bork: nothing to send` and
  sends nothing
- `/bork /me waves` shows `*** /bork: cannot wrap another command` and sends
  nothing, so that a server command is never mangled

The input history keeps what was typed (`/bork the nation`), not the rewritten
text.

### /help

The server answers `/help` and does not know `/bork`. When the command sent was
`/help`, the client adds a line to the end of the reply (`withBorkHelp` in
`internal/ui/app.go`):

```
/bork <text> (Speak like-a a Svedish cheff Bork Bork Bork!)
```

It is on its own line, below the server's command list. Other reply-only
commands are not changed.

---

## Post checkbox

The panel field order is title, slug, body, topics, public, nsfw, bork. Tab
moves to the checkbox and Space ticks it.

- The box always starts unticked, for a new post and for an edit.
- When ticked, the post body is run through `bork.Bork` in the feed's
  `ComposeSubmitMsg` handler (`internal/ui/screens/feed.go`). The title, slug
  and topics are not changed.
- This applies to a post edit as well as a new post, because they share the
  handler.
- Only the copy that is submitted is rewritten. If a publish fails and the panel
  comes back, it still holds the text as typed.

---

## Not covered

Replies, guild posts, profile bios and Journal notes have no bork option.

---

## Tests

| File | Covers |
|------|--------|
| `internal/bork/bork_test.go` | each rule, case handling, protected text, trailer placement, empty input |
| `internal/ui/screens/screens_test.go` | `/bork` in CIRC: rewritten body, empty text, wrapped command |
| `internal/ui/screens/cmail_test.go` | the same for C-Mail |
| `internal/ui/screens/compose_test.go` | checkbox reachable by Tab, Space toggles, reset on open and edit |
| `internal/ui/screens/feed_test.go` | post content is rewritten only when the box is ticked |
| `internal/ui/app_test.go` | `/help` gains the `/bork` line on its own line, help modal rows |
