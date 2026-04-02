# 14 — Post topics

## Purpose
When creating a new post, the user can attach up to 3 topics. Topics are stored on the post and displayed in the feed and post detail views. They are separate from the post body.

## Behaviour
- Pressing `n` on the feed opens the compose box as before, plus a **topics input row** that appears directly below it.
- The topics field accepts a **comma-separated list** of topic names (e.g. `go, my cool topic, tui`). No `#` prefix is required or expected.
- Maximum **3 topics** per post. If more than 3 are entered, only the first 3 are submitted.
- **Tab** cycles focus between the post body and the topics field:
  - Body focused → `Tab` → topics focused (body border dims, topics border activates)
  - Topics focused → `Tab` → body focused (topics border dims, body border activates)
- **Ctrl+S** submits from either field, combining the body content and parsed topics.
- **Esc** cancels from either field, closing both the compose box and the topics input.
- Topics are **not** available when replying — only for top-level new posts.
- The status bar displays `Ctrl+S · send   Tab · topics   Enter · paragraph   Esc · cancel` while the feed compose is open.

## Topic parsing — `ParseTopics(s string) []string`
1. Split on `,`
2. Trim leading/trailing whitespace from each part
3. Drop empty parts
4. Cap at 3

## Focus / dimming
`ComposeModel` gained a `focused bool` field. `SetFocused(bool) (ComposeModel, tea.Cmd)` calls `textarea.Blur()` or `textarea.Focus()` on the underlying widget so the cursor hides/shows correctly, and returns the blink cmd when re-focusing.

## Key files
| File | Symbol | Role |
|---|---|---|
| `internal/ui/screens/feed.go` | `SubmitNewPostMsg.Topics []string` | Carries parsed topics to the app |
| `internal/ui/screens/feed.go` | `FeedModel.topicsInput` / `topicsFocused` | Topics text input and focus state |
| `internal/ui/screens/feed.go` | `ParseTopics(s string) []string` | Parses and caps the comma-separated input |
| `internal/ui/screens/feed.go` | `viewportHeight()` | Subtracts 3 extra rows for the topics box when compose is active |
| `internal/ui/screens/compose.go` | `ComposeModel.SetFocused(bool)` | Dims/activates compose border and hides/shows textarea cursor |
| `internal/ui/app.go` | `createPostCmd(content, topics)` | Passes topics to `client.CreatePost` |
