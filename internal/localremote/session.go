// Package localremote runs an optional, short-lived browser remote desktop on
// localhost. It never exposes Guacamole or guacd beyond the local machine.
package localremote

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Opener opens the generated loopback URL in the user's browser.
type Opener func(context.Context, string) error

// Config describes a single local browser-remote session.
type Config struct {
	Protocol          string
	Host              string
	Port              int
	UsernameHint      string
	DomainHint        string
	CertificatePolicy string
	Runner            Runner
	Opener            Opener
	OpenBrowser       bool
}

// Credentials live only in the loopback broker for the few milliseconds needed
// to build an encrypted, short-lived Guacamole launch token. They are never
// persisted by WOL.
type Credentials struct {
	Username string
	Domain   string
	Password string
}

// Session is a running localhost broker and its private Docker sidecars.
type Session struct {
	URL       string
	closeOnce sync.Once
	closeFn   func() error
	closeErr  error
}

// Close terminates the HTTP broker, containers, and private Docker network.
func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		if s.closeFn != nil {
			s.closeErr = s.closeFn()
		}
	})
	return s.closeErr
}

// Start launches isolated Guacamole sidecars and a loopback-only HTTP broker.
func Start(ctx context.Context, cfg Config) (*Session, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	runner := runnerOrDefault(cfg.Runner)
	id, err := randomHex(8)
	if err != nil {
		return nil, err
	}
	key, err := randomBytes(16)
	if err != nil {
		return nil, err
	}
	docker := &dockerRuntime{
		runner: runner, network: "wol-local-" + id,
		guacd: "wol-guacd-" + id, guacamole: "wol-guacamole-" + id,
		secretHex: fmt.Sprintf("%x", key),
	}
	upstreamURL, err := docker.start(ctx)
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return nil, joinErrors(err, docker.close(cleanupCtx))
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return nil, joinErrors(fmt.Errorf("start localhost broker: %w", err), docker.close(cleanupCtx))
	}
	addr := listener.Addr().(*net.TCPAddr)
	expectedHost := net.JoinHostPort("127.0.0.1", strconv.Itoa(addr.Port))
	oneTimeToken, err := randomHex(32)
	if err != nil {
		_ = listener.Close()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return nil, joinErrors(err, docker.close(cleanupCtx))
	}
	cookieToken, err := randomHex(32)
	if err != nil {
		_ = listener.Close()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return nil, joinErrors(err, docker.close(cleanupCtx))
	}
	upstream, _ := url.Parse(upstreamURL)
	broker := newBroker(expectedHost, oneTimeToken, cookieToken, cfg, key, upstream)
	server := &http.Server{
		Handler:           broker,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = server.Serve(listener)
	}()
	session := &Session{URL: "http://" + expectedHost + "/s/" + oneTimeToken}
	session.closeFn = func() error {
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 15*time.Second)
		httpErr := server.Shutdown(shutdownCtx)
		cancelShutdown()
		<-done

		// Give sidecar cleanup its own deadline. If graceful HTTP shutdown uses
		// its full allowance, reusing shutdownCtx would make every Docker cleanup
		// command start with an already-expired context and leak local resources.
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelCleanup()
		return joinErrors(httpErr, docker.close(cleanupCtx))
	}
	go func() {
		select {
		case <-ctx.Done():
			_ = session.Close()
		case <-done:
			_ = session.Close()
		}
	}()
	if cfg.OpenBrowser {
		if cfg.Opener == nil {
			_ = session.Close()
			return nil, errors.New("local remote: OpenBrowser requires an Opener")
		}
		if err := cfg.Opener(ctx, session.URL); err != nil {
			_ = session.Close()
			return nil, fmt.Errorf("open localhost remote session: %w", err)
		}
	}
	return session, nil
}

func validateConfig(cfg Config) error {
	switch cfg.Protocol {
	case "rdp", "vnc", "ssh":
	default:
		return fmt.Errorf("local remote: unsupported protocol %q (use rdp, vnc, or ssh)", cfg.Protocol)
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return fmt.Errorf("local remote: invalid port %d", cfg.Port)
	}
	host := strings.TrimSpace(cfg.Host)
	if host == "" || len(host) > 253 || strings.ContainsAny(host, "/\\@?#") {
		return errors.New("local remote: invalid target host")
	}
	if strings.TrimSpace(cfg.UsernameHint) != cfg.UsernameHint || len(cfg.UsernameHint) > 128 || strings.ContainsAny(cfg.UsernameHint, "\r\n\x00") {
		return errors.New("local remote: invalid username hint")
	}
	if strings.TrimSpace(cfg.DomainHint) != cfg.DomainHint || len(cfg.DomainHint) > 128 || strings.ContainsAny(cfg.DomainHint, "\r\n\x00") {
		return errors.New("local remote: invalid domain hint")
	}
	if cfg.CertificatePolicy == "" {
		cfg.CertificatePolicy = "strict"
	}
	if cfg.CertificatePolicy != "strict" && cfg.CertificatePolicy != "trust-local" {
		return errors.New("local remote: certificate policy must be strict or trust-local")
	}
	if cfg.CertificatePolicy == "trust-local" && cfg.Protocol != "rdp" {
		return errors.New("local remote: trust-local certificate policy is only valid for RDP")
	}
	return nil
}

