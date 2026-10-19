// Package webhook is how events reach the addresses people registered in
// Settings › Webhooks, the way the Rails core sent them: JSON, signed with
// the endpoint's secret (X-Runwell-Signature: sha256= and the HMAC-SHA256
// of the X-Runwell-Timestamp, a dot, and the body), so the receiver can
// tell it came from this install and wasn't replayed later. The core keeps
// the endpoints and deliveries (webhook_endpoints, webhook_deliveries) and
// decides when to try; this package matches, signs and posts.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/outbound"
)

const (
	// MaxFailures in a row switch an endpoint off.
	MaxFailures = 20
	// Attempts is how many times a delivery is tried before it's given up.
	Attempts = 8
	// KeepFor is how long deliveries are kept.
	KeepFor = 30 * 24 * time.Hour
)

// Wants is whether an endpoint listening for kinds (none: every event;
// "todo.*": a family) hears an event. Sign-in and security events (on the
// security subjects) go only to endpoints that ask for them.
func Wants(kinds []string, includeSecurity bool, kind, subjectType string, securitySubjects []string) bool {
	if slices.Contains(securitySubjects, subjectType) && !includeSecurity {
		return false
	}
	if len(kinds) == 0 {
		return true
	}
	for _, k := range kinds {
		if prefix, ok := strings.CutSuffix(k, "*"); ok && strings.HasSuffix(k, ".*") {
			if strings.HasPrefix(kind, prefix) {
				return true
			}
		} else if k == kind {
			return true
		}
	}
	return false
}

var kindSplit = regexp.MustCompile(`[\s,]+`)

// KindsFromText reads "agreement.approved, todo.*" into kinds; nil for
// every event.
func KindsFromText(text string) []string {
	var out []string
	for _, k := range kindSplit.Split(text, -1) {
		if k = strings.TrimSpace(k); k != "" && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out
}

const base58 = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// NewSecret is a signing secret as the Rails core made them: whsec_ and 32
// base58 characters.
func NewSecret() string {
	b := make([]byte, 32)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(base58))))
		b[i] = base58[n.Int64()]
	}
	return "whsec_" + string(b)
}

// Sign is the signature header's value for a body sent at timestamp.
func Sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// Backoff is how long to wait before try number attempts+1, growing
// polynomially (Active Job's polynomially_longer: attempts⁴ + 2 seconds).
func Backoff(attempts int) time.Duration {
	return time.Duration(attempts*attempts*attempts*attempts+2) * time.Second
}

// Result is how one try went.
type Result struct {
	Status   int    // 0 when nothing answered
	Error    string // "" when it worked
	Duration time.Duration
}

// OK is whether the endpoint took it (a 2xx).
func (r Result) OK() bool { return r.Status >= 200 && r.Status < 300 }

// Post sends one delivery's body to url, signed with secret.
func Post(ctx context.Context, url, secret, kind string, deliveryID int64, body []byte, userAgent string) Result {
	start := time.Now()
	done := func(status int, err string) Result {
		if len(err) > 200 {
			err = err[:199] + "…"
		}
		return Result{Status: status, Error: err, Duration: time.Since(start)}
	}
	if err := outbound.CheckURL(url); err != nil {
		return done(0, err.Error())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return done(0, err.Error())
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-Runwell-Event", kind)
	req.Header.Set("X-Runwell-Delivery", strconv.FormatInt(deliveryID, 10))
	req.Header.Set("X-Runwell-Timestamp", ts)
	req.Header.Set("X-Runwell-Signature", Sign(secret, ts, body))
	res, err := outbound.Client.Do(req)
	if err != nil {
		return done(0, err.Error())
	}
	defer res.Body.Close()
	// Read some of the answer, so the connection can be reused; it isn't kept.
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<16))
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return done(res.StatusCode, strings.TrimSpace(strings.TrimPrefix(res.Status, strconv.Itoa(res.StatusCode))))
	}
	return done(res.StatusCode, "")
}
