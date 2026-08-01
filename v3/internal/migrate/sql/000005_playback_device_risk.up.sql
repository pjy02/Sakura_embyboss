INSERT INTO permissions(code,description) VALUES
('playback.read','Read current playback and playback history'),
('devices.read','Read device profiles and access decisions'),
('devices.write','Manage device allowlists, denylists and custom rules'),
('risk.read','Read risk rules, events and action traces'),
('risk.write','Manage risk rules and acknowledge or reverse events')
ON CONFLICT(code) DO NOTHING;

INSERT INTO role_permissions(role_id,permission_code)
SELECT role_id,code FROM
  (VALUES ('00000000-0000-4000-8000-000000000001'::uuid),('00000000-0000-4000-8000-000000000002'::uuid)) roles(role_id)
  CROSS JOIN permissions
WHERE code IN ('playback.read','devices.read','devices.write','risk.read','risk.write')
ON CONFLICT DO NOTHING;

INSERT INTO dynamic_settings(key,value,value_type,updated_by) VALUES
('playback.sync_interval_seconds','30'::jsonb,'integer','system:migration'),
('playback.history_retention_days','180'::jsonb,'integer','system:migration'),
('risk.max_instance_failures','3'::jsonb,'integer','system:migration'),
('risk.circuit_cooldown_seconds','120'::jsonb,'integer','system:migration'),
('risk.telegram_alert_account_ids','[]'::jsonb,'array','system:migration'),
('risk.notify_affected_account','true'::jsonb,'boolean','system:migration')
ON CONFLICT(key) DO NOTHING;

INSERT INTO setting_revisions(setting_key,revision,value,value_type,actor,reason)
SELECT key,revision,value,value_type,updated_by,'phase 5 initial value'
FROM dynamic_settings
WHERE key IN ('playback.sync_interval_seconds','playback.history_retention_days','risk.max_instance_failures','risk.circuit_cooldown_seconds','risk.telegram_alert_account_ids','risk.notify_affected_account')
ON CONFLICT(setting_key,revision) DO NOTHING;

