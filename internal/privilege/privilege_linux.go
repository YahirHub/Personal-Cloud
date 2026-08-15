//go:build linux

package privilege

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

const capSysAdmin = 21

func Ensure() (bool, error) {
	if os.Geteuid() == 0 || hasCapability(capSysAdmin) {
		return false, nil
	}
	stdin, err := os.Stdin.Stat()
	if err != nil || stdin.Mode()&os.ModeCharDevice == 0 {
		return false, nil
	}
	sudo, err := exec.LookPath("sudo")
	if err != nil {
		return false, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return false, err
	}
	args := append([]string{"sudo", "-E", "--", exe}, os.Args[1:]...)
	if err := syscall.Exec(sudo, args, os.Environ()); err != nil {
		return false, fmt.Errorf("reintentar con sudo: %w", err)
	}
	return true, nil
}

func hasCapability(bit uint) bool {
	file, err := os.Open("/proc/self/status")
	if err != nil {
		return false
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "CapEff:") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "CapEff:"))
		n, err := strconv.ParseUint(value, 16, 64)
		return err == nil && n&(uint64(1)<<bit) != 0
	}
	return false
}
