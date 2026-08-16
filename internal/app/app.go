package app

import (
	"context"
	"crypto/rand"
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
	"personalcloud/internal/backup"
	"personalcloud/internal/catalog"
	"personalcloud/internal/config"
	"personalcloud/internal/ratelimit"
	storagepkg "personalcloud/internal/storage"
	"personalcloud/internal/store"
	"personalcloud/internal/streaming"
	"personalcloud/internal/vfs"
	webdavserver "personalcloud/internal/webdav"
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
	storageManager    *storagepkg.Manager
	catalog           *catalog.Catalog
	indexer           *catalog.Indexer
	vfs               *vfs.FS
	streamer          *streaming.Manager
	webdav            *webdavserver.Server
	davAuthMu         sync.Mutex
	batchMu           sync.Mutex
	davAuthSecret     [32]byte
	downloadSecret    [32]byte
	shareSecret       [32]byte
	davAuthCache      map[string]davAuthCacheEntry
	batchDownloads    map[string]batchDownload
	batchZIP          chan struct{}
	lastBackupDay     string
	stop              chan struct{}
	stopOnce          sync.Once
}

type pageData struct {
	Title                  string
	Description            string
	CurrentPath            string
	CSRFToken              string
	Error                  string
	Info                   string
	User                   *store.User
	RetryAfter             int
	StorageItems           []storagePageItem
	StorageError           string
	StorageSummary         storageSummary
	StorageSummaryLoaded   bool
	StorageUsageItems      []storageUsageItem
	StorageLargestFiles    []explorerItem
	Stats                  dashboardStats
	HomeFolders            []explorerRoot
	HomeFiles              []homeFileItem
	Media                  []mediaPageItem
	MediaOffset            int
	MediaNext              int
	MediaHasMore           bool
	MediaTotal             int
	GalleryType            string
	GallerySort            string
	GalleryFilters         int
	ListingMode            string
	ListingBaseURL         string
	ListingInfiniteURL     string
	ListingPagesURL        string
	ListingPrevURL         string
	ListingNextURL         string
	ListingPage            int
	ListingPrev            int
	ListingNext            int
	ListingHasPrev         bool
	ListingHasNext         bool
	ExplorerItems          []explorerItem
	ExplorerFolders        []explorerItem
	ExplorerFiles          []explorerItem
	ExplorerRoots          []explorerRoot
	Breadcrumbs            []breadcrumbItem
	ExplorerPath           string
	ExplorerRoot           string
	ExplorerRelative       string
	ExplorerCanWrite       bool
	ExplorerHasMore        bool
	ExplorerNext           int
	SearchQuery            string
	SearchMode             bool
	FileCollection         string
	FileCollectionTitle    string
	FileCollectionSubtitle string
	FileTypeFilter         string
	FileModifiedFilter     string
	FileSourceFilter       string
	FileFilterAction       string
	FileFilterCount        int
	MaxUploadBytes         int64
	MaxUploadBatchFiles    int
	Settings               store.AppSettings
	SettingsSyncText       string
	MoveDestinations       []moveDestination
	IntegrityUnits         []integrityUnitView
	Shares                 []sharePageItem
	PublicShare            *publicShareView
	PublicSharePage        bool
	SharePasswordRequired  bool
	SharePasswordError     string
	ShareEmbed             bool
}

type contextKey string

const userContextKey contextKey = "user"

