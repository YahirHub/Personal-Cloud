package backup

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const keepMetadataBackups = 7

type Manifest struct {
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	Contents  []string  `json:"contents"`
}

// CreateMetadata crea un backup compacto del estado crítico y del snapshot del
// catálogo. No copia originales ni previews/thumbnails, porque estos últimos son
// regenerables a partir de las unidades registradas.
func CreateMetadata(dataDir, statePath string, catalogSnapshot []byte, now time.Time) (string, error) {
	backupDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return "", fmt.Errorf("crear directorio de backups: %w", err)
	}
	name := "metadata-" + now.UTC().Format("20060102") + ".zip"
	target := filepath.Join(backupDir, name)
	tmp, err := os.CreateTemp(backupDir, ".metadata-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return "", err
	}

	zw := zip.NewWriter(tmp)
	contents := make([]string, 0, 2)
	if data, readErr := os.ReadFile(statePath); readErr == nil {
		if err := writeZipFile(zw, "state.json", data); err != nil {
			_ = zw.Close()
			_ = tmp.Close()
			return "", err
		}
		contents = append(contents, "state.json")
	} else if !os.IsNotExist(readErr) {
		_ = zw.Close()
		_ = tmp.Close()
		return "", readErr
	}
	if len(catalogSnapshot) > 0 {
		if err := writeZipFile(zw, "catalog/snapshot.json", catalogSnapshot); err != nil {
			_ = zw.Close()
			_ = tmp.Close()
			return "", err
		}
		contents = append(contents, "catalog/snapshot.json")
	}
	manifestData, err := json.MarshalIndent(Manifest{Version: 1, CreatedAt: now.UTC(), Contents: contents}, "", "  ")
	if err != nil {
		_ = zw.Close()
		_ = tmp.Close()
		return "", err
	}
	if err := writeZipFile(zw, "manifest.json", append(manifestData, '\n')); err != nil {
		_ = zw.Close()
		_ = tmp.Close()
		return "", err
	}
	if err := zw.Close(); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	_ = os.Remove(target)
	if err := os.Rename(tmpName, target); err != nil {
		return "", err
	}
	if err := pruneMetadataBackups(backupDir, keepMetadataBackups); err != nil {
		return target, err
	}
	return target, nil
}

func writeZipFile(zw *zip.Writer, name string, data []byte) error {
	header := &zip.FileHeader{Name: filepath.ToSlash(name), Method: zip.Deflate}
	header.SetMode(0o600)
	header.SetModTime(time.Now().UTC())
	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

func pruneMetadataBackups(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "metadata-") && strings.HasSuffix(entry.Name(), ".zip") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	if len(names) <= keep {
		return nil
	}
	for _, name := range names[:len(names)-keep] {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
