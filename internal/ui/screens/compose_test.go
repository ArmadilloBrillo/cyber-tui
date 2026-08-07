package screens

import "testing"

func TestPostComposePanel_SlugValue_LowercasesOnRead(t *testing.T) {
	m := NewPostComposePanel(80)
	m.slugInput.SetValue("MyPost-42")
	if got := m.SlugValue(); got != "mypost-42" {
		t.Errorf("SlugValue() = %q, want %q (typed case preserved in the field, lowercased on read)", got, "mypost-42")
	}
	if got := m.slugInput.Value(); got != "MyPost-42" {
		t.Errorf("slugInput.Value() = %q, want unchanged %q", got, "MyPost-42")
	}
}
