package localremote

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

type authDocument struct {
	Username    string                    `json:"username"`
	Expires     int64                     `json:"expires"`
	Connections map[string]authConnection `json:"connections"`
}

type authConnection struct {
	Protocol   string            `json:"protocol"`
	Parameters map[string]string `json:"parameters"`
}

func randomBytes(n int) ([]byte, error) {
	value := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return nil, fmt.Errorf("generate secure random value: %w", err)
	}
	return value, nil
}

func randomHex(n int) (string, error) {
	value, err := randomBytes(n)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func buildAuthToken(key []byte, cfg Config, credentials Credentials, now time.Time) (string, error) {
	parameters := map[string]string{
		"hostname": cfg.Host,
		"port":     fmt.Sprintf("%d", cfg.Port),
	}
	if credentials.Username != "" {
		parameters["username"] = credentials.Username
	}
	if credentials.Domain != "" && cfg.Protocol == "rdp" {
		parameters["domain"] = credentials.Domain
	}
	if credentials.Password != "" {
		parameters["password"] = credentials.Password
	}
	if cfg.Protocol == "rdp" && cfg.CertificatePolicy == "trust-local" {
		parameters["ignore-cert"] = "true"
	}
	document := authDocument{
		Username: "local-wol-session",
		Expires:  now.Add(2 * time.Minute).UnixMilli(),
		Connections: map[string]authConnection{
			"Remote session": {Protocol: cfg.Protocol, Parameters: parameters},
		},
	}
	plaintext, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("encode Guacamole launch document: %w", err)
	}
	defer clear(plaintext)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(plaintext)
	signed := append(mac.Sum(nil), plaintext...)
	defer clear(signed)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create Guacamole token cipher: %w", err)
	}
	pad := aes.BlockSize - len(signed)%aes.BlockSize
	for range pad {
		signed = append(signed, byte(pad))
	}
	ciphertext := make([]byte, len(signed))
	cipher.NewCBCEncrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(ciphertext, signed)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}
