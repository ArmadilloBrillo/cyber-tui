# 05 — HTTP Client

Branch: `feature/http-client`

## What was built

A production-ready `HTTPClient` that implements the `api.Client` interface against the
cyberspace.online v0.2 REST API. Previously `HTTPClient` was an empty struct; all work
was routed to `MockClient`.

## Files changed

| File | Change |
|---|---|
| `internal/api/client.go` | Full implementation (wire types, helpers, 7 methods, RTDB stubs) |
| `internal/api/client_test.go` | New — 9 httptest-based integration tests |
| `cmd/cyber-tui/main.go` | Client selection via `CYBERSPACE_USE_MOCK`, corrected base URL |
| `.env.example` | Removed `CYBERSPACE_API_TOKEN`, added `CYBERSPACE_USE_MOCK`, fixed base URL |

## Design decisions

### Token handling
Login returns three tokens: `idToken` (Bearer auth), `refreshToken` (long-lived),
and `rtdbToken` (Firebase RTDB, for future chat). All three are stored in `model.Tokens`
and held in `HTTPClient.tokens`.

### Automatic token refresh
`doRequest` retries once on HTTP 401 for any path outside `/v1/auth/`. It calls the
private `refresh()` method directly (not through `doRequest`) to avoid recursion. If the
refresh also fails, `ErrUnauthorized` is returned and the UI should redirect to the login
screen.

### Typed errors
`APIError` carries `Code`, `Message`, and `Status`. Callers can use `errors.As` or a
direct type assertion to handle specific codes. Two sentinel values are exported:
`ErrUnauthorized` (401) and `ErrRateLimited` (429).

### Test injection
`NewHTTPClientForTesting(baseURL string, hc *http.Client)` accepts an `*http.Client`
wired to an `httptest.Server`. This keeps internal fields unexported while still allowing
full round-trip tests without hitting the real API.

### Chat / DMs — deferred
`GetRooms`, `GetRoomMessages`, `SendRoomMessage`, `GetConversations`, `GetMessages`, and
`SendMessage` return `fmt.Errorf("not implemented: … — see feature/rtdb-chat")`. The
cyberspace.online chat layer is served over Firebase Realtime Database SSE streams, not
the REST API, and will be implemented separately.

### Concurrency caveat
`HTTPClient` is not goroutine-safe. `tokens` is mutated by `Login` and the refresh logic.
Bubble Tea commands run in separate goroutines; a `sync.Mutex` should be added if
concurrent token mutation becomes a problem in practice.

## Client selection (main.go)

| Environment | Behaviour |
|---|---|
| `CYBERSPACE_USE_MOCK=1` | `MockClient` — static fake data, no network |
| _(default)_ | `HTTPClient` against `https://api.cyberspace.online` |
| `CYBERSPACE_API_BASE_URL=<url>` | Override the base URL for staging/local proxy |

## Running

```bash
# Real login screen (enter your cyberspace.online credentials)
go run ./cmd/cyber-tui

# Mock mode — no credentials required
CYBERSPACE_USE_MOCK=1 go run ./cmd/cyber-tui
```
