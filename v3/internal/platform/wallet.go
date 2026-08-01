package platform

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
)

var currencyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,15}$`)

type ledgerPosting struct {
	AccountID uuid.UUID
	Side      string
	Amount    int64
}

func (s *Service) Wallet(ctx context.Context, accountID uuid.UUID, currency string) (Wallet, error) {
	currency = normalizeCurrency(currency)
	if !currencyPattern.MatchString(currency) {
		return Wallet{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Wallet{}, err
	}
	defer tx.Rollback(ctx)
	wallet, err := s.ensureWalletTx(ctx, tx, accountID, currency)
	if err != nil {
		return Wallet{}, err
	}
	wallet.Balance, err = walletBalanceTx(ctx, tx, wallet.LedgerAccountID)
	if err != nil {
		return Wallet{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Wallet{}, err
	}
	return wallet, nil
}

func normalizeCurrency(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return "POINTS"
	}
	return value
}

func (s *Service) ensureWalletTx(ctx context.Context, tx pgx.Tx, accountID uuid.UUID, currency string) (Wallet, error) {
	var accountStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM accounts WHERE id=$1`, accountID).Scan(&accountStatus); err != nil {
		return Wallet{}, notFound(err)
	}
	ledgerID := uuid.New()
	_, err := tx.Exec(ctx, `INSERT INTO ledger_accounts(id,owner_type,owner_id,code,currency,normal_side) VALUES($1,'account',$2,'wallet',$3,'credit') ON CONFLICT(owner_type,owner_id,code,currency) DO NOTHING`, ledgerID, accountID.String(), currency)
	if err != nil {
		return Wallet{}, err
	}
	if err = tx.QueryRow(ctx, `SELECT id FROM ledger_accounts WHERE owner_type='account' AND owner_id=$1 AND code='wallet' AND currency=$2`, accountID.String(), currency).Scan(&ledgerID); err != nil {
		return Wallet{}, err
	}
	walletID := uuid.New()
	_, err = tx.Exec(ctx, `INSERT INTO wallets(id,account_id,ledger_account_id,currency,status) VALUES($1,$2,$3,$4,CASE WHEN $5='active' THEN 'active' ELSE 'frozen' END) ON CONFLICT(account_id,currency) DO NOTHING`, walletID, accountID, ledgerID, currency, accountStatus)
	if err != nil {
		return Wallet{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE wallets SET status=CASE WHEN $3='active' THEN 'active' ELSE 'frozen' END,updated_at=CASE WHEN status IS DISTINCT FROM CASE WHEN $3='active' THEN 'active' ELSE 'frozen' END THEN NOW() ELSE updated_at END WHERE account_id=$1 AND currency=$2 AND status<>'closed'`, accountID, currency, accountStatus); err != nil {
		return Wallet{}, err
	}
	var wallet Wallet
	err = tx.QueryRow(ctx, `SELECT id,account_id,ledger_account_id,currency,status,created_at,updated_at FROM wallets WHERE account_id=$1 AND currency=$2 FOR UPDATE`, accountID, currency).Scan(&wallet.ID, &wallet.AccountID, &wallet.LedgerAccountID, &wallet.Currency, &wallet.Status, &wallet.CreatedAt, &wallet.UpdatedAt)
	return wallet, err
}

func walletBalanceTx(ctx context.Context, tx pgx.Tx, ledgerAccountID uuid.UUID) (int64, error) {
	var balance int64
	err := tx.QueryRow(ctx, `SELECT COALESCE(SUM(CASE WHEN e.side='credit' THEN e.amount ELSE -e.amount END),0) FROM ledger_entries e JOIN ledger_transactions t ON t.id=e.transaction_id AND t.status='posted' WHERE e.ledger_account_id=$1`, ledgerAccountID).Scan(&balance)
	return balance, err
}

func systemLedgerAccountTx(ctx context.Context, tx pgx.Tx, code, currency string) (uuid.UUID, error) {
	id := uuid.New()
	normalSide := "debit"
	if code == "membership_revenue" {
		normalSide = "credit"
	}
	if _, err := tx.Exec(ctx, `INSERT INTO ledger_accounts(id,owner_type,owner_id,code,currency,normal_side) VALUES($1,'system','system',$2,$3,$4) ON CONFLICT(owner_type,owner_id,code,currency) DO NOTHING`, id, code, currency, normalSide); err != nil {
		return uuid.Nil, err
	}
	err := tx.QueryRow(ctx, `SELECT id FROM ledger_accounts WHERE owner_type='system' AND owner_id='system' AND code=$1 AND currency=$2 AND status='active'`, code, currency).Scan(&id)
	return id, notFound(err)
}

func (s *Service) postLedgerTx(ctx context.Context, tx pgx.Tx, kind, currency, idempotencyKey, referenceType, referenceID, description string, reversalOf *uuid.UUID, metadata map[string]any, actor identity.Actor, postings []ledgerPosting) (LedgerTransaction, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	currency = normalizeCurrency(currency)
	if idempotencyKey == "" || len(idempotencyKey) > 255 || !currencyPattern.MatchString(currency) || len(postings) < 2 {
		return LedgerTransaction{}, identity.ErrInvalid
	}
	var existingID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM ledger_transactions WHERE idempotency_key=$1`, idempotencyKey).Scan(&existingID)
	if err == nil {
		return s.ledgerTransactionTx(ctx, tx, existingID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return LedgerTransaction{}, err
	}
	var debit, credit int64
	for _, posting := range postings {
		if posting.Amount <= 0 || posting.Side != "debit" && posting.Side != "credit" {
			return LedgerTransaction{}, identity.ErrInvalid
		}
		if posting.Side == "debit" {
			debit += posting.Amount
		} else {
			credit += posting.Amount
		}
	}
	if debit <= 0 || debit != credit {
		return LedgerTransaction{}, identity.ErrInvalid
	}
	id := uuid.New()
	transactionNo := "TX-" + time.Now().UTC().Format("20060102150405") + "-" + strings.ToUpper(strings.ReplaceAll(uuid.NewString()[:8], "-", ""))
	_, err = tx.Exec(ctx, `INSERT INTO ledger_transactions(id,transaction_no,idempotency_key,kind,currency,reference_type,reference_id,reversal_of_id,description,metadata,actor) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''),$8,NULLIF($9,''),$10,$11)`, id, transactionNo, idempotencyKey, kind, currency, referenceType, referenceID, reversalOf, description, jsonBytes(metadata), actor.Label())
	if err != nil {
		return LedgerTransaction{}, identity.ErrConflict
	}
	for _, posting := range postings {
		if _, err = tx.Exec(ctx, `INSERT INTO ledger_entries(transaction_id,ledger_account_id,side,amount) VALUES($1,$2,$3,$4)`, id, posting.AccountID, posting.Side, posting.Amount); err != nil {
			return LedgerTransaction{}, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE ledger_transactions SET status='posted',posted_at=NOW() WHERE id=$1 AND status='draft'`, id); err != nil {
		return LedgerTransaction{}, err
	}
	return s.ledgerTransactionTx(ctx, tx, id)
}

func (s *Service) ledgerTransactionTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (LedgerTransaction, error) {
	var item LedgerTransaction
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT id,transaction_no,kind,currency,COALESCE(reference_type,''),COALESCE(reference_id,''),reversal_of_id,COALESCE(description,''),metadata,actor,created_at,posted_at FROM ledger_transactions WHERE id=$1 AND status='posted'`, id).Scan(&item.ID, &item.TransactionNo, &item.Kind, &item.Currency, &item.ReferenceType, &item.ReferenceID, &item.ReversalOfID, &item.Description, &raw, &item.Actor, &item.CreatedAt, &item.PostedAt)
	if err != nil {
		return LedgerTransaction{}, notFound(err)
	}
	item.Metadata = decodeJSON(raw)
	rows, err := tx.Query(ctx, `SELECT e.id,e.ledger_account_id,a.code,e.side,e.amount,e.created_at FROM ledger_entries e JOIN ledger_accounts a ON a.id=e.ledger_account_id WHERE e.transaction_id=$1 ORDER BY e.id`, id)
	if err != nil {
		return LedgerTransaction{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var entry LedgerEntry
		if err = rows.Scan(&entry.ID, &entry.LedgerAccountID, &entry.AccountCode, &entry.Side, &entry.Amount, &entry.CreatedAt); err != nil {
			return LedgerTransaction{}, err
		}
		item.Entries = append(item.Entries, entry)
	}
	return item, rows.Err()
}

func (s *Service) ListLedger(ctx context.Context, accountID uuid.UUID, currency string, limit int) ([]LedgerTransaction, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	currency = normalizeCurrency(currency)
	rows, err := s.db.Query(ctx, `SELECT t.id FROM ledger_transactions t JOIN ledger_entries e ON e.transaction_id=t.id JOIN wallets w ON w.ledger_account_id=e.ledger_account_id WHERE w.account_id=$1 AND w.currency=$2 AND t.status='posted' GROUP BY t.id,t.created_at ORDER BY t.created_at DESC LIMIT $3`, accountID, currency, limit)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	out := make([]LedgerTransaction, 0, len(ids))
	for _, id := range ids {
		tx, beginErr := s.db.Begin(ctx)
		if beginErr != nil {
			return nil, beginErr
		}
		item, getErr := s.ledgerTransactionTx(ctx, tx, id)
		_ = tx.Rollback(ctx)
		if getErr != nil {
			return nil, getErr
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Service) AdjustWallet(ctx context.Context, accountID uuid.UUID, amount int64, currency, reason, idempotencyKey string, actor identity.Actor) (LedgerTransaction, error) {
	if amount == 0 || strings.TrimSpace(reason) == "" || len(reason) > 500 || strings.TrimSpace(idempotencyKey) == "" || len(idempotencyKey) > 160 {
		return LedgerTransaction{}, identity.ErrInvalid
	}
	currency = normalizeCurrency(currency)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return LedgerTransaction{}, err
	}
	defer tx.Rollback(ctx)
	fullKey := "adjust:" + accountID.String() + ":" + strings.TrimSpace(idempotencyKey)
	var replayID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM ledger_transactions WHERE idempotency_key=$1`, fullKey).Scan(&replayID)
	if err == nil {
		return s.ledgerTransactionTx(ctx, tx, replayID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return LedgerTransaction{}, err
	}
	wallet, err := s.ensureWalletTx(ctx, tx, accountID, currency)
	if err != nil {
		return LedgerTransaction{}, err
	}
	if wallet.Status != "active" {
		return LedgerTransaction{}, identity.ErrForbidden
	}
	system, err := systemLedgerAccountTx(ctx, tx, "admin_adjustment", currency)
	if err != nil {
		return LedgerTransaction{}, err
	}
	value := amount
	if value < 0 {
		value = -value
		balance, balanceErr := walletBalanceTx(ctx, tx, wallet.LedgerAccountID)
		if balanceErr != nil {
			return LedgerTransaction{}, balanceErr
		}
		if balance < value {
			return LedgerTransaction{}, identity.ErrForbidden
		}
	}
	postings := []ledgerPosting{{AccountID: system, Side: "debit", Amount: value}, {AccountID: wallet.LedgerAccountID, Side: "credit", Amount: value}}
	if amount < 0 {
		postings[0].Side = "credit"
		postings[1].Side = "debit"
	}
	item, err := s.postLedgerTx(ctx, tx, "admin_adjustment", currency, fullKey, "account", accountID.String(), reason, nil, map[string]any{"signed_amount": amount}, actor, postings)
	if err != nil {
		return LedgerTransaction{}, err
	}
	if err = audit(ctx, tx, actor, "wallet.adjust", "account", accountID.String(), map[string]any{"transaction_id": item.ID, "amount": amount, "currency": currency, "reason": reason}); err != nil {
		return LedgerTransaction{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return LedgerTransaction{}, err
	}
	return item, nil
}

func (s *Service) LedgerInvariant(ctx context.Context, transactionID uuid.UUID) error {
	var debit, credit int64
	var count int
	err := s.db.QueryRow(ctx, `SELECT COUNT(*),COALESCE(SUM(amount) FILTER(WHERE side='debit'),0),COALESCE(SUM(amount) FILTER(WHERE side='credit'),0) FROM ledger_entries WHERE transaction_id=$1`, transactionID).Scan(&count, &debit, &credit)
	if err != nil {
		return err
	}
	if count < 2 || debit != credit {
		return fmt.Errorf("ledger transaction %s is not balanced", transactionID)
	}
	return nil
}
