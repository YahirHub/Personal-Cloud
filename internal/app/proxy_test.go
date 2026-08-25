package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"personalcloud/internal/auth"
	"personalcloud/internal/config"
	"personalcloud/internal/store"
)

func TestHTTPSMiddlewareRejectsUnverifiedHTTP(t *testing.T) {
	application := &App{cfg: config.Config{RequireHTTPS: true}, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	req := httptest.NewRequest(http.MethodGet, "http://localhost/iniciar-sesion", nil)
	rr := httptest.NewRecorder()
	application.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusPermanentRedirect {
		t.Fatalf("HTTP no verificado debía redirigirse: status=%d", rr.Code)
	}
}

func TestRequestIsHTTPSRejectsForwardedHeadersFromUntrustedPeer(t *testing.T) {
	application := &App{cfg: config.Config{TrustAllProxies: false}}
	req := httptest.NewRequest(http.MethodGet, "http://localhost/iniciar-sesion", nil)
	req.RemoteAddr = "198.51.100.10:4567"
	req.Header.Set("X-Forwarded-Proto", "https")
	if application.requestIsHTTPS(req) {
		t.Fatal("un peer no confiable no debe poder declarar HTTPS mediante forwarded headers")
	}
}

func TestRequestIsHTTPSAcceptsTrustedProxyHeaders(t *testing.T) {
	application := &App{cfg: config.Config{TrustAllProxies: true}}

	cases := []struct {
		name   string
		header string
		value  string
	}{
		{name: "x-forwarded-proto", header: "X-Forwarded-Proto", value: "https"},
		{name: "forwarded", header: "Forwarded", value: `for=192.0.2.10;proto=https`},
		{name: "cloudflare-visitor", header: "CF-Visitor", value: `{"scheme":"https"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://localhost/iniciar-sesion", nil)
			req.Header.Set(tc.header, tc.value)
			if !application.requestIsHTTPS(req) {
				t.Fatalf("la petición proxy debía reconocerse como HTTPS: %s=%q", tc.header, tc.value)
			}
		})
	}
}

func TestLoginBehindTrustedHTTPSProxyKeepsSession(t *testing.T) {
	storePath := t.TempDir() + "/state.json"
	db, err := store.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	password := "contraseña-segura-12345"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	user, err := db.CreateFirstAdmin(context.Background(), "admin", hash)
	if err != nil {
		t.Fatal(err)
	}
	user.OnboardingCompleted = true
	if err := db.CompleteOnboarding(context.Background(), user.ID); err != nil {
		t.Fatal(err)
	}

	application, err := New(config.Config{
		Addr:            ":0",
		DataDir:         t.TempDir(),
		SessionTTL:      time.Hour,
		CookieSecure:    true,
		RequireHTTPS:    true,
		TrustAllProxies: true,
	}, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()

	// Execute directly against the handler rather than the client transport.
	page := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://localhost/iniciar-sesion", nil)
	request.RemoteAddr = "127.0.0.1:34567"
	request.Header.Set("X-Forwarded-Proto", "https")
	application.Handler().ServeHTTP(page, request)
	csrf := findCookie(page.Result().Cookies(), csrfCookieName)
	if page.Code != http.StatusOK || csrf == nil {
		t.Fatalf("login GET detrás de proxy: status=%d csrf=%v", page.Code, csrf != nil)
	}

	form := url.Values{
		"csrf_token": {csrf.Value},
		"username":   {"admin"},
		"password":   {password},
	}
	post := httptest.NewRecorder()
	postReq := httptest.NewRequest(http.MethodPost, "http://localhost/iniciar-sesion", strings.NewReader(form.Encode()))
	postReq.RemoteAddr = "127.0.0.1:34567"
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.Header.Set("X-Forwarded-Proto", "https")
	postReq.AddCookie(csrf)
	application.Handler().ServeHTTP(post, postReq)
	if post.Code != http.StatusOK {
		t.Fatalf("login detrás de proxy: status=%d body=%s", post.Code, post.Body.String())
	}
	if post.Header().Get("Refresh") != "0; url=/inicio" {
		t.Fatalf("login detrás de proxy: refresh=%q", post.Header().Get("Refresh"))
	}
	if post.Header().Get("Cache-Control") != "no-store, no-cache, must-revalidate, private" {
		t.Fatalf("login debe impedir cache: Cache-Control=%q", post.Header().Get("Cache-Control"))
	}
	if post.Header().Get("X-PC-Session-Created") != "1" {
		t.Fatalf("login debe indicar que la sesión fue emitida")
	}
	if session := findCookie(post.Result().Cookies(), sessionCookieName); session == nil || !session.Secure {
		t.Fatalf("login no emitió una cookie Secure de sesión")
	}
}

func TestLoginSuccessUses200ResponseForSessionCookie(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	password := "contraseña-segura-12345"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	user, err := db.CreateFirstAdmin(context.Background(), "admin", hash)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteOnboarding(context.Background(), user.ID); err != nil {
		t.Fatal(err)
	}

	application, err := New(config.Config{
		Addr: ":0", DataDir: t.TempDir(), SessionTTL: time.Hour,
		CookieSecure: true, RequireHTTPS: true, TrustAllProxies: true,
	}, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()

	page := httptest.NewRecorder()
	get := httptest.NewRequest(http.MethodGet, "http://localhost/iniciar-sesion", nil)
	get.RemoteAddr = "127.0.0.1:34567"
	get.Header.Set("X-Forwarded-Proto", "https")
	application.Handler().ServeHTTP(page, get)
	csrf := findCookie(page.Result().Cookies(), csrfCookieName)
	if csrf == nil {
		t.Fatal("no se emitió CSRF")
	}

	form := url.Values{"csrf_token": {csrf.Value}, "username": {"admin"}, "password": {password}}
	post := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/iniciar-sesion", strings.NewReader(form.Encode()))
	req.RemoteAddr = "127.0.0.1:34567"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(csrf)
	application.Handler().ServeHTTP(post, req)

	if post.Code != http.StatusOK {
		t.Fatalf("login debía devolver 200: %d", post.Code)
	}
	if post.Header().Get("Refresh") != "0; url=/inicio" {
		t.Fatalf("refresh inesperado: %q", post.Header().Get("Refresh"))
	}
	if post.Header().Get("X-PC-Login-Established") != "1" {
		t.Fatal("faltó indicador de sesión establecida")
	}
	session := findCookie(post.Result().Cookies(), sessionCookieName)
	if session == nil || !session.Secure || !session.HttpOnly {
		t.Fatalf("cookie de sesión inválida: %+v", session)
	}
}
