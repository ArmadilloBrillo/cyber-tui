package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
	"github.com/ragnar/cyber-tui/internal/youtube"
)

// songField identifies which input has focus inside a SongPromptModel.
type songField int

const (
	songFieldURL songField = iota
	songFieldArtist
	songFieldTitle
	songFieldGenre
	songFieldCount
)

// Field limits mirror the API's documented constraints (v0.8.7) for a
// post/reply audio attachment — see docs/00-latest-api-reference.md
// ("Create Entry" → attachments).
const (
	songArtistCharLimit = 100
	songTitleCharLimit  = 150
	songGenreCharLimit  = 50
)

// SongPromptModel is the ctrl+j "attach song" modal for cIRC: a YouTube URL
// plus artist/title/genre, the latter two auto-filled (best-effort, via
// internal/youtube's oEmbed lookup — triggered by App, not this model, same
// split as the inline-image fetch pipeline) but always editable before
// submit. It never talks to the network itself, mirroring
// IconPickerModel/PathPromptModel staying pure UI.
type SongPromptModel struct {
	urlInput    textinput.Model
	artistInput textinput.Model
	titleInput  textinput.Model
	genreInput  textinput.Model
	focus       songField
	loading     bool   // set by App while a metadata fetch is in flight
	status      string // validation warning or fetch-outcome note; cleared on the next keystroke
}

// NewSongPromptModel builds an empty song-attach prompt.
func NewSongPromptModel() SongPromptModel {
	newField := func(placeholder string) textinput.Model {
		ti := textinput.New()
		ti.Placeholder = placeholder
		ti.Width = 40
		return ti
	}
	artist := newField("artist")
	artist.CharLimit = songArtistCharLimit
	title := newField("title")
	title.CharLimit = songTitleCharLimit
	genre := newField("genre (max 50, lowercase)")
	genre.CharLimit = songGenreCharLimit
	return SongPromptModel{
		urlInput:    newField("https://youtube.com/watch?v=..."),
		artistInput: artist,
		titleInput:  title,
		genreInput:  genre,
	}
}

// Open resets every field and focuses the URL input.
func (m SongPromptModel) Open() (SongPromptModel, tea.Cmd) {
	m.focus = songFieldURL
	m.loading = false
	m.status = ""
	m.urlInput.SetValue("")
	m.artistInput.SetValue("")
	m.titleInput.SetValue("")
	m.genreInput.SetValue("")
	m.artistInput.Blur()
	m.titleInput.Blur()
	m.genreInput.Blur()
	return m, m.urlInput.Focus()
}

func (m SongPromptModel) URLValue() string    { return strings.TrimSpace(m.urlInput.Value()) }
func (m SongPromptModel) ArtistValue() string { return strings.TrimSpace(m.artistInput.Value()) }
func (m SongPromptModel) TitleValue() string  { return strings.TrimSpace(m.titleInput.Value()) }
func (m SongPromptModel) GenreValue() string {
	return strings.ToLower(strings.TrimSpace(m.genreInput.Value()))
}

// OnURLField reports whether the URL field currently has focus — App uses
// this to decide whether Enter should trigger a metadata fetch (URL field)
// or just advance focus (every other field).
func (m SongPromptModel) OnURLField() bool { return m.focus == songFieldURL }

// OnLastField reports whether the last field (genre) has focus — App uses
// this to decide whether Enter should submit.
func (m SongPromptModel) OnLastField() bool { return m.focus == songFieldGenre }

// SetWarning attaches a validation message shown below the fields until the
// next keystroke — mirrors PathPromptModel.SetWarning.
func (m SongPromptModel) SetWarning(text string) SongPromptModel {
	m.status = text
	return m
}

// SetLoading marks a metadata fetch as in flight (or done), showing a
// "fetching…" line in place of any status message while true.
func (m SongPromptModel) SetLoading(loading bool) SongPromptModel {
	m.loading = loading
	if loading {
		m.status = ""
	}
	return m
}

// ApplyMetadata fills the artist/title fields from a successful fetch and
// clears the loading state.
func (m SongPromptModel) ApplyMetadata(title, artist string) SongPromptModel {
	m.loading = false
	m.titleInput.SetValue(title)
	m.artistInput.SetValue(artist)
	return m
}

// FetchFailed clears the loading state and leaves a note that auto-fill
// didn't work — the user can still fill artist/title by hand.
func (m SongPromptModel) FetchFailed() SongPromptModel {
	m.loading = false
	m.status = "couldn't auto-fill — enter artist/title manually"
	return m
}

