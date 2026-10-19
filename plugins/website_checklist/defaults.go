package websitechecklist

// The standard checklist (the Rails Site::Defaults): what to gather and
// record to take a site live, in the order to do it. Access comes first,
// since every later step needs it. How guides the person through a step and
// Example shows what to record; both are read from here, not stored. Keys
// are fixed once used; order, group, title and detail can change, and
// applyStandard brings every site up to date.

// groups are the standard checklist's groups, in order; custom steps
// follow under "Custom".
var groups = []string{"Access", "Forms and email", "Launch", "After launch"}

// allGroups are the groups a site's page lists, in order (Item::GROUPS).
var allGroups = append(append([]string{}, groups...), "Custom")

type defaultItem struct {
	Key, Group, Title, Detail, How, Example string
}

var defaults = []defaultItem{
	{Key: "access:registrar", Group: "Access", Title: "Domain registrar",
		Detail:  "Where the domain is registered, whose account it's in, and when it renews. Nothing about the domain can change without it.",
		How:     "Ask the client where they bought the domain (GoDaddy, Namecheap, Squarespace, Cloudflare…). A whois lookup names the registrar if they don't know. Ask to be added as a user or delegate rather than for their password, and check auto-renew is on.",
		Example: "Namecheap, the client's account; we're a delegate; renews 3 Mar 2027, auto-renew on"},
	{Key: "access:dns", Group: "Access", Title: "DNS",
		Detail:  "Where the domain's DNS is hosted, and that we can edit its records.",
		How:     "Often the registrar, but the nameservers say for sure (whois, or dig NS example.com). Before changing anything, export or screenshot every record as it is now: the email ones (MX, SPF, DKIM) above all.",
		Example: "Cloudflare (ns1/ns2.cloudflare.com), we're an admin; records exported 12 Oct"},
	{Key: "access:email", Group: "Access", Title: "The domain's email",
		Detail:  "Who hosts email on the domain, so the switch doesn't break it.",
		How:     "Read the MX records: Google Workspace, Microsoft 365, the web host's own mail… Whatever it is, its records have to survive the switch untouched.",
		Example: "Google Workspace (MX smtp.google.com), SPF and DKIM in place"},
	{Key: "access:current_site", Group: "Access", Title: "The current website",
		Detail:  "Where the site lives now, and a login, to copy its content and list its addresses for redirects.",
		How:     "Get the host and an admin login. Before it goes away, save a list of its pages (its sitemap.xml, or a crawl) and anything that only lives there: forms, downloads, tracking codes.",
		Example: "WordPress on Bluehost, admin login in the vault; 42 pages listed"},
	{Key: "access:search_console", Group: "Access", Title: "Google Search Console",
		Detail:  "The site's property, with us as an owner.",
		How:     "Search Console › Settings › Users and permissions › Add user: our address, as Owner. No property yet? Add a Domain property and verify it with the TXT record it gives, in DNS.",
		Example: "sc-domain:example.com, we're Owner"},
	{Key: "access:analytics", Group: "Access", Title: "Google Analytics",
		Detail:  "The GA4 property and its measurement ID, with us as an editor.",
		How:     "Analytics › Admin › Property access management › Add: our address, as Editor. The measurement ID (G-…) is under Admin › Data streams. None yet? Create the property and a web stream.",
		Example: "Property 412345678, G-ABC123XYZ, we're Editor"},
	{Key: "access:business_profile", Group: "Access", Title: "Google Business Profile",
		Detail:  "The business's listing, with us as a manager, so its website link can be updated at launch.",
		How:     "business.google.com › the profile › Business Profile settings › Managers › Add: our address, as Manager. Not every site has one; mark it not applicable if so.",
		Example: "Acme Plumbing, Grand Rapids; we're Manager"},
	{Key: "access:bing", Group: "Access", Title: "Bing Webmaster Tools",
		Detail:  "Bing also feeds DuckDuckGo, Yahoo and ChatGPT search.",
		How:     "bing.com/webmasters › Import from Google Search Console once that's set up: it brings the site and its verification across.",
		Example: "Imported from Search Console 12 Oct"},

	{Key: "forms:any:notification", Group: "Forms and email", Title: "Who gets form notifications",
		Detail:  "Everyone who gets an email when someone sends a form (to and CCs), as the client asked.",
		How:     "Ask the client who should hear about each form; a shared inbox outlives a person leaving.",
		Example: "office@example.com; cc owner@example.com"},
	{Key: "sender:from_name", Group: "Forms and email", Title: "Name form emails come from",
		Detail:  "The name people see in their inbox.",
		Example: "Acme Plumbing"},
	{Key: "sender:from_email", Group: "Forms and email", Title: "Address form emails come from",
		Detail:  "An address on the site's own domain gets delivered; a Gmail address sent by the site lands in spam.",
		Example: "hello@example.com"},
	{Key: "sender:dns", Group: "Forms and email", Title: "SPF, DKIM and DMARC for the sending domain",
		Detail:  "Without them, form emails land in spam.",
		How:     "Add the records the sending service gives you to DNS, beside the domain's own email ones (one SPF record, merged). Check them with a tool such as mail-tester.com.",
		Example: "SPF include added, DKIM key sel1, DMARC p=none; mail-tester 10/10"},
	{Key: "forms:any:notification_wording", Group: "Forms and email", Title: "Notification wording",
		Detail:  "The subject and what the notification says.",
		Example: "“New enquiry from the website: {name}”"},
	{Key: "forms:any:confirmation", Group: "Forms and email", Title: "Confirmation email, approved by the client",
		Detail:  "What the person who wrote in gets back.",
		How:     "Send the client the wording and record when they approved it.",
		Example: "Approved by Jane, 10 Oct"},
	{Key: webhooksKey, Group: "Forms and email", Title: "Webhooks",
		Detail: "Where submissions go besides email (a CRM, Zapier, a spreadsheet): each one's address and the keys and values it's set up with.",
		How:    "Add each webhook with its address, and the keys and values it needs (headers, field names, an account or list ID). Keep secrets in the vault, not here: name where they are."},

	{Key: "launch:redirects", Group: "Launch", Title: "Old addresses redirected",
		Detail:  "Every page of the current site goes to its new address with a 301, so search rankings and old links carry over.",
		How:     "From the list of the current site's pages, map each to its new page (or the closest one) and set up 301s. Test a handful after the switch.",
		Example: "42 redirects, checked 14 Oct"},
	{Key: "launch:dns_records", Group: "Launch", Title: "DNS switched to the new site",
		Detail:  "The records changed for the new site, with their old and new values, so it can be undone.",
		How:     "A day before, lower the TTL on the records you'll change. On the day, change only the web records (A, AAAA, CNAME for www); leave MX, SPF and DKIM alone.",
		Example: "A @ 1.2.3.4 → 76.76.21.21; CNAME www → cname.host.com; TTL back to 3600"},
	{Key: "launch:domain", Group: "Launch", Title: "Domain and HTTPS",
		Detail:  "The live address, served over HTTPS, with www and the bare domain going to one of them.",
		Example: "https://example.com; www redirects; certificate valid"},
	{Key: "launch:email_check", Group: "Launch", Title: "Email still arrives",
		Detail:  "Mail to the domain's own addresses still arrives after the switch.",
		How:     "Send a message to one of the client's addresses from outside, and ask them to confirm it arrived.",
		Example: "Sent to office@ 14 Oct, Jane confirmed"},
	{Key: "forms:any:test", Group: "Launch", Title: "Test submission received",
		Detail:  "A test sent through the live form arrived: the notification at every address, the confirmation, and each webhook.",
		Example: "Sent 14 Oct 10:20; office@ and owner@ got it; CRM shows it"},
	{Key: "launch:signoff", Group: "Launch", Title: "Client signed off",
		Detail:  "Who approved the site, and when.",
		Example: "Jane Smith, by email, 14 Oct"},

	{Key: "search:sitemap_url", Group: "After launch", Title: "Sitemap URL",
		Detail:  "The address of the site's sitemap.xml.",
		Example: "https://example.com/sitemap.xml"},
	{Key: "search:google", Group: "After launch", Title: "Sitemap submitted to Google",
		Detail:  "In Search Console, with the date it was submitted.",
		How:     "Search Console › Sitemaps › add the sitemap URL. Then URL inspection › Request indexing on the home page.",
		Example: "Submitted 14 Oct, status Success"},
	{Key: "search:bing", Group: "After launch", Title: "Sitemap submitted to Bing",
		Detail:  "In Bing Webmaster Tools, with the date it was submitted.",
		Example: "Submitted 14 Oct"},
	{Key: "search:analytics", Group: "After launch", Title: "Analytics receiving data",
		Detail:  "The measurement ID is on every page, and visits show up.",
		How:     "Open the site, then Analytics › Reports › Realtime: your visit should be there within a minute.",
		Example: "Realtime shows visits, 14 Oct"},
	{Key: "search:business_profile", Group: "After launch", Title: "Website link on the Business Profile",
		Detail:  "The Google Business Profile points at the new site's address.",
		Example: "Updated to https://example.com 14 Oct"},
}

// webhooksKey is the Webhooks step, which records webhooks rather than a
// line of text.
const webhooksKey = "webhooks:integrations"

// byKey is the standard steps by key.
var byKey = func() map[string]defaultItem {
	m := map[string]defaultItem{}
	for _, d := range defaults {
		m[d.Key] = d
	}
	return m
}()
