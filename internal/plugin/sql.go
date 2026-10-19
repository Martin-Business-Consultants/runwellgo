package plugin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// A plugin granted "tables" keeps its records in tables of its own, named
// <key>_… (website_checklist_sites), which its manifest's Schema makes. It
// reaches them with plain SQL through its API, and only them: every
// statement is read before it runs, and one that names any other table
// (the core's, another plugin's, SQLite's own), changes the schema, or holds
// more than one statement is refused. The core's records are read and
// changed through Records and the write verbs, never by SQL.
//
// The check reads table names where SQL puts them (after FROM, JOIN, INTO,
// UPDATE, TABLE and in FROM's comma list); it's a guard, not a sandbox, so
// a built-in plugin is still reviewed like the rest of the code.

// keywords that end a FROM list or can't be a table name.
var sqlKeywords = map[string]bool{
	"where": true, "group": true, "order": true, "limit": true, "having": true, "on": true, "using": true,
	"join": true, "inner": true, "left": true, "right": true, "full": true, "outer": true, "cross": true, "natural": true,
	"union": true, "except": true, "intersect": true, "window": true, "as": true, "set": true, "values": true,
	"select": true, "returning": true, "default": true, "lateral": true,
}

// forbidden are statements a plugin's queries may not run.
var forbidden = map[string]bool{
	"attach": true, "detach": true, "pragma": true, "vacuum": true, "create": true, "drop": true, "alter": true,
	"reindex": true, "analyze": true, "begin": true, "commit": true, "rollback": true, "savepoint": true, "release": true,
}

// token is one piece of a statement: a word or quoted name (name true), or
// punctuation.
type token struct {
	text string
	name bool
}

