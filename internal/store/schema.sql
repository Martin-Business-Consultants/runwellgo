-- The Rails core's tables, as db/schema.rb has them (runwellv2 2.29), so a
-- database the Rails core wrote opens here as it is; then the Go core's own.
-- Every statement is IF NOT EXISTS. Columns the Go core adds to a Rails table
-- (sessions' token_digest, …) are nullable or defaulted, so ensureColumns can
-- add them in place. JSON columns are text, as Rails keeps them in SQLite.

CREATE TABLE IF NOT EXISTS "access_tokens" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "user_id" integer, "name" varchar NOT NULL, "kind" varchar DEFAULT 'personal' NOT NULL, "token_digest" varchar NOT NULL, "refresh_token_digest" varchar, "oauth_client_id" integer, "expires_at" datetime(6), "refresh_expires_at" datetime(6), "last_used_at" datetime(6), "revoked_at" datetime(6), "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL, "read_only" boolean DEFAULT 0 NOT NULL, "contact_id" integer, "paused_at" datetime(6));
CREATE INDEX IF NOT EXISTS "index_access_tokens_on_contact_id" ON "access_tokens" ("contact_id");
CREATE INDEX IF NOT EXISTS "index_access_tokens_on_oauth_client_id" ON "access_tokens" ("oauth_client_id");
CREATE UNIQUE INDEX IF NOT EXISTS "index_access_tokens_on_refresh_token_digest" ON "access_tokens" ("refresh_token_digest");
CREATE UNIQUE INDEX IF NOT EXISTS "index_access_tokens_on_token_digest" ON "access_tokens" ("token_digest");
CREATE INDEX IF NOT EXISTS "index_access_tokens_on_user_id" ON "access_tokens" ("user_id");
CREATE TABLE IF NOT EXISTS "action_mailbox_inbound_emails" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "status" integer DEFAULT 0 NOT NULL, "message_id" varchar NOT NULL, "message_checksum" varchar NOT NULL, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS "index_action_mailbox_inbound_emails_uniqueness" ON "action_mailbox_inbound_emails" ("message_id", "message_checksum");
CREATE TABLE IF NOT EXISTS "active_storage_attachments" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "name" varchar NOT NULL, "record_type" varchar NOT NULL, "record_id" bigint NOT NULL, "blob_id" bigint NOT NULL, "created_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_active_storage_attachments_on_blob_id" ON "active_storage_attachments" ("blob_id");
CREATE UNIQUE INDEX IF NOT EXISTS "index_active_storage_attachments_uniqueness" ON "active_storage_attachments" ("record_type", "record_id", "name", "blob_id");
CREATE TABLE IF NOT EXISTS "active_storage_blobs" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "key" varchar NOT NULL, "filename" varchar NOT NULL, "content_type" varchar, "metadata" text, "service_name" varchar NOT NULL, "byte_size" bigint NOT NULL, "checksum" varchar, "created_at" datetime(6) NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS "index_active_storage_blobs_on_key" ON "active_storage_blobs" ("key");
CREATE TABLE IF NOT EXISTS "active_storage_variant_records" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "blob_id" bigint NOT NULL, "variation_digest" varchar NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS "index_active_storage_variant_records_uniqueness" ON "active_storage_variant_records" ("blob_id", "variation_digest");
CREATE TABLE IF NOT EXISTS "agent_calls" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "access_token_id" integer NOT NULL, "tool" varchar NOT NULL, "status" varchar NOT NULL, "code" varchar, "summary" varchar, "duration_ms" integer, "created_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_agent_calls_on_access_token_id_and_created_at" ON "agent_calls" ("access_token_id", "created_at");
CREATE INDEX IF NOT EXISTS "index_agent_calls_on_access_token_id" ON "agent_calls" ("access_token_id");
CREATE TABLE IF NOT EXISTS "agent_idempotency_keys" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "access_token_id" integer NOT NULL, "key" varchar NOT NULL, "tool" varchar NOT NULL, "response" json DEFAULT '{}' NOT NULL, "created_at" datetime(6) NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS "index_agent_idempotency_keys_on_access_token_id_and_key" ON "agent_idempotency_keys" ("access_token_id", "key");
CREATE INDEX IF NOT EXISTS "index_agent_idempotency_keys_on_created_at" ON "agent_idempotency_keys" ("created_at");
CREATE TABLE IF NOT EXISTS "agreement_versions" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "engagement_id" integer NOT NULL, "number" integer NOT NULL, "kind" varchar DEFAULT 'initial' NOT NULL, "summary" varchar, "reason" text, "cadence" varchar, "amount_cents" integer DEFAULT 0 NOT NULL, "price_delta_cents" integer DEFAULT 0 NOT NULL, "snapshot" json, "content_hash" varchar, "sent_at" datetime(6), "sent_by_id" integer, "superseded_by_id" integer, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS "index_agreement_versions_on_engagement_id_and_number" ON "agreement_versions" ("engagement_id", "number");
CREATE INDEX IF NOT EXISTS "index_agreement_versions_on_engagement_id" ON "agreement_versions" ("engagement_id");
CREATE INDEX IF NOT EXISTS "index_agreement_versions_on_sent_by_id" ON "agreement_versions" ("sent_by_id");
CREATE INDEX IF NOT EXISTS "index_agreement_versions_on_superseded_by_id" ON "agreement_versions" ("superseded_by_id");
CREATE TABLE IF NOT EXISTS "approval_links" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "agreement_version_id" integer NOT NULL, "contact_id" integer NOT NULL, "token" varchar NOT NULL, "expires_at" datetime(6) NOT NULL, "opened_at" datetime(6), "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_approval_links_on_agreement_version_id" ON "approval_links" ("agreement_version_id");
CREATE INDEX IF NOT EXISTS "index_approval_links_on_contact_id" ON "approval_links" ("contact_id");
CREATE UNIQUE INDEX IF NOT EXISTS "index_approval_links_on_token" ON "approval_links" ("token");
CREATE TABLE IF NOT EXISTS "approvals" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "agreement_version_id" integer NOT NULL, "decision" varchar NOT NULL, "contact_id" integer, "approver_name" varchar, "approver_email" varchar, "comment" text, "method" varchar NOT NULL, "evidence" text, "ip_address" varchar, "user_agent" varchar, "content_hash" varchar, "decided_at" datetime(6) NOT NULL, "recorded_by_id" integer, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS "index_approvals_on_agreement_version_id" ON "approvals" ("agreement_version_id");
CREATE INDEX IF NOT EXISTS "index_approvals_on_contact_id" ON "approvals" ("contact_id");
CREATE INDEX IF NOT EXISTS "index_approvals_on_recorded_by_id" ON "approvals" ("recorded_by_id");
CREATE TABLE IF NOT EXISTS "assignments" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "todo_id" integer NOT NULL, "user_id" integer NOT NULL, "assigner_id" integer, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_assignments_on_assigner_id" ON "assignments" ("assigner_id");
CREATE UNIQUE INDEX IF NOT EXISTS "index_assignments_on_todo_id_and_user_id" ON "assignments" ("todo_id", "user_id");
CREATE INDEX IF NOT EXISTS "index_assignments_on_todo_id" ON "assignments" ("todo_id");
CREATE INDEX IF NOT EXISTS "index_assignments_on_user_id" ON "assignments" ("user_id");
CREATE TABLE IF NOT EXISTS "clients" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "name" varchar NOT NULL, "status" varchar DEFAULT 'active' NOT NULL, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL, "time_zone" varchar, "internal" boolean DEFAULT 0 NOT NULL, "ai_excluded" boolean DEFAULT 0 NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS "index_clients_on_name" ON "clients" ("name");
CREATE TABLE IF NOT EXISTS "commitments" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "client_id" integer NOT NULL, "engagement_id" integer, "description" varchar NOT NULL, "owner_kind" varchar DEFAULT 'us' NOT NULL, "user_id" integer, "contact_id" integer, "due_on" date NOT NULL, "resolution" varchar, "resolved_at" datetime(6), "resolution_note" text, "source" varchar NOT NULL, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_commitments_on_client_id" ON "commitments" ("client_id");
CREATE INDEX IF NOT EXISTS "index_commitments_on_contact_id" ON "commitments" ("contact_id");
CREATE INDEX IF NOT EXISTS "index_commitments_on_engagement_id" ON "commitments" ("engagement_id");
CREATE INDEX IF NOT EXISTS "index_commitments_on_user_id" ON "commitments" ("user_id");
CREATE TABLE IF NOT EXISTS "contacts" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "client_id" integer NOT NULL, "name" varchar NOT NULL, "email" varchar, "phone" varchar, "role" varchar, "can_approve" boolean DEFAULT 0 NOT NULL, "archived_at" datetime(6), "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL, "portal_access" boolean DEFAULT 0 NOT NULL);
CREATE INDEX IF NOT EXISTS "index_contacts_on_client_id" ON "contacts" ("client_id");
CREATE INDEX IF NOT EXISTS "index_contacts_on_email" ON "contacts" ("email");
CREATE TABLE IF NOT EXISTS "custom_fields" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "model_type" varchar NOT NULL, "label" varchar NOT NULL, "key" varchar NOT NULL, "kind" varchar DEFAULT 'text' NOT NULL, "choices" json, "position" integer NOT NULL, "listed" boolean DEFAULT 0 NOT NULL, "client_visible" boolean DEFAULT 0 NOT NULL, "archived_at" datetime(6), "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS "index_custom_fields_on_model_type_and_key" ON "custom_fields" ("model_type", "key");
CREATE INDEX IF NOT EXISTS "index_custom_fields_on_model_type_and_position" ON "custom_fields" ("model_type", "position");
CREATE TABLE IF NOT EXISTS "custom_values" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "custom_field_id" integer NOT NULL, "subject_type" varchar NOT NULL, "subject_id" integer NOT NULL, "value" text NOT NULL, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS "index_custom_values_uniqueness" ON "custom_values" ("custom_field_id", "subject_type", "subject_id");
CREATE INDEX IF NOT EXISTS "index_custom_values_on_custom_field_id" ON "custom_values" ("custom_field_id");
CREATE INDEX IF NOT EXISTS "index_custom_values_on_subject" ON "custom_values" ("subject_type", "subject_id");
CREATE TABLE IF NOT EXISTS "documents" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "documentable_type" varchar NOT NULL, "documentable_id" integer NOT NULL, "uploaded_by_id" integer, "client_visible" boolean DEFAULT 0 NOT NULL, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_documents_on_documentable" ON "documents" ("documentable_type", "documentable_id");
CREATE INDEX IF NOT EXISTS "index_documents_on_uploaded_by_id" ON "documents" ("uploaded_by_id");
CREATE TABLE IF NOT EXISTS "engagements" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "client_id" integer NOT NULL, "ref" varchar NOT NULL, "label" varchar DEFAULT 'work_order' NOT NULL, "shape" varchar DEFAULT 'fixed' NOT NULL, "title" varchar NOT NULL, "description" text, "estimate_notes" text, "closed_at" datetime(6), "close_reason" varchar, "created_by_id" integer, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL, "close_outcome" varchar);
CREATE INDEX IF NOT EXISTS "index_engagements_on_client_id" ON "engagements" ("client_id");
CREATE INDEX IF NOT EXISTS "index_engagements_on_created_by_id" ON "engagements" ("created_by_id");
CREATE UNIQUE INDEX IF NOT EXISTS "index_engagements_on_ref" ON "engagements" ("ref");
CREATE TABLE IF NOT EXISTS "events" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "subject_type" varchar NOT NULL, "subject_id" bigint NOT NULL, "kind" varchar NOT NULL, "payload" json, "actor_user_id" integer, "actor" varchar, "source" varchar NOT NULL, "occurred_at" datetime(6) NOT NULL, "created_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_events_on_actor_user_id" ON "events" ("actor_user_id");
CREATE INDEX IF NOT EXISTS "index_events_on_subject_type_and_subject_id_and_occurred_at" ON "events" ("subject_type", "subject_id", "occurred_at");
CREATE TABLE IF NOT EXISTS "exports" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "user_id" integer NOT NULL, "state" varchar DEFAULT 'queued' NOT NULL, "tables_count" integer, "rows_count" integer, "files_count" integer, "error" varchar, "completed_at" datetime(6), "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_exports_on_user_id" ON "exports" ("user_id");
CREATE TABLE IF NOT EXISTS "identities" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "user_id" integer NOT NULL, "provider" varchar NOT NULL, "uid" varchar NOT NULL, "email" varchar, "last_used_at" datetime(6), "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS "index_identities_on_provider_and_uid" ON "identities" ("provider", "uid");
CREATE INDEX IF NOT EXISTS "index_identities_on_user_id" ON "identities" ("user_id");
CREATE TABLE IF NOT EXISTS "invitations" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "email_address" varchar NOT NULL, "role" varchar DEFAULT 'member' NOT NULL, "invited_by_id" integer NOT NULL, "user_id" integer, "sent_at" datetime(6) NOT NULL, "accepted_at" datetime(6), "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_invitations_on_email_address" ON "invitations" ("email_address");
CREATE INDEX IF NOT EXISTS "index_invitations_on_invited_by_id" ON "invitations" ("invited_by_id");
CREATE INDEX IF NOT EXISTS "index_invitations_on_user_id" ON "invitations" ("user_id");
CREATE TABLE IF NOT EXISTS "mentions" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "source_type" varchar NOT NULL, "source_id" integer NOT NULL, "mentioner_id" integer NOT NULL, "mentionee_id" integer NOT NULL, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_mentions_on_mentionee_id" ON "mentions" ("mentionee_id");
CREATE INDEX IF NOT EXISTS "index_mentions_on_mentioner_id" ON "mentions" ("mentioner_id");
CREATE UNIQUE INDEX IF NOT EXISTS "index_mentions_on_source_type_and_source_id_and_mentionee_id" ON "mentions" ("source_type", "source_id", "mentionee_id");
CREATE INDEX IF NOT EXISTS "index_mentions_on_source" ON "mentions" ("source_type", "source_id");
CREATE TABLE IF NOT EXISTS "notes" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "subject_type" varchar NOT NULL, "subject_id" bigint NOT NULL, "body" text NOT NULL, "kind" varchar DEFAULT 'internal' NOT NULL, "occurred_at" datetime(6) NOT NULL, "author_id" integer, "source" varchar NOT NULL, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL, "written_as" varchar);
CREATE INDEX IF NOT EXISTS "index_notes_on_author_id" ON "notes" ("author_id");
CREATE INDEX IF NOT EXISTS "index_notes_on_subject_type_and_subject_id_and_occurred_at" ON "notes" ("subject_type", "subject_id", "occurred_at");
CREATE TABLE IF NOT EXISTS "notifications" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "user_id" integer NOT NULL, "creator_id" integer NOT NULL, "source_type" varchar NOT NULL, "source_id" integer NOT NULL, "read_at" datetime(6), "unread_count" integer DEFAULT 0 NOT NULL, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_notifications_on_creator_id" ON "notifications" ("creator_id");
CREATE INDEX IF NOT EXISTS "index_notifications_on_source" ON "notifications" ("source_type", "source_id");
CREATE INDEX IF NOT EXISTS "index_notifications_on_user_id_and_read_at_and_updated_at" ON "notifications" ("user_id", "read_at", "updated_at");
CREATE INDEX IF NOT EXISTS "index_notifications_on_user_id" ON "notifications" ("user_id");
CREATE TABLE IF NOT EXISTS "oauth_clients" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "uid" varchar NOT NULL, "name" varchar NOT NULL, "redirect_uris" json NOT NULL, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS "index_oauth_clients_on_uid" ON "oauth_clients" ("uid");
CREATE TABLE IF NOT EXISTS "oauth_grants" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "oauth_client_id" integer NOT NULL, "user_id" integer, "code_digest" varchar NOT NULL, "redirect_uri" varchar NOT NULL, "code_challenge" varchar NOT NULL, "expires_at" datetime(6) NOT NULL, "used_at" datetime(6), "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL, "read_only" boolean DEFAULT 0 NOT NULL, "contact_id" integer);
CREATE UNIQUE INDEX IF NOT EXISTS "index_oauth_grants_on_code_digest" ON "oauth_grants" ("code_digest");
CREATE INDEX IF NOT EXISTS "index_oauth_grants_on_contact_id" ON "oauth_grants" ("contact_id");
CREATE INDEX IF NOT EXISTS "index_oauth_grants_on_oauth_client_id" ON "oauth_grants" ("oauth_client_id");
CREATE INDEX IF NOT EXISTS "index_oauth_grants_on_user_id" ON "oauth_grants" ("user_id");
CREATE TABLE IF NOT EXISTS "plugin_changes" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "requested_by_id" integer, "key" varchar, "repo" varchar NOT NULL, "action" varchar NOT NULL, "from_version" varchar, "to_version" varchar, "status" varchar DEFAULT 'running' NOT NULL, "message" text, "finished_at" datetime(6), "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_plugin_changes_on_requested_by_id" ON "plugin_changes" ("requested_by_id");
CREATE INDEX IF NOT EXISTS "index_plugin_changes_on_status" ON "plugin_changes" ("status");
CREATE TABLE IF NOT EXISTS "portal_sessions" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "contact_id" integer NOT NULL, "ip_address" varchar, "user_agent" varchar, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL, "token_digest" varchar, "csrf" varchar, "last_seen_at" datetime(6));
CREATE INDEX IF NOT EXISTS "index_portal_sessions_on_contact_id" ON "portal_sessions" ("contact_id");
CREATE TABLE IF NOT EXISTS "questions" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "user_id" integer NOT NULL, "text" text NOT NULL, "choices" json DEFAULT '[]' NOT NULL, "subject_type" varchar, "subject_id" bigint, "context" text, "asked_by" varchar NOT NULL, "source" varchar NOT NULL, "answer" text, "answered_at" datetime(6), "dismissed_at" datetime(6), "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_questions_on_user_id_and_answered_at_and_dismissed_at" ON "questions" ("user_id", "answered_at", "dismissed_at");
CREATE INDEX IF NOT EXISTS "index_questions_on_user_id" ON "questions" ("user_id");
CREATE TABLE IF NOT EXISTS "requests" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "client_id" integer, "contact_id" integer, "sender_name" varchar, "sender_email" varchar, "subject" varchar NOT NULL, "body" text, "source" varchar NOT NULL, "status" varchar DEFAULT 'open' NOT NULL, "promoted_type" varchar, "promoted_id" bigint, "dismissed_reason" varchar, "received_at" datetime(6) NOT NULL, "triaged_by_id" integer, "triaged_at" datetime(6), "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_requests_on_client_id" ON "requests" ("client_id");
CREATE INDEX IF NOT EXISTS "index_requests_on_contact_id" ON "requests" ("contact_id");
CREATE INDEX IF NOT EXISTS "index_requests_on_status" ON "requests" ("status");
CREATE INDEX IF NOT EXISTS "index_requests_on_triaged_by_id" ON "requests" ("triaged_by_id");
CREATE TABLE IF NOT EXISTS "scope_items" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "agreement_version_id" integer NOT NULL, "description" varchar NOT NULL, "internal_estimate" varchar, "price_cents" integer DEFAULT 0 NOT NULL, "position" integer NOT NULL, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_scope_items_on_agreement_version_id" ON "scope_items" ("agreement_version_id");
CREATE TABLE IF NOT EXISTS "searchable_documents" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "searchable_type" varchar NOT NULL, "searchable_id" varchar NOT NULL, "client_id" varchar, "created_at" datetime(6));
CREATE UNIQUE INDEX IF NOT EXISTS "idx_on_searchable_type_searchable_id_be150b479a" ON "searchable_documents" ("searchable_type", "searchable_id");
CREATE TABLE IF NOT EXISTS "sessions" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "user_id" integer NOT NULL, "ip_address" varchar, "user_agent" varchar, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL, "token_digest" varchar, "csrf" varchar, "last_seen_at" datetime(6));
CREATE INDEX IF NOT EXISTS "index_sessions_on_user_id" ON "sessions" ("user_id");
CREATE TABLE IF NOT EXISTS "settings" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "terminology" json DEFAULT '{}' NOT NULL, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL, "plugin_states" json DEFAULT '{}' NOT NULL, "index_view" varchar DEFAULT 'cards' NOT NULL, "theme" varchar DEFAULT 'system' NOT NULL, "radius" varchar DEFAULT 'fizzy' NOT NULL, "scheme" varchar DEFAULT 'fizzy' NOT NULL, "font" varchar DEFAULT 'system' NOT NULL, "setup_dismissed_at" datetime(6), "mail_from_name" varchar, "mail_from_email" varchar, "latest_release" json DEFAULT '{}' NOT NULL, "release_checked_at" datetime(6), "plugin_releases" json DEFAULT '{}' NOT NULL, "app_host" varchar, "app_protocol" varchar, "seen_host" varchar, "release_check_requested_at" datetime(6), "release_check_error" varchar, "test_email" json DEFAULT '{}' NOT NULL, "requests_email" varchar, "client_agent_approvals" boolean DEFAULT 1 NOT NULL, "inbound_ingress" varchar, "inbound_password" text, "time_zone" varchar, "smtp_address" varchar, "smtp_port" integer, "smtp_username" varchar, "smtp_password" text, "smtp_security" varchar DEFAULT 'starttls' NOT NULL, "prices" boolean DEFAULT 1 NOT NULL, "text_size" varchar DEFAULT 'default' NOT NULL, "require_two_factor" boolean DEFAULT 0 NOT NULL, "custom_css" text, "custom_css_everywhere" boolean DEFAULT 0 NOT NULL, "providers_only" boolean DEFAULT 0 NOT NULL, "ai_enabled" boolean DEFAULT 0 NOT NULL, "ai_provider" varchar, "ai_api_key" varchar, "ai_api_base" varchar, "ai_model" varchar, "ai_fast_model" varchar, "ai_monthly_budget_cents" integer, "ai_portal" boolean DEFAULT 0 NOT NULL, "ai_test" json);
CREATE TABLE IF NOT EXISTS "sign_in_providers" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "key" varchar NOT NULL, "client_id" varchar, "client_secret" varchar, "tenant_id" varchar, "enabled" boolean DEFAULT 0 NOT NULL, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL, "issuer" varchar, "label" varchar);
CREATE UNIQUE INDEX IF NOT EXISTS "index_sign_in_providers_on_key" ON "sign_in_providers" ("key");
CREATE TABLE IF NOT EXISTS "todos" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "engagement_id" integer NOT NULL, "scope_item_id" integer, "title" varchar NOT NULL, "description" text, "status" varchar DEFAULT 'planned' NOT NULL, "owner_id" integer, "due_on" date, "position" integer NOT NULL, "client_visible" boolean DEFAULT 1 NOT NULL, "completed_at" datetime(6), "created_by_id" integer, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL, "board_position" integer NOT NULL);
CREATE INDEX IF NOT EXISTS "index_todos_on_created_by_id" ON "todos" ("created_by_id");
CREATE INDEX IF NOT EXISTS "index_todos_on_engagement_id_and_status_and_position" ON "todos" ("engagement_id", "status", "position");
CREATE INDEX IF NOT EXISTS "index_todos_on_engagement_id" ON "todos" ("engagement_id");
CREATE INDEX IF NOT EXISTS "index_todos_on_owner_id" ON "todos" ("owner_id");
CREATE INDEX IF NOT EXISTS "index_todos_on_scope_item_id" ON "todos" ("scope_item_id");
CREATE INDEX IF NOT EXISTS "index_todos_on_status_and_board_position" ON "todos" ("status", "board_position");
CREATE TABLE IF NOT EXISTS "upgrades" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "created_at" datetime(6) NOT NULL, "external_id" varchar, "external_url" varchar, "finished_at" datetime(6), "from_version" varchar NOT NULL, "message" text, "requested_by_id" integer NOT NULL, "status" varchar DEFAULT 'running' NOT NULL, "to_version" varchar NOT NULL, "updated_at" datetime(6) NOT NULL, "via" varchar NOT NULL);
CREATE INDEX IF NOT EXISTS "index_upgrades_on_requested_by_id" ON "upgrades" ("requested_by_id");
CREATE INDEX IF NOT EXISTS "index_upgrades_on_status" ON "upgrades" ("status");
CREATE TABLE IF NOT EXISTS "users" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "email_address" varchar NOT NULL, "password_digest" varchar NOT NULL, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL, "name" varchar DEFAULT '' NOT NULL, "nav_plugins" json, "role" varchar DEFAULT 'member' NOT NULL, "deactivated_at" datetime(6), "agent_owner_id" integer, "text_size" varchar, "otp_secret" varchar, "otp_enabled_at" datetime(6), "otp_last_used_at" datetime(6), "otp_recovery_codes" json, "passwordless" boolean DEFAULT 0 NOT NULL, "link_signed_in_at" datetime(6));
CREATE UNIQUE INDEX IF NOT EXISTS "index_users_on_agent_owner_id" ON "users" ("agent_owner_id");
CREATE UNIQUE INDEX IF NOT EXISTS "index_users_on_email_address" ON "users" ("email_address");
CREATE TABLE IF NOT EXISTS "webhook_deliveries" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "webhook_endpoint_id" integer NOT NULL, "event_id" integer, "kind" varchar NOT NULL, "attempts" integer DEFAULT 0 NOT NULL, "status_code" integer, "error" varchar, "duration_ms" integer, "delivered_at" datetime(6), "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_webhook_deliveries_on_event_id" ON "webhook_deliveries" ("event_id");
CREATE INDEX IF NOT EXISTS "index_webhook_deliveries_on_webhook_endpoint_id_and_created_at" ON "webhook_deliveries" ("webhook_endpoint_id", "created_at");
CREATE INDEX IF NOT EXISTS "index_webhook_deliveries_on_webhook_endpoint_id" ON "webhook_deliveries" ("webhook_endpoint_id");
CREATE TABLE IF NOT EXISTS "webhook_endpoints" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "url" varchar NOT NULL, "description" varchar, "secret" varchar NOT NULL, "kinds" json, "include_security" boolean DEFAULT 0 NOT NULL, "active" boolean DEFAULT 1 NOT NULL, "failures_in_a_row" integer DEFAULT 0 NOT NULL, "last_delivered_at" datetime(6), "created_by_id" integer, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL);
CREATE INDEX IF NOT EXISTS "index_webhook_endpoints_on_created_by_id" ON "webhook_endpoints" ("created_by_id");
CREATE VIRTUAL TABLE IF NOT EXISTS "searchable_documents_fts" USING fts5(title, content, tokenize='porter');
CREATE UNIQUE INDEX IF NOT EXISTS "index_sessions_on_token_digest" ON "sessions" ("token_digest");
CREATE UNIQUE INDEX IF NOT EXISTS "index_portal_sessions_on_token_digest" ON "portal_sessions" ("token_digest");

