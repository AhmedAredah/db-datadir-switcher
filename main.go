package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/cmd/fyne_settings/settings"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/getlantern/systray"
	"github.com/zalando/go-keyring"
)

// Configuration structure
type Config struct {
	MariaDBBin		  string				`json:"mariadb_bin"`
	ConfigPath		  string				`json:"config_path"` // User-editable config directory
	LastUsedConfig	 string				`json:"last_used_config"`
	ProcessNames		map[string]string `json:"process_names"`
	ServiceNames		map[string]string `json:"service_names"`
	AutoDetected		bool				  `json:"auto_detected"`
	UseServiceControl bool				  `json:"use_service_control"`
	RequireElevation  bool				  `json:"require_elevation"`
}

// MariaDBConfig represents a detected configuration file
type MariaDBConfig struct {
	Name		  string `json:"name"`		  // Friendly name (e.g., "internal", "external", "development")
	Path		  string `json:"path"`		  // Full path to config file
	DataDir	  string `json:"data_dir"`	 // Data directory from config
	Port		  string `json:"port"`		  // Port from config
	Description string `json:"description"` // User description
	IsActive	 bool	`json:"is_active"`	// Currently running with this config
	Exists		bool	`json:"exists"`		// File exists
}

// MariaDBStatus represents the current state
type MariaDBStatus struct {
	IsRunning	bool	`json:"is_running"`
	ConfigFile  string `json:"config_file"` // Current config file path
	ConfigName  string `json:"config_name"` // Friendly name of config
	DataPath	 string `json:"data_path"`
	ProcessID	int	 `json:"process_id"`
	Port		  string `json:"port"`
	ServiceName string `json:"service_name,omitempty"`
	Version	  string `json:"version,omitempty"`
}

type MySQLCredentials struct {
	Username string
	Password string
	Host     string
	Port     string
}

var (
	config			  Config
	currentStatus	 MariaDBStatus
	availableConfigs []MariaDBConfig
	fyneApp			 fyne.App
	mainWindow		 fyne.Window
	logger			  *Logger
	statusCardRef	 *widget.Card
	savedCredentials *MySQLCredentials
	systrayRunning   bool
)

// Logger for debugging
type Logger struct {
	file *os.File
}

func NewLogger() *Logger {
	logPath := filepath.Join(getAppDataDir(), "dbswitcher.log")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return &Logger{}
	}
	return &Logger{file: file}
}

func (l *Logger) Log(format string, args ...interface{}) {
	if l.file != nil {
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		fmt.Fprintf(l.file, "[%s] %s\n", timestamp, fmt.Sprintf(format, args...))
	}
}

func (l *Logger) Close() {
	if l.file != nil {
		l.file.Close()
	}
}

// Show Fyne appearance settings
func showAppearanceSettings() {
	if fyneApp == nil {
		fyneApp = app.NewWithID("mariadb-switcher")
	}
	
	// Create appearance settings window
	settingsWindow := fyneApp.NewWindow("Appearance Settings")
	settingsWindow.Resize(fyne.NewSize(400, 300))
	
	// Create settings content using Fyne's built-in settings
	settingsContent := settings.NewSettings().LoadAppearanceScreen(settingsWindow)
	settingsWindow.SetContent(settingsContent)
	settingsWindow.Show()
}

// Keyring service constants
const (
	keyringService = "DBSwitcher"
	keyringAccount = "mysql_credentials"
)

// Save credentials to system keyring
func saveCredentialsToKeyring(creds MySQLCredentials) error {
	// Serialize credentials to JSON
	data, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %v", err)
	}
	
	// Store in system keyring
	err = keyring.Set(keyringService, keyringAccount, string(data))
	if err != nil {
		return fmt.Errorf("failed to save to keyring: %v", err)
	}
	
	logger.Log("Credentials saved to system keyring")
	return nil
}

// Load credentials from system keyring
func loadCredentialsFromKeyring() (*MySQLCredentials, error) {
	// Retrieve from system keyring
	data, err := keyring.Get(keyringService, keyringAccount)
	if err != nil {
		if err == keyring.ErrNotFound {
			return nil, nil // No saved credentials
		}
		return nil, fmt.Errorf("failed to load from keyring: %v", err)
	}
	
	// Deserialize credentials
	var creds MySQLCredentials
	err = json.Unmarshal([]byte(data), &creds)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal credentials: %v", err)
	}
	
	logger.Log("Credentials loaded from system keyring")
	return &creds, nil
}

// Delete credentials from system keyring
func deleteCredentialsFromKeyring() error {
	err := keyring.Delete(keyringService, keyringAccount)
	if err != nil && err != keyring.ErrNotFound {
		return fmt.Errorf("failed to delete from keyring: %v", err)
	}
	
	logger.Log("Credentials deleted from system keyring")
	return nil
}

// Create a dialog to get MySQL credentials from user
func showCredentialsDialog(parent fyne.Window, onSuccess func(creds MySQLCredentials), onCancel func()) {
	// Create form fields
	usernameEntry := widget.NewEntry()
	usernameEntry.SetPlaceHolder("root")
	if savedCredentials != nil && savedCredentials.Username != "" {
		usernameEntry.SetText(savedCredentials.Username)
	} else {
		usernameEntry.SetText("root") // Default suggestion
	}
	
	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("Enter password (leave empty if none)")
	if savedCredentials != nil {
		passwordEntry.SetText(savedCredentials.Password)
	}
	
	hostEntry := widget.NewEntry()
	hostEntry.SetPlaceHolder("localhost")
	if savedCredentials != nil && savedCredentials.Host != "" {
		hostEntry.SetText(savedCredentials.Host)
	} else {
		hostEntry.SetText("localhost")
	}
	
	portEntry := widget.NewEntry()
	portEntry.SetPlaceHolder("3306")
	if savedCredentials != nil && savedCredentials.Port != "" {
		portEntry.SetText(savedCredentials.Port)
	} else {
		portEntry.SetText("3306")
	}
	
	// Remember credentials checkbox options
	rememberSessionCheck := widget.NewCheck("Remember for this session", nil)
	rememberSessionCheck.SetChecked(true)
	
	rememberPermanentCheck := widget.NewCheck("Save credentials permanently (secure storage)", nil)
	rememberPermanentCheck.SetChecked(savedCredentials != nil)
	
	// Create form
	items := []*widget.FormItem{
		widget.NewFormItem("Username", usernameEntry),
		widget.NewFormItem("Password", passwordEntry),
		widget.NewFormItem("Host", hostEntry),
		widget.NewFormItem("Port", portEntry),
		widget.NewFormItem("", rememberSessionCheck),
		widget.NewFormItem("", rememberPermanentCheck),
	}
	
	// Create dialog with custom buttons
	d := dialog.NewForm("MySQL Admin Credentials", "Connect", "Cancel", items, 
		func(confirmed bool) {
			if confirmed {
				creds := MySQLCredentials{
					Username: usernameEntry.Text,
					Password: passwordEntry.Text,
					Host:     hostEntry.Text,
					Port:     portEntry.Text,
				}
				
				// Set defaults if empty
				if creds.Username == "" {
					creds.Username = "root"
				}
				if creds.Host == "" {
					creds.Host = "localhost"
				}
				if creds.Port == "" {
					creds.Port = "3306"
				}
				
				// Save credentials for session if requested
				if rememberSessionCheck.Checked {
					savedCredentials = &creds
				}
				
				// Save credentials permanently if requested
				if rememberPermanentCheck.Checked {
					if err := saveCredentialsToKeyring(creds); err != nil {
						logger.Log("Failed to save credentials to keyring: %v", err)
						dialog.ShowError(fmt.Errorf("Failed to save credentials: %v", err), parent)
					} else {
						savedCredentials = &creds
					}
				} else if !rememberPermanentCheck.Checked && savedCredentials != nil {
					// If unchecked, remove saved credentials
					if err := deleteCredentialsFromKeyring(); err != nil {
						logger.Log("Failed to delete credentials from keyring: %v", err)
					}
				}
				
				onSuccess(creds)
			} else {
				onCancel()
			}
		}, parent)
	
	d.Resize(fyne.NewSize(400, 300))
	d.Show()
}

// Test MySQL connection with provided credentials
func testMySQLConnection(creds MySQLCredentials) error {
	mysqlPath := filepath.Join(config.MariaDBBin, "mysql")
	if runtime.GOOS == "windows" {
		mysqlPath += ".exe"
	}
	
	// Build command with credentials
	args := []string{
		"-h", creds.Host,
		"-P", creds.Port,
		"-u", creds.Username,
	}
	
	// Add password if provided
	if creds.Password != "" {
		args = append(args, fmt.Sprintf("-p%s", creds.Password))
	}
	
	// Add test query
	args = append(args, "-e", "SELECT 1")
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	cmd := exec.CommandContext(ctx, mysqlPath, args...)
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		return fmt.Errorf("connection failed: %v\nOutput: %s", err, string(output))
	}
	
	return nil
}

// Get application data directory (for app settings, not user configs)
func getAppDataDir() string {
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

// Get user config directory (for MariaDB configs)
func getUserConfigDir() string {
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

// Platform-specific implementations
func init() {
	logger = NewLogger()
	loadConfig()
	scanForConfigs()
	
	// Load saved credentials from keyring
	if creds, err := loadCredentialsFromKeyring(); err != nil {
		logger.Log("Failed to load saved credentials: %v", err)
	} else if creds != nil {
		savedCredentials = creds
		logger.Log("Loaded saved credentials for user: %s", creds.Username)
	}
}

func loadConfig() {
	// Default configuration
	config = Config{
		ProcessNames: map[string]string{
			"windows": "mysqld.exe",
			"linux":	"mysqld",
			"darwin":  "mysqld",
			"freebsd": "mysqld",
		},
		ServiceNames: map[string]string{
			"windows": "MariaDB",
			"linux":	"mariadb",
			"darwin":  "mariadb",
			"freebsd": "mysql-server",
		},
		UseServiceControl: false,
		RequireElevation:  false,
	}

	// Set user config directory
	config.ConfigPath = getUserConfigDir()

	// Try to load existing config
	configFile := getConfigPath()
	if data, err := os.ReadFile(configFile); err == nil {
		if err := json.Unmarshal(data, &config); err != nil {
			logger.Log("Error parsing config: %v", err)
		}
	} else {
		// Auto-detect and create config
		autoDetectConfig()
		config.AutoDetected = true
		saveConfig()
	}

	// Ensure config directory exists and is set correctly
	if config.ConfigPath == "" || !pathExists(config.ConfigPath) {
		config.ConfigPath = getUserConfigDir()
		saveConfig()
	}
}

func getConfigPath() string {
	return filepath.Join(getAppDataDir(), "settings.json")
}

func saveConfig() error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(getConfigPath(), data, 0644)
}

func autoDetectConfig() {
	logger.Log("Auto-detecting configuration for %s", runtime.GOOS)

	// Detect MariaDB installation
	config.MariaDBBin = detectMariaDBBin()

	// Check if we need elevation
	config.RequireElevation = checkElevationRequired()
	config.UseServiceControl = checkServiceControlAvailable()

	logger.Log("Auto-detection complete: bin=%s", config.MariaDBBin)
}

// Scan for all available MariaDB configuration files
func scanForConfigs() {
	availableConfigs = []MariaDBConfig{}

	// Ensure config directory exists
	ensureConfigDirectory()
	configDir := config.ConfigPath

	// Find all .ini and .cnf files
	patterns := []string{"*.ini", "*.cnf"}
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(filepath.Join(configDir, pattern))
		for _, match := range matches {
			configName := strings.TrimSuffix(filepath.Base(match), filepath.Ext(match))

			// Parse the config file to get details
			parsedConfig := parseConfigFile(match)
			parsedConfig.Name = configName
			parsedConfig.Path = match
			parsedConfig.Exists = true

			// Check if this is the active config
			if currentStatus.IsRunning && currentStatus.ConfigFile == match {
				parsedConfig.IsActive = true
			}

			availableConfigs = append(availableConfigs, parsedConfig)
		}
	}

	// Sort configs by name
	sort.Slice(availableConfigs, func(i, j int) bool {
		return availableConfigs[i].Name < availableConfigs[j].Name
	})

	logger.Log("Found %d configuration files in %s", len(availableConfigs), configDir)
}

