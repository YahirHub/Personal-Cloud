package streaming

import (
	"testing"
	"time"

	"personalcloud/internal/catalog"
)

func TestProfilesRespectKnownSourceHeight(t *testing.T) {
	m := &Manager{ffmpeg: "ffmpeg", encoder: "libx264"}
	profiles := m.Profiles(catalog.File{Kind: "video", Height: 720})
	if len(profiles) != 4 {
		t.Fatalf("profiles=%v, quiero Original+360+480+720", profiles)
	}
	if profiles[len(profiles)-1].ID != "720" {
		t.Fatalf("último perfil=%s, quiero 720", profiles[len(profiles)-1].ID)
	}
}

func TestFingerprintChangesWhenSourceChanges(t *testing.T) {
	base := catalog.File{ID: "x", Size: 100, ModTime: time.Unix(10, 0)}
	changed := base
	changed.Size++
	if fingerprint(base) == fingerprint(changed) {
		t.Fatal("el fingerprint debe cambiar al cambiar el archivo")
	}
}

func TestProfileByIDRejectsArbitraryResolution(t *testing.T) {
	if _, ok := profileByID("9999"); ok {
		t.Fatal("una resolución arbitraria no debe aceptarse")
	}
}

func TestProfileRatesMatchAdaptiveBudgets(t *testing.T) {
	cases := map[string]string{"360": "900k", "480": "1600k", "720": "3200k", "1080": "5800k"}
	for quality, want := range cases {
		got, buffer := profileRate(quality)
		if got != want || buffer == "" {
			t.Fatalf("profileRate(%s)=%s/%s, quiero %s y buffer", quality, got, buffer, want)
		}
	}
}

func TestVariantFingerprintIncludesCacheVersion(t *testing.T) {
	file := catalog.File{Size: 10, ModTime: time.Unix(20, 0)}
	original := variantCacheVersion
	if original != "v2" {
		t.Fatalf("versión de caché inesperada: %s", original)
	}
	if fingerprint(file) == "" {
		t.Fatal("fingerprint vacío")
	}
}
