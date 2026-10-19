package core

import (
	"net/http"
	"slices"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Commitments, many at once: resolve them, or delete open ones (resolved
// ones stay as history).

func init() { routeSets = append(routeSets, (*App).bulkCommitmentRoutes) }

func (a *App) bulkCommitmentRoutes(mux *http.ServeMux) {
	a.handle(mux, "DELETE /api/bulk/commitments", a.bulkDeleteCommitments, need("delete_records"), tool("bulk_delete_commitments", "Delete several open commitments at once",
		bodyParam("ids", "array", "The commitments' ids.", true)))
	a.handle(mux, "POST /api/bulk/commitments/resolution", a.bulkResolveCommitments, tool("bulk_resolve_commitments", "Resolve several commitments at once",
		bodyParam("ids", "array", "The commitments' ids.", true),
		bodyParam("resolution", "string", "done (the default), missed or dropped.", false)))
}

// selectedCommitments are the selected commitments that exist.
func (a *App) selectedCommitments(c *Ctx, body map[string]any) ([]*Commitment, error) {
	ids := workBulkIDs(c, body)
	if len(ids) == 0 {
		return nil, &apierr.Error{Status: 422, Code: "refused", Message: "Select at least one first."}
	}
	list, err := a.loadCommitments(c.R.Context(), a.store, `cm.id IN (`+store.Placeholders(len(ids))+`) ORDER BY cm.id`, idArgs(ids)...)
	if err == nil && len(list) == 0 {
		err = &apierr.Error{Status: 422, Code: "refused", Message: "Select at least one first."}
	}
	return list, err
}

func bulkCommitmentLabel(cm *Commitment) string { return truncate(cm.Description, 40) }

func (a *App) bulkDeleteCommitments(c *Ctx) error {
	ctx := c.R.Context()
	var body map[string]any
	if err := c.OptionalBody(&body); err != nil {
		return err
	}
	list, err := a.selectedCommitments(c, body)
	if err != nil {
		return err
	}
	count, skipped := 0, []workBulkSkip{}
	for _, cm := range list {
		if !cm.Open() {
			skipped = append(skipped, workBulkSkip{bulkCommitmentLabel(cm), "it’s resolved, so it stays as history"})
			continue
		}
		if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.deleteCommitment(cm.ID) }); err != nil {
			skipped = append(skipped, workBulkSkip{bulkCommitmentLabel(cm), workBulkReason(err)})
			continue
		}
		count++
	}
	return workBulkAnswer(c, "Deleted %d commitments", count, skipped)
}

func (a *App) bulkResolveCommitments(c *Ctx) error {
	ctx := c.R.Context()
	var body map[string]any
	if err := c.Body(&body); err != nil {
		return err
	}
	resolution := str(body["resolution"])
	if !slices.Contains(CommitmentResolutions, resolution) {
		resolution = "done"
	}
	list, err := a.selectedCommitments(c, body)
	if err != nil {
		return err
	}
	count, skipped := 0, []workBulkSkip{}
	for _, cm := range list {
		if !cm.Open() {
			skipped = append(skipped, workBulkSkip{bulkCommitmentLabel(cm), "it’s already resolved"})
			continue
		}
		if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.resolveCommitment(cm, resolution, "") }); err != nil {
			skipped = append(skipped, workBulkSkip{bulkCommitmentLabel(cm), workBulkReason(err)})
			continue
		}
		count++
	}
	return workBulkAnswer(c, "Marked %d commitments "+resolution, count, skipped)
}
