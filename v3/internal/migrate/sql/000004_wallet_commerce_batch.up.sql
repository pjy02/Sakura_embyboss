INSERT INTO permissions(code,description) VALUES
('wallets.read','Read wallet balances and immutable ledger'),
('wallets.adjust','Post audited administrator wallet adjustments'),
('commerce.products.read','Read recharge and membership products'),
('commerce.products.write','Manage recharge and membership products'),
('commerce.orders.read','Read recharge orders and refunds'),
('commerce.orders.write','Manage recharge orders, callbacks and refunds'),
('account_tags.read','Read account tags'),
('account_tags.write','Manage account tags'),
('batch_operations.read','Read batch operation progress'),
('batch_operations.write','Create, pause, resume, retry and cancel batch operations')
ON CONFLICT(code) DO NOTHING;

INSERT INTO role_permissions(role_id,permission_code)
SELECT role_id,code FROM
  (VALUES ('00000000-0000-4000-8000-000000000001'::uuid),('00000000-0000-4000-8000-000000000002'::uuid)) roles(role_id)
  CROSS JOIN permissions
WHERE code IN ('wallets.read','wallets.adjust','commerce.products.read','commerce.products.write','commerce.orders.read','commerce.orders.write','account_tags.read','account_tags.write','batch_operations.read','batch_operations.write')
ON CONFLICT DO NOTHING;

INSERT INTO dynamic_settings(key,value,value_type,updated_by) VALUES
('wallet.default_currency','"POINTS"'::jsonb,'string','system:migration'),
('batch.chunk_size','50'::jsonb,'integer','system:migration'),
('batch.max_targets','10000'::jsonb,'integer','system:migration'),
('commerce.order_ttl_minutes','30'::jsonb,'integer','system:migration')
ON CONFLICT(key) DO NOTHING;

INSERT INTO setting_revisions(setting_key,revision,value,value_type,actor,reason)
SELECT key,revision,value,value_type,updated_by,'phase 4 initial value'
FROM dynamic_settings
WHERE key IN ('wallet.default_currency','batch.chunk_size','batch.max_targets','commerce.order_ttl_minutes')
ON CONFLICT(setting_key,revision) DO NOTHING;

