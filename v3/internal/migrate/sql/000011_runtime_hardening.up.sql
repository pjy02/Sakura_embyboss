CREATE TABLE emby_policy_management (
    binding_id UUID PRIMARY KEY REFERENCES emby_account_bindings(id) ON DELETE CASCADE,
    baseline_folders JSONB NOT NULL DEFAULT '[]'::jsonb,
    last_managed_folders JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE platform_tasks DROP CONSTRAINT IF EXISTS platform_tasks_task_type_check;
ALTER TABLE platform_tasks ADD CONSTRAINT platform_tasks_task_type_check CHECK (task_type IN (
  'emby.provision','emby.sync','emby.reconcile','emby.import','emby.playback_sync',
  'risk.action','risk.revert','media.match','moviepilot.submit',
  'entitlement.sync','emby.favorite','emby.favorite_sync','line.probe'
));

INSERT INTO dynamic_settings(key,value,value_type,updated_by) VALUES
('lines.probe_interval_seconds','300'::jsonb,'integer','system:migration')
ON CONFLICT(key) DO NOTHING;

INSERT INTO setting_revisions(setting_key,revision,value,value_type,actor,reason)
SELECT key,revision,value,value_type,updated_by,'runtime hardening default'
FROM dynamic_settings WHERE key='lines.probe_interval_seconds'
ON CONFLICT(setting_key,revision) DO NOTHING;
