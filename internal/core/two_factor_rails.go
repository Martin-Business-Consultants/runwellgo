package core

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/sha1" //nolint:gosec // Rails' older key derivation, read only
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"hash"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/secret"
)

// Values the Rails core kept with Active Record encryption (a person's
// two-factor secret, a sign-in provider's client secret, a webhook's
// signing secret) open here when the environment carries the Rails
// install's keys (ACTIVE_RECORD_ENCRYPTION_PRIMARY_KEY and
// ACTIVE_RECORD_ENCRYPTION_KEY_DERIVATION_SALT, as bin/install made them),
// so a Rails install's people keep their two-factor sign-in. What Go
// writes is sealed with the install's own key.

// railsKeys are the AES keys Active Record encryption derives from its
// primary keys: PBKDF2 over the salt, 2^16 rounds, SHA-256 (Rails 7.1+)
// and SHA-1 (older data).
var railsKeys = sync.OnceValue(func() [][]byte {
	primary := os.Getenv("ACTIVE_RECORD_ENCRYPTION_PRIMARY_KEY")
	salt := os.Getenv("ACTIVE_RECORD_ENCRYPTION_KEY_DERIVATION_SALT")
	if primary == "" || salt == "" {
		return nil
	}
	var keys [][]byte
	for p := range strings.SplitSeq(primary, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		for _, h := range []func() hash.Hash{sha256.New, sha1.New} {
			if k, err := pbkdf2.Key(h, p, []byte(salt), 1<<16, 32); err == nil {
				keys = append(keys, k)
			}
		}
	}
	return keys
})

// railsDecrypt opens an Active Record encrypted value ({"p": …, "h":
// {"iv": …, "at": …}}); ok false when it isn't one or no key opens it.
func railsDecrypt(stored string) (string, bool) {
	if !strings.HasPrefix(strings.TrimSpace(stored), "{") {
		return "", false
	}
	var msg struct {
		P string         `json:"p"`
		H map[string]any `json:"h"`
	}
	if json.Unmarshal([]byte(stored), &msg) != nil || msg.P == "" {
		return "", false
	}
	payload, err1 := base64.StdEncoding.DecodeString(msg.P)
	iv, err2 := base64.StdEncoding.DecodeString(str(msg.H["iv"]))
	tag, err3 := base64.StdEncoding.DecodeString(str(msg.H["at"]))
	if err1 != nil || err2 != nil || err3 != nil || len(tag) != 16 {
		return "", false
	}
	sealed := append(append([]byte{}, payload...), tag...)
	for _, key := range railsKeys() {
		block, err := aes.NewCipher(key)
		if err != nil {
			continue
		}
		gcm, err := cipher.NewGCMWithNonceSize(block, len(iv))
		if err != nil {
			continue
		}
		plain, err := gcm.Open(nil, iv, sealed, nil)
		if err != nil {
			continue
		}
		if c, _ := msg.H["c"].(bool); c {
			r, err := zlib.NewReader(bytes.NewReader(plain))
			if err != nil {
				return "", false
			}
			if plain, err = io.ReadAll(io.LimitReader(r, 1<<20)); err != nil {
				return "", false
			}
		}
		return string(plain), true
	}
	return "", false
}

// openSecret reads a secret column however it was written: sealed by this
// install, encrypted by the Rails core, or (an import, a seed) in the
// clear. ok false for one that can't be read here.
func (a *App) openSecret(stored string) (string, bool) {
	switch {
	case stored == "":
		return "", false
	case strings.HasPrefix(stored, secret.SealedPrefix):
		return a.secrets.Open(stored)
	case strings.HasPrefix(strings.TrimSpace(stored), "{"):
		return railsDecrypt(stored)
	}
	return stored, true
}

// railsSecretKeyBase is what the Rails core signed with (SECRET_KEY_BASE),
// else the install's own key: recovery codes are digested with it, so a
// Rails person's codes keep working when the environment carries it.
func (a *App) railsSecretKeyBase() []byte {
	if k := os.Getenv("SECRET_KEY_BASE"); k != "" {
		return []byte(k)
	}
	return []byte(a.secrets.Hex())
}
