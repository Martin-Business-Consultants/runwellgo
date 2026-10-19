package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
)

// Settings › Appearance: the theme everyone starts on, the colors, corners,
// font and text size, custom CSS, and the logo and favicon. The app reads
// them at startup from GET /api/appearance (open: the sign-in page and the
// portal look the same) and applies them (ui/src/lib/appearance.svelte.js).

func init() { routeSets = append(routeSets, (*App).appearanceRoutes) }

// The choices, in order, with their names.
var (
	Themes  = [][2]string{{"system", "System"}, {"light", "Light"}, {"dark", "Dark"}}
	Radii   = [][2]string{{"sharp", "Sharp"}, {"fizzy", "Soft"}, {"round", "Round"}}
	Schemes = [][2]string{{"fizzy", "Indigo"}, {"forest", "Forest"}, {"plum", "Plum"}, {"ocean", "Ocean"}, {"graphite", "Graphite"}}
	Fonts   = [][2]string{{"system", "System"}, {"humanist", "Humanist"}, {"rounded", "Rounded"}, {"serif", "Serif"}, {"mono", "Mono"}}
)

// customCSSLimit is the most custom CSS may be.
const customCSSLimit = 100 << 10

// customCSSForbidden is what custom CSS may not use: it can't reach outside
// the install or run anything.
var customCSSForbidden = []struct {
	re   *regexp.Regexp
	name string
}{
	{regexp.MustCompile(`(?i)@import`), "@import"},
	{regexp.MustCompile(`(?i)url\(\s*(["']?\s*data:)?`), "url() other than a data: URI"},
	{regexp.MustCompile(`(?i)expression\s*\(`), "expression()"},
	{regexp.MustCompile(`(?i)javascript\s*:`), "javascript:"},
	{regexp.MustCompile(`(?i)behavior\s*:`), "behavior"},
	{regexp.MustCompile(`(?i)-moz-binding`), "-moz-binding"},
	{regexp.MustCompile(`</`), "</"},
}

func (a *App) appearanceRoutes(mux *http.ServeMux) {
	ms := need("manage_settings")
	a.handle(mux, "GET /api/settings/appearance", a.showAppearance, ms, tool("show_appearance", "Show appearance settings"))
	a.handle(mux, "PATCH /api/settings/appearance", a.updateAppearance, ms, tool("update_appearance", "Change appearance",
		body("setting", `{theme: system|light|dark, radius: sharp|fizzy|round, scheme: fizzy|forest|plum|ocean|graphite, font: system|humanist|rounded|serif|mono, text_size: small|default|large|larger, custom_css: text, custom_css_everywhere: boolean}`, false),
		bodyParam("remove", "string", "logo or favicon: removes it.", false)))
	a.handle(mux, "GET /api/settings/theme_brief", a.showThemeBrief, ms, tool("theme_brief", "Get the brief for writing a theme",
		note("The theming guide (design tokens, rules, an example) with this install's appearance and custom CSS. Write a theme from it, then apply it with update_appearance (custom_css).")))
	a.handle(mux, "GET /api/appearance", a.publicAppearance, open)
	a.handle(mux, "GET /api/appearance/{name}", a.appearanceFile, open)
}

func (a *App) appearanceJSON(ctx context.Context, s *Settings) (Obj, error) {
	logo, err := a.attachedBlob(ctx, a.store, "Setting", s.ID, "logo")
	if err != nil {
		return nil, err
	}
	favicon, err := a.attachedBlob(ctx, a.store, "Setting", s.ID, "favicon")
	if err != nil {
		return nil, err
	}
	return O("summary", "Theme "+s.Theme+", "+s.Scheme+" colors, "+s.Radius+" corners, "+s.Font+" font",
		"theme", s.Theme, "scheme", s.Scheme, "radius", s.Radius, "font", s.Font, "text_size", s.TextSize,
		"logo", logo != nil, "favicon", favicon != nil, "custom_css", nullable(s.CustomCSS), "custom_css_everywhere", s.CustomCSSEverywhere,
		"choices", O("theme", choiceKeys(Themes), "scheme", choiceKeys(Schemes), "radius", choiceKeys(Radii), "font", choiceKeys(Fonts), "text_size", choiceKeys(TextSizes)),
		"names", O("theme", choicesObj(Themes), "scheme", choicesObj(Schemes), "radius", choicesObj(Radii), "font", choicesObj(Fonts), "text_size", choicesObj(TextSizes))), nil
}

