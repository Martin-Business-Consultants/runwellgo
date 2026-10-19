package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ---- time ----------------------------------------------------------------

// Rails writes SQLite datetimes as UTC text with microseconds; so does this.
const dbTimeFormat = "2006-01-02 15:04:05.999999"

// Time is a nullable timestamp that reads Rails' text format and writes JSON
// the way Rails does (ISO 8601, milliseconds, Z).
type Time struct {
	time.Time
	Valid bool
}

// Now is the current time, in UTC.
func Now() Time { return Time{Time: time.Now().UTC(), Valid: true} }

// TimeOf is t in UTC; the zero time is null.
func TimeOf(t time.Time) Time { return Time{Time: t.UTC(), Valid: !t.IsZero()} }

// Scan reads a column: NULL, a time.Time, or text in any format ParseTime
// takes.
func (t *Time) Scan(v any) error {
	switch x := v.(type) {
	case nil:
		*t = Time{}
	case time.Time:
		*t = TimeOf(x)
	case string:
		return t.parse(x)
	case []byte:
		return t.parse(string(x))
	default:
		return fmt.Errorf("time: can't scan %T", v)
	}
	return nil
}

// ParseTime reads s in any of the formats Scan takes; "" is a null time.
func ParseTime(s string) (Time, error) {
	var t Time
	err := t.parse(s)
	return t, err
}

func (t *Time) parse(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		*t = Time{}
		return nil
	}
	for _, layout := range []string{dbTimeFormat, time.RFC3339Nano, "2006-01-02T15:04:05.999999", "2006-01-02"} {
		if p, err := time.Parse(layout, s); err == nil {
			*t = TimeOf(p)
			return nil
		}
	}
	return fmt.Errorf("time: can't parse %q", s)
}

// DB is t as a column value: Rails' text format, or NULL.
func (t Time) DB() any {
	if !t.Valid {
		return nil
	}
	return t.Time.UTC().Format(dbTimeFormat)
}

// MarshalJSON writes ISO 8601 with milliseconds, in UTC; null when null.
func (t Time) MarshalJSON() ([]byte, error) {
	if !t.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(t.Time.UTC().Format("2006-01-02T15:04:05.000Z"))
}

// UnmarshalJSON reads null or a string in any format ParseTime takes.
func (t *Time) UnmarshalJSON(b []byte) error {
	var s *string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s == nil {
		*t = Time{}
		return nil
	}
	return t.parse(*s)
}

// ISO is the seconds-precision form some answers have always used.
func (t Time) ISO() any {
	if !t.Valid {
		return nil
	}
	return t.Time.UTC().Format(time.RFC3339)
}

// ---- JSON columns --------------------------------------------------------

// JSON is a json column: raw, scanned from text, never nil once read.
type JSON json.RawMessage

// Scan reads a column's text, keeping its own copy of the bytes.
func (j *JSON) Scan(v any) error {
	switch x := v.(type) {
	case nil:
		*j = nil
	case string:
		*j = JSON(x)
	case []byte:
		*j = append(JSON(nil), x...)
	default:
		return fmt.Errorf("json: can't scan %T", v)
	}
	return nil
}

// MarshalJSON writes j as it is; null when it's empty.
func (j JSON) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return j, nil
}

// UnmarshalJSON keeps b as it is.
func (j *JSON) UnmarshalJSON(b []byte) error {
	*j = append(JSON(nil), b...)
	return nil
}

// Or is j, or def when j is empty or null.
func (j JSON) Or(def string) JSON {
	if len(j) == 0 || string(j) == "null" {
		return JSON(def)
	}
	return j
}

// Map is j as an object; empty when it isn't one.
func (j JSON) Map() map[string]any {
	m := map[string]any{}
	_ = json.Unmarshal(j, &m)
	if m == nil {
		m = map[string]any{}
	}
	return m
}

// List is j as a list; empty when it isn't one.
func (j JSON) List() []any {
	var l []any
	_ = json.Unmarshal(j, &l)
	if l == nil {
		l = []any{}
	}
	return l
}

// MustJSON marshals v, which can only fail on a programming error.
func MustJSON(v any) JSON {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return JSON(b)
}

// NullInt is a nullable integer id, null in JSON when absent.
type NullInt struct {
	sql.NullInt64
}

// MarshalJSON writes the number, or null.
func (n NullInt) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(n.Int64)
}

// DB is n as a column value, or NULL.
func (n NullInt) DB() any {
	if !n.Valid {
		return nil
	}
	return n.Int64
}

// IntOf is v, not null.
func IntOf(v int64) NullInt { return NullInt{sql.NullInt64{Int64: v, Valid: true}} }
