package screens

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

const bioCharLimit = 127

// Field indices for the profile edit form.
const (
	fieldBio           = 0
	fieldWebsiteName   = 1
	fieldWebsiteUrl    = 2
	fieldWebsiteImgUrl = 3
	fieldLocationName  = 4
	fieldLatitude      = 5
	fieldLongitude     = 6
	numProfileFields   = 7
)

// inputIdx converts a focusedField value (excluding fieldBio=0) to the
// corresponding index in the inputs slice.
func inputIdx(field int) int {
	return field - 1
}

var profileFieldLabels = [numProfileFields]string{
	"Bio",
	"Website Name",
	"Website URL",
	"Website Img URL",
	"Location",
	"Latitude",
	"Longitude",
}

type ProfileModel struct {
	user           model.User
	compose        ComposeModel
	inputs         []textinput.Model // 7 inputs (all fields except bio)
	editMode       bool
	focusedField   int
	width          int
	err            error
	saved          bool
	readOnly       bool
	canGoBack      bool
	isFollowing    bool
	followID       string
	followFeedback string
}

// SaveProfileMsg carries all editable profile fields.
type SaveProfileMsg struct {
	Bio             string
	WebsiteName     string
	WebsiteUrl      string
	WebsiteImageUrl string
	LocationName    string
	Latitude        string // empty or numeric string; parsed in app.go
	Longitude       string
}

func newProfileInputs() []textinput.Model {
	placeholders := [numProfileFields - 1]string{
		"website name (e.g. My Blog)",
		"https://…",
		"https://… (image url)",
		"city, country",
		"e.g. 48.8566",
		"e.g. 2.3522",
	}
	inputs := make([]textinput.Model, numProfileFields-1)
	for i, ph := range placeholders {
		ti := textinput.New()
		ti.Placeholder = ph
		inputs[i] = ti
	}
	return inputs
}

func NewProfileModel() ProfileModel {
	return ProfileModel{
		compose: NewComposeModel(0).SetCharLimit(bioCharLimit),
		inputs:  newProfileInputs(),
	}
}

func (m ProfileModel) SetReadOnly(readOnly bool) ProfileModel {
	m.readOnly = readOnly
	return m
}

func (m ProfileModel) SetCanGoBack(v bool) ProfileModel {
	m.canGoBack = v
	return m
}

func (m ProfileModel) SetFollowState(following bool, followID string) ProfileModel {
	m.isFollowing = following
	m.followID = followID
	return m
}

// IncrementFollowersCount adjusts the displayed follower count by delta (±1).
func (m ProfileModel) IncrementFollowersCount(delta int) ProfileModel {
	m.user.FollowersCount += delta
	return m
}

func (m ProfileModel) SetFollowFeedback(text string) ProfileModel {
	m.followFeedback = text
	return m
}

func (m ProfileModel) SetUser(u model.User) ProfileModel {
	m.user = u
	m.followFeedback = ""
	return m
}

func (m ProfileModel) SetError(err error) ProfileModel {
	m.err = err
	return m
}

// ComposeActive reports whether the edit form is open, used by app.go to
// route key events past global shortcuts.
func (m ProfileModel) ComposeActive() bool { return m.editMode }

func (m ProfileModel) Init() tea.Cmd { return nil }

// openEditForm prepopulates all inputs from the current user and activates
// edit mode, focused on the bio field by default.
func (m ProfileModel) openEditForm() (ProfileModel, tea.Cmd) {
	m.editMode = true
	m.focusedField = fieldBio
	m.saved = false

	// Populate textinputs from user fields.
	m.inputs[inputIdx(fieldWebsiteName)].SetValue(m.user.WebsiteName)
	m.inputs[inputIdx(fieldWebsiteUrl)].SetValue(m.user.WebsiteUrl)
	m.inputs[inputIdx(fieldWebsiteImgUrl)].SetValue(m.user.WebsiteImageUrl)
	m.inputs[inputIdx(fieldLocationName)].SetValue(m.user.LocationName)
	if m.user.LocationLatitude != 0 {
		m.inputs[inputIdx(fieldLatitude)].SetValue(fmt.Sprintf("%g", m.user.LocationLatitude))
	} else {
		m.inputs[inputIdx(fieldLatitude)].SetValue("")
	}
	if m.user.LocationLongitude != 0 {
		m.inputs[inputIdx(fieldLongitude)].SetValue(fmt.Sprintf("%g", m.user.LocationLongitude))
	} else {
		m.inputs[inputIdx(fieldLongitude)].SetValue("")
	}

	// Blur all textinputs (bio is the initial focus).
	for i := range m.inputs {
		m.inputs[i].Blur()
	}

	var cmd tea.Cmd
	m.compose, cmd = m.compose.OpenWithContent("bio", "what's your story…", m.user.Bio)
	return m, cmd
}

