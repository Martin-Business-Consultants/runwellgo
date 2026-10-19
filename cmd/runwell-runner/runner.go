package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"
)

// errCancelled and errStopped say why a job's context ended: a person
// cancelled it in Runwell, or this runner is shutting down.
var (
	errCancelled = errors.New("cancelled in Runwell")
	errStopped   = errors.New("the runner was stopped")
)

// runner pulls jobs and does them, as many at once as its config allows.
type runner struct {
	cfg    *Config
	client *client
	log    *slog.Logger
	red    *redactor
	noPush bool
	noPR   bool
	once   bool

	mu      sync.Mutex
	running map[int64]context.CancelCauseFunc
	cleaned map[int64]bool
	paused  bool
	poll    time.Duration
	wg      sync.WaitGroup
}

func runCmd(ctx context.Context, stop func(), args []string, errOut io.Writer) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	path := fs.String("config", defaultConfigPath(), "")
	noPush := fs.Bool("no-push", false, "")
	noPR := fs.Bool("no-pr", false, "")
	once := fs.Bool("once", false, "")
	format := fs.String("log-format", os.Getenv("LOG_FORMAT"), "")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	log, err := newLogger(*format, errOut)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(*path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.WorkDir, 0o700); err != nil {
		return err
	}
	r := &runner{cfg: cfg, client: newClient(cfg), log: log, red: newRedactor(cfg.Token),
		noPush: *noPush || cfg.NoPush, noPR: *noPR || cfg.NoPR, once: *once,
		running: map[int64]context.CancelCauseFunc{}, cleaned: map[int64]bool{}, poll: 5 * time.Second}
	// After the first signal, a second one ends it at once.
	go func() {
		<-ctx.Done()
		stop()
	}()
	return r.loop(ctx)
}

func (r *runner) busy() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.running)
}

// beat tells Runwell it's here, and does what the answer asks: stop
// cancelled jobs, clean up declined ones.
func (r *runner) beat(ctx context.Context) error {
	b, err := r.client.heartbeat(ctx, info(ctx, r.cfg, r.busy()))
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.paused = b.Paused
	if b.PollSeconds > 0 {
		r.poll = time.Duration(b.PollSeconds) * time.Second
	}
	for _, id := range b.Cancel {
		if cancel := r.running[id]; cancel != nil {
			cancel(errCancelled)
		}
	}
	var todo []Cleanup
	for _, c := range b.Cleanup {
		if !r.cleaned[c.JobID] {
			r.cleaned[c.JobID] = true
			todo = append(todo, c)
		}
	}
	r.mu.Unlock()
	for _, c := range todo {
		r.cleanup(ctx, c)
	}
	return nil
}

func (r *runner) loop(ctx context.Context) error {
	r.log.Info("runner starting", "url", r.cfg.URL, "repos", r.cfg.AllowedRepos, "concurrency", r.cfg.Concurrency, "no_push", r.noPush, "no_pr", r.noPR)
	if err := r.beat(ctx); err != nil {
		var ae *apiError
		if errors.As(err, &ae) && ae.Status == 401 {
			return errors.New("Runwell doesn't take this runner's token: it was removed, or the Runners plugin is off") //nolint:staticcheck // ST1005: Runwell is a name
		}
		r.log.Warn("heartbeat", "err", err)
	}
	go func() {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := r.beat(ctx); err != nil && ctx.Err() == nil {
					r.log.Warn("heartbeat", "err", err)
				}
			}
		}
	}()
	did := 0
	for ctx.Err() == nil {
		r.mu.Lock()
		free, paused, poll := len(r.running) < r.cfg.Concurrency, r.paused, r.poll
		r.mu.Unlock()
		if r.once && did > 0 {
			if r.busy() == 0 {
				break
			}
			free = false
		}
		if free && !paused {
			job, err := r.client.claim(ctx, r.cfg.AllowedRepos)
			switch {
			case err != nil && ctx.Err() == nil:
				r.log.Warn("claim", "err", err)
			case job != nil:
				did++
				r.start(ctx, job)
				continue // there may be more
			}
		}
		select {
		case <-ctx.Done():
		case <-time.After(poll):
		}
	}
	// Stopping: what's running is told to stop, reports, and cleans up.
	r.mu.Lock()
	for _, cancel := range r.running {
		cancel(errStopped)
	}
	n := len(r.running)
	r.mu.Unlock()
	if n > 0 {
		r.log.Info("stopping: ending the jobs in hand", "jobs", n)
	}
	r.wg.Wait()
	r.log.Info("runner stopped")
	return nil
}

func (r *runner) start(ctx context.Context, job *Job) {
	// The job's context outlives the loop's by nothing: a signal stops it,
	// with errStopped as the cause.
	jctx, cancel := context.WithCancelCause(context.WithoutCancel(ctx))
	r.mu.Lock()
	r.running[job.ID] = cancel
	r.mu.Unlock()
	r.wg.Go(func() {
		defer func() {
			r.mu.Lock()
			delete(r.running, job.ID)
			r.mu.Unlock()
			cancel(nil)
		}()
		r.do(jctx, cancel, job)
	})
}
