package store

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestTimeScan(t *testing.T) {
	t.Parallel()
	want := time.Date(2026, 3, 4, 5, 6, 7, 123456000, time.UTC)
	tests := []struct {
		name    string
		in      any
		want    Time
		wantErr bool
	}{
		{"null", nil, Time{}, false},
		{"Rails text", "2026-03-04 05:06:07.123456", Time{want, true}, false},
		{"Rails text as bytes", []byte("2026-03-04 05:06:07.123456"), Time{want, true}, false},
		{"RFC 3339 elsewhere", "2026-03-04T06:06:07.123456+01:00", Time{want, true}, false},
		{"without a zone", "2026-03-04T05:06:07.123456", Time{want, true}, false},
		{"a date", "2026-03-04", Time{time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC), true}, false},
		{"blank", "  ", Time{}, false},
		{"a time.Time", want.In(time.FixedZone("x", 3600)), Time{want, true}, false},
		{"zero time.Time", time.Time{}, Time{}, false},
		{"garbage", "yesterday", Time{}, true},
		{"a number", int64(5), Time{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got Time
			err := got.Scan(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Scan(%#v) err %v", tt.in, err)
			}
			if !tt.wantErr && (got.Valid != tt.want.Valid || !got.Equal(tt.want.Time) || got.Location() != time.UTC) {
				t.Errorf("Scan(%#v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestTimeWrites(t *testing.T) {
	t.Parallel()
	at := TimeOf(time.Date(2026, 3, 4, 5, 6, 7, 123456789, time.FixedZone("x", -3600)))
	if got := at.DB(); got != "2026-03-04 06:06:07.123456" {
		t.Errorf("DB() = %v", got)
	}
	if got := at.ISO(); got != "2026-03-04T06:06:07Z" {
		t.Errorf("ISO() = %v", got)
	}
	b, err := json.Marshal(at)
	if err != nil || string(b) != `"2026-03-04T06:06:07.123Z"` {
		t.Errorf("MarshalJSON = %s, %v", b, err)
	}
	var none Time
	if none.DB() != nil || none.ISO() != nil {
		t.Error("a null time writes as NULL")
	}
	if b, _ := json.Marshal(none); string(b) != "null" {
		t.Errorf("a null time marshals as %s", b)
	}
	// What the DB form writes, Scan reads back (to the microsecond).
	var back Time
	if err := back.Scan(at.DB()); err != nil || !back.Equal(at.Truncate(time.Microsecond)) {
		t.Errorf("round trip: %v %v", back, err)
	}
}

func TestTimeUnmarshalJSON(t *testing.T) {
	t.Parallel()
	var v struct{ At Time }
	if err := json.Unmarshal([]byte(`{"At":"2026-03-04T05:06:07Z"}`), &v); err != nil || !v.At.Valid || v.At.Hour() != 5 {
		t.Errorf("got %v, %v", v.At, err)
	}
	if err := json.Unmarshal([]byte(`{"At":null}`), &v); err != nil || v.At.Valid {
		t.Errorf("null: %v, %v", v.At, err)
	}
	if err := json.Unmarshal([]byte(`{"At":5}`), &v); err == nil {
		t.Error("a number is a time")
	}
	if _, err := ParseTime("nope"); err == nil {
		t.Error("ParseTime took nope")
	}
}

func TestJSON(t *testing.T) {
	t.Parallel()
	var j JSON
	src := []byte(`{"a":1}`)
	if err := j.Scan(src); err != nil {
		t.Fatal(err)
	}
	src[2] = 'X' // the driver reuses its buffer; the scanned value keeps its own
	if string(j) != `{"a":1}` {
		t.Errorf("Scan aliased the driver's bytes: %s", j)
	}
	if err := j.Scan(42); err == nil {
		t.Error("scanned a number")
	}
	if err := j.Scan(nil); err != nil || j != nil {
		t.Errorf("Scan(nil) = %s, %v", j, err)
	}

	tests := []struct {
		name string
		j    JSON
		m    map[string]any
		l    []any
		or   string
		js   string
	}{
		{"object", JSON(`{"a":1}`), map[string]any{"a": 1.0}, []any{}, `{"a":1}`, `{"a":1}`},
		{"list", JSON(`[1,"x"]`), map[string]any{}, []any{1.0, "x"}, `[1,"x"]`, `[1,"x"]`},
		{"null", JSON(`null`), map[string]any{}, []any{}, `{}`, `null`},
		{"empty", nil, map[string]any{}, []any{}, `{}`, `null`},
		{"broken", JSON(`{`), map[string]any{}, []any{}, `{`, `{`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.j.Map(); !reflect.DeepEqual(got, tt.m) {
				t.Errorf("Map() = %v", got)
			}
			if got := tt.j.List(); !reflect.DeepEqual(got, tt.l) {
				t.Errorf("List() = %v", got)
			}
			if got := tt.j.Or("{}"); string(got) != tt.or {
				t.Errorf("Or() = %s", got)
			}
			if tt.name == "broken" {
				return // not something MarshalJSON is handed
			}
			b, err := json.Marshal(struct{ V JSON }{tt.j})
			if err != nil || string(b) != `{"V":`+tt.js+`}` {
				t.Errorf("MarshalJSON = %s, %v", b, err)
			}
		})
	}

	var v struct{ V JSON }
	if err := json.Unmarshal([]byte(`{"V": {"b": [1, 2]}}`), &v); err != nil || string(v.V) != `{"b": [1, 2]}` {
		t.Errorf("UnmarshalJSON kept %s, %v", v.V, err)
	}
	if got := MustJSON(map[string]int{"n": 1}); string(got) != `{"n":1}` {
		t.Errorf("MustJSON = %s", got)
	}
}

func TestNullInt(t *testing.T) {
	t.Parallel()
	var none NullInt
	if b, _ := json.Marshal(none); string(b) != "null" || none.DB() != nil {
		t.Errorf("null: %s %v", b, none.DB())
	}
	n := IntOf(7)
	if b, _ := json.Marshal(n); string(b) != "7" || n.DB() != int64(7) {
		t.Errorf("7: %s %v", b, n.DB())
	}
	if err := none.Scan(int64(3)); err != nil || none != IntOf(3) {
		t.Errorf("Scan(3) = %v, %v", none, err)
	}
}

func TestPlaceholders(t *testing.T) {
	t.Parallel()
	for n, want := range map[int]string{0: "", 1: "?", 3: "?,?,?"} {
		if got := Placeholders(n); got != want {
			t.Errorf("Placeholders(%d) = %q, want %q", n, got, want)
		}
	}
}
