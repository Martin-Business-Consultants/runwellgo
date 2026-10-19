package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
	"github.com/Martin-Business-Consultants/runwellgo/internal/webhook"
)

// Settings › Webhooks: addresses that hear of every event (or the kinds
// they name) as signed JSON, so other tools (Zapier, n8n, a data warehouse,
// your own scripts) can follow along without a plugin. Sign-in and security
// events go only to endpoints that ask for them. A failed delivery is tried
// again with growing waits; twenty failures in a row switch an endpoint
// off. Data leaves the install from here, so adding, changing and removing
// endpoints is for an owner in the browser; agents may only list them
// (without secrets).

func init() {
	routeSets = append(routeSets, (*App).webhookRoutes)
	onEvent(func(a *App, ctx context.Context, ev Event) { a.dispatchWebhooks(ctx, ev) })
	every("webhook retries", time.Minute, func(a *App, ctx context.Context) error { return a.retryWebhooks(ctx) })
	every("webhook deliveries expiry", 24*time.Hour, func(a *App, ctx context.Context) error {
		_, err := a.store.ExecContext(ctx, `DELETE FROM webhook_deliveries WHERE created_at < ?`, store.TimeOf(time.Now().Add(-webhook.KeepFor)).DB())
		return err
	})
}

func (a *App) webhookRoutes(mux *http.ServeMux) {
	ms := need("manage_settings")
	a.handle(mux, "GET /api/settings/webhooks", a.listWebhooks, ms, tool("list_webhooks", "List webhooks",
		note("The addresses events are sent to, which kinds, and how delivery is going. Secrets aren't included.")))
	a.handle(mux, "POST /api/settings/webhooks", a.createWebhook, ms)
	a.handle(mux, "PATCH /api/settings/webhooks/{id}", a.updateWebhook, ms)
	a.handle(mux, "DELETE /api/settings/webhooks/{id}", a.destroyWebhook, ms)
}

type webhookEndpoint struct {
	ID              int64
	URL             string
	Description     string
	Secret          string // as stored
	Kinds           []string
	IncludeSecurity bool
	Active          bool
	Failures        int
	LastDeliveredAt store.Time
}

const webhookColumns = `id, url, COALESCE(description, ''), secret, kinds, include_security, active, failures_in_a_row, last_delivered_at`

func scanWebhook(r interface{ Scan(...any) error }) (*webhookEndpoint, error) {
	var e webhookEndpoint
	var kinds store.JSON
	if err := r.Scan(&e.ID, &e.URL, &e.Description, &e.Secret, &kinds, &e.IncludeSecurity, &e.Active, &e.Failures, &e.LastDeliveredAt); err != nil {
		return nil, err
	}
	for _, k := range kinds.List() {
		e.Kinds = append(e.Kinds, str(k))
	}
	return &e, nil
}

func (a *App) webhookEndpoints(ctx context.Context, db store.Querier, where string, args ...any) ([]*webhookEndpoint, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+webhookColumns+` FROM webhook_endpoints WHERE `+where+` ORDER BY created_at, id`, args...)
	if err != nil {
		return nil, err
	}
	return store.ScanAll(rows, func(r *sqlRows) (*webhookEndpoint, error) { return scanWebhook(r) })
}

// ---- delivering ----------------------------------------------------------------

// dispatchWebhooks queues a delivery of ev to every active endpoint that
// wants it, and makes the first try.
func (a *App) dispatchWebhooks(ctx context.Context, ev Event) {
	endpoints, err := a.webhookEndpoints(ctx, a.store, `active = 1`)
	if errors.Is(err, context.Canceled) {
		return // the app is closing
	}
	if err != nil {
		a.log.Warn("webhooks", "err", err)
		return
	}
	for _, e := range endpoints {
		if !webhook.Wants(e.Kinds, e.IncludeSecurity, ev.Kind, ev.SubjectType, SecuritySubjects) {
			continue
		}
		now := store.Now().DB()
		res, err := a.store.ExecContext(ctx, `INSERT INTO webhook_deliveries (webhook_endpoint_id, event_id, kind, attempts, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)`,
			e.ID, ev.ID, ev.Kind, now, now)
		if err != nil {
			a.log.Warn("webhooks", "err", err)
			continue
		}
		id, _ := res.LastInsertId()
		a.later("webhook", func(ctx context.Context) { a.deliverWebhook(ctx, id) })
	}
}

// ping queues a test delivery to an endpoint and tries it in the background.
func (a *App) pingWebhook(ctx context.Context, endpointID int64) error {
	now := store.Now().DB()
	res, err := a.store.ExecContext(ctx, `INSERT INTO webhook_deliveries (webhook_endpoint_id, kind, attempts, created_at, updated_at) VALUES (?, 'ping', 0, ?, ?)`, endpointID, now, now)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	a.later("webhook", func(ctx context.Context) { a.deliverWebhook(ctx, id) })
	return nil
}

