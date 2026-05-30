package ssh_test

import (
	"path/filepath"
	"testing"

	"github.com/ragnar/cyber-tui/internal/api"
	internalssh "github.com/ragnar/cyber-tui/internal/ssh"
)

// TestServe_SurfacesListenError verifies Serve returns the listen failure rather
// than blocking forever (the previous version discarded ListenAndServe errors).
func TestServe_SurfacesListenError(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "host_key")
	calls := 0
	newClient := func() api.Client {
		calls++
		return api.NewMockClient()
	}

	// Port out of range makes ListenAndServe fail immediately.
	err := internalssh.Serve("127.0.0.1:99999999", keyPath, newClient)
	if err == nil {
		t.Fatal("expected an error from an unbindable address, got nil")
	}
	if calls != 0 {
		t.Errorf("client factory called %d times before any connection, want 0", calls)
	}
}