// closeEditForm exits edit mode and closes the compose box.
func (m ProfileModel) closeEditForm() ProfileModel {
	m.editMode = false
	m.compose = m.compose.Close()
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
	return m
}

// moveFocus shifts focus by delta (+1 or -1) wrapping around all fields.
func (m ProfileModel) moveFocus(delta int) (ProfileModel, tea.Cmd) {
	// Blur current field.
	if m.focusedField == fieldBio {
		m.compose, _ = m.compose.SetFocused(false)
	} else {
		m.inputs[inputIdx(m.focusedField)].Blur()
	}

	m.focusedField = (m.focusedField + delta + numProfileFields) % numProfileFields

	// Focus new field.
	var cmd tea.Cmd
	if m.focusedField == fieldBio {
		m.compose, cmd = m.compose.SetFocused(true)
	} else {
		cmd = m.inputs[inputIdx(m.focusedField)].Focus()
	}
	return m, cmd
}

// buildSaveMsg collects all current field values into a SaveProfileMsg.
func (m ProfileModel) buildSaveMsg() SaveProfileMsg {
	return SaveProfileMsg{
		Bio:             m.compose.Content(),
		WebsiteName:     m.inputs[inputIdx(fieldWebsiteName)].Value(),
		WebsiteUrl:      m.inputs[inputIdx(fieldWebsiteUrl)].Value(),
		WebsiteImageUrl: m.inputs[inputIdx(fieldWebsiteImgUrl)].Value(),
		LocationName:    m.inputs[inputIdx(fieldLocationName)].Value(),
		Latitude:        m.inputs[inputIdx(fieldLatitude)].Value(),
		Longitude:       m.inputs[inputIdx(fieldLongitude)].Value(),
	}
}

