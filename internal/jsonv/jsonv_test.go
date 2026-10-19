package jsonv

import (
	"encoding/json"
	"math"
	"testing"
)

func TestString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, ""},
		{"string", "a b", "a b"},
		{"whole number", float64(3), "3"},
		{"fraction", 2.5, "2.5"},
		{"big number", 1e21, "1000000000000000000000"},
		{"bool", true, "true"},
		{"list", []any{"a", 1.0}, "[a 1]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := String(tt.in); got != tt.want {
				t.Errorf("String(%#v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBool(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   any
		want bool
	}{
		{true, true}, {false, false},
		{"1", true}, {"true", true}, {"yes", true}, {"on", true},
		{"0", false}, {"TRUE", false}, {"", false}, {"off", false},
		{1.0, true}, {-1.0, true}, {0.0, false},
		{nil, false}, {map[string]any{}, false},
	}
	for _, tt := range tests {
		if got := Bool(tt.in); got != tt.want {
			t.Errorf("Bool(%#v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestInt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		in     any
		want   int64
		wantOK bool
	}{
		{"number", float64(42), 42, true},
		{"negative", float64(-7), -7, true},
		{"fraction", 1.5, 0, false},
		{"NaN", math.NaN(), 0, false},
		{"infinity", math.Inf(1), 0, false},
		{"past int64", 1e19, 0, false},
		{"numeric string", " 12 ", 12, true},
		{"empty string", "", 0, false},
		{"word", "twelve", 0, false},
		{"json.Number", json.Number("9007199254740993"), 9007199254740993, true},
		{"json.Number fraction", json.Number("1.5"), 0, false},
		{"null", nil, 0, false},
		{"bool", true, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := Int(tt.in)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("Int(%#v) = %d, %v; want %d, %v", tt.in, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

// FuzzInt: whatever a request body's number is, Int either refuses it or
// answers the integer it is, never a wrapped or truncated one.
func FuzzInt(f *testing.F) {
	for _, v := range []float64{0, 1, -1, 1.5, 1 << 53, 1 << 63, -(1 << 63), math.MaxFloat64} {
		f.Add(v)
	}
	f.Fuzz(func(t *testing.T, v float64) {
		n, ok := Int(v)
		if ok && float64(n) != v {
			t.Fatalf("Int(%v) = %d", v, n)
		}
	})
}
