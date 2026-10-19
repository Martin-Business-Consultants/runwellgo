package websitechecklist

// schema is the Rails plugin's tables as its migrations leave them (the
// CMS columns it no longer reads included), so an imported Rails database's
// rows are this plugin's rows, with the Rails indexes' names, so an imported
// database's indexes are these.
const schema = `
CREATE TABLE IF NOT EXISTS "website_checklist_sites" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "client_id" integer NOT NULL, "name" varchar NOT NULL, "url" varchar, "cms_url" varchar, "cms_token" text, "synced_at" datetime(6), "sync_error" text, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_website_checklist_sites_on_client_id" ON "website_checklist_sites" ("client_id");

CREATE TABLE IF NOT EXISTS "website_checklist_items" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "site_id" integer NOT NULL, "key" varchar NOT NULL, "group" varchar NOT NULL, "subject" varchar, "title" varchar NOT NULL, "detail" text, "kind" varchar DEFAULT 'recorded' NOT NULL, "value" text, "confirmed_value" text, "state" varchar DEFAULT 'unconfirmed' NOT NULL, "problem" text, "note" text, "confirmed_by_name" varchar, "confirmed_at" datetime(6), "detected_at" datetime(6), "href" varchar, "position" integer DEFAULT 0 NOT NULL, "custom" boolean DEFAULT FALSE NOT NULL, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL, "detected_value" text, CONSTRAINT "fk_rails_website_checklist_items_site" FOREIGN KEY ("site_id") REFERENCES "website_checklist_sites" ("id"));
CREATE INDEX IF NOT EXISTS "index_website_checklist_items_on_site_id" ON "website_checklist_items" ("site_id");
CREATE UNIQUE INDEX IF NOT EXISTS "index_website_checklist_items_on_site_id_and_key" ON "website_checklist_items" ("site_id", "key");

CREATE TABLE IF NOT EXISTS "website_checklist_webhooks" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "item_id" integer NOT NULL, "name" varchar, "url" varchar, "pairs" text, "position" integer DEFAULT 0 NOT NULL, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL, CONSTRAINT "fk_rails_website_checklist_webhooks_item" FOREIGN KEY ("item_id") REFERENCES "website_checklist_items" ("id"));
CREATE INDEX IF NOT EXISTS "index_website_checklist_webhooks_on_item_id" ON "website_checklist_webhooks" ("item_id");

CREATE TABLE IF NOT EXISTS "website_checklist_audits" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "site_id" integer, "site_name" varchar, "item_id" integer, "record_type" varchar NOT NULL, "record_id" integer, "record_label" varchar, "action" varchar NOT NULL, "diff" text, "actor_id" integer, "actor_name" varchar, "source" varchar, "via" varchar, "created_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_website_checklist_audits_on_site_id" ON "website_checklist_audits" ("site_id");
CREATE INDEX IF NOT EXISTS "index_website_checklist_audits_on_item_id" ON "website_checklist_audits" ("item_id");
CREATE INDEX IF NOT EXISTS "index_website_checklist_audits_on_actor_id" ON "website_checklist_audits" ("actor_id");

CREATE TABLE IF NOT EXISTS "website_checklist_scans" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "site_id" integer NOT NULL, "user_id" integer NOT NULL, "domain" varchar NOT NULL, "state" varchar DEFAULT 'queued' NOT NULL, "facts" text, "error" text, "started_at" datetime(6), "finished_at" datetime(6), "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL, CONSTRAINT "fk_rails_website_checklist_scans_site" FOREIGN KEY ("site_id") REFERENCES "website_checklist_sites" ("id"));
CREATE INDEX IF NOT EXISTS "index_website_checklist_scans_on_site_id" ON "website_checklist_scans" ("site_id");

CREATE TABLE IF NOT EXISTS "website_checklist_recommendations" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "scan_id" integer NOT NULL, "item_id" integer NOT NULL, "value" text, "note" text, "evidence" text, "confidence" varchar, "attach_file" varchar, "state" varchar DEFAULT 'pending' NOT NULL, "decided_by_name" varchar, "decided_at" datetime(6), "position" integer DEFAULT 0 NOT NULL, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL, CONSTRAINT "fk_rails_website_checklist_recommendations_scan" FOREIGN KEY ("scan_id") REFERENCES "website_checklist_scans" ("id"), CONSTRAINT "fk_rails_website_checklist_recommendations_item" FOREIGN KEY ("item_id") REFERENCES "website_checklist_items" ("id"));
CREATE INDEX IF NOT EXISTS "index_website_checklist_recommendations_on_scan_id" ON "website_checklist_recommendations" ("scan_id");
CREATE INDEX IF NOT EXISTS "index_website_checklist_recommendations_on_item_id" ON "website_checklist_recommendations" ("item_id");

CREATE TABLE IF NOT EXISTS "website_checklist_todo_links" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "site_id" integer NOT NULL, "item_id" integer, "todo_id" integer NOT NULL, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL, CONSTRAINT "fk_rails_website_checklist_todo_links_site" FOREIGN KEY ("site_id") REFERENCES "website_checklist_sites" ("id"), CONSTRAINT "fk_rails_website_checklist_todo_links_item" FOREIGN KEY ("item_id") REFERENCES "website_checklist_items" ("id"));
CREATE INDEX IF NOT EXISTS "index_website_checklist_todo_links_on_site_id" ON "website_checklist_todo_links" ("site_id");
CREATE INDEX IF NOT EXISTS "index_website_checklist_todo_links_on_item_id" ON "website_checklist_todo_links" ("item_id");
CREATE INDEX IF NOT EXISTS "index_website_checklist_todo_links_on_todo_id" ON "website_checklist_todo_links" ("todo_id");
`