-- ---- the Go core's own -------------------------------------------------

-- Plugins: Go packages compiled into the binary. What's installed, what it
-- was granted, whether it's on (internal/plugin).
CREATE TABLE IF NOT EXISTS plugins (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  key varchar NOT NULL UNIQUE,
  name varchar NOT NULL,
  version varchar NOT NULL,
  description text,
  manifest json DEFAULT '{}' NOT NULL,
  granted json DEFAULT '{}' NOT NULL,
  enabled boolean DEFAULT 0 NOT NULL,
  created_at datetime(6) NOT NULL,
  updated_at datetime(6) NOT NULL
);

-- Each plugin's own storage, keyed by its key.
CREATE TABLE IF NOT EXISTS plugin_kv (
  plugin_key varchar NOT NULL,
  key varchar NOT NULL,
  value blob,
  updated_at datetime(6) NOT NULL,
  PRIMARY KEY (plugin_key, key)
);

-- A plugin's notifications (plugin.API.Notify): its words and the app path
-- each opens; a notifications row (source_type PluginNotification) puts it
-- in the person's bell. group_key collapses unread ones into one.
CREATE TABLE IF NOT EXISTS plugin_notifications (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  plugin_key varchar NOT NULL,
  user_id integer NOT NULL,
  from_id integer,
  title varchar NOT NULL,
  body text,
  path varchar NOT NULL,
  source varchar,
  subject varchar,
  group_key varchar,
  created_at datetime(6) NOT NULL,
  updated_at datetime(6) NOT NULL
);
CREATE INDEX IF NOT EXISTS index_plugin_notifications_on_plugin ON plugin_notifications (plugin_key, user_id, group_key);

