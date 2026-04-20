package screens

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/markdown"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// confirmKind tracks which action is awaiting user confirmation.
type confirmKind int

const (
	confirmNone    confirmKind = iota
	confirmPublish             // ctrl+p: publish note as a post
	confirmDelete              // d: delete note
)

// JournalModel is the screen for viewing and editing private notes (Journal tab).
// In list mode the user browses notes. Pressing n or enter opens edit mode,
// where the compose box and topics input are used to write or update a note.
// ctrl+s saves, ctrl+p publishes (with confirmation), d deletes (with confirmation).
// Pressing h in list mode loads the revision history for the selected note.
type JournalModel struct {
	notes      []model.Note
	nextCursor string
	exhausted  bool
	loading    bool

	selectedIdx  int
	noteOffsets  []int // start line of each note within viewport content
	editMode     bool  // true while compose is open
	isNewNote    bool  // true = create, false = update existing
	editingID    string

	compose       ComposeModel
	topicsInput   textinput.Model
	topicsFocused bool

	confirming confirmKind

	// Revision history state.
	revisionsMode   bool                // true when viewing revision history
	revisions       []model.NoteRevision
	revisionsCursor string
	revisionsNoteID string
	revSelectedIdx  int
	revPreview      *model.Note // non-nil while previewing a specific revision

	viewport viewport.Model
	ready    bool
	width    int
	height   int

	err               error
	timeDisplayFormat string
	loc               *time.Location
	relaxed           bool
}

func NewJournalModel(width int) JournalModel {
	ti := textinput.New()
	ti.Placeholder = "add topics  (journal, idea, …  max 3)"
	return JournalModel{
		compose:     NewComposeModel(width),
		topicsInput: ti,
		loc:         time.UTC,
	}
}

// SetNotes replaces the note list (first load / refresh) and resets scroll.
func (m JournalModel) SetNotes(notes []model.Note, cursor string) JournalModel {
	m.notes = notes
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.loading = false
	m.selectedIdx = 0
	m.err = nil
	if m.ready {
		m = m.refreshContent()
		m.viewport.GotoTop()
	}
	return m
}

