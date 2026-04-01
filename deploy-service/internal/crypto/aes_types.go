package crypto

// Service шифрует и расшифровывает данные с помощью AES-256-GCM.
// Nonce добавляется в начало шифртекста, после чего вся последовательность кодируется в base64.
type Service struct {
	key []byte
}
