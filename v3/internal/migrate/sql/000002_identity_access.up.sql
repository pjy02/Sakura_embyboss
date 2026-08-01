CREATE TABLE accounts (
    id UUID PRIMARY KEY,
    display_name VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('pending','active','suspended','banned','deleted')),
    revision BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX accounts_status_created_idx ON accounts(status, created_at DESC);

CREATE TABLE account_identities (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    kind VARCHAR(24) NOT NULL CHECK (kind IN ('local','telegram','emby','legacy')),
    subject VARCHAR(255) NOT NULL,
    username VARCHAR(255),
    username_normalized VARCHAR(255),
    password_hash TEXT,
    verified_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    disabled BOOLEAN NOT NULL DEFAULT FALSE,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(kind, subject)
);

CREATE UNIQUE INDEX account_identities_kind_username_idx
    ON account_identities(kind, username_normalized) WHERE username_normalized IS NOT NULL;
CREATE UNIQUE INDEX account_identities_single_primary_idx
    ON account_identities(account_id, kind) WHERE kind IN ('local','telegram');

CREATE TABLE sessions (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    token_hash BYTEA NOT NULL UNIQUE,
    csrf_hash BYTEA NOT NULL,
    user_agent VARCHAR(512),
    ip_address VARCHAR(64),
    expires_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX sessions_account_active_idx ON sessions(account_id, expires_at) WHERE revoked_at IS NULL;

CREATE TABLE permissions (
    code VARCHAR(100) PRIMARY KEY,
    description VARCHAR(500) NOT NULL
);
CREATE TABLE roles (
    id UUID PRIMARY KEY,
    code VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL,
    system BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE role_permissions (
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_code VARCHAR(100) NOT NULL REFERENCES permissions(code) ON DELETE CASCADE,
    PRIMARY KEY(role_id, permission_code)
);
CREATE TABLE account_roles (
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    assigned_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(account_id, role_id)
);

INSERT INTO permissions(code, description) VALUES
('accounts.read','Read accounts'),('accounts.write','Edit accounts'),('accounts.lifecycle','Change account lifecycle'),
('roles.read','Read roles'),('roles.write','Manage roles'),
('settings.read','Read dynamic settings'),('settings.write','Update dynamic settings'),('settings.rollback','Rollback settings'),
('credentials.read','Read credential metadata'),('credentials.write','Manage credentials'),
('audit.read','Read audit logs'),('api_clients.read','Read API clients'),('api_clients.write','Manage API clients');

INSERT INTO roles(id, code, name, system) VALUES
('00000000-0000-4000-8000-000000000001','owner','Owner',TRUE),
('00000000-0000-4000-8000-000000000002','admin','Administrator',TRUE),
('00000000-0000-4000-8000-000000000003','user','User',TRUE);
INSERT INTO role_permissions(role_id, permission_code)
SELECT '00000000-0000-4000-8000-000000000001'::uuid, code FROM permissions;
INSERT INTO role_permissions(role_id, permission_code)
SELECT '00000000-0000-4000-8000-000000000002'::uuid, code FROM permissions
WHERE code NOT IN ('roles.write','credentials.write','api_clients.write');

CREATE TABLE api_clients (
    id UUID PRIMARY KEY,
    name VARCHAR(120) NOT NULL,
    token_prefix VARCHAR(20) NOT NULL,
    token_hash BYTEA NOT NULL UNIQUE,
    scopes TEXT[] NOT NULL DEFAULT '{}',
    active BOOLEAN NOT NULL DEFAULT TRUE,
    expires_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_by UUID REFERENCES accounts(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);

CREATE TABLE dynamic_settings (
    key VARCHAR(160) PRIMARY KEY,
    value JSONB NOT NULL,
    value_type VARCHAR(20) NOT NULL CHECK (value_type IN ('boolean','integer','string','object','array')),
    revision BIGINT NOT NULL DEFAULT 1,
    updated_by VARCHAR(255) NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE setting_revisions (
    id BIGSERIAL PRIMARY KEY,
    setting_key VARCHAR(160) NOT NULL,
    revision BIGINT NOT NULL,
    value JSONB NOT NULL,
    value_type VARCHAR(20) NOT NULL CHECK (value_type IN ('boolean','integer','string','object','array')),
    actor VARCHAR(255) NOT NULL,
    reason VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(setting_key, revision)
);
INSERT INTO dynamic_settings(key,value,value_type,updated_by) VALUES
('auth.local_registration_enabled','true'::jsonb,'boolean','system:migration'),
('auth.session_ttl_hours','168'::jsonb,'integer','system:migration');
INSERT INTO setting_revisions(setting_key,revision,value,value_type,actor,reason)
SELECT key,revision,value,value_type,updated_by,'initial value' FROM dynamic_settings;

CREATE TABLE credentials (
    id UUID PRIMARY KEY,
    name VARCHAR(120) NOT NULL UNIQUE,
    kind VARCHAR(64) NOT NULL,
    ciphertext BYTEA NOT NULL,
    nonce BYTEA NOT NULL,
    key_version INTEGER NOT NULL DEFAULT 1,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE audit_logs (
    id BIGSERIAL PRIMARY KEY,
    actor_kind VARCHAR(32) NOT NULL,
    actor_id VARCHAR(255) NOT NULL,
    action VARCHAR(160) NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    resource_id VARCHAR(255),
    request_id VARCHAR(100),
    ip_address VARCHAR(64),
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX audit_logs_created_idx ON audit_logs(created_at DESC);
CREATE INDEX audit_logs_resource_idx ON audit_logs(resource_type,resource_id);

CREATE TABLE account_lifecycle_events (
    id BIGSERIAL PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id),
    from_status VARCHAR(20) NOT NULL,
    to_status VARCHAR(20) NOT NULL,
    reason VARCHAR(500) NOT NULL,
    actor VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE telegram_link_requests (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    code_hash BYTEA NOT NULL UNIQUE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','confirmed','expired','canceled')),
    telegram_user_id BIGINT,
    telegram_username VARCHAR(255),
    expires_at TIMESTAMPTZ NOT NULL,
    confirmed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE legacy_import_runs (
    id UUID PRIMARY KEY,
    source_fingerprint VARCHAR(128) NOT NULL,
    mode VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL,
    summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);
CREATE TABLE legacy_account_mappings (
    source_kind VARCHAR(32) NOT NULL,
    source_key VARCHAR(255) NOT NULL,
    account_id UUID NOT NULL REFERENCES accounts(id),
    import_run_id UUID REFERENCES legacy_import_runs(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(source_kind, source_key)
);