// AppendNotes adds a page of notes to the existing list (pagination).
func (m JournalModel) AppendNotes(notes []model.Note, cursor string) JournalModel {
	m.notes = append(m.notes, notes...)
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.loading = false
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// PrependNote inserts a newly created note at the top of the list.
func (m JournalModel) PrependNote(note model.Note) JournalModel {
	m.notes = append([]model.Note{note}, m.notes...)
	m.selectedIdx = 0
	if m.ready {
		m = m.refreshContent()
		m.viewport.GotoTop()
	}
	return m
}

// UpdateNoteContent replaces the content of an existing note in the local list.
func (m JournalModel) UpdateNoteContent(noteID, content string, topics []string) JournalModel {
	for i, n := range m.notes {
		if n.ID == noteID {
			m.notes[i].Content = content
			m.notes[i].Topics = topics
			m.notes[i].RevisionNumber++
			break
		}
	}
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// DeleteNote removes a note from the local list by ID.
func (m JournalModel) DeleteNote(noteID string) JournalModel {
	for i, n := range m.notes {
		if n.ID == noteID {
			m.notes = append(m.notes[:i], m.notes[i+1:]...)
			if m.selectedIdx >= len(m.notes) && m.selectedIdx > 0 {
				m.selectedIdx = len(m.notes) - 1
			}
			break
		}
	}
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// SetRevisions stores revision history for a note and switches to revisions mode.
func (m JournalModel) SetRevisions(noteID string, revs []model.NoteRevision, cursor string) JournalModel {
	m.revisionsMode = true
	m.revisionsNoteID = noteID
	m.revisions = revs
	m.revisionsCursor = cursor
	m.revSelectedIdx = 0
	m.revPreview = nil
	if m.ready {
		m = m.refreshRevisionsContent()
	}
	return m
}

// SetRevisionPreview stores a specific revision's content for preview display.
func (m JournalModel) SetRevisionPreview(note model.Note) JournalModel {
	n := note
	m.revPreview = &n
	if m.ready {
		m = m.refreshRevisionsContent()
	}
	return m
}

// SetError stores an error to display in the view.
func (m JournalModel) SetError(err error) JournalModel {
	m.err = err
	m.loading = false
	return m
}

func (m JournalModel) location() *time.Location {
	if m.loc == nil {
		return time.UTC
	}
	return m.loc
}

// ComposeActive reports whether the edit compose box is open (used by App to
// bypass global key shortcuts when a text input has focus).
func (m JournalModel) ComposeActive() bool { return m.editMode }

func (m JournalModel) Init() tea.Cmd { return nil }

func (m JournalModel) Update(msg tea.Msg) (JournalModel, tea.Cmd) {
	switch msg := msg.(type) {
	case SharedConfigMsg:
		m.timeDisplayFormat = msg.Settings.TimeDisplayFormat
		if msg.Loc != nil {
			m.loc = msg.Loc
		}
		m.relaxed = msg.Relaxed
		m.width = msg.Width
		m.height = msg.Height
		m.compose = m.compose.SetWidth(msg.Width)
		innerW := msg.Width - 4
		if innerW < 1 {
			innerW = 1
		}
		m.topicsInput.Width = innerW
		if m.ready {
			m.viewport.Width = msg.Width
			m.viewport.Height = m.viewportHeight()
			m = m.refreshContent()
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.compose = m.compose.SetWidth(msg.Width)
		innerW := msg.Width - 4
		if innerW < 1 {
			innerW = 1
		}
		m.topicsInput.Width = innerW
		if !m.ready {
			m.viewport = viewport.New(msg.Width, m.viewportHeight())
			m = m.refreshContent()
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = m.viewportHeight()
			m = m.refreshContent()
		}
		return m, nil

	case tea.KeyMsg:
		// Confirmation overlay intercepts all keys while active.
		if m.confirming != confirmNone {
			return m.handleConfirmKey(msg)
		}

		if m.revisionsMode {
			return m.handleRevisionsKey(msg)
		}

		if m.editMode {
			return m.handleEditKey(msg)
		}

		return m.handleListKey(msg)
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)

	if m.ready && !m.loading && !m.exhausted && m.viewport.AtBottom() && m.nextCursor != "" {
		m.loading = true
		cursor := m.nextCursor
		return m, tea.Batch(cmd, func() tea.Msg {
			return LoadMoreJournalMsg{Cursor: cursor}
		})
	}

	return m, cmd
}

// handleConfirmKey processes y/n/esc while a confirmation prompt is shown.
func (m JournalModel) handleConfirmKey(msg tea.KeyMsg) (JournalModel, tea.Cmd) {
	switch msg.String() {
	case "y":
		action := m.confirming
		m.confirming = confirmNone
		switch action {
		case confirmPublish:
			content := m.compose.Content()
			topics := ParseTopics(m.topicsInput.Value())
			m = m.closeEdit() // closeEdit resets viewport height
			return m, func() tea.Msg {
				return SubmitPublishNoteMsg{Content: content, Topics: topics}
			}
		case confirmDelete:
			if m.selectedIdx < len(m.notes) {
				noteID := m.notes[m.selectedIdx].ID
				m.viewport.Height = m.viewportHeight()
				return m, func() tea.Msg { return SubmitDeleteNoteMsg{NoteID: noteID} }
			}
		}
	case "n", "esc":
		m.confirming = confirmNone
		m.viewport.Height = m.viewportHeight()
	}
	return m, nil
}

// handleEditKey processes keys while the compose box is open.
// ctrl+s and ctrl+p are intercepted here before compose sees them.
func (m JournalModel) handleEditKey(msg tea.KeyMsg) (JournalModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+p":
		if m.compose.Content() != "" {
			m.confirming = confirmPublish
			m.viewport.Height = m.viewportHeight()
		}
		return m, nil

	case "ctrl+s":
		if m.topicsFocused {
			// Save from topics input focus.
			return m.submitSave()
		}
		// Intercept before compose so it doesn't emit ComposeSubmitMsg.
		return m.submitSave()

	case "esc":
		if m.topicsFocused {
			m.topicsFocused = false
			m.topicsInput.Blur()
			var cmd tea.Cmd
			m.compose, cmd = m.compose.SetFocused(true)
			return m, cmd
		}
		m = m.closeEdit()
		return m, nil

	case "tab":
		if m.topicsFocused {
			m.topicsFocused = false
			m.topicsInput.Blur()
			var cmd tea.Cmd
			m.compose, cmd = m.compose.SetFocused(true)
			return m, cmd
		}
		m.topicsFocused = true
		m.compose, _ = m.compose.SetFocused(false)
		cmd := m.topicsInput.Focus()
		return m, cmd
	}

	if m.topicsFocused {
		var cmd tea.Cmd
		filtered, ok := filterAmbiguousKeyMsg(msg)
		if !ok {
			return m, nil
		}
		m.topicsInput, cmd = m.topicsInput.Update(filtered)
		return m, cmd
	}

	prevBox := m.compose.BoxHeight()
	var cmd tea.Cmd
	m.compose, cmd = m.compose.Update(msg)
	// Recalculate viewport height if the compose box grew or shrank.
	if m.compose.BoxHeight() != prevBox {
		m.viewport.Height = m.viewportHeight()
	}
	return m, cmd
}

// submitSave emits a SubmitSaveNoteMsg for create or update.
func (m JournalModel) submitSave() (JournalModel, tea.Cmd) {
	content := m.compose.Content()
	topics := ParseTopics(m.topicsInput.Value())
	noteID := m.editingID // empty if new note
	isNew := m.isNewNote
	m = m.closeEdit()
	if isNew {
		return m, func() tea.Msg {
			return SubmitSaveNoteMsg{NoteID: "", Content: content, Topics: topics}
		}
	}
	return m, func() tea.Msg {
		return SubmitSaveNoteMsg{NoteID: noteID, Content: content, Topics: topics}
	}
}

// handleListKey processes keys while browsing the note list.
func (m JournalModel) handleListKey(msg tea.KeyMsg) (JournalModel, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.selectedIdx > 0 {
			m.selectedIdx--
			m = m.refreshContent()
			m = m.ensureSelectedVisible()
		}
		return m, nil

	case "down", "j":
		if m.selectedIdx < len(m.notes)-1 {
			m.selectedIdx++
			m = m.refreshContent()
			m = m.ensureSelectedVisible()
		} else if !m.loading && !m.exhausted && m.nextCursor != "" {
			m.loading = true
			cursor := m.nextCursor
			return m, func() tea.Msg { return LoadMoreJournalMsg{Cursor: cursor} }
		}
		return m, nil

	case "enter":
		if len(m.notes) > 0 && m.selectedIdx < len(m.notes) {
			return m.openNote(m.notes[m.selectedIdx])
		}
		return m, nil

	case "n":
		return m.openNewNote()

	case "d":
		if len(m.notes) > 0 && m.selectedIdx < len(m.notes) {
			m.confirming = confirmDelete
			m.viewport.Height = m.viewportHeight()
		}
		return m, nil

	case "h":
		if len(m.notes) > 0 && m.selectedIdx < len(m.notes) {
			noteID := m.notes[m.selectedIdx].ID
			return m, func() tea.Msg { return LoadNoteRevisionsMsg{NoteID: noteID} }
		}
		return m, nil
	}
	return m, nil
}

// handleRevisionsKey processes keys while the revision history view is active.
func (m JournalModel) handleRevisionsKey(msg tea.KeyMsg) (JournalModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.revPreview != nil {
			// Exit preview → back to revision list.
			m.revPreview = nil
			if m.ready {
				m = m.refreshRevisionsContent()
			}
			return m, nil
		}
		// Exit revision mode → back to note list.
		m.revisionsMode = false
		m.revisions = nil
		m.revisionsCursor = ""
		m.revisionsNoteID = ""
		m.revPreview = nil
		if m.ready {
			m = m.refreshContent()
		}
		return m, nil

	case "j", "down":
		if m.revPreview != nil {
			// Scroll preview viewport.
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		if m.revSelectedIdx < len(m.revisions)-1 {
			m.revSelectedIdx++
			if m.ready {
				m = m.refreshRevisionsContent()
			}
		}
		return m, nil

	case "k", "up":
		if m.revPreview != nil {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		if m.revSelectedIdx > 0 {
			m.revSelectedIdx--
			if m.ready {
				m = m.refreshRevisionsContent()
			}
		}
		return m, nil

	case "enter":
		if m.revPreview != nil {
			return m, nil
		}
		if len(m.revisions) > 0 && m.revSelectedIdx < len(m.revisions) {
			rev := m.revisions[m.revSelectedIdx]
			noteID := m.revisionsNoteID
			revNum := rev.RevisionNumber
			return m, func() tea.Msg {
				return LoadNoteRevisionMsg{NoteID: noteID, RevisionNumber: revNum}
			}
		}
		return m, nil
	}
	return m, nil
}

// refreshRevisionsContent rebuilds the viewport content for the revisions view.
func (m JournalModel) refreshRevisionsContent() JournalModel {
	if m.revPreview != nil {
		m.viewport.SetContent(m.buildRevisionPreviewContent())
	} else {
		m.viewport.SetContent(m.buildRevisionListContent())
	}
	m.viewport.GotoTop()
	return m
}

// buildRevisionListContent renders the list of revisions.
func (m JournalModel) buildRevisionListContent() string {
	if len(m.revisions) == 0 {
		return theme.Subtle.Render("  no revisions found.")
	}

	// Find the note title for the header.
	noteTitle := ""
	for _, n := range m.notes {
		if n.ID == m.revisionsNoteID {
			noteTitle = markdown.FirstLine(n.Content)
			break
		}
	}
	if len([]rune(noteTitle)) > 40 {
		noteTitle = string([]rune(noteTitle)[:39]) + "…"
	}

	var sb strings.Builder
	sb.WriteString(theme.Title.Render("Revisions") + " — " + theme.Subtle.Render(noteTitle) + "\n\n")

	for i, rev := range m.revisions {
		selected := i == m.revSelectedIdx
		ts := rev.CreatedAt.Format("02-Jan-2006 15:04")
		label := fmt.Sprintf("Rev %d", rev.RevisionNumber)
		if i == 0 {
			label += " (latest)"
		}
		preview := markdown.FirstLine(rev.Content)
		if len([]rune(preview)) > 50 {
			preview = string([]rune(preview)[:49]) + "…"
		}

		boxStyle := theme.Border
		if selected {
			boxStyle = theme.ActiveBorder
		}
		if m.width > 2 {
			boxStyle = boxStyle.Width(m.width - 2)
		}
		row := theme.Highlight.Render(label) + "  " + theme.Subtle.Render(ts) + "\n" +
			theme.Base.Render(preview)
		sb.WriteString(boxStyle.Render(row))
		sb.WriteString("\n")
	}

	sb.WriteString("\n" + theme.Subtle.Render("j/k · navigate   enter · preview   esc · back"))
	return sb.String()
}

// buildRevisionPreviewContent renders the full content of the previewed revision.
func (m JournalModel) buildRevisionPreviewContent() string {
	if m.revPreview == nil {
		return ""
	}
	n := m.revPreview
	ts := n.CreatedAt.Format("02-Jan-2006 15:04")
	header := theme.Title.Render(fmt.Sprintf("Rev %d", n.RevisionNumber)) +
		"  " + theme.Subtle.Render(ts)
	if len(n.Topics) > 0 {
		topics := ""
		for _, t := range n.Topics {
			topics += theme.Subtle.Render("#"+t) + " "
		}
		header += "\n" + topics
	}
	innerWidth := m.width - 4
	if innerWidth < 1 {
		innerWidth = 80
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		"",
		markdown.Render(n.Content, innerWidth),
		"",
		theme.Subtle.Render("j/k · scroll   esc · back"),
	)
}

// openNote puts a selected note into edit mode.
func (m JournalModel) openNote(note model.Note) (JournalModel, tea.Cmd) {
	m.editMode = true
	m.isNewNote = false
	m.editingID = note.ID
	m.topicsFocused = false
	m.topicsInput.SetValue(strings.Join(note.Topics, ", "))
	m.topicsInput.Blur()
	var cmd tea.Cmd
	m.compose, cmd = m.compose.OpenWithContent("editing note", "write your note…", note.Content)
	m.compose = m.compose.GotoStart() // scroll textarea viewport to line 1
	m.viewport.Height = m.viewportHeight()
	return m, cmd
}

// openNewNote opens a blank compose for creating a new note.
func (m JournalModel) openNewNote() (JournalModel, tea.Cmd) {
	m.editMode = true
	m.isNewNote = true
	m.editingID = ""
	m.topicsFocused = false
	m.topicsInput.SetValue("")
	m.topicsInput.Blur()
	var cmd tea.Cmd
	m.compose, cmd = m.compose.Open("new note", "write your note…")
	m.viewport.Height = m.viewportHeight()
	return m, cmd
}

// closeEdit exits edit mode and restores the full viewport height.
func (m JournalModel) closeEdit() JournalModel {
	m.editMode = false
	m.isNewNote = false
	m.editingID = ""
	m.topicsFocused = false
	m.topicsInput.Blur()
	m.compose = m.compose.Close()
	m.viewport.Height = m.viewportHeight()
	m = m.refreshContent()
	return m
}

// confirmBoxHeight is the number of rows consumed by the confirmation prompt box.
const confirmBoxHeight = 3 // border-top + content + border-bottom

// viewportHeight returns the number of rows available for the note list.
// It shrinks to make room for the compose box, topics input, and confirmation prompt.
func (m JournalModel) viewportHeight() int {
	h := m.height - theme.ChromeHeight
	if m.editMode {
		h -= m.compose.BoxHeight()
		h -= 3 // topics input: border top + content + border bottom
	}
	if m.confirming != confirmNone {
		h -= confirmBoxHeight
	}
	if h < 1 {
		h = 1
	}
	return h
}

// refreshContent rebuilds the viewport note list and tracks per-note start lines.
func (m JournalModel) refreshContent() JournalModel {
	content, offsets := m.buildContent()
	m.noteOffsets = offsets
	m.viewport.SetContent(content)
	return m
}

// ensureSelectedVisible scrolls the minimum amount so the selected note card is
// fully visible. Uses the tracked noteOffsets for accurate positioning.
func (m JournalModel) ensureSelectedVisible() JournalModel {
	if !m.ready || len(m.noteOffsets) == 0 || m.selectedIdx >= len(m.notes) {
		return m
	}
	noteStart := m.noteOffsets[m.selectedIdx]
	noteHeight := lipgloss.Height(m.renderNote(m.notes[m.selectedIdx], false))
	noteEnd := noteStart + noteHeight - 1

	viewTop := m.viewport.YOffset
	viewBottom := viewTop + m.viewport.Height - 1

	if noteStart < viewTop {
		m.viewport.SetYOffset(noteStart)
	} else if noteEnd > viewBottom {
		if noteHeight <= m.viewport.Height {
			m.viewport.SetYOffset(noteEnd - m.viewport.Height + 1)
		} else {
			m.viewport.SetYOffset(noteStart)
		}
	}
	return m
}

// buildContent renders all notes into a single string and returns the start
// line of each note so ensureSelectedVisible can scroll accurately.
func (m JournalModel) buildContent() (string, []int) {
	if len(m.notes) == 0 {
		if m.loading {
			return theme.Subtle.Render("  loading notes…"), nil
		}
		return theme.Subtle.Render("  no notes yet"), nil
	}

	sepLines := 1 // dense: single blank line between cards
	if m.relaxed {
		sepLines = 2
	}

	offsets := make([]int, len(m.notes))
	var sb strings.Builder
	currentLine := 0
	for i, note := range m.notes {
		offsets[i] = currentLine
		rendered := m.renderNote(note, i == m.selectedIdx)
		sb.WriteString(rendered)
		sb.WriteString("\n")
		currentLine += lipgloss.Height(rendered) + (sepLines - 1)
		if m.relaxed {
			sb.WriteString("\n")
		}
	}

	if m.loading {
		sb.WriteString(theme.Subtle.Render("  loading more…"))
		sb.WriteString("\n")
	} else if m.exhausted {
		sb.WriteString(theme.Subtle.Render("  — end of journal —"))
		sb.WriteString("\n")
	}

	return sb.String(), offsets
}

// renderNote renders a single note as a selectable list row.
// Shows the first non-empty line of content, topics, and creation date.
func (m JournalModel) renderNote(note model.Note, selected bool) string {
	innerWidth := m.width - 4
	if innerWidth < 1 {
		innerWidth = 40 // safe fallback before first WindowSizeMsg
	}

	// First non-empty line as preview (markdown syntax stripped).
	preview := markdown.FirstLine(note.Content)
	if preview == "" {
		preview = "(empty)"
	}

	timestamp := displayTime(note.CreatedAt, m.location(), m.timeDisplayFormat, true)

	left := theme.Highlight.Render(preview)
	right := theme.Subtle.Render(timestamp)

	gap := innerWidth - lipgloss.Width(left) - lipgloss.Width(right)
	var header string
	if gap > 0 {
		header = left + strings.Repeat(" ", gap) + right
	} else {
		header = left
	}

	// Topics line.
	var topicsStr string
	for _, t := range note.Topics {
		topicsStr += theme.Subtle.Render("#"+t) + " "
	}

	var content string
	if topicsStr != "" {
		content = lipgloss.JoinVertical(lipgloss.Left, header, topicsStr)
	} else {
		content = header
	}

	boxStyle := theme.Border
	if selected {
		boxStyle = theme.ActiveBorder
	}
	if m.width > 2 {
		boxStyle = boxStyle.Width(m.width - 2)
	}
	return boxStyle.Render(content)
}


func (m JournalModel) View() string {
	if m.err != nil {
		return theme.Error.Render(fmt.Sprintf("journal error: %s", m.err))
	}
	if !m.ready {
		return theme.Subtle.Render("loading journal…")
	}

	// Revision history view takes over the entire screen.
	if m.revisionsMode {
		return m.viewport.View()
	}

	// Confirmation overlay replaces bottom area.
	if m.confirming != confirmNone {
		prompt := m.confirmPrompt()
		promptView := theme.ActiveBorder.Width(m.width - 2).Render(prompt)
		if m.editMode {
			return lipgloss.JoinVertical(lipgloss.Left,
				m.viewport.View(),
				m.compose.View(),
				m.renderTopicsBox(),
				promptView,
			)
		}
		return lipgloss.JoinVertical(lipgloss.Left,
			m.viewport.View(),
			promptView,
		)
	}

	if m.editMode {
		return lipgloss.JoinVertical(lipgloss.Left,
			m.viewport.View(),
			m.compose.View(),
			m.renderTopicsBox(),
		)
	}

	return m.viewport.View()
}

func (m JournalModel) renderTopicsBox() string {
	style := theme.Border
	if m.topicsFocused {
		style = theme.ActiveBorder
	}
	if m.width > 2 {
		style = style.Width(m.width - 2)
	}
	return style.Render(m.topicsInput.View())
}

func (m JournalModel) confirmPrompt() string {
	switch m.confirming {
	case confirmPublish:
		return theme.Highlight.Render("Publish note as post?") + "  " +
			theme.Base.Render("[y]es") + "  " +
			theme.Subtle.Render("[n]o / esc")
	case confirmDelete:
		return theme.Error.Render("Delete this note?") + "  " +
			theme.Base.Render("[y]es") + "  " +
			theme.Subtle.Render("[n]o / esc")
	}
	return ""
}
