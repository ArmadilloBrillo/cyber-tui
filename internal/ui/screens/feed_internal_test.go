package screens

import (
	"testing"

	"github.com/ragnar/cyber-tui/internal/model"
)

// TestFeedSetPosts_EvictsStaleBodyCache guards against bodyCache (keyed by
// post ID, populated in renderPost) growing for the life of the session —
// SetPosts wholesale-replacing m.posts is the only point a post can
// permanently drop out of the loaded list, so that's where stale cache
// entries must be dropped too.
func TestFeedSetPosts_EvictsStaleBodyCache(t *testing.T) {
	m := NewFeedModel()
	m.bodyCache["gone"] = feedBodyCacheEntry{body: "stale"}
	m.bodyCache["kept"] = feedBodyCacheEntry{body: "fresh"}

	m = m.SetPosts([]model.Post{{ID: "kept"}}, "")

	if _, ok := m.bodyCache["gone"]; ok {
		t.Error("expected bodyCache entry for a post no longer in m.posts to be evicted")
	}
	if _, ok := m.bodyCache["kept"]; !ok {
		t.Error("expected bodyCache entry for a post still in m.posts to survive")
	}
}
