INSERT INTO permissions(code,description) VALUES
('integrations.read','Read integration diagnostics'),
('integrations.probe','Run external integration diagnostics')
ON CONFLICT(code) DO NOTHING;

INSERT INTO role_permissions(role_id,permission_code)
SELECT role_id,code FROM
  (VALUES ('00000000-0000-4000-8000-000000000001'::uuid),('00000000-0000-4000-8000-000000000002'::uuid)) roles(role_id)
  CROSS JOIN permissions
WHERE code IN ('integrations.read','integrations.probe')
ON CONFLICT DO NOTHING;

ALTER TABLE entitlement_codes
  ADD COLUMN instance_id UUID REFERENCES emby_instances(id) ON DELETE CASCADE;
CREATE INDEX entitlement_codes_instance_idx ON entitlement_codes(instance_id,status,created_at DESC);

CREATE TABLE line_probe_samples (
    id BIGSERIAL PRIMARY KEY,
    line_id UUID NOT NULL REFERENCES line_endpoints(id) ON DELETE CASCADE,
    status VARCHAR(32) NOT NULL CHECK(status IN ('healthy','degraded','unhealthy')),
    http_status INTEGER,
    latency_ms INTEGER,
    error_message VARCHAR(2000),
    checked_by VARCHAR(255) NOT NULL,
    checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX line_probe_samples_line_idx ON line_probe_samples(line_id,checked_at DESC);

CREATE TABLE emby_favorites (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    instance_id UUID NOT NULL REFERENCES emby_instances(id) ON DELETE CASCADE,
    binding_id UUID NOT NULL REFERENCES emby_account_bindings(id) ON DELETE CASCADE,
    media_id UUID REFERENCES media_catalog(id) ON DELETE SET NULL,
    remote_item_id VARCHAR(128) NOT NULL,
    title VARCHAR(500) NOT NULL,
    media_type VARCHAR(40),
    image_tag VARCHAR(255),
    remote_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    desired_favorite BOOLEAN NOT NULL DEFAULT TRUE,
    remote_favorite BOOLEAN NOT NULL DEFAULT TRUE,
    sync_status VARCHAR(20) NOT NULL DEFAULT 'synced' CHECK(sync_status IN ('pending','synced','failed','missing')),
    last_error VARCHAR(2000),
    last_synced_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(binding_id,remote_item_id)
);
CREATE INDEX emby_favorites_account_idx ON emby_favorites(account_id,desired_favorite,updated_at DESC);

CREATE TABLE integration_probe_results (
    id UUID PRIMARY KEY,
    integration VARCHAR(32) NOT NULL CHECK(integration IN ('emby','tmdb','moviepilot','telegram')),
    target VARCHAR(500) NOT NULL,
    status VARCHAR(20) NOT NULL CHECK(status IN ('healthy','unhealthy')),
    latency_ms INTEGER NOT NULL,
    detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_message VARCHAR(2000),
    checked_by VARCHAR(255) NOT NULL,
    checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX integration_probe_results_kind_idx ON integration_probe_results(integration,checked_at DESC);

ALTER TABLE platform_tasks DROP CONSTRAINT IF EXISTS platform_tasks_task_type_check;
ALTER TABLE platform_tasks ADD CONSTRAINT platform_tasks_task_type_check CHECK (task_type IN (
  'emby.provision','emby.sync','emby.reconcile','emby.import','emby.playback_sync',
  'risk.action','risk.revert','media.match','moviepilot.submit',
  'entitlement.sync','emby.favorite','emby.favorite_sync'
));

INSERT INTO dynamic_settings(key,value,value_type,updated_by) VALUES
('lines.probe_timeout_seconds','10'::jsonb,'integer','system:migration'),
('entitlements.sync_interval_seconds','300'::jsonb,'integer','system:migration'),
('favorites.sync_interval_seconds','3600'::jsonb,'integer','system:migration'),
('telegram.api_base_url','"https://api.telegram.org"'::jsonb,'string','system:migration'),
('moviepilot.health_path','"/api/v1/system/setting"'::jsonb,'string','system:migration')
ON CONFLICT(key) DO NOTHING;
INSERT INTO setting_revisions(setting_key,revision,value,value_type,actor,reason)
SELECT key,revision,value,value_type,updated_by,'runtime domain completion default'
FROM dynamic_settings WHERE key IN ('lines.probe_timeout_seconds','entitlements.sync_interval_seconds','favorites.sync_interval_seconds','telegram.api_base_url','moviepilot.health_path')
ON CONFLICT(setting_key,revision) DO NOTHING;
