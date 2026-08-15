//go:build linux

package storage

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"personalcloud/internal/store"
)

type linuxMount struct {
	majorMinor string
	mountPoint string
	fsType     string
	source     string
	readOnly   bool
}

func discoverPlatformVolumes(ctx context.Context, mountRoot string) ([]DiscoveredVolume, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	mounts, err := readLinuxMounts("/proc/self/mountinfo")
	if err != nil {
		return nil, err
	}
	rootBase := ""
	for _, mount := range mounts {
		if mount.mountPoint == "/" {
			rootBase = linuxBaseBlock(mount.source)
			break
		}
	}

	labels := linuxLabelMap()
	hardware := linuxHardwareIDMap()
	identities := linuxPersistentIdentities()
	result := make([]DiscoveredVolume, 0, len(identities))
	for _, identity := range identities {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		device, err := filepath.EvalSymlinks(identity.link)
		if err != nil {
			continue
		}
		base := linuxBaseBlock(device)
		system := base != "" && base == rootBase
		mount, mounted := findLinuxMount(mounts, device)
		if mounted && isCriticalLinuxMount(mount.mountPoint) {
			system = true
		}
		label := labels[device]
		if label == "" && mounted {
			label = filepath.Base(mount.mountPoint)
		}
		if label == "" {
			label = "Unidad " + shortID(identity.value)
		}
		removable := linuxRemovable(base)
		var capacity, free uint64
		readOnly, mountPoint, fsType := false, "", ""
		if mounted {
			mountPoint, fsType, readOnly = mount.mountPoint, mount.fsType, mount.readOnly
			capacity, free = linuxSpace(mount.mountPoint)
		}
		result = append(result, DiscoveredVolume{
			PersistentID: identity.kind + ":" + identity.value,
			HardwareID:   hardware[device], IdentityStable: true,
			Name: label, Label: label, Platform: "linux", Device: identity.link,
			MountPoint: mountPoint, FSType: fsType, Mounted: mounted, ReadOnly: readOnly,
			System: system, Removable: removable, Capacity: capacity, Free: free,
		})
	}
	return result, nil
}

type linuxIdentity struct{ kind, value, link string }

func linuxPersistentIdentities() []linuxIdentity {
	var result []linuxIdentity
	seen := map[string]bool{}
	for _, spec := range []struct{ dir, kind string }{{"/dev/disk/by-uuid", "uuid"}, {"/dev/disk/by-partuuid", "partuuid"}} {
		entries, err := os.ReadDir(spec.dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			link := filepath.Join(spec.dir, entry.Name())
			real, err := filepath.EvalSymlinks(link)
			if err != nil {
				continue
			}
			if seen[real] {
				continue
			}
			seen[real] = true
			result = append(result, linuxIdentity{kind: spec.kind, value: entry.Name(), link: link})
		}
	}
	return result
}

func linuxHardwareIDMap() map[string]string {
	result := map[string]string{}
	entries, err := os.ReadDir("/dev/disk/by-id")
	if err != nil {
		return result
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "wwn-") || strings.Contains(name, "-part") {
			continue
		}
		real, err := filepath.EvalSymlinks(filepath.Join("/dev/disk/by-id", name))
		if err != nil {
			continue
		}
		result[real] = "by-id:" + name
	}
	// También relaciona las particiones con el identificador del disco padre cuando existe.
	for _, entry := range entries {
		name := entry.Name()
		idx := strings.LastIndex(name, "-part")
		if idx <= 0 {
			continue
		}
		baseName := name[:idx]
		if !strings.HasPrefix(baseName, "usb-") && !strings.HasPrefix(baseName, "ata-") && !strings.HasPrefix(baseName, "nvme-") {
			continue
		}
		real, err := filepath.EvalSymlinks(filepath.Join("/dev/disk/by-id", name))
		if err == nil {
			result[real] = "by-id:" + baseName
		}
	}
	return result
}

func isCriticalLinuxMount(mountPoint string) bool {
	switch filepath.Clean(mountPoint) {
	case "/", "/boot", "/boot/efi", "/usr", "/var":
		return true
	default:
		return false
	}
}

