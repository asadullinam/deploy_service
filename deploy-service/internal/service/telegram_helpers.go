package service

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
)

func normalizeTelegramUsername(value string) string {
	username := strings.TrimSpace(value)
	username = strings.TrimPrefix(username, "@")
	if username == "" {
		return ""
	}
	filtered := strings.Builder{}
	for _, r := range username {
		switch {
		case r >= 'a' && r <= 'z':
			filtered.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			filtered.WriteRune(r)
		case r >= '0' && r <= '9':
			filtered.WriteRune(r)
		case r == '_':
			filtered.WriteRune(r)
		}
	}
	return filtered.String()
}

func generateTelegramLinkCode() string {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "telegram-link-fallback"
	}
	return strings.TrimRight(base64.RawURLEncoding.EncodeToString(buf), "=")
}
