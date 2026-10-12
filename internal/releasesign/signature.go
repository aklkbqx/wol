// Package releasesign verifies detached Ed25519 signatures over exact checksum
// manifest bytes. Trust comes from a public key obtained outside the download.
package releasesign

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
)

const MaxManifestBytes = 1 << 20
const MaxKeyBytes = 16 << 10

// ReadFile bounds local input and rejects directories and other special files.
func ReadFile(path string, limit int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("input must be a regular file")
	}
	if info.Size() > limit {
		return nil, errors.New("input exceeds size limit")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("input exceeds size limit")
	}
	return data, nil
}

func decodeKey(data []byte, kind string) ([]byte, error) {
	if len(data) > MaxKeyBytes {
		return nil, errors.New("key exceeds size limit")
	}
	data = bytes.TrimSpace(data)
	if !bytes.HasPrefix(data, []byte("-----BEGIN "+kind+"-----")) {
		return nil, fmt.Errorf("expected a single %s PEM block", kind)
	}
	block, rest := pem.Decode(data)
	if block == nil || block.Type != kind || len(block.Headers) != 0 || len(bytes.TrimSpace(rest)) != 0 {
		return nil, fmt.Errorf("expected a single %s PEM block", kind)
	}
	return block.Bytes, nil
}

func ParsePublicKey(data []byte) (ed25519.PublicKey, error) {
	der, err := decodeKey(data, "PUBLIC KEY")
	if err != nil {
		return nil, err
	}
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, errors.New("invalid PKIX public key")
	}
	key, ok := parsed.(ed25519.PublicKey)
	if !ok || len(key) != ed25519.PublicKeySize {
		return nil, errors.New("public key must be Ed25519")
	}
	return key, nil
}

// Fingerprint is the lowercase hex SHA-256 of the key's PKIX DER encoding, the
// same value as `openssl pkey -pubin -outform DER | sha256sum`.
func Fingerprint(key ed25519.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:]), nil
}

func ParsePrivateKey(data []byte) (ed25519.PrivateKey, error) {
	der, err := decodeKey(data, "PRIVATE KEY")
	if err != nil {
		return nil, err
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, errors.New("invalid PKCS8 private key")
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok || len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("private key must be Ed25519")
	}
	return key, nil
}

func Sign(key ed25519.PrivateKey, manifest []byte) ([]byte, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid Ed25519 private key")
	}
	if len(manifest) == 0 || len(manifest) > MaxManifestBytes {
		return nil, errors.New("invalid manifest size")
	}
	return ed25519.Sign(key, manifest), nil
}

func Verify(key ed25519.PublicKey, manifest, signature []byte) error {
	if len(key) != ed25519.PublicKeySize {
		return errors.New("invalid Ed25519 public key")
	}
	if len(manifest) == 0 || len(manifest) > MaxManifestBytes {
		return errors.New("invalid manifest size")
	}
	if len(signature) != ed25519.SignatureSize || !ed25519.Verify(key, manifest, signature) {
		return errors.New("invalid manifest signature")
	}
	return nil
}
