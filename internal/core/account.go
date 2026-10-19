package core

import (
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"slices"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/secret"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Settings › Your account, for everyone: choices that are yours alone, not
// the install's. Your name, address, password and picture; how you sign in
// (an emailed link, Google…, two-factor); text size, which beats the
// install's default (Settings › Appearance); and your menu.

func init() { routeSets = append(routeSets, (*App).accountRoutes) }

func (a *App) accountRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/settings/account", a.showAccount, tool("show_account", "Show your account settings",
		note("Your own choices: text size (blank follows the install's default).")))
	a.handle(mux, "PATCH /api/settings/account", a.updateAccount, tool("update_account", "Change your account settings",
		note("Set your own text size, or blank to follow the install's default. name changes the name people see; email_address (with password_challenge, your password) the address you sign in with."),
		body("user", `{text_size: small|default|large|larger|"", name, email_address, password_challenge}`, true)))
	a.handle(mux, "PATCH /api/settings/account/password", a.updateOwnPassword)
	a.handle(mux, "POST /api/settings/account/avatar", a.uploadAvatar)
	a.handle(mux, "DELETE /api/settings/account/avatar", a.removeAvatar)
	a.handle(mux, "PATCH /api/settings/menu", a.updateMenu, tool("update_your_menu", "Arrange the plugins in your menu",
		note("Which plugins' links show in your nav, and their order (the g 1, g 2… keys follow it). order: every plugin key, in the order you want; shown: the keys to show (the rest are hidden). move: \"key:up\" or \"key:down\" to move one place."),
		bodyParam("order", "array", "Every plugin key, in the order you want.", false),
		bodyParam("shown", "array", "The keys to show; the rest are hidden.", false),
		bodyParam("move", "string", `"key:up" or "key:down" to move one place.`, false)))
	a.handle(mux, "PATCH /api/settings/passwordless", a.updatePasswordless)
	a.handle(mux, "DELETE /api/settings/identities/{id}", a.destroyIdentity)
}

// TextSizes are the text sizes, in order, with their names.
var TextSizes = [][2]string{{"small", "Small"}, {"default", "Default"}, {"large", "Large"}, {"larger", "Larger"}}

func choiceKeys(choices [][2]string) []string {
	out := make([]string, len(choices))
	for i, c := range choices {
		out[i] = c[0]
	}
	return out
}

func choiceName(choices [][2]string, key string) string {
	for _, c := range choices {
		if c[0] == key {
			return c[1]
		}
	}
	return key
}

func choicesObj(choices [][2]string) Obj {
	o := Obj{}
	for _, c := range choices {
		o = o.Set(c[0], c[1])
	}
	return o
}

func (a *App) showAccount(c *Ctx) error {
	ctx := c.R.Context()
	me := c.Actor.User.Person()
	out, err := a.accountJSON(ctx, me)
	if err != nil {
		return err
	}
	c.Summary = str(out.Get("summary"))
	return c.JSON(200, out)
}

