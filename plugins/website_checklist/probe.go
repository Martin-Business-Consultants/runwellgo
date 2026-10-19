package websitechecklist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// What can be found out about a domain from outside, without logging in
// anywhere (the Rails Site::Probe): its DNS records, through Go's resolver
// (who hosts DNS and email, SPF, DMARC, verifications), the registry's RDAP
// record through rdap.org (the registrar, when it was registered and
// expires), how its addresses answer (HTTPS, www, where they redirect), its
// home page (title, platform, Analytics and Tag Manager IDs, forms), and
// robots.txt and the sitemap. Every part fails on its own, as an "error" in
// its facts. Files worth keeping (the sitemap) are in files. Only public
// addresses are fetched: the host's public client refuses any other.

const (
	fetchTimeout = 8 * time.Second
	maxBody      = 4<<20 - 1 // what a plugin may read of an answer
	maxRedirects = 5
	userAgent    = "Runwell website audit"
)

var domainFormat = regexp.MustCompile(`^[a-z0-9.-]+\.[a-z]{2,}$`)

// normalizeDomain is "https://www.Example.com/about" as "example.com".
func normalizeDomain(input string) string {
	text := strings.ToLower(strings.TrimSpace(input))
	raw := text
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return text
	}
	return strings.TrimPrefix(u.Hostname(), "www.")
}

// probeFile is a file worth keeping: its name, type and content.
type probeFile struct {
	name, contentType string
	body              []byte
}

type prober struct {
	api    *plugin.API
	client *http.Client
	domain string
	files  []probeFile

	robotsOnce sync.Once
	robots     obj
	robotsErr  error
}

// probe gathers the facts about domain, in the Rails probe's shape, and
// the files worth keeping.
func probe(ctx context.Context, api *plugin.API, domain string) (obj, []probeFile) {
	p := &prober{api: api, domain: domain}
	if c, err := api.PublicHTTP(); err == nil {
		// Redirects are followed by hand, so each hop is kept.
		cc := *c
		cc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		p.client = &cc
	}
	parts := []struct {
		name string
		fn   func(context.Context) (obj, error)
	}{
		{"dns", p.dns}, {"registration", p.registration}, {"addresses", p.addresses},
		{"home_page", p.homePage}, {"robots_txt", p.robotsTxt}, {"sitemap", p.sitemap},
	}
	out := make([]obj, len(parts))
	var wg sync.WaitGroup
	for i, pt := range parts {
		wg.Go(func() { out[i] = part(func() (obj, error) { return pt.fn(ctx) }) })
	}
	wg.Wait()
	facts := o("domain", domain)
	for i, pt := range parts {
		facts = append(facts, kv{pt.name, out[i]})
	}
	return facts, p.files
}

func part(fn func() (obj, error)) (out obj) {
	defer func() {
		if r := recover(); r != nil {
			out = o("error", truncate(fmt.Sprint(r), 200))
		}
	}()
	facts, err := fn()
	if err != nil {
		return o("error", truncate(err.Error(), 200))
	}
	return facts
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-3]) + "..."
}

// ---- DNS -----------------------------------------------------------------------------------

var (
	spfRecord     = regexp.MustCompile(`(?i)^v=spf1`)
	dmarcRecord   = regexp.MustCompile(`(?i)^v=DMARC1`)
	verification  = regexp.MustCompile(`(?i)verification|verify|site-verification|MS=|facebook-domain`)
	otherTXTSkips = regexp.MustCompile(`(?i)^v=spf1|verification|verify|MS=|facebook-domain`)
)

