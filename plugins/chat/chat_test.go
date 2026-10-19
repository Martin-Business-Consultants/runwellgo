package chat

import (
	"testing"
	"time"
)

func TestCleanName(t *testing.T) {
	for in, want := range map[string]string{"General Chat": "general-chat", "#Design  Team!": "design-team", " --x-- ": "x", "": ""} {
		if got := cleanName(in); got != want {
			t.Errorf("cleanName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDMKeyIsTheSameWhoeverStartsIt(t *testing.T) {
	if dmKey("group", []int64{9, 2, 5}) != dmKey("group", []int64{5, 9, 2}) {
		t.Fatal("dm keys differ by order")
	}
}

func TestAvailableHours(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	h := hours{On: true, Days: []int{1, 2, 3, 4, 5}, Start: "09:00", End: "17:00", Zone: "America/New_York"}
	monday10 := time.Date(2026, 10, 19, 10, 0, 0, 0, loc)
	monday18 := time.Date(2026, 10, 19, 18, 0, 0, 0, loc)
	sunday10 := time.Date(2026, 10, 18, 10, 0, 0, 0, loc)
	if !h.available(monday10, time.UTC) || h.available(monday18, time.UTC) || h.available(sunday10, time.UTC) {
		t.Fatal("hours read wrong")
	}
	night := hours{On: true, Days: []int{0, 1, 2, 3, 4, 5, 6}, Start: "22:00", End: "06:00", Zone: "UTC"}
	if !night.available(time.Date(2026, 10, 19, 23, 0, 0, 0, time.UTC), nil) || night.available(time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC), nil) {
		t.Fatal("overnight hours read wrong")
	}
	if !(hours{}).available(monday18, nil) {
		t.Fatal("no hours set is always available")
	}
}

func TestPresence(t *testing.T) {
	here := profile{Active: true, SeenAt: stamp()}
	if presenceOf(here, true, true) != "online" || presenceOf(here, true, false) != "away" || presenceOf(here, false, true) != "offline" {
		t.Fatal("presence read wrong")
	}
	if presenceOf(profile{Active: false, SeenAt: stamp()}, true, true) != "away" {
		t.Fatal("an idle tab is away")
	}
}
