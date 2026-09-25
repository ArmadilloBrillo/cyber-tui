package screens

import (
	"errors"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbletea"
)

// DefaultHardBreakKey is the compose key that inserts a markdown hard break.
const DefaultHardBreakKey = "alt+enter"

// hardBreakText is what a hard break inserts. Two trailing spaces plus a
// newline is the only single-line-break form the website's renderer keeps.
const hardBreakText = "  \n"

// hardBreakReserved holds keys the compose screens, the textarea or the app
// already act on. Binding one of them would shadow it.
var hardBreakReserved = map[string]bool{
	"esc": true, "ctrl+c": true, "ctrl+s": true, "ctrl+d": true, "ctrl+p": true,
	"ctrl+o": true, "ctrl+q": true, "ctrl+t": true, "ctrl+]": true, "ctrl+g": true,
	"ctrl+j": true, "ctrl+left": true, "ctrl+right": true, "shift+tab": true,
	"ctrl+a": true, "ctrl+e": true, "ctrl+k": true, "ctrl+u": true, "ctrl+w": true,
	"ctrl+v": true, "ctrl+f": true, "ctrl+b": true, "ctrl+n": true, "ctrl+h": true,
	"alt+d": true, "alt+f": true, "alt+b": true, "alt+c": true, "alt+l": true,
	"alt+u": true, "alt+<": true, "alt+>": true, "alt+backspace": true, "alt+delete": true,
}

// ValidateHardBreakKey checks a hard-break key. It must carry a modifier
// (ctrl, alt or shift) so plain typing is never swallowed, and must not be a
// key the compose screens, the textarea or the app already act on.
func ValidateHardBreakKey(k string) error {
	switch {
	case k == "" || strings.Contains(k, " "):
		return errors.New("that key cannot be bound")
	case hardBreakReserved[k]:
		return errors.New("already used by compose or the app")
	case !strings.HasPrefix(k, "ctrl+") && !strings.HasPrefix(k, "alt+") && !strings.HasPrefix(k, "shift+"):
		return errors.New("needs ctrl, alt or shift so typing is not affected")
	}
	return nil
}

// HardBreakKeyFromMsg turns a captured keypress into a key string, rejecting
// pastes and multi-rune input that are not a single keypress.
func HardBreakKeyFromMsg(msg tea.KeyMsg) (string, error) {
	if msg.Paste || (msg.Type == tea.KeyRunes && len(msg.Runes) > 1) {
		return "", errors.New("press a single key")
	}
	return msg.String(), nil
}

// resolveHardBreakKey returns the stored key, falling back to the default
// when it is empty or no longer valid.
func resolveHardBreakKey(spec string) string {
	if ValidateHardBreakKey(spec) != nil {
		return DefaultHardBreakKey
	}
	return spec
}

// convertPaste rewrites a bracketed paste so the layout survives the website's
// renderer, which folds a bare newline into a space: single newlines become
// hard breaks, blank lines stay paragraph breaks, and runs of blank lines
// collapse to one. Fenced code blocks are left as pasted.
func convertPaste(msg tea.KeyMsg) tea.KeyMsg {
	if !msg.Paste || msg.Type != tea.KeyRunes {
		return msg
	}
	s := strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(string(msg.Runes))
	lines := strings.Split(s, "\n")
	var b strings.Builder
	inFence, prevBlank := false, false
	for i, line := range lines {
		fence := strings.HasPrefix(strings.TrimSpace(line), "```")
		if fence {
			inFence = !inFence
		}
		literal := inFence || fence
		blank := !literal && strings.TrimSpace(line) == ""
		if blank && prevBlank {
			continue
		}
		prevBlank = blank
		if !literal {
			line = strings.TrimRight(line, " \t")
		}
		b.WriteString(line)
		if i == len(lines)-1 {
			break
		}
		next := strings.TrimSpace(lines[i+1])
		if literal || blank || next == "" || strings.HasPrefix(next, "```") {
			b.WriteString("\n")
		} else {
			b.WriteString(hardBreakText)
		}
	}
	msg.Runes = []rune(b.String())
	return msg
}

// HardBreakLabel returns the key to display for a stored binding, treating an
// unset value as the default.
func HardBreakLabel(spec string) string {
	if spec == "" {
		return DefaultHardBreakKey
	}
	return spec
}

// insertHardBreak inserts a hard break at the cursor and scrolls it into view.
// InsertString alone does not scroll, so at maximum editor height the new line
// would stay out of view until the next keypress; an update with a message the
// textarea ignores runs its scroll-to-cursor step.
func insertHardBreak(ta textarea.Model) textarea.Model {
	ta.InsertString(hardBreakText)
	ta, _ = ta.Update(nil)
	return ta
}