// Parse a MariaDB config file to extract key information
func parseConfigFile(configPath string) MariaDBConfig {
	config := MariaDBConfig{
		Path:	configPath,
		Exists: pathExists(configPath),
	}

	file, err := os.Open(configPath)
	if err != nil {
		return config
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inMysqldSection := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// Check for [mysqld] section
		if strings.HasPrefix(line, "[") {
			inMysqldSection = strings.ToLower(line) == "[mysqld]"
			continue
		}

		if inMysqldSection {
			// Parse key=value pairs
			if strings.Contains(line, "=") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(strings.ToLower(parts[0]))
					value := strings.Trim(strings.TrimSpace(parts[1]), "\"'")

					switch key {
					case "datadir", "data_dir":
						config.DataDir = value
					case "port":
						config.Port = value
					case "description", "comment":
						config.Description = value
					}
				}
			}
		}
	}

	// Set default port if not specified
	if config.Port == "" {
		config.Port = "3306"
	}

	return config
}

// Ensure config directory exists and create README for user guidance
func ensureConfigDirectory() {
	configDir := config.ConfigPath
	os.MkdirAll(configDir, 0755)

	// Create a README file with instructions
	readmePath := filepath.Join(configDir, "README.txt")
	if !pathExists(readmePath) {
		readme := `MariaDB Configuration Files
============================

This directory is for your MariaDB configuration files.
The DBSwitcher will automatically detect any .ini or .cnf files you place here.

File Format:
- Use .ini or .cnf extension
- Files must contain a [mysqld] section
- Key settings: datadir, port, description (optional)

To add a new configuration:
1. Create a new .ini or .cnf file in this directory
2. Add your MariaDB settings (copy from your existing MariaDB installation)
3. Modify the datadir and port as needed
4. The DBSwitcher will automatically detect it

Tips:
- Use descriptive filenames (e.g., production.ini, testing.ini)
- Add a 'description' field in [mysqld] section for clarity in the UI
- Different configs can use different ports to run simultaneously
- Use forward slashes (/) in paths for better compatibility
- Backup your configurations regularly

Configuration Directory Location:
` + configDir

		os.WriteFile(readmePath, []byte(readme), 0644)
		logger.Log("Created README file for config directory: %s", readmePath)
	}
}

func detectMariaDBBin() string {
	// Use 'which' or 'where' command first
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("where", "mysqld.exe")
	} else {
		cmd = exec.Command("which", "mysqld")
	}

	if output, err := cmd.Output(); err == nil {
		path := strings.TrimSpace(string(output))
		if path != "" {
			return filepath.Dir(path)
		}
	}

	// Fall back to common paths
	switch runtime.GOOS {
	case "windows":
		return detectWindowsMariaDB()
	case "linux":
		return detectLinuxMariaDB()
	case "darwin":
		return detectMacMariaDB()
	case "freebsd":
		return detectFreeBSDMariaDB()
	default:
		return "/usr/local/bin"
	}
}

func detectWindowsMariaDB() string {
	// Check PATH first
	pathEnv := os.Getenv("PATH")
	paths := strings.Split(pathEnv, ";")
	for _, path := range paths {
		if strings.Contains(strings.ToLower(path), "mariadb") || strings.Contains(strings.ToLower(path), "mysql") {
			if _, err := os.Stat(filepath.Join(path, "mysqld.exe")); err == nil {
				return path
			}
		}
	}

	// Common installation paths
	commonPaths := []string{
		`C:\Program Files\MariaDB*\bin`,
		`C:\Program Files (x86)\MariaDB*\bin`,
		`C:\MariaDB*\bin`,
		`C:\xampp\mysql\bin`,
		`C:\wamp64\bin\mariadb\mariadb*\bin`,
		`C:\wamp\bin\mysql\mysql*\bin`,
	}

	for _, pattern := range commonPaths {
		matches, _ := filepath.Glob(pattern)
		for _, match := range matches {
			if _, err := os.Stat(filepath.Join(match, "mysqld.exe")); err == nil {
				return match
			}
		}
	}

	return ""
}

func detectLinuxMariaDB() string {
	paths := []string{
		"/usr/sbin",
		"/usr/bin",
		"/usr/local/bin",
		"/usr/local/mysql/bin",
		"/opt/mariadb/bin",
		"/opt/mysql/bin",
	}

	for _, path := range paths {
		if _, err := os.Stat(filepath.Join(path, "mysqld")); err == nil {
			return path
		}
	}
	return "/usr/bin"
}

func detectMacMariaDB() string {
	paths := []string{
		"/usr/local/mysql/bin",
		"/opt/homebrew/bin",
		"/opt/homebrew/opt/mariadb/bin",
		"/usr/local/bin",
		"/opt/local/bin",
		"/Applications/MAMP/Library/bin",
	}

	for _, path := range paths {
		if _, err := os.Stat(filepath.Join(path, "mysqld")); err == nil {
			return path
		}
	}
	return "/usr/local/bin"
}

func detectFreeBSDMariaDB() string {
	paths := []string{
		"/usr/local/libexec",
		"/usr/local/bin",
		"/usr/local/mysql/bin",
	}

	for _, path := range paths {
		if _, err := os.Stat(filepath.Join(path, "mysqld")); err == nil {
			return path
		}
	}
	return "/usr/local/bin"
}

func isDriveRemovable(path string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	
	if len(path) < 2 || path[1] != ':' {
		return false
	}
	
	// Use PowerShell to get drive type
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		fmt.Sprintf("(Get-WmiObject Win32_LogicalDisk -Filter \"Name='%s'\").DriveType", path[:2]))
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	
	driveType := strings.TrimSpace(string(output))
	if driveType == "2" { // Removable Disk
		return true
	}
	
	return false
}

func detectExternalDrive() string {
	switch runtime.GOOS {
	case "windows":
		// Check for removable drives
		for _, drive := range "DEFGHIJKLMNOPQRSTUVWXYZ" {
			drivePath := string(drive) + ":\\"
			if isDriveRemovable(drivePath[:2]) {
				mariadbPath := filepath.Join(drivePath, "MariaDB", "data")
				if pathExists(mariadbPath) {
					return drivePath[:2]
				}
			}
		}
	case "darwin":
		// Check /Volumes for external drives
		volumes, _ := os.ReadDir("/Volumes")
		for _, vol := range volumes {
			volPath := filepath.Join("/Volumes", vol.Name())
			mariadbPath := filepath.Join(volPath, "MariaDB", "data")
			if pathExists(mariadbPath) {
				return volPath
			}
		}
	case "linux", "freebsd":
		// Check common mount points
		mountPoints := []string{"/mnt", "/media", "/run/media"}
		for _, mp := range mountPoints {
			if dirs, err := os.ReadDir(mp); err == nil {
				for _, dir := range dirs {
					dirPath := filepath.Join(mp, dir.Name())
					mariadbPath := filepath.Join(dirPath, "MariaDB", "data")
					if pathExists(mariadbPath) {
						return dirPath
					}
				}
			}
		}
	}
	return ""
}

func checkElevationRequired() bool {
	switch runtime.GOOS {
	case "windows":
		// Check if we can access service control
		cmd := exec.Command("sc", "query")
		return cmd.Run() != nil
	case "linux", "darwin", "freebsd":
		// Check if we're root
		return os.Geteuid() != 0
	}
	return false
}

func checkServiceControlAvailable() bool {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("sc", "query", config.ServiceNames["windows"])
		return cmd.Run() == nil
	case "linux":
		cmd := exec.Command("systemctl", "status", config.ServiceNames["linux"])
		return cmd.Run() == nil
	case "darwin":
		cmd := exec.Command("launchctl", "list")
		output, _ := cmd.Output()
		return strings.Contains(string(output), "mariadb") || strings.Contains(string(output), "mysql")
	}
	return false
}

// Get MariaDB status with config detection
func getMariaDBStatus() MariaDBStatus {
	status := MariaDBStatus{
		IsRunning:  false,
		ConfigFile: "",
		ConfigName: "Unknown",
		DataPath:	"Not found",
		ProcessID:  0,
	}

	processName := config.ProcessNames[runtime.GOOS]
	if processName == "" {
		processName = "mysqld"
	}

	// Find running MariaDB process
	pid, cmdLine, found := findProcessWithCmdLine(processName)
	if found {
		status.IsRunning = true
		status.ProcessID = pid

		// Try to extract config file from command line
		if configFile := extractConfigFromCmdLine(cmdLine); configFile != "" {
			status.ConfigFile = configFile
			// Find matching config name
			for _, cfg := range availableConfigs {
				if cfg.Path == configFile {
					status.ConfigName = cfg.Name
					break
				}
			}
		}

		// Try to get version
		status.Version = getMariaDBVersion()

		// Try to query MariaDB for actual datadir and port
		if dataDir := queryMariaDBVariable("datadir"); dataDir != "" {
			status.DataPath = dataDir
		}
		if port := queryMariaDBVariable("port"); port != "" {
			status.Port = port
		}
	}

	return status
}

// getMariaDBStatusWithUI gets MariaDB status and can prompt user for credentials if needed
func getMariaDBStatusWithUI(parent fyne.Window, onComplete func(status MariaDBStatus)) {
	status := MariaDBStatus{
		IsRunning:  false,
		ConfigFile: "",
		ConfigName: "Unknown",
		DataPath:   "Not found",
		ProcessID:  0,
	}

	processName := config.ProcessNames[runtime.GOOS]
	if processName == "" {
		processName = "mysqld"
	}

	// Find running MariaDB process
	pid, cmdLine, found := findProcessWithCmdLine(processName)
	if found {
		status.IsRunning = true
		status.ProcessID = pid

		// Try to extract config file from command line
		if configFile := extractConfigFromCmdLine(cmdLine); configFile != "" {
			status.ConfigFile = configFile
			// Find matching config name
			for _, cfg := range availableConfigs {
				if cfg.Path == configFile {
					status.ConfigName = cfg.Name
					break
				}
			}
		}

		// Try to get version
		status.Version = getMariaDBVersion()

		// Query MariaDB for actual datadir and port using UI if needed
		var pendingQueries = 2
		var queryComplete = func() {
			pendingQueries--
			if pendingQueries == 0 {
				onComplete(status)
			}
		}

		// Query datadir
		queryMariaDBVariableWithUI("datadir", parent, func(result string) {
			if result != "" {
				status.DataPath = result
			}
			queryComplete()
		})

		// Query port
		queryMariaDBVariableWithUI("port", parent, func(result string) {
			if result != "" {
				status.Port = result
			}
			queryComplete()
		})
	} else {
		// MariaDB not running, return status immediately
		onComplete(status)
	}
}

// Enhanced process detection that also gets command line
func findProcessWithCmdLine(processName string) (int, string, bool) {
	switch runtime.GOOS {
	case "windows":
		return findWindowsProcessWithCmdLine(processName)
	case "linux", "darwin", "freebsd":
		return findUnixProcessWithCmdLine(processName)
	}
	return 0, "", false
}

