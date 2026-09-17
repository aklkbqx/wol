package sunshine

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultAdminPort      = 47990
	DefaultGameStreamPort = 47989
	DefaultRTSPPort       = 48010
)

// Client interacts with the Sunshine Web Administration REST API.
type Client struct {
	Host       string
	Port       int
	Username   string
	Password   string
	HTTPClient *http.Client
}

// NewClient creates a Sunshine API client with TLS verification skipped for self-signed certificates.
func NewClient(host string, port int, username, password string) *Client {
	if port <= 0 {
		port = DefaultAdminPort
	}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return &Client{
		Host:     strings.TrimSpace(host),
		Port:     port,
		Username: strings.TrimSpace(username),
		Password: strings.TrimSpace(password),
		HTTPClient: &http.Client{
			Transport: transport,
			Timeout:   5 * time.Second,
		},
	}
}

// Pair submits a 4-digit Moonlight PIN to Sunshine's /api/pin endpoint.
func (c *Client) Pair(ctx context.Context, pin string, clientName string) error {
	pin = strings.TrimSpace(pin)
	if pin == "" {
		return errors.New("pairing PIN is required")
	}
	if clientName == "" {
		clientName = "wol-client"
	}

	payload, err := json.Marshal(map[string]string{
		"pin":  pin,
		"name": clientName,
	})
	if err != nil {
		return fmt.Errorf("marshal pair payload: %w", err)
	}

	url := fmt.Sprintf("https://%s/api/pin", net.JoinHostPort(c.Host, strconv.Itoa(c.Port)))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create pairing request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token := c.csrfToken(ctx); token != "" {
		req.Header.Set("X-CSRF-Token", token)
	}
	if c.Username != "" || c.Password != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("connect to Sunshine API at %s:%d: %w", c.Host, c.Port, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("pairing failed: Sunshine returned HTTP %d", resp.StatusCode)
	}
	if !pairStatusOK(body) {
		return errors.New("pairing failed: Sunshine rejected the PIN")
	}
	return nil
}

func (c *Client) csrfToken(ctx context.Context) string {
	if c == nil || c.HTTPClient == nil {
		return ""
	}
	url := fmt.Sprintf("https://%s/api/csrf-token", net.JoinHostPort(c.Host, strconv.Itoa(c.Port)))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	if c.Username != "" || c.Password != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var payload struct {
		Token string `json:"csrf_token"`
		CSRF  string `json:"csrfToken"`
	}
	if json.NewDecoder(resp.Body).Decode(&payload) != nil {
		return ""
	}
	if payload.Token != "" {
		return payload.Token
	}
	return payload.CSRF
}

func pairStatusOK(body []byte) bool {
	if len(bytes.TrimSpace(body)) == 0 {
		return true
	}
	var payload struct {
		Status json.RawMessage `json:"status"`
	}
	if json.Unmarshal(body, &payload) != nil || len(payload.Status) == 0 {
		return true
	}
	var flag bool
	if json.Unmarshal(payload.Status, &flag) == nil {
		return flag
	}
	var text string
	if json.Unmarshal(payload.Status, &text) == nil {
		return text == "true" || text == "1"
	}
	return true
}

// Probe checks if a TCP port on the Sunshine host is accepting connections.
func Probe(ctx context.Context, host string, port int) bool {
	if port <= 0 {
		port = DefaultGameStreamPort
	}
	probeCtx, cancel := context.WithTimeout(ctx, 900*time.Millisecond)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(probeCtx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err == nil {
		_ = conn.Close()
		return true
	}
	return false
}
