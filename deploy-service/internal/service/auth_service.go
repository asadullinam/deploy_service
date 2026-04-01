package service

import (
	"context"
	"deploy-service/internal/auth"
	"deploy-service/internal/domain"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"log"
	"strings"
	"time"
)

func NewAuthService(users UserStore, txStore BillingTransactionStore, jwtSecret string, jwtTTL time.Duration, defaultBalanceRUB float64) *AuthService {
	return &AuthService{
		users:             users,
		txStore:           txStore,
		jwtSecret:         jwtSecret,
		jwtTTL:            jwtTTL,
		defaultBalanceRUB: defaultBalanceRUB,
	}
}

func (s *AuthService) Register(ctx context.Context, req domain.RegisterRequest) (domain.TokenResponse, error) {
	if strings.TrimSpace(req.Email) == "" || strings.TrimSpace(req.Password) == "" {
		return domain.TokenResponse{}, fmt.Errorf("email and password are required")
	}
	if _, exists := s.users.GetByEmail(ctx, req.Email); exists {
		return domain.TokenResponse{}, domain.ErrEmailTaken
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return domain.TokenResponse{}, fmt.Errorf("hash password: %w", err)
	}
	telegramUsername := normalizeTelegramUsername(req.TelegramUsername)
	var telegramLinkExpiresAt *time.Time
	telegramLinkCode := ""
	telegramNotificationsEnabled := false
	if telegramUsername != "" {
		expiresAt := time.Now().UTC().Add(24 * time.Hour)
		telegramLinkExpiresAt = &expiresAt
		telegramLinkCode = generateTelegramLinkCode()
		telegramNotificationsEnabled = true
	}
	user := domain.User{
		ID:                           domain.NewID(),
		Email:                        req.Email,
		BalanceRUB:                   s.defaultBalanceRUB,
		PasswordHash:                 string(hash),
		TelegramUsername:             telegramUsername,
		TelegramLinkCode:             telegramLinkCode,
		TelegramLinkExpiresAt:        telegramLinkExpiresAt,
		TelegramNotificationsEnabled: telegramNotificationsEnabled,
		CreatedAt:                    time.Now().UTC(),
	}
	if err := s.users.Create(ctx, user); err != nil {
		return domain.TokenResponse{}, err
	}
	token, err := auth.GenerateToken(user.ID, user.Email, s.jwtSecret, s.jwtTTL)
	if err != nil {
		return domain.TokenResponse{}, fmt.Errorf("generate token: %w", err)
	}
	return domain.TokenResponse{Token: token}, nil
}

func (s *AuthService) Login(ctx context.Context, req domain.LoginRequest) (domain.TokenResponse, error) {
	user, exists := s.users.GetByEmail(ctx, req.Email)
	if !exists {
		return domain.TokenResponse{}, domain.ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return domain.TokenResponse{}, domain.ErrInvalidCredentials
	}
	token, err := auth.GenerateToken(user.ID, user.Email, s.jwtSecret, s.jwtTTL)
	if err != nil {
		return domain.TokenResponse{}, fmt.Errorf("generate token: %w", err)
	}
	return domain.TokenResponse{Token: token}, nil
}

func (s *AuthService) GetUser(ctx context.Context, userID string) (domain.User, error) {
	user, exists := s.users.GetByID(ctx, userID)
	if !exists {
		return domain.User{}, domain.ErrUserNotFound
	}
	return user, nil
}

func (s *AuthService) TopUpBalance(ctx context.Context, userID string, amountRUB float64) (domain.User, error) {
	if amountRUB <= 0 {
		return domain.User{}, fmt.Errorf("top up amount must be positive")
	}
	user, exists := s.users.GetByID(ctx, userID)
	if !exists {
		return domain.User{}, domain.ErrUserNotFound
	}
	user.BalanceRUB += amountRUB
	if err := s.users.UpdateBalance(ctx, user.ID, user.BalanceRUB); err != nil {
		return domain.User{}, err
	}
	tx := domain.BillingTransaction{
		ID:          domain.NewID(),
		UserID:      userID,
		Type:        domain.TransactionTypeTopUp,
		AmountRUB:   amountRUB,
		Description: fmt.Sprintf("balance top-up: +%.2f ₽", amountRUB),
		CreatedAt:   time.Now().UTC(),
	}
	if err := s.txStore.Record(ctx, tx); err != nil {
		fmt.Printf("WARNING: failed to record top_up transaction: %v\n", err)
	}
	return user, nil
}

// SetYooKassa подключает реальный платёжный шлюз.
// client и store могут быть nil — тогда InitiateTopUp вернёт ошибку.
func (s *AuthService) SetYooKassa(client YooKassaClient, store YooKassaPaymentStore, returnURL string) {
	s.yookassaClient = client
	s.paymentStore = store
	s.yookassaReturn = returnURL
}

// InitiateTopUp создаёт платёж в YooKassa на сумму amountRUB и возвращает URL для редиректа.
func (s *AuthService) InitiateTopUp(ctx context.Context, userID string, amountRUB float64) (string, error) {
	if s.yookassaClient == nil || s.paymentStore == nil {
		return "", fmt.Errorf("payment gateway not configured")
	}
	if amountRUB < 1 {
		return "", fmt.Errorf("minimum top-up amount is 900 RUB")
	}

	internalID := domain.NewID()
	description := fmt.Sprintf("Пополнение баланса %.0f ₽", amountRUB)
	metadata := map[string]string{
		"user_id":             userID,
		"internal_payment_id": internalID,
	}

	yookassaID, confirmationURL, err := s.yookassaClient.CreatePayment(ctx, amountRUB, internalID, s.yookassaReturn, description, metadata)
	if err != nil {
		return "", fmt.Errorf("create yookassa payment: %w", err)
	}

	payment := domain.YooKassaPayment{
		ID:         internalID,
		YooKassaID: yookassaID,
		UserID:     userID,
		AmountRUB:  amountRUB,
		Status:     domain.YooKassaPaymentStatusPending,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	if err := s.paymentStore.Create(ctx, payment); err != nil {
		log.Printf("WARNING: failed to save yookassa payment record %s: %v", internalID, err)
	}

	return confirmationURL, nil
}

// HandleYooKassaWebhook обрабатывает уведомление payment.succeeded от YooKassa.
// Идемпотентно: повторный вызов с тем же yookassaPaymentID ничего не делает.
func (s *AuthService) HandleYooKassaWebhook(ctx context.Context, yookassaPaymentID string) error {
	if s.paymentStore == nil {
		return nil
	}

	payment, exists := s.paymentStore.GetByYooKassaID(ctx, yookassaPaymentID)
	if !exists {
		log.Printf("yookassa webhook: unknown payment ID %s, skipping", yookassaPaymentID)
		return nil
	}
	if payment.Status == domain.YooKassaPaymentStatusSucceeded {
		return nil // уже обработано
	}

	if err := s.paymentStore.MarkSucceeded(ctx, yookassaPaymentID); err != nil {
		return fmt.Errorf("mark payment succeeded: %w", err)
	}

	if _, err := s.TopUpBalance(ctx, payment.UserID, payment.AmountRUB); err != nil {
		return fmt.Errorf("credit balance for payment %s: %w", yookassaPaymentID, err)
	}

	log.Printf("yookassa payment %s credited: user=%s +%.4f USD (%.2f RUB)", yookassaPaymentID, payment.UserID, payment.AmountRUB, payment.AmountRUB)
	return nil
}
