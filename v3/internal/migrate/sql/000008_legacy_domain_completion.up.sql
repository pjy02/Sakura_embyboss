INSERT INTO permissions(code,description) VALUES
('entitlements.read','Read entitlement codes and account grants'),
('entitlements.write','Manage entitlement codes and account grants'),
('lines.read','Read line endpoints'),
('lines.write','Manage line endpoints'),
('reviews.interact','React to and report media reviews')
ON CONFLICT(code) DO NOTHING;

INSERT INTO role_permissions(role_id,permission_code)
SELECT role_id,code FROM
  (VALUES ('00000000-0000-4000-8000-000000000001'::uuid),('00000000-0000-4000-8000-000000000002'::uuid)) roles(role_id)
  CROSS JOIN permissions
WHERE code IN ('entitlements.read','entitlements.write','lines.read','lines.write','reviews.interact')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions(role_id,permission_code)
SELECT '00000000-0000-4000-8000-000000000003'::uuid,code
FROM permissions WHERE code='reviews.interact'
ON CONFLICT DO NOTHING;

CREATE TABLE entitlement_codes (
    id UUID PRIMARY KEY,
    code_hash BYTEA NOT NULL UNIQUE,
    code_prefix VARCHAR(16) NOT NULL,
    code_hint VARCHAR(32) NOT NULL,
    resource_kind VARCHAR(32) NOT NULL DEFAULT 'emby_library',
    resource_key VARCHAR(160) NOT NULL,
    duration_days INTEGER NOT NULL CHECK(duration_days BETWEEN 1 AND 3650),
    status VARCHAR(20) NOT NULL DEFAULT 'available' CHECK(status IN ('available','reserved','redeemed','expired','revoked')),
    reserved_by UUID REFERENCES accounts(id) ON DELETE SET NULL,
    reservation_token VARCHAR(80),
    reserved_at TIMESTAMPTZ,
    issued_by VARCHAR(255) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX entitlement_codes_status_idx ON entitlement_codes(status,created_at DESC);
CREATE INDEX entitlement_codes_resource_idx ON entitlement_codes(resource_kind,resource_key,status);

CREATE TABLE account_entitlements (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    instance_id UUID REFERENCES emby_instances(id) ON DELETE CASCADE,
    binding_id UUID REFERENCES emby_account_bindings(id) ON DELETE SET NULL,
    resource_kind VARCHAR(32) NOT NULL DEFAULT 'emby_library',
    resource_key VARCHAR(160) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK(status IN ('active','expired','revoked')),
    source_code_id UUID REFERENCES entitlement_codes(id) ON DELETE SET NULL,
    starts_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(account_id,instance_id,resource_kind,resource_key)
);
CREATE INDEX account_entitlements_account_idx ON account_entitlements(account_id,status,expires_at);
CREATE INDEX account_entitlements_instance_idx ON account_entitlements(instance_id,resource_key,status);

CREATE TABLE line_endpoints (
    id UUID PRIMARY KEY,
    name VARCHAR(120) NOT NULL,
    base_url VARCHAR(1000) NOT NULL UNIQUE,
    region VARCHAR(120),
    carrier VARCHAR(120),
    audience VARCHAR(32) NOT NULL DEFAULT 'all',
    weight INTEGER NOT NULL DEFAULT 100 CHECK(weight BETWEEN 0 AND 100000),
    sort_order INTEGER NOT NULL DEFAULT 0,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    maintenance BOOLEAN NOT NULL DEFAULT FALSE,
    revision BIGINT NOT NULL DEFAULT 1,
    last_status VARCHAR(32) NOT NULL DEFAULT 'unknown',
    last_latency_ms INTEGER,
    last_error VARCHAR(2000),
    last_checked_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX line_endpoints_order_idx ON line_endpoints(enabled,maintenance,sort_order,id);
CREATE INDEX line_endpoints_status_idx ON line_endpoints(last_status,last_checked_at);

CREATE TABLE review_reactions (
    review_id UUID NOT NULL REFERENCES media_reviews(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    reaction VARCHAR(20) NOT NULL DEFAULT 'like' CHECK(reaction IN ('like')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(review_id,account_id,reaction)
);
CREATE INDEX review_reactions_account_idx ON review_reactions(account_id,created_at DESC);

CREATE TABLE review_reports (
    id UUID PRIMARY KEY,
    review_id UUID NOT NULL REFERENCES media_reviews(id) ON DELETE CASCADE,
    reporter_account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    reason VARCHAR(80) NOT NULL,
    detail VARCHAR(1000),
    status VARCHAR(20) NOT NULL DEFAULT 'open' CHECK(status IN ('open','resolved','dismissed')),
    resolution VARCHAR(1000),
    resolved_by VARCHAR(255),
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(review_id,reporter_account_id)
);
CREATE INDEX review_reports_status_idx ON review_reports(status,created_at DESC);

-- Every legacy-domain adapter writes the exact source row here.  This provides
-- an immutable, queryable fallback when a row is merged, rebuilt, or cannot be
-- represented one-to-one in the active v3 schema.
CREATE TABLE migration_archive_records (
    id UUID PRIMARY KEY,
    source_table VARCHAR(128) NOT NULL,
    source_key VARCHAR(500) NOT NULL,
    source_payload JSONB NOT NULL,
    payload_sha256 CHAR(64) NOT NULL,
    disposition VARCHAR(24) NOT NULL CHECK(disposition IN ('transformed','archived','deferred')),
    target_table VARCHAR(128),
    target_key VARCHAR(500),
    detail VARCHAR(2000),
    import_run_id UUID NOT NULL REFERENCES legacy_import_runs(id) ON DELETE RESTRICT,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(source_table,source_key)
);
CREATE INDEX migration_archive_source_idx ON migration_archive_records(source_table,disposition);
CREATE INDEX migration_archive_run_idx ON migration_archive_records(import_run_id,source_table);
