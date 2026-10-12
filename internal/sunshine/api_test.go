package sunshine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPairSuccess(t *testing.T) {
	var receivedPin, receivedName string
	var authHeader string

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pin" {
			http.NotFound(w, r)
			return
		}
		authHeader = r.Header.Get("Authorization")
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		receivedPin = body["pin"]
		receivedName = body["name"]
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	port, _ := strconv.Atoi(u.Port())

	client := NewClient(u.Hostname(), port, "admin", "secret", "")
	client.HTTPClient = ts.Client()

	err := client.Pair(context.Background(), "1234", "my-mac")
	if err != nil {
		t.Fatalf("Pair failed: %v", err)
	}

	if receivedPin != "1234" {
		t.Errorf("got pin %s, want 1234", receivedPin)
	}
	if receivedName != "my-mac" {
		t.Errorf("got name %s, want my-mac", receivedName)
	}
	if authHeader == "" {
		t.Errorf("expected basic auth header, got none")
	}
}

func TestPairRejectedStatus(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":false}`))
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	port, _ := strconv.Atoi(u.Port())
	client := NewClient(u.Hostname(), port, "", "", "")
	client.HTTPClient = ts.Client()
	if err := client.Pair(context.Background(), "0000", "mac"); err == nil {
		t.Fatal("expected error when Sunshine status is false")
	}
}

func TestPairHTTPError(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	port, _ := strconv.Atoi(u.Port())

	client := NewClient(u.Hostname(), port, "", "", "")
	client.HTTPClient = ts.Client()

	err := client.Pair(context.Background(), "9999", "mac")
	if err == nil {
		t.Fatal("expected error on 401 Unauthorized, got nil")
	}
}

func newQuietTLSServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, string, int) {
	t.Helper()
	ts := httptest.NewUnstartedServer(handler)
	ts.Config.ErrorLog = log.New(io.Discard, "", 0)
	ts.StartTLS()
	t.Cleanup(ts.Close)
	u, _ := url.Parse(ts.URL)
	port, _ := strconv.Atoi(u.Port())
	return ts, u.Hostname(), port
}

func serverFingerprint(ts *httptest.Server) string {
	sum := sha256.Sum256(ts.Certificate().Raw)
	return hex.EncodeToString(sum[:])
}

func TestPairRefusesUntrustedCertificateBeforeSendingCredentials(t *testing.T) {
	var requests atomic.Int32
	ts, host, port := newQuietTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	err := NewClient(host, port, "admin", "secret", "").Pair(context.Background(), "1234", "mac")
	var untrusted *UntrustedCertificateError
	if !errors.As(err, &untrusted) {
		t.Fatalf("Pair error = %v, want UntrustedCertificateError", err)
	}
	if untrusted.Fingerprint != serverFingerprint(ts) {
		t.Fatalf("reported fingerprint %s, server certificate is %s", untrusted.Fingerprint, serverFingerprint(ts))
	}
	if n := requests.Load(); n != 0 {
		t.Fatalf("server received %d requests over an unverified connection", n)
	}
}

func TestPairWithPinnedCertificateSendsCredentials(t *testing.T) {
	var user, pass string
	ts, host, port := newQuietTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/pin" {
			user, pass, _ = r.BasicAuth()
		}
		w.WriteHeader(http.StatusOK)
	})
	if err := NewClient(host, port, "admin", "secret", serverFingerprint(ts)).Pair(context.Background(), "1234", "mac"); err != nil {
		t.Fatalf("Pair with pinned certificate failed: %v", err)
	}
	if user != "admin" || pass != "secret" {
		t.Fatalf("pinned pairing sent credentials %q/%q, want admin/secret", user, pass)
	}
}

func TestPairRejectsCertificateThatDoesNotMatchPin(t *testing.T) {
	var requests atomic.Int32
	ts, host, port := newQuietTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	err := NewClient(host, port, "admin", "secret", strings.Repeat("0", 64)).Pair(context.Background(), "1234", "mac")
	var untrusted *UntrustedCertificateError
	if !errors.As(err, &untrusted) || untrusted.Fingerprint != serverFingerprint(ts) {
		t.Fatalf("Pair error = %v, want UntrustedCertificateError for %s", err, serverFingerprint(ts))
	}
	if n := requests.Load(); n != 0 {
		t.Fatalf("server received %d requests despite a pin mismatch", n)
	}
}

func TestNormalizeFingerprint(t *testing.T) {
	const want = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	for _, input := range []string{
		"01:23:45:67:89:AB:CD:EF:01:23:45:67:89:AB:CD:EF:01:23:45:67:89:AB:CD:EF:01:23:45:67:89:AB:CD:EF",
		"  0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef\n",
	} {
		got, err := NormalizeFingerprint(input)
		if err != nil || got != want {
			t.Fatalf("NormalizeFingerprint(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"", "0123", strings.Repeat("zz", 32), strings.Repeat("ab", 33)} {
		if got, err := NormalizeFingerprint(input); err == nil {
			t.Fatalf("NormalizeFingerprint(%q) = %q, want error", input, got)
		}
	}
}