CREATE TABLE emby_instance_runtime_health (
    instance_id UUID PRIMARY KEY REFERENCES emby_instances(id) ON DELETE CASCADE,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    circuit_open_until TIMESTAMPTZ,
    last_success_at TIMESTAMPTZ,
    last_failure_at TIMESTAMPTZ,
    last_error VARCHAR(2000),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE device_access_rules (
    id UUID PRIMARY KEY,
    instance_id UUID REFERENCES emby_instances(id) ON DELETE CASCADE,
    name VARCHAR(120) NOT NULL,
    description VARCHAR(500),
    decision VARCHAR(12) NOT NULL CHECK(decision IN ('allow','deny')),
    match_field VARCHAR(32) NOT NULL CHECK(match_field IN ('client_name','device_name','device_id','platform','remote_ip')),
    match_operator VARCHAR(16) NOT NULL CHECK(match_operator IN ('exact','contains','prefix','regex')),
    match_value VARCHAR(255) NOT NULL,
    action VARCHAR(24) NOT NULL DEFAULT 'stop_session' CHECK(action IN ('none','stop_session','disable_user')),
    observation_mode BOOLEAN NOT NULL DEFAULT FALSE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    built_in BOOLEAN NOT NULL DEFAULT FALSE,
    priority INTEGER NOT NULL DEFAULT 100 CHECK(priority BETWEEN 0 AND 100000),
    revision BIGINT NOT NULL DEFAULT 1,
    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX device_access_rules_eval_idx ON device_access_rules(enabled,instance_id,priority,id);

INSERT INTO device_access_rules(id,name,description,decision,match_field,match_operator,match_value,action,enabled,built_in,priority,created_by)
SELECT gen_random_uuid(),'Built-in allow: '||client,'Built-in known client allowlist','allow','client_name','exact',client,'none',TRUE,TRUE,10,'system:migration'
FROM unnest(ARRAY['Emby for iOS','Emby for Android','Emby Theater','Emby for macOS','Emby for Apple TV','Infuse-Direct','SenPlayer','AfuseKt','Conflux','Yamby','Xfuse','Terminus Player','Reflix','Forward','Hills','Tsukimi','iPlay','Filebox','Emby Web','Emby Windows','Filebar']) client;

CREATE TABLE risk_rules (
    id UUID PRIMARY KEY,
    instance_id UUID REFERENCES emby_instances(id) ON DELETE CASCADE,
    code VARCHAR(80) NOT NULL,
    name VARCHAR(120) NOT NULL,
    description VARCHAR(500),
    rule_type VARCHAR(40) NOT NULL CHECK(rule_type IN ('concurrent_streams','transcoding','bitrate','remote_ip','custom')),
    condition JSONB NOT NULL DEFAULT '{}'::jsonb,
    severity VARCHAR(12) NOT NULL CHECK(severity IN ('low','medium','high','critical')),
    action VARCHAR(24) NOT NULL DEFAULT 'none' CHECK(action IN ('none','stop_session','disable_user')),
    observation_mode BOOLEAN NOT NULL DEFAULT TRUE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    cooldown_seconds INTEGER NOT NULL DEFAULT 300 CHECK(cooldown_seconds BETWEEN 0 AND 604800),
    revision BIGINT NOT NULL DEFAULT 1,
    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(instance_id,code)
);
CREATE UNIQUE INDEX risk_rules_global_code_uidx ON risk_rules(code) WHERE instance_id IS NULL;
CREATE INDEX risk_rules_eval_idx ON risk_rules(enabled,instance_id,rule_type,id);

CREATE TABLE playback_sessions (
    id UUID PRIMARY KEY,
    instance_id UUID NOT NULL REFERENCES emby_instances(id) ON DELETE CASCADE,
    binding_id UUID REFERENCES emby_account_bindings(id) ON DELETE SET NULL,
    account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    remote_session_id VARCHAR(160) NOT NULL,
    playback_key VARCHAR(360) NOT NULL,
    remote_user_id VARCHAR(128),
    remote_username VARCHAR(255),
    item_id VARCHAR(128) NOT NULL,
    item_name VARCHAR(500) NOT NULL,
    item_type VARCHAR(80),
    series_name VARCHAR(500),
    client_name VARCHAR(255),
    device_name VARCHAR(255),
    device_id VARCHAR(255),
    platform VARCHAR(120),
    remote_ip VARCHAR(128),
    play_method VARCHAR(40),
    transcoding BOOLEAN NOT NULL DEFAULT FALSE,
    bitrate BIGINT,
    position_ticks BIGINT NOT NULL DEFAULT 0,
    runtime_ticks BIGINT,
    paused BOOLEAN NOT NULL DEFAULT FALSE,
    device_decision VARCHAR(16) NOT NULL DEFAULT 'unmatched' CHECK(device_decision IN ('allowed','denied','unmatched')),
    matched_device_rule_id UUID REFERENCES device_access_rules(id) ON DELETE SET NULL,
    raw_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(instance_id,remote_session_id)
);
CREATE INDEX playback_sessions_instance_idx ON playback_sessions(instance_id,last_seen_at DESC);
CREATE INDEX playback_sessions_account_idx ON playback_sessions(account_id,last_seen_at DESC);

CREATE TABLE playback_history (
    id UUID PRIMARY KEY,
    instance_id UUID NOT NULL REFERENCES emby_instances(id) ON DELETE CASCADE,
    binding_id UUID REFERENCES emby_account_bindings(id) ON DELETE SET NULL,
    account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    remote_session_id VARCHAR(160) NOT NULL,
    playback_key VARCHAR(360) NOT NULL,
    remote_user_id VARCHAR(128),
    remote_username VARCHAR(255),
    item_id VARCHAR(128) NOT NULL,
    item_name VARCHAR(500) NOT NULL,
    item_type VARCHAR(80),
    series_name VARCHAR(500),
    client_name VARCHAR(255),
    device_name VARCHAR(255),
    device_id VARCHAR(255),
    platform VARCHAR(120),
    remote_ip VARCHAR(128),
    play_method VARCHAR(40),
    transcoding BOOLEAN NOT NULL DEFAULT FALSE,
    peak_bitrate BIGINT,
    max_position_ticks BIGINT NOT NULL DEFAULT 0,
    runtime_ticks BIGINT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at TIMESTAMPTZ,
    raw_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    UNIQUE(instance_id,playback_key)
);
CREATE INDEX playback_history_instance_time_idx ON playback_history(instance_id,started_at DESC);
CREATE INDEX playback_history_account_time_idx ON playback_history(account_id,started_at DESC);

CREATE TABLE device_profiles (
    id UUID PRIMARY KEY,
    instance_id UUID NOT NULL REFERENCES emby_instances(id) ON DELETE CASCADE,
    binding_id UUID REFERENCES emby_account_bindings(id) ON DELETE SET NULL,
    account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    remote_user_id VARCHAR(128) NOT NULL DEFAULT '',
    device_key VARCHAR(255) NOT NULL,
    device_id VARCHAR(255),
    device_name VARCHAR(255),
    client_name VARCHAR(255),
    platform VARCHAR(120),
    first_ip VARCHAR(128),
    last_ip VARCHAR(128),
    session_count BIGINT NOT NULL DEFAULT 0,
    transcode_count BIGINT NOT NULL DEFAULT 0,
    maximum_bitrate BIGINT,
    access_decision VARCHAR(16) NOT NULL DEFAULT 'unmatched' CHECK(access_decision IN ('allowed','denied','unmatched')),
    matched_rule_id UUID REFERENCES device_access_rules(id) ON DELETE SET NULL,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    UNIQUE(instance_id,remote_user_id,device_key)
);
CREATE INDEX device_profiles_account_idx ON device_profiles(account_id,last_seen_at DESC);
CREATE INDEX device_profiles_decision_idx ON device_profiles(access_decision,last_seen_at DESC);

CREATE TABLE risk_events (
    id UUID PRIMARY KEY,
    instance_id UUID NOT NULL REFERENCES emby_instances(id) ON DELETE CASCADE,
    rule_id UUID REFERENCES risk_rules(id) ON DELETE SET NULL,
    device_rule_id UUID REFERENCES device_access_rules(id) ON DELETE SET NULL,
    playback_session_id UUID REFERENCES playback_sessions(id) ON DELETE SET NULL,
    binding_id UUID REFERENCES emby_account_bindings(id) ON DELETE SET NULL,
    account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    dedupe_key VARCHAR(500) NOT NULL UNIQUE,
    source VARCHAR(32) NOT NULL CHECK(source IN ('device_rule','risk_rule','manual')),
    severity VARCHAR(12) NOT NULL CHECK(severity IN ('low','medium','high','critical')),
    title VARCHAR(255) NOT NULL,
    reason VARCHAR(1000) NOT NULL,
    evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    rule_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    observation_mode BOOLEAN NOT NULL DEFAULT TRUE,
    recommended_action VARCHAR(24) NOT NULL DEFAULT 'none',
    status VARCHAR(24) NOT NULL DEFAULT 'open' CHECK(status IN ('open','acknowledged','resolved','false_positive')),
    disposition_reason VARCHAR(1000),
    disposition_by VARCHAR(255),
    disposition_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX risk_events_status_time_idx ON risk_events(status,severity,created_at DESC);
CREATE INDEX risk_events_instance_time_idx ON risk_events(instance_id,created_at DESC);
CREATE INDEX risk_events_account_time_idx ON risk_events(account_id,created_at DESC);

CREATE TABLE risk_actions (
    id UUID PRIMARY KEY,
    event_id UUID NOT NULL REFERENCES risk_events(id) ON DELETE CASCADE,
    instance_id UUID NOT NULL REFERENCES emby_instances(id) ON DELETE CASCADE,
    task_id UUID REFERENCES platform_tasks(id) ON DELETE SET NULL,
    action_type VARCHAR(24) NOT NULL CHECK(action_type IN ('stop_session','disable_user')),
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','running','succeeded','failed','canceled','revert_pending','reverting','reverted','revert_failed')),
    reason VARCHAR(1000) NOT NULL,
    remote_session_id VARCHAR(160),
    remote_user_id VARCHAR(128),
    before_state JSONB NOT NULL DEFAULT '{}'::jsonb,
    after_state JSONB NOT NULL DEFAULT '{}'::jsonb,
    result JSONB NOT NULL DEFAULT '{}'::jsonb,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error VARCHAR(2000),
    executed_at TIMESTAMPTZ,
    reverted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(event_id,action_type)
);
CREATE INDEX risk_actions_status_idx ON risk_actions(status,created_at);

CREATE TABLE risk_event_timeline (
    id BIGSERIAL PRIMARY KEY,
    event_id UUID NOT NULL REFERENCES risk_events(id) ON DELETE CASCADE,
    event_type VARCHAR(40) NOT NULL,
    actor VARCHAR(255) NOT NULL,
    reason VARCHAR(1000),
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX risk_event_timeline_event_idx ON risk_event_timeline(event_id,id);
