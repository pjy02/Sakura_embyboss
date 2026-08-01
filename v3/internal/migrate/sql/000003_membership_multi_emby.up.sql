INSERT INTO permissions(code, description) VALUES
('memberships.read','Read membership plans and assignments'),
('memberships.write','Manage membership plans and assignments'),
('invitations.read','Read invitation metadata'),
('invitations.write','Generate and revoke invitations'),
('emby_instances.read','Read Emby instances and health'),
('emby_instances.write','Manage Emby instances'),
('emby_bindings.read','Read Emby users and account bindings'),
('emby_bindings.write','Manage imports, claims and bindings'),
('emby_sync.read','Read Emby synchronization and reconciliation state'),
('emby_sync.write','Enqueue Emby synchronization and reconciliation tasks')
ON CONFLICT(code) DO NOTHING;

INSERT INTO role_permissions(role_id, permission_code)
SELECT '00000000-0000-4000-8000-000000000001'::uuid, code
FROM permissions
WHERE code IN ('memberships.read','memberships.write','invitations.read','invitations.write','emby_instances.read','emby_instances.write','emby_bindings.read','emby_bindings.write','emby_sync.read','emby_sync.write')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions(role_id, permission_code)
SELECT '00000000-0000-4000-8000-000000000002'::uuid, code
FROM permissions
WHERE code IN ('memberships.read','memberships.write','invitations.read','invitations.write','emby_instances.read','emby_instances.write','emby_bindings.read','emby_bindings.write','emby_sync.read','emby_sync.write')
ON CONFLICT DO NOTHING;

INSERT INTO dynamic_settings(key,value,value_type,updated_by) VALUES
('site.code_prefix','"SAKURA"'::jsonb,'string','system:migration'),
('registration.invite_required','true'::jsonb,'boolean','system:migration'),
('emby.sync_interval_seconds','300'::jsonb,'integer','system:migration'),
('emby.reconcile_interval_seconds','900'::jsonb,'integer','system:migration'),
('emby.task_max_attempts','8'::jsonb,'integer','system:migration')
ON CONFLICT(key) DO NOTHING;

INSERT INTO setting_revisions(setting_key,revision,value,value_type,actor,reason)
SELECT d.key,d.revision,d.value,d.value_type,d.updated_by,'phase 3 initial value'
FROM dynamic_settings d
WHERE d.key IN ('site.code_prefix','registration.invite_required','emby.sync_interval_seconds','emby.reconcile_interval_seconds','emby.task_max_attempts')
ON CONFLICT(setting_key,revision) DO NOTHING;

