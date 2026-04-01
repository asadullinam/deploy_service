package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrInvalidToken = errors.New("invalid token")

var ErrTokenExpired = errors.New("token expired")

func GenerateToken(userID, email, secret string, ttl time.Duration) (string, error) {
	header := base64.RawURLEncoding.EncodeToString(mustJSON(map[string]string{"alg": "HS256", "typ": "JWT"}))
	payload := base64.RawURLEncoding.EncodeToString(mustJSON(Claims{
		UserID: userID,
		Email:  email,
		Exp:    time.Now().Add(ttl).Unix(),
	}))
	sig := sign(header+"."+payload, secret)
	return header + "." + payload + "." + sig, nil
}

func ValidateClaims(token, secret string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrInvalidToken
	}
	expected := sign(parts[0]+"."+parts[1], secret)
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return Claims{}, ErrInvalidToken
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var claims Claims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if time.Now().Unix() > claims.Exp {
		return Claims{}, ErrTokenExpired
	}
	return claims, nil
}

func sign(data, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("mustJSON: %v", err))
	}
	return b
}
