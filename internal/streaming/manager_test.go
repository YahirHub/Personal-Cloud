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
