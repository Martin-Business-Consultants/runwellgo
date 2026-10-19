package ai

// schema is the Rails plugin's tables as its migrations leave them (ai_chats,
// ai_messages, ai_suggestions, ai_settings with model_list, ai_exclusions),
// so an imported Rails database's rows are this plugin's rows, and two of
// its own for what Rails kept in RubyLLM's tables: the tool calls a reply
// made (ai_tool_calls, with the person's decision on a change) and what
// each call to the provider cost (ai_usages); and the agents an owner
// defines (ai_agents) with their runs (ai_runs, each with a chat of its
// own, purpose "agent", for its transcript); and Approvals, the inbox of
// what waits on a person (ai_approvals); and the names of people removed
// from Runwell (ai_removed_people), for what they leave behind. A chat's ruby_llm_model_id
// is required in a Rails database; a new one here leaves it empty. A Rails
// database from before model_list gets the column added.
const schema = `
CREATE TABLE IF NOT EXISTS "ai_chats" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "ruby_llm_model_id" bigint, "cancelled" boolean DEFAULT FALSE NOT NULL, "user_id" integer, "contact_id" integer, "subject_type" varchar, "subject_id" integer, "purpose" varchar DEFAULT 'ask' NOT NULL, "title" varchar, "replying_since" datetime(6), "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_ai_chats_on_contact_id" ON "ai_chats" ("contact_id");
CREATE INDEX IF NOT EXISTS "index_ai_chats_on_subject" ON "ai_chats" ("subject_type", "subject_id");
CREATE INDEX IF NOT EXISTS "index_ai_chats_on_user_id_and_purpose_and_updated_at" ON "ai_chats" ("user_id", "purpose", "updated_at");
CREATE INDEX IF NOT EXISTS "index_ai_chats_on_user_id" ON "ai_chats" ("user_id");

CREATE TABLE IF NOT EXISTS "ai_messages" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "ai_chat_id" bigint NOT NULL, "role" varchar NOT NULL, "content" text, "cache_until_here" boolean DEFAULT FALSE NOT NULL, "thinking_text" text, "thinking_signature" text, "citations" json, "server_tool_calls" json, "raw_content" json, "raw_reasoning" json, "finish_reason" varchar, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_ai_messages_on_ai_chat_id" ON "ai_messages" ("ai_chat_id");

CREATE TABLE IF NOT EXISTS "ai_suggestions" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "user_id" integer, "subject_type" varchar, "subject_id" integer, "ai_chat_id" integer, "kind" varchar NOT NULL, "state" varchar DEFAULT 'working' NOT NULL, "payload" json, "error" varchar, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_ai_suggestions_on_ai_chat_id" ON "ai_suggestions" ("ai_chat_id");
CREATE INDEX IF NOT EXISTS "index_ai_suggestions_on_kind_subject_created_at" ON "ai_suggestions" ("kind", "subject_type", "subject_id", "created_at");
CREATE INDEX IF NOT EXISTS "index_ai_suggestions_on_subject" ON "ai_suggestions" ("subject_type", "subject_id");
CREATE INDEX IF NOT EXISTS "index_ai_suggestions_on_user_id" ON "ai_suggestions" ("user_id");

CREATE TABLE IF NOT EXISTS "ai_settings" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "enabled" boolean DEFAULT FALSE NOT NULL, "provider" varchar, "api_key" varchar, "api_base" varchar, "model" varchar, "fast_model" varchar, "monthly_budget_cents" integer, "portal" boolean DEFAULT FALSE NOT NULL, "last_test" json, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL, "model_list" json);

CREATE TABLE IF NOT EXISTS "ai_exclusions" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "client_id" integer NOT NULL, "user_id" integer, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS "index_ai_exclusions_on_client_id" ON "ai_exclusions" ("client_id");
CREATE INDEX IF NOT EXISTS "index_ai_exclusions_on_user_id" ON "ai_exclusions" ("user_id");

CREATE TABLE IF NOT EXISTS "ai_tool_calls" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "ai_message_id" bigint NOT NULL, "tool_call_id" varchar NOT NULL, "name" varchar NOT NULL, "arguments" json, "approval" varchar, "result" text, "is_error" boolean DEFAULT FALSE NOT NULL, "position" integer DEFAULT 0 NOT NULL, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_ai_tool_calls_on_ai_message_id" ON "ai_tool_calls" ("ai_message_id");

CREATE TABLE IF NOT EXISTS "ai_usages" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "ai_chat_id" bigint NOT NULL, "ai_message_id" bigint, "provider" varchar NOT NULL, "model" varchar NOT NULL, "input_tokens" integer, "output_tokens" integer, "cache_read_tokens" integer, "cache_write_tokens" integer, "total_cost" decimal(16,10), "created_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_ai_usages_on_ai_chat_id" ON "ai_usages" ("ai_chat_id");
CREATE INDEX IF NOT EXISTS "index_ai_usages_on_created_at" ON "ai_usages" ("created_at");
ALTER TABLE "ai_settings" ADD COLUMN "model_list" json;

CREATE TABLE IF NOT EXISTS "ai_agents" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "name" varchar NOT NULL, "handle" varchar NOT NULL, "description" text, "instructions" text, "model" varchar DEFAULT 'main' NOT NULL, "tools" varchar DEFAULT 'read' NOT NULL, "allowed_tools" json, "client_ids" json, "roles" json, "run_budget_cents" integer, "max_steps" integer DEFAULT 8 NOT NULL, "paused" boolean DEFAULT FALSE NOT NULL, "avatar_file_id" integer, "created_by_id" integer, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS "index_ai_agents_on_handle" ON "ai_agents" ("handle");

CREATE TABLE IF NOT EXISTS "ai_runs" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "agent_id" integer NOT NULL, "user_id" integer NOT NULL, "ai_chat_id" integer NOT NULL, "mention_id" integer, "status" varchar DEFAULT 'queued' NOT NULL, "source" varchar NOT NULL, "record" varchar, "path" varchar, "subject" varchar, "prompt" text, "ack_reply_id" varchar, "error" text, "cost" decimal(16,10), "started_at" datetime(6), "finished_at" datetime(6), "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_ai_runs_on_agent_id" ON "ai_runs" ("agent_id");
CREATE INDEX IF NOT EXISTS "index_ai_runs_on_user_id" ON "ai_runs" ("user_id");
CREATE INDEX IF NOT EXISTS "index_ai_runs_on_status" ON "ai_runs" ("status");

CREATE TABLE IF NOT EXISTS "ai_approvals" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "source" varchar NOT NULL, "source_id" varchar NOT NULL, "kind" varchar, "title" varchar NOT NULL, "summary" text, "record" varchar, "link" varchar, "details" text, "requested_by" integer, "approvers" json, "approve" json, "decline" json, "status" varchar DEFAULT 'waiting' NOT NULL, "outcome" varchar, "outcome_text" text, "note" text, "decided_by" integer, "decided_at" datetime(6), "ai_chat_id" integer, "tool_call_id" varchar, "run_id" integer, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS "index_ai_approvals_on_source" ON "ai_approvals" ("source", "source_id");
CREATE INDEX IF NOT EXISTS "index_ai_approvals_on_status" ON "ai_approvals" ("status");

ALTER TABLE "ai_chats" ADD COLUMN "model" varchar;
CREATE TABLE IF NOT EXISTS "ai_feedback" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "ai_message_id" integer NOT NULL, "user_id" integer NOT NULL, "rating" integer NOT NULL, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS "index_ai_feedback_on_message_and_user" ON "ai_feedback" ("ai_message_id", "user_id");
CREATE TABLE IF NOT EXISTS "ai_removed_people" ("user_id" integer PRIMARY KEY NOT NULL, "name" varchar NOT NULL, "successor_id" integer, "removed_at" datetime(6) NOT NULL);
`
