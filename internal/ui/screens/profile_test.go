package screens_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/screens"
)

func testUser() model.User {
	return model.User{
		ID:             "uid-123",
		Username:       "ragnar",
		Bio:            "test bio",
		FollowersCount: 35,
		FollowingCount: 45,
	}
}

// --- SetFollowState ---

func TestProfileSetFollowState_True(t *testing.T) {
	// SetFollowState(true) → pressing 'f' should emit UnfollowUserMsg (not FollowUserMsg).
	m := screens.NewProfileModel().SetUser(testUser()).SetReadOnly(true)
	m = m.SetFollowState(true, "follow-id-abc")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if cmd == nil {
		t.Fatal("expected a cmd when pressing f while following")
	}
	if _, ok := cmd().(screens.UnfollowUserMsg); !ok {
		t.Errorf("expected UnfollowUserMsg when isFollowing=true, got %T", cmd())
	}
}

func TestProfileSetFollowState_False(t *testing.T) {
	// SetFollowState(false) → pressing 'f' should emit FollowUserMsg (not UnfollowUserMsg).
	m := screens.NewProfileModel().SetUser(testUser()).SetReadOnly(true)
	m = m.SetFollowState(false, "")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if cmd == nil {
		t.Fatal("expected a cmd when pressing f while not following")
	}
	if _, ok := cmd().(screens.FollowUserMsg); !ok {
		t.Errorf("expected FollowUserMsg when isFollowing=false, got %T", cmd())
	}
}

// --- counts display ---

func TestProfileView_CountsDisplayed(t *testing.T) {
	m := screens.NewProfileModel().SetUser(testUser())
	m, _ = m.Update(screens.SharedConfigMsg{Settings: model.Settings{ShowFollowerCount: true}})
	view := m.View()

	if !strings.Contains(view, "35 followers") {
		t.Errorf("View should contain '35 followers', got:\n%s", view)
	}
	if !strings.Contains(view, "45 following") {
		t.Errorf("View should contain '45 following', got:\n%s", view)
	}
}

func TestProfileView_CountsHidden(t *testing.T) {
	m := screens.NewProfileModel().SetUser(testUser())
	// showFollowerCount defaults to false — counts must not appear
	view := m.View()

	if strings.Contains(view, "followers") {
		t.Errorf("View should not contain 'followers' when showFollowerCount=false, got:\n%s", view)
	}
}

// --- 'f' key — follow ---

func TestProfileUpdate_FKey_EmitsFollowMsg_WhenNotFollowing(t *testing.T) {
	m := screens.NewProfileModel().SetUser(testUser()).SetReadOnly(true).SetFollowState(false, "")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	_ = updated
	if cmd == nil {
		t.Fatal("expected a cmd, got nil")
	}
	msg := cmd()
	followMsg, ok := msg.(screens.FollowUserMsg)
	if !ok {
		t.Fatalf("expected FollowUserMsg, got %T", msg)
	}
	if followMsg.UserID != "uid-123" {
		t.Errorf("UserID = %q, want uid-123", followMsg.UserID)
	}
}

// --- 'f' key — unfollow ---

func TestProfileUpdate_FKey_EmitsUnfollowMsg_WhenFollowing(t *testing.T) {
	m := screens.NewProfileModel().SetUser(testUser()).SetReadOnly(true).SetFollowState(true, "follow-id-abc")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	_ = updated
	if cmd == nil {
		t.Fatal("expected a cmd, got nil")
	}
	msg := cmd()
	unfollowMsg, ok := msg.(screens.UnfollowUserMsg)
	if !ok {
		t.Fatalf("expected UnfollowUserMsg, got %T", msg)
	}
	if unfollowMsg.FollowID != "follow-id-abc" {
		t.Errorf("FollowID = %q, want follow-id-abc", unfollowMsg.FollowID)
	}
}

// --- 'p' key — poke ---