// tokenize splits SQL into words, names and punctuation, dropping string
// literals and comments.
func tokenize(q string) []token {
	var out []token
	for i := 0; i < len(q); {
		c := q[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '-' && i+1 < len(q) && q[i+1] == '-':
			for i < len(q) && q[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(q) && q[i+1] == '*':
			end := strings.Index(q[i+2:], "*/")
			if end < 0 {
				i = len(q)
			} else {
				i += end + 4
			}
		case c == '\'':
			i++
			for i < len(q) {
				if q[i] == '\'' {
					if i+1 < len(q) && q[i+1] == '\'' {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
			out = append(out, token{text: "'"})
		case c == '"' || c == '`' || c == '[':
			closing := c
			if c == '[' {
				closing = ']'
			}
			j := strings.IndexByte(q[i+1:], closing)
			if j < 0 {
				j = len(q) - i - 1
			}
			out = append(out, token{text: strings.ToLower(q[i+1 : i+1+j]), name: true})
			i += j + 2
		case c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9':
			j := i
			for j < len(q) && (q[j] == '_' || q[j] == '$' || q[j] >= 'a' && q[j] <= 'z' || q[j] >= 'A' && q[j] <= 'Z' || q[j] >= '0' && q[j] <= '9') {
				j++
			}
			out = append(out, token{text: strings.ToLower(q[i:j]), name: true})
			i = j
		default:
			out = append(out, token{text: string(c)})
			i++
		}
	}
	return out
}

// checkStatement refuses a query that isn't one statement on the plugin's
// own tables.
func checkStatement(key, q string) error {
	toks := tokenize(q)
	own := func(name string) error {
		// schema-qualified (main.x) is read as its last part
		if !strings.HasPrefix(name, key+"_") {
			return fmt.Errorf("%q isn't one of this plugin's tables (they're named %s_…)", name, key)
		}
		return nil
	}
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.text == ";" {
			if i != len(toks)-1 {
				return errors.New("one statement at a time")
			}
			continue
		}
		if !t.name {
			continue
		}
		if i == 0 && forbidden[t.text] {
			return fmt.Errorf("a plugin's queries can't %s: its tables come from its manifest's schema", strings.ToUpper(t.text))
		}
		switch t.text {
		case "from", "join", "into", "update", "table":
			j := i + 1
			for {
				if j >= len(toks) {
					break
				}
				n := toks[j]
				if n.text == "(" { // a subquery: its own FROM is checked as it's read
					break
				}
				if !n.name || sqlKeywords[n.text] {
					break
				}
				name := n.text
				if j+2 < len(toks) && toks[j+1].text == "." && toks[j+2].name {
					name = toks[j+2].text
					j += 2
				}
				if err := own(name); err != nil {
					return err
				}
				// FROM a, b: the comma list (skipping an alias).
				j++
				if j < len(toks) && toks[j].name && !sqlKeywords[toks[j].text] {
					j++ // alias
				}
				if t.text == "from" && j < len(toks) && toks[j].text == "," {
					j++
					continue
				}
				break
			}
		}
	}
	return nil
}

// checkSchema accepts only CREATE TABLE / CREATE INDEX IF NOT EXISTS on
// the plugin's own names, and ALTER TABLE … ADD COLUMN on its own tables
// (a column a newer version adds to a table an older one, or the Rails
// plugin, made; applySchema skips it where the column is already there).
func checkSchema(key, schema string) error {
	for _, stmt := range splitStatements(schema) {
		toks := tokenize(stmt)
		words := []string{}
		for _, t := range toks {
			if t.name {
				words = append(words, t.text)
			}
		}
		if len(words) >= 5 && words[0] == "alter" && words[1] == "table" && words[3] == "add" {
			if !strings.HasPrefix(words[2], key+"_") {
				return fmt.Errorf("%q isn't named %s_…", words[2], key)
			}
			continue
		}
		ok := len(words) >= 6 && words[0] == "create" &&
			((words[1] == "table" && words[2] == "if" && words[3] == "not" && words[4] == "exists") ||
				(words[1] == "index" && words[2] == "if" && words[3] == "not" && words[4] == "exists") ||
				(words[1] == "unique" && words[2] == "index" && words[3] == "if" && words[4] == "not" && words[5] == "exists"))
		if !ok {
			return fmt.Errorf("only CREATE TABLE IF NOT EXISTS, CREATE [UNIQUE] INDEX IF NOT EXISTS and ALTER TABLE … ADD COLUMN: %.60s", stmt)
		}
		name := words[5]
		if words[1] == "unique" {
			name = words[6]
		}
		// A table is <key>_…; an index may also be named the Rails way,
		// index_<key>_…, so a ported plugin's indexes keep their names.
		isIndex := words[1] == "index" || words[1] == "unique"
		if !strings.HasPrefix(name, key+"_") && !(isIndex && strings.HasPrefix(name, "index_"+key+"_")) {
			return fmt.Errorf("%q isn't named %s_…", name, key)
		}
		if words[1] == "index" || words[1] == "unique" {
			for i, w := range words {
				if w == "on" && i+1 < len(words) && !strings.HasPrefix(words[i+1], key+"_") {
					return fmt.Errorf("index %q is on %q, not one of this plugin's tables", name, words[i+1])
				}
			}
		}
	}
	return nil
}

// splitStatements splits a schema on its semicolons (outside quotes).
func splitStatements(schema string) []string {
	var out []string
	var cur strings.Builder
	quote := byte(0)
	for i := 0; i < len(schema); i++ {
		c := schema[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == ';':
			if s := strings.TrimSpace(cur.String()); s != "" {
				out = append(out, s)
			}
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		out = append(out, s)
	}
	return out
}

// applySchema makes a plugin's tables, statement by statement.
func applySchema(ctx context.Context, db *store.DB, key, schema string) error {
	if err := checkSchema(key, schema); err != nil {
		return err
	}
	for _, stmt := range splitStatements(schema) {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			// A column added before (ALTER TABLE … ADD COLUMN, run each load).
			if strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			return fmt.Errorf("%.60s: %w", stmt, err)
		}
	}
	return nil
}

// Querier is what a plugin's SQL runs on: its API, or a transaction of its.
type Querier interface {
	Exec(ctx context.Context, query string, args ...any) (sql.Result, error)
	Query(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRow(ctx context.Context, query string, args ...any) *Row
}

// Row is one row a query answered, or why it couldn't.
type Row struct {
	row *sql.Row
	err error
}

// Scan reads the row; sql.ErrNoRows when there was none.
func (r *Row) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	return r.row.Scan(dest...)
}

func (a *API) tables(q string) error {
	if err := a.need("tables", a.grants.Tables); err != nil {
		return err
	}
	return checkStatement(a.key, q)
}

// Exec runs a statement on the plugin's own tables (tables).
func (a *API) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if err := a.tables(query); err != nil {
		return nil, err
	}
	return a.h.db.ExecContext(ctx, query, args...)
}

// Query reads the plugin's own tables (tables). Close the rows.
func (a *API) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if err := a.tables(query); err != nil {
		return nil, err
	}
	return a.h.db.QueryContext(ctx, query, args...)
}

// QueryRow reads one row of the plugin's own tables (tables).
func (a *API) QueryRow(ctx context.Context, query string, args ...any) *Row {
	if err := a.tables(query); err != nil {
		return &Row{err: err}
	}
	return &Row{row: a.h.db.QueryRowContext(ctx, query, args...)}
}

// Tx runs fn in one transaction on the plugin's own tables (tables): all of
// it or none.
func (a *API) Tx(ctx context.Context, fn func(tx Querier) error) error {
	if err := a.need("tables", a.grants.Tables); err != nil {
		return err
	}
	return a.h.db.Tx(ctx, func(t *sql.Tx) error { return fn(&pluginTx{key: a.key, tx: t}) })
}

type pluginTx struct {
	key string
	tx  *sql.Tx
}

func (p *pluginTx) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if err := checkStatement(p.key, query); err != nil {
		return nil, err
	}
	return p.tx.ExecContext(ctx, query, args...)
}

func (p *pluginTx) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if err := checkStatement(p.key, query); err != nil {
		return nil, err
	}
	return p.tx.QueryContext(ctx, query, args...)
}

func (p *pluginTx) QueryRow(ctx context.Context, query string, args ...any) *Row {
	if err := checkStatement(p.key, query); err != nil {
		return &Row{err: err}
	}
	return &Row{row: p.tx.QueryRowContext(ctx, query, args...)}
}

// API satisfies Querier too.
var _ Querier = (*API)(nil)
