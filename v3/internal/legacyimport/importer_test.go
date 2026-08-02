package legacyimport

import "testing"

func TestAnalyzeReportsDeterministicImportConflicts(t *testing.T) {
	accounts := []legacyAccount{
		{ID: "not-a-uuid", TG: 42},
		{ID: "00000000-0000-4000-8000-000000000099", TG: 42},
	}
	identities := []legacyIdentity{{ID: "identity-1", AccountID: "missing"}}
	conflicts := analyze(accounts, identities)
	if len(conflicts) != 3 {
		t.Fatalf("expected three preflight conflicts, got %v", conflicts)
	}
}

func TestLegacyValueNormalization(t *testing.T) {
	if got := normalizeCurrency("coins"); got != "POINTS" {
		t.Fatalf("coins normalized to %q", got)
	}
	if got := legacyPlanCode(7, ""); got != "v2-plan-7" {
		t.Fatalf("empty plan code normalized to %q", got)
	}
	if got := legacyPlanCode(7, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-extra"); len(got) != 64 {
		t.Fatalf("plan code length = %d", len(got))
	}
}

func TestLegacyDomainAdapterCatalogIsCompleteAndUnique(t *testing.T) {
	want := map[string]bool{
		"point_transactions": true, "billing_entries": true, "account_lifecycle_events": true,
		"emby2": true, "partition_codes": true, "partition_grants": true, "line_endpoints": true,
		"playback_sessions": true, "known_devices": true, "device_client_rules": true,
		"security_events": true, "risk_rules": true, "media_requests": true, "request_records": true,
		"media_reviews": true, "review_reactions": true, "review_reports": true,
		"automation_rules": true, "operation_tasks": true, "config_revisions": true, "api_clients": true,
		"idempotency_records": true, "job_runs": true, "system_events": true, "automation_runs": true,
		"line_health_samples": true, "service_probes": true, "alert_deliveries": true,
	}
	if len(adapterTableSpecs) != len(want) {
		t.Fatalf("expected %d adapter tables, got %d", len(want), len(adapterTableSpecs))
	}
	seen := map[string]bool{}
	for _, spec := range adapterTableSpecs {
		if !want[spec.name] || seen[spec.name] || spec.key == "" {
			t.Fatalf("invalid adapter spec %#v", spec)
		}
		seen[spec.name] = true
	}
}

func TestLifecycleStatusMappingIsConservative(t *testing.T) {
	if from, to := lifecycleStatuses("ban"); from != "active" || to != "banned" {
		t.Fatalf("ban mapping = %s -> %s", from, to)
	}
	if from, to := lifecycleStatuses("restore"); from != "suspended" || to != "active" {
		t.Fatalf("restore mapping = %s -> %s", from, to)
	}
}

func TestLegacyDomainNormalizersKeepTargetConstraints(t *testing.T) {
	if normalizeMediaRequestStatus("submitted") != "requested" || normalizeMoviePilotStatus("failed") != "failed" {
		t.Fatal("media status normalization is unsafe")
	}
	if normalizeSeverity("warning") != "medium" || normalizeMatchOperator("wildcard") != "contains" {
		t.Fatal("risk or device normalization is unsafe")
	}
	if got := stablePositiveInt64("same-key"); got <= 0 || got != stablePositiveInt64("same-key") {
		t.Fatalf("legacy media id is not stable: %d", got)
	}
}
