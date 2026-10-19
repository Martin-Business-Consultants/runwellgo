package core

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Small helpers the areas share.

// humanize is "work_order" → "Work order".
func humanize(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// pluralize is "3 clients", "1 client" (an English plural with s, or the
// word as given when it already ends in s).
func pluralize(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", strings.TrimSuffix(word, "s"))
	}
	if !strings.HasSuffix(word, "s") && !strings.HasSuffix(word, "work") {
		word += "s"
	}
	return fmt.Sprintf("%d %s", n, word)
}

// nullable is s, or nil (NULL, JSON null) when it's blank.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// countBy reads "SELECT id, count(*) … GROUP BY id" into id → count.
func countBy(ctx context.Context, db store.Querier, query string, args ...any) (map[int64]int, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	out := map[int64]int{}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// countByString reads "SELECT key, count(*) … GROUP BY key" into key → count.
func countByString(ctx context.Context, db store.Querier, query string, args ...any) (map[string]int, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	defer rows.Close()
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, rows.Err()
}

// update sets columns on one row (and updated_at), in a stable order.
func (t *Tx) update(table string, id int64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sets := make([]string, 0, len(keys)+1)
	args := make([]any, 0, len(keys)+2)
	for _, k := range keys {
		sets = append(sets, `"`+k+`" = ?`)
		args = append(args, fields[k])
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, store.Now().DB(), id)
	_, err := t.Exec(`UPDATE `+table+` SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	return err
}

// ids are the ids of a list, for an IN (…) query.
func idArgs(ids []int64) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}
