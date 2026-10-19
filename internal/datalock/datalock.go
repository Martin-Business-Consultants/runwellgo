// Package datalock keeps an import off a data directory a server is using:
// `serve` holds DATA_DIR/.lock shared while it runs (two servers may
// overlap, as a zero-downtime deploy starts the new one before stopping the
// old; SQLite keeps them straight), and `import`, which replaces the
// database and files under it, takes it exclusively, so it refuses while
// any server holds it.
// The lock is the operating system's (flock, LockFileEx), so it's released
// when the process ends however it ends: there's no stale lock to clear.
package datalock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrLocked is a data directory another process holds.
var ErrLocked = errors.New("the data directory is in use")

// Lock is a held lock; Release lets it go.
type Lock struct {
	f *os.File
}

// Acquire takes dir's lock for itself without waiting (an import):
// ErrLocked (wrapped, saying which process holds it when that's known) when
// another holds it at all.
func Acquire(dir string) (*Lock, error) { return acquire(dir, true) }

// AcquireShared takes dir's lock alongside other servers without waiting
// (serve): ErrLocked only when an import holds it.
func AcquireShared(dir string) (*Lock, error) { return acquire(dir, false) }

func acquire(dir string, exclusive bool) (*Lock, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, ".lock")
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600) //nolint:gosec // the operator's own --data path
	if err != nil {
		return nil, err
	}
	if err := lock(f, exclusive); err != nil {
		f.Close()
		if errors.Is(err, errWouldBlock) {
			return nil, fmt.Errorf("%w%s", ErrLocked, holder(path))
		}
		return nil, fmt.Errorf("locking %s: %w", path, err)
	}
	// Who holds it, for the message another process shows; the lock itself
	// is the OS's, not this number.
	if err := f.Truncate(0); err == nil {
		_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	}
	return &Lock{f: f}, nil
}

// Release lets the lock go. Safe to call twice.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := errors.Join(unlock(l.f), l.f.Close())
	l.f = nil
	return err
}

// holder is " (process N)" from the lock file, when it says.
func holder(path string) string {
	raw, err := os.ReadFile(path) //nolint:gosec // the lock file Acquire just opened
	if err != nil {
		return ""
	}
	if pid, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil && pid > 0 {
		return fmt.Sprintf(" (process %d)", pid)
	}
	return ""
}