func TestProfileUpdate_PKey_EmitsPokeMsg_WhenReadOnly(t *testing.T) {
	m := screens.NewProfileModel().SetUser(testUser()).SetReadOnly(true)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	_ = updated
	if cmd == nil {
		t.Fatal("expected a cmd, got nil")
	}
	msg := cmd()
	pokeMsg, ok := msg.(screens.PokeUserMsg)
	if !ok {
		t.Fatalf("expected PokeUserMsg, got %T", msg)
	}
	if pokeMsg.Username != "ragnar" {
		t.Errorf("Username = %q, want ragnar", pokeMsg.Username)
	}
}

func TestProfileUpdate_PKey_NoOp_OnOwnProfile(t *testing.T) {
	m := screens.NewProfileModel().SetUser(testUser()).SetReadOnly(false)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if cmd != nil {
		if _, ok := cmd().(screens.PokeUserMsg); ok {
			t.Error("did not expect PokeUserMsg on own (non-read-only) profile")
		}
	}
}

// --- IncrementFollowersCount ---

func TestProfileIncrementFollowersCount(t *testing.T) {
	m := screens.NewProfileModel().SetUser(testUser()) // FollowersCount = 35
	m, _ = m.Update(screens.SharedConfigMsg{Settings: model.Settings{ShowFollowerCount: true}})
	m = m.IncrementFollowersCount(1)

	view := m.View()
	if !strings.Contains(view, "36 followers") {
		t.Errorf("View should contain '36 followers' after increment, got:\n%s", view)
	}

	m = m.IncrementFollowersCount(-1)
	view = m.View()
	if !strings.Contains(view, "35 followers") {
		t.Errorf("View should contain '35 followers' after decrement, got:\n%s", view)
	}
}

// --- follow feedback ---

func TestProfileSetFollowFeedback(t *testing.T) {
	m := screens.NewProfileModel().SetUser(testUser()).SetFollowFeedback("following.")

	view := m.View()
	if !strings.Contains(view, "following.") {
		t.Errorf("View should contain 'following.', got:\n%s", view)
	}
}

// --- own profile: 'f' key should do nothing ---

func TestProfileUpdate_FKey_NoEffect_OwnProfile(t *testing.T) {
	// readOnly=false means it's the user's own profile — 'f' should emit nothing.
	m := screens.NewProfileModel().SetUser(testUser()).SetReadOnly(false)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if cmd != nil {
		t.Errorf("own profile should not emit a cmd for 'f', got non-nil cmd")
	}
}

// --- new profile fields: display ---

func testUserFull() model.User {
	u := testUser()
	u.WebsiteName = "My Blog"
	u.WebsiteUrl = "https://ragnar.dev"
	u.WebsiteImageUrl = "https://ragnar.dev/avatar.png"
	u.LocationName = "Cape Town"
	u.LocationLatitude = -33.9249
	u.LocationLongitude = 18.4241
	return u
}

func TestProfileView_WebsiteShown(t *testing.T) {
	m := screens.NewProfileModel().SetUser(testUserFull())
	view := m.View()
	if !strings.Contains(view, "My Blog") {
		t.Errorf("View should contain websiteName, got:\n%s", view)
	}
	if !strings.Contains(view, "ragnar.dev") {
		t.Errorf("View should contain websiteUrl, got:\n%s", view)
	}
}

func TestProfileView_WebsiteImageUrlShown(t *testing.T) {
	m := screens.NewProfileModel().SetUser(testUserFull())
	view := m.View()
	if !strings.Contains(view, "avatar.png") {
		t.Errorf("View should contain websiteImageUrl, got:\n%s", view)
	}
}

func TestProfileView_LocationShown(t *testing.T) {
	m := screens.NewProfileModel().SetUser(testUserFull())
	view := m.View()
	if !strings.Contains(view, "Cape Town") {
		t.Errorf("View should contain locationName, got:\n%s", view)
	}
	if !strings.Contains(view, "-33.9249") {
		t.Errorf("View should contain latitude, got:\n%s", view)
	}
}

// --- edit form ---

