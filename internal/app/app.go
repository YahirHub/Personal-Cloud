package app

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"personalcloud/internal/auth"
	"personalcloud/internal/config"
	"personalcloud/internal/ratelimit"
	"personalcloud/internal/store"
	"personalcloud/internal/webui"
)

type App struct {
	cfg               config.Config
	store             *store.Store
	logger            *slog.Logger
	renderer          *webui.Renderer
	limiter           *ratelimit.Limiter
	setupCode         string
	dummyPasswordHash string
	mux               *http.ServeMux
	stop              chan struct{}
	stopOnce          sync.Once
}

type pageData struct {
	Title       string
	Description string
	CurrentPath string
	CSRFToken   string
	Error       string
	Info        string
	User        *store.User
	RetryAfter  int
}

type contextKey string

const userContextKey contextKey = "user"

var (
	setupPolicy     = ratelimit.Policy{MaxAttempts: 5, Window: 10 * time.Minute}
	loginIPPolicy   = ratelimit.Policy{MaxAttempts: 12, Window: 15 * time.Minute}
	loginUserPolicy = ratelimit.Policy{MaxAttempts: 6, Window: 15 * time.Minute}
)

const (
	auditRetention = 90 * 24 * time.Hour
	auditMaxRows   = 50_000
)

func New(cfg config.Config, storage *store.Store, logger *slog.Logger) (*App, error) {
	renderer, err := webui.NewRenderer()
	if err != nil {
		return nil, err
	}
	dummyHash, err := auth.HashPassword("dummy-password-never-used-123456")
	if err != nil {
		return nil, fmt.Errorf("preparar autenticación: %w", err)
	}

	a := &App{
		cfg:               cfg,
		store:             storage,
		logger:            logger,
		renderer:          renderer,
		limiter:           ratelimit.New(),
		mux:               http.NewServeMux(),
		dummyPasswordHash: dummyHash,
		stop:              make(chan struct{}),
	}

	exists, err := storage.AdminExists(context.Background())
	if err != nil {
		return nil, fmt.Errorf("comprobar administrador: %w", err)
	}
	if !exists {
		a.setupCode, err = auth.NewSetupCode()
		if err != nil {
			return nil, err
		}
		logger.Warn("CONFIGURACIÓN INICIAL REQUERIDA", "url", "/setup", "codigo", a.setupCode)
	}

	a.routes()
	go a.housekeeping()
	return a, nil
}

func (a *App) Handler() http.Handler {
	return a.securityHeaders(a.requestLog(a.mux))
}

func (a *App) Close() {
	a.stopOnce.Do(func() { close(a.stop) })
}

func (a *App) routes() {
	staticFS, err := webui.StaticFS()
	if err != nil {
		panic(err)
	}
	a.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	a.mux.HandleFunc("GET /", a.home)
	a.mux.HandleFunc("GET /setup", a.setupGet)
	a.mux.HandleFunc("POST /setup", a.setupPost)
	a.mux.HandleFunc("GET /iniciar-sesion", a.loginGet)
	a.mux.HandleFunc("POST /iniciar-sesion", a.loginPost)
	a.mux.Handle("POST /cerrar-sesion", a.requireAuth(http.HandlerFunc(a.logoutPost)))
	a.mux.Handle("GET /bienvenida", a.requireAuth(http.HandlerFunc(a.onboardingGet)))
	a.mux.Handle("POST /bienvenida/completar", a.requireAuth(http.HandlerFunc(a.onboardingCompletePost)))
	a.mux.Handle("GET /inicio", a.requireAuth(http.HandlerFunc(a.dashboardGet)))
	a.mux.Handle("GET /almacenamiento", a.requireAuth(http.HandlerFunc(a.storageGet)))
	a.mux.Handle("GET /fotos", a.requireAuth(http.HandlerFunc(a.photosGet)))
}

func (a *App) housekeeping() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-a.stop:
			return
		case <-ticker.C:
			a.limiter.Cleanup()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := a.store.DeleteExpiredSessions(ctx); err != nil {
				a.logger.Warn("no se pudieron limpiar sesiones vencidas", "error", err)
			}
			if err := a.store.CleanupAudit(ctx, auditRetention, auditMaxRows); err != nil {
				a.logger.Warn("no se pudo limpiar la auditoría", "error", err)
			}
			cancel()
		}
	}
}

func (a *App) render(w http.ResponseWriter, status int, page string, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := a.renderer.Render(w, page, data); err != nil {
		a.logger.Error("error renderizando plantilla", "page", page, "error", err)
	}
}

func (a *App) csrfData(w http.ResponseWriter, r *http.Request, data pageData) pageData {
	data.CSRFToken = a.ensureCSRF(w, r)
	return data
}

func userFromContext(ctx context.Context) *store.User {
	user, _ := ctx.Value(userContextKey).(*store.User)
	return user
}

func secureEqual(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func retrySeconds(d time.Duration) int {
	seconds := int(d.Round(time.Second).Seconds())
	if seconds < 1 {
		return 1
	}
	return seconds
}

func (a *App) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	remote := net.ParseIP(strings.TrimSpace(host))
	if remote == nil {
		return "unknown"
	}
	if !a.isTrustedProxy(remote) {
		return remote.String()
	}
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded == "" {
		return remote.String()
	}
	parts := strings.Split(forwarded, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := net.ParseIP(strings.TrimSpace(parts[i]))
		if candidate == nil {
			continue
		}
		if !a.isTrustedProxy(candidate) {
			return candidate.String()
		}
	}
	return remote.String()
}

func (a *App) isTrustedProxy(ip net.IP) bool {
	for _, network := range a.cfg.TrustedProxyNets {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func (a *App) tooMany(w http.ResponseWriter, r *http.Request, page string, data pageData, wait time.Duration) {
	seconds := retrySeconds(wait)
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	data.Error = "Demasiados intentos. Espera un momento antes de volver a intentarlo."
	data.RetryAfter = seconds
	data = a.csrfData(w, r, data)
	a.render(w, http.StatusTooManyRequests, page, data)
}