// BuildAttachment validates the current field values and, if valid, returns
// a model.Attachment ready to send as a post/reply's native audio attachment
// — the same shape the API's chat audioAttachment field uses (type "audio",
// origin "youtube"). artist/title/genre are all required per the API's
// documented attachment shape (docs/00-latest-api-reference.md); length and
// case are enforced upstream via textinput.CharLimit and GenreValue's
// lowercasing. Shared validation for both this and BuildCommand — so the
// chat /song modal also requires genre now, even though its own text syntax
// still shows it bracketed as optional.
func (m SongPromptModel) BuildAttachment() (model.Attachment, bool) {
	url := m.URLValue()
	artist := m.ArtistValue()
	title := m.TitleValue()
	genre := m.GenreValue()
	if _, ok := youtube.ExtractVideoID(url); !ok {
		return model.Attachment{}, false
	}
	if artist == "" || title == "" || genre == "" {
		return model.Attachment{}, false
	}
	return model.Attachment{Type: "audio", Src: url, Origin: "youtube", Artist: artist, Title: title, Genre: genre}, true
}

// BuildCommand validates the current field values and, if valid, returns the
// "/song <url> | <artist> | <title> | <genre>" string ready to hand to the
// chat composer.
func (m SongPromptModel) BuildCommand() (string, bool) {
	att, ok := m.BuildAttachment()
	if !ok {
		return "", false
	}
	cmd := "/song " + att.Src + " | " + att.Artist + " | " + att.Title + " | " + att.Genre
	return cmd, true
}

// NextField advances focus by one field, wrapping from genre back to url —
// used for both Tab and an Enter that just moves on rather than submitting
// or fetching.
func (m SongPromptModel) NextField() (SongPromptModel, tea.Cmd) { return m.moveFocus(1) }

func (m SongPromptModel) moveFocus(delta int) (SongPromptModel, tea.Cmd) {
	switch m.focus {
	case songFieldURL:
		m.urlInput.Blur()
	case songFieldArtist:
		m.artistInput.Blur()
	case songFieldTitle:
		m.titleInput.Blur()
	case songFieldGenre:
		m.genreInput.Blur()
	}
	m.focus = songField((int(m.focus) + delta + int(songFieldCount)) % int(songFieldCount))
	var cmd tea.Cmd
	switch m.focus {
	case songFieldURL:
		cmd = m.urlInput.Focus()
	case songFieldArtist:
		cmd = m.artistInput.Focus()
	case songFieldTitle:
		cmd = m.titleInput.Focus()
	case songFieldGenre:
		cmd = m.genreInput.Focus()
	}
	return m, cmd
}

// Update handles every key except esc/enter/ctrl+s, which App intercepts
// itself (see App.handleSongPromptKey) — mirrors IconPickerModel's split
// between model-local keys and App-level dismissal/submission.
func (m SongPromptModel) Update(msg tea.KeyMsg) (SongPromptModel, tea.Cmd) {
	switch msg.String() {
	case "tab":
		return m.moveFocus(1)
	case "shift+tab":
		return m.moveFocus(-1)
	}
	m.status = ""
	var cmd tea.Cmd
	switch m.focus {
	case songFieldURL:
		m.urlInput, cmd = m.urlInput.Update(msg)
	case songFieldArtist:
		m.artistInput, cmd = m.artistInput.Update(msg)
	case songFieldTitle:
		m.titleInput, cmd = m.titleInput.Update(msg)
	case songFieldGenre:
		m.genreInput, cmd = m.genreInput.Update(msg)
	}
	return m, cmd
}

func (m SongPromptModel) View() string {
	title := theme.Title.Render("Attach Song")
	label := func(s string) string { return theme.Subtle.Render(fmt.Sprintf("%-8s› ", s)) }

	rows := []string{
		title, "",
		label("url") + m.urlInput.View(),
		label("artist") + m.artistInput.View(),
		label("title") + m.titleInput.View(),
		label("genre") + m.genreInput.View(),
	}
	switch {
	case m.loading:
		rows = append(rows, "", theme.Subtle.Render("fetching metadata…"))
	case m.status != "":
		rows = append(rows, "", theme.Error.Render(m.status))
	}
	rows = append(rows, "", theme.Subtle.Render("tab next field   enter on url · fetch   enter on genre / ctrl+s · send   esc cancel"))
	return theme.ActiveBorder.Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}