func (a *App) accountJSON(ctx context.Context, me *User) (Obj, error) {
	s, err := a.settings(ctx)
	if err != nil {
		return nil, err
	}
	var textSize string
	var passwordless bool
	if err := a.store.QueryRowContext(ctx, `SELECT COALESCE(text_size, ''), passwordless FROM users WHERE id = ?`, me.ID).Scan(&textSize, &passwordless); err != nil {
		return nil, err
	}
	tf, err := a.loadTwoFactor(ctx, a.store, me.ID)
	if err != nil {
		return nil, err
	}
	identities, err := a.identitiesJSON(ctx, me.ID)
	if err != nil {
		return nil, err
	}
	providers, err := a.usableProviders(ctx)
	if err != nil {
		return nil, err
	}
	menu, err := a.menuJSON(ctx, me.ID)
	if err != nil {
		return nil, err
	}
	avatar, err := a.attachedBlob(ctx, a.store, "User", me.ID, "avatar")
	if err != nil {
		return nil, err
	}
	summary := "Text size follows the install's default (" + choiceName(TextSizes, s.TextSize) + ")"
	var ts any
	if textSize != "" {
		ts = textSize
		summary = "Text size " + choiceName(TextSizes, textSize)
	}
	var since any
	if tf.On() {
		since = tf.EnabledAt
	}
	provs := []Obj{}
	for _, k := range providers {
		provs = append(provs, O("key", k, "name", a.providerName(ctx, k)))
	}
	return O("summary", summary, "id", me.ID, "name", me.Name, "email_address", me.Email, "role", me.Role,
		"text_size", ts, "install_text_size", s.TextSize, "text_sizes", choicesObj(TextSizes), "passwordless", passwordless,
		"providers_only", s.ProvidersOnly && me.Role != "owner",
		"two_factor", O("on", tf.On(), "since", since, "recovery_codes_left", len(tf.Codes), "required", s.RequireTwoFactor),
		"identities", identities, "providers", provs, "menu", menu, "avatar", avatar != nil), nil
}

func (a *App) updateAccount(c *Ctx) error {
	body, err := c.Wrapped("user")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	me := c.Actor.User.Person()
	summary := "Saved."
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if has(body, "text_size") {
			ts := str(body["text_size"])
			if !slices.Contains(choiceKeys(TextSizes), ts) {
				ts = ""
			}
			if err := t.update("users", me.ID, map[string]any{"text_size": nullable(ts)}); err != nil {
				return err
			}
			summary = "Text size follows the install's default."
			if ts != "" {
				summary = "Text size set to " + choiceName(TextSizes, ts) + "."
			}
		}
		profile := map[string]any{}
		for _, k := range []string{"name", "email_address"} {
			if has(body, k) {
				profile[k] = body[k]
			}
		}
		if len(profile) == 0 {
			return nil
		}
		if email, ok := profile["email_address"]; ok && !strings.EqualFold(strings.TrimSpace(str(email)), me.Email) {
			var digest string
			if err := t.QueryRow(`SELECT password_digest FROM users WHERE id = ?`, me.ID).Scan(&digest); err != nil {
				return err
			}
			if !secret.CheckPassword(digest, str(body["password_challenge"])) {
				bad := apierr.Invalid{}
				bad.Add("password_challenge", "is invalid")
				return fullInvalid(bad)
			}
		}
		summary = "Your account is saved."
		return t.updateUserProfile(me, profile)
	})
	if err != nil {
		return err
	}
	out, err := a.accountJSON(ctx, me)
	if err != nil {
		return err
	}
	c.Summary = summary
	return c.JSON(200, out.Set("summary", summary))
}

// updateOwnPassword changes your password, with the current one; every
// other browser you're signed in on is signed out.
func (a *App) updateOwnPassword(c *Ctx) error {
	if err := refuseAgent(c); err != nil {
		return err
	}
	body, err := c.Wrapped("user")
	if err != nil {
		return err
	}
	if err := a.checkOwnPassword(c, str(body["password_challenge"])); err != nil {
		bad := apierr.Invalid{}
		bad.Add("password_challenge", "is invalid")
		return fullInvalid(bad)
	}
	confirmation := str(body["password_confirmation"])
	if err := checkNewPassword(str(body["password"]), &confirmation); err != nil {
		return err
	}
	me := c.Actor.User.Person()
	keep := int64(0)
	if c.Actor.Session != nil {
		keep = c.Actor.Session.ID
	}
	err = a.write(c.R.Context(), c.Actor, func(t *Tx) error {
		digest, err := secret.HashPassword(str(body["password"]))
		if err != nil {
			return err
		}
		if _, err := t.Exec(`UPDATE users SET password_digest = ?, updated_at = ? WHERE id = ?`, digest, store.Now().DB(), me.ID); err != nil {
			return err
		}
		if err := t.dropAccountTokens(purposePasswordReset, me.ID); err != nil {
			return err
		}
		if _, err := t.Exec(`DELETE FROM sessions WHERE user_id = ? AND id != ?`, me.ID, keep); err != nil {
			return err
		}
		return t.Event("User", me.ID, "user.password_changed", nil)
	})
	if err != nil {
		return err
	}
	c.Summary = "Your password is changed. Any other browser you were signed in on is signed out."
	return c.JSON(200, O("summary", c.Summary))
}