func (a *App) showAppearance(c *Ctx) error {
	ctx := c.R.Context()
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	out, err := a.appearanceJSON(ctx, s)
	if err != nil {
		return err
	}
	c.Summary = str(out.Get("summary"))
	return c.JSON(200, out)
}

// checkCustomCSS is what's wrong with css, in words; nil when it's safe.
func checkCustomCSS(css string) []string {
	var problems []string
	if len(css) > customCSSLimit {
		problems = append(problems, "is over 100 KB")
	}
	var found []string
	for _, f := range customCSSForbidden {
		for _, m := range f.re.FindAllStringSubmatch(css, -1) {
			if f.name == "url() other than a data: URI" && len(m) > 1 && m[1] != "" {
				continue
			}
			found = append(found, f.name)
			break
		}
	}
	if len(found) > 0 {
		problems = append(problems, "can't use "+toSentence(found, "and"))
	}
	return problems
}

func (a *App) updateAppearance(c *Ctx) error {
	ctx := c.R.Context()
	fields := map[string]any{}
	remove := ""
	type upload struct {
		name, filename, ct string
		r                  io.ReadSeeker
	}
	var uploads []upload
	if strings.HasPrefix(c.R.Header.Get("Content-Type"), "multipart/") {
		c.R.Body = http.MaxBytesReader(c.W, c.R.Body, MaxUpload()+1<<20)
		if err := c.R.ParseMultipartForm(32 << 20); err != nil {
			return apierr.BadRequest("Send the files as multipart/form-data.")
		}
		for k, v := range c.R.MultipartForm.Value {
			key := strings.TrimSuffix(strings.TrimPrefix(k, "setting["), "]")
			if k == "remove" {
				remove = v[0]
			} else if len(v) > 0 {
				fields[key] = v[0]
			}
		}
		for _, name := range []string{"logo", "favicon"} {
			for _, k := range []string{name, "setting[" + name + "]"} {
				f, h, err := c.R.FormFile(k)
				if err != nil {
					continue
				}
				defer f.Close()
				head := make([]byte, 512)
				n, _ := io.ReadFull(f, head)
				ct := http.DetectContentType(head[:n])
				if strings.HasSuffix(strings.ToLower(h.Filename), ".svg") {
					ct = "image/svg+xml"
				} else if strings.HasSuffix(strings.ToLower(h.Filename), ".ico") {
					ct = "image/x-icon"
				}
				if !slices.Contains(append(slices.Clone(imageTypes), "image/svg+xml", "image/x-icon", "image/vnd.microsoft.icon"), ct) {
					bad := apierr.Invalid{}
					bad.Add(name, "should be a PNG, SVG, JPEG, WebP or ICO image")
					return fullInvalid(bad)
				}
				if _, err := f.Seek(0, io.SeekStart); err != nil {
					return err
				}
				uploads = append(uploads, upload{name, h.Filename, ct, f})
			}
		}
	} else {
		var body struct {
			Setting map[string]any `json:"setting"`
			Remove  string         `json:"remove"`
		}
		if err := c.OptionalBody(&body); err != nil {
			return err
		}
		if body.Setting != nil {
			fields = body.Setting
		}
		remove = body.Remove
	}
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	set := map[string]any{}
	bad := apierr.Invalid{}
	for _, f := range []struct {
		key     string
		choices [][2]string
	}{{"theme", Themes}, {"radius", Radii}, {"scheme", Schemes}, {"font", Fonts}, {"text_size", TextSizes}} {
		if !has(fields, f.key) {
			continue
		}
		v := str(fields[f.key])
		if !slices.Contains(choiceKeys(f.choices), v) {
			bad.Add(f.key, "is not included in the list")
			continue
		}
		set[f.key] = v
	}
	cssChanged := false
	if has(fields, "custom_css") {
		css := strings.TrimSpace(str(fields["custom_css"]))
		for _, p := range checkCustomCSS(css) {
			bad.Add("custom_css", p)
		}
		if css != s.CustomCSS {
			cssChanged = true
		}
		set["custom_css"] = nullable(css)
	}
	if has(fields, "custom_css_everywhere") {
		set["custom_css_everywhere"] = boolOf(fields["custom_css_everywhere"])
	}
	if err := fullInvalid(bad); err != nil {
		return err
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if err := t.update("settings", s.ID, set); err != nil {
			return err
		}
		if cssChanged {
			var digest any
			if css := str(set["custom_css"]); css != "" {
				sum := sha256.Sum256([]byte(css))
				digest = hex.EncodeToString(sum[:])[:16]
			}
			if err := t.Event("Setting", s.ID, "settings.custom_css", map[string]any{"digest": digest}); err != nil {
				return err
			}
		}
		for _, u := range uploads {
			b, err := t.putBlob(u.filename, u.ct, u.r)
			if err != nil {
				if err == errTooLarge {
					return apierr.TooLarge(MaxUpload())
				}
				return err
			}
			if err := t.attach("Setting", s.ID, u.name, b); err != nil {
				return err
			}
		}
		if remove == "logo" || remove == "favicon" {
			return t.detach("Setting", s.ID, remove)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if s, err = a.settings(ctx); err != nil {
		return err
	}
	out, err := a.appearanceJSON(ctx, s)
	if err != nil {
		return err
	}
	c.Summary = "Appearance saved."
	return c.JSON(200, out.Set("summary", c.Summary))
}

// publicAppearance is what every screen starts from: the install's look,
// signed in or not.
func (a *App) publicAppearance(c *Ctx) error {
	ctx := c.R.Context()
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	logo, err := a.attachedBlob(ctx, a.store, "Setting", s.ID, "logo")
	if err != nil {
		return err
	}
	favicon, err := a.attachedBlob(ctx, a.store, "Setting", s.ID, "favicon")
	if err != nil {
		return err
	}
	var logoURL, faviconURL any
	if logo != nil {
		logoURL = "/api/appearance/logo?v=" + logo.Key[:8]
	}
	if favicon != nil {
		faviconURL = "/api/appearance/favicon?v=" + favicon.Key[:8]
	}
	c.W.Header().Set("Cache-Control", "no-cache")
	return c.JSON(200, O("theme", s.Theme, "scheme", s.Scheme, "radius", s.Radius, "font", s.Font, "text_size", s.TextSize,
		"custom_css", nullable(s.CustomCSS), "custom_css_everywhere", s.CustomCSSEverywhere, "logo_url", logoURL, "favicon_url", faviconURL,
		"brand", a.BrandName(ctx)))
}

// appearanceFile is the logo or the favicon.
func (a *App) appearanceFile(c *Ctx) error {
	name := c.Param("name")
	if name != "logo" && name != "favicon" {
		return notFound("file")
	}
	ctx := c.R.Context()
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	b, err := a.attachedBlob(ctx, a.store, "Setting", s.ID, name)
	if err != nil {
		return err
	}
	if b == nil {
		return notFound(name)
	}
	if b.ContentType == "image/svg+xml" {
		// An SVG logo shows in an <img>, where it can't run anything.
		h := c.W.Header()
		h.Set("Content-Type", "image/svg+xml")
		h.Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": b.Filename}))
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeFile(c.W, c.R, a.blobPath(b.Key))
		return nil
	}
	c.W.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	a.serveBlob(c.W, c.R, b, false)
	return nil
}

// ---- the brief for AI ----------------------------------------------------------

func (a *App) themeBrief(ctx context.Context) (string, error) {
	s, err := a.settings(ctx)
	if err != nil {
		return "", err
	}
	logo, err := a.attachedBlob(ctx, a.store, "Setting", s.ID, "logo")
	if err != nil {
		return "", err
	}
	brand := a.BrandName(ctx)
	who := "a Runwell install"
	if brand != "Runwell" {
		who = brand + "'s Runwell install"
	}
	hasLogo := "none yet"
	if logo != nil {
		hasLogo = "yes"
	}
	css := "No custom CSS yet."
	if s.CustomCSS != "" {
		css = "Its custom CSS now (change it rather than starting over, unless asked):\n\n```css\n" + s.CustomCSS + "\n```"
	}
	return "You're writing a theme for " + who + ": a stylesheet\nthat sets Runwell's design tokens. Follow the guide below exactly: tokens first, both light and\n" +
		"dark, nothing it forbids. Answer with one CSS block and a line on what you chose and why.\n\n" +
		"Ask me for the business's name, logo, website or colors if I haven't given them.\n\n" +
		"## This install now\n\n" +
		"- Theme: " + choiceName(Themes, s.Theme) + "; colors: " + choiceName(Schemes, s.Scheme) + "; corners: " + choiceName(Radii, s.Radius) +
		"; font: " + choiceName(Fonts, s.Font) + "; text size: " + choiceName(TextSizes, s.TextSize) + "\n" +
		"- Logo: " + hasLogo + "\n\n" + css + "\n\n---\n\n" + themingGuide, nil
}

func (a *App) showThemeBrief(c *Ctx) error {
	brief, err := a.themeBrief(c.R.Context())
	if err != nil {
		return err
	}
	if c.Query("format") == "text" || strings.HasPrefix(c.R.Header.Get("Accept"), "text/plain") {
		c.W.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, err := c.W.Write([]byte(brief))
		return err
	}
	c.Summary = "The theming guide and this install's appearance. Write a theme from it, then apply it with update_appearance (custom_css)."
	return c.JSON(200, O("summary", c.Summary, "brief", brief))
}

// themingGuide is the guide to Runwell's design tokens, for whoever writes
// a theme: the app is shadcn-svelte, so its tokens are shadcn's.
const themingGuide = `# Theming Runwell

Runwell's look is built on CSS custom properties (design tokens). A theme is a short stylesheet that
gives those tokens new values, so the whole app changes at once: every card, button, table and the
sidebar read them. Paste it into Settings › Appearance › Custom CSS.

Start with Settings › Appearance (theme, colors, corners, font, text size, logo and favicon). Reach
for custom CSS when those choices aren't enough to make Runwell look like your own.

## The contract

- **Tokens are the stable surface.** The tokens below keep their names and meaning across
  releases. A theme that only sets tokens keeps working when Runwell updates.
- **Class names are not.** Utility classes change as the app does. Don't target them.
- **Custom CSS loads last**, after the app's stylesheet and Appearance's choices (colors,
  corners, font), so a plain rule on ` + "`:root`" + ` and ` + "`.dark`" + ` wins without ` + "`!important`" + `.
- **Colors are any CSS color**; ` + "`oklch(…)`" + ` reads best: ` + "`--primary: oklch(0.55 0.16 277);`" + `.
- **Dark mode is a second block.** Set colors for light under ` + "`:root`" + `, and again for dark under ` + "`.dark`" + `.
- **Not allowed:** ` + "`@import`, `url()`" + ` other than ` + "`data:`" + ` URIs, ` + "`expression()`, `javascript:`, `behavior`, `-moz-binding`" + `
  and ` + "`</`" + `. Fonts must be ones people have installed, or embedded as ` + "`data:`" + ` URIs. Runwell refuses a
  stylesheet that breaks these rules.
- **Escape hatch:** add ` + "`?theme=off`" + ` to any address to see the page without the custom CSS.

## Tokens

| Token | What it colors |
| --- | --- |
| ` + "`--background` / `--foreground`" + ` | The page and its text |
| ` + "`--card` / `--card-foreground`" + ` | Cards and panels |
| ` + "`--popover` / `--popover-foreground`" + ` | Menus, popovers, dialogs |
| ` + "`--primary` / `--primary-foreground`" + ` | Buttons, the current item, links |
| ` + "`--secondary` / `--secondary-foreground`" + ` | Secondary buttons |
| ` + "`--muted` / `--muted-foreground`" + ` | Quiet fills and secondary text |
| ` + "`--accent` / `--accent-foreground`" + ` | Hovered rows and menu items |
| ` + "`--destructive`" + ` | Deleting, errors |
| ` + "`--border`, `--input`, `--ring`" + ` | Hairlines, input borders, focus rings |
| ` + "`--sidebar`, `--sidebar-foreground`, `--sidebar-primary`, `--sidebar-accent`, `--sidebar-border`" + ` | The sidebar |
| ` + "`--chart-1` … `--chart-5`" + ` | Charts |
| ` + "`--radius`" + ` | Corners (every radius is worked out from it) |
| ` + "`--app-font`" + ` | The font |

## An example

` + "```css" + `
:root {
  --primary: oklch(0.58 0.14 40);
  --ring: oklch(0.58 0.14 40);
  --sidebar-primary: oklch(0.58 0.14 40);
  --background: oklch(0.99 0.004 80);
  --radius: 0.75rem;
}
.dark {
  --primary: oklch(0.72 0.13 40);
  --ring: oklch(0.72 0.13 40);
  --sidebar-primary: oklch(0.72 0.13 40);
}
` + "```" + `
`
