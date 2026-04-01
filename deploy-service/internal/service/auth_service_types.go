package service

import "time"

type AuthService struct {
	users             UserStore
	txStore           BillingTransactionStore
	jwtSecret         string
	jwtTTL            time.Duration
	defaultBalanceRUB float64

	// YooKassa — заполняется через SetYooKassa, nil означает отсутствие реального шлюза.
	yookassaClient YooKassaClient
	paymentStore   YooKassaPaymentStore
	yookassaReturn string // URL возврата после оплаты
}
