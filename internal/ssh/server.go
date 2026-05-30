package ssh

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/ragnar/cyber-tui/internal/api"
	"github.com/ragnar/cyber-tui/internal/ui"
)

// Serve starts the Wish SSH server so remote users can connect with
// `ssh <host> -p <port>` and get a full TUI session.
//
// newClient is called once per connection so each session gets its own API
// client and authentication state; sessions never share a client. Sessions are
// marked ephemeral so a remote login is never written to the host's config file.
//
// The server is experimental and performs no SSH authentication: any client that
// can reach the address gets a session. Restrict network exposure accordingly.
func Serve(addr, hostKeyPath string, newClient func() api.Client) error {
	handler := bubbletea.Middleware(func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
		return ui.NewApp(newClient()).WithEphemeralSession(), []tea.ProgramOption{
			tea.WithAltScreen(),
		}
	})

	srv, err := wish.NewServer(
		wish.WithAddress(addr),
		wish.WithHostKeyPath(hostKeyPath),
		wish.WithMiddleware(handler),
	)
	if err != nil {
		return err
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	serveErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			serveErr <- err
		}
	}()

	select {
	case err := <-serveErr:
		return err
	case <-done:
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}
