package main

import (
	"os"
	"regexp"
	"slices"
	"strings"
)

// redactor takes secrets out of what the runner sends back: its own token,
// the values of this process's secret-looking environment variables, and
// anything shaped like a well-known token. A line of an agent's output
// could carry any of them.
type redactor struct {
	values []string
}

var tokenShapes = regexp.MustCompile(`(rwm_|rwt_|ghp_|gho_|ghu_|ghs_|ghr_|github_pat_|sk-ant-|sk-proj-|sk-|xox[abprs]-|AKIA)[A-Za-z0-9_\-]{8,}`)

var secretName = regexp.MustCompile(`(?i)(TOKEN|SECRET|PASSWORD|PASSWD|API_?KEY|ACCESS_?KEY|PRIVATE_?KEY|CREDENTIALS?)`)

func newRedactor(extra ...string) *redactor {
	r := &redactor{}
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		if secretName.MatchString(k) && len(v) >= 8 {
			r.values = append(r.values, v)
		}
	}
	for _, v := range extra {
		if len(v) >= 8 {
			r.values = append(r.values, v)
		}
	}
	// Longest first, so a secret that holds another goes whole.
	slices.SortFunc(r.values, func(a, b string) int { return len(b) - len(a) })
	return r
}

func (r *redactor) clean(s string) string {
	for _, v := range r.values {
		s = strings.ReplaceAll(s, v, "[redacted]")
	}
	return tokenShapes.ReplaceAllString(s, "[redacted]")
}