// deliverWebhook makes one try of a delivery and records it; a failure
// waits for retryWebhooks.
func (a *App) deliverWebhook(ctx context.Context, id int64) {
	if err := a.tryWebhook(ctx, id); err != nil {
		a.log.Warn("webhook delivery", "delivery", id, "err", err)
	}
}

func (a *App) tryWebhook(ctx context.Context, id int64) error {
	var endpointID int64
	var eventID store.NullInt
	var kind string
	var attempts int
	var created store.Time
	if err := a.store.QueryRowContext(ctx, `SELECT webhook_endpoint_id, event_id, kind, attempts, created_at FROM webhook_deliveries WHERE id = ?`, id).
		Scan(&endpointID, &eventID, &kind, &attempts, &created); err != nil {
		return err
	}
	e, err := scanWebhook(a.store.QueryRowContext(ctx, `SELECT `+webhookColumns+` FROM webhook_endpoints WHERE id = ?`, endpointID))
	if err != nil || !e.Active {
		return err
	}
	body, err := a.webhookBody(ctx, id, eventID, kind)
	if err != nil {
		return err
	}
	var result webhook.Result
	if secretText, ok := a.openSecret(e.Secret); ok {
		result = webhook.Post(ctx, e.URL, secretText, kind, id, body, "Runwell/"+a.cfg.Version+" (+"+a.Host()+")")
	} else {
		result = webhook.Result{Error: "the signing secret can't be read here: remove this webhook and add it again"}
	}
	return a.write(ctx, nil, func(t *Tx) error {
		var status, delivered, errText any
		if result.Status != 0 {
			status = result.Status
		}
		if result.OK() {
			delivered = store.Now().DB()
		} else {
			errText = result.Error
		}
		if _, err := t.Exec(`UPDATE webhook_deliveries SET attempts = attempts + 1, status_code = ?, error = ?, duration_ms = ?, delivered_at = ?, updated_at = ? WHERE id = ?`,
			status, errText, result.Duration.Milliseconds(), delivered, store.Now().DB(), id); err != nil {
			return err
		}
		if result.OK() {
			_, err := t.Exec(`UPDATE webhook_endpoints SET failures_in_a_row = 0, last_delivered_at = ? WHERE id = ?`, store.Now().DB(), e.ID)
			return err
		}
		if _, err := t.Exec(`UPDATE webhook_endpoints SET failures_in_a_row = failures_in_a_row + 1, updated_at = ? WHERE id = ?`, store.Now().DB(), e.ID); err != nil {
			return err
		}
		if e.Failures+1 >= webhook.MaxFailures && e.Active {
			if err := t.update("webhook_endpoints", e.ID, map[string]any{"active": false}); err != nil {
				return err
			}
			s, err := a.settings(ctx)
			if err != nil {
				return err
			}
			return t.Event("Setting", s.ID, "settings.webhook", map[string]any{"url": e.URL, "active": false, "reason": "20 failures in a row"})
		}
		return nil
	})
}

// retryWebhooks tries again the failed deliveries whose wait is over.
func (a *App) retryWebhooks(ctx context.Context) error {
	rows, err := a.store.QueryContext(ctx, `SELECT d.id, d.attempts, d.updated_at FROM webhook_deliveries d JOIN webhook_endpoints e ON e.id = d.webhook_endpoint_id
		WHERE e.active = 1 AND d.delivered_at IS NULL AND d.attempts > 0 AND d.attempts < ? AND d.created_at > ?`, webhook.Attempts, store.TimeOf(time.Now().Add(-48*time.Hour)).DB())
	if err != nil {
		return err
	}
	type due struct {
		id       int64
		attempts int
		at       store.Time
	}
	list, err := store.ScanAll(rows, func(r *sqlRows) (due, error) {
		var d due
		return d, r.Scan(&d.id, &d.attempts, &d.at)
	})
	if err != nil {
		return err
	}
	for _, d := range list {
		if time.Since(d.at.Time) >= webhook.Backoff(d.attempts) {
			a.deliverWebhook(ctx, d.id)
		}
	}
	return nil
}

