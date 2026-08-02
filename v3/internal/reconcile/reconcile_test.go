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

func TestEvaluateBlocksIncompleteOrDeferredLegacyDomainAdapter(t *testing.T) {
	report := Report{LastImportStatus: "completed", Tables: []TableCheck{
		{Name: "emby2", Rows: 3, ArchivedRows: 2, MissingRows: 1, Disposition: "transform"},
		{Name: "risk_rules", Rows: 2, ArchivedRows: 2, DeferredRows: 1, Disposition: "transform"},
	}}
	report.Evaluate()
	if report.Pass || len(report.Blockers) != 2 {
		t.Fatalf("expected per-table archive and deferred blockers, got %#v", report)
	}
}

func TestEvaluateAllowsFullyAccountedLegacyDomainAdapter(t *testing.T) {
	report := Report{LastImportStatus: "completed", Tables: []TableCheck{{Name: "media_requests", Rows: 3, ArchivedRows: 3, TransformedRows: 3, Disposition: "transform", Implemented: true}}}
	report.Evaluate()
	if !report.Pass {
		t.Fatalf("expected accounted adapter to pass, got %#v", report)
	}
}

func TestLegacyDomainDispositionCatalogHasNoPendingAdapter(t *testing.T) {
	if len(legacyDomainAdapters) != 28 {
		t.Fatalf("expected 28 legacy domain adapters, got %d", len(legacyDomainAdapters))
	}
	for table := range legacyDomainAdapters {
		if tableDisposition[table] == "pending_adapter" || tableDisposition[table] == "" {
			t.Fatalf("legacy domain table %s is not implemented", table)
		}
	}
}

func TestEvaluateRequiresFinancialAndLifecycleTransformation(t *testing.T) {
	report := Report{LastImportStatus: "completed", Tables: []TableCheck{{Name: "point_transactions", Rows: 2, ArchivedRows: 2, TransformedRows: 1, TargetRows: 1, ArchiveOnlyRows: 1, Disposition: "transform"}}}
	report.Evaluate()
	if report.Pass || len(report.Blockers) != 1 {
		t.Fatalf("expected mandatory transformation blocker, got %#v", report)
	}
}

func TestEvaluateAllowsCompleteLowValueArchive(t *testing.T) {
	report := Report{LastImportStatus: "completed", Tables: []TableCheck{{Name: "service_probes", Rows: 4, ArchivedRows: 4, ArchiveOnlyRows: 4, Disposition: "archive", Implemented: true}}}
	report.Evaluate()
	if !report.Pass {
		t.Fatalf("expected complete low-value archive to pass, got %#v", report)
	}
}

func TestEvaluateAllowsCompleteMandatoryTransformation(t *testing.T) {
	report := Report{LastImportStatus: "completed", Tables: []TableCheck{{Name: "billing_entries", Rows: 3, ArchivedRows: 3, TransformedRows: 3, TargetRows: 3, Disposition: "transform", Implemented: true}}}
	report.Evaluate()
	if !report.Pass {
		t.Fatalf("expected complete billing transformation to pass, got %#v", report)
	}
}