var (
	setupPolicy          = ratelimit.Policy{MaxAttempts: 5, Window: 10 * time.Minute}
	loginIPPolicy        = ratelimit.Policy{MaxAttempts: 12, Window: 15 * time.Minute}
	loginUserPolicy      = ratelimit.Policy{MaxAttempts: 6, Window: 15 * time.Minute}
	downloadTicketPolicy = ratelimit.Policy{MaxAttempts: 120, Window: time.Minute}
	bulkActionPolicy     = ratelimit.Policy{MaxAttempts: 30, Window: time.Minute}
	videoTranscodePolicy = ratelimit.Policy{MaxAttempts: 20, Window: time.Hour}
	sharePasswordPolicy  = ratelimit.Policy{MaxAttempts: 8, Window: 15 * time.Minute}
	shareManagePolicy    = ratelimit.Policy{MaxAttempts: 30, Window: time.Minute}
	shareVideoPolicy     = ratelimit.Policy{MaxAttempts: 12, Window: time.Hour}
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

	catalogStore, err := catalog.Open(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("abrir catálogo: %w", err)
	}
	storageManager := storagepkg.NewManager(storage, logger, cfg.MountRoot)
	virtualFS := vfs.New(storageManager, storage)
	indexer := catalog.NewIndexer(catalogStore, storageManager, logger)
	streamer := streaming.New(cfg.DataDir, virtualFS, logger)
	dav := webdavserver.New(virtualFS, "/webdav", cfg.MaxUploadBytes)
	dav.OnMutation = func(ctx context.Context, virtualPath string) {
		storageID, err := virtualFS.StorageID(ctx, virtualPath)
		if err == nil {
			indexer.Enqueue(storageID)
		}
	}
	var davAuthSecret [32]byte
	if _, err := rand.Read(davAuthSecret[:]); err != nil {
		return nil, fmt.Errorf("preparar caché de autenticación WebDAV: %w", err)
	}
	var downloadSecret [32]byte
	if _, err := rand.Read(downloadSecret[:]); err != nil {
		return nil, fmt.Errorf("preparar descargas seguras: %w", err)
	}
	var shareSecret [32]byte
	if _, err := rand.Read(shareSecret[:]); err != nil {
		return nil, fmt.Errorf("preparar enlaces públicos: %w", err)
	}

	a := &App{
		cfg:               cfg,
		store:             storage,
		logger:            logger,
		renderer:          renderer,
		limiter:           ratelimit.New(),
		mux:               http.NewServeMux(),
		dummyPasswordHash: dummyHash,
		storageManager:    storageManager,
		catalog:           catalogStore,
		indexer:           indexer,
		vfs:               virtualFS,
		streamer:          streamer,
		webdav:            dav,
		davAuthSecret:     davAuthSecret,
		downloadSecret:    downloadSecret,
		shareSecret:       shareSecret,
		davAuthCache:      make(map[string]davAuthCacheEntry),
		batchDownloads:    make(map[string]batchDownload),
		batchZIP:          make(chan struct{}, 1),
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
	return a.securityHeaders(a.requestLog(a.enforceHTTPS(a.mux)))
}

func (a *App) Close() {
	a.stopOnce.Do(func() {
		close(a.stop)
		if a.indexer != nil {
			a.indexer.Close()
		}
		if a.streamer != nil {
			a.streamer.Close()
		}
		if a.storageManager != nil {
			a.storageManager.Close()
		}
		if a.catalog != nil {
			_ = a.catalog.Close()
		}
	})
}

func (a *App) routes() {
	staticFS, err := webui.StaticFS()
	if err != nil {
		panic(err)
	}
	a.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	a.mux.HandleFunc("GET /{$}", a.home)
	a.mux.HandleFunc("GET /setup", a.setupGet)
	a.mux.HandleFunc("POST /setup", a.setupPost)
	a.mux.HandleFunc("GET /iniciar-sesion", a.loginGet)
	a.mux.HandleFunc("POST /iniciar-sesion", a.loginPost)
	a.mux.Handle("POST /cerrar-sesion", a.requireAuth(http.HandlerFunc(a.logoutPost)))
	a.mux.Handle("GET /bienvenida", a.requireAuth(http.HandlerFunc(a.onboardingGet)))
	a.mux.Handle("POST /bienvenida/completar", a.requireAuth(http.HandlerFunc(a.onboardingCompletePost)))
	a.mux.Handle("GET /inicio", a.requireAuth(http.HandlerFunc(a.dashboardGet)))
	a.mux.Handle("GET /almacenamiento", a.requireAuth(http.HandlerFunc(a.storageGet)))
	a.mux.Handle("GET /archivos", a.requireAuth(http.HandlerFunc(a.filesGet)))
	a.mux.Handle("GET /archivos/ver/{path...}", a.requireAuth(http.HandlerFunc(a.filesGet)))
	a.mux.Handle("GET /recientes", a.requireAuth(http.HandlerFunc(a.recentFilesGet)))
	a.mux.Handle("GET /destacados", a.requireAuth(http.HandlerFunc(a.starredFilesGet)))
	a.mux.Handle("GET /compartidos", a.requireAuth(http.HandlerFunc(a.sharedFilesGet)))
	a.mux.Handle("GET /api/archivos/listado", a.requireAuth(http.HandlerFunc(a.filesListAPI)))
	a.mux.Handle("POST /archivos/subir", a.requireAuth(http.HandlerFunc(a.filesUploadPost)))
	a.mux.Handle("GET /galeria", a.requireAuth(http.HandlerFunc(a.galleryGet)))
	a.mux.Handle("GET /api/galeria", a.requireAuth(http.HandlerFunc(a.galleryAPI)))
	a.mux.Handle("GET /api/galeria/disponibilidad", a.requireAuth(http.HandlerFunc(a.galleryAvailabilityAPI)))
	a.mux.Handle("GET /api/indexacion", a.requireAuth(http.HandlerFunc(a.indexStatusAPI)))
	a.mux.Handle("GET /galeria/{id}/miniatura", a.requireAuth(http.HandlerFunc(a.photoThumbnailGet)))
	a.mux.Handle("GET /galeria/{id}/vista-previa", a.requireAuth(http.HandlerFunc(a.photoPreviewGet)))
	a.mux.Handle("GET /fotos", a.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/galeria", http.StatusMovedPermanently)
	})))
	a.mux.Handle("GET /archivo/{id}/original", a.requireAuth(http.HandlerFunc(a.originalFileGet)))
	a.mux.Handle("GET /archivo/{id}/pdf", a.requireAuth(http.HandlerFunc(a.filePDFPreviewGet)))
	a.mux.Handle("GET /archivo/{id}/html", a.requireAuth(http.HandlerFunc(a.fileHTMLPreviewGet)))
	a.mux.Handle("GET /api/archivo/{id}/contenido", a.requireAuth(http.HandlerFunc(a.fileTextContentGet)))
	a.mux.Handle("POST /api/archivo/{id}/contenido", a.requireAuth(http.HandlerFunc(a.fileTextContentPost)))
	a.mux.Handle("GET /api/archivo/{id}/info", a.requireAuth(http.HandlerFunc(a.fileInfoAPI)))
	a.mux.Handle("POST /api/archivo/{id}/destacar", a.requireAuth(http.HandlerFunc(a.fileStarPost)))
	a.mux.Handle("GET /api/archivo/{id}/compartir", a.requireAuth(http.HandlerFunc(a.fileShareInfoGet)))
	a.mux.Handle("POST /api/archivo/{id}/compartir", a.requireAuth(http.HandlerFunc(a.fileSharePost)))
	a.mux.Handle("GET /api/compartidos/{id}", a.requireAuth(http.HandlerFunc(a.shareInfoGet)))
	a.mux.Handle("POST /api/compartidos/{id}/configurar", a.requireAuth(http.HandlerFunc(a.shareConfigurePost)))
	a.mux.Handle("POST /api/compartidos/{id}/renovar", a.requireAuth(http.HandlerFunc(a.shareRenewPost)))
	a.mux.Handle("POST /api/compartidos/{id}/eliminar", a.requireAuth(http.HandlerFunc(a.shareDeletePost)))
	a.mux.Handle("POST /api/compartidos/eliminar-todos", a.requireAuth(http.HandlerFunc(a.sharesDeleteAllPost)))
	a.mux.Handle("POST /api/archivo/{id}/renombrar", a.requireAuth(http.HandlerFunc(a.fileRenamePost)))
	a.mux.Handle("GET /api/video/{id}/calidades", a.requireAuth(http.HandlerFunc(a.videoQualitiesGet)))
	a.mux.Handle("POST /api/video/{id}/preparar", a.requireAuth(http.HandlerFunc(a.videoVariantPreparePost)))
	a.mux.Handle("GET /api/video/{id}/estado", a.requireAuth(http.HandlerFunc(a.videoVariantStatusGet)))
	a.mux.Handle("GET /archivo/{id}/video/{quality}", a.requireAuth(http.HandlerFunc(a.videoVariantGet)))
	a.mux.Handle("POST /api/descargas", a.requireAuth(http.HandlerFunc(a.downloadTicketPost)))
	a.mux.Handle("GET /descarga/{token}", a.requireAuth(http.HandlerFunc(a.secureDownloadGet)))
	a.mux.Handle("POST /almacenamiento/registrar", a.requireAuth(http.HandlerFunc(a.storageRegisterPost)))
	a.mux.Handle("POST /almacenamiento/{id}/configuracion", a.requireAuth(http.HandlerFunc(a.storageUpdatePost)))
	a.mux.Handle("POST /almacenamiento/{id}/montar", a.requireAuth(http.HandlerFunc(a.storageMountPost)))
	a.mux.Handle("POST /almacenamiento/{id}/desmontar", a.requireAuth(http.HandlerFunc(a.storageUnmountPost)))
	a.mux.Handle("POST /almacenamiento/{id}/indexar", a.requireAuth(http.HandlerFunc(a.storageIndexPost)))
	a.mux.Handle("GET /api/almacenamiento/{id}", a.requireAuth(http.HandlerFunc(a.storageInfoAPI)))
	a.mux.Handle("POST /api/almacenamiento/{id}/indexar", a.requireAuth(http.HandlerFunc(a.storageIndexAPI)))
	a.mux.Handle("POST /api/almacenamiento/{id}/montar", a.requireAuth(http.HandlerFunc(a.storageMountAPI)))
	a.mux.Handle("POST /almacenamiento/{id}/danados/omitir", a.requireAuth(http.HandlerFunc(a.storageIgnoreDamagedPost)))
	a.mux.Handle("POST /almacenamiento/{id}/danados/eliminar", a.requireAuth(http.HandlerFunc(a.storageDeleteDamagedPost)))
	a.mux.Handle("GET /configuracion", a.requireAuth(http.HandlerFunc(a.settingsGet)))
	a.mux.Handle("POST /configuracion/sincronizacion", a.requireAuth(http.HandlerFunc(a.settingsSyncPost)))
	a.mux.Handle("POST /configuracion/sincronizar", a.requireAuth(http.HandlerFunc(a.settingsSyncNowPost)))
	a.mux.Handle("POST /configuracion/sincronizar/{id}", a.requireAuth(http.HandlerFunc(a.settingsSyncUnitPost)))
	a.mux.Handle("POST /configuracion/verificar-integridad", a.requireAuth(http.HandlerFunc(a.settingsVerifyNowPost)))
	a.mux.Handle("POST /configuracion/verificar-integridad/{id}", a.requireAuth(http.HandlerFunc(a.settingsVerifyUnitPost)))
	a.mux.Handle("GET /api/carpetas", a.requireAuth(http.HandlerFunc(a.moveFoldersGet)))
	a.mux.Handle("POST /api/carpetas/crear", a.requireAuth(http.HandlerFunc(a.moveFolderCreatePost)))
	a.mux.Handle("POST /api/elementos/mover", a.requireAuth(http.HandlerFunc(a.elementsMovePost)))
	a.mux.Handle("POST /api/elementos/destacar", a.requireAuth(http.HandlerFunc(a.elementsStarPost)))
	a.mux.Handle("POST /api/elementos/eliminar", a.requireAuth(http.HandlerFunc(a.elementsDeletePost)))

	// Enlaces públicos: el token aleatorio es la credencial; la contraseña opcional
	// se valida antes de servir cualquier byte del archivo.
	a.mux.HandleFunc("GET /s/{token}", a.publicShareGet)
	a.mux.HandleFunc("POST /s/{token}", a.publicSharePasswordPost)
	a.mux.HandleFunc("GET /s/{token}/embed", a.publicShareEmbedGet)
	a.mux.HandleFunc("POST /s/{token}/embed", a.publicSharePasswordPost)
	a.mux.HandleFunc("GET /s/{token}/contenido", a.publicShareContentGet)
	a.mux.HandleFunc("GET /s/{token}/video/calidades", a.publicShareVideoQualitiesGet)
	a.mux.HandleFunc("POST /s/{token}/video/preparar", a.publicShareVideoPreparePost)
	a.mux.HandleFunc("GET /s/{token}/video/estado", a.publicShareVideoStatusGet)
	a.mux.HandleFunc("GET /s/{token}/video/{quality}", a.publicShareVideoVariantGet)
	a.mux.Handle("POST /api/elementos/descargar", a.requireAuth(http.HandlerFunc(a.batchDownloadTicketPost)))
	a.mux.Handle("GET /descarga-lote/{token}", a.requireAuth(http.HandlerFunc(a.batchDownloadGet)))
	a.mux.HandleFunc("GET /salud", a.healthGet)
	a.mux.Handle("/webdav", a.webdavAuth(a.webdav))
	a.mux.Handle("/webdav/", a.webdavAuth(a.webdav))
}

