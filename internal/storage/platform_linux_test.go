//go:build linux

package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadLinuxMounts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mountinfo")
	content := "36 25 8:17 / /media/Mis\\040Fotos rw,nosuid,nodev - ext4 /dev/sdb1 rw\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	mounts, err := readLinuxMounts(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 1 || mounts[0].mountPoint != "/media/Mis Fotos" || mounts[0].source != "/dev/sdb1" {
		t.Fatalf("mount inesperado: %#v", mounts)
	}
}

func TestCriticalLinuxMount(t *testing.T) {
	for _, mountPoint := range []string{"/", "/boot", "/boot/efi", "/usr", "/var"} {
		if !isCriticalLinuxMount(mountPoint) {
			t.Fatalf("%s debe considerarse crítico", mountPoint)
		}
	}
	if isCriticalLinuxMount("/mnt/media") {
		t.Fatal("un mount de datos no debe considerarse crítico")
	}
}
