INSERT INTO permissions(code,description) VALUES
('media.read','Read TMDB catalog and Emby media matches'),
('media.write','Refresh media metadata and matching tasks'),
('media_requests.read','Read media requests and download state'),
('media_requests.write','Moderate requests and submit MoviePilot jobs'),
('tickets.read','Read support tickets including internal notes'),
('tickets.write','Reply, add internal notes and change ticket state'),
('reviews.read','Read reviews and moderation state'),
('reviews.write','Moderate reviews'),
('broadcasts.read','Read broadcast delivery progress'),
('broadcasts.write','Create and control broadcasts'),
('automations.read','Read automation rules and executions'),
('automations.write','Manage automation rules')
ON CONFLICT(code) DO NOTHING;

INSERT INTO role_permissions(role_id,permission_code)
SELECT role_id,code FROM
  (VALUES ('00000000-0000-4000-8000-000000000001'::uuid),('00000000-0000-4000-8000-000000000002'::uuid)) roles(role_id)
  CROSS JOIN permissions
WHERE code IN ('media.read','media.write','media_requests.read','media_requests.write','tickets.read','tickets.write','reviews.read','reviews.write','broadcasts.read','broadcasts.write','automations.read','automations.write')
ON CONFLICT DO NOTHING;

INSERT INTO dynamic_settings(key,value,value_type,updated_by) VALUES
('tmdb.api_base_url','"https://api.themoviedb.org"'::jsonb,'string','system:migration'),
('tmdb.credential_name','"tmdb.api_token"'::jsonb,'string','system:migration'),
('tmdb.language','"zh-CN"'::jsonb,'string','system:migration'),
('moviepilot.api_base_url','"http://moviepilot:3000"'::jsonb,'string','system:migration'),
('moviepilot.credential_name','"moviepilot.api_token"'::jsonb,'string','system:migration'),
('moviepilot.search_path','"/api/v1/search/title"'::jsonb,'string','system:migration'),
('moviepilot.submit_path','"/api/v1/download/add"'::jsonb,'string','system:migration'),
('media.auto_match_enabled','true'::jsonb,'boolean','system:migration'),
('reviews.require_moderation','true'::jsonb,'boolean','system:migration'),
('tickets.auto_close_days','14'::jsonb,'integer','system:migration')
ON CONFLICT(key) DO NOTHING;

INSERT INTO setting_revisions(setting_key,revision,value,value_type,actor,reason)
SELECT key,revision,value,value_type,updated_by,'phase 6 initial value'
FROM dynamic_settings
WHERE key IN ('tmdb.api_base_url','tmdb.credential_name','tmdb.language','moviepilot.api_base_url','moviepilot.credential_name','moviepilot.search_path','moviepilot.submit_path','media.auto_match_enabled','reviews.require_moderation','tickets.auto_close_days')
ON CONFLICT(setting_key,revision) DO NOTHING;

CREATE TABLE media_catalog (
    id UUID PRIMARY KEY,
    provider VARCHAR(24) NOT NULL DEFAULT 'tmdb',
    external_id BIGINT NOT NULL,
    media_type VARCHAR(16) NOT NULL CHECK(media_type IN ('movie','tv')),
    title VARCHAR(500) NOT NULL,
    original_title VARCHAR(500),
    overview TEXT,
    release_date DATE,
    poster_path VARCHAR(500),
    backdrop_path VARCHAR(500),
    popularity DOUBLE PRECISION NOT NULL DEFAULT 0,
    vote_average DOUBLE PRECISION NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_refreshed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(provider,external_id,media_type)
);
CREATE INDEX media_catalog_title_idx ON media_catalog(LOWER(title));
CREATE INDEX media_catalog_popularity_idx ON media_catalog(popularity DESC);

