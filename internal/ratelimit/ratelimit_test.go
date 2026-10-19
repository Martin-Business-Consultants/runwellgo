package ratelimit

import (
	"fmt"
	"testing"
	"testing/synctest"
	"time"
)

func TestLimiterStaysBounded(t *testing.T) {
	t.Parallel()
	l := New(1)
	for i := range maxRateKeys + 10 {
		l.Allow(fmt.Sprint("k", i))
	}
	if n := len(l.window); n > maxRateKeys {
		t.Fatalf("%d keys remembered, cap %d", n, maxRateKeys)
	}
	if ok, _ := l.Allow("again"); !ok {
		t.Fatal("a new caller is let through")
	}
	if ok, _ := l.Allow("again"); ok {
		t.Fatal("its second request in the minute isn't")
	}
}

func TestLimiterWindows(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		l := New(2)
		allow := func(key string, want bool, wantWait int) {
			t.Helper()
			ok, wait := l.Allow(key)
			if ok != want || wait != wantWait {
				t.Fatalf("Allow(%q) = %v, %d; want %v, %d", key, ok, wait, want, wantWait)
			}
		}
		allow("a", true, 0)
		allow("a", true, 0)
		allow("a", false, 61)
		allow("b", true, 0) // keys are counted apart
		time.Sleep(30 * time.Second)
		allow("a", false, 31)
		time.Sleep(30 * time.Second)
		allow("a", true, 0) // a new minute
	})
}

func TestNoLimit(t *testing.T) {
	t.Parallel()
	for _, limit := range []int{0, -1} {
		l := New(limit)
		for range 1000 {
			if ok, _ := l.Allow("k"); !ok {
				t.Fatalf("New(%d) refused a request", limit)
			}
		}
		if d := l.Describe(); d != "none" {
			t.Errorf("New(%d).Describe() = %q", limit, d)
		}
	}
	if d := New(30).Describe(); d != "30 a minute" {
		t.Errorf("Describe() = %q", d)
	}
}
