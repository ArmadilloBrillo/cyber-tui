package screens

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func typeInto(m SongPromptModel, s string) SongPromptModel {
	for _, r := range s {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func TestSongPromptModel_Open_FocusesURLAndClearsFields(t *testing.T) {
	m := NewSongPromptModel()
	m = typeInto(m, "stale")
	m, _ = m.Open()

	if !m.OnURLField() {
		t.Error("expected Open to focus the url field")
	}
	if m.URLValue() != "" || m.ArtistValue() != "" || m.TitleValue() != "" || m.GenreValue() != "" {
		t.Errorf("expected all fields cleared after Open, got url=%q artist=%q title=%q genre=%q",
			m.URLValue(), m.ArtistValue(), m.TitleValue(), m.GenreValue())
	}
}

func TestSongPromptModel_Tab_CyclesThroughAllFieldsAndWraps(t *testing.T) {
	m, _ := NewSongPromptModel().Open()
	if !m.OnURLField() {
		t.Fatal("setup: expected to start on url field")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> artist
	if m.OnURLField() || m.OnLastField() {
		t.Errorf("after 1 tab: expected artist field, OnURLField=%v OnLastField=%v", m.OnURLField(), m.OnLastField())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> title
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> genre
	if !m.OnLastField() {
		t.Fatal("after 3 tabs: expected genre (last) field")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // wraps -> url
	if !m.OnURLField() {
		t.Error("expected tab from the last field to wrap back to url")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab}) // wraps backward -> genre
	if !m.OnLastField() {
		t.Error("expected shift+tab from url to wrap back to genre")
	}
}

func TestSongPromptModel_Update_TypesIntoFocusedFieldOnly(t *testing.T) {
	m, _ := NewSongPromptModel().Open()
	m = typeInto(m, "https://youtu.be/dQw4w9WgXcQ")
	if m.URLValue() != "https://youtu.be/dQw4w9WgXcQ" {
		t.Fatalf("URLValue() = %q, want the typed url", m.URLValue())
	}

	m, _ = m.NextField()
	m = typeInto(m, "Rick Astley")
	if m.ArtistValue() != "Rick Astley" {
		t.Errorf("ArtistValue() = %q, want %q", m.ArtistValue(), "Rick Astley")
	}
	if m.URLValue() != "https://youtu.be/dQw4w9WgXcQ" {
		t.Errorf("typing into artist changed url field to %q", m.URLValue())
	}
}

func TestSongPromptModel_BuildCommand(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		artist  string
		title   string
		genre   string
		wantCmd string
		wantOK  bool
	}{
		{
			name: "full", url: "https://youtu.be/dQw4w9WgXcQ", artist: "Rick Astley",
			title: "Never Gonna Give You Up", genre: "pop",
			wantCmd: "/song https://youtu.be/dQw4w9WgXcQ | Rick Astley | Never Gonna Give You Up | pop",
			wantOK:  true,
		},
		{
			name: "genre omitted when blank", url: "https://youtu.be/dQw4w9WgXcQ", artist: "Rick Astley",
			title:   "Never Gonna Give You Up",
			wantCmd: "/song https://youtu.be/dQw4w9WgXcQ | Rick Astley | Never Gonna Give You Up",
			wantOK:  true,
		},
		{name: "missing artist", url: "https://youtu.be/dQw4w9WgXcQ", title: "Title", wantOK: false},
		{name: "missing title", url: "https://youtu.be/dQw4w9WgXcQ", artist: "Artist", wantOK: false},
		{name: "invalid url", url: "https://vimeo.com/12345", artist: "Artist", title: "Title", wantOK: false},
		{name: "empty url", artist: "Artist", title: "Title", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := NewSongPromptModel().Open()
			m = typeInto(m, tt.url)
			m, _ = m.NextField()
			m = typeInto(m, tt.artist)
			m, _ = m.NextField()
			m = typeInto(m, tt.title)
			m, _ = m.NextField()
			m = typeInto(m, tt.genre)

			cmd, ok := m.BuildCommand()
			if ok != tt.wantOK {
				t.Fatalf("BuildCommand() ok = %v, want %v (cmd=%q)", ok, tt.wantOK, cmd)
			}
			if ok && cmd != tt.wantCmd {
				t.Errorf("BuildCommand() = %q, want %q", cmd, tt.wantCmd)
			}
		})
	}
}

func TestSongPromptModel_ApplyMetadata_FillsArtistAndTitleAndClearsLoading(t *testing.T) {
	m, _ := NewSongPromptModel().Open()
	m = m.SetLoading(true)

	m = m.ApplyMetadata("Never Gonna Give You Up", "Rick Astley")

	if m.loading {
		t.Error("expected ApplyMetadata to clear loading")
	}
	if m.TitleValue() != "Never Gonna Give You Up" || m.ArtistValue() != "Rick Astley" {
		t.Errorf("got title=%q artist=%q, want the fetched values", m.TitleValue(), m.ArtistValue())
	}
}

func TestSongPromptModel_FetchFailed_ClearsLoadingAndSetsNote(t *testing.T) {
	m, _ := NewSongPromptModel().Open()
	m = m.SetLoading(true)

	m = m.FetchFailed()

	if m.loading {
		t.Error("expected FetchFailed to clear loading")
	}
	if m.status == "" {
		t.Error("expected FetchFailed to leave a status note")
	}
}

func TestSongPromptModel_Update_KeystrokeClearsStatus(t *testing.T) {
	m, _ := NewSongPromptModel().Open()
	m = m.SetWarning("not a recognized YouTube URL")

	m = typeInto(m, "h")

	if m.status != "" {
		t.Errorf("expected a keystroke to clear the status message, got %q", m.status)
	}
}