type brokerHandler struct {
	expectedHost string
	oneTimeToken string
	cookieToken  string
	config       Config
	key          []byte
	launchToken  string
	proxy        *httputil.ReverseProxy
	mu           sync.Mutex
	consumed     bool
}

func newBroker(expectedHost, oneTimeToken, cookieToken string, cfg Config, key []byte, upstream *url.URL) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = upstream.Host
		req.Header.Set("X-Forwarded-Host", expectedHost)
		req.Header.Set("X-Forwarded-Proto", "http")
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "Remote engine is still starting. Please retry shortly.", http.StatusBadGateway)
	}
	proxy.ModifyResponse = func(response *http.Response) error {
		location := response.Header.Get("Location")
		if location == "" {
			return nil
		}
		parsed, err := url.Parse(location)
		if err != nil {
			return nil
		}
		if parsed.IsAbs() && parsed.Host == upstream.Host {
			parsed.Scheme = "http"
			parsed.Host = expectedHost
			response.Header.Set("Location", parsed.String())
		}
		return nil
	}
	return &brokerHandler{expectedHost: expectedHost, oneTimeToken: oneTimeToken, cookieToken: cookieToken, config: cfg, key: append([]byte(nil), key...), proxy: proxy}
}

func (b *brokerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w, b.expectedHost)
	if r.Host != b.expectedHost || !validOrigin(r, b.expectedHost) {
		http.Error(w, "Invalid local session origin.", http.StatusForbidden)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/s/") {
		b.consume(w, r)
		return
	}
	if !b.validCookie(r) {
		http.Error(w, "Local remote session is unavailable.", http.StatusUnauthorized)
		return
	}
	switch {
	case r.URL.Path == "/session":
		serveLoginPage(w, b.config, "")
	case r.URL.Path == "/connect":
		b.connect(w, r)
	case r.URL.Path == "/remote":
		b.remote(w, r)
	case r.URL.Path == "/healthz":
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	case strings.HasPrefix(r.URL.Path, "/guacamole/") || r.URL.Path == "/guacamole":
		b.proxy.ServeHTTP(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (b *brokerHandler) connect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed.", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		serveLoginPage(w, b.config, "Could not read the credentials. Please try again.")
		return
	}
	credentials := Credentials{
		Username: strings.TrimSpace(r.Form.Get("username")),
		Domain:   strings.TrimSpace(r.Form.Get("domain")),
		Password: r.Form.Get("password"),
	}
	if err := validateCredentials(b.config.Protocol, credentials); err != nil {
		serveLoginPage(w, b.config, err.Error())
		return
	}
	launchToken, err := buildAuthToken(b.key, b.config, credentials, time.Now())
	credentials.Password = ""
	if err != nil {
		http.Error(w, "Unable to prepare the encrypted local session.", http.StatusInternalServerError)
		return
	}
	b.mu.Lock()
	b.launchToken = launchToken
	b.mu.Unlock()
	http.Redirect(w, r, "/remote", http.StatusSeeOther)
}

func validateCredentials(protocol string, credentials Credentials) error {
	if len(credentials.Username) > 128 || strings.ContainsAny(credentials.Username, "\r\n\x00") {
		return errors.New("Username is invalid.")
	}
	if len(credentials.Domain) > 128 || strings.ContainsAny(credentials.Domain, "\r\n\x00") {
		return errors.New("Domain is invalid.")
	}
	if len(credentials.Password) > 4096 || strings.ContainsRune(credentials.Password, '\x00') {
		return errors.New("Password is invalid.")
	}
	if protocol == "ssh" && credentials.Username == "" {
		return errors.New("SSH requires a username.")
	}
	return nil
}

func (b *brokerHandler) remote(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	launchToken := b.launchToken
	b.launchToken = ""
	b.mu.Unlock()
	if launchToken == "" {
		http.Redirect(w, r, "/session", http.StatusSeeOther)
		return
	}
	servePage(w, launchToken)
}

func (b *brokerHandler) consume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed.", http.StatusMethodNotAllowed)
		return
	}
	presented := strings.TrimPrefix(r.URL.Path, "/s/")
	b.mu.Lock()
	valid := !b.consumed && len(presented) == len(b.oneTimeToken) && subtle.ConstantTimeCompare([]byte(presented), []byte(b.oneTimeToken)) == 1
	if valid {
		b.consumed = true
		b.oneTimeToken = ""
	}
	b.mu.Unlock()
	if !valid {
		http.Error(w, "This one-time session link has expired.", http.StatusGone)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "wol_local_session", Value: b.cookieToken, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 900,
	})
	http.Redirect(w, r, "/session", http.StatusSeeOther)
}

func (b *brokerHandler) validCookie(r *http.Request) bool {
	cookie, err := r.Cookie("wol_local_session")
	return err == nil && len(cookie.Value) == len(b.cookieToken) && subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(b.cookieToken)) == 1
}

func validOrigin(r *http.Request, host string) bool {
	origin := r.Header.Get("Origin")
	return origin == "" || origin == "http://"+host
}

func setSecurityHeaders(w http.ResponseWriter, host string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; worker-src 'self' blob:; connect-src 'self' ws://"+host+"; frame-src 'self'; frame-ancestors 'self'; base-uri 'none'; form-action 'self'")
}