// sendProfileKey fires a single key through ProfileModel.Update.
func sendProfileKey(m screens.ProfileModel, key string) (screens.ProfileModel, tea.Cmd) {
	var msg tea.KeyMsg
	switch key {
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		msg = tea.KeyMsg{Type: tea.KeyShiftTab}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+s":
		msg = tea.KeyMsg{Type: tea.KeyCtrlS}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	return m.Update(msg)
}

func openEditForm(t *testing.T) screens.ProfileModel {
	t.Helper()
	m := screens.NewProfileModel().SetUser(testUserFull()).SetReadOnly(false)
	m, _ = sendProfileKey(m, "e")
	if !m.ComposeActive() {
		t.Fatal("expected ComposeActive after pressing e")
	}
	return m
}

func TestProfileEditForm_Opens(t *testing.T) {
	m := openEditForm(t)
	view := m.View()
	if !strings.Contains(view, "Website Name") {
		t.Errorf("Edit form should contain 'Website Name', got:\n%s", view)
	}
	if !strings.Contains(view, "ctrl+s · save") {
		t.Errorf("Edit form should contain hint, got:\n%s", view)
	}
}

func TestProfileEditForm_PrepopulatesFields(t *testing.T) {
	m := openEditForm(t)
	view := m.View()
	if !strings.Contains(view, "My Blog") {
		t.Errorf("Edit form should prepopulate websiteName, got:\n%s", view)
	}
	if !strings.Contains(view, "Cape Town") {
		t.Errorf("Edit form should prepopulate locationName, got:\n%s", view)
	}
}

func TestProfileEditForm_TabCyclesFields(t *testing.T) {
	m := openEditForm(t)
	// e opens focused on bio (field 1); tab should move to field 2
	for i := 0; i < 8; i++ {
		m, _ = sendProfileKey(m, "tab")
	}
	// After 8 tabs from field 1, we're back at field 1 (bio)
	if !m.ComposeActive() {
		t.Error("expected ComposeActive to remain true after full tab cycle")
	}
}

func TestProfileEditForm_EscCloses(t *testing.T) {
	m := openEditForm(t)
	// Tab to a textinput field first (field 2 = websiteName), then Esc.
	m, _ = sendProfileKey(m, "tab")
	m, _ = sendProfileKey(m, "esc")
	if m.ComposeActive() {
		t.Error("expected ComposeActive to be false after Esc on textinput field")
	}
}

func TestProfileEditForm_CtrlS_EmitsSaveMsg(t *testing.T) {
	m := openEditForm(t)
	// Tab to a textinput field.
	m, _ = sendProfileKey(m, "tab")
	m2, cmd := sendProfileKey(m, "ctrl+s")
	_ = m2
	if cmd == nil {
		t.Fatal("expected a cmd after Ctrl+S, got nil")
	}
	msg := cmd()
	saveMsg, ok := msg.(screens.SaveProfileMsg)
	if !ok {
		t.Fatalf("expected SaveProfileMsg, got %T", msg)
	}
	if saveMsg.WebsiteName != "My Blog" {
		t.Errorf("SaveProfileMsg.WebsiteName = %q, want My Blog", saveMsg.WebsiteName)
	}
	if saveMsg.LocationName != "Cape Town" {
		t.Errorf("SaveProfileMsg.LocationName = %q, want Cape Town", saveMsg.LocationName)
	}
}

func TestProfileEditForm_ComposeSubmit_EmitsSaveMsg(t *testing.T) {
	m := openEditForm(t)
	// bio is focused; send ComposeSubmitMsg directly (as the compose model would).
	m2, cmd := m.Update(screens.ComposeSubmitMsg{Content: "new bio"})
	_ = m2
	if cmd == nil {
		t.Fatal("expected a cmd after ComposeSubmitMsg, got nil")
	}
	msg := cmd()
	saveMsg, ok := msg.(screens.SaveProfileMsg)
	if !ok {
		t.Fatalf("expected SaveProfileMsg, got %T", msg)
	}
	if saveMsg.Bio != "new bio" {
		t.Errorf("SaveProfileMsg.Bio = %q, want new bio", saveMsg.Bio)
	}
}

func TestProfileEditForm_ComposeCancel_ClosesForm(t *testing.T) {
	m := openEditForm(t)
	m2, _ := m.Update(screens.ComposeCancelMsg{})
	if m2.ComposeActive() {
		t.Error("expected ComposeActive to be false after ComposeCancelMsg")
	}
}

// --- FilterNSFW ---

func profileWithPosts(posts []model.Post) screens.ProfileModel {
	m := screens.NewProfileModel().SetUser(testUser()).SetReadOnly(true)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = m.SetUserPosts(posts, "")
	// Switch to Posts tab (Info→Posts is one Tab press in view mode)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	return m
}

func TestProfile_FilterNSFW_HidesNSFWPost(t *testing.T) {
	posts := []model.Post{
		{ID: "pp1", AuthorUsername: "ragnar", Content: "safe"},
		{ID: "pp2", AuthorUsername: "ragnar", Content: "nsfw", IsNSFW: true},
		{ID: "pp3", AuthorUsername: "ragnar", Content: "also safe"},
	}
	m := profileWithPosts(posts)
	m, _ = m.Update(nsfwFilterMsg(true))

	// Navigate to end of visible list (2 visible, max index 1)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	// Enter should return pp3, not pp2
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a cmd on enter")
	}
	msg := cmd()
	sp, ok := msg.(screens.ShowProfilePostMsg)
	if !ok {
		t.Fatalf("expected ShowProfilePostMsg, got %T", msg)
	}
	if sp.Post.ID != "pp3" {
		t.Errorf("expected pp3 (safe), got %s", sp.Post.ID)
	}
}

