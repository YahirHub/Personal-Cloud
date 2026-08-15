package storage

import (
	"testing"

	"personalcloud/internal/store"
)

func TestSanitizeVirtualRoot(t *testing.T) {
	valid := map[string]string{
		"Fotos":        "Fotos",
		" Documentos ": "Documentos",
		"Fotos2026":    "Fotos2026",
		"Xbox":         "Xbox",
		"/Media/":      "",
	}
	for input, want := range valid {
		if got := sanitizeVirtualRoot(input); got != want {
			t.Fatalf("sanitizeVirtualRoot(%q)=%q, want %q", input, got, want)
		}
	}
	for _, input := range []string{"..", "a/b", `a\\b`} {
		if got := sanitizeVirtualRoot(input); got != "" {
			t.Fatalf("esperaba rechazo para %q, obtuvo %q", input, got)
		}
	}
}

func TestMatchRegisteredVolumeFallsBackToUniqueHardwareIdentity(t *testing.T) {
	cfg := store.StorageVolume{PersistentID: "volume:old", HardwareID: "fsserial:abcd", Label: "ALUMNOS", FSType: "exfat"}
	volumes := []DiscoveredVolume{{PersistentID: "volume:new", HardwareID: "FSSERIAL:ABCD", Label: "ALUMNOS", FSType: "exfat"}}
	got, ok, rebound := matchRegisteredVolume(cfg, volumes)
	if !ok || !rebound || got.PersistentID != "volume:new" {
		t.Fatalf("match=%+v ok=%v rebound=%v", got, ok, rebound)
	}
}

func TestMatchRegisteredVolumeRefusesAmbiguousHardwareIdentity(t *testing.T) {
	cfg := store.StorageVolume{PersistentID: "uuid:old", HardwareID: "by-id:usb-disk"}
	volumes := []DiscoveredVolume{
		{PersistentID: "uuid:a", HardwareID: "by-id:usb-disk"},
		{PersistentID: "uuid:b", HardwareID: "by-id:usb-disk"},
	}
	if _, ok, _ := matchRegisteredVolume(cfg, volumes); ok {
		t.Fatal("una identidad de hardware compartida sin segundo factor no debe reconectarse automáticamente")
	}
}

func TestMatchRegisteredVolumeUsesLabelAndFilesystemToDisambiguate(t *testing.T) {
	cfg := store.StorageVolume{PersistentID: "uuid:old", HardwareID: "by-id:usb-disk", Label: "CLASE-A", FSType: "exfat"}
	volumes := []DiscoveredVolume{
		{PersistentID: "uuid:a", HardwareID: "by-id:usb-disk", Label: "CLASE-A", FSType: "exfat"},
		{PersistentID: "uuid:b", HardwareID: "by-id:usb-disk", Label: "CLASE-B", FSType: "exfat"},
	}
	got, ok, rebound := matchRegisteredVolume(cfg, volumes)
	if !ok || !rebound || got.PersistentID != "uuid:a" {
		t.Fatalf("match=%+v ok=%v rebound=%v", got, ok, rebound)
	}
}

func TestMatchRegisteredVolumeRejectsContradictorySecondaryIdentity(t *testing.T) {
	cfg := store.StorageVolume{PersistentID: "uuid:old", HardwareID: "by-id:usb-disk", Label: "CLASE-A", FSType: "exfat"}
	volumes := []DiscoveredVolume{{PersistentID: "uuid:new", HardwareID: "by-id:usb-disk", Label: "CLASE-B", FSType: "exfat"}}
	if got, ok, rebound := matchRegisteredVolume(cfg, volumes); ok || rebound {
		t.Fatalf("una unidad con identidad secundaria contradictoria no debe reasociarse: match=%+v ok=%v rebound=%v", got, ok, rebound)
	}
}
