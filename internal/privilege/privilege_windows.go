//go:build windows

package privilege

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

var (
	shell32           = syscall.NewLazyDLL("shell32.dll")
	procIsUserAnAdmin = shell32.NewProc("IsUserAnAdmin")
	procShellExecuteW = shell32.NewProc("ShellExecuteW")
)

// Ensure intenta relanzar el servidor elevado. Devuelve true cuando el nuevo
// proceso fue lanzado y el proceso actual debe terminar.
func Ensure() (bool, error) {
	elevated, _, _ := procIsUserAnAdmin.Call()
	if elevated != 0 {
		return false, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return false, err
	}
	cwd, _ := os.Getwd()
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	params, _ := syscall.UTF16PtrFromString(joinWindowsArgs(os.Args[1:]))
	dir, _ := syscall.UTF16PtrFromString(cwd)
	result, _, callErr := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(params)),
		uintptr(unsafe.Pointer(dir)),
		1,
	)
	if result <= 32 {
		if callErr != nil && !errors.Is(callErr, syscall.Errno(0)) {
			return false, callErr
		}
		return false, syscall.Errno(result)
	}
	return true, nil
}

func joinWindowsArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = syscall.EscapeArg(arg)
	}
	return strings.Join(quoted, " ")
}
