package core

import (
	"context"
	"database/sql"
	"regexp"
	"strconv"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Search is one SQLite FTS5 index over everything searchable, in the Rails
// core's tables (searchable_documents, and searchable_documents_fts whose
// rowid is the document's id), so a Rails database's index works as it is.
// Each searchable type registers how to read its title, text and client
// (searchable, in an init); a write queues what it touched (Tx.Reindex),
// and the index catches up once it commits.

type searchRef struct {
	Type string
	ID   int64
}

// SearchDoc is what the index keeps of a record.
type SearchDoc struct {
	Title, Content string
	ClientID       int64
	CreatedAt      store.Time
}

// searchSource reads one record for the index; ok false when it's gone (or
// shouldn't be found).
type searchSource func(ctx context.Context, db store.Querier, id int64) (doc SearchDoc, ok bool, err error)

var searchSources = map[string]searchSource{}

// SearchTypes are the searchable types, in the order Rails' Searchable::MODELS
// lists them, for reindexing everything.
var SearchTypes = []string{}

// searchable registers a type for the index.
func searchable(typ string, src searchSource) {
	if _, ok := searchSources[typ]; !ok {
		SearchTypes = append(SearchTypes, typ)
	}
	searchSources[typ] = src
}

var tags = regexp.MustCompile(`<[^>]*>`)

// plainText is rich text's words, for the index.
func plainText(html string) string {
	s := tags.ReplaceAllString(html, " ")
	s = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// reindex brings each record's entry up to date: replaced, or removed when
// the record is gone.
func (a *App) reindex(ctx context.Context, refs ...searchRef) {
	for _, ref := range refs {
		if err := a.reindexOne(ctx, ref); err != nil {
			a.log.Warn("search index", "type", ref.Type, "id", ref.ID, "err", err)
		}
	}
}

func (a *App) reindexOne(ctx context.Context, ref searchRef) error {
	src := searchSources[ref.Type]
	if src == nil {
		return nil
	}
	doc, ok, err := src(ctx, a.store, ref.ID)
	if err != nil {
		return err
	}
	return a.store.Tx(ctx, func(tx *sql.Tx) error {
		var docID int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM searchable_documents WHERE searchable_type = ? AND searchable_id = ?`, ref.Type, strconv.FormatInt(ref.ID, 10)).Scan(&docID)
		if err != nil && !isNoRows(err) {
			return err
		}
		if docID != 0 {
			if _, err := tx.ExecContext(ctx, `DELETE FROM searchable_documents_fts WHERE rowid = ?`, docID); err != nil {
				return err
			}
		}
		if !ok {
			if docID != 0 {
				_, err = tx.ExecContext(ctx, `DELETE FROM searchable_documents WHERE id = ?`, docID)
			}
			return err
		}
		var client any
		if doc.ClientID != 0 {
			client = strconv.FormatInt(doc.ClientID, 10)
		}
		if docID == 0 {
			res, err := tx.ExecContext(ctx, `INSERT INTO searchable_documents (searchable_type, searchable_id, client_id, created_at) VALUES (?, ?, ?, ?)`,
				ref.Type, strconv.FormatInt(ref.ID, 10), client, doc.CreatedAt.DB())
			if err != nil {
				return err
			}
			if docID, err = res.LastInsertId(); err != nil {
				return err
			}
		} else if _, err := tx.ExecContext(ctx, `UPDATE searchable_documents SET client_id = ? WHERE id = ?`, client, docID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO searchable_documents_fts (rowid, title, content) VALUES (?, ?, ?)`, docID, doc.Title, plainText(doc.Content))
		return err
	})
}

// ReindexAll rebuilds the whole index now: the CLI's search reindex, an
// import.
func (a *App) ReindexAll(ctx context.Context) error {
	if _, err := a.store.ExecContext(ctx, `DELETE FROM searchable_documents`); err != nil {
		return err
	}
	if _, err := a.store.ExecContext(ctx, `DELETE FROM searchable_documents_fts`); err != nil {
		return err
	}
	for _, typ := range SearchTypes {
		ids, err := store.Int64s(ctx, a.store, `SELECT id FROM `+searchTable(typ))
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := a.reindexOne(ctx, searchRef{typ, id}); err != nil {
				return err
			}
		}
	}
	return nil
}

// searchTable is a type's table: Client → clients, Todo → todos.
func searchTable(typ string) string {
	var b strings.Builder
	for i, r := range typ {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String() + "s"
}

// SearchHit is one result, ranked by relevance, with the matched words
// marked (<mark>…</mark>, the rest escaped).
type SearchHit struct {
	Type     string `json:"type"`
	ID       int64  `json:"id"`
	ClientID int64  `json:"client_id,omitempty"`
	Title    string `json:"title"`
	Snippet  string `json:"snippet"`
}

var searchJunk = regexp.MustCompile(`[^\w"]`)

// cleanTerms keeps what FTS5 can always parse: words, and quotes when they
// pair.
func cleanTerms(terms string) string {
	terms = searchJunk.ReplaceAllString(terms, " ")
	if strings.Count(terms, `"`)%2 == 1 {
		terms = strings.ReplaceAll(terms, `"`, "")
	}
	return strings.Join(strings.Fields(terms), " ")
}

// search answers the best 50 matches for terms.
func (a *App) search(ctx context.Context, terms string) ([]SearchHit, error) {
	terms = cleanTerms(terms)
	if terms == "" {
		return []SearchHit{}, nil
	}
	rows, err := a.store.QueryContext(ctx, `SELECT d.searchable_type, d.searchable_id, COALESCE(d.client_id, ''),
			highlight(searchable_documents_fts, 0, char(1), char(2)), snippet(searchable_documents_fts, 1, char(1), char(2), '…', 20)
		FROM searchable_documents_fts JOIN searchable_documents d ON d.id = searchable_documents_fts.rowid
		WHERE searchable_documents_fts MATCH ? ORDER BY bm25(searchable_documents_fts) LIMIT 50`, terms)
	if err != nil {
		return nil, err
	}
	hits, err := store.ScanAll(rows, func(r *sql.Rows) (SearchHit, error) {
		var h SearchHit
		var id, client string
		err := r.Scan(&h.Type, &id, &client, &h.Title, &h.Snippet)
		h.ID, _ = strconv.ParseInt(id, 10, 64)
		h.ClientID, _ = strconv.ParseInt(client, 10, 64)
		h.Title, h.Snippet = marked(h.Title), marked(h.Snippet)
		return h, err
	})
	if hits == nil {
		hits = []SearchHit{}
	}
	return hits, err
}

// marked escapes s and turns FTS5's \x01…\x02 into <mark>…</mark>.
func marked(s string) string {
	s = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
	return strings.NewReplacer("\x01", "<mark>", "\x02", "</mark>").Replace(s)
}
