package app

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"personalcloud/internal/config"
	"personalcloud/internal/store"
)

func TestBootstrapFlow(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	application, err := New(config.Config{Addr: ":0", DataDir: t.TempDir(), SessionTTL: time.Hour}, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	handler := application.Handler()

	home := httptest.NewRecorder()
	handler.ServeHTTP(home, httptest.NewRequest(http.MethodGet, "/", nil))
	if home.Code != http.StatusSeeOther || home.Header().Get("Location") != "/setup" {
		t.Fatalf("inicio debía redirigir a setup: status=%d location=%q", home.Code, home.Header().Get("Location"))
	}

	setupPage := httptest.NewRecorder()
	handler.ServeHTTP(setupPage, httptest.NewRequest(http.MethodGet, "/setup", nil))
	if setupPage.Code != http.StatusOK {
		t.Fatalf("setup GET: %d", setupPage.Code)
	}
	csrf := findCookie(setupPage.Result().Cookies(), csrfCookieName)
	if csrf == nil {
		t.Fatal("setup no emitió cookie CSRF")
	}

	post := func(code string) *httptest.ResponseRecorder {
		form := url.Values{
			"csrf_token":            {csrf.Value},
			"setup_code":            {code},
			"username":              {"admin"},
			"password":              {"contraseña-segura-12345"},
			"password_confirmation": {"contraseña-segura-12345"},
		}
		req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(csrf)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}

	wrong := post("AAAA-BBBB-CCCC")
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("código incorrecto debía fallar: %d", wrong.Code)
	}

	correct := post(application.setupCode)
	if correct.Code != http.StatusSeeOther || correct.Header().Get("Location") != "/bienvenida" {
		t.Fatalf("setup correcto: status=%d location=%q", correct.Code, correct.Header().Get("Location"))
	}
	if findCookie(correct.Result().Cookies(), sessionCookieName) == nil {
		t.Fatal("setup correcto no creó sesión")
	}

	after := httptest.NewRecorder()
	handler.ServeHTTP(after, httptest.NewRequest(http.MethodGet, "/setup", nil))
	if after.Code != http.StatusSeeOther || after.Header().Get("Location") != "/iniciar-sesion" {
		t.Fatalf("setup debía quedar cerrado: status=%d location=%q", after.Code, after.Header().Get("Location"))
	}
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func TestClientIPUsesRightmostUntrustedHop(t *testing.T) {
	_, loopback, err := net.ParseCIDR("127.0.0.1/32")
	if err != nil {
		t.Fatal(err)
	}
	application := &App{cfg: config.Config{TrustedProxyNets: []*net.IPNet{loopback}}}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:4567"
	req.Header.Set("X-Forwarded-For", "198.51.100.99, 203.0.113.7")

	if got := application.clientIP(req); got != "203.0.113.7" {
		t.Fatalf("IP inesperada: %q", got)
	}
}
