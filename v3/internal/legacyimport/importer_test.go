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
