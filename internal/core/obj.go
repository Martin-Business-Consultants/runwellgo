package core

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Obj is a JSON object that keeps its keys in the order written, so answers
// read the way the Rails core's did (id first, timestamps last).
type Obj []kv

type kv struct {
	K string
	V any
}

// O builds an Obj from alternating keys and values.
func O(pairs ...any) Obj {
	o := make(Obj, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		k, ok := pairs[i].(string)
		if !ok {
			panic(fmt.Sprintf("O: key %d is a %T, not a string", i, pairs[i]))
		}
		o = append(o, kv{k, pairs[i+1]})
	}
	return o
}

// Set replaces k's value, or adds k at the end.
func (o Obj) Set(k string, v any) Obj {
	for i := range o {
		if o[i].K == k {
			o[i].V = v
			return o
		}
	}
	return append(o, kv{k, v})
}

// Get is k's value; nil when o has no k.
func (o Obj) Get(k string) any {
	for _, p := range o {
		if p.K == k {
			return p.V
		}
	}
	return nil
}

// MarshalJSON writes o's keys in order.
func (o Obj) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, p := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, _ := marshal(p.K)
		buf.Write(k)
		buf.WriteByte(':')
		v, err := marshal(p.V)
		if err != nil {
			return nil, err
		}
		buf.Write(v)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// marshal is json.Marshal without HTML escaping: answers are JSON for apps
// and agents, never inlined in a page, so "&" stays "&" as Rails wrote it.
func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