// webhookBody is what a delivery sends: the event, who did it, what it
// happened to (with its address), and its payload; or a ping.
func (a *App) webhookBody(ctx context.Context, deliveryID int64, eventID store.NullInt, kind string) ([]byte, error) {
	host := a.Host()
	if !eventID.Valid {
		return json.Marshal(O("id", "ping-"+strconv.FormatInt(deliveryID, 10), "kind", "ping", "occurred_at", time.Now().UTC().Format(time.RFC3339),
			"install", host, "message", "A test from Settings › Webhooks."))
	}
	var ev Event
	var payload store.JSON
	var actorUser store.NullInt
	var actor, userName, userEmail string
	err := a.store.QueryRowContext(ctx, `SELECT e.id, e.subject_type, e.subject_id, e.kind, e.payload, e.actor_user_id, COALESCE(e.actor, ''), e.source, e.occurred_at,
		COALESCE(u.name, ''), COALESCE(u.email_address, '') FROM events e LEFT JOIN users u ON u.id = e.actor_user_id WHERE e.id = ?`, eventID.Int64).
		Scan(&ev.ID, &ev.SubjectType, &ev.SubjectID, &ev.Kind, &payload, &actorUser, &actor, &ev.Source, &ev.OccurredAt, &userName, &userEmail)
	if err != nil {
		return nil, err
	}
	name := strOr(userName, strOr(userEmail, strOr(actor, "system")))
	var via any
	if ev.Source == "agent" {
		via = nullable(actor)
	}
	names := a.subjectNames(ctx, []subjectRef{{ev.SubjectType, ev.SubjectID}})
	subj := names[subjectRef{ev.SubjectType, ev.SubjectID}]
	var url any
	if !slices.Contains(SecuritySubjects, ev.SubjectType) && subj.path != "" {
		url = a.BaseURL() + subj.path
	}
	var subjName any
	if subj.found {
		subjName = subj.name
	}
	return json.Marshal(O("id", ev.ID, "kind", ev.Kind, "occurred_at", ev.OccurredAt.UTC().Format(time.RFC3339), "install", host, "source", ev.Source,
		"actor", O("name", name, "user_id", actorUser, "via", via),
		"subject", O("type", ev.SubjectType, "id", ev.SubjectID, "name", subjName, "url", url),
		"payload", payload.Or("{}")))
}

// ---- Settings › Webhooks --------------------------------------------------------

func (a *App) listWebhooks(c *Ctx) error {
	ctx := c.R.Context()
	endpoints, err := a.webhookEndpoints(ctx, a.store, `1 = 1`)
	if err != nil {
		return err
	}
	full := !c.Actor.Agent()
	list := []Obj{}
	on := 0
	for _, e := range endpoints {
		if e.Active {
			on++
		}
		var kinds any
		if len(e.Kinds) > 0 {
			kinds = e.Kinds
		}
		o := O("id", e.ID, "url", e.URL, "description", nullable(e.Description), "kinds", kinds, "include_security", e.IncludeSecurity,
			"active", e.Active, "failures_in_a_row", e.Failures, "last_delivered_at", e.LastDeliveredAt)
		if full {
			secretText, _ := a.openSecret(e.Secret)
			deliveries, err := a.recentDeliveries(ctx, e.ID)
			if err != nil {
				return err
			}
			o = o.Set("kinds_text", strings.Join(e.Kinds, ", ")).Set("secret", nullable(secretText)).Set("deliveries", deliveries)
		}
		list = append(list, o)
	}
	c.Summary = "No webhooks. An owner adds them in Settings › Webhooks."
	if len(endpoints) > 0 {
		c.Summary = pluralize(len(endpoints), "webhook") + ", " + strconv.Itoa(on) + " on"
	}
	return c.JSON(200, O("summary", c.Summary, "webhooks", list))
}

func (a *App) recentDeliveries(ctx context.Context, endpointID int64) ([]Obj, error) {
	rows, err := a.store.QueryContext(ctx, `SELECT id, kind, attempts, status_code, COALESCE(error, ''), duration_ms, delivered_at, created_at FROM webhook_deliveries
		WHERE webhook_endpoint_id = ? ORDER BY created_at DESC, id DESC LIMIT 8`, endpointID)
	if err != nil {
		return nil, err
	}
	out, err := store.ScanAll(rows, func(r *sqlRows) (Obj, error) {
		var id int64
		var kind, errText string
		var attempts int
		var status, ms store.NullInt
		var delivered, created store.Time
		if err := r.Scan(&id, &kind, &attempts, &status, &errText, &ms, &delivered, &created); err != nil {
			return nil, err
		}
		ok := status.Valid && status.Int64 >= 200 && status.Int64 < 300
		return O("id", id, "kind", kind, "attempts", attempts, "status_code", status, "ok", ok, "error", nullable(errText),
			"duration_ms", ms, "delivered_at", delivered, "created_at", created), nil
	})
	if out == nil {
		out = []Obj{}
	}
	return out, err
}

var webhookURLRE = regexp.MustCompile(`(?i)^https?://[^\s/$.?#].[^\s]*$`)

