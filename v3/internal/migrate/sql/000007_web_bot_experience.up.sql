INSERT INTO permissions(code,description) VALUES
('dashboard.read','Read aggregated administration dashboard and realtime task state')
ON CONFLICT(code) DO NOTHING;

INSERT INTO role_permissions(role_id,permission_code)
SELECT role_id,'dashboard.read'
FROM (VALUES
  ('00000000-0000-4000-8000-000000000001'::uuid),
  ('00000000-0000-4000-8000-000000000002'::uuid)
) roles(role_id)
ON CONFLICT DO NOTHING;

INSERT INTO dynamic_settings(key,value,value_type,updated_by) VALUES
('ui.site_name','"Sakura"'::jsonb,'string','system:migration'),
('ui.support_url','""'::jsonb,'string','system:migration'),
('ui.registration_notice','"欢迎使用 Sakura 用户中心"'::jsonb,'string','system:migration'),
('bot.commands_enabled','true'::jsonb,'boolean','system:migration'),
('bot.admin_commands_enabled','true'::jsonb,'boolean','system:migration')
ON CONFLICT(key) DO NOTHING;

INSERT INTO setting_revisions(setting_key,revision,value,value_type,actor,reason)
SELECT key,revision,value,value_type,updated_by,'phase 7 initial value'
FROM dynamic_settings
WHERE key IN ('ui.site_name','ui.support_url','ui.registration_notice','bot.commands_enabled','bot.admin_commands_enabled')
ON CONFLICT(setting_key,revision) DO NOTHING;
