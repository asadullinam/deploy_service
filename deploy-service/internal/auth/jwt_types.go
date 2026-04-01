package auth

type Claims struct {
	UserID string `json:"sub"`
	Email  string `json:"email"`
	Exp    int64  `json:"exp"`
}
