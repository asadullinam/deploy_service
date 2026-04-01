package domain

import "time"

const (
	YooKassaPaymentStatusPending   = "pending"
	YooKassaPaymentStatusSucceeded = "succeeded"
	YooKassaPaymentStatusCanceled  = "canceled"
)

type YooKassaPayment struct {
	ID         string    // внутренний ID, e.g. "ykpay-<nano>"
	YooKassaID string    // ID платежа в YooKassa
	UserID     string
	AmountRUB  float64
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
