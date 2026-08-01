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