func mountPlatformVolume(ctx context.Context, cfg store.StorageVolume, detected DiscoveredVolume, mountRoot string) (string, error) {
	if detected.Mounted && detected.MountPoint != "" {
		return detected.MountPoint, nil
	}
	device := detected.Device
	if device == "" {
		device = cfg.Device
	}
	if device == "" {
		return "", errors.New("no se conoce el dispositivo de la unidad")
	}
	if _, err := os.Stat(device); err != nil {
		return "", ErrOffline
	}
	target := cfg.PreferredMountPoint
	if target == "" || target == "/" {
		target = defaultMountPoint(mountRoot, detected)
	}
	if target == "" {
		return "", errors.New("no se pudo determinar el punto de montaje")
	}
	if err := os.MkdirAll(target, 0o700); err != nil {
		return "", fmt.Errorf("crear punto de montaje: %w", err)
	}

	flags := uintptr(syscall.MS_NOSUID | syscall.MS_NODEV)
	if cfg.ReadOnly {
		flags |= syscall.MS_RDONLY
	}
	fsType := detected.FSType
	if fsType == "" {
		fsType = cfg.FSType
	}
	if fsType != "" && fsType != "fuseblk" {
		if err := syscall.Mount(device, target, fsType, flags, ""); err == nil {
			return target, nil
		}
	}

	// Algunos formatos (por ejemplo NTFS mediante FUSE) requieren el helper de
	// montaje del sistema. La aplicación sigue controlando cuándo se monta y
	// desmonta, pero delega el driver del filesystem al sistema operativo.
	args := []string{device, target, "-o", "nosuid,nodev"}
	if cfg.ReadOnly {
		args[len(args)-1] = "ro,nosuid,nodev"
	}
	cmd := exec.CommandContext(ctx, "mount", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("mount: %v: %s", err, strings.TrimSpace(string(output)))
	}
	return target, nil
}

func unmountPlatformVolume(ctx context.Context, cfg store.StorageVolume, mountPoint string) error {
	if mountPoint == "" || mountPoint == "/" {
		return errors.New("punto de montaje inválido")
	}
	if err := syscall.Unmount(mountPoint, 0); err == nil {
		return nil
	}
	cmd := exec.CommandContext(ctx, "umount", mountPoint)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("umount: %v: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func readLinuxMounts(path string) ([]linuxMount, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("leer mountinfo: %w", err)
	}
	defer file.Close()

	var mounts []linuxMount
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, " - ")
		if len(parts) != 2 {
			continue
		}
		left := strings.Fields(parts[0])
		right := strings.Fields(parts[1])
		if len(left) < 6 || len(right) < 2 {
			continue
		}
		mounts = append(mounts, linuxMount{
			majorMinor: left[2],
			mountPoint: unescapeMountInfo(left[4]),
			fsType:     right[0],
			source:     unescapeMountInfo(right[1]),
			readOnly:   hasMountOption(left[5], "ro"),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("procesar mountinfo: %w", err)
	}
	return mounts, nil
}

func findLinuxMount(mounts []linuxMount, device string) (linuxMount, bool) {
	realDevice, _ := filepath.EvalSymlinks(device)
	for _, mount := range mounts {
		realSource, err := filepath.EvalSymlinks(mount.source)
		if err != nil {
			realSource = mount.source
		}
		if realSource == realDevice || mount.source == device {
			return mount, true
		}
	}
	return linuxMount{}, false
}

func linuxLabelMap() map[string]string {
	result := map[string]string{}
	entries, err := os.ReadDir("/dev/disk/by-label")
	if err != nil {
		return result
	}
	for _, entry := range entries {
		path := filepath.Join("/dev/disk/by-label", entry.Name())
		real, err := filepath.EvalSymlinks(path)
		if err == nil {
			result[real] = entry.Name()
		}
	}
	return result
}

func linuxBaseBlock(device string) string {
	name := filepath.Base(device)
	if name == "" || name == "." {
		return ""
	}
	sysPath := filepath.Join("/sys/class/block", name)
	real, err := filepath.EvalSymlinks(sysPath)
	if err != nil {
		return name
	}
	if _, err := os.Stat(filepath.Join(sysPath, "partition")); err == nil {
		parent := filepath.Base(filepath.Dir(real))
		if parent != "block" && parent != "" {
			return parent
		}
	}
	return name
}

func linuxRemovable(base string) bool {
	if base == "" {
		return false
	}
	data, err := os.ReadFile(filepath.Join("/sys/class/block", base, "removable"))
	if err == nil && strings.TrimSpace(string(data)) == "1" {
		return true
	}
	real, err := filepath.EvalSymlinks(filepath.Join("/sys/class/block", base))
	if err == nil && strings.Contains(strings.ToLower(filepath.ToSlash(real)), "/usb") {
		return true
	}
	return false
}

func linuxSpace(path string) (uint64, uint64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0
	}
	blockSize := uint64(stat.Bsize)
	return stat.Blocks * blockSize, stat.Bavail * blockSize
}

func hasMountOption(options, wanted string) bool {
	for _, option := range strings.Split(options, ",") {
		if option == wanted {
			return true
		}
	}
	return false
}

func unescapeMountInfo(value string) string {
	replacer := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return replacer.Replace(value)
}

func shortID(value string) string {
	if len(value) <= 8 {
		return value
	}
	return value[:8]
}

func parseUint(value string) uint64 {
	n, _ := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	return n
}