func findWindowsProcessWithCmdLine(processName string) (int, string, bool) {
	// Use PowerShell to get process info
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		fmt.Sprintf("Get-WmiObject Win32_Process -Filter \"Name='%s'\" | Select-Object ProcessId,CommandLine | ConvertTo-Json", processName))
	output, err := cmd.Output()
	if err != nil {
		return 0, "", false
	}

	// Handle single object or array
	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" {
		return 0, "", false
	}

	// Try to parse as array first
	var processes []struct {
		ProcessId   int    `json:"ProcessId"`
		CommandLine string `json:"CommandLine"`
	}
	
	if err := json.Unmarshal([]byte(outputStr), &processes); err == nil {
		// Multiple processes found
		if len(processes) > 0 {
			return processes[0].ProcessId, processes[0].CommandLine, true
		}
	} else {
		// Try to parse as single object
		var process struct {
			ProcessId   int    `json:"ProcessId"`
			CommandLine string `json:"CommandLine"`
		}
		if err := json.Unmarshal([]byte(outputStr), &process); err == nil {
			return process.ProcessId, process.CommandLine, true
		}
	}
	
	return 0, "", false
}

func findUnixProcessWithCmdLine(processName string) (int, string, bool) {
	// Use ps to get full command line
	cmd := exec.Command("ps", "aux")
	output, err := cmd.Output()
	if err != nil {
		return 0, "", false
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, processName) && !strings.Contains(line, "grep") {
			fields := strings.Fields(line)
			if len(fields) >= 11 {
				if pid, err := strconv.Atoi(fields[1]); err == nil {
					// Reconstruct command line from fields 10 onward
					cmdLine := strings.Join(fields[10:], " ")
					return pid, cmdLine, true
				}
			}
		}
	}

	return 0, "", false
}

// Extract config file path from command line
func extractConfigFromCmdLine(cmdLine string) string {
	// Look for --defaults-file= parameter
	if idx := strings.Index(cmdLine, "--defaults-file="); idx != -1 {
		start := idx + len("--defaults-file=")
		end := strings.IndexAny(cmdLine[start:], " \t\n")
		if end == -1 {
			return strings.Trim(cmdLine[start:], "\"'")
		}
		return strings.Trim(cmdLine[start:start+end], "\"'")
	}

	// Look for --defaults-file parameter with space
	parts := strings.Fields(cmdLine)
	for i, part := range parts {
		if part == "--defaults-file" && i+1 < len(parts) {
			return strings.Trim(parts[i+1], "\"'")
		}
	}

	return ""
}

func queryMariaDBVariable(variable string) string {
	logger.Log("Querying MariaDB variable: %s", variable)
	
	// Try to load saved credentials from keyring if not already loaded
	if savedCredentials == nil {
		if creds, err := loadCredentialsFromKeyring(); err == nil && creds != nil {
			savedCredentials = creds
		}
	}
	
	// Try with saved credentials first if available and valid
	if savedCredentials != nil {
		// Skip validation for password as it can be empty for some MySQL setups
		if savedCredentials.Username != "" && savedCredentials.Host != "" {
			logger.Log("Using saved credentials for variable query: %s", variable)
			if result := execMySQLQueryWithCredentials(variable, *savedCredentials); result != "" {
				return result
			}
			logger.Log("Saved credentials failed for variable query: %s", variable)
		} else {
			logger.Log("Saved credentials are incomplete for variable query: %s", variable)
			savedCredentials = nil // Clear invalid credentials
		}
	}
	
	logger.Log("No valid saved credentials available for variable query: %s", variable)
	return ""
}

// queryMariaDBVariableWithUI tries to query using credentials, prompting user if needed
func queryMariaDBVariableWithUI(variable string, parent fyne.Window, onComplete func(result string)) {
	logger.Log("Querying MariaDB variable with UI: %s", variable)
	
	// First try the synchronous version (with saved or default credentials)
	if result := queryMariaDBVariable(variable); result != "" {
		onComplete(result)
		return
	}
	
	// If that failed, show credential dialog
	if parent != nil {
		fyne.Do(func() {
			showCredentialsDialog(parent, func(creds MySQLCredentials) {
				// Set defaults if empty
				if creds.Username == "" {
					creds.Username = "root"
				}
				if creds.Host == "" {
					creds.Host = "localhost"
				}
				if creds.Port == "" {
					creds.Port = "3306"
				}
				
				// Test credentials with the query
				result := execMySQLQueryWithCredentials(variable, creds)
				if result != "" {
					// Save valid credentials for future use
					savedCredentials = &creds
					logger.Log("Successfully queried variable %s with provided credentials", variable)
					onComplete(result)
				} else {
					logger.Log("Failed to query variable %s with provided credentials", variable)
					onComplete("")
				}
			}, func() {
				// User cancelled
				logger.Log("User cancelled credential dialog for variable query: %s", variable)
				onComplete("")
			})
		})
	} else {
		// No UI available, return empty
		logger.Log("No UI available for credential prompt, returning empty for variable: %s", variable)
		onComplete("")
	}
}

