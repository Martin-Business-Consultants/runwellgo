package datalock

import (
	"errors"
	"testing"
)

func TestAcquire(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	first, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	// A second holder, even in this process (a lock is per open file), is
	// refused at once, and told who has it.
	if _, err := Acquire(dir); !errors.Is(err, ErrLocked) {
		t.Fatalf("second acquire: %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("second release: %v", err)
	}
	again, err := Acquire(dir)
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	again.Release()
}

// Servers share the directory (a deploy overlaps the old and the new); an
// import needs it to itself, and a server can't start under an import.
func TestSharedAndExclusive(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	old, err := AcquireShared(dir)
	if err != nil {
		t.Fatal(err)
	}
	next, err := AcquireShared(dir)
	if err != nil {
		t.Fatalf("a second server: %v", err)
	}
	if _, err := Acquire(dir); !errors.Is(err, ErrLocked) {
		t.Fatalf("an import under servers: %v", err)
	}
	if err := errors.Join(old.Release(), next.Release()); err != nil {
		t.Fatal(err)
	}
	imp, err := Acquire(dir)
	if err != nil {
		t.Fatalf("an import alone: %v", err)
	}
	if _, err := AcquireShared(dir); !errors.Is(err, ErrLocked) {
		t.Fatalf("a server under an import: %v", err)
	}
	if err := imp.Release(); err != nil {
		t.Fatal(err)
	}
}