func (p *prober) dns(ctx context.Context) (obj, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	domain, r := p.domain, net.DefaultResolver
	host := func(s string) string { return strings.TrimSuffix(s, ".") }
	nameservers, wwwCNAME, mx := []string{}, []string{}, []string{}
	if list, err := r.LookupNS(ctx, domain); err == nil {
		for _, ns := range list {
			nameservers = append(nameservers, host(ns.Host))
		}
		slices.Sort(nameservers)
	}
	ips := func(name, network string) []string {
		out := []string{}
		list, _ := r.LookupIP(ctx, network, name)
		for _, ip := range list {
			out = append(out, ip.String())
		}
		return out
	}
	if cname, err := r.LookupCNAME(ctx, "www."+domain); err == nil && host(cname) != "www."+domain {
		wwwCNAME = append(wwwCNAME, host(cname))
	}
	if list, err := r.LookupMX(ctx, domain); err == nil {
		slices.SortStableFunc(list, func(x, y *net.MX) int { return int(x.Pref) - int(y.Pref) })
		for _, m := range list {
			mx = append(mx, fmt.Sprintf("%d %s", m.Pref, host(m.Host)))
		}
	}
	txt, _ := r.LookupTXT(ctx, domain)
	dmarcTXT, _ := r.LookupTXT(ctx, "_dmarc."+domain)
	grep := func(list []string, re *regexp.Regexp, keep bool) []string {
		out := []string{}
		for _, s := range list {
			if re.MatchString(s) == keep {
				out = append(out, s)
			}
		}
		return out
	}
	other := grep(txt, otherTXTSkips, false)
	for i := range other {
		other[i] = truncate(other[i], 120)
	}
	return o("nameservers", nameservers, "a", ips(domain, "ip4"), "aaaa", ips(domain, "ip6"), "www_cname", wwwCNAME, "www_a", ips("www."+domain, "ip4"),
		"mx", mx, "spf", grep(txt, spfRecord, true), "dmarc", grep(dmarcTXT, dmarcRecord, true), "verifications", grep(txt, verification, true), "other_txt", other), nil
}

// ---- HTTP ----------------------------------------------------------------------------------

type answer struct {
	status   int
	location string
	body     string
	headers  http.Header
	url      string
	hops     []string
	err      string
}

// fetch is one GET, its body read to the cap.
func (p *prober) fetch(ctx context.Context, u, accept string) (*answer, error) {
	if p.client == nil {
		return nil, fmt.Errorf("http:public wasn't granted to this plugin")
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return nil, fmt.Errorf("only http and https")
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", accept)
	res, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, maxBody))
	return &answer{status: res.StatusCode, location: res.Header.Get("Location"), body: strings.ToValidUTF8(string(body), "�"), headers: res.Header}, nil
}

// follow fetches u, following its redirects (each hop kept); an error is
// in the answer.
func (p *prober) follow(ctx context.Context, u, accept string) *answer {
	var hops []string
	for range maxRedirects {
		a, err := p.fetch(ctx, u, accept)
		if err != nil {
			return &answer{url: u, hops: hops, err: truncate(unwrap(err), 160)}
		}
		if a.status < 300 || a.status > 399 || a.location == "" {
			a.url, a.hops = u, hops
			return a
		}
		hops = append(hops, fmt.Sprintf("%d → %s", a.status, a.location))
		next, err := url.Parse(u)
		if err == nil {
			next, err = next.Parse(a.location)
		}
		if err != nil {
			return &answer{url: u, hops: hops, err: truncate(err.Error(), 160)}
		}
		u = next.String()
	}
	return &answer{url: u, hops: hops, err: "too many redirects"}
}

// unwrap is a fetch's error without Go's request preamble.
func unwrap(err error) string {
	if ue, ok := errors.AsType[*url.Error](err); ok {
		return ue.Err.Error()
	}
	return err.Error()
}

func (a *answer) failure() string {
	if a.err != "" {
		return a.err
	}
	return fmt.Sprintf("answered %d", a.status)
}

// registration is the registry's RDAP record (through rdap.org's redirect
// to the domain's registry).
func (p *prober) registration(ctx context.Context) (obj, error) {
	a := p.follow(ctx, "https://rdap.org/domain/"+url.PathEscape(p.domain), "application/rdap+json")
	if a.status != http.StatusOK {
		if a.err != "" {
			return nil, fmt.Errorf("the registry answered %s", a.err)
		}
		return nil, fmt.Errorf("the registry answered %d", a.status)
	}
	var data struct {
		Entities []struct {
			Roles      []string          `json:"roles"`
			VcardArray []json.RawMessage `json:"vcardArray"`
			Links      []struct {
				Href string `json:"href"`
			} `json:"links"`
		} `json:"entities"`
		Events []struct {
			Action string `json:"eventAction"`
			Date   string `json:"eventDate"`
		} `json:"events"`
		Status      []string `json:"status"`
		Nameservers []struct {
			LDHName string `json:"ldhName"`
		} `json:"nameservers"`
	}
	if err := json.Unmarshal([]byte(a.body), &data); err != nil {
		return nil, err
	}
	var registrar, registrarURL any
	for _, e := range data.Entities {
		if !slices.Contains(e.Roles, "registrar") {
			continue
		}
		if len(e.VcardArray) > 1 {
			var props [][]any
			if json.Unmarshal(e.VcardArray[1], &props) == nil {
				for _, pr := range props {
					if len(pr) > 0 && pr[0] == "fn" {
						registrar = pr[len(pr)-1]
					}
				}
			}
		}
		if len(e.Links) > 0 {
			registrarURL = e.Links[0].Href
		}
		break
	}
	events := map[string]any{}
	for _, e := range data.Events {
		events[e.Action] = e.Date
	}
	ns := []string{}
	for _, n := range data.Nameservers {
		ns = append(ns, strings.ToLower(n.LDHName))
	}
	slices.Sort(ns)
	return o("registrar", registrar, "registrar_url", registrarURL, "registered", events["registration"], "expires", events["expiration"],
		"last_changed", events["last changed"], "status", data.Status, "nameservers", ns), nil
}

