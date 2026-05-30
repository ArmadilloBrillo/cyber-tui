package api_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/ragnar/cyber-tui/internal/api"
)

// TestHTTPClient_ConcurrentTokenAccess exercises the mutex that guards the
// client's tokens. It runs reads (GetFeed builds the Authorization header) and
// writes (LoginWithRefreshToken triggers a token refresh) from many goroutines
// at once. Run with -race: without the mutex the detector reports a data race.
func TestHTTPClient_ConcurrentTokenAccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/login":
			io.WriteString(w, `{"data":{"idToken":"id","refreshToken":"r","rtdbToken":"rt"}}`)
		case "/v1/auth/refresh":
			io.WriteString(w, `{"data":{"idToken":"id2","rtdbToken":"rt2"}}`)
		default:
			io.WriteString(w, `{"data":[]}`)
		}
	}))
	defer srv.Close()

	c := api.NewHTTPClientForTesting(srv.URL, srv.Client())
	if _, err := c.Login("e@x", "p"); err != nil {
		t.Fatalf("login: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if _, _, err := c.GetFeed(""); err != nil {
					t.Errorf("GetFeed: %v", err)
					return
				}
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if _, err := c.LoginWithRefreshToken("r"); err != nil {
					t.Errorf("LoginWithRefreshToken: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
