//go:build windows

package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"personalcloud/internal/store"
)

const (
	driveRemovable      = 2
	driveFixed          = 3
	fileReadOnlyVolume  = 0x00080000
	fsctlLockVolume     = 0x00090018
	fsctlUnlockVolume   = 0x0009001c
	fsctlDismountVolume = 0x00090020
)

var (
	kernel32DLL                 = syscall.NewLazyDLL("kernel32.dll")
	procFindFirstVolumeW        = kernel32DLL.NewProc("FindFirstVolumeW")
	procFindNextVolumeW         = kernel32DLL.NewProc("FindNextVolumeW")
	procFindVolumeClose         = kernel32DLL.NewProc("FindVolumeClose")
	procGetVolumePathNamesW     = kernel32DLL.NewProc("GetVolumePathNamesForVolumeNameW")
	procGetVolumeInformationW   = kernel32DLL.NewProc("GetVolumeInformationW")
	procGetDriveTypeW           = kernel32DLL.NewProc("GetDriveTypeW")
	procGetDiskFreeSpaceExW     = kernel32DLL.NewProc("GetDiskFreeSpaceExW")
	procGetLogicalDrives        = kernel32DLL.NewProc("GetLogicalDrives")
	procFlushFileBuffers        = kernel32DLL.NewProc("FlushFileBuffers")
	procSetVolumeMountPointW    = kernel32DLL.NewProc("SetVolumeMountPointW")
	procDeleteVolumeMountPointW = kernel32DLL.NewProc("DeleteVolumeMountPointW")
)