func TestProfile_FilterNSFW_Off_ShowsAll(t *testing.T) {
	posts := []model.Post{
		{ID: "pp1", AuthorUsername: "ragnar", Content: "safe"},
		{ID: "pp2", AuthorUsername: "ragnar", Content: "nsfw", IsNSFW: true},
	}
	m := profileWithPosts(posts)
	m, _ = m.Update(nsfwFilterMsg(false))

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a cmd on enter")
	}
	msg := cmd()
	sp, ok := msg.(screens.ShowProfilePostMsg)
	if !ok {
		t.Fatalf("expected ShowProfilePostMsg, got %T", msg)
	}
	if sp.Post.ID != "pp2" {
		t.Errorf("expected pp2 (nsfw), got %s", sp.Post.ID)
	}
}

// --- pagination in-flight guard ---

// TestProfile_LoadMore_SkipsWhileInFlight reproduces repeated down-presses
// near the bottom of a list tab (arrow-repeat / held key). Without an
// in-flight guard, each press re-dispatches a LoadMore*Msg carrying the same
// stale cursor before the first request's response lands, causing the
// eventual responses to be appended more than once (duplicate items).
func TestProfile_LoadMore_SkipsWhileInFlight(t *testing.T) {
	posts := []model.Post{
		{ID: "pp1", AuthorUsername: "ragnar", Content: "one"},
		{ID: "pp2", AuthorUsername: "ragnar", Content: "two"},
		{ID: "pp3", AuthorUsername: "ragnar", Content: "three"},
		{ID: "pp4", AuthorUsername: "ragnar", Content: "four"},
		{ID: "pp5", AuthorUsername: "ragnar", Content: "five"},
		{ID: "pp6", AuthorUsername: "ragnar", Content: "six"},
	}
	m := screens.NewProfileModel().SetUser(testUser()).SetReadOnly(true)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = m.SetUserPosts(posts, "next-cursor") // non-empty cursor: more pages available
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Move to the last 3 items (n-3) to enter pagination range, then keep
	// pressing down as if the key were held/repeated.
	var cmds []tea.Cmd
	for i := 0; i < 5; i++ {
		var cmd tea.Cmd
		m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		cmds = append(cmds, cmd)
	}

	fired := 0
	for _, cmd := range cmds {
		if cmd == nil {
			continue
		}
		if _, ok := cmd().(screens.LoadMoreUserPostsMsg); ok {
			fired++
		}
	}
	if fired != 1 {
		t.Errorf("expected exactly 1 LoadMoreUserPostsMsg while a fetch is in flight, got %d", fired)
	}
}
