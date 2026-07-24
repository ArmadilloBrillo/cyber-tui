package screens

import (
	"context"
	"time"

	"github.com/ragnar/cyber-tui/internal/api"
	"github.com/ragnar/cyber-tui/internal/model"
)

// reconnectBackoffSchedule is the delay before each backed-off retry after
// the first (immediate) reconnect attempt fails. A var, not a const, so
// tests can shrink it. Shared by the C-Mail and CIRC reconnect sequences.
var reconnectBackoffSchedule = []time.Duration{
	1 * time.Second,
	2 * time.Second,
	4 * time.Second,
	8 * time.Second,
	15 * time.Second,
}

// maxReconnectAttempts bounds the total attempts made after a live stream
// dies: 1 immediate attempt plus one per entry in reconnectBackoffSchedule.
var maxReconnectAttempts = 1 + len(reconnectBackoffSchedule)

// reconnectDelay returns the backoff before retry attempt n (n >= 1),
// clamped to the last schedule entry once attempts exceed its length.
func reconnectDelay(attempt int) time.Duration {
	i := max(attempt-1, 0)
	if i >= len(reconnectBackoffSchedule) {
		i = len(reconnectBackoffSchedule) - 1
	}
	return reconnectBackoffSchedule[i]
}

// attemptReconnect refreshes the session token and re-subscribes, the shared
// two-step recovery used by both the C-Mail and CIRC reconnect commands
// (which differ only in which Subscribe* method they call).
func attemptReconnect(client api.Client, ctx context.Context, subscribe func(context.Context) (<-chan model.Message, context.CancelFunc, error)) (<-chan model.Message, context.CancelFunc, error) {
	if err := client.RefreshSession(); err != nil {
		return nil, nil, err
	}
	return subscribe(ctx)
}
