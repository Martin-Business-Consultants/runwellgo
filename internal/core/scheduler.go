package core

import (
	"context"
	"database/sql"
	"time"
)

// sqlRows is a result set, as store.ScanAll's scanners take it.
type sqlRows = sql.Rows

// The scheduler runs what the Rails core ran on a timer (Solid Queue's
// recurring jobs): each area adds its tasks with every, in an init.

type recurring struct {
	name     string
	interval time.Duration
	fn       func(a *App, ctx context.Context) error
}

var recurringTasks []recurring

// every runs fn every interval, the first time a minute after starting.
func every(name string, interval time.Duration, fn func(a *App, ctx context.Context) error) {
	recurringTasks = append(recurringTasks, recurring{name, interval, fn})
}

func (a *App) scheduler(ctx context.Context) {
	last := make([]time.Time, len(recurringTasks))
	start := time.Now()
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			if now.Sub(start) < time.Minute {
				continue
			}
			for i, t := range recurringTasks {
				if !last[i].IsZero() && now.Sub(last[i]) < t.interval {
					continue
				}
				last[i] = now
				if err := t.fn(a, ctx); err != nil {
					a.log.Warn("scheduled task", "task", t.name, "err", err)
				}
			}
		}
	}
}
