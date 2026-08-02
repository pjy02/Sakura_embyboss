ALTER TABLE account_lifecycle_events ADD COLUMN legacy_source_key VARCHAR(500);
CREATE UNIQUE INDEX account_lifecycle_events_legacy_source_uidx
    ON account_lifecycle_events(legacy_source_key) WHERE legacy_source_key IS NOT NULL;

CREATE UNIQUE INDEX audit_logs_legacy_import_request_uidx
    ON audit_logs(request_id) WHERE request_id LIKE 'legacy-v2-%';
