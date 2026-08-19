package screens

import (
	"testing"

	"github.com/ragnar/cyber-tui/internal/model"
)

// TestGuildsSetGuildPosts_EvictsStalePostBodyCache mirrors
// TestFeedSetPosts_EvictsStaleBodyCache (feed_internal_test.go) —
// SetGuildPosts wholesale-replacing m.posts is the point a post can
// permanently drop out of the loaded list, so that's where stale
// postBodyCache entries must be dropped too.
func TestGuildsSetGuildPosts_EvictsStalePostBodyCache(t *testing.T) {
	m := NewGuildsModel()
	m.postBodyCache["gone"] = feedBodyCacheEntry{body: "stale"}
	m.postBodyCache["kept"] = feedBodyCacheEntry{body: "fresh"}

	m = m.SetGuildPosts([]model.Post{{ID: "kept"}}, "")

	if _, ok := m.postBodyCache["gone"]; ok {
		t.Error("expected postBodyCache entry for a post no longer in m.posts to be evicted")
	}
	if _, ok := m.postBodyCache["kept"]; !ok {
		t.Error("expected postBodyCache entry for a post still in m.posts to survive")
	}
}

// TestGuildsThreadReplies_EvictsStaleReplyBodyCache guards replyBodyCache
// (keyed by reply ID) against growing for the life of the session —
// GuildThreadRepliesMsg wholesale-replacing m.threadReplies is the point a
// reply can permanently drop out of the loaded thread.
func TestGuildsThreadReplies_EvictsStaleReplyBodyCache(t *testing.T) {
	m := NewGuildsModel()
	m = m.SetGuildPosts([]model.Post{{ID: "p1"}}, "")
	m.replyBodyCache["gone"] = replyBodyCacheEntry{rendered: "stale"}
	m.replyBodyCache["kept"] = replyBodyCacheEntry{rendered: "fresh"}

	m, _ = m.Update(GuildThreadRepliesMsg{PostID: "p1", Replies: []model.Reply{{ID: "kept"}}})

	if _, ok := m.replyBodyCache["gone"]; ok {
		t.Error("expected replyBodyCache entry for a reply no longer in m.threadReplies to be evicted")
	}
	if _, ok := m.replyBodyCache["kept"]; !ok {
		t.Error("expected replyBodyCache entry for a reply still in m.threadReplies to survive")
	}
}