func discoverPlatformVolumes(ctx context.Context, mountRoot string) ([]DiscoveredVolume, error) {
	buffer := make([]uint16, 1024)
	handle, _, callErr := procFindFirstVolumeW.Call(uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	if handle == ^uintptr(0) {
		return nil, fmt.Errorf("enumerar volúmenes: %w", callErr)
	}
	defer procFindVolumeClose.Call(handle)

	systemDrive := strings.ToUpper(strings.TrimSpace(os.Getenv("SystemDrive")))
	if systemDrive == "" {
		systemDrive = "C:"
	}
	var result []DiscoveredVolume
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		volumeName := syscall.UTF16ToString(buffer)
		if volumeName != "" {
			paths, _ := windowsVolumePaths(volumeName)
			mountPoint := firstWindowsMountPoint(paths)
			typeTarget := volumeName
			if mountPoint != "" {
				typeTarget = mountPoint
			}
			driveType := windowsDriveType(typeTarget)
			if driveType == driveFixed || driveType == driveRemovable {
				label, fsType, flags, serial := windowsVolumeInfo(volumeName)
				capacity, free := windowsSpace(volumeName)
				system := false
				for _, path := range paths {
					if strings.EqualFold(strings.TrimSuffix(path, `\`), systemDrive) {
						system = true
						break
					}
				}
				// Volúmenes fijos ocultos muy pequeños suelen ser EFI/recovery, no almacenamiento del usuario.
				if !system && driveType == driveFixed && mountPoint == "" && capacity > 0 && capacity < 512<<20 {
					goto nextVolume
				}
				if label == "" {
					if mountPoint != "" {
						label = strings.TrimSuffix(mountPoint, `\`)
					} else {
						label = "Unidad " + shortWindowsVolumeName(volumeName)
					}
				}
				result = append(result, DiscoveredVolume{
					PersistentID:   "volume:" + strings.ToLower(volumeName),
					HardwareID:     fmt.Sprintf("fsserial:%08x", serial),
					IdentityStable: true,
					Name:           label,
					Label:          label,
					Platform:       "windows",
					Device:         volumeName,
					VolumeName:     volumeName,
					MountPoint:     mountPoint,
					FSType:         fsType,
					Mounted:        mountPoint != "",
					ReadOnly:       flags&fileReadOnlyVolume != 0,
					System:         system,
					Removable:      driveType == driveRemovable,
					Capacity:       capacity,
					Free:           free,
				})
			}
		}

	nextVolume:
		for i := range buffer {
			buffer[i] = 0
		}
		r1, _, nextErr := procFindNextVolumeW.Call(handle, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
		if r1 == 0 {
			if errno, ok := nextErr.(syscall.Errno); ok && errno == syscall.ERROR_NO_MORE_FILES {
				break
			}
			break
		}
	}
	return result, nil
}

func discoverPlatformPresence(ctx context.Context) (map[string]struct{}, error) {
	buffer := make([]uint16, 1024)
	handle, _, callErr := procFindFirstVolumeW.Call(uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	if handle == ^uintptr(0) {
		return nil, fmt.Errorf("enumerar presencia de volúmenes: %w", callErr)
	}
	defer procFindVolumeClose.Call(handle)
	result := make(map[string]struct{})
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		volumeName := syscall.UTF16ToString(buffer)
		if volumeName != "" {
			result[strings.ToLower("volume:"+volumeName)] = struct{}{}
		}
		for i := range buffer {
			buffer[i] = 0
		}
		r1, _, nextErr := procFindNextVolumeW.Call(handle, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
		if r1 == 0 {
			if errno, ok := nextErr.(syscall.Errno); ok && errno == syscall.ERROR_NO_MORE_FILES {
				break
			}
			break
		}
	}
	return result, nil
}

func mountPlatformVolume(ctx context.Context, cfg store.StorageVolume, detected DiscoveredVolume, mountRoot string) (string, error) {
	if detected.Mounted && detected.MountPoint != "" {
		return detected.MountPoint, nil
	}
	volumeName := detected.VolumeName
	if volumeName == "" {
		volumeName = cfg.VolumeName
	}
	if volumeName == "" {
		return "", errors.New("no se conoce el GUID del volumen")
	}
	target := cfg.PreferredMountPoint
	if !isDriveRoot(target) {
		target = chooseWindowsDriveRoot()
	}
	if target == "" {
		return "", errors.New("no hay una letra de unidad libre para montar el volumen")
	}
	_ = ctx
	targetPtr, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return "", err
	}
	volumePtr, err := syscall.UTF16PtrFromString(volumeName)
	if err != nil {
		return "", err
	}
	r1, _, callErr := procSetVolumeMountPointW.Call(uintptr(unsafe.Pointer(targetPtr)), uintptr(unsafe.Pointer(volumePtr)))
	if r1 == 0 {
		return "", fmt.Errorf("asignar punto de montaje %s: %w", target, callErr)
	}
	return target, nil
}

func unmountPlatformVolume(ctx context.Context, cfg store.StorageVolume, mountPoint string) error {
	if !isDriveRoot(mountPoint) {
		return errors.New("Windows solo desmonta automáticamente puntos con letra de unidad")
	}
	devicePath := `\\.\` + strings.TrimSuffix(mountPoint, `\`)
	ptr, err := syscall.UTF16PtrFromString(devicePath)
	if err != nil {
		return err
	}
	handle, err := syscall.CreateFile(ptr, syscall.GENERIC_READ|syscall.GENERIC_WRITE, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		return fmt.Errorf("abrir volumen para desmontar: %w", err)
	}
	defer syscall.CloseHandle(handle)

	procFlushFileBuffers.Call(uintptr(handle))
	if err := windowsVolumeControl(handle, fsctlLockVolume); err != nil {
		return fmt.Errorf("bloquear volumen: %w", err)
	}
	locked := true
	defer func() {
		if locked {
			_ = windowsVolumeControl(handle, fsctlUnlockVolume)
		}
	}()
	if err := windowsVolumeControl(handle, fsctlDismountVolume); err != nil {
		return fmt.Errorf("desmontar volumen: %w", err)
	}

	_ = ctx
	mountPtr, err := syscall.UTF16PtrFromString(mountPoint)
	if err != nil {
		return err
	}
	r1, _, callErr := procDeleteVolumeMountPointW.Call(uintptr(unsafe.Pointer(mountPtr)))
	if r1 == 0 {
		return fmt.Errorf("retirar punto de montaje %s: %w", mountPoint, callErr)
	}
	if err := windowsVolumeControl(handle, fsctlUnlockVolume); err == nil {
		locked = false
	}
	return nil
}

func windowsVolumeControl(handle syscall.Handle, code uint32) error {
	var returned uint32
	return syscall.DeviceIoControl(handle, code, nil, 0, nil, 0, &returned, nil)
}

func windowsVolumePaths(volumeName string) ([]string, error) {
	ptr, err := syscall.UTF16PtrFromString(volumeName)
	if err != nil {
		return nil, err
	}
	size := uint32(256)
	for attempts := 0; attempts < 3; attempts++ {
		buffer := make([]uint16, size)
		var needed uint32
		r1, _, callErr := procGetVolumePathNamesW.Call(
			uintptr(unsafe.Pointer(ptr)),
			uintptr(unsafe.Pointer(&buffer[0])),
			uintptr(size),
			uintptr(unsafe.Pointer(&needed)),
		)
		if r1 != 0 {
			return splitWindowsMultiString(buffer), nil
		}
		if needed > size {
			size = needed + 1
			continue
		}
		return nil, callErr
	}
	return nil, errors.New("no se pudieron obtener puntos de montaje")
}

func windowsVolumeInfo(volumeName string) (string, string, uint32, uint32) {
	ptr, err := syscall.UTF16PtrFromString(volumeName)
	if err != nil {
		return "", "", 0, 0
	}
	label := make([]uint16, 261)
	fsName := make([]uint16, 64)
	var serial, maxComponent, flags uint32
	r1, _, _ := procGetVolumeInformationW.Call(
		uintptr(unsafe.Pointer(ptr)),
		uintptr(unsafe.Pointer(&label[0])), uintptr(len(label)),
		uintptr(unsafe.Pointer(&serial)),
		uintptr(unsafe.Pointer(&maxComponent)),
		uintptr(unsafe.Pointer(&flags)),
		uintptr(unsafe.Pointer(&fsName[0])), uintptr(len(fsName)),
	)
	if r1 == 0 {
		return "", "", 0, 0
	}
	return syscall.UTF16ToString(label), syscall.UTF16ToString(fsName), flags, serial
}

func windowsSpace(path string) (uint64, uint64) {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0
	}
	var available, total, free uint64
	r1, _, _ := procGetDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(ptr)),
		uintptr(unsafe.Pointer(&available)),
		uintptr(unsafe.Pointer(&total)),
		uintptr(unsafe.Pointer(&free)),
	)
	if r1 == 0 {
		return 0, 0
	}
	return total, free
}

func windowsDriveType(path string) uint32 {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0
	}
	r1, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(ptr)))
	return uint32(r1)
}

func splitWindowsMultiString(buffer []uint16) []string {
	var result []string
	start := 0
	for i, value := range buffer {
		if value != 0 {
			continue
		}
		if i == start {
			break
		}
		result = append(result, syscall.UTF16ToString(buffer[start:i]))
		start = i + 1
	}
	return result
}

func firstWindowsMountPoint(paths []string) string {
	for _, path := range paths {
		if isDriveRoot(path) {
			return path
		}
	}
	if len(paths) > 0 {
		return paths[0]
	}
	return ""
}

func isDriveRoot(path string) bool {
	clean := filepath.Clean(strings.TrimSpace(path))
	return len(clean) == 3 && clean[1] == ':' && (clean[2] == '\\' || clean[2] == '/')
}

func chooseWindowsDriveRoot() string {
	mask, _, _ := procGetLogicalDrives.Call()
	for letter := byte('Z'); letter >= 'D'; letter-- {
		bit := uintptr(1) << (letter - 'A')
		if mask&bit == 0 {
			return string([]byte{letter, ':', '\\'})
		}
	}
	return ""
}

func shortWindowsVolumeName(value string) string {
	value = strings.TrimSuffix(strings.TrimPrefix(value, `\\?\Volume{`), `}\`)
	if len(value) > 8 {
		return value[:8]
	}
	return value
}
