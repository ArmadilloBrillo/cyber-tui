package screens

import (
	"testing"

	"github.com/ragnar/cyber-tui/internal/model"
)

// TestTopicsSetTopicPosts_EvictsStalePostBodyCache mirrors
// TestFeedSetPosts_EvictsStaleBodyCache (feed_internal_test.go) —
// SetTopicPosts wholesale-replacing m.posts is the point a post can
// permanently drop out of the loaded list, so that's where stale
// postBodyCache entries must be dropped too.
func TestTopicsSetTopicPosts_EvictsStalePostBodyCache(t *testing.T) {
	m := NewTopicsModel()
	m.postBodyCache["gone"] = feedBodyCacheEntry{body: "stale"}
	m.postBodyCache["kept"] = feedBodyCacheEntry{body: "fresh"}

	m = m.SetTopicPosts([]model.Post{{ID: "kept"}}, "")

	if _, ok := m.postBodyCache["gone"]; ok {
		t.Error("expected postBodyCache entry for a post no longer in m.posts to be evicted")
	}
	if _, ok := m.postBodyCache["kept"]; !ok {
		t.Error("expected postBodyCache entry for a post still in m.posts to survive")
	}
}

// TestTopicsThreadReplies_EvictsStaleReplyBodyCache guards replyBodyCache
// (keyed by reply ID) against growing for the life of the session —
// TopicThreadRepliesMsg wholesale-replacing m.threadReplies is the point a
// reply can permanently drop out of the loaded thread.
func TestTopicsThreadReplies_EvictsStaleReplyBodyCache(t *testing.T) {
	m := NewTopicsModel()
	m = m.SetTopicPosts([]model.Post{{ID: "p1"}}, "")
	m.threadPostID = "p1" // TopicThreadRepliesMsg is applied against threadPostID, not postIndex
	m.replyBodyCache["gone"] = replyBodyCacheEntry{rendered: "stale"}
	m.replyBodyCache["kept"] = replyBodyCacheEntry{rendered: "fresh"}

	m, _ = m.Update(TopicThreadRepliesMsg{PostID: "p1", Replies: []model.Reply{{ID: "kept"}}})

	if _, ok := m.replyBodyCache["gone"]; ok {
		t.Error("expected replyBodyCache entry for a reply no longer in m.threadReplies to be evicted")
	}
	if _, ok := m.replyBodyCache["kept"]; !ok {
		t.Error("expected replyBodyCache entry for a reply still in m.threadReplies to survive")
	}
}
