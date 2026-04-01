package yookassa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const defaultBaseURL = "https://api.yookassa.ru/v3"

// Client отправляет запросы к YooKassa REST API.
type Client struct {
	shopID    string
	secretKey string
	baseURL   string
	http      *http.Client
}

// NewClient создаёт клиент с заданными учётными данными.
func NewClient(shopID, secretKey string) *Client {
	return &Client{
		shopID:    shopID,
		secretKey: secretKey,
		baseURL:   defaultBaseURL,
		http:      &http.Client{},
	}
}

// NewClientFromEnvironment читает YOOKASSA_SHOP_ID и YOOKASSA_SECRET_KEY.
// Возвращает nil, если переменные не заданы (режим без реального шлюза).
func NewClientFromEnvironment() *Client {
	shopID := strings.TrimSpace(os.Getenv("YOOKASSA_SHOP_ID"))
	secretKey := strings.TrimSpace(os.Getenv("YOOKASSA_SECRET_KEY"))
	if shopID == "" || secretKey == "" {
		return nil
	}
	return NewClient(shopID, secretKey)
}

// CreatePayment создаёт платёж в YooKassa и возвращает его ID и URL для редиректа пользователя.
func (c *Client) CreatePayment(ctx context.Context, amountRUB float64, idempotenceKey, returnURL, description string, metadata map[string]string) (paymentID, confirmationURL string, err error) {
	var reqBody createPaymentRequest
	reqBody.Amount.Value = fmt.Sprintf("%.2f", amountRUB)
	reqBody.Amount.Currency = "RUB"
	reqBody.Confirmation.Type = "redirect"
	reqBody.Confirmation.ReturnURL = returnURL
	reqBody.Description = description
	reqBody.Metadata = metadata
	reqBody.Capture = true

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/payments", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", "", fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth(c.shopID, c.secretKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotence-Key", idempotenceKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var apiErr apiError
		if jsonErr := json.Unmarshal(respBytes, &apiErr); jsonErr == nil && apiErr.Description != "" {
			return "", "", fmt.Errorf("yookassa error %s: %s", apiErr.Code, apiErr.Description)
		}
		return "", "", fmt.Errorf("yookassa HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var payment createPaymentResponse
	if err := json.Unmarshal(respBytes, &payment); err != nil {
		return "", "", fmt.Errorf("unmarshal response: %w", err)
	}

	return payment.ID, payment.Confirmation.ConfirmationURL, nil
}
