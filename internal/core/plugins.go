package core

import (
	"context"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
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