-- A mention of a plugin's mentionable (plugin_key's mentionable_id, an AI
-- agent): in a note on a record (source 'note', note_id) or relayed from a
-- plugin's own content (source its key, ref its address for replies), by
-- author_id. A reply to it is addressed by its id.
CREATE TABLE IF NOT EXISTS plugin_mentions (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  plugin_key varchar NOT NULL,
  mentionable_id varchar NOT NULL,
  source varchar NOT NULL,
  subject_type varchar,
  subject_id integer,
  note_id integer,
  ref varchar,
  author_id integer NOT NULL,
  created_at datetime(6) NOT NULL
);
-- Tokens a plugin issued to machines it works with (plugin.API.IssueToken,
-- the "machines" grant): only the digest is kept, and a token reaches only
-- its plugin's machine routes.
CREATE TABLE IF NOT EXISTS plugin_machine_tokens (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  plugin_key varchar NOT NULL,
  label varchar NOT NULL,
  token_digest varchar NOT NULL UNIQUE,
  created_at datetime(6) NOT NULL,
  last_used_at datetime(6),
  revoked_at datetime(6)
);
CREATE INDEX IF NOT EXISTS index_plugin_machine_tokens_on_plugin_key ON plugin_machine_tokens (plugin_key);

-- One-time links mailed to a person: a password reset, a sign-in link, an
-- invitation's. Only the digest is kept; a link works once, until it
-- expires. contact_id is a portal contact's sign-in link.
CREATE TABLE IF NOT EXISTS account_tokens (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  user_id integer,
  contact_id integer,
  purpose varchar NOT NULL,
  token_digest varchar NOT NULL UNIQUE,
  new_email varchar,
  expires_at datetime(6) NOT NULL,
  used_at datetime(6),
  created_at datetime(6) NOT NULL
);
CREATE INDEX IF NOT EXISTS index_account_tokens_on_user ON account_tokens (user_id, purpose);
