package core

import (
	"os"
	"path/filepath"
	"runtime"
)

// GetAppDataDir returns the application data directory (for app settings, not user configs)
func GetAppDataDir() string {
	var dir string

	switch runtime.GOOS {
	case "windows":
		dir = os.Getenv("LOCALAPPDATA") // Use LOCALAPPDATA for app data
		if dir == "" {
			dir = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
		}
	case "darwin":
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, "Library", "Application Support")
	default: // Linux and others
		dir = os.Getenv("XDG_DATA_HOME")
		if dir == "" {
			home, _ := os.UserHomeDir()
			dir = filepath.Join(home, ".local", "share")
		}
	}

	appDir := filepath.Join(dir, "DBSwitcher")
	os.MkdirAll(appDir, 0755)
	return appDir
}

// GetUserConfigDir returns the user config directory (for MariaDB configs)
func GetUserConfigDir() string {
	var dir string

	switch runtime.GOOS {
	case "windows":
		dir = os.Getenv("APPDATA") // Use APPDATA for user configs
		if dir == "" {
			dir = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
		dir = filepath.Join(dir, "DBSwitcher", "configs")
	case "darwin":
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config", "DBSwitcher")
	default: // Linux and others
		dir = os.Getenv("XDG_CONFIG_HOME")
		if dir == "" {
			home, _ := os.UserHomeDir()
			dir = filepath.Join(home, ".config")
		}
		dir = filepath.Join(dir, "DBSwitcher")
	}

	os.MkdirAll(dir, 0755)
	return dir
}

// GetCurrentWorkingDir returns the current working directory
func GetCurrentWorkingDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return dir
}

// GetCurrentUser returns the current user name
func GetCurrentUser() string {
	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("USERNAME")
	}
	return user
}