// execMySQLQueryWithCredentials executes a MySQL query with provided credentials
func execMySQLQueryWithCredentials(variable string, creds MySQLCredentials) string {
	mysqlPath := filepath.Join(config.MariaDBBin, "mysql")
	if runtime.GOOS == "windows" {
		mysqlPath += ".exe"
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// Build command with credentials
	args := []string{
		"-h", creds.Host,
		"-P", creds.Port,
		"-u", creds.Username,
	}
	
	// Add password if provided
	if creds.Password != "" {
		args = append(args, fmt.Sprintf("-p%s", creds.Password))
	}
	
	// Add query options and the actual query
	args = append(args, "-s", "-N", "-e", fmt.Sprintf("SELECT @@%s;", variable))
	
	cmd := exec.CommandContext(ctx, mysqlPath, args...)
	output, err := cmd.Output()
	
	if err != nil {
		logger.Log("MySQL query failed for variable %s: %v", variable, err)
		return ""
	}
	
	result := strings.TrimSpace(string(output))
	logger.Log("MySQL query for %s returned: %s", variable, result)
	return result
}

func getMariaDBVersion() string {
	mysqldPath := filepath.Join(config.MariaDBBin, "mysqld")
	if runtime.GOOS == "windows" {
		mysqldPath += ".exe"
	}

	cmd := exec.Command(mysqldPath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "Unknown"
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 {
		parts := strings.Fields(lines[0])
		for i, part := range parts {
			if strings.Contains(part, "Ver") && i+1 < len(parts) {
				return parts[i+1]
			}
		}
	}
	return "Unknown"
}

func getDefaultDataDir() string {
	switch runtime.GOOS {
	case "windows":
		if config.MariaDBBin != "" {
			return filepath.Join(filepath.Dir(config.MariaDBBin), "data")
		}
		return `C:\Program Files\MariaDB\data`
	case "linux":
		return "/var/lib/mysql"
	case "darwin":
		return "/usr/local/var/mysql"
	case "freebsd":
		return "/var/db/mysql"
	}
	return ""
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func getExecutableName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

func stopLinuxService() error {
	if config.RequireElevation {
		cmd := exec.Command("sudo", "systemctl", "stop", config.ServiceNames["linux"])
		return cmd.Run()
	}
	cmd := exec.Command("systemctl", "stop", config.ServiceNames["linux"])
	return cmd.Run()
}

func stopMacService() error {
	if config.RequireElevation {
		cmd := exec.Command("sudo", "launchctl", "unload", "-w",
			fmt.Sprintf("/Library/LaunchDaemons/com.mariadb.%s.plist", config.ServiceNames["darwin"]))
		return cmd.Run()
	}
	cmd := exec.Command("launchctl", "unload", "-w",
		fmt.Sprintf("~/Library/LaunchAgents/com.mariadb.%s.plist", config.ServiceNames["darwin"]))
	return cmd.Run()
}

// validateCredentials checks if credentials have required fields
func validateCredentials(creds MySQLCredentials) error {
	if creds.Username == "" {
		return fmt.Errorf("username is required")
	}
	if creds.Password == "" {
		return fmt.Errorf("password is required")
	}
	if creds.Host == "" {
		return fmt.Errorf("host is required")
	}
	return nil
}

// stopMariaDBServiceWithUI handles the stop operation with UI credential dialog
func stopMariaDBServiceWithUI(parent fyne.Window, onComplete func(error)) {
	logger.Log("Attempting to stop MariaDB service with UI")
	
	// Check if MySQL is actually running
	if !isMariaDBRunning() {
		logger.Log("MariaDB/MySQL is not running")
		onComplete(nil)
		return
	}
	
	// Use saved credentials if available and valid
	if savedCredentials != nil {
		if err := validateCredentials(*savedCredentials); err != nil {
			logger.Log("Saved credentials are incomplete: %v", err)
			savedCredentials = nil // Clear invalid credentials
		} else {
			logger.Log("Using saved credentials for graceful shutdown...")
			err := stopMySQLWithCredentials(*savedCredentials)
			if err == nil {
				time.Sleep(3 * time.Second)
				if !isMariaDBRunning() {
					logger.Log("Successfully stopped MySQL with saved credentials")
					onComplete(nil)
					return
				}
			}
			logger.Log("Failed to stop with saved credentials: %v", err)
		}
	}
	
	// Show credential dialog if no saved credentials or they failed
	showCredentialsDialog(parent, func(creds MySQLCredentials) {
		// Validate provided credentials
		if err := validateCredentials(creds); err != nil {
			onComplete(fmt.Errorf("Invalid credentials: %v", err))
			return
		}
		
		// Save valid credentials for future use
		savedCredentials = &creds
		
		// Try to stop with provided credentials
		err := stopMySQLWithCredentials(creds)
		if err != nil {
			onComplete(fmt.Errorf("Failed to stop MySQL: %v", err))
		} else {
			time.Sleep(3 * time.Second)
			if !isMariaDBRunning() {
				logger.Log("Successfully stopped MySQL with provided credentials")
				onComplete(nil)
			} else {
				onComplete(fmt.Errorf("MySQL graceful shutdown completed but process is still running"))
			}
		}
	}, func() {
		// User cancelled
		onComplete(fmt.Errorf("Operation cancelled by user"))
	})
}


func isPortAvailable(port string) bool {
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

func findProcessUsingPort(port string) {
	if runtime.GOOS != "windows" {
		return
	}
	
	// Use netstat to find what's using the port
	cmd := exec.Command("netstat", "-ano", "-p", "tcp")
	output, err := cmd.Output()
	if err != nil {
		logger.Log("Failed to run netstat: %v", err)
		return
	}
	
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, ":"+port) && strings.Contains(line, "LISTENING") {
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				pid := fields[len(fields)-1]
				logger.Log("Port %s is being used by PID: %s", port, pid)
				
				// Try to get process name
				if pidInt, err := strconv.Atoi(pid); err == nil {
					getProcessName(pidInt)
				}
			}
		}
	}
}

func getProcessName(pid int) {
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV")
	output, err := cmd.Output()
	if err != nil {
		return
	}
	
	lines := strings.Split(string(output), "\n")
	if len(lines) > 1 {
		// Parse CSV output
		fields := strings.Split(lines[1], ",")
		if len(fields) > 0 {
			processName := strings.Trim(fields[0], "\"")
			logger.Log("PID %d is process: %s", pid, processName)
		}
	}
}

func runElevated(name string, args ...string) error {
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

func startMariaDBWithConfig(configFile string) error {
	logger.Log("========================================")
	logger.Log("STARTING MARIADB")
	logger.Log("========================================")
	
	// Check if MariaDB is already running
	if isMariaDBRunning() {
		logger.Log("MariaDB is already running, need to stop it first")
		
		// Always ensure we have a window context for dialogs
		var window fyne.Window
		if mainWindow != nil {
			window = mainWindow
		} else if fyneApp != nil {
			window = fyneApp.NewWindow("MariaDB Control")
			defer window.Close()
		} else {
			// Create a temporary app and window if needed
			fyneApp = app.NewWithID("mariadb-switcher")
			window = fyneApp.NewWindow("MariaDB Control")
			defer window.Close()
		}
		
		// Show dialog to stop existing instance
		stopCompleted := make(chan bool)
		
		dialog.ShowConfirm("MariaDB Already Running",
			"MariaDB is already running. Would you like to stop it and start with the new configuration?",
			func(confirmed bool) {
				if confirmed {
					// Use the centralized stop function with UI credential handling
					stopMariaDBServiceWithUI(window, func(err error) {
						if err != nil {
							logger.Log("Failed to stop MariaDB: %v", err)
							stopCompleted <- false
						} else {
							stopCompleted <- true
						}
					})
				} else {
					stopCompleted <- false
				}
			}, window)
		
		// Wait for stop operation
		success := <-stopCompleted
		if !success {
			return fmt.Errorf("cannot start: existing MariaDB instance is running")
		}
		
		// Extra wait after stop
		time.Sleep(2 * time.Second)
	}

	// Validate config.MariaDBBin
	if config.MariaDBBin == "" {
		logger.Log("ERROR: MariaDB binary path is empty!")
		return fmt.Errorf("MariaDB binary path not configured")
	}

	// Check if binary directory exists
	if !pathExists(config.MariaDBBin) {
		logger.Log("ERROR: MariaDB binary directory does not exist: %s", config.MariaDBBin)
		return fmt.Errorf("MariaDB binary directory not found: %s", config.MariaDBBin)
	}

	// Build full mysqld path
	mysqldPath := filepath.Join(config.MariaDBBin, getExecutableName("mysqld"))
	logger.Log("Full mysqld path: %s", mysqldPath)
	
	// Check if mysqld exists
	if !pathExists(mysqldPath) {
		logger.Log("ERROR: mysqld not found at: %s", mysqldPath)
		
		// Try mariadbd as alternative
		mariadbdPath := filepath.Join(config.MariaDBBin, getExecutableName("mariadbd"))
		if pathExists(mariadbdPath) {
			logger.Log("Found mariadbd instead of mysqld at: %s", mariadbdPath)
			mysqldPath = mariadbdPath
		} else {
			// Try to find mysqld using which/where
			var findCmd *exec.Cmd
			if runtime.GOOS == "windows" {
				findCmd = exec.Command("where", "mysqld.exe")
			} else {
				findCmd = exec.Command("which", "mysqld")
			}
			
			if output, err := findCmd.Output(); err == nil {
				foundPath := strings.TrimSpace(string(output))
				logger.Log("Found mysqld at: %s", foundPath)
				mysqldPath = foundPath
			} else {
				logger.Log("Could not find mysqld in system PATH")
				return fmt.Errorf("mysqld not found at: %s", mysqldPath)
			}
		}
	}

	// Check if config file exists
	if !pathExists(configFile) {
		logger.Log("ERROR: Configuration file not found: %s", configFile)
		return fmt.Errorf("configuration file not found: %s", configFile)
	}

	// Get absolute path for config file
	absConfigFile, err := filepath.Abs(configFile)
	if err != nil {
		logger.Log("ERROR: Cannot get absolute path for config: %v", err)
		return fmt.Errorf("cannot get absolute path for config: %v", err)
	}
	logger.Log("Absolute config file path: %s", absConfigFile)

	// Parse config first to validate it
	logger.Log("Parsing configuration file...")
	configData := parseConfigFile(configFile)
	logger.Log("Config parsed - DataDir: %s, Port: %s", configData.DataDir, configData.Port)
	
	// Validate and prepare data directory
	if configData.DataDir != "" {
		// Convert to absolute path if relative
		if !filepath.IsAbs(configData.DataDir) {
			configData.DataDir = filepath.Join(filepath.Dir(absConfigFile), configData.DataDir)
			logger.Log("Converted relative datadir to absolute: %s", configData.DataDir)
		}

		if !pathExists(configData.DataDir) {
			logger.Log("Data directory does not exist, creating: %s", configData.DataDir)
			if err := os.MkdirAll(configData.DataDir, 0755); err != nil {
				logger.Log("ERROR: Failed to create data directory: %v", err)
				return fmt.Errorf("failed to create data directory: %v", err)
			}
		}

		// Check if data directory is empty and needs initialization
		if isEmpty, _ := isDirEmpty(configData.DataDir); isEmpty {
			logger.Log("Data directory is empty, needs initialization")
			if err := initializeDataDir(configData.DataDir); err != nil {
				logger.Log("ERROR: Failed to initialize data directory: %v", err)
				// Try alternative initialization
				if err := initializeDataDirAlternative(configData.DataDir, absConfigFile); err != nil {
					return fmt.Errorf("failed to initialize data directory: %v", err)
				}
			}
		} else {
			// Check for critical files in data directory
			if !validateDataDirectory(configData.DataDir) {
				logger.Log("WARNING: Data directory may be corrupted or incomplete")
				// Optionally ask user if they want to reinitialize
			}
		}
	}

	// Check if MySQL/MariaDB is still running - no force stop
	logger.Log("Checking if all MySQL/MariaDB processes are stopped...")
	if isMariaDBRunning() {
		return fmt.Errorf("MySQL/MariaDB is still running - please stop it gracefully with credentials before starting a new instance")
	}
	
	// Double-check the port is free
	if !isPortAvailable(configData.Port) {
		logger.Log("Port %s is still in use", configData.Port)
		return fmt.Errorf("cannot start - port %s is occupied by another process. Please stop all MySQL/MariaDB services gracefully", configData.Port)
	}
	
	logger.Log("Port %s is confirmed available", configData.Port)

	// First, try to validate the config file syntax
	logger.Log("Validating configuration file syntax...")
	if err := validateConfigFile(mysqldPath, absConfigFile); err != nil {
		logger.Log("WARNING: Config file validation failed: %v", err)
		// Continue anyway, as some versions don't support --validate-config
	}

	// Start the MariaDB process with better error capture
	logger.Log("Starting MariaDB with configuration...")
	
	// Create command with proper arguments
	args := []string{
		fmt.Sprintf("--defaults-file=%s", absConfigFile),
		"--console", // Add console output for debugging
	}
	
	cmd := exec.Command(mysqldPath, args...)
	
	// Capture both stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	// Set working directory to bin directory
	cmd.Dir = config.MariaDBBin
	
	// Platform-specific configuration
	if runtime.GOOS == "windows" {
		// Use CREATE_NEW_PROCESS_GROUP to detach process from parent
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true, // Hide console window
			CreationFlags: 0x00000200, // CREATE_NEW_PROCESS_GROUP - allows process to survive parent termination
		}
	}
	
	logger.Log("Executing command: %s %s", mysqldPath, strings.Join(args, " "))
	
	// Start the process
	err = cmd.Start()
	if err != nil {
		logger.Log("ERROR: Failed to start process: %v", err)
		return fmt.Errorf("failed to start MariaDB: %v", err)
	}
	
	logger.Log("Process started with PID: %d", cmd.Process.Pid)
	
	// Release the process so it's detached from parent and can survive parent termination
	err = cmd.Process.Release()
	if err != nil {
		logger.Log("WARNING: Could not release process: %v", err)
		// Continue anyway - the process group flag should still work
	} else {
		logger.Log("Process successfully detached from parent")
	}
	
	// Brief wait to allow process to initialize before verification
	logger.Log("Waiting 3 seconds for MariaDB to initialize...")
	time.Sleep(3 * time.Second)
	
	// Additional verification - try to connect
	logger.Log("Verifying MariaDB is accessible...")
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		if isMariaDBRunning() && isPortListening(configData.Port) {
			logger.Log("MariaDB is running and accepting connections")
			break
		}
		logger.Log("Waiting for MariaDB to be ready... (%d/%d)", i+1, maxRetries)
		time.Sleep(1 * time.Second)
	}
	
	// Final verification
	if !isMariaDBRunning() {
		stdoutStr := stdout.String()
		stderrStr := stderr.String()
		logger.Log("ERROR: MariaDB process not found after startup")
		if stdoutStr != "" {
			logger.Log("Final stdout: %s", stdoutStr)
		}
		if stderrStr != "" {
			logger.Log("Final stderr: %s", stderrStr)
		}
		return fmt.Errorf("MariaDB failed to start - process not found. Check logs for details")
	}
	
	// Save the last used config
	config.LastUsedConfig = absConfigFile
	saveConfig()
	
	// Update global status
	currentStatus = getMariaDBStatus()
	
	logger.Log("========================================")
	logger.Log("MARIADB STARTED SUCCESSFULLY")
	logger.Log("========================================")
	
	return nil
}

// Helper function to validate config file
func validateConfigFile(mysqldPath, configFile string) error {
	cmd := exec.Command(mysqldPath, "--defaults-file="+configFile, "--validate-config")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("validation failed: %s", string(output))
	}
	return nil
}

// Helper function to check if data directory is valid
func validateDataDirectory(dataDir string) bool {
	// Check for essential files/directories
	essentialPaths := []string{
		filepath.Join(dataDir, "mysql"),
		filepath.Join(dataDir, "performance_schema"),
		filepath.Join(dataDir, "ibdata1"),
	}
	
	for _, path := range essentialPaths {
		if !pathExists(path) {
			logger.Log("Missing essential data directory component: %s", path)
			return false
		}
	}
	return true
}

// Alternative initialization method
func initializeDataDirAlternative(dataDir, configFile string) error {
	logger.Log("Attempting alternative data directory initialization...")
	
	// Try with mysql_install_db first
	installDbPath := filepath.Join(config.MariaDBBin, "mysql_install_db")
	if runtime.GOOS == "windows" {
		installDbPath += ".exe"
	}
	
	if pathExists(installDbPath) {
		cmd := exec.Command(installDbPath, 
			"--datadir="+dataDir,
			"--defaults-file="+configFile,
			"--auth-root-authentication-method=normal")
		output, err := cmd.CombinedOutput()
		logger.Log("mysql_install_db output: %s", string(output))
		if err == nil {
			return nil
		}
	}
	
	// Try mysqld --initialize-insecure
	mysqldPath := filepath.Join(config.MariaDBBin, getExecutableName("mysqld"))
	cmd := exec.Command(mysqldPath, 
		"--initialize-insecure",
		"--datadir="+dataDir,
		"--defaults-file="+configFile)
	output, err := cmd.CombinedOutput()
	logger.Log("mysqld --initialize output: %s", string(output))
	return err
}

