package core

import (
	"context"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Agent calls: every tool call an agent makes with a token, which tool, how
// it went and how long it took. The token's person (and an owner, for
// every token) sees them in Connected apps; kept 90 days. And idempotency
// keys: a write sent with one answers, when sent again with the same key
// (a retry after a timeout), the way it did the first time instead of
// running twice; kept a day.

const (
	agentCallsKept      = 90 * 24 * time.Hour
	idempotencyKeysKept = 24 * time.Hour
)

func init() {
	every("agent call cleanup", 6*time.Hour, func(a *App, ctx context.Context) error {
		if _, err := a.store.ExecContext(ctx, `DELETE FROM agent_calls WHERE created_at < ?`, store.TimeOf(time.Now().Add(-agentCallsKept)).DB()); err != nil {
			return err
		}
		_, err := a.store.ExecContext(ctx, `DELETE FROM agent_idempotency_keys WHERE created_at < ?`, store.TimeOf(time.Now().Add(-idempotencyKeysKept)).DB())
		return err
	})
}

// recordAgentCall logs one call made with tokenID. It's bookkeeping: a
// failure is logged, and the answer goes out anyway.
func (a *App) recordAgentCall(ctx context.Context, tokenID int64, tool string, body map[string]any, started time.Time) {
	var code any
	if c := str(body["code"]); c != "" {
		code = c
	}
	summary := nullable(truncate(str(body["summary"]), 250))
	_, err := a.store.ExecContext(context.WithoutCancel(ctx), `INSERT INTO agent_calls (access_token_id, tool, status, code, summary, duration_ms, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		tokenID, tool, str(body["status"]), code, summary, time.Since(started).Milliseconds(), store.Now().DB())
	if err != nil {
		a.log.Warn("agent call", "tool", tool, "err", err)
	}
}

// priorAnswer is how a write sent with key first answered, and which tool
// it was; ok false when the key is new.
func (a *App) priorAnswer(ctx context.Context, tokenID int64, key string) (tool string, answer map[string]any, ok bool, err error) {
	var raw store.JSON
	err = a.store.QueryRowContext(ctx, `SELECT tool, response FROM agent_idempotency_keys WHERE access_token_id = ? AND key = ?`, tokenID, key).Scan(&tool, &raw)
	if isNoRows(err) {
		return "", nil, false, nil
	} else if err != nil {
		return "", nil, false, err
	}
	return tool, raw.Map(), true, nil
}

// rememberAnswer keeps a successful write's answer under its key. Another
// call with the key got there first: theirs stands.
func (a *App) rememberAnswer(ctx context.Context, tokenID int64, key, tool string, answer map[string]any) {
	raw, err := marshal(answer)
	if err != nil {
		return
	}
	_, err = a.store.ExecContext(context.WithoutCancel(ctx), `INSERT INTO agent_idempotency_keys (access_token_id, key, tool, response, created_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (access_token_id, key) DO NOTHING`, tokenID, key, tool, string(raw), store.Now().DB())
	if err != nil {
		a.log.Warn("idempotency key", "tool", tool, "err", err)
	}
}

// agentCallsJSON are tokens' recent calls, newest first, at most n each.
func (a *App) agentCallsJSON(ctx context.Context, tokenIDs []int64, n int) (map[int64][]Obj, error) {
	out := map[int64][]Obj{}
	if len(tokenIDs) == 0 {
		return out, nil
	}
	rows, err := a.store.QueryContext(ctx, `SELECT id, access_token_id, tool, status, COALESCE(code, ''), COALESCE(summary, ''), duration_ms, created_at FROM (
			SELECT *, row_number() OVER (PARTITION BY access_token_id ORDER BY created_at DESC, id DESC) AS n FROM agent_calls WHERE access_token_id IN (`+store.Placeholders(len(tokenIDs))+`)
		) WHERE n <= ? ORDER BY created_at DESC, id DESC`, append(idArgs(tokenIDs), n)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, tokenID int64
		var tool, status, code, summary string
		var duration store.NullInt
		var at store.Time
		if err := rows.Scan(&id, &tokenID, &tool, &status, &code, &summary, &duration, &at); err != nil {
			return nil, err
		}
		out[tokenID] = append(out[tokenID], O("id", id, "tool", tool, "status", status, "code", nullable(code), "summary", nullable(summary), "duration_ms", duration, "created_at", at))
	}
	return out, rows.Err()
}