// formFile is the upload in field of a multipart request (at most
// MaxUpload); its file is the caller's to close.
func formFile(c *Ctx, field string) (multipart.File, *multipart.FileHeader, error) {
	c.R.Body = http.MaxBytesReader(c.W, c.R.Body, MaxUpload()+1<<20)
	if err := c.R.ParseMultipartForm(32 << 20); err != nil {
		if _, ok := err.(*http.MaxBytesError); ok {
			return nil, nil, apierr.TooLarge(MaxUpload())
		}
		return nil, nil, apierr.BadRequest("Send the file as multipart/form-data, in " + field + ".")
	}
	f, h, err := c.R.FormFile(field)
	if err != nil {
		return nil, nil, apierr.BadRequest("Send the file as multipart/form-data, in " + field + ".")
	}
	return f, h, nil
}

// imageTypes are what a picture may be (an avatar, the logo).
var imageTypes = []string{"image/png", "image/jpeg", "image/gif", "image/webp"}

func (a *App) uploadAvatar(c *Ctx) error {
	f, h, err := formFile(c, "avatar")
	if err != nil {
		return err
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	ct := http.DetectContentType(head[:n])
	if !slices.Contains(imageTypes, ct) {
		bad := apierr.Invalid{}
		bad.Add("avatar", "should be a PNG, JPEG, GIF or WebP image")
		return fullInvalid(bad)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	me := c.Actor.User.Person()
	err = a.write(c.R.Context(), c.Actor, func(t *Tx) error {
		b, err := t.putBlob(h.Filename, ct, f)
		if err != nil {
			if err == errTooLarge {
				return apierr.TooLarge(MaxUpload())
			}
			return err
		}
		return t.attach("User", me.ID, "avatar", b)
	})
	if err != nil {
		return err
	}
	c.Summary = "Your picture is saved."
	return c.JSON(200, O("summary", c.Summary, "avatar", true))
}

func (a *App) removeAvatar(c *Ctx) error {
	me := c.Actor.User.Person()
	if err := a.write(c.R.Context(), c.Actor, func(t *Tx) error { return t.detach("User", me.ID, "avatar") }); err != nil {
		return err
	}
	c.Summary = "Your picture is removed."
	return c.JSON(200, O("summary", c.Summary, "avatar", false))
}

func (a *App) updatePasswordless(c *Ctx) error {
	if err := refuseAgent(c); err != nil {
		return err
	}
	body, err := c.Wrapped("user")
	if err != nil {
		return err
	}
	on := boolOf(body["passwordless"])
	me := c.Actor.User.Person()
	err = a.write(c.R.Context(), c.Actor, func(t *Tx) error {
		if err := t.update("users", me.ID, map[string]any{"passwordless": on}); err != nil {
			return err
		}
		kind := "user.link_sign_in_off"
		if on {
			kind = "user.link_sign_in_on"
		}
		return t.Event("User", me.ID, kind, nil)
	})
	if err != nil {
		return err
	}
	c.Summary = "Sign-in by link is off."
	if on {
		c.Summary = "You can sign in with an emailed link."
	}
	return c.JSON(200, O("summary", c.Summary, "passwordless", on))
}

// ---- your menu -------------------------------------------------------------

// pluginNavItems, when set (the plugins area), lists every page of the
// plugins that are on, in the order they registered: [{key, label, href}].
// The account area arranges them per person for the sidebar (navPlugins).
var pluginNavItems func(a *App, ctx context.Context) ([]Obj, error)

// navArrangement is a person's nav_plugins: their order, and what's hidden.
func (a *App) navArrangement(ctx context.Context, userID int64) (order, hidden []string) {
	var raw store.JSON
	_ = a.store.QueryRowContext(ctx, `SELECT nav_plugins FROM users WHERE id = ?`, userID).Scan(&raw)
	m := raw.Map()
	for _, v := range asList(m["order"]) {
		order = append(order, str(v))
	}
	for _, v := range asList(m["hidden"]) {
		hidden = append(hidden, str(v))
	}
	return order, hidden
}

// arrangedNavItems are every plugin page in the person's order (theirs
// first, the rest after in registered order), each marked shown or not.
func (a *App) arrangedNavItems(ctx context.Context, userID int64) ([]Obj, error) {
	if pluginNavItems == nil {
		return []Obj{}, nil
	}
	items, err := pluginNavItems(a, ctx)
	if err != nil {
		return nil, err
	}
	byKey := map[string]Obj{}
	var keys []string
	for _, it := range items {
		k := str(it.Get("key"))
		byKey[k] = it
		keys = append(keys, k)
	}
	order, hidden := a.navArrangement(ctx, userID)
	var arranged []string
	for _, k := range order {
		if _, ok := byKey[k]; ok && !slices.Contains(arranged, k) {
			arranged = append(arranged, k)
		}
	}
	for _, k := range keys {
		if !slices.Contains(arranged, k) {
			arranged = append(arranged, k)
		}
	}
	out := []Obj{}
	for _, k := range arranged {
		out = append(out, slices.Clone(byKey[k]).Set("shown", !slices.Contains(hidden, k)))
	}
	return out, nil
}

func (a *App) menuJSON(ctx context.Context, userID int64) ([]Obj, error) {
	return a.arrangedNavItems(ctx, userID)
}

func (a *App) updateMenu(c *Ctx) error {
	var body struct {
		Order []any  `json:"order"`
		Shown []any  `json:"shown"`
		Move  string `json:"move"`
	}
	if err := c.OptionalBody(&body); err != nil {
		return err
	}
	ctx := c.R.Context()
	me := c.Actor.User.Person()
	items, err := a.arrangedNavItems(ctx, me.ID)
	if err != nil {
		return err
	}
	var keys []string
	labels := map[string]string{}
	for _, it := range items {
		k := str(it.Get("key"))
		keys = append(keys, k)
		labels[k] = str(it.Get("label"))
	}
	var order []string
	for _, v := range body.Order {
		if k := str(v); slices.Contains(keys, k) && !slices.Contains(order, k) {
			order = append(order, k)
		}
	}
	for _, k := range keys {
		if !slices.Contains(order, k) {
			order = append(order, k)
		}
	}
	var shown []string
	for _, v := range body.Shown {
		if k := str(v); slices.Contains(keys, k) {
			shown = append(shown, k)
		}
	}
	moved := ""
	if key, dir, ok := strings.Cut(body.Move, ":"); ok {
		if from := slices.Index(order, key); from >= 0 {
			to := from + 1
			if dir == "up" {
				to = from - 1
			}
			if to >= 0 && to < len(order) {
				order[from], order[to] = order[to], order[from]
				moved = labels[key]
			}
		}
	}
	hidden := []string{}
	for _, k := range order {
		if !slices.Contains(shown, k) {
			hidden = append(hidden, k)
		}
	}
	if order == nil {
		order = []string{}
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		return t.update("users", me.ID, map[string]any{"nav_plugins": string(store.MustJSON(map[string]any{"order": order, "hidden": hidden}))})
	})
	if err != nil {
		return err
	}
	c.Summary = "Your menu is saved."
	if moved != "" {
		c.Summary = "Moved " + moved + "."
	}
	menu, err := a.menuJSON(ctx, me.ID)
	if err != nil {
		return err
	}
	return c.JSON(200, O("summary", c.Summary, "menu", menu))
}
