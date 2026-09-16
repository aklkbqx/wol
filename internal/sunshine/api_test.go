package sunshine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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

	client := NewClient(u.Hostname(), port, "admin", "secret")
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
	client := NewClient(u.Hostname(), port, "", "")
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

	client := NewClient(u.Hostname(), port, "", "")
	client.HTTPClient = ts.Client()

	err := client.Pair(context.Background(), "9999", "mac")
	if err == nil {
		t.Fatal("expected error on 401 Unauthorized, got nil")
	}
}
