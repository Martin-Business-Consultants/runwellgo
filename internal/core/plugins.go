package core

import (
	"context"
	"fmt"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
	"github.com/Martin-Business-Consultants/runwellgo/v3/plugin"
)

// PluginTables makes the own tables (each manifest's Schema) of every
// plugin in mods, the ones the binary was built with, in db: an imported
// data export's plugin rows then have somewhere to go
// (backup.Options.PluginTables).
func PluginTables(mods []*plugin.Module) func(ctx context.Context, db *store.DB) error {
	return func(ctx context.Context, db *store.DB) error {
		for _, m := range mods {
			if m.Manifest.Schema == "" {
				continue
			}
			if err := plugin.ApplySchema(ctx, db, m.Manifest.Key, m.Manifest.Schema); err != nil {
				return fmt.Errorf("%s: %w", m.Manifest.Key, err)
			}
		}
		return nil
	}
}

func init() {
	// Plugins' tasks: each checked every minute, run when due (a nightly
	// one in the small hours of the install's time zone).
	every("plugin tasks", time.Minute, func(a *App, ctx context.Context) error {
		a.plugins.RunTasks(ctx, time.Now(), a.zone(ctx))
		return nil
	})
}