CREATE TABLE ledger_accounts (
    id UUID PRIMARY KEY,
    owner_type VARCHAR(20) NOT NULL CHECK(owner_type IN ('system','account')),
    owner_id VARCHAR(128) NOT NULL,
    code VARCHAR(80) NOT NULL,
    currency VARCHAR(16) NOT NULL,
    normal_side VARCHAR(8) NOT NULL CHECK(normal_side IN ('debit','credit')),
    status VARCHAR(16) NOT NULL DEFAULT 'active' CHECK(status IN ('active','frozen','closed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(owner_type,owner_id,code,currency)
);

INSERT INTO ledger_accounts(id,owner_type,owner_id,code,currency,normal_side) VALUES
('00000000-0000-4000-8000-000000000201','system','system','cash_clearing','POINTS','debit'),
('00000000-0000-4000-8000-000000000202','system','system','membership_revenue','POINTS','credit'),
('00000000-0000-4000-8000-000000000203','system','system','admin_adjustment','POINTS','debit');

CREATE TABLE wallets (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    ledger_account_id UUID NOT NULL UNIQUE REFERENCES ledger_accounts(id),
    currency VARCHAR(16) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'active' CHECK(status IN ('active','frozen','closed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(account_id,currency)
);

CREATE TABLE ledger_transactions (
    id UUID PRIMARY KEY,
    transaction_no VARCHAR(80) NOT NULL UNIQUE,
    idempotency_key VARCHAR(255) NOT NULL UNIQUE,
    kind VARCHAR(32) NOT NULL CHECK(kind IN ('recharge','refund','admin_adjustment','membership_purchase','batch_adjustment')),
    currency VARCHAR(16) NOT NULL,
    status VARCHAR(12) NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','posted')),
    reference_type VARCHAR(40),
    reference_id VARCHAR(128),
    reversal_of_id UUID REFERENCES ledger_transactions(id),
    description VARCHAR(500),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    actor VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    posted_at TIMESTAMPTZ
);
CREATE INDEX ledger_transactions_reference_idx ON ledger_transactions(reference_type,reference_id,created_at DESC);
CREATE INDEX ledger_transactions_reversal_idx ON ledger_transactions(reversal_of_id);

CREATE TABLE ledger_entries (
    id BIGSERIAL PRIMARY KEY,
    transaction_id UUID NOT NULL REFERENCES ledger_transactions(id),
    ledger_account_id UUID NOT NULL REFERENCES ledger_accounts(id),
    side VARCHAR(8) NOT NULL CHECK(side IN ('debit','credit')),
    amount BIGINT NOT NULL CHECK(amount > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(transaction_id,ledger_account_id,side)
);
CREATE INDEX ledger_entries_account_idx ON ledger_entries(ledger_account_id,created_at DESC);

CREATE FUNCTION sakura_ledger_entry_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE transaction_status VARCHAR(12);
BEGIN
  IF TG_OP <> 'INSERT' THEN
    RAISE EXCEPTION 'ledger entries are immutable';
  END IF;
  SELECT status INTO transaction_status FROM ledger_transactions WHERE id=NEW.transaction_id FOR SHARE;
  IF transaction_status IS DISTINCT FROM 'draft' THEN
    RAISE EXCEPTION 'entries can only be added to a draft transaction';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER ledger_entries_immutable BEFORE INSERT OR UPDATE OR DELETE ON ledger_entries
FOR EACH ROW EXECUTE FUNCTION sakura_ledger_entry_guard();

CREATE FUNCTION sakura_ledger_transaction_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE debit_total BIGINT; credit_total BIGINT; entry_count INTEGER; invalid_currency INTEGER;
BEGIN
  IF TG_OP='INSERT' THEN
    IF NEW.status<>'draft' OR NEW.posted_at IS NOT NULL THEN RAISE EXCEPTION 'ledger transactions must be created as drafts'; END IF;
    RETURN NEW;
  END IF;
  IF TG_OP='DELETE' THEN RAISE EXCEPTION 'ledger transactions are immutable'; END IF;
  IF OLD.status='draft' AND NEW.status='posted'
     AND OLD.id=NEW.id AND OLD.transaction_no=NEW.transaction_no AND OLD.idempotency_key=NEW.idempotency_key
     AND OLD.kind=NEW.kind AND OLD.currency=NEW.currency
     AND OLD.reference_type IS NOT DISTINCT FROM NEW.reference_type AND OLD.reference_id IS NOT DISTINCT FROM NEW.reference_id
     AND OLD.reversal_of_id IS NOT DISTINCT FROM NEW.reversal_of_id
     AND OLD.description IS NOT DISTINCT FROM NEW.description AND OLD.metadata=NEW.metadata AND OLD.actor=NEW.actor THEN
    SELECT COUNT(*),COALESCE(SUM(amount) FILTER(WHERE side='debit'),0),COALESCE(SUM(amount) FILTER(WHERE side='credit'),0),
           COUNT(*) FILTER(WHERE a.currency<>NEW.currency)
      INTO entry_count,debit_total,credit_total,invalid_currency
      FROM ledger_entries e JOIN ledger_accounts a ON a.id=e.ledger_account_id
      WHERE e.transaction_id=NEW.id;
    IF entry_count<2 OR debit_total<=0 OR debit_total<>credit_total OR invalid_currency<>0 THEN
      RAISE EXCEPTION 'ledger transaction is not balanced';
    END IF;
    IF NEW.posted_at IS NULL THEN NEW.posted_at=NOW(); END IF;
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'ledger transactions are immutable after creation';
END $$;
CREATE TRIGGER ledger_transactions_immutable BEFORE INSERT OR UPDATE OR DELETE ON ledger_transactions
FOR EACH ROW EXECUTE FUNCTION sakura_ledger_transaction_guard();

CREATE TABLE recharge_products (
    id UUID PRIMARY KEY,
    code VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(120) NOT NULL,
    description VARCHAR(1000),
    price_minor BIGINT NOT NULL CHECK(price_minor>0),
    payment_currency VARCHAR(8) NOT NULL DEFAULT 'CNY',
    grant_amount BIGINT NOT NULL CHECK(grant_amount>0),
    wallet_currency VARCHAR(16) NOT NULL DEFAULT 'POINTS',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    revision BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE membership_products (
    id UUID PRIMARY KEY,
    code VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(120) NOT NULL,
    plan_id UUID NOT NULL REFERENCES membership_plans(id),
    duration_days INTEGER NOT NULL CHECK(duration_days BETWEEN 1 AND 3650),
    price_amount BIGINT NOT NULL CHECK(price_amount>0),
    wallet_currency VARCHAR(16) NOT NULL DEFAULT 'POINTS',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    revision BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE recharge_orders (
    id UUID PRIMARY KEY,
    order_no VARCHAR(80) NOT NULL UNIQUE,
    idempotency_key VARCHAR(200) NOT NULL UNIQUE,
    account_id UUID NOT NULL REFERENCES accounts(id),
    product_id UUID NOT NULL REFERENCES recharge_products(id),
    provider VARCHAR(40) NOT NULL,
    external_order_id VARCHAR(160),
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','paid','partially_refunded','refunded','canceled','expired')),
    price_minor BIGINT NOT NULL,
    payment_currency VARCHAR(8) NOT NULL,
    grant_amount BIGINT NOT NULL,
    wallet_currency VARCHAR(16) NOT NULL,
    paid_amount_minor BIGINT,
    ledger_transaction_id UUID UNIQUE REFERENCES ledger_transactions(id),
    expires_at TIMESTAMPTZ NOT NULL,
    paid_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(provider,external_order_id)
);
CREATE INDEX recharge_orders_account_idx ON recharge_orders(account_id,created_at DESC);

CREATE TABLE payment_callback_events (
    id BIGSERIAL PRIMARY KEY,
    provider VARCHAR(40) NOT NULL,
    event_id VARCHAR(180) NOT NULL,
    order_id UUID NOT NULL REFERENCES recharge_orders(id),
    external_order_id VARCHAR(160) NOT NULL,
    amount_minor BIGINT NOT NULL,
    payload_hash BYTEA NOT NULL,
    ledger_transaction_id UUID REFERENCES ledger_transactions(id),
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(provider,event_id)
);

CREATE TABLE recharge_refunds (
    id UUID PRIMARY KEY,
    refund_no VARCHAR(80) NOT NULL UNIQUE,
    order_id UUID NOT NULL REFERENCES recharge_orders(id),
    wallet_amount BIGINT NOT NULL CHECK(wallet_amount>0),
    external_amount_minor BIGINT NOT NULL CHECK(external_amount_minor>0),
    reason VARCHAR(500) NOT NULL,
    status VARCHAR(20) NOT NULL CHECK(status IN ('succeeded')),
    ledger_transaction_id UUID NOT NULL UNIQUE REFERENCES ledger_transactions(id),
    idempotency_key VARCHAR(200) NOT NULL UNIQUE,
    actor VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE membership_purchases (
    id UUID PRIMARY KEY,
    purchase_no VARCHAR(80) NOT NULL UNIQUE,
    account_id UUID NOT NULL REFERENCES accounts(id),
    product_id UUID NOT NULL REFERENCES membership_products(id),
    membership_id UUID NOT NULL REFERENCES account_memberships(id),
    amount BIGINT NOT NULL CHECK(amount>0),
    currency VARCHAR(16) NOT NULL,
    ledger_transaction_id UUID NOT NULL UNIQUE REFERENCES ledger_transactions(id),
    idempotency_key VARCHAR(200) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE account_tags (
    id UUID PRIMARY KEY,
    code VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL,
    color VARCHAR(16),
    description VARCHAR(500),
    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE account_tag_assignments (
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    tag_id UUID NOT NULL REFERENCES account_tags(id) ON DELETE CASCADE,
    assigned_by VARCHAR(255) NOT NULL,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(account_id,tag_id)
);

CREATE TABLE batch_operations (
    id UUID PRIMARY KEY,
    operation_type VARCHAR(32) NOT NULL CHECK(operation_type IN ('tag_add','tag_remove','membership_adjust','notification')),
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','running','paused','retry','succeeded','failed','canceled')),
    target_spec JSONB NOT NULL,
    payload JSONB NOT NULL,
    idempotency_key VARCHAR(160) NOT NULL UNIQUE,
    total_count INTEGER NOT NULL,
    processed_count INTEGER NOT NULL DEFAULT 0,
    succeeded_count INTEGER NOT NULL DEFAULT 0,
    failed_count INTEGER NOT NULL DEFAULT 0,
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3,
    lease_owner VARCHAR(120),
    lease_expires_at TIMESTAMPTZ,
    last_error VARCHAR(2000),
    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX batch_operations_claim_idx ON batch_operations(status,created_at);

CREATE TABLE batch_operation_items (
    id BIGSERIAL PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES batch_operations(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id),
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','running','succeeded','failed','skipped')),
    attempts INTEGER NOT NULL DEFAULT 0,
    result JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_error VARCHAR(1000),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    UNIQUE(operation_id,account_id)
);
CREATE INDEX batch_operation_items_pending_idx ON batch_operation_items(operation_id,status,id);

CREATE TABLE account_notifications (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    batch_operation_id UUID REFERENCES batch_operations(id),
    title VARCHAR(160) NOT NULL,
    body VARCHAR(4000) NOT NULL,
    channel VARCHAR(20) NOT NULL DEFAULT 'in_app' CHECK(channel IN ('in_app','telegram')),
    status VARCHAR(16) NOT NULL DEFAULT 'unread' CHECK(status IN ('unread','read')),
    delivery_status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK(delivery_status IN ('pending','sending','sent','failed')),
    delivery_attempts INTEGER NOT NULL DEFAULT 0,
    delivery_lease_owner VARCHAR(120),
    delivery_lease_expires_at TIMESTAMPTZ,
    next_delivery_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at TIMESTAMPTZ,
    delivery_error VARCHAR(1000),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    read_at TIMESTAMPTZ,
    UNIQUE(batch_operation_id,account_id,channel)
);
CREATE INDEX account_notifications_account_idx ON account_notifications(account_id,status,created_at DESC);
CREATE INDEX account_notifications_delivery_idx ON account_notifications(channel,delivery_status,next_delivery_at);
