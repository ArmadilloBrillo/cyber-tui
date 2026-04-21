Run the apifetch developer tool to call the cyberspace.online API using the saved session from ~/.cyber-tui.json.

Usage: /apifetch <path> [options]

Steps:
1. Run `go run ./cmd/apifetch $ARGUMENTS` from the project root
2. Show the pretty-printed JSON response to the user

Examples the user might pass as $ARGUMENTS:
- `/v1/users/me`
- `/v1/posts?limit=5`
- `--method POST --body '{"content":"hello"}' /v1/posts`
- `--method DELETE /v1/posts/some-post-id`

If the tool errors (no saved session, refresh failed, API error), show the error message clearly.