// Parse MariaDB error messages for common issues
func parseMariaDBError(errorOutput string) string {
	errorLower := strings.ToLower(errorOutput)
	
	// Common error patterns
	if strings.Contains(errorLower, "access denied") {
		return "Access denied - check file permissions"
	}
	if strings.Contains(errorLower, "permission denied") {
		return "Permission denied - may need to run as administrator"
	}
	if strings.Contains(errorLower, "already running") {
		return "Another instance is already running"
	}
	if strings.Contains(errorLower, "can't create/write to file") {
		return "Cannot write to data directory - check permissions"
	}
	if strings.Contains(errorLower, "port") && strings.Contains(errorLower, "use") {
		return "Port is already in use"
	}
	if strings.Contains(errorLower, "unknown variable") {
		return "Invalid configuration file - unknown variable"
	}
	if strings.Contains(errorLower, "innodb") && strings.Contains(errorLower, "log file") {
		return "InnoDB log file error - may need to delete ib_logfile*"
	}
	if strings.Contains(errorLower, "table 'mysql.user' doesn't exist") {
		return "Database not initialized - data directory needs initialization"
	}
	
	// Return first line of error if no pattern matches
	lines := strings.Split(errorOutput, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "Version:") {
			return line
		}
	}
	
	return "Unknown error - check logs for details"
}

// Check if port is actually listening (not just available)
func isPortListening(port string) bool {
	conn, err := net.Dial("tcp", "127.0.0.1:"+port)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}


// Add menu item for credential management
func createCredentialsMenu() *fyne.MenuItem {
	return fyne.NewMenuItem("MySQL Credentials", func() {
		if fyneApp == nil {
			fyneApp = app.NewWithID("mariadb-switcher")
		}
		window := fyneApp.NewWindow("Manage Credentials")
		window.Resize(fyne.NewSize(400, 250))
		
		if savedCredentials != nil {
			info := fmt.Sprintf("Current saved credentials:\nUsername: %s\nHost: %s\nPort: %s",
				savedCredentials.Username, savedCredentials.Host, savedCredentials.Port)
			
			content := container.NewVBox(
				widget.NewLabel(info),
				widget.NewSeparator(),
				widget.NewButton("Update Credentials", func() {
					showCredentialsDialog(window, func(creds MySQLCredentials) {
						savedCredentials = &creds
						dialog.ShowInformation("Success", "Credentials updated", window)
					}, func() {})
				}),
				widget.NewButton("Clear Credentials", func() {
					savedCredentials = nil
					// Also remove from keyring
					if err := deleteCredentialsFromKeyring(); err != nil {
						logger.Log("Failed to delete credentials from keyring: %v", err)
						dialog.ShowError(fmt.Errorf("Failed to clear saved credentials: %v", err), window)
					} else {
						dialog.ShowInformation("Success", "Credentials cleared from memory and secure storage", window)
					}
					window.Close()
				}),
				widget.NewButton("Test Connection", func() {
					if err := testMySQLConnection(*savedCredentials); err != nil {
						dialog.ShowError(err, window)
					} else {
						dialog.ShowInformation("Success", "Connection successful!", window)
					}
				}),
			)
			
			window.SetContent(content)
		} else {
			content := container.NewVBox(
				widget.NewLabel("No credentials saved"),
				widget.NewButton("Add Credentials", func() {
					showCredentialsDialog(window, func(creds MySQLCredentials) {
						savedCredentials = &creds
						dialog.ShowInformation("Success", "Credentials saved", window)
					}, func() {})
				}),
			)
			window.SetContent(content)
		}
		
		window.Show()
	})
}

// Helper functions for debugging

func getCurrentWorkingDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return dir
}

func getCurrentUser() string {
	if runtime.GOOS == "windows" {
		return os.Getenv("USERNAME")
	}
	return os.Getenv("USER")
}

func verifyMariaDBConnection(port string) error {
	mysqlPath := filepath.Join(config.MariaDBBin, "mysql")
	if runtime.GOOS == "windows" {
		mysqlPath += ".exe"
	}
	
	// Try to connect using mysql client
	cmd := exec.Command(mysqlPath, "-u", "root", "--skip-password", 
		"-h", "127.0.0.1", "-P", port, "-e", "SELECT 1")
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		return fmt.Errorf("connection failed: %v, output: %s", err, string(output))
	}
	
	return nil
}

func checkWindowsEventLog() {
	if runtime.GOOS != "windows" {
		return
	}
	
	// Try to read recent Windows Event Log entries
	cmd := exec.Command("wevtutil", "qe", "Application", 
		"/q:*[System[TimeCreated[@SystemTime>='"+time.Now().Add(-1*time.Minute).Format(time.RFC3339)+"']]]",
		"/f:text", "/c:5")
	
	output, err := cmd.Output()
	if err != nil {
		logger.Log("Could not read Windows Event Log: %v", err)
		return
	}
	
	logOutput := string(output)
	if strings.Contains(logOutput, "mysql") || strings.Contains(logOutput, "mariadb") {
		logger.Log("Relevant Windows Event Log entries:\n%s", logOutput)
	}
}

func tryAlternativeStart(mysqldPath, configFile string) error {
	logger.Log("Trying alternative start method with console output...")
	
	// Try starting with --console flag for more output
	cmd := exec.Command(mysqldPath, "--defaults-file="+configFile, "--console")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if runtime.GOOS == "windows" {
		// Try without special flags
		cmd.SysProcAttr = nil
	}
	
	err := cmd.Start()
	if err != nil {
		logger.Log("Alternative start failed: %v", err)
		return err
	}
	
	logger.Log("Alternative start initiated with PID: %d", cmd.Process.Pid)
	
	// Don't wait, just let it run
	time.Sleep(3 * time.Second)
	
	return nil
}

func isDirEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

func initializeDataDir(dataDir string) error {
	// Run mysql_install_db or mysqld --initialize
	installDbPath := filepath.Join(config.MariaDBBin, "mysql_install_db")
	if runtime.GOOS == "windows" {
		installDbPath += ".exe"
	}

	var cmd *exec.Cmd
	if pathExists(installDbPath) {
		cmd = exec.Command(installDbPath, "--datadir="+dataDir)
	} else {
		// Fallback to mysqld --initialize
		mysqldPath := filepath.Join(config.MariaDBBin, getExecutableName("mysqld"))
		cmd = exec.Command(mysqldPath, "--initialize", "--datadir="+dataDir)
	}

	return cmd.Run()
}

// GUI Implementation
func createSystemTray() {
	if systrayRunning {
		logger.Log("System tray already running, skipping creation")
		return
	}
	
	systrayRunning = true
	logger.Log("Starting system tray")
	systray.Run(onTrayReady, onTrayExit)
}

func onTrayReady() {
	systray.SetTitle("DBSwitcher")
	systray.SetTooltip("MariaDB Configuration Switcher")

	// Menu items
	mShow := systray.AddMenuItem("Show", "Show main window")
	systray.AddSeparator()
	mStatus := systray.AddMenuItem("Show Status", "Show current MariaDB status")
	systray.AddSeparator()
	
	// Add dynamic config menu items
	mConfigMenu := systray.AddMenuItem("Start with Config →", "Choose configuration")
	var configSubMenus []*systray.MenuItem
	for _, cfg := range availableConfigs {
		subItem := mConfigMenu.AddSubMenuItem(cfg.Name, cfg.Description)
		configSubMenus = append(configSubMenus, subItem)
	}
	
	mStop := systray.AddMenuItem("Stop MariaDB", "Stop MariaDB service")
	systray.AddSeparator()
	mSettings := systray.AddMenuItem("Settings", "Open settings")
	mLogs := systray.AddMenuItem("View Logs", "View application logs")
	mOpenFolder := systray.AddMenuItem("Open Config Folder", "Open configuration folder")
	mAbout := systray.AddMenuItem("About", "About this application")
	systray.AddSeparator()
	mExit := systray.AddMenuItem("Exit", "Exit monitor")

	// Update tray icon initially
	updateTrayIcon()

	// Start periodic updates
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			updateTrayIcon()
		}
	}()

	// Handle menu clicks
	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				logger.Log("Show menu clicked")
				fyne.Do(func() {
					showMainWindow()
				})
			case <-mStatus.ClickedCh:
				logger.Log("Status menu clicked")
				fyne.Do(func() {
					showStatusDialog()
				})
			case <-mStop.ClickedCh:
				fyne.Do(func() {
					confirmStopMariaDB()
				})
			case <-mSettings.ClickedCh:
				fyne.Do(func() {
					showSettings()
				})
			case <-mLogs.ClickedCh:
				fyne.Do(func() {
					showLogs()
				})
			case <-mOpenFolder.ClickedCh:
				openFolder(config.ConfigPath)
			case <-mAbout.ClickedCh:
				fyne.Do(func() {
					showAbout()
				})
			case <-mExit.ClickedCh:
				logger.Log("Application exiting")
				logger.Close()
				systray.Quit()
			case <-time.After(100 * time.Millisecond):
				// Check config submenu items
				for i, subItem := range configSubMenus {
					select {
					case <-subItem.ClickedCh:
						if i < len(availableConfigs) {
							go startMariaDBWithConfig(availableConfigs[i].Path)
						}
					default:
					}
				}
			}
		}
	}()
}

func onTrayExit() {
	logger.Log("System tray exiting")
	systrayRunning = false
}

func updateTrayIcon() {
	currentStatus = getMariaDBStatus()

	if currentStatus.IsRunning {
		systray.SetTitle("DBSwitcher ✓")
		// Sanitize tooltip text to prevent systray errors
		configName := currentStatus.ConfigName
		port := currentStatus.Port
		if configName == "" {
			configName = "Unknown"
		}
		if port == "" {
			port = "Unknown"
		}
		tooltip := fmt.Sprintf("MariaDB Running (%s - Port %s)", configName, port)
		// Limit tooltip length to prevent Windows systray issues
		if len(tooltip) > 127 {
			tooltip = tooltip[:124] + "..."
		}
		systray.SetTooltip(tooltip)
	} else {
		systray.SetTitle("DBSwitcher ✗")
		systray.SetTooltip("MariaDB Stopped")
	}
}

func showMainWindow() {
	logger.Log("showMainWindow called")
	
	fyne.Do(func() {
		if fyneApp == nil {
			logger.Log("Creating new fyneApp")
			fyneApp = app.NewWithID("mariadb-switcher")
			fyneApp.SetIcon(nil)
		}
		
		// Always create a new window since Fyne windows can't be properly restored after hiding
		logger.Log("Creating new main window")
		mainWindow = fyneApp.NewWindow("DBSwitcher - MariaDB Configuration Manager")
		mainWindow.Resize(fyne.NewSize(1000, 700))

		// Create main interface
		statusCardRef = createStatusCard()
		configCard := createConfigCard()
		quickActionsCard := createQuickActionsCard()

		// Auto-refresh ticker with proper UI update
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				oldStatus := currentStatus
				currentStatus = getMariaDBStatus()
				
				// Only refresh if status changed
				if oldStatus.IsRunning != currentStatus.IsRunning ||
					oldStatus.ConfigName != currentStatus.ConfigName {
					fyne.Do(func() {
						fyne.CurrentApp().Driver().CanvasForObject(statusCardRef).Refresh(statusCardRef)
						updateStatusCard(statusCardRef)
					})
				}
			}
		}()

		// Main content with tabs
		tabs := container.NewAppTabs(
			container.NewTabItem("Dashboard", container.NewVBox(
				statusCardRef,
				quickActionsCard,
			)),
			container.NewTabItem("Configurations", configCard),
		)

		// Create menu
		mainWindow.SetMainMenu(createMainMenu())

		// Set main content
		mainWindow.SetContent(tabs)

		// Initial status update with UI support for credentials
		getMariaDBStatusWithUI(mainWindow, func(status MariaDBStatus) {
			currentStatus = status
			updateStatusCard(statusCardRef)
		})

		// Set up close handler to hide to tray instead of closing
		mainWindow.SetCloseIntercept(func() {
			logger.Log("Main window close intercepted - hiding window")
			mainWindow.Hide()
			mainWindow = nil // Clear reference so we create a new one next time
		})
		
		// Show and bring to front
		logger.Log("Showing main window")
		mainWindow.Show()
		mainWindow.RequestFocus()
	})
}

