package plugin

import (
	"context"
	"testing"
)

// "*.<what>" hears any plugin's <key>.<what>, never a core event.
func TestWildcardHooks(t *testing.T) {
	list := []string{"mention.created", "*.approval.requested"}
	for event, want := range map[string]bool{
		"mention.created": true, "runners.approval.requested": true, "chat.approval.requested": true,
		"todo.status": false, "runners.approval.withdrawn": false, "approval.requested": false,
	} {
		if hooks(list, event) != want {
			t.Errorf("hooks(%q) = %v", event, !want)
		}
	}
	m := Manifest{Key: "x", Name: "X", Version: "1", Hooks: []string{"*.approval.requested", "*"}}
	if err := m.validate(); err == nil {
		t.Error("a bare * hook passed")
	}
}

// A module offering mentionables needs the grant and an OnEvent.
func TestMentionablesNeedTheGrant(t *testing.T) {
	m := &Module{Manifest: Manifest{Key: "bots", Name: "Bots", Version: "1"}, Mentionables: func(_ context.Context, _ *API) ([]Mentionable, error) { return nil, nil }}
	if err := m.validate(); err == nil {
		t.Error("mentionables without the grant passed")
	}
	if !(Mentionable{Roles: []string{"owner"}}).MentionedBy("owner") || (Mentionable{Roles: []string{"owner"}}).MentionedBy("member") || !(Mentionable{}).MentionedBy("member") {
		t.Error("MentionedBy")
	}
}