CREATE TABLE membership_plans (
    id UUID PRIMARY KEY,
    code VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL,
    description VARCHAR(1000),
    duration_days INTEGER NOT NULL CHECK (duration_days BETWEEN 1 AND 3650),
    entitlements JSONB NOT NULL DEFAULT '{}'::jsonb,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    revision BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX membership_plans_one_default_idx ON membership_plans(is_default) WHERE is_default;

INSERT INTO membership_plans(id,code,name,description,duration_days,entitlements,enabled,is_default,sort_order)
VALUES('00000000-0000-4000-8000-000000000101','standard','标准会员','默认 Emby 会员方案',30,'{"max_emby_accounts":3,"stream_limit":2}'::jsonb,TRUE,TRUE,100);

CREATE TABLE account_memberships (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    plan_id UUID NOT NULL REFERENCES membership_plans(id),
    status VARCHAR(20) NOT NULL CHECK (status IN ('active','grace','expired','canceled')),
    starts_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    source VARCHAR(32) NOT NULL,
    source_ref VARCHAR(255),
    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX account_memberships_one_current_idx ON account_memberships(account_id) WHERE status IN ('active','grace');
CREATE INDEX account_memberships_expiry_idx ON account_memberships(status,expires_at);

CREATE TABLE invitation_codes (
    id UUID PRIMARY KEY,
    code_hash BYTEA NOT NULL UNIQUE,
    code_prefix VARCHAR(32) NOT NULL,
    code_hint VARCHAR(24) NOT NULL,
    kind VARCHAR(20) NOT NULL CHECK (kind IN ('registration','renewal')),
    plan_id UUID NOT NULL REFERENCES membership_plans(id),
    max_uses INTEGER NOT NULL DEFAULT 1 CHECK (max_uses BETWEEN 1 AND 10000),
    used_count INTEGER NOT NULL DEFAULT 0 CHECK (used_count >= 0),
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active','used','expired','revoked')),
    expires_at TIMESTAMPTZ,
    issued_by VARCHAR(255) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX invitation_codes_status_expiry_idx ON invitation_codes(status,expires_at);

CREATE TABLE invitation_redemptions (
    id UUID PRIMARY KEY,
    invitation_id UUID NOT NULL REFERENCES invitation_codes(id),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    membership_id UUID NOT NULL REFERENCES account_memberships(id),
    idempotency_key VARCHAR(160),
    redeemed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(invitation_id,account_id),
    UNIQUE(idempotency_key)
);

CREATE TABLE emby_instances (
    id UUID PRIMARY KEY,
    name VARCHAR(120) NOT NULL UNIQUE,
    base_url VARCHAR(512) NOT NULL UNIQUE,
    credential_name VARCHAR(120) NOT NULL REFERENCES credentials(name),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    verify_tls BOOLEAN NOT NULL DEFAULT TRUE,
    priority INTEGER NOT NULL DEFAULT 100,
    status VARCHAR(20) NOT NULL DEFAULT 'unknown' CHECK (status IN ('unknown','healthy','degraded','unhealthy','disabled')),
    server_id VARCHAR(128),
    server_version VARCHAR(64),
    last_error VARCHAR(1000),
    last_latency_ms INTEGER,
    last_checked_at TIMESTAMPTZ,
    last_snapshot_at TIMESTAMPTZ,
    revision BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX emby_instances_one_default_idx ON emby_instances(is_default) WHERE is_default;
CREATE INDEX emby_instances_enabled_priority_idx ON emby_instances(enabled,priority);

CREATE TABLE platform_tasks (
    id UUID PRIMARY KEY,
    task_type VARCHAR(40) NOT NULL CHECK (task_type IN ('emby.provision','emby.sync','emby.reconcile','emby.import')),
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','retry','succeeded','failed','dead','canceled')),
    idempotency_key VARCHAR(200) NOT NULL UNIQUE,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    result JSONB NOT NULL DEFAULT '{}'::jsonb,
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 8 CHECK (max_attempts BETWEEN 1 AND 100),
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_owner VARCHAR(120),
    lease_expires_at TIMESTAMPTZ,
    last_error VARCHAR(2000),
    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX platform_tasks_claim_idx ON platform_tasks(status,available_at,created_at);

CREATE TABLE emby_provision_requests (
    id UUID PRIMARY KEY,
    task_id UUID NOT NULL UNIQUE REFERENCES platform_tasks(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    instance_id UUID NOT NULL REFERENCES emby_instances(id),
    membership_id UUID REFERENCES account_memberships(id),
    invitation_id UUID REFERENCES invitation_codes(id),
    requested_username VARCHAR(255) NOT NULL,
    requested_username_normalized VARCHAR(255) NOT NULL,
    password_ciphertext BYTEA NOT NULL,
    password_nonce BYTEA NOT NULL,
    password_key_version INTEGER NOT NULL,
    password_expires_at TIMESTAMPTZ NOT NULL,
    remote_user_id VARCHAR(128),
    preflight_completed BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','provisioning','succeeded','failed','canceled')),
    last_error VARCHAR(1000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(instance_id,requested_username_normalized)
);

CREATE TABLE emby_account_bindings (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    instance_id UUID NOT NULL REFERENCES emby_instances(id),
    remote_user_id VARCHAR(128) NOT NULL,
    remote_username VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended','missing','error','deleted')),
    origin VARCHAR(24) NOT NULL CHECK (origin IN ('provision','remote_import','legacy_import','manual_claim')),
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at TIMESTAMPTZ,
    remote_disabled BOOLEAN,
    remote_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    claimed_at TIMESTAMPTZ,
    last_synced_at TIMESTAMPTZ,
    last_error VARCHAR(1000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(instance_id,remote_user_id),
    UNIQUE(account_id,instance_id)
);
CREATE INDEX emby_account_bindings_account_idx ON emby_account_bindings(account_id,status);
CREATE INDEX emby_account_bindings_instance_idx ON emby_account_bindings(instance_id,status);

CREATE TABLE remote_emby_users (
    id UUID PRIMARY KEY,
    instance_id UUID NOT NULL REFERENCES emby_instances(id) ON DELETE CASCADE,
    remote_user_id VARCHAR(128) NOT NULL,
    username VARCHAR(255) NOT NULL,
    username_normalized VARCHAR(255) NOT NULL,
    disabled BOOLEAN NOT NULL DEFAULT FALSE,
    claim_status VARCHAR(20) NOT NULL DEFAULT 'unclaimed' CHECK (claim_status IN ('unclaimed','claimed','ignored','missing')),
    binding_id UUID REFERENCES emby_account_bindings(id) ON DELETE SET NULL,
    snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    missing_since TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(instance_id,remote_user_id)
);
CREATE INDEX remote_emby_users_claim_idx ON remote_emby_users(instance_id,claim_status,username_normalized);

CREATE TABLE emby_claim_tokens (
    id UUID PRIMARY KEY,
    remote_user_id UUID NOT NULL REFERENCES remote_emby_users(id) ON DELETE CASCADE,
    token_hash BYTEA NOT NULL UNIQUE,
    token_hint VARCHAR(24) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active','used','expired','revoked')),
    expires_at TIMESTAMPTZ NOT NULL,
    issued_by VARCHAR(255) NOT NULL,
    used_by_account_id UUID REFERENCES accounts(id),
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE remote_state_snapshots (
    id BIGSERIAL PRIMARY KEY,
    instance_id UUID NOT NULL REFERENCES emby_instances(id) ON DELETE CASCADE,
    task_id UUID REFERENCES platform_tasks(id) ON DELETE SET NULL,
    snapshot_kind VARCHAR(20) NOT NULL CHECK (snapshot_kind IN ('probe','sync','reconcile','import')),
    status VARCHAR(20) NOT NULL,
    remote_user_count INTEGER NOT NULL DEFAULT 0,
    bound_user_count INTEGER NOT NULL DEFAULT 0,
    unclaimed_user_count INTEGER NOT NULL DEFAULT 0,
    missing_user_count INTEGER NOT NULL DEFAULT 0,
    changes JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_message VARCHAR(2000),
    captured_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX remote_state_snapshots_instance_time_idx ON remote_state_snapshots(instance_id,captured_at DESC);
