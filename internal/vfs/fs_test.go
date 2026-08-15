package vfs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanVirtualPathRejectsTraversal(t *testing.T) {
	for _, value := range []string{"/Fotos/../secret", `\\Fotos\\x`, "/Fotos/ok\x00bad"} {
		if got := cleanVirtualPath(value); got != "" {
			t.Fatalf("esperaba rechazo para %q, obtuvo %q", value, got)
		}
	}
	if got := cleanVirtualPath("/Fotos/2026/a.jpg"); got != "/Fotos/2026/a.jpg" {
		t.Fatalf("ruta inesperada: %q", got)
	}
}

func TestSafePhysicalPathRejectsEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlinks no disponibles: %v", err)
	}
	if _, err := safePhysicalPath(root, filepath.Join("escape", "file.txt"), true); err == nil {
		t.Fatal("esperaba rechazo de symlink fuera de la raíz")
	}
}

func TestSafePhysicalPathRejectsInternalSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "alias")); err != nil {
		t.Skipf("symlinks no disponibles: %v", err)
	}
	if _, err := safePhysicalPath(root, filepath.Join("alias", "file.txt"), true); err == nil {
		t.Fatal("el VFS no debe seguir symlinks aunque apunten dentro de la raíz")
	}
}
