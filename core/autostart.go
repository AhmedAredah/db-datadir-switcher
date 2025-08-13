package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// SetAutoStart enables or disables auto-start functionality
func SetAutoStart(enable bool) error {
	if enable {
		return enableAutoStart()
	}
	return disableAutoStart()
}

// IsAutoStartEnabled checks if auto-start is currently enabled
func IsAutoStartEnabled() bool {
	switch runtime.GOOS {
	case "windows":
		return isWindowsAutoStartEnabled()
	case "darwin":
		return isMacAutoStartEnabled()
	case "linux":
		return isLinuxAutoStartEnabled()
	default:
		return false
	}
}

// enableAutoStart enables auto-start for the current platform
func enableAutoStart() error {
	AppLogger.Info("Enabling auto-start for platform: %s", runtime.GOOS)
	
	switch runtime.GOOS {
	case "windows":
		return enableWindowsAutoStart()
	case "darwin":
		return enableMacAutoStart()
	case "linux":
		return enableLinuxAutoStart()
	default:
		return fmt.Errorf("auto-start not supported on platform: %s", runtime.GOOS)
	}
}

// disableAutoStart disables auto-start for the current platform
func disableAutoStart() error {
	AppLogger.Info("Disabling auto-start for platform: %s", runtime.GOOS)
	
	switch runtime.GOOS {
	case "windows":
		return disableWindowsAutoStart()
	case "darwin":
		return disableMacAutoStart()
	case "linux":
		return disableLinuxAutoStart()
	default:
		return fmt.Errorf("auto-start not supported on platform: %s", runtime.GOOS)
	}
}

// Windows Auto-Start Implementation
func enableWindowsAutoStart() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}
	
	// Add to Windows Registry Run key
	key := `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run`
	appName := "DBSwitcher"
	
	// Use --minimized flag for auto-start
	cmd := exec.Command("reg", "add", key, "/v", appName, "/t", "REG_SZ", "/d", fmt.Sprintf(`"%s" --minimized`, exePath), "/f")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add registry entry: %v\nOutput: %s", err, string(output))
	}
	
	AppLogger.Info("Windows auto-start enabled in registry")
	return nil
}

func disableWindowsAutoStart() error {
	key := `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run`
	appName := "DBSwitcher"
	
	cmd := exec.Command("reg", "delete", key, "/v", appName, "/f")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if it's just because the entry doesn't exist
		if strings.Contains(string(output), "unable to find") {
			AppLogger.Debug("Registry entry does not exist (already disabled)")
			return nil
		}
		return fmt.Errorf("failed to remove registry entry: %v\nOutput: %s", err, string(output))
	}
	
	AppLogger.Info("Windows auto-start disabled")
	return nil
}

func isWindowsAutoStartEnabled() bool {
	key := `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run`
	appName := "DBSwitcher"
	
	cmd := exec.Command("reg", "query", key, "/v", appName)
	err := cmd.Run()
	return err == nil
}

// macOS Auto-Start Implementation
func enableMacAutoStart() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}
	
	// Create LaunchAgent plist
	homeDir, _ := os.UserHomeDir()
	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	os.MkdirAll(launchAgentsDir, 0755)
	
	plistPath := filepath.Join(launchAgentsDir, "com.dbswitcher.app.plist")
	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.dbswitcher.app</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>--minimized</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<false/>
</dict>
</plist>`, exePath)
	
	if err := os.WriteFile(plistPath, []byte(plistContent), 0644); err != nil {
		return fmt.Errorf("failed to create plist file: %v", err)
	}
	
	// Load the launch agent
	cmd := exec.Command("launchctl", "load", plistPath)
	if err := cmd.Run(); err != nil {
		AppLogger.Warn("Failed to load launch agent: %v", err)
		// Continue anyway, file is created
	}
	
	AppLogger.Info("macOS auto-start enabled (LaunchAgent)")
	return nil
}

func disableMacAutoStart() error {
	homeDir, _ := os.UserHomeDir()
	plistPath := filepath.Join(homeDir, "Library", "LaunchAgents", "com.dbswitcher.app.plist")
	
	// Unload if running
	cmd := exec.Command("launchctl", "unload", plistPath)
	cmd.Run() // Ignore errors
	
	// Remove plist file
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove plist file: %v", err)
	}
	
	AppLogger.Info("macOS auto-start disabled")
	return nil
}

func isMacAutoStartEnabled() bool {
	homeDir, _ := os.UserHomeDir()
	plistPath := filepath.Join(homeDir, "Library", "LaunchAgents", "com.dbswitcher.app.plist")
	return PathExists(plistPath)
}

// Linux Auto-Start Implementation
func enableLinuxAutoStart() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}
	
	// Create desktop file for autostart
	homeDir, _ := os.UserHomeDir()
	autostartDir := filepath.Join(homeDir, ".config", "autostart")
	os.MkdirAll(autostartDir, 0755)
	
	desktopPath := filepath.Join(autostartDir, "dbswitcher.desktop")
	desktopContent := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=DBSwitcher
Comment=MariaDB Configuration Manager
Exec=%s --minimized
Icon=database
Terminal=false
NoDisplay=true
X-GNOME-Autostart-enabled=true
`, exePath)
	
	if err := os.WriteFile(desktopPath, []byte(desktopContent), 0644); err != nil {
		return fmt.Errorf("failed to create desktop file: %v", err)
	}
	
	AppLogger.Info("Linux auto-start enabled (desktop file)")
	return nil
}

func disableLinuxAutoStart() error {
	homeDir, _ := os.UserHomeDir()
	desktopPath := filepath.Join(homeDir, ".config", "autostart", "dbswitcher.desktop")
	
	if err := os.Remove(desktopPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove desktop file: %v", err)
	}
	
	AppLogger.Info("Linux auto-start disabled")
	return nil
}

func isLinuxAutoStartEnabled() bool {
	homeDir, _ := os.UserHomeDir()
	desktopPath := filepath.Join(homeDir, ".config", "autostart", "dbswitcher.desktop")
	return PathExists(desktopPath)
}

// UpdateAutoStartSetting updates the auto-start setting based on config
func UpdateAutoStartSetting() error {
	currentEnabled := IsAutoStartEnabled()
	shouldEnable := AppConfig.AutoStartWithSystem
	
	if currentEnabled != shouldEnable {
		AppLogger.Info("Auto-start setting changed from %t to %t", currentEnabled, shouldEnable)
		return SetAutoStart(shouldEnable)
	}
	
	AppLogger.Debug("Auto-start setting unchanged: %t", currentEnabled)
	return nil
}