package reconcile

import "testing"

func TestEvaluateBlocksUnsafeCutover(t *testing.T) {
	report := Report{Accounts: CountCheck{Missing: 1}, Wallets: []MoneyCheck{{Currency: "POINTS", Source: 10, Imported: 9, Difference: -1}}, LastImportStatus: "completed"}
	report.Evaluate()
	if report.Pass || len(report.Blockers) != 2 {
		t.Fatalf("expected two blockers, got %#v", report)
	}
}

func TestEvaluateAllowsExactMigrationAndWarnsBeforeEmbyAdoption(t *testing.T) {
	report := Report{Accounts: CountCheck{Source: 2, Target: 2}, EmbyIdentities: CountCheck{Source: 1, Target: 1}, Wallets: []MoneyCheck{{Currency: "POINTS", Source: 10, Imported: 10, Current: 10}}, LastImportStatus: "completed"}
	report.Evaluate()
	if !report.Pass || len(report.Warnings) != 1 {
		t.Fatalf("expected pass with adoption warning, got %#v", report)
	}
}

func TestEvaluateBlocksNonEmptyTableWithoutAdapter(t *testing.T) {
	report := Report{LastImportStatus: "completed", Tables: []TableCheck{{Name: "support_tickets", Rows: 3, Disposition: "pending_adapter"}}}
	report.Evaluate()
	if report.Pass || len(report.Blockers) != 1 {
		t.Fatalf("expected unsupported source table blocker, got %#v", report)
	}
}
