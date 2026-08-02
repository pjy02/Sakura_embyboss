# v2 legacy-domain import completion

Migration `000008_legacy_domain_completion` adds the v3 persistence required by
the final v2 import: entitlement codes and grants, line endpoints, review
reactions and reports, plus `migration_archive_records`.

The importer now reads the original 18 v2 domain tables explicitly:

`emby2`, `partition_codes`, `partition_grants`, `line_endpoints`,
`playback_sessions`, `known_devices`, `device_client_rules`, `security_events`,
`risk_rules`, `media_requests`, `request_records`, `media_reviews`,
`review_reactions`, `review_reports`, `automation_rules`, `operation_tasks`,
`config_revisions`, and `api_clients`.

It also treats three records as mandatory transformations:

- `point_transactions` must resolve to the balanced transaction already
  imported from canonical `account_ledger_entries`; the importer refuses to
  post the amount a second time;
- `billing_entries` become deterministic audit records linked to their order;
- `account_lifecycle_events` become both v3 lifecycle and audit events.

These seven low-value runtime-history tables are copied only to the unified
archive and are never replayed: `idempotency_records`, `job_runs`,
`system_events`, `automation_runs`, `line_health_samples`, `service_probes`,
and `alert_deliveries`. `operation_tasks` follows the same archive policy after
it reaches a terminal state; an active task remains `deferred` and blocks
cutover. `alembic_version` stays only in the encrypted full MySQL backup because
it is schema metadata rather than business data.

Every source row is copied to `migration_archive_records` with a deterministic
identity and SHA-256 payload checksum. Re-running `import-v2 --apply` updates the
same archive record and uses deterministic target identifiers or database
conflict guards, so it cannot create duplicate domain rows.

Each archive record has one of three dispositions:

- `transformed`: represented in an active v3 domain table;
- `archived`: intentionally not resumed, such as a terminal v2 operation task
  or an online playback session that the v3 worker will rebuild;
- `deferred`: a required account, Emby instance, review, or setting mapping is
  missing, or a v2 operation task is still active.

`reconcile-v2` reports source, preserved, transformed, archive-only, deferred
and missing row counts for every table. Missing archive rows and any deferred
rows are hard cutover blockers. Point transactions, billing entries and account
lifecycle events additionally require `transformed_rows == source rows`; merely
archiving one of these mandatory records cannot pass the gate. This makes
merges and rebuilds auditable without requiring a false one-to-one target row
count for low-value history.

The import intentionally disables migrated automation rules and retires old API
client credentials. An administrator must review automation rules and issue new
API tokens after cutover.