func (a *App) housekeeping() {
	a.backupMetadataIfNeeded()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	maintenanceTicks := 0
	for {
		select {
		case <-a.stop:
			return
		case <-ticker.C:
			a.periodicSyncIfNeeded()
			a.cleanupBatchDownloads()
			maintenanceTicks++
			if maintenanceTicks < 30 {
				continue
			}
			maintenanceTicks = 0
			a.limiter.Cleanup()
			if a.streamer != nil {
				a.streamer.Cleanup()
			}
			a.backupMetadataIfNeeded()
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

func (a *App) backupMetadataIfNeeded() {
	now := time.Now().UTC()
	day := now.Format("20060102")
	if a.lastBackupDay == day {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	exists, err := a.store.AdminExists(ctx)
	if err != nil || !exists {
		return
	}
	snapshot, err := a.catalog.SnapshotBytes(ctx)
	if err != nil {
		a.logger.Warn("no se pudo preparar snapshot del catálogo", "error", err)
		return
	}
	path, err := backup.CreateMetadata(a.cfg.DataDir, a.cfg.StorePath(), snapshot, now)
	if err != nil {
		a.logger.Warn("no se pudo crear backup de metadatos", "error", err)
		return
	}
	a.lastBackupDay = day
	a.logger.Info("backup de metadatos creado", "path", path)
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
	if data.User != nil && !data.StorageSummaryLoaded {
		data.StorageSummary = a.storageSummaryForContext(r.Context())
		data.StorageSummaryLoaded = true
	}
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
