package domain

import "time"

type TransactionType string

const (
	TransactionTypeTopUp      TransactionType = "top_up"
	TransactionTypeCharge     TransactionType = "charge"
	TransactionTypeRefund     TransactionType = "refund"
	TransactionTypeAdjustment TransactionType = "adjustment"
)

// BillingTransaction — это одна запись в журнале операций.
// AmountRUB знаковый: положительное значение означает зачисление (top_up, refund), отрицательное — списание (charge).
type BillingTransaction struct {
	ID          string          `json:"id"`
	UserID      string          `json:"userId"`
	ProjectID   string          `json:"projectId,omitempty"`
	Type        TransactionType `json:"type"`
	AmountRUB   float64         `json:"amountRub"`
	Description string          `json:"description"`
	CreatedAt   time.Time       `json:"createdAt"`
}

type BillingSummary struct {
	UserID                      string     `json:"userId"`
	Email                       string     `json:"email"`
	BalanceRUB                  float64    `json:"balanceRub"`
	SpentThisMonth              float64    `json:"spentThisMonth"`
	RefundRUB                   float64    `json:"refundRub"`
	PendingChargesRUB           float64    `json:"pendingChargesRub"`
	AvailableRUB                float64    `json:"availableRub"`
	ExemptFromGuard             bool       `json:"exemptFromGuard"`
	GracePeriodEndsAt           *time.Time `json:"gracePeriodEndsAt,omitempty"`
	GracePeriodRemainingSeconds int64      `json:"gracePeriodRemainingSeconds,omitempty"`
}

type TopUpBalanceRequest struct {
	AmountRUB float64 `json:"amountRub"`
}
