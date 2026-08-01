package platform

import (
	"time"

	"github.com/google/uuid"
)

type Wallet struct {
	ID              uuid.UUID `json:"id"`
	AccountID       uuid.UUID `json:"account_id"`
	LedgerAccountID uuid.UUID `json:"ledger_account_id"`
	Currency        string    `json:"currency"`
	Status          string    `json:"status"`
	Balance         int64     `json:"balance"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type LedgerEntry struct {
	ID              int64     `json:"id"`
	LedgerAccountID uuid.UUID `json:"ledger_account_id"`
	AccountCode     string    `json:"account_code"`
	Side            string    `json:"side"`
	Amount          int64     `json:"amount"`
	CreatedAt       time.Time `json:"created_at"`
}

type LedgerTransaction struct {
	ID            uuid.UUID      `json:"id"`
	TransactionNo string         `json:"transaction_no"`
	Kind          string         `json:"kind"`
	Currency      string         `json:"currency"`
	ReferenceType string         `json:"reference_type,omitempty"`
	ReferenceID   string         `json:"reference_id,omitempty"`
	ReversalOfID  *uuid.UUID     `json:"reversal_of_id,omitempty"`
	Description   string         `json:"description,omitempty"`
	Metadata      map[string]any `json:"metadata"`
	Actor         string         `json:"actor"`
	Entries       []LedgerEntry  `json:"entries"`
	CreatedAt     time.Time      `json:"created_at"`
	PostedAt      time.Time      `json:"posted_at"`
}

type RechargeProduct struct {
	ID              uuid.UUID `json:"id"`
	Code            string    `json:"code"`
	Name            string    `json:"name"`
	Description     string    `json:"description,omitempty"`
	PriceMinor      int64     `json:"price_minor"`
	PaymentCurrency string    `json:"payment_currency"`
	GrantAmount     int64     `json:"grant_amount"`
	WalletCurrency  string    `json:"wallet_currency"`
	Enabled         bool      `json:"enabled"`
	SortOrder       int       `json:"sort_order"`
	Revision        int64     `json:"revision"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type MembershipProduct struct {
	ID             uuid.UUID `json:"id"`
	Code           string    `json:"code"`
	Name           string    `json:"name"`
	PlanID         uuid.UUID `json:"plan_id"`
	PlanCode       string    `json:"plan_code"`
	PlanName       string    `json:"plan_name"`
	DurationDays   int       `json:"duration_days"`
	PriceAmount    int64     `json:"price_amount"`
	WalletCurrency string    `json:"wallet_currency"`
	Enabled        bool      `json:"enabled"`
	SortOrder      int       `json:"sort_order"`
	Revision       int64     `json:"revision"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type RechargeOrder struct {
	ID                  uuid.UUID  `json:"id"`
	OrderNo             string     `json:"order_no"`
	AccountID           uuid.UUID  `json:"account_id"`
	ProductID           uuid.UUID  `json:"product_id"`
	ProductName         string     `json:"product_name"`
	Provider            string     `json:"provider"`
	ExternalOrderID     string     `json:"external_order_id,omitempty"`
	Status              string     `json:"status"`
	PriceMinor          int64      `json:"price_minor"`
	PaymentCurrency     string     `json:"payment_currency"`
	GrantAmount         int64      `json:"grant_amount"`
	WalletCurrency      string     `json:"wallet_currency"`
	PaidAmountMinor     *int64     `json:"paid_amount_minor,omitempty"`
	LedgerTransactionID *uuid.UUID `json:"ledger_transaction_id,omitempty"`
	ExpiresAt           time.Time  `json:"expires_at"`
	PaidAt              *time.Time `json:"paid_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type RechargeRefund struct {
	ID                  uuid.UUID `json:"id"`
	RefundNo            string    `json:"refund_no"`
	OrderID             uuid.UUID `json:"order_id"`
	WalletAmount        int64     `json:"wallet_amount"`
	ExternalAmountMinor int64     `json:"external_amount_minor"`
	Reason              string    `json:"reason"`
	Status              string    `json:"status"`
	LedgerTransactionID uuid.UUID `json:"ledger_transaction_id"`
	Actor               string    `json:"actor"`
	CreatedAt           time.Time `json:"created_at"`
}

type MembershipPurchase struct {
	ID                  uuid.UUID  `json:"id"`
	PurchaseNo          string     `json:"purchase_no"`
	AccountID           uuid.UUID  `json:"account_id"`
	ProductID           uuid.UUID  `json:"product_id"`
	Membership          Membership `json:"membership"`
	Amount              int64      `json:"amount"`
	Currency            string     `json:"currency"`
	LedgerTransactionID uuid.UUID  `json:"ledger_transaction_id"`
	CreatedAt           time.Time  `json:"created_at"`
}

type AccountTag struct {
	ID          uuid.UUID `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Color       string    `json:"color,omitempty"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Notification struct {
	ID               uuid.UUID      `json:"id"`
	AccountID        uuid.UUID      `json:"account_id"`
	BatchID          *uuid.UUID     `json:"batch_operation_id,omitempty"`
	Title            string         `json:"title"`
	Body             string         `json:"body"`
	Channel          string         `json:"channel"`
	Status           string         `json:"status"`
	DeliveryStatus   string         `json:"delivery_status"`
	DeliveryAttempts int            `json:"delivery_attempts"`
	DeliveryError    string         `json:"delivery_error,omitempty"`
	Metadata         map[string]any `json:"metadata"`
	CreatedAt        time.Time      `json:"created_at"`
	ReadAt           *time.Time     `json:"read_at,omitempty"`
}

type TelegramNotificationDelivery struct {
	NotificationID uuid.UUID `json:"notification_id"`
	TelegramUserID int64     `json:"telegram_user_id"`
	Title          string    `json:"title"`
	Body           string    `json:"body"`
}

type BatchTarget struct {
	AccountIDs []uuid.UUID `json:"account_ids,omitempty"`
	Status     string      `json:"status,omitempty"`
	TagIDs     []uuid.UUID `json:"tag_ids,omitempty"`
}

type BatchOperation struct {
	ID             uuid.UUID      `json:"id"`
	OperationType  string         `json:"operation_type"`
	Status         string         `json:"status"`
	TargetSpec     map[string]any `json:"target_spec"`
	Payload        map[string]any `json:"payload"`
	TotalCount     int            `json:"total_count"`
	ProcessedCount int            `json:"processed_count"`
	SucceededCount int            `json:"succeeded_count"`
	FailedCount    int            `json:"failed_count"`
	Attempts       int            `json:"attempts"`
	MaxAttempts    int            `json:"max_attempts"`
	LastError      string         `json:"last_error,omitempty"`
	CreatedBy      string         `json:"created_by"`
	CreatedAt      time.Time      `json:"created_at"`
	StartedAt      *time.Time     `json:"started_at,omitempty"`
	FinishedAt     *time.Time     `json:"finished_at,omitempty"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type BatchItem struct {
	ID          int64          `json:"id"`
	OperationID uuid.UUID      `json:"operation_id"`
	AccountID   uuid.UUID      `json:"account_id"`
	Status      string         `json:"status"`
	Attempts    int            `json:"attempts"`
	Result      map[string]any `json:"result"`
	LastError   string         `json:"last_error,omitempty"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	FinishedAt  *time.Time     `json:"finished_at,omitempty"`
}
