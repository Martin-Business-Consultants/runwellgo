package plugin

import (
	"context"
	"time"
)

// RunTasks runs every enabled plugin's tasks that are due (the Rails
// nightly run and its kin), each in the background: one plugin failing or
// running slow never holds up another. When each last ran is kept in the
// core's state (plugin_tasks: {"<key>/<name>": time}), so a restart doesn't
// run them all again. A nightly task runs once a day, in the small hours
// of the install's time zone when it's up then.
func (h *Host) RunTasks(ctx context.Context, now time.Time, loc *time.Location) {
	state, err := h.env.Setting(ctx, "plugin_tasks")
	if err != nil {
		h.log.Warn("plugin tasks", "err", err)
		return
	}
	changed := false
	for _, p := range h.List() {
		if !p.Enabled || p.module == nil {
			continue
		}
		for _, t := range p.Manifest.Tasks {
			id := p.Key + "/" + t.Name
			last, _ := time.Parse(time.RFC3339, str(state[id]))
			if !due(t, last, now, loc) {
				continue
			}
			state[id] = now.UTC().Format(time.RFC3339)
			changed = true
			fn := p.module.Tasks[t.Name]
			h.spawn(ctx, "plugin task", func(ctx context.Context) {
				_, api := h.enabled(p.Key, nil)
				if api == nil {
					return
				}
				// A task gets longer than a call: a sync can take a while.
				ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
				defer cancel()
				if err := guard(ctx, "plugin "+id, func(context.Context) error { return fn(ctx, api) }); err != nil {
					h.log.Warn("plugin task", "plugin", p.Key, "task", t.Name, "err", err)
				}
			})
		}
	}
	if changed {
		if err := h.env.ReplaceSetting(ctx, "plugin_tasks", state); err != nil {
			h.log.Warn("plugin tasks", "err", err)
		}
	}
}

// due is whether t, last run at last, runs now.
func due(t TaskSpec, last, now time.Time, loc *time.Location) bool {
	if t.Every != "nightly" {
		return now.Sub(last) >= t.Interval()
	}
	if loc == nil {
		loc = time.UTC
	}
	local := now.In(loc)
	ranToday := !last.IsZero() && last.In(loc).Format(time.DateOnly) == local.Format(time.DateOnly)
	if ranToday {
		return false
	}
	// In the small hours; or, missed (the server was down then), now.
	return local.Hour() < 5 || now.Sub(last) > 30*time.Hour
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