func showStatusDialog() {
	if fyneApp == nil {
		fyneApp = app.NewWithID("mariadb-switcher")
	}
	window := fyneApp.NewWindow("MariaDB Status")
	window.Resize(fyne.NewSize(500, 400))

	// Get status with UI support for credentials
	getMariaDBStatusWithUI(window, func(status MariaDBStatus) {
		var statusText string
		if status.IsRunning {
			statusText = fmt.Sprintf(`Status: Running
Configuration: %s
Config File: %s
Data Path: %s
Process ID: %d
Port: %s
Version: %s`, status.ConfigName, filepath.Base(status.ConfigFile),
				status.DataPath, status.ProcessID, status.Port, status.Version)
		} else {
			statusText = "Status: Not Running"
		}

		// Add configuration info
		statusText += fmt.Sprintf(`

System Configuration:
MariaDB Binary: %s
Config Directory: %s
Auto-Detected: %v
Service Control: %v`, config.MariaDBBin, config.ConfigPath, 
			config.AutoDetected, config.UseServiceControl)

		entry := widget.NewMultiLineEntry()
		entry.SetText(statusText)
		entry.Disable()

		content := container.NewBorder(
			widget.NewLabel("Current MariaDB Status"),
			widget.NewButton("Close", func() { window.Close() }),
			nil, nil,
			container.NewScroll(entry),
		)

		window.SetContent(content)
		window.Show()
	})
}


func openFileInEditor(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("notepad", path)
	case "darwin":
		cmd = exec.Command("open", "-t", path)
	default:
		// Try common editors
		editors := []string{"xdg-open", "gedit", "kate", "nano", "vi"}
		for _, editor := range editors {
			if _, err := exec.LookPath(editor); err == nil {
				cmd = exec.Command(editor, path)
				break
			}
		}
	}

	if cmd != nil {
		cmd.Start()
	}
}

func openFolder(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}

	if cmd != nil {
		cmd.Start()
	}
}

// Gracefully stop MySQL using admin credentials
func stopMySQLWithCredentials(creds MySQLCredentials) error {
	mysqladminPath := filepath.Join(config.MariaDBBin, "mysqladmin")
	if runtime.GOOS == "windows" {
		mysqladminPath += ".exe"
	}
	
	if !pathExists(mysqladminPath) {
		return fmt.Errorf("mysqladmin not found at %s", mysqladminPath)
	}
	
	// Build shutdown command
	args := []string{
		"-h", creds.Host,
		"-P", creds.Port,
		"-u", creds.Username,
	}
	
	// Add password if provided
	if creds.Password != "" {
		args = append(args, fmt.Sprintf("-p%s", creds.Password))
	}
	
	args = append(args, "shutdown")
	
	// Log the full command for debugging
	cmdStr := mysqladminPath + " " + strings.Join(args, " ")
	fmt.Printf("Executing command: %s\n", cmdStr)
	logger.Log("Executing graceful shutdown with mysqladmin...")
	logger.Log("Command: %s %s", mysqladminPath, strings.Join(args, " "))
	cmd := exec.Command(mysqladminPath, args...)
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		logger.Log("mysqladmin shutdown error: %v\nOutput: %s", err, string(output))
		return fmt.Errorf("shutdown failed: %v", err)
	}
	// wait for 3 seconds
	time.Sleep(3 * time.Second)
	
	logger.Log("MySQL shutdown command executed successfully")
	return nil
}

// Check if MariaDB/MySQL is running
func isMariaDBRunning() bool {
	processName := config.ProcessNames[runtime.GOOS]
	if processName == "" {
		processName = "mysqld"
	}
	
	_, _, found := findProcessWithCmdLine(processName)
	return found
}

// Modified confirmStopMariaDB to include credential prompt
func confirmStopMariaDB() {
	if fyneApp == nil {
		fyneApp = app.NewWithID("mariadb-switcher")
	}
	window := fyneApp.NewWindow("Stop MariaDB")
	window.Resize(fyne.NewSize(400, 200))
	
	// Check if MySQL is running
	if !isMariaDBRunning() {
		dialog.ShowInformation("Info", "MariaDB/MySQL is not currently running", window)
		window.Close()
		return
	}
	
	// Create stop button with credential handling - PRIMARY ACTION
	stopWithCredsBtn := widget.NewButton("Stop with Credentials", func() {
		stopMariaDBServiceWithUI(window, func(err error) {
			if err != nil {
				fyne.CurrentApp().SendNotification(&fyne.Notification{
					Title:   "Stop Failed",
					Content: fmt.Sprintf("Failed to stop: %v", err),
				})
				dialog.ShowError(err, window)
			} else {
				fyne.CurrentApp().SendNotification(&fyne.Notification{
					Title:   "Success",
					Content: "MariaDB/MySQL stopped successfully",
				})
				dialog.ShowInformation("Success", "MariaDB/MySQL has been stopped gracefully", window)
				window.Close()
			}
		})
	})
	stopWithCredsBtn.Importance = widget.HighImportance  // CHANGED: Set to high importance
	
	// Info about manual stop (replaces force stop button)
	manualStopBtn := widget.NewButton("Manual Stop Instructions", func() {
		instructions := `To manually stop MySQL/MariaDB safely:

Windows:
• Open Services (services.msc)
• Find MySQL/MariaDB service and click Stop
• Or use: sc stop mysql

Linux:
• sudo systemctl stop mysql
• Or: sudo systemctl stop mariadb

macOS:
• sudo brew services stop mysql
• Or: sudo launchctl unload -w /Library/LaunchDaemons/com.oracle.oss.mysql.mysqld.plist`
		
		dialog.ShowInformation("Manual Stop Instructions", instructions, window)
	})
	manualStopBtn.Importance = widget.LowImportance
	
	cancelBtn := widget.NewButton("Cancel", func() {
		window.Close()
	})
	
	// Add informative text
	infoLabel := widget.NewLabel("Please provide MySQL admin credentials to stop gracefully.\nThis ensures all data is properly saved before shutdown.")
	infoLabel.Wrapping = fyne.TextWrapWord
	
	content := container.NewVBox(
		infoLabel,  // ADDED: Informative text
		widget.NewSeparator(),
		stopWithCredsBtn,
		manualStopBtn,
		widget.NewSeparator(),
		cancelBtn,
	)
	
	window.SetContent(container.NewCenter(content))
	window.Show()
}

func showSettings() {
	if fyneApp == nil {
		fyneApp = app.NewWithID("mariadb-switcher")
	}
	window := fyneApp.NewWindow("Settings")
	window.Resize(fyne.NewSize(700, 600))

	// Create form fields
	mariadbBinEntry := widget.NewEntry()
	mariadbBinEntry.SetText(config.MariaDBBin)

	browseMariaDBBtn := widget.NewButton("Browse...", func() {
		dialog.ShowFolderOpen(func(dir fyne.ListableURI, err error) {
			if err == nil && dir != nil {
				mariadbBinEntry.SetText(dir.Path())
			}
		}, window)
	})

	configPathEntry := widget.NewEntry()
	configPathEntry.SetText(config.ConfigPath)

	browseConfigBtn := widget.NewButton("Browse...", func() {
		dialog.ShowFolderOpen(func(dir fyne.ListableURI, err error) {
			if err == nil && dir != nil {
				configPathEntry.SetText(dir.Path())
			}
		}, window)
	})

	useServiceCheck := widget.NewCheck("Use Service Control", func(checked bool) {
		config.UseServiceControl = checked
	})
	useServiceCheck.SetChecked(config.UseServiceControl)

	requireElevationCheck := widget.NewCheck("Require Elevation", func(checked bool) {
		config.RequireElevation = checked
	})
	requireElevationCheck.SetChecked(config.RequireElevation)

	// Create tabs for different settings sections
	generalTab := container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("MariaDB Binary Path", 
				container.NewBorder(nil, nil, nil, browseMariaDBBtn, mariadbBinEntry)),
			widget.NewFormItem("Config Directory",
				container.NewBorder(nil, nil, nil, browseConfigBtn, configPathEntry)),
		),
		widget.NewSeparator(),
		useServiceCheck,
		requireElevationCheck,
	)

	// Advanced settings
	processNameEntry := widget.NewEntry()
	processNameEntry.SetText(config.ProcessNames[runtime.GOOS])
	
	serviceNameEntry := widget.NewEntry()
	serviceNameEntry.SetText(config.ServiceNames[runtime.GOOS])

	advancedTab := container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("Process Name", processNameEntry),
			widget.NewFormItem("Service Name", serviceNameEntry),
		),
		widget.NewSeparator(),
		widget.NewLabel(fmt.Sprintf("Platform: %s/%s", runtime.GOOS, runtime.GOARCH)),
		widget.NewLabel(fmt.Sprintf("Settings Location: %s", getConfigPath())),
		widget.NewLabel(fmt.Sprintf("Logs Location: %s", filepath.Join(getAppDataDir(), "dbswitcher.log"))),
	)

	tabs := container.NewAppTabs(
		container.NewTabItem("General", generalTab),
		container.NewTabItem("Advanced", advancedTab),
	)

	// Buttons
	saveBtn := widget.NewButton("Save", func() {
		config.MariaDBBin = mariadbBinEntry.Text
		config.ConfigPath = configPathEntry.Text
		config.ProcessNames[runtime.GOOS] = processNameEntry.Text
		config.ServiceNames[runtime.GOOS] = serviceNameEntry.Text

		if err := saveConfig(); err != nil {
			dialog.ShowError(err, window)
		} else {
			dialog.ShowInformation("Success", "Settings saved successfully", window)
			scanForConfigs() // Rescan configs if path changed
		}
	})

	autoDetectBtn := widget.NewButton("Auto-Detect", func() {
		dialog.ShowConfirm("Auto-Detect", "This will reset settings to auto-detected values. Continue?", 
			func(confirmed bool) {
				if confirmed {
					autoDetectConfig()
					mariadbBinEntry.SetText(config.MariaDBBin)
					processNameEntry.SetText(config.ProcessNames[runtime.GOOS])
					serviceNameEntry.SetText(config.ServiceNames[runtime.GOOS])
				}
			}, window)
	})

	testBtn := widget.NewButton("Test Connection", func() {
		getMariaDBStatusWithUI(window, func(status MariaDBStatus) {
			if status.IsRunning {
				dialog.ShowInformation("Test Result",
					fmt.Sprintf("MariaDB is running\nVersion: %s\nConfig: %s\nPort: %s\nData Path: %s",
						status.Version, status.ConfigName, status.Port, status.DataPath),
					window)
			} else {
				dialog.ShowInformation("Test Result", "MariaDB is not running", window)
			}
		})
	})

	content := container.NewBorder(
		nil,
		container.NewHBox(saveBtn, autoDetectBtn, testBtn, 
			widget.NewButton("Cancel", func() { window.Close() })),
		nil, nil,
		tabs,
	)

	window.SetContent(content)
	window.Show()
}

