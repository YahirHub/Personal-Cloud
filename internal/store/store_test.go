package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFirstAdminSessionAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	exists, err := storage.AdminExists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("no debía existir administrador")
	}

	user, err := storage.CreateFirstAdmin(ctx, "admin", "hash-de-prueba")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storage.CreateFirstAdmin(ctx, "otro", "hash"); !errors.Is(err, ErrAdminExists) {
		t.Fatalf("error inesperado creando segundo admin: %v", err)
	}

	if err := storage.CreateSession(ctx, user.ID, "digest", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := storage.UserBySessionTokenHash(ctx, "digest")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != user.ID {
		t.Fatalf("usuario inesperado: %s", got.ID)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err = reopened.UserBySessionTokenHash(ctx, "digest")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != user.ID {
		t.Fatalf("sesión no persistió: %s", got.ID)
	}
}

func TestExpiredSessionIsRejectedAndCleaned(t *testing.T) {
	storage, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	ctx := context.Background()

	user, err := storage.CreateFirstAdmin(ctx, "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.CreateSession(ctx, user.ID, "expired", time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.UserBySessionTokenHash(ctx, "expired"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("una sesión vencida no debía autenticar: %v", err)
	}
	if err := storage.DeleteExpiredSessions(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestOpenRestoresBackupIfMainStateIsMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := storage.CreateFirstAdmin(ctx, "admin", "hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.UserByUsername(ctx, "admin"); err != nil {
		t.Fatal(err)
	}

	// Una segunda escritura genera el .bak con una copia válida del estado.
	if err := storage.CompleteOnboarding(ctx, storage.state.Users[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatalf("no se creó backup: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	recovered, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if _, err := recovered.UserByUsername(ctx, "admin"); err != nil {
		t.Fatalf("no recuperó el estado desde backup: %v", err)
	}
}

func TestAuditCleanupKeepsNewestRows(t *testing.T) {
	dir := t.TempDir()
	storage, err := Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if err := storage.Audit(ctx, "", "login", "fallido", "127.0.0.1"); err != nil {
			t.Fatal(err)
		}
	}
	if err := storage.CleanupAudit(ctx, 0, 2); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := 0
	for _, b := range content {
		if b == '\n' {
			lines++
		}
	}
	if lines != 2 {
		t.Fatalf("esperaba 2 eventos, obtuvo %d", lines)
	}
}

func TestOpenMigratesVersion1WithoutLosingUser(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	legacy := `{"version":1,"users":[{"id":"u1","username":"admin","password_hash":"hash","role":"admin","onboarding_completed":true,"created_at":"2026-08-15T12:00:00Z"}],"sessions":[]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	user, err := storage.UserByUsername(context.Background(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	if !user.OnboardingCompleted {
		t.Fatal("la migración perdió el onboarding existente")
	}
	if storage.state.Version != stateVersion {
		t.Fatalf("versión migrada inesperada: %d", storage.state.Version)
	}
}

func TestOpenMigratesVersion2FromR11WithoutLosingStorage(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	legacy := `{"version":2,"users":[{"id":"u1","username":"admin","password_hash":"hash","role":"admin","onboarding_completed":true,"created_at":"2026-08-15T12:00:00Z"}],"sessions":[],"volumes":[{"id":"v1","persistent_id":"volume:test","name":"USB","virtual_root":"E","category":"mixed","idle_timeout_seconds":300,"auto_unmount":true,"registered_at":"2026-08-15T12:00:00Z","updated_at":"2026-08-15T12:00:00Z"}]}`
	if err := os.WriteFile(statePath, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	storage, err := Open(statePath)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	if storage.state.Version != stateVersion {
		t.Fatalf("versión migrada inesperada: %d", storage.state.Version)
	}
	volumes, err := storage.ListStorageVolumes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(volumes) != 1 || volumes[0].PersistentID != "volume:test" || volumes[0].VirtualRoot != "E" {
		t.Fatalf("la migración perdió la unidad registrada: %+v", volumes)
	}
	settings, err := storage.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if settings.SyncIntervalMinutes != 0 {
		t.Fatalf("la sincronización periódica debe migrar desactivada, obtuvo %d", settings.SyncIntervalMinutes)
	}
}

func TestVirtualRootAllowsOrdinaryXAndZero(t *testing.T) {
	for _, root := range []string{"Fotos2026", "Xbox", "Disco0"} {
		if !validVirtualRoot(root) {
			t.Fatalf("raíz válida rechazada: %q", root)
		}
	}
}

func TestStorageVolumePersistsStableIdentityMetadata(t *testing.T) {
	storage, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	ctx := context.Background()
	volume, err := storage.RegisterStorageVolume(ctx, RegisterVolumeInput{
		PersistentID:   "volume:\\\\?\\volume{abc}\\",
		HardwareID:     "fsserial:deadbeef",
		IdentityStable: true,
		Name:           "USB", Platform: "windows", VirtualRoot: "E", Category: "mixed", IdleTimeoutSeconds: 300, AutoUnmount: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := storage.StorageVolumeByID(ctx, volume.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PersistentID != volume.PersistentID || got.HardwareID != "fsserial:deadbeef" || !got.IdentityStable {
		t.Fatalf("identidad persistida inesperada: %+v", got)
	}
}

func TestSettingsPersistSyncInterval(t *testing.T) {
	storage, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	settings := AppSettings{SyncIntervalMinutes: 30, LastSyncAt: time.Now().UTC().Truncate(time.Second)}
	if err := storage.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	got, err := storage.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.SyncIntervalMinutes != 30 || !got.LastSyncAt.Equal(settings.LastSyncAt) {
		t.Fatalf("settings=%+v", got)
	}
	if err := storage.UpdateSettings(ctx, AppSettings{SyncIntervalMinutes: 1}); err == nil {
		t.Fatal("debe rechazar intervalos menores de 5 minutos")
	}
}
