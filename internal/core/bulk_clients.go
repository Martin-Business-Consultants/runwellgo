package core

import (
	"net/http"
	"slices"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// Clients, many at once (Bulk::ClientsController): their status, or delete
// (only those with no engagements). Each goes through the same rules as
// doing it alone; one it doesn't apply to is skipped and named.

func init() { routeSets = append(routeSets, (*App).bulkClientRoutes) }

func (a *App) bulkClientRoutes(mux *http.ServeMux) {
	a.handle(mux, "PATCH /api/bulk/clients", a.bulkUpdateClients, tool("bulk_update_clients", "Change several clients' status at once",
		bodyParam("ids", "array", "The clients' ids.", true),
		body("client", `{status: active|dormant|former}`, true)))
	a.handle(mux, "DELETE /api/bulk/clients", a.bulkDeleteClients, need("delete_records"), tool("bulk_delete_clients", "Delete several clients at once",
		note("Only clients with no engagements are deleted; the rest are skipped and named."),
		bodyParam("ids", "array", "The clients' ids.", true)))
}

// selectedClients are the clients picked, in id order.
func (a *App) selectedClients(c *Ctx, body map[string]any) ([]*Client, error) {
	ids := workBulkIDs(c, body)
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := a.store.QueryContext(c.R.Context(), `SELECT `+clientColumns+` FROM clients WHERE id IN (`+store.Placeholders(len(ids))+`) ORDER BY id`, idArgs(ids)...)
	if err != nil {
		return nil, err
	}
	return store.ScanAll(rows, func(r *sqlRows) (*Client, error) { return scanClient(r) })
}

func (a *App) clientsNoun(c *Ctx) string {
	s, err := a.settings(c.R.Context())
	if err != nil {
		return "clients"
	}
	return strings.ToLower(s.Term("client", 2))
}

func (a *App) bulkUpdateClients(c *Ctx) error {
	var body map[string]any
	if err := c.Body(&body); err != nil {
		return err
	}
	fields, _ := body["client"].(map[string]any)
	status := str(fields["status"])
	if !slices.Contains(ClientStatuses, status) {
		inv := apierr.Invalid{}
		inv.Add("status", "is not included in the list")
		return inv.Err()
	}
	clients, err := a.selectedClients(c, body)
	if err != nil {
		return err
	}
	if len(clients) == 0 {
		return &apierr.Error{Status: 422, Code: "refused", Message: "Select at least one first."}
	}
	count, skipped := 0, []workBulkSkip{}
	for _, cl := range clients {
		err := a.write(c.R.Context(), c.Actor, func(t *Tx) error {
			if cl.Status == status {
				return nil
			}
			if err := t.update("clients", cl.ID, map[string]any{"status": status}); err != nil {
				return err
			}
			return t.Event("Client", cl.ID, "client.status_changed", map[string]any{"from": cl.Status, "to": status})
		})
		if err != nil {
			skipped = append(skipped, workBulkSkip{cl.Name, workBulkReason(err)})
			continue
		}
		count++
	}
	return workBulkAnswer(c, "Marked %d "+a.clientsNoun(c)+" "+strings.ToLower(humanize(status)), count, skipped)
}

func (a *App) bulkDeleteClients(c *Ctx) error {
	var body map[string]any
	if err := c.OptionalBody(&body); err != nil {
		return err
	}
	clients, err := a.selectedClients(c, body)
	if err != nil {
		return err
	}
	if len(clients) == 0 {
		return &apierr.Error{Status: 422, Code: "refused", Message: "Select at least one first."}
	}
	count, skipped := 0, []workBulkSkip{}
	for _, cl := range clients {
		if err := a.write(c.R.Context(), c.Actor, func(t *Tx) error { return t.deleteClient(cl) }); err != nil {
			skipped = append(skipped, workBulkSkip{cl.Name, "it has engagements"})
			continue
		}
		count++
	}
	return workBulkAnswer(c, "Deleted %d "+a.clientsNoun(c), count, skipped)
}