// addresses is how https://, https://www. and http:// answer, and where
// they end up.
func (p *prober) addresses(ctx context.Context) (obj, error) {
	urls := []string{"https://" + p.domain + "/", "https://www." + p.domain + "/", "http://" + p.domain + "/"}
	answers := make([]*answer, len(urls))
	var wg sync.WaitGroup
	for i, u := range urls {
		wg.Go(func() { answers[i] = p.follow(ctx, u, "*/*") })
	}
	wg.Wait()
	out := obj{}
	for i, u := range urls {
		a := answers[i]
		row := obj{}
		if a.status != 0 {
			row = append(row, kv{"status", a.status})
		}
		row = append(row, kv{"ends_at", a.url}, kv{"redirects", orEmpty(a.hops)})
		if a.err != "" {
			row = append(row, kv{"error", a.err})
		}
		out = append(out, kv{u, row})
	}
	return out, nil
}

var (
	titleTag      = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	spaces        = regexp.MustCompile(`\s+`)
	ga4IDs        = regexp.MustCompile(`\b(G-[A-Z0-9]{6,12})\b`)
	uaIDs         = regexp.MustCompile(`\b(UA-\d{4,10}-\d{1,4})\b`)
	gtmIDs        = regexp.MustCompile(`\b(GTM-[A-Z0-9]{4,10})\b`)
	formTags      = regexp.MustCompile(`(?i)<form\b[^>]*>`)
	formActions   = regexp.MustCompile(`(?i)<form\b[^>]*action=["']([^"']+)["']`)
	mailtos       = regexp.MustCompile(`(?i)mailto:([^"'?\s>]+)`)
	sitemapLines  = regexp.MustCompile(`(?im)^\s*sitemap:\s*(\S+)`)
	disallowAll   = regexp.MustCompile(`(?im)^\s*disallow:\s*/\s*$`)
	locTags       = regexp.MustCompile(`(?is)<loc>\s*(.*?)\s*</loc>`)
	locOpen       = regexp.MustCompile(`(?i)<loc>`)
	sitemapIsList = regexp.MustCompile(`<sitemapindex`)
)

// homePage reads the home page: its title, platform, Analytics and Tag
// Manager IDs, verifications, forms and where they post.
func (p *prober) homePage(ctx context.Context) (obj, error) {
	a := p.follow(ctx, "https://"+p.domain+"/", "*/*")
	if a.status != http.StatusOK {
		return nil, fmt.Errorf("%s", a.failure())
	}
	html := a.body
	out := o("url", a.url)
	if m := titleTag.FindStringSubmatch(html); m != nil {
		out = append(out, kv{"title", truncate(strings.TrimSpace(spaces.ReplaceAllString(m[1], " ")), 150)})
	}
	if d := meta(html, "description"); d != "" {
		out = append(out, kv{"description", truncate(d, 200)})
	}
	if g := meta(html, "generator"); g != "" {
		out = append(out, kv{"generator", g})
	}
	server := a.headers.Get("Server")
	hints := []string{}
	for _, h := range []struct{ name, marker string }{{"WordPress", "wp-content"}, {"Shopify", "cdn.shopify.com"}, {"Wix", "wixstatic.com"}, {"Squarespace", "squarespace"}, {"Webflow", "webflow"}} {
		if strings.Contains(html, h.marker) {
			hints = append(hints, h.name)
		}
	}
	if strings.Contains(html, "astro-island") || strings.Contains(html, "/_astro/") {
		hints = append(hints, "Astro")
	}
	if strings.Contains(server, "cloudflare") {
		hints = append(hints, "Cloudflare")
	}
	if strings.Contains(strings.ToLower(server), "vercel") {
		hints = append(hints, "Vercel")
	}
	if strings.Contains(strings.ToLower(server), "netlify") {
		hints = append(hints, "Netlify")
	}
	posts := []string{}
	for _, m := range formActions.FindAllStringSubmatch(html, -1) {
		host := m[1]
		if base, err := url.Parse(a.url); err == nil {
			if ref, err := base.Parse(m[1]); err == nil {
				host = ref.Hostname()
			}
		}
		if !slices.Contains(posts, host) && len(posts) < 5 {
			posts = append(posts, host)
		}
	}
	emails := []string{}
	for _, m := range mailtos.FindAllStringSubmatch(html, -1) {
		if e := strings.ToLower(m[1]); !slices.Contains(emails, e) && len(emails) < 5 {
			emails = append(emails, e)
		}
	}
	out = append(out, kv{"platform_hints", hints}, kv{"analytics_ids", matches(ga4IDs, html)}, kv{"universal_analytics_ids", matches(uaIDs, html)},
		kv{"tag_manager_ids", matches(gtmIDs, html)})
	if v := meta(html, "google-site-verification"); v != "" {
		out = append(out, kv{"google_site_verification", v})
	}
	if v := meta(html, "msvalidate.01"); v != "" {
		out = append(out, kv{"bing_verification", v})
	}
	out = append(out, kv{"forms", len(formTags.FindAllString(html, -1))}, kv{"form_posts_to", posts}, kv{"email_addresses", emails})
	if server != "" {
		out = append(out, kv{"server", server})
	}
	return out, nil
}