func showLogs() {
	if fyneApp == nil {
		fyneApp = app.NewWithID("mariadb-switcher")
	}
	window := fyneApp.NewWindow("Application Logs")
	window.Resize(fyne.NewSize(800, 600))

	logPath := filepath.Join(getAppDataDir(), "dbswitcher.log")
	logContent := "No logs available"

	if data, err := os.ReadFile(logPath); err == nil {
		logContent = string(data)
		if logContent == "" {
			logContent = "Log file is empty"
		}
	}

	entry := widget.NewMultiLineEntry()
	entry.SetText(logContent)
	entry.Disable()

	clearBtn := widget.NewButton("Clear Logs", func() {
		if err := os.WriteFile(logPath, []byte(""), 0644); err == nil {
			entry.SetText("Logs cleared")
			dialog.ShowInformation("Success", "Logs cleared successfully", window)
		}
	})

	refreshBtn := widget.NewButton("Refresh", func() {
		if data, err := os.ReadFile(logPath); err == nil {
			entry.SetText(string(data))
		}
	})

	content := container.NewBorder(
		widget.NewLabel("Application Logs"),
		container.NewHBox(clearBtn, refreshBtn, 
			widget.NewButton("Close", func() { window.Close() })),
		nil, nil,
		container.NewScroll(entry),
	)

	window.SetContent(content)
	window.Show()
}

func showAbout() {
	if fyneApp == nil {
		fyneApp = app.NewWithID("mariadb-switcher")
	}
	window := fyneApp.NewWindow("About")
	window.Resize(fyne.NewSize(450, 350))

	aboutText := `DBSwitcher - MariaDB Configuration Manager
Version: 2.1.0

A cross-platform tool for managing multiple MariaDB 
configurations and switching between them seamlessly.

Features:
- Unlimited configuration profiles
- Auto-detection of MariaDB installation
- Support for Windows, Linux, macOS, and FreeBSD
- System tray integration
- Easy configuration switching
- Real-time process monitoring
- User-editable configurations

Configuration Directory:
` + config.ConfigPath + `

© 2024 - Ahmed Aredah`

	entry := widget.NewMultiLineEntry()
	entry.SetText(aboutText)
	entry.Disable()
	
	content := container.NewBorder(
		widget.NewLabel("About DBSwitcher"),
		widget.NewButton("Close", func() { window.Close() }),
		nil, nil,
		entry,
	)

	window.SetContent(content)
	window.Show()
}

// Diagnose MariaDB installation
func diagnoseMariaDB() {
	logger.Log("=== MariaDB Installation Diagnosis ===")
	logger.Log("OS: %s", runtime.GOOS)
	logger.Log("Config MariaDB Binary Path: %s", config.MariaDBBin)
	
	// Check if the binary directory exists
	if pathExists(config.MariaDBBin) {
		logger.Log("Binary directory exists: %s", config.MariaDBBin)
		
		// List files in the binary directory
		files, err := os.ReadDir(config.MariaDBBin)
		if err == nil {
			logger.Log("Files in binary directory:")
			for _, file := range files {
				if strings.Contains(file.Name(), "mysql") {
					logger.Log("  - %s", file.Name())
				}
			}
		}
	} else {
		logger.Log("Binary directory does not exist: %s", config.MariaDBBin)
	}
	
	mysqldPath := filepath.Join(config.MariaDBBin, getExecutableName("mysqld"))
	logger.Log("Expected mysqld path: %s", mysqldPath)
	logger.Log("mysqld exists: %v", pathExists(mysqldPath))
	
	// Try to run mysqld --version
	if pathExists(mysqldPath) {
		cmd := exec.Command(mysqldPath, "--version")
		output, err := cmd.Output()
		if err != nil {
			logger.Log("Error running mysqld --version: %v", err)
		} else {
			logger.Log("mysqld version: %s", strings.TrimSpace(string(output)))
		}
	}
	
	// Check config directory
	logger.Log("Config directory: %s", config.ConfigPath)
	logger.Log("Config directory exists: %v", pathExists(config.ConfigPath))
	
	if pathExists(config.ConfigPath) {
		files, err := os.ReadDir(config.ConfigPath)
		if err == nil {
			logger.Log("Config files:")
			for _, file := range files {
				if strings.HasSuffix(file.Name(), ".ini") || strings.HasSuffix(file.Name(), ".cnf") {
					logger.Log("  - %s", file.Name())
				}
			}
		}
	}
	logger.Log("=== End Diagnosis ===")
}

func main() {
	// Parse command line arguments
	runInTray := false
	showHelp := false

	for _, arg := range os.Args[1:] {
		switch arg {
		case "--tray", "-t":
			runInTray = true
		case "--help", "-h":
			showHelp = true
		case "--version", "-v":
			fmt.Println("DBSwitcher v2.1.0 - MariaDB Configuration Manager")
			return
		case "--config-dir":
			// Allow overriding config directory
			if len(os.Args) > 2 {
				config.ConfigPath = os.Args[2]
				saveConfig()
			}
		}
	}

	if showHelp {
		fmt.Println(`DBSwitcher - MariaDB Configuration Manager

Usage:
dbswitcher [options]

Options:
--tray, -t			  Run in system tray
--config-dir <path>  Set configuration directory
--help, -h			  Show this help message
--version, -v		  Show version information

Without options, the application runs in GUI mode.

Configuration files are stored in:
Windows: %APPDATA%\DBSwitcher\configs
Linux/macOS: ~/.config/DBSwitcher

Examples:
dbswitcher						  # Run GUI
dbswitcher --tray				# Run in system tray
dbswitcher --config-dir /path # Set custom config directory`)
		return
	}

	logger.Log("Application started with args: %v", os.Args)
	
	// Run diagnosis on startup
	diagnoseMariaDB()

	if runInTray {
		createSystemTray()
	} else {
		// Run GUI version
		fyneApp = app.NewWithID("mariadb-switcher")
		fyneApp.SetIcon(nil)
		mainWindow = fyneApp.NewWindow("DBSwitcher - MariaDB Configuration Manager")
		mainWindow.Resize(fyne.NewSize(1000, 700))

		// Create main interface and store reference
		statusCardRef = createStatusCard()
		configCard := createConfigCard()
		quickActionsCard := createQuickActionsCard()

		// Auto-refresh ticker with proper UI update
		go func() {
			ticker := time.NewTicker(5 * time.Second) // Reduced to 5 seconds for faster updates
			defer ticker.Stop()
			for range ticker.C {
				oldStatus := currentStatus
				currentStatus = getMariaDBStatus()
				
				// Only update UI if status changed
				if oldStatus.IsRunning != currentStatus.IsRunning || 
					oldStatus.ProcessID != currentStatus.ProcessID ||
					oldStatus.ConfigName != currentStatus.ConfigName {
					fyne.Do(func() {
						fyne.CurrentApp().Driver().CanvasForObject(statusCardRef).Refresh(statusCardRef)
						updateStatusCard(statusCardRef)
					})
				}
			}
		}()

		// Main content with tabs
		tabs := container.NewAppTabs(
			container.NewTabItem("Dashboard", container.NewVBox(
				statusCardRef,
				quickActionsCard,
			)),
			container.NewTabItem("Configurations", configCard),
		)

		// Create menu
		mainWindow.SetMainMenu(createMainMenu())

		// Set main content
		mainWindow.SetContent(tabs)

		// Initial status update with UI support for credentials
		getMariaDBStatusWithUI(mainWindow, func(status MariaDBStatus) {
			currentStatus = status
			updateStatusCard(statusCardRef)
		})

		// Set up close handler to minimize to tray
		mainWindow.SetCloseIntercept(func() {
			logger.Log("Main window hiding to system tray")
			mainWindow.Hide()
			// Start system tray if not already running
			if !systrayRunning {
				go createSystemTray()
			}
		})

		mainWindow.ShowAndRun()
	}
}

func createMainMenu() *fyne.MainMenu {
	return fyne.NewMainMenu(
		fyne.NewMenu("File",
			fyne.NewMenuItem("Open Config Folder", func() {
				openFolder(config.ConfigPath)
			}),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Settings", func() {
				showSettings()
			}),
			fyne.NewMenuItem("Exit", func() {
				fyneApp.Quit()
			}),
		),
		fyne.NewMenu("View",
			fyne.NewMenuItem("Refresh", func() {
				refreshMainUI()
			}),
			fyne.NewMenuItem("Logs", func() {
				showLogs()
			}),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Appearance", func() {
				showAppearanceSettings()
			}),
		),
		fyne.NewMenu("Tools",
			createCredentialsMenu(),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Run in System Tray", func() {
				mainWindow.Hide()
				if !systrayRunning {
					go createSystemTray()
				}
			}),
		),
		fyne.NewMenu("Help",
			fyne.NewMenuItem("About", func() {
				showAbout()
			}),
			fyne.NewMenuItem("Documentation", func() {
				// Open documentation URL
				switch(runtime.GOOS) {
				case("windows"):
					exec.Command("cmd", "/c", "start", "https://mariadb.org/documentation/").Start()
				case("darwin"):
					exec.Command("open", "https://mariadb.org/documentation/").Start()
				default:
					exec.Command("xdg-open", "https://mariadb.org/documentation/").Start()
				}
			}),
		),
	)
}

func createStatusCard() *widget.Card {
	statusLabel := widget.NewLabel("Checking...")
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}

	versionLabel := widget.NewLabel("Version: Unknown")
	configLabel := widget.NewLabel("Config: None")
	portLabel := widget.NewLabel("Port: -")
	pidLabel := widget.NewLabel("PID: -")
	dataLabel := widget.NewLabel("Data: -")

	card := widget.NewCard("MariaDB Status", "", container.NewVBox(
		statusLabel,
		widget.NewSeparator(),
		container.NewGridWithColumns(2,
			versionLabel,
			configLabel,
			portLabel,
			pidLabel,
		),
		dataLabel,
	))

	// Store references for updates
	card.Content.(*fyne.Container).Objects[0] = statusLabel
	infoContainer := card.Content.(*fyne.Container).Objects[2].(*fyne.Container)
	infoContainer.Objects[0] = versionLabel
	infoContainer.Objects[1] = configLabel
	infoContainer.Objects[2] = portLabel
	infoContainer.Objects[3] = pidLabel
	card.Content.(*fyne.Container).Objects[3] = dataLabel

	return card
}

func updateStatusCard(card *widget.Card) {
	content := card.Content.(*fyne.Container)
	statusLabel := content.Objects[0].(*widget.Label)
	infoContainer := content.Objects[2].(*fyne.Container)
	versionLabel := infoContainer.Objects[0].(*widget.Label)
	configLabel := infoContainer.Objects[1].(*widget.Label)
	portLabel := infoContainer.Objects[2].(*widget.Label)
	pidLabel := infoContainer.Objects[3].(*widget.Label)
	dataLabel := content.Objects[3].(*widget.Label)

	if currentStatus.IsRunning {
		statusLabel.SetText("✅ MariaDB is Running")
		versionLabel.SetText(fmt.Sprintf("Version: %s", currentStatus.Version))
		configLabel.SetText(fmt.Sprintf("Config: %s", currentStatus.ConfigName))
		portLabel.SetText(fmt.Sprintf("Port: %s", currentStatus.Port))
		pidLabel.SetText(fmt.Sprintf("PID: %d", currentStatus.ProcessID))
		dataLabel.SetText(fmt.Sprintf("Data: %s", currentStatus.DataPath))
	} else {
		statusLabel.SetText("🔴 MariaDB is Stopped")
		versionLabel.SetText("Version: -")
		configLabel.SetText("Config: -")
		portLabel.SetText("Port: -")
		pidLabel.SetText("PID: -")
		dataLabel.SetText("Data: -")
	}
}

