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
	Protocol     string
	Host         string
	Port         int
	UsernameHint string
	Runner       Runner
	Opener       Opener
	OpenBrowser  bool
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
	launchToken, err := buildAuthToken(key, cfg, time.Now())
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
	broker := newBroker(expectedHost, oneTimeToken, cookieToken, launchToken, upstream)
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
	return nil
}

type brokerHandler struct {
	expectedHost string
	oneTimeToken string
	cookieToken  string
	launchToken  string
	proxy        *httputil.ReverseProxy
	mu           sync.Mutex
	consumed     bool
}

func newBroker(expectedHost, oneTimeToken, cookieToken, launchToken string, upstream *url.URL) http.Handler {
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
	return &brokerHandler{expectedHost: expectedHost, oneTimeToken: oneTimeToken, cookieToken: cookieToken, launchToken: launchToken, proxy: proxy}
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
		servePage(w, b.launchToken)
	case r.URL.Path == "/healthz":
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	case strings.HasPrefix(r.URL.Path, "/guacamole/") || r.URL.Path == "/guacamole":
		b.proxy.ServeHTTP(w, r)
	default:
		http.NotFound(w, r)
	}
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
