// Package agreements is the switch for Agreements: the versioned scope an
// engagement is agreed on, sent to the client and approved from a link or
// the portal. The agreements themselves are the core's (internal/core
// agreements.go, scope_items.go, approvals.go): this plugin carries no code
// of its own, only whether they're on. It ships on; an owner who doesn't
// send agreements switches it off in Settings › Plugins, and engagements
// are then simply underway. Switching it off (or removing it) leaves every
// agreement in the database as it was; switching it on brings them back.
package agreements

import "github.com/Martin-Business-Consultants/runwellgo/internal/plugin"

// Key is the plugin's key.
const Key = "agreements"

// Module is the plugin, for the core to register (internal/core/plugins.go).
func Module() *plugin.Module {
	return &plugin.Module{
		Manifest: plugin.Manifest{
			Key:         Key,
			Name:        "Agreements",
			Version:     "1.0.0",
			Author:      "Runwell",
			Description: "Agreements on engagements: scope items, versions sent to the client, approval links and decisions in the portal. Off, engagements are underway as soon as they start; agreements already made stay, and come back when it's on again.",
		},
		Default: true,
	}
}
