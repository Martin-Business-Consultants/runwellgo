// Package jsonv reads values decoded from JSON into any (a request body,
// a settings key) as the type the code wants, loosely.
package jsonv

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// String is v as text: a string as it is, a number without a trailing
// .0, nil as "".
func String(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprint(x)
	}
}

// Bool is v as a checkbox would mean it: true, "1", "true", "yes", "on",
// or a non-zero number.
func Bool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "1" || x == "true" || x == "yes" || x == "on"
	case float64:
		return x != 0
	}
	return false
}

// Int reads an id from JSON: a number or a numeric string; ok false for
// null or "".
func Int(v any) (int64, bool) {
	switch x := v.(type) {
	case float64:
		// A fraction, or past what an int64 holds, isn't an id.
		if x != math.Trunc(x) || math.Abs(x) >= 1<<63 {
			return 0, false
		}
		return int64(x), true
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		return n, err == nil
	case json.Number:
		n, err := x.Int64()
		return n, err == nil
	}
	return 0, false
}
