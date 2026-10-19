package core

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Engagements, many at once, from a table's selection or an agent: each
// goes through the same verb and rules as doing it alone, and one the
// action doesn't apply to is skipped and named rather than failing the
// rest (Rails' BulkAction).

func init() { routeSets = append(routeSets, (*App).engagementBulkRoutes) }

func (a *App) engagementBulkRoutes(mux *http.ServeMux) {
	a.handle(mux, "DELETE /api/bulk/engagements", a.bulkDeleteEngagements, need("delete_records"), tool("bulk_delete_engagements", "Delete several engagements at once",
		note("Only engagements never sent to the client are deleted; sent ones are skipped (close them instead)."),
		bodyParam("ids", "array", "The engagements' ids.", true)))
	a.handle(mux, "POST /api/bulk/engagements/closure", a.bulkCloseEngagements, need("close_engagements"), tool("bulk_close_engagements", "Mark several engagements complete (or cancelled) at once",
		bodyParam("ids", "array", "The engagements' ids.", true),
		bodyParam("outcome", "string", "completed (the default) or cancelled.", false),
		bodyParam("reason", "string", "Why, for them all.", false)))
}

// engagementBulkMax is the most one bulk action takes.
const engagementBulkMax = 500

// bulkEngagementIDs are the selection: ids as a list or "1,2,3", positive,
// unique, at most 500.
func bulkEngagementIDs(v any) []int64 {
	var raw []string
	switch x := v.(type) {
	case []any:
		for _, it := range x {
			raw = append(raw, strings.Split(fmt.Sprint(it), ",")...)
		}
	case string:
		raw = strings.Split(x, ",")
	case float64:
		raw = []string{strconv.FormatInt(int64(x), 10)}
	}
	var ids []int64
	for _, s := range raw {
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err == nil && n > 0 && !slices.Contains(ids, n) {
			ids = append(ids, n)
		}
	}
	return ids[:min(len(ids), engagementBulkMax)]
}

// bulkEngagements runs fn on each selected engagement in its own write: nil
// when it worked, or the reason it didn't. done is the message for what
// worked, with %d for how many.
func (a *App) bulkEngagements(c *Ctx, body map[string]any, done string, fn func(t *Tx, e *Engagement) error) error {
	ctx := c.R.Context()
	ids := bulkEngagementIDs(body["ids"])
	var list []*Engagement
	if len(ids) > 0 {
		var err error
		if list, err = a.loadEngagements(ctx, a.store, `e.id IN (`+store.Placeholders(len(ids))+`)`, idArgs(ids)...); err != nil {
			return err
		}
	}
	if len(list) == 0 {
		return refused("Select at least one first.")
	}
	count := 0
	var skipped []string
	for _, e := range list {
		err := a.write(ctx, c.Actor, func(t *Tx) error { return fn(t, e) })
		if err == nil {
			count++
			continue
		}
		reason := err.Error()
		if ae, ok := errors.AsType[*apierr.Error](err); ok {
			reason = ae.Message
			if ae.Errors != nil {
				reason = sentence(ae)
			}
		}
		skipped = append(skipped, "“"+e.Ref+"” ("+strOr(reason, "it couldn’t be changed")+")")
	}
	var parts []string
	if count > 0 {
		parts = append(parts, fmt.Sprintf(done, count))
	}
	if len(skipped) > 0 {
		more := ""
		if len(skipped) > 5 {
			more = fmt.Sprintf(" and %d more", len(skipped)-5)
		}
		parts = append(parts, fmt.Sprintf("Skipped %d: %s%s.", len(skipped), strings.Join(skipped[:min(5, len(skipped))], "; "), more))
	}
	msg := strings.Join(parts, ". ")
	if !strings.HasSuffix(msg, ".") {
		msg += "."
	}
	if skipped == nil {
		skipped = []string{}
	}
	c.Summary = msg
	if count == 0 {
		return c.JSON(422, O("error", "refused", "message", msg, "summary", msg, "count", count, "skipped", skipped))
	}
	return c.JSON(200, O("summary", msg, "count", count, "skipped", skipped))
}

func (a *App) bulkDeleteEngagements(c *Ctx) error {
	var body map[string]any
	if err := c.OptionalBody(&body); err != nil {
		return err
	}
	s, err := a.settings(c.R.Context())
	if err != nil {
		return err
	}
	noun := strings.ToLower(s.Term("engagement", 2))
	return a.bulkEngagements(c, body, "Deleted %d "+noun, func(t *Tx, e *Engagement) error {
		if !e.Deletable() {
			return refused("it was sent to the client; close it instead")
		}
		return t.destroyEngagement(e.ID)
	})
}

func (a *App) bulkCloseEngagements(c *Ctx) error {
	var body map[string]any
	if err := c.OptionalBody(&body); err != nil {
		return err
	}
	outcome := str(body["outcome"])
	if !slices.Contains(EngagementCloseOutcomes, outcome) {
		outcome = "completed"
	}
	reason := strings.TrimSpace(str(body["reason"]))
	s, err := a.settings(c.R.Context())
	if err != nil {
		return err
	}
	noun := strings.ToLower(s.Term("engagement", 2))
	return a.bulkEngagements(c, body, humanize(outcome)+" %d "+noun, func(t *Tx, e *Engagement) error {
		if e.Closed() {
			return refused("it’s already closed")
		}
		_, err := t.closeEngagement(e, outcome, reason)
		return err
	})
}
