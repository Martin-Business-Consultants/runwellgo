package core

import "context"

// A record's answer is put together by the areas that keep things on it: a
// client's show answer has its contacts (clients), engagements
// (engagements), open commitments (commitments), notes (notes) and
// documents (documents), each added by its own area in an init:
//
//	addSection("Client", "engagements", func(a *App, ctx context.Context, id int64) (any, error) { … })
//
// so no area reaches into another's tables to answer. Sections come in the
// order they were added; a key added twice replaces the first.

type section struct {
	key string
	fn  func(a *App, ctx context.Context, id int64) (any, error)
}

var sections = map[string][]section{}

// addSection adds a key to a record type's show answer.
func addSection(typ, key string, fn func(a *App, ctx context.Context, id int64) (any, error)) {
	for i, s := range sections[typ] {
		if s.key == key {
			sections[typ][i].fn = fn
			return
		}
	}
	sections[typ] = append(sections[typ], section{key, fn})
}

// withSections adds every section of typ for id to o, in order.
func (a *App) withSections(ctx context.Context, typ string, id int64, o Obj) (Obj, error) {
	for _, s := range sections[typ] {
		v, err := s.fn(a, ctx, id)
		if err != nil {
			return nil, err
		}
		o = o.Set(s.key, v)
	}
	return o, nil
}

// Deleting a record takes what hangs off it with it (the Rails dependent:
// :destroy): each area that keeps rows pointing at a record type adds what
// to remove, in an init, run inside the delete's write.
var cascades = map[string][]func(t *Tx, id int64) error{}

// onDelete adds what deleting a record of typ removes.
func onDelete(typ string, fn func(t *Tx, id int64) error) {
	cascades[typ] = append(cascades[typ], fn)
}

// cascade removes what hangs off a record being deleted: its events, its
// custom values, its search entry, and whatever areas added.
func (t *Tx) cascade(typ string, id int64) error {
	for _, fn := range cascades[typ] {
		if err := fn(t, id); err != nil {
			return err
		}
	}
	for _, q := range []string{
		`DELETE FROM custom_values WHERE subject_type = ? AND subject_id = ?`,
		`DELETE FROM events WHERE subject_type = ? AND subject_id = ?`,
	} {
		if _, err := t.Exec(q, typ, id); err != nil {
			return err
		}
	}
	t.Reindex(typ, id) // gone from the index once committed
	return nil
}
