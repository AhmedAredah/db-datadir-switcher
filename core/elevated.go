package core

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// RunElevated runs a command with elevated privileges
func RunElevated(name string, args ...string) error {
	switch runtime.GOOS {
	case "windows":
		// Use PowerShell to run as administrator
		psCmd := fmt.Sprintf("Start-Process '%s' -ArgumentList '%s' -Verb RunAs -Wait",
			name, strings.Join(args, "','"))
		cmd := exec.Command("powershell", "-Command", psCmd)
		return cmd.Run()
	default:
		// Unix systems use sudo
		allArgs := append([]string{name}, args...)
		cmd := exec.Command("sudo", allArgs...)
		return cmd.Run()
	}
}