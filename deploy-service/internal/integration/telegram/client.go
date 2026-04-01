package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type Client struct {
	token       string
	botUsername string
	client      *http.Client
	baseURL     string
}

func NewClientFromEnvironment() *Client {
	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if token == "" {
		return nil
	}
	return &Client{
		token:       token,
		botUsername: strings.TrimPrefix(strings.TrimSpace(os.Getenv("TELEGRAM_BOT_USERNAME")), "@"),
		baseURL:     fmt.Sprintf("https://api.telegram.org/bot%s", token),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) BotUsername() string {
	if c == nil {
		return ""
	}
	return c.botUsername
}

func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) error {
	if c == nil {
		return nil
	}
	payload := map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("telegram sendMessage returned status %d", resp.StatusCode)
	}
	return nil
}
