package chat

// schema is chat's tables. Times are UTC ISO 8601 strings, so they sort.
// A conversation is a channel (named, public or private), a dm (two
// people, or one with themselves) or a group (a few people); dm_key keeps
// one dm or group per set of people. A member's last_read_id is the last
// message they've seen there. A message with a parent_id is a reply in
// that message's thread; deleted ones keep their row (shown as deleted).
// chat_files are the files on a message (attached to Chat::Message), with
// an image's size; a message's agent is who wrote it when a plugin's
// mentionable replied (JSON: id, name, handle, avatar, via), on behalf of
// its user_id; chat_profiles are each person's chat settings;
// chat_removed_people name the people removed from Runwell, whose messages
// stay (removed.go).
const schema = `
CREATE TABLE IF NOT EXISTS "chat_conversations" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "kind" varchar NOT NULL, "name" varchar, "topic" varchar, "description" text, "private" boolean DEFAULT 0 NOT NULL, "dm_key" varchar, "created_by_id" integer, "archived_at" varchar, "last_message_at" varchar, "created_at" varchar NOT NULL, "updated_at" varchar NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS "index_chat_conversations_on_dm_key" ON "chat_conversations" ("dm_key");

CREATE TABLE IF NOT EXISTS "chat_members" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "conversation_id" integer NOT NULL, "user_id" integer NOT NULL, "last_read_id" integer DEFAULT 0 NOT NULL, "muted" boolean DEFAULT 0 NOT NULL, "created_at" varchar NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS "index_chat_members_on_conversation_id_and_user_id" ON "chat_members" ("conversation_id", "user_id");
CREATE INDEX IF NOT EXISTS "index_chat_members_on_user_id" ON "chat_members" ("user_id");

CREATE TABLE IF NOT EXISTS "chat_messages" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "conversation_id" integer NOT NULL, "user_id" integer NOT NULL, "parent_id" integer, "body" text, "body_text" text, "via" varchar, "edited_at" varchar, "deleted_at" varchar, "reply_count" integer DEFAULT 0 NOT NULL, "last_reply_at" varchar, "created_at" varchar NOT NULL, "updated_at" varchar NOT NULL);
CREATE INDEX IF NOT EXISTS "index_chat_messages_on_conversation" ON "chat_messages" ("conversation_id", "parent_id", "id");
CREATE INDEX IF NOT EXISTS "index_chat_messages_on_parent_id" ON "chat_messages" ("parent_id");

CREATE TABLE IF NOT EXISTS "chat_reactions" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "message_id" integer NOT NULL, "user_id" integer NOT NULL, "emoji" varchar NOT NULL, "created_at" varchar NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS "index_chat_reactions_on_message_user_emoji" ON "chat_reactions" ("message_id", "user_id", "emoji");

CREATE TABLE IF NOT EXISTS "chat_files" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "message_id" integer NOT NULL, "file_id" integer NOT NULL, "filename" varchar NOT NULL, "content_type" varchar, "byte_size" integer DEFAULT 0 NOT NULL, "width" integer, "height" integer, "created_at" varchar NOT NULL);
CREATE INDEX IF NOT EXISTS "index_chat_files_on_message_id" ON "chat_files" ("message_id");

ALTER TABLE "chat_messages" ADD COLUMN "agent" text;

CREATE TABLE IF NOT EXISTS "chat_profiles" ("user_id" integer PRIMARY KEY NOT NULL, "display_name" varchar, "status_text" varchar, "status_emoji" varchar, "hours" text, "notify" varchar DEFAULT 'mentions' NOT NULL, "sound" boolean DEFAULT 1 NOT NULL, "browser" boolean DEFAULT 0 NOT NULL, "avatar_file_id" integer, "active" boolean DEFAULT 1 NOT NULL, "seen_at" varchar, "updated_at" varchar NOT NULL);

CREATE TABLE IF NOT EXISTS "chat_removed_people" ("user_id" integer PRIMARY KEY NOT NULL, "name" varchar NOT NULL, "successor_id" integer, "removed_at" varchar NOT NULL);
`