// webhookFields reads the form's fields into e, and what's wrong with them.
func webhookFields(e *webhookEndpoint, body map[string]any) apierr.Invalid {
	bad := apierr.Invalid{}
	if has(body, "url") {
		e.URL = strings.TrimSpace(str(body["url"]))
	}
	if has(body, "description") {
		e.Description = strings.TrimSpace(str(body["description"]))
	}
	if has(body, "kinds_text") {
		e.Kinds = webhook.KindsFromText(str(body["kinds_text"]))
	}
	if has(body, "include_security") {
		e.IncludeSecurity = boolOf(body["include_security"])
	}
	if has(body, "active") {
		e.Active = boolOf(body["active"])
	}
	if e.URL == "" {
		bad.Add("url", "can't be blank")
	} else if !webhookURLRE.MatchString(e.URL) {
		bad.Add("url", "should start with https:// (or http://)")
	}
	if len([]rune(e.Description)) > 120 {
		bad.Add("description", "is too long (maximum is 120 characters)")
	}
	return bad
}

func kindsColumn(kinds []string) any {
	if len(kinds) == 0 {
		return nil
	}
	return string(store.MustJSON(kinds))
}

func (a *App) createWebhook(c *Ctx) error {
	if err := refuseAgent(c); err != nil {
		return err
	}
	body, err := c.Wrapped("webhook_endpoint")
	if err != nil {
		return err
	}
	e := &webhookEndpoint{Active: true}
	if err := fullInvalid(webhookFields(e, body)); err != nil {
		return err
	}
	ctx := c.R.Context()
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		now := store.Now().DB()
		e.ID, err = t.Insert(`INSERT INTO webhook_endpoints (url, description, secret, kinds, include_security, active, created_by_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			e.URL, nullable(e.Description), a.secrets.Seal(webhook.NewSecret()), kindsColumn(e.Kinds), e.IncludeSecurity, e.Active, c.Actor.User.Person().ID, now, now)
		if err != nil {
			return err
		}
		return t.Event("Setting", s.ID, "settings.webhook", map[string]any{"url": e.URL, "added": true})
	})
	if err != nil {
		return err
	}
	if err := a.pingWebhook(ctx, e.ID); err != nil {
		return err
	}
	c.Summary = "Webhook added, and a test sent to it. Its secret is under it, to check signatures."
	return c.JSON(201, O("summary", c.Summary, "id", e.ID))
}

func (a *App) loadWebhook(c *Ctx, db store.Querier) (*webhookEndpoint, error) {
	id, err := c.IntParam("id")
	if err != nil {
		return nil, err
	}
	e, err := scanWebhook(db.QueryRowContext(c.R.Context(), `SELECT `+webhookColumns+` FROM webhook_endpoints WHERE id = ?`, id))
	if isNoRows(err) {
		return nil, notFound("webhook")
	}
	return e, err
}

func (a *App) updateWebhook(c *Ctx) error {
	if err := refuseAgent(c); err != nil {
		return err
	}
	var body map[string]any
	if err := c.Body(&body); err != nil {
		return err
	}
	ctx := c.R.Context()
	e, err := a.loadWebhook(c, a.store)
	if err != nil {
		return err
	}
	if boolOf(body["ping"]) {
		if err := a.pingWebhook(ctx, e.ID); err != nil {
			return err
		}
		c.Summary = "Test sent."
		return c.JSON(200, O("summary", c.Summary))
	}
	fields, _ := body["webhook_endpoint"].(map[string]any)
	if fields == nil {
		return apierr.BadRequest("param is missing or the value is empty: webhook_endpoint")
	}
	if err := fullInvalid(webhookFields(e, fields)); err != nil {
		return err
	}
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		set := map[string]any{"url": e.URL, "description": nullable(e.Description), "kinds": kindsColumn(e.Kinds),
			"include_security": e.IncludeSecurity, "active": e.Active}
		if has(fields, "active") && e.Active {
			set["failures_in_a_row"] = 0
		}
		if err := t.update("webhook_endpoints", e.ID, set); err != nil {
			return err
		}
		return t.Event("Setting", s.ID, "settings.webhook", map[string]any{"url": e.URL, "active": e.Active})
	})
	if err != nil {
		return err
	}
	c.Summary = "Webhook saved."
	return c.JSON(200, O("summary", c.Summary))
}

func (a *App) destroyWebhook(c *Ctx) error {
	if err := refuseAgent(c); err != nil {
		return err
	}
	ctx := c.R.Context()
	e, err := a.loadWebhook(c, a.store)
	if err != nil {
		return err
	}
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if _, err := t.Exec(`DELETE FROM webhook_deliveries WHERE webhook_endpoint_id = ?`, e.ID); err != nil {
			return err
		}
		if _, err := t.Exec(`DELETE FROM webhook_endpoints WHERE id = ?`, e.ID); err != nil {
			return err
		}
		return t.Event("Setting", s.ID, "settings.webhook", map[string]any{"url": e.URL, "removed": true})
	})
	if err != nil {
		return err
	}
	c.Summary = "Webhook removed."
	return c.JSON(200, O("summary", c.Summary))
}