CREATE TABLE media_matches (
    id UUID PRIMARY KEY,
    media_id UUID NOT NULL REFERENCES media_catalog(id) ON DELETE CASCADE,
    instance_id UUID NOT NULL REFERENCES emby_instances(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL CHECK(status IN ('pending','matched','not_found','failed')),
    remote_item_id VARCHAR(128),
    remote_title VARCHAR(500),
    remote_item_type VARCHAR(80),
    remote_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_error VARCHAR(2000),
    matched_at TIMESTAMPTZ,
    last_checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(media_id,instance_id)
);
CREATE INDEX media_matches_instance_status_idx ON media_matches(instance_id,status,updated_at);

CREATE TABLE media_requests (
    id UUID PRIMARY KEY,
    request_no VARCHAR(40) NOT NULL UNIQUE,
    media_id UUID NOT NULL REFERENCES media_catalog(id),
    requested_by UUID NOT NULL REFERENCES accounts(id),
    status VARCHAR(24) NOT NULL DEFAULT 'requested' CHECK(status IN ('requested','approved','queued','downloading','completed','rejected','canceled')),
    priority INTEGER NOT NULL DEFAULT 0 CHECK(priority BETWEEN 0 AND 1000),
    note VARCHAR(1000),
    subscriber_count INTEGER NOT NULL DEFAULT 1,
    duplicate_of_id UUID REFERENCES media_requests(id),
    resolution_reason VARCHAR(1000),
    resolved_by VARCHAR(255),
    resolved_at TIMESTAMPTZ,
    revision BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX media_requests_active_media_uidx ON media_requests(media_id) WHERE status IN ('requested','approved','queued','downloading');
CREATE INDEX media_requests_status_time_idx ON media_requests(status,created_at DESC);

CREATE TABLE media_request_subscriptions (
    request_id UUID NOT NULL REFERENCES media_requests(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    note VARCHAR(1000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(request_id,account_id)
);
CREATE INDEX media_request_subscriptions_account_idx ON media_request_subscriptions(account_id,created_at DESC);

CREATE TABLE media_request_events (
    id BIGSERIAL PRIMARY KEY,
    request_id UUID NOT NULL REFERENCES media_requests(id) ON DELETE CASCADE,
    event_type VARCHAR(40) NOT NULL,
    from_status VARCHAR(24),
    to_status VARCHAR(24),
    actor VARCHAR(255) NOT NULL,
    reason VARCHAR(1000),
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX media_request_events_request_idx ON media_request_events(request_id,id);

CREATE TABLE moviepilot_jobs (
    id UUID PRIMARY KEY,
    media_id UUID NOT NULL REFERENCES media_catalog(id),
    request_id UUID REFERENCES media_requests(id) ON DELETE SET NULL,
    task_id UUID REFERENCES platform_tasks(id) ON DELETE SET NULL,
    idempotency_key VARCHAR(255) NOT NULL UNIQUE,
    status VARCHAR(24) NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','submitting','submitted','downloading','completed','failed','canceled')),
    external_job_id VARCHAR(255),
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    result JSONB NOT NULL DEFAULT '{}'::jsonb,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error VARCHAR(2000),
    submitted_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX moviepilot_jobs_active_media_uidx ON moviepilot_jobs(media_id) WHERE status IN ('pending','submitting','submitted','downloading','completed');
CREATE UNIQUE INDEX moviepilot_jobs_external_uidx ON moviepilot_jobs(external_job_id) WHERE external_job_id IS NOT NULL;

CREATE TABLE support_tickets (
    id UUID PRIMARY KEY,
    ticket_no VARCHAR(40) NOT NULL UNIQUE,
    account_id UUID NOT NULL REFERENCES accounts(id),
    subject VARCHAR(200) NOT NULL,
    category VARCHAR(40) NOT NULL,
    priority VARCHAR(12) NOT NULL DEFAULT 'normal' CHECK(priority IN ('low','normal','high','urgent')),
    status VARCHAR(20) NOT NULL DEFAULT 'open' CHECK(status IN ('open','waiting_user','waiting_staff','resolved','closed')),
    assigned_to UUID REFERENCES accounts(id) ON DELETE SET NULL,
    last_public_reply_at TIMESTAMPTZ,
    last_internal_note_at TIMESTAMPTZ,
    resolved_at TIMESTAMPTZ,
    closed_at TIMESTAMPTZ,
    revision BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX support_tickets_account_idx ON support_tickets(account_id,updated_at DESC);
CREATE INDEX support_tickets_status_idx ON support_tickets(status,priority,updated_at DESC);

CREATE TABLE ticket_messages (
    id UUID PRIMARY KEY,
    ticket_id UUID NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE,
    author_account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    author_label VARCHAR(255) NOT NULL,
    body VARCHAR(8000) NOT NULL,
    is_internal BOOLEAN NOT NULL DEFAULT FALSE,
    attachments JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX ticket_messages_public_idx ON ticket_messages(ticket_id,is_internal,created_at,id);

CREATE TABLE media_reviews (
    id UUID PRIMARY KEY,
    media_id UUID NOT NULL REFERENCES media_catalog(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    rating SMALLINT NOT NULL CHECK(rating BETWEEN 1 AND 10),
    title VARCHAR(200),
    body VARCHAR(6000) NOT NULL,
    contains_spoilers BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected','hidden')),
    moderation_reason VARCHAR(1000),
    moderated_by VARCHAR(255),
    moderated_at TIMESTAMPTZ,
    revision BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(media_id,account_id)
);
CREATE INDEX media_reviews_public_idx ON media_reviews(media_id,status,created_at DESC);
CREATE INDEX media_reviews_moderation_idx ON media_reviews(status,created_at);

CREATE TABLE notification_preferences (
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    event_key VARCHAR(80) NOT NULL,
    channel VARCHAR(20) NOT NULL CHECK(channel IN ('in_app','telegram')),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(account_id,event_key,channel)
);

CREATE TABLE broadcasts (
    id UUID PRIMARY KEY,
    batch_operation_id UUID NOT NULL UNIQUE REFERENCES batch_operations(id) ON DELETE CASCADE,
    title VARCHAR(160) NOT NULL,
    body VARCHAR(4000) NOT NULL,
    event_key VARCHAR(80) NOT NULL,
    channel VARCHAR(20) NOT NULL CHECK(channel IN ('in_app','telegram')),
    target_spec JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE automation_rules (
    id UUID PRIMARY KEY,
    code VARCHAR(80) NOT NULL UNIQUE,
    name VARCHAR(160) NOT NULL,
    description VARCHAR(500),
    trigger_event VARCHAR(80) NOT NULL,
    conditions JSONB NOT NULL DEFAULT '{}'::jsonb,
    actions JSONB NOT NULL DEFAULT '[]'::jsonb,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    priority INTEGER NOT NULL DEFAULT 100 CHECK(priority BETWEEN 0 AND 100000),
    revision BIGINT NOT NULL DEFAULT 1,
    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX automation_rules_trigger_idx ON automation_rules(enabled,trigger_event,priority,id);

CREATE TABLE automation_events (
    id UUID PRIMARY KEY,
    event_key VARCHAR(255) NOT NULL UNIQUE,
    event_type VARCHAR(80) NOT NULL,
    subject_type VARCHAR(40) NOT NULL,
    subject_id VARCHAR(128) NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','running','succeeded','failed')),
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    lease_owner VARCHAR(120),
    lease_expires_at TIMESTAMPTZ,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error VARCHAR(2000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX automation_events_pending_idx ON automation_events(status,available_at,created_at);

CREATE TABLE automation_executions (
    id BIGSERIAL PRIMARY KEY,
    event_id UUID NOT NULL REFERENCES automation_events(id) ON DELETE CASCADE,
    rule_id UUID NOT NULL REFERENCES automation_rules(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL CHECK(status IN ('succeeded','failed','skipped')),
    result JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_message VARCHAR(2000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(event_id,rule_id)
);