func matches(re *regexp.Regexp, s string) []string {
	out := []string{}
	for _, m := range re.FindAllStringSubmatch(s, -1) {
		if !slices.Contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}

// meta is a <meta name=…>'s content, either way round.
func meta(html, name string) string {
	q := regexp.QuoteMeta(name)
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)<meta[^>]+name=["']` + q + `["'][^>]*content=["']([^"']*)["']`),
		regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']*)["'][^>]*name=["']` + q + `["']`),
	} {
		if m := re.FindStringSubmatch(html); m != nil {
			return m[1]
		}
	}
	return ""
}

// robotsTxt is what robots.txt says: its sitemaps, and whether it shuts
// everything out. Read once (the sitemap needs it too).
func (p *prober) robotsTxt(ctx context.Context) (obj, error) {
	p.robotsOnce.Do(func() {
		a := p.follow(ctx, "https://"+p.domain+"/robots.txt", "*/*")
		if a.status != http.StatusOK {
			p.robotsErr = fmt.Errorf("%s", a.failure())
			return
		}
		sitemaps := []string{}
		for _, m := range sitemapLines.FindAllStringSubmatch(a.body, -1) {
			if !slices.Contains(sitemaps, m[1]) {
				sitemaps = append(sitemaps, m[1])
			}
		}
		p.robots = o("sitemaps", sitemaps, "disallows_everything", disallowAll.MatchString(a.body))
	})
	return p.robots, p.robotsErr
}

// sitemap is the first sitemap found (robots.txt's, then the usual
// addresses), kept as a file.
func (p *prober) sitemap(ctx context.Context) (obj, error) {
	var candidates []string
	if r, err := p.robotsTxt(ctx); err == nil {
		for _, kv := range r {
			if list, ok := kv.v.([]string); ok && kv.k == "sitemaps" {
				candidates = append(candidates, list...)
			}
		}
	}
	for _, u := range []string{"https://" + p.domain + "/sitemap.xml", "https://" + p.domain + "/sitemap_index.xml"} {
		if !slices.Contains(candidates, u) {
			candidates = append(candidates, u)
		}
	}
	for _, u := range candidates {
		a := p.follow(ctx, u, "*/*")
		if a.status != http.StatusOK || !strings.Contains(a.body, "<") {
			continue
		}
		p.files = append(p.files, probeFile{name: "sitemap.xml", contentType: "application/xml", body: []byte(a.body)})
		kind := "urlset"
		if sitemapIsList.MatchString(a.body) {
			kind = "index"
		}
		first := []string{}
		for _, m := range locTags.FindAllStringSubmatch(a.body, 10) {
			first = append(first, m[1])
		}
		return o("url", a.url, "kind", kind, "entries", len(locOpen.FindAllString(a.body, -1)), "first_entries", first, "kept_as_file", "sitemap.xml"), nil
	}
	return nil, fmt.Errorf("none found at %s", strings.Join(candidates, ", "))
}

func orEmpty(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}
