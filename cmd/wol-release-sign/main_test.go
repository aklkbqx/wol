package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestSigningWorkflow(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	privatePath, publicPath := filepath.Join(dir, "private.pem"), filepath.Join(dir, "public.pem")
	manifestPath, signaturePath := filepath.Join(dir, "SHA256SUMS"), filepath.Join(dir, "SHA256SUMS.sig")
	manifest := []byte("exact manifest bytes\n")
	for path, data := range map[string][]byte{
		privatePath:  pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}),
		publicPath:   pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}),
		manifestPath: manifest,
	} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	args := []string{"sign", "--key", privatePath, "--manifest", manifestPath, "--output", signaturePath}
	if err := run(args, &out); err != nil {
		t.Fatal(err)
	}
	sig, err := os.ReadFile(signaturePath)
	if err != nil {
		t.Fatal(err)
	}
	if !ed25519.Verify(pub, manifest, sig) {
		t.Fatal("signature is not compatible with exact-byte Ed25519 verification")
	}
	if err := run(args, &out); err == nil {
		t.Fatal("existing signature overwritten")
	}
	unchanged, err := os.ReadFile(signaturePath)
	if err != nil || !bytes.Equal(unchanged, sig) {
		t.Fatal("refusal changed signature")
	}
	verify := []string{"verify", "--key", publicPath, "--manifest", manifestPath, "--signature", signaturePath}
	if err := run(verify, &out); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(manifest, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(verify, &out); err == nil {
		t.Fatal("modified manifest accepted")
	}
	if bytes.Contains(out.Bytes(), privateDER) {
		t.Fatal("private key printed")
	}
}

func TestFingerprintPrintsSHA256OfPublicKeyDER(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	publicPath, privatePath := filepath.Join(dir, "public.pem"), filepath.Join(dir, "private.pem")
	if err := os.WriteFile(publicPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(privatePath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"fingerprint", "--key", publicPath}, &out); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(publicDER)
	if got, want := out.String(), hex.EncodeToString(sum[:])+"\n"; got != want {
		t.Fatalf("fingerprint output = %q, want %q", got, want)
	}
	if err := run([]string{"fingerprint", "--key", privatePath}, &out); err == nil {
		t.Fatal("fingerprint accepted a private key")
	}
}
