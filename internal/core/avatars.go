package core

import (
	"fmt"
	"hash/crc32"
	"html"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// Avatars: a person's picture (their "avatar" attachment), else their
// initials on a color picked from their id, as the Rails core drew them.
// Open, like the Rails one: an emailed notification shows it too.

// AvatarColors are the initials' backgrounds (User::Avatar::AVATAR_COLORS).
var AvatarColors = []string{
	"#AF2E1B", "#CC6324", "#3B4B59", "#BFA07A", "#ED8008", "#ED3F1C", "#BF1B1B", "#736B1E", "#D07B53",
	"#736356", "#AD1D1D", "#BF7C2A", "#C09C6F", "#698F9C", "#7C956B", "#5D618F", "#3B3633", "#67695E",
}

func init() { routeSets = append(routeSets, (*App).avatarRoutes) }

func (a *App) avatarRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/users/{id}/avatar", a.showAvatar, open)
}

// avatarColor is a person's color: crc32 of their id, as Rails picks it.
func avatarColor(id int64) string {
	return AvatarColors[crc32.ChecksumIEEE([]byte(strconv.FormatInt(id, 10)))%uint32(len(AvatarColors))] //nolint:gosec // a fixed list of 18
}

// nameInitials are the first letter of each word, upper case ("Ann Bell" →
// "AB"), as Rails' name.scan(/\b\p{L}/).
func nameInitials(name string) string {
	var b strings.Builder
	inWord := false
	for _, r := range name {
		word := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
		if word && !inWord && unicode.IsLetter(r) {
			b.WriteRune(unicode.ToUpper(r))
		}
		inWord = word
	}
	return b.String()
}

func (a *App) showAvatar(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	var name, email string
	var updated store.Time
	err = a.store.QueryRowContext(ctx, `SELECT name, email_address, updated_at FROM users WHERE id = ?`, id).Scan(&name, &email, &updated)
	if isNoRows(err) {
		return notFound("user")
	} else if err != nil {
		return err
	}
	if b, err := a.attachedBlob(ctx, a.store, "User", id, "avatar"); err != nil {
		return err
	} else if b != nil && strings.HasPrefix(b.ContentType, "image/") {
		a.serveBlob(c.W, c.R, b, false)
		return nil
	}
	// Someone else's is cached a while; your own follows a rename at once.
	etag := fmt.Sprintf(`"avatar-%d-%d"`, id, updated.UnixNano())
	h := c.W.Header()
	h.Set("ETag", etag)
	if c.Actor != nil && c.Actor.User != nil && c.Actor.User.Person().ID == id {
		h.Set("Cache-Control", "private, no-cache")
	} else {
		h.Set("Cache-Control", "private, max-age=1800, stale-while-revalidate=604800")
	}
	if c.R.Header.Get("If-None-Match") == etag {
		c.W.WriteHeader(http.StatusNotModified)
		return nil
	}
	if name == "" {
		name = email
	}
	initials := nameInitials(name)
	fit := ""
	if len([]rune(initials)) >= 3 {
		fit = ` textLength="85%" lengthAdjust="spacingAndGlyphs"`
	}
	svg := `<svg version="1.1" xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 512 512" class="avatar" aria-hidden="true">
  <defs><clipPath id="porthole"><circle cx="50%" cy="50%" r="50%" /></clipPath></defs>
  <g>
    <rect width="100%" height="100%" rx="50" fill="` + avatarColor(id) + `" />
    <text x="50%" y="50%" fill="#FFFFFF" text-anchor="middle" dy="0.35em"` + fit + `
      font-family="-apple-system, BlinkMacSystemFont, Segoe UI, Roboto, Helvetica, Arial, sans-serif" font-size="230" font-weight="800" letter-spacing="-5">` +
		html.EscapeString(initials) + `</text>
  </g>
</svg>
`
	h.Set("Content-Type", "image/svg+xml")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	c.W.WriteHeader(http.StatusOK)
	_, err = c.W.Write([]byte(svg))
	return err
}