func createQuickActionsCard() *widget.Card {
	// Quick start dropdown
	configOptions := []string{}
	for _, cfg := range availableConfigs {
		configOptions = append(configOptions, cfg.Name)
	}

	configSelect := widget.NewSelect(configOptions, func(selected string) {
		// Find and start the selected config
		for _, cfg := range availableConfigs {
			if cfg.Name == selected {
				go func(config MariaDBConfig) {
					// Show starting notification
					fyne.CurrentApp().SendNotification(&fyne.Notification{
						Title:	"Starting MariaDB",
						Content: fmt.Sprintf("Starting %s configuration...", config.Name),
					})
					
					err := startMariaDBWithConfig(config.Path)
					
					// Update status after start attempt
					refreshMainUI()
					
					// Update UI on main thread
					fyne.Do(func() {
						fyne.CurrentApp().Driver().CanvasForObject(mainWindow.Content()).Refresh(mainWindow.Content())
						
						// Show result notification
						if err != nil {
							fyne.CurrentApp().SendNotification(&fyne.Notification{
								Title:	"MariaDB Start Failed",
								Content: err.Error(),
							})
							dialog.ShowError(err, mainWindow)
						} else {
							fyne.CurrentApp().SendNotification(&fyne.Notification{
								Title:	"MariaDB Started",
								Content: fmt.Sprintf("Successfully started with %s configuration on port %s", config.Name, config.Port),
							})
							dialog.ShowInformation("Success",
								fmt.Sprintf("MariaDB started with %s configuration\nPort: %s", config.Name, config.Port), 
								mainWindow)
						}
					})
				}(cfg)
				break
			}
		}
	})
	configSelect.PlaceHolder = "Select configuration..."

	startBtn := widget.NewButton("Start", func() {
		if configSelect.Selected != "" {
			configSelect.OnChanged(configSelect.Selected)
		}
	})

	stopBtn := widget.NewButton("Stop MariaDB", func() {
		if currentStatus.IsRunning {
			go func() {
				fyne.CurrentApp().SendNotification(&fyne.Notification{
					Title:	"Stopping MariaDB",
					Content: "Stopping MariaDB service...",
				})
				
				stopMariaDBServiceWithUI(mainWindow, func(err error) {
					refreshMainUI()
					
					// Force UI refresh
					fyne.Do(func() {
						fyne.CurrentApp().Driver().CanvasForObject(mainWindow.Content()).Refresh(mainWindow.Content())
						
						if err != nil {
							fyne.CurrentApp().SendNotification(&fyne.Notification{
								Title:	"Stop Failed",
								Content: err.Error(),
							})
							dialog.ShowError(err, mainWindow)
						} else {
							fyne.CurrentApp().SendNotification(&fyne.Notification{
								Title:	"MariaDB Stopped",
								Content: "MariaDB has been stopped successfully",
							})
							dialog.ShowInformation("Success", "MariaDB stopped successfully", mainWindow)
						}
					})
				})
			}()
		}
	})

	restartBtn := widget.NewButton("Restart", func() {
		if currentStatus.IsRunning {
			go func() {
				// Get current config
				currentConfig := currentStatus.ConfigFile
				if currentConfig == "" && config.LastUsedConfig != "" {
					currentConfig = config.LastUsedConfig
				}
				
				// Stop with UI credential handling
				stopMariaDBServiceWithUI(mainWindow, func(stopErr error) {
					if stopErr != nil {
						// Update UI on main thread
						fyne.Do(func() {
							dialog.ShowError(stopErr, mainWindow)
						})
						return
					}
					
					time.Sleep(3 * time.Second)
					
					// Start with same config
					if currentConfig != "" {
						startErr := startMariaDBWithConfig(currentConfig)
						refreshMainUI()
						
						// Update UI on main thread
						fyne.Do(func() {
							if startErr != nil {
								dialog.ShowError(startErr, mainWindow)
							} else {
								dialog.ShowInformation("Success", "MariaDB restarted successfully", mainWindow)
							}
						})
					}
				})
			}()
		}
	})


	openFolderBtn := widget.NewButton("Open Config Folder", func() {
		openFolder(config.ConfigPath)
	})

	return widget.NewCard("Quick Actions", "", container.NewVBox(
		container.NewBorder(nil, nil, widget.NewLabel("Start with:"), startBtn, configSelect),
		container.NewGridWithColumns(2, stopBtn, restartBtn),
		openFolderBtn,
	))
}

func refreshMainUI() {
	if mainWindow != nil && mainWindow.Content() != nil {
		// Use the UI-enabled version that can prompt for credentials
		getMariaDBStatusWithUI(mainWindow, func(status MariaDBStatus) {
			currentStatus = status
			scanForConfigs()
			
			// Refresh all UI components in the main UI thread
			fyne.Do(func() {
				fyne.CurrentApp().Driver().CanvasForObject(mainWindow.Content()).Refresh(mainWindow.Content())
				
				// If you have a reference to the status card, update it directly
				if statusCard := findStatusCard(); statusCard != nil {
					updateStatusCard(statusCard)
				}
			})
		})
	}
}

// Helper to find the status card in the UI
func findStatusCard() *widget.Card {
	if mainWindow == nil || mainWindow.Content() == nil {
		return nil
	}
	
	// Navigate through the tabs to find the status card
	if tabs, ok := mainWindow.Content().(*container.AppTabs); ok {
		if tabs.Items[0].Content != nil {
			if vbox, ok := tabs.Items[0].Content.(*fyne.Container); ok && len(vbox.Objects) > 0 {
				if card, ok := vbox.Objects[0].(*widget.Card); ok {
					return card
				}
			}
		}
	}
	return nil
}

func createConfigCard() fyne.CanvasObject {
	// Refresh configs
	scanForConfigs()

	// Track selected config index
	var selectedConfig int = -1

	// Create config list with better formatting
	configList := widget.NewList(
		func() int { return len(availableConfigs) },
		func() fyne.CanvasObject {
			nameLabel := widget.NewLabel("Config Name")
			nameLabel.TextStyle = fyne.TextStyle{Bold: true}
			portLabel := widget.NewLabel("Port: 3306")
			statusLabel := widget.NewLabel("Ready")
			descLabel := widget.NewLabel("Description")
			
			return container.NewVBox(
				container.NewHBox(nameLabel, layout.NewSpacer(), portLabel, statusLabel),
				descLabel,
				widget.NewSeparator(),
			)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			c := o.(*fyne.Container)
			cfg := availableConfigs[i]
			
			topRow := c.Objects[0].(*fyne.Container)
			nameLabel := topRow.Objects[0].(*widget.Label)
			portLabel := topRow.Objects[2].(*widget.Label)
			statusLabel := topRow.Objects[3].(*widget.Label)
			descLabel := c.Objects[1].(*widget.Label)
			
			nameLabel.SetText(cfg.Name)
			portLabel.SetText("Port: " + cfg.Port)
			
			status := "Ready"
			if cfg.IsActive && currentStatus.IsRunning {
				status = "● ACTIVE"
				statusLabel.TextStyle = fyne.TextStyle{Bold: true}
			} else if !cfg.Exists {
				status = "Missing"
			}
			statusLabel.SetText(status)
			
			desc := cfg.Description
			if desc == "" {
				desc = fmt.Sprintf("Data: %s", cfg.DataDir)
			}
			descLabel.SetText(desc)
		},
	)

	// Handle selection
	configList.OnSelected = func(id widget.ListItemID) {
		selectedConfig = id
	}

	// Status bar
	statusBar := widget.NewLabel("")
	updateStatusBar := func() {
		if currentStatus.IsRunning {
			statusBar.SetText(fmt.Sprintf("MariaDB is running with %s configuration on port %s", 
				currentStatus.ConfigName, currentStatus.Port))
		} else {
			statusBar.SetText("MariaDB is not running")
		}
	}
	updateStatusBar()

	// Buttons
	startBtn := widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), func() {
		if selectedConfig >= 0 && selectedConfig < len(availableConfigs) {
			cfg := availableConfigs[selectedConfig]
			statusBar.SetText(fmt.Sprintf("Starting %s configuration...", cfg.Name))
			
			go func(config MariaDBConfig) {
				err := startMariaDBWithConfig(config.Path)
				
				// Always update status after operation
				refreshMainUI()
				
				// Update UI on main thread
				fyne.Do(func() {
					fyne.CurrentApp().Driver().CanvasForObject(mainWindow.Content()).Refresh(mainWindow.Content())
					
					if err != nil {
						fyne.CurrentApp().SendNotification(&fyne.Notification{
							Title:	"Start Failed",
							Content: fmt.Sprintf("Failed to start %s: %v", config.Name, err),
						})
						statusBar.SetText("Failed to start MariaDB")
						dialog.ShowError(err, mainWindow)
					} else {
						fyne.CurrentApp().SendNotification(&fyne.Notification{
							Title:	"MariaDB Started",
							Content: fmt.Sprintf("Successfully started %s on port %s", config.Name, config.Port),
						})
						dialog.ShowInformation("Success",
							fmt.Sprintf("Started MariaDB with %s configuration\nPort: %s\nData: %s", 
								config.Name, config.Port, config.DataDir), mainWindow)
						configList.Refresh()
						updateStatusBar()
					}
				})
			}(cfg)
		}
	})

	stopBtn := widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), func() {
		statusBar.SetText("Stopping MariaDB...")
		go func() {
			stopMariaDBServiceWithUI(mainWindow, func(err error) {
				refreshMainUI()
				
				// Update UI on main thread
				fyne.Do(func() {
					if err != nil {
						dialog.ShowError(err, mainWindow)
						statusBar.SetText("Failed to stop MariaDB")
					} else {
						dialog.ShowInformation("Success", "MariaDB stopped successfully", mainWindow)
						configList.Refresh()
						updateStatusBar()
					}
				})
			})
		}()
	})

	editBtn := widget.NewButtonWithIcon("Edit", theme.DocumentIcon(), func() {
		if selectedConfig >= 0 && selectedConfig < len(availableConfigs) {
			cfg := availableConfigs[selectedConfig]
			openFileInEditor(cfg.Path)
		}
	})

	deleteBtn := widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), func() {
		if selectedConfig >= 0 && selectedConfig < len(availableConfigs) {
			cfg := availableConfigs[selectedConfig]
			dialog.ShowConfirm("Delete Configuration",
				fmt.Sprintf("Are you sure you want to delete %s.ini?", cfg.Name),
				func(confirm bool) {
					if confirm {
						os.Remove(cfg.Path)
						scanForConfigs()
						configList.Refresh()
					}
				}, mainWindow)
		}
	})

	openFolderBtn := widget.NewButtonWithIcon("Open Folder", theme.FolderOpenIcon(), func() {
		openFolder(config.ConfigPath)
	})

	refreshBtn := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
		refreshMainUI()
		configList.Refresh()
		updateStatusBar()
	})

	// Layout
	toolbar := container.NewHBox(
		startBtn,
		stopBtn,
		widget.NewSeparator(),
		editBtn,
		deleteBtn,
		widget.NewSeparator(),
		openFolderBtn,
		refreshBtn,
	)

	// Info panel
	infoText := fmt.Sprintf(`Configuration Directory: %s
Available Configurations: %d
Current Status: `, config.ConfigPath, len(availableConfigs))
	
	if currentStatus.IsRunning {
		infoText += fmt.Sprintf("Running (%s)", currentStatus.ConfigName)
	} else {
		infoText += "Stopped"
	}
	
	infoLabel := widget.NewLabel(infoText)

	content := container.NewBorder(
		container.NewVBox(
			toolbar,
			widget.NewSeparator(),
		),
		container.NewVBox(
			widget.NewSeparator(),
			statusBar,
			infoLabel,
		),
		nil, nil,
		configList,
	)

	return content
}