func (m ProfileModel) Update(msg tea.Msg) (ProfileModel, tea.Cmd) {
	switch msg := msg.(type) {
	case SharedConfigMsg:
		w := msg.Width
		if w > 80 {
			w = 80
		}
		m.compose = m.compose.SetWidth(w)
		inputW := w - 20
		if inputW < 10 {
			inputW = 10
		}
		for i := range m.inputs {
			m.inputs[i].Width = inputW
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		w := msg.Width
		if w > 80 {
			w = 80
		}
		m.compose = m.compose.SetWidth(w)
		inputW := w - 20
		if inputW < 10 {
			inputW = 10
		}
		for i := range m.inputs {
			m.inputs[i].Width = inputW
		}
		return m, nil

	case tea.KeyMsg:
		if m.editMode {
			// Navigation and global actions intercepted before field routing.
			switch msg.String() {
			case "tab":
				return m.moveFocus(1)
			case "shift+tab":
				return m.moveFocus(-1)
			case "esc":
				if m.focusedField != fieldBio {
					return m.closeEditForm(), nil
				}
				// Bio field: fall through so compose emits ComposeCancelMsg.
			case "ctrl+s":
				if m.focusedField != fieldBio {
					save := m.buildSaveMsg()
					return m.closeEditForm(), func() tea.Msg { return save }
				}
				// Bio field: fall through so compose emits ComposeSubmitMsg.
			}

			// Route to the active field.
			var cmd tea.Cmd
			if m.focusedField == fieldBio {
				m.compose, cmd = m.compose.Update(msg)
			} else {
				idx := inputIdx(m.focusedField)
				m.inputs[idx], cmd = m.inputs[idx].Update(msg)
			}
			return m, cmd
		}

		// Not in edit mode.
		switch msg.String() {
		case "esc":
			if m.readOnly || m.canGoBack {
				return m, func() tea.Msg { return BackFromProfileMsg{} }
			}
		case "e":
			if !m.readOnly {
				return m.openEditForm()
			}
		case "f":
			if m.readOnly {
				if m.isFollowing {
					return m, func() tea.Msg { return UnfollowUserMsg{FollowID: m.followID} }
				}
				return m, func() tea.Msg { return FollowUserMsg{UserID: m.user.ID} }
			}
		}

	// ComposeSubmitMsg arrives when Ctrl+S is pressed inside the bio compose box.
	case ComposeSubmitMsg:
		save := m.buildSaveMsg()
		save.Bio = msg.Content
		return m.closeEditForm(), func() tea.Msg { return save }

	// ComposeCancelMsg arrives when Esc is pressed inside the bio compose box.
	case ComposeCancelMsg:
		return m.closeEditForm(), nil
	}
	return m, nil
}

// View renders the profile screen. In edit mode it shows the multi-field form;
// otherwise it renders the profile card.
func (m ProfileModel) View() string {
	if m.err != nil {
		return theme.Error.Render(fmt.Sprintf("profile error: %s", m.err))
	}

	username := theme.Title.Render("@" + m.user.Username)

	if m.editMode {
		return m.editFormView(username)
	}

	counts := theme.Subtle.Render(fmt.Sprintf(
		"%d followers · %d following · %d posts",
		m.user.FollowersCount, m.user.FollowingCount, m.user.PostsCount,
	))

	var rows []string
	rows = append(rows, username, counts, "")

	rows = append(rows, theme.Base.Render(m.user.Bio))

	if m.user.WebsiteUrl != "" || m.user.WebsiteName != "" {
		label := m.user.WebsiteName
		if label == "" {
			label = m.user.WebsiteUrl
		} else if m.user.WebsiteUrl != "" {
			label = label + "  " + theme.Subtle.Render(m.user.WebsiteUrl)
		}
		rows = append(rows, "", theme.Subtle.Render("web: ")+theme.Base.Render(label))
	}

	if m.user.WebsiteImageUrl != "" {
		rows = append(rows, theme.Subtle.Render("img: ")+theme.Base.Render(m.user.WebsiteImageUrl))
	}

	if m.user.LocationName != "" {
		loc := m.user.LocationName
		if m.user.LocationLatitude != 0 || m.user.LocationLongitude != 0 {
			loc += fmt.Sprintf("  (%g, %g)", m.user.LocationLatitude, m.user.LocationLongitude)
		}
		rows = append(rows, theme.Subtle.Render("loc: ")+theme.Base.Render(loc))
	} else if m.user.LocationLatitude != 0 || m.user.LocationLongitude != 0 {
		rows = append(rows, theme.Subtle.Render("loc: ")+
			theme.Base.Render(fmt.Sprintf("%g, %g", m.user.LocationLatitude, m.user.LocationLongitude)))
	}

	rows = append(rows, "")

	var feedback string
	if m.saved && !m.readOnly {
		feedback = theme.Highlight.Render("saved.")
	} else if m.followFeedback != "" {
		feedback = theme.Highlight.Render(m.followFeedback)
	}

	var hint string
	switch {
	case m.readOnly && m.isFollowing:
		hint = theme.Subtle.Render("esc · back   f · unfollow")
	case m.readOnly:
		hint = theme.Subtle.Render("esc · back   f · follow")
	case m.canGoBack:
		hint = theme.Subtle.Render("esc · back   e · edit profile")
	default:
		hint = theme.Subtle.Render("e · edit profile")
	}

	rows = append(rows, hint, feedback)

	return theme.Border.Width(76).Render(
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
}

func (m ProfileModel) editFormView(username string) string {
	const labelW = 16 // pad all labels to this width

	var rows []string
	rows = append(rows, username, "")

	for field := 0; field < numProfileFields; field++ {
		label := profileFieldLabels[field]
		labelStr := theme.Subtle.Render(fmt.Sprintf("%-*s", labelW, label))

		if field == fieldBio {
			focused := m.focusedField == fieldBio
			borderStyle := theme.Border
			if focused {
				borderStyle = theme.ActiveBorder
			}
			used := len(m.compose.Content())
			counterStr := fmt.Sprintf("%d/%d", used, bioCharLimit)
			var counter string
			if used >= bioCharLimit {
				counter = theme.Error.Render(counterStr)
			} else {
				counter = theme.Subtle.Render(counterStr)
			}
			bioBox := lipgloss.JoinVertical(lipgloss.Left,
				borderStyle.Render(m.compose.View()),
				counter,
			)
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, labelStr, bioBox))
		} else {
			idx := inputIdx(field)
			focused := m.focusedField == field
			borderStyle := theme.Border
			if focused {
				borderStyle = theme.ActiveBorder
			}
			inputBox := borderStyle.Render(m.inputs[idx].View())
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, labelStr, inputBox))
		}
	}

	rows = append(rows, "", theme.Subtle.Render("ctrl+s · save   esc · cancel   tab · next field"))

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

