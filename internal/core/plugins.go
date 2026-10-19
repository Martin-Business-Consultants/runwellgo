package core

import (
	"context"
	"fmt"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
	ai "github.com/Martin-Business-Consultants/runwellgo/plugins/ai"
	"github.com/Martin-Business-Consultants/runwellgo/plugins/chat"
	"github.com/Martin-Business-Consultants/runwellgo/plugins/hello"
	myday "github.com/Martin-Business-Consultants/runwellgo/plugins/my_day"
	"github.com/Martin-Business-Consultants/runwellgo/plugins/reports"
	"github.com/Martin-Business-Consultants/runwellgo/plugins/runners"
	timetracking "github.com/Martin-Business-Consultants/runwellgo/plugins/time_tracking"
	websitechecklist "github.com/Martin-Business-Consultants/runwellgo/plugins/website_checklist"
)

// builtinPlugins are the plugins this binary has. Adding one is adding its
// package here; the default ones are installed and switched on at first
// run (plugin.Host.EnsureDefaults), the rest wait for an owner to install
// them from Settings › Plugins.
// PluginTables makes every built-in plugin's own tables (its manifest's
// Schema) in db: an imported data export's plugin rows then have somewhere
// to go (backup.Options.PluginTables).
func PluginTables(ctx context.Context, db *store.DB) error {
	for _, m := range builtinPlugins() {
		if m.Manifest.Schema == "" {
			continue
		}
		if err := plugin.ApplySchema(ctx, db, m.Manifest.Key, m.Manifest.Schema); err != nil {
			return fmt.Errorf("%s: %w", m.Manifest.Key, err)
		}
	}
	return nil
}

func builtinPlugins() []*plugin.Module {
	return []*plugin.Module{hello.Module(), timetracking.Module(), myday.Module(), websitechecklist.Module(), ai.Module(), chat.Module(), reports.Module(), runners.Module()}
}

func init() {
	// Plugins' tasks: each checked every minute, run when due (a nightly
	// one in the small hours of the install's time zone).
	every("plugin tasks", time.Minute, func(a *App, ctx context.Context) error {
		a.plugins.RunTasks(ctx, time.Now(), a.zone(ctx))
		return nil
	})
}
