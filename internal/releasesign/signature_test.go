package releasesign

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestKeyParsingAndSignatureBoundaries(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})
	if parsed, err := ParsePublicKey(publicPEM); err != nil || !bytes.Equal(parsed, pub) {
		t.Fatalf("public key: %v", err)
	}
	if parsed, err := ParsePrivateKey(privatePEM); err != nil || !bytes.Equal(parsed, priv) {
		t.Fatalf("private key: %v", err)
	}
	for _, data := range [][]byte{nil, privatePEM, append(append([]byte(nil), publicPEM...), publicPEM...), append(append([]byte(nil), publicPEM...), []byte("junk")...), bytes.Repeat([]byte("x"), MaxKeyBytes+1)} {
		if _, err := ParsePublicKey(data); err == nil {
			t.Fatal("invalid public key accepted")
		}
	}
	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&other.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePublicKey(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})); err == nil {
		t.Fatal("non-Ed25519 public key accepted")
	}
	der, err = x509.MarshalPKCS8PrivateKey(other)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{nil, publicPEM, append(append([]byte(nil), privatePEM...), privatePEM...), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})} {
		if _, err := ParsePrivateKey(data); err == nil {
			t.Fatal("invalid private key accepted")
		}
	}
	manifest := []byte("manifest\n")
	sig, err := Sign(priv, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(pub, manifest, sig); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{nil, sig[:63], append(append([]byte(nil), sig...), 0)} {
		if err := Verify(pub, manifest, bad); err == nil {
			t.Fatal("invalid signature accepted")
		}
	}
	if err := Verify(nil, manifest, sig); err == nil {
		t.Fatal("invalid public key accepted")
	}
	if _, err := Sign(nil, manifest); err == nil {
		t.Fatal("invalid private key accepted")
	}
	for _, bad := range [][]byte{nil, bytes.Repeat([]byte("x"), MaxManifestBytes+1)} {
		if _, err := Sign(priv, bad); err == nil {
			t.Fatal("invalid manifest accepted")
		}
		if err := Verify(pub, bad, sig); err == nil {
			t.Fatal("invalid manifest accepted")
		}
	}
	if err := Verify(pub, append(manifest, '\n'), sig); err == nil {
		t.Fatal("changed bytes accepted")
	}
}

func TestReadFileBounds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input")
	if _, err := ReadFile(dir, 8); err == nil {
		t.Fatal("directory accepted")
	}
	for _, size := range []int{0, 8, 9} {
		if err := os.WriteFile(path, bytes.Repeat([]byte("x"), size), 0600); err != nil {
			t.Fatal(err)
		}
		data, err := ReadFile(path, 8)
		if size <= 8 {
			if err != nil || len(data) != size {
				t.Fatalf("size %d: %v", size, err)
			}
		} else if err == nil || data != nil {
			t.Fatal("oversized file accepted")
		}
	}
}
