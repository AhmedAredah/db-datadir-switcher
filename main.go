package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
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
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/getlantern/systray"
)

// Configuration structure
type Config struct {
	MariaDBBin        string            `json:"mariadb_bin"`
	ConfigPath        string            `json:"config_path"` // User-editable config directory
	LastUsedConfig    string            `json:"last_used_config"`
	ProcessNames      map[string]string `json:"process_names"`
	ServiceNames      map[string]string `json:"service_names"`
	AutoDetected      bool              `json:"auto_detected"`
	UseServiceControl bool              `json:"use_service_control"`
	RequireElevation  bool              `json:"require_elevation"`
}

// MariaDBConfig represents a detected configuration file
type MariaDBConfig struct {
	Name        string `json:"name"`        // Friendly name (e.g., "internal", "external", "development")
	Path        string `json:"path"`        // Full path to config file
	DataDir     string `json:"data_dir"`    // Data directory from config
	Port        string `json:"port"`        // Port from config
	Description string `json:"description"` // User description
	IsActive    bool   `json:"is_active"`   // Currently running with this config
	Exists      bool   `json:"exists"`      // File exists
}

// MariaDBStatus represents the current state
type MariaDBStatus struct {
	IsRunning   bool   `json:"is_running"`
	ConfigFile  string `json:"config_file"` // Current config file path
	ConfigName  string `json:"config_name"` // Friendly name of config
	DataPath    string `json:"data_path"`
	ProcessID   int    `json:"process_id"`
	Port        string `json:"port"`
	ServiceName string `json:"service_name,omitempty"`
	Version     string `json:"version,omitempty"`
}

var (
	config           Config
	currentStatus    MariaDBStatus
	availableConfigs []MariaDBConfig
	fyneApp          fyne.App
	mainWindow       fyne.Window
	logger           *Logger
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
}

func loadConfig() {
	// Default configuration
	config = Config{
		ProcessNames: map[string]string{
			"windows": "mysqld.exe",
			"linux":   "mysqld",
			"darwin":  "mysqld",
			"freebsd": "mysqld",
		},
		ServiceNames: map[string]string{
			"windows": "MariaDB",
			"linux":   "mariadb",
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
	if data, err := ioutil.ReadFile(configFile); err == nil {
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
	return ioutil.WriteFile(getConfigPath(), data, 0644)
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
		Path:   configPath,
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

		ioutil.WriteFile(readmePath, []byte(readme), 0644)
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

func detectExternalDrive() string {
	switch runtime.GOOS {
	case "windows":
		// Check for removable drives
		for _, drive := range "DEFGHIJKLMNOPQRSTUVWXYZ" {
			drivePath := string(drive) + ":\\"
			if isDriveRemovable(drivePath) {
				mariadbPath := filepath.Join(drivePath, "MariaDB", "data")
				if pathExists(mariadbPath) {
					return drivePath[:2]
				}
			}
		}
	case "darwin":
		// Check /Volumes for external drives
		volumes, _ := ioutil.ReadDir("/Volumes")
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
			if dirs, err := ioutil.ReadDir(mp); err == nil {
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

func isDriveRemovable(path string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	// This would need Windows API calls for proper detection
	// For now, assume drives after C: might be removable
	return path[0] > 'C'
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
	// Use WMIC to get command line
	cmd := exec.Command("wmic", "process", "where", fmt.Sprintf("name='%s'", processName),
		"get", "ProcessId,CommandLine", "/FORMAT:CSV")
	output, err := cmd.Output()
	if err != nil {
		return 0, "", false
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, processName) {
			parts := strings.Split(line, ",")
			if len(parts) >= 3 {
				cmdLine := parts[1]
				pidStr := parts[2]
				if pid, err := strconv.Atoi(strings.TrimSpace(pidStr)); err == nil {
					return pid, cmdLine, true
				}
			}
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
	mysqlPath := filepath.Join(config.MariaDBBin, "mysql")
	if runtime.GOOS == "windows" {
		mysqlPath += ".exe"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try without password first
	cmd := exec.CommandContext(ctx, mysqlPath, "-u", "root", "--skip-password",
		"-s", "-N", "-e", fmt.Sprintf("SELECT @@%s;", variable))
	output, err := cmd.Output()
	if err != nil {
		// Try with socket for Unix systems
		if runtime.GOOS != "windows" {
			socketPaths := []string{
				"/var/run/mysqld/mysqld.sock",
				"/tmp/mysql.sock",
				"/var/lib/mysql/mysql.sock",
			}
			for _, socket := range socketPaths {
				if pathExists(socket) {
					cmd = exec.CommandContext(ctx, mysqlPath, "-u", "root",
						"--socket="+socket, "-s", "-N", "-e", fmt.Sprintf("SELECT @@%s;", variable))
					output, err = cmd.Output()
					if err == nil {
						break
					}
				}
			}
		}
	}

	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(output))
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

// Process management
func stopMariaDBService() error {
	logger.Log("Attempting to stop MariaDB service")

	if config.UseServiceControl {
		switch runtime.GOOS {
		case "windows":
			return stopWindowsService()
		case "linux":
			return stopLinuxService()
		case "darwin":
			return stopMacService()
		}
	}

	// Fallback to process termination
	return stopMariaDBProcess()
}

func stopWindowsService() error {
	if config.RequireElevation {
		return runElevated("sc", "stop", config.ServiceNames["windows"])
	}
	cmd := exec.Command("sc", "stop", config.ServiceNames["windows"])
	return cmd.Run()
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

func stopMariaDBProcess() error {
	processName := config.ProcessNames[runtime.GOOS]
	if processName == "" {
		processName = "mysqld"
	}

	if pid, _, found := findProcessWithCmdLine(processName); found {
		logger.Log("Stopping MariaDB process with PID: %d", pid)

		if runtime.GOOS == "windows" {
			cmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
			return cmd.Run()
		}

		// Unix systems
		process, err := os.FindProcess(pid)
		if err != nil {
			return err
		}

		// Try graceful shutdown first
		if err := process.Signal(syscall.SIGTERM); err == nil {
			time.Sleep(5 * time.Second)

			// Check if still running
			if _, _, stillRunning := findProcessWithCmdLine(processName); !stillRunning {
				return nil
			}
		}

		// Force kill if still running
		return process.Signal(syscall.SIGKILL)
	}

	return fmt.Errorf("MariaDB process not found")
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
	logger.Log("Starting MariaDB with config: %s", configFile)
	logger.Log("MariaDB binary path: %s", config.MariaDBBin)

	// Stop existing process first
	stopMariaDBService()
	time.Sleep(3 * time.Second)

	mysqldPath := filepath.Join(config.MariaDBBin, getExecutableName("mysqld"))
	logger.Log("Full mysqld path: %s", mysqldPath)

	if !pathExists(mysqldPath) {
		return fmt.Errorf("mysqld not found at: %s", mysqldPath)
	}

	// Check if config file exists
	if !pathExists(configFile) {
		return fmt.Errorf("configuration file not found: %s", configFile)
	}

	// Validate data directory from config
	configData := parseConfigFile(configFile)
	logger.Log("Config data directory: %s, Port: %s", configData.DataDir, configData.Port)
	
	if configData.DataDir != "" && !pathExists(configData.DataDir) {
		// Try to create data directory
		logger.Log("Creating data directory: %s", configData.DataDir)
		if err := os.MkdirAll(configData.DataDir, 0755); err != nil {
			return fmt.Errorf("data directory does not exist and could not be created: %s - %v", configData.DataDir, err)
		}
		// Initialize if empty
		if isEmpty, _ := isDirEmpty(configData.DataDir); isEmpty {
			logger.Log("Initializing empty data directory")
			if err := initializeDataDir(configData.DataDir); err != nil {
				return fmt.Errorf("failed to initialize data directory: %v", err)
			}
		}
	}

	// Create a pipe to capture error output
	var stderr bytes.Buffer
	cmd := exec.Command(mysqldPath, fmt.Sprintf("--defaults-file=%s", configFile))
	cmd.Stderr = &stderr

	// Platform-specific process configuration
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: 0x08000000, // CREATE_NO_WINDOW
		}
	}

	logger.Log("Executing command: %s --defaults-file=%s", mysqldPath, configFile)
	
	// Try running with --help first to validate the config file syntax
	testCmd := exec.Command(mysqldPath, fmt.Sprintf("--defaults-file=%s", configFile), "--help", "--verbose")
	testOutput, testErr := testCmd.CombinedOutput()
	if testErr != nil {
		logger.Log("Config validation failed: %v", testErr)
		logger.Log("Config validation output: %s", string(testOutput))
		return fmt.Errorf("configuration file validation failed: %s", string(testOutput))
	}
	
	err := cmd.Start()
	if err != nil {
		logger.Log("Failed to start MariaDB: %v", err)
		return fmt.Errorf("failed to start MariaDB: %v", err)
	}

	logger.Log("Process started with PID: %d", cmd.Process.Pid)

	// Wait a bit for the process to either start or fail
	done := make(chan error)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		// Process exited quickly - likely an error
		errorOutput := stderr.String()
		logger.Log("MariaDB process exited immediately with error: %v", err)
		logger.Log("Error output: %s", errorOutput)
		if errorOutput != "" {
			return fmt.Errorf("MariaDB failed to start: %s", errorOutput)
		}
		return fmt.Errorf("MariaDB process exited immediately: %v", err)
	case <-time.After(3 * time.Second):
		// Process is still running after 3 seconds - good sign
		logger.Log("Process still running after 3 seconds, checking for mysqld process")
	}

	// Additional wait and verify startup
	time.Sleep(2 * time.Second)

	processName := config.ProcessNames[runtime.GOOS]
	if processName == "" {
		processName = "mysqld"
	}

	if _, _, found := findProcessWithCmdLine(processName); found {
		logger.Log("MariaDB started successfully with config: %s", filepath.Base(configFile))
		config.LastUsedConfig = configFile
		saveConfig()
		return nil
	}

	// Try to get any error output
	errorOutput := stderr.String()
	if errorOutput != "" {
		logger.Log("MariaDB stderr output: %s", errorOutput)
		return fmt.Errorf("MariaDB failed to start. Error output: %s", errorOutput)
	}

	return fmt.Errorf("MariaDB failed to start - process not found after launch")
}

func isDirEmpty(dir string) (bool, error) {
	entries, err := ioutil.ReadDir(dir)
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
	systray.Run(onTrayReady, onTrayExit)
}

func onTrayReady() {
	systray.SetTitle("DBSwitcher")
	systray.SetTooltip("MariaDB Configuration Switcher")

	// Menu items
	mStatus := systray.AddMenuItem("Show Status", "Show current MariaDB status")
	mConfigs := systray.AddMenuItem("Configuration Manager", "Manage configurations")
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
			case <-mStatus.ClickedCh:
				showStatusDialog()
			case <-mConfigs.ClickedCh:
				showConfigurationManager()
			case <-mStop.ClickedCh:
				confirmStopMariaDB()
			case <-mSettings.ClickedCh:
				showSettings()
			case <-mLogs.ClickedCh:
				showLogs()
			case <-mOpenFolder.ClickedCh:
				openFolder(config.ConfigPath)
			case <-mAbout.ClickedCh:
				showAbout()
			case <-mExit.ClickedCh:
				logger.Log("Application exiting")
				logger.Close()
				systray.Quit()
			default:
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
	// Cleanup
}

func updateTrayIcon() {
	currentStatus = getMariaDBStatus()

	if currentStatus.IsRunning {
		systray.SetTitle("DBSwitcher ✓")
		systray.SetTooltip(fmt.Sprintf("MariaDB Running (%s - Port %s)", currentStatus.ConfigName, currentStatus.Port))
	} else {
		systray.SetTitle("DBSwitcher ✗")
		systray.SetTooltip("MariaDB Stopped")
	}
}

func showStatusDialog() {
	if fyneApp == nil {
		fyneApp = app.New()
	}
	window := fyneApp.NewWindow("MariaDB Status")
	window.Resize(fyne.NewSize(500, 400))

	status := getMariaDBStatus()

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
}

func showConfigurationManager() {
	if fyneApp == nil {
		fyneApp = app.New()
	}
	window := fyneApp.NewWindow("Configuration Manager")
	window.Resize(fyne.NewSize(900, 600))

	// Refresh configs
	scanForConfigs()
	currentStatus = getMariaDBStatus()

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
			go func() {
				err := startMariaDBWithConfig(cfg.Path)
				scanForConfigs()
				currentStatus = getMariaDBStatus()
				
				// Update UI on main thread
				fyne.Do(func() {
					if err != nil {
						dialog.ShowError(err, window)
						statusBar.SetText("Failed to start MariaDB")
					} else {
						dialog.ShowInformation("Success",
							fmt.Sprintf("Started MariaDB with %s configuration", cfg.Name), window)
						configList.Refresh()
						updateStatusBar()
					}
				})
			}()
		}
	})

	stopBtn := widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), func() {
		statusBar.SetText("Stopping MariaDB...")
		go func() {
			err := stopMariaDBService()
			scanForConfigs()
			currentStatus = getMariaDBStatus()
			
			// Update UI on main thread
			fyne.Do(func() {
				if err != nil {
					dialog.ShowError(err, window)
					statusBar.SetText("Failed to stop MariaDB")
				} else {
					dialog.ShowInformation("Success", "MariaDB stopped successfully", window)
					configList.Refresh()
					updateStatusBar()
				}
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
				}, window)
		}
	})

	openFolderBtn := widget.NewButtonWithIcon("Open Folder", theme.FolderOpenIcon(), func() {
		openFolder(config.ConfigPath)
	})

	refreshBtn := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
		scanForConfigs()
		currentStatus = getMariaDBStatus()
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

	window.SetContent(content)
	window.Show()
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

func confirmStopMariaDB() {
	if fyneApp == nil {
		fyneApp = app.New()
	}
	window := fyneApp.NewWindow("Confirm Stop")

	dialog.ShowConfirm("Stop MariaDB", "Are you sure you want to stop MariaDB?", func(confirmed bool) {
		if confirmed {
			err := stopMariaDBService()
			if err != nil {
				dialog.ShowError(err, window)
			} else {
				dialog.ShowInformation("Success", "MariaDB stopped successfully", window)
			}
		}
	}, window)

	window.Resize(fyne.NewSize(300, 150))
	window.Show()
}

func showSettings() {
	if fyneApp == nil {
		fyneApp = app.New()
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
		status := getMariaDBStatus()
		if status.IsRunning {
			dialog.ShowInformation("Test Result",
				fmt.Sprintf("MariaDB is running\nVersion: %s\nConfig: %s\nPort: %s\nData Path: %s",
					status.Version, status.ConfigName, status.Port, status.DataPath),
				window)
		} else {
			dialog.ShowInformation("Test Result", "MariaDB is not running", window)
		}
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
		fyneApp = app.New()
	}
	window := fyneApp.NewWindow("Application Logs")
	window.Resize(fyne.NewSize(800, 600))

	logPath := filepath.Join(getAppDataDir(), "dbswitcher.log")
	logContent := "No logs available"

	if data, err := ioutil.ReadFile(logPath); err == nil {
		logContent = string(data)
		if logContent == "" {
			logContent = "Log file is empty"
		}
	}

	entry := widget.NewMultiLineEntry()
	entry.SetText(logContent)
	entry.Disable()

	clearBtn := widget.NewButton("Clear Logs", func() {
		if err := ioutil.WriteFile(logPath, []byte(""), 0644); err == nil {
			entry.SetText("Logs cleared")
			dialog.ShowInformation("Success", "Logs cleared successfully", window)
		}
	})

	refreshBtn := widget.NewButton("Refresh", func() {
		if data, err := ioutil.ReadFile(logPath); err == nil {
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
		fyneApp = app.New()
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

© 2024 - Enhanced Multi-Config Edition`

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
		files, err := ioutil.ReadDir(config.MariaDBBin)
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
		files, err := ioutil.ReadDir(config.ConfigPath)
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
 --tray, -t           Run in system tray
 --config-dir <path>  Set configuration directory
 --help, -h           Show this help message
 --version, -v        Show version information

Without options, the application runs in GUI mode.

Configuration files are stored in:
 Windows: %APPDATA%\DBSwitcher\configs
 Linux/macOS: ~/.config/DBSwitcher

Examples:
 dbswitcher                    # Run GUI
 dbswitcher --tray            # Run in system tray
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
   	fyneApp = app.New()
   	fyneApp.SetIcon(nil) // You can set a custom icon here
   	mainWindow = fyneApp.NewWindow("DBSwitcher - MariaDB Configuration Manager")
   	mainWindow.Resize(fyne.NewSize(1000, 700))

   	// Create main interface
   	statusCard := createStatusCard()
   	configCard := createConfigCard()
   	quickActionsCard := createQuickActionsCard()

   	// Auto-refresh ticker
   	go func() {
   		ticker := time.NewTicker(10 * time.Second)
   		defer ticker.Stop()
   		for range ticker.C {
   			currentStatus = getMariaDBStatus()
   			// Use fyne.Do for thread-safe UI updates
   			fyne.Do(func() {
   				updateStatusCard(statusCard)
   			})
   		}
   	}()

   	// Main content with tabs
   	tabs := container.NewAppTabs(
   		container.NewTabItem("Dashboard", container.NewVBox(
   			statusCard,
   			quickActionsCard,
   		)),
   		container.NewTabItem("Configurations", configCard),
   	)

   	// Create menu
   	mainWindow.SetMainMenu(createMainMenu())

   	// Set main content
   	mainWindow.SetContent(tabs)

   	// Initial status update
   	currentStatus = getMariaDBStatus()
   	updateStatusCard(statusCard)

   	// Set up close handler
   	mainWindow.SetCloseIntercept(func() {
   		logger.Log("Application closing")
   		logger.Close()
   		mainWindow.Close()
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
   			scanForConfigs()
   			currentStatus = getMariaDBStatus()
   		}),
   		fyne.NewMenuItem("Logs", func() {
   			showLogs()
   		}),
   	),
   	fyne.NewMenu("Tools",
   		fyne.NewMenuItem("Configuration Manager", func() {
   			showConfigurationManager()
   		}),
   		fyne.NewMenuItem("Run in System Tray", func() {
   			mainWindow.Hide()
   			go createSystemTray()
   		}),
   	),
   	fyne.NewMenu("Help",
   		fyne.NewMenuItem("About", func() {
   			showAbout()
   		}),
   		fyne.NewMenuItem("Documentation", func() {
   			// Open documentation URL
   			if runtime.GOOS == "windows" {
   				exec.Command("cmd", "/c", "start", "https://mariadb.org/documentation/").Start()
   			} else if runtime.GOOS == "darwin" {
   				exec.Command("open", "https://mariadb.org/documentation/").Start()
   			} else {
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
   			go func() {
   				err := startMariaDBWithConfig(cfg.Path)
   				currentStatus = getMariaDBStatus()
   				
   				// Update UI on main thread
   				fyne.Do(func() {
   					if err != nil {
   						dialog.ShowError(err, mainWindow)
   					} else {
   						dialog.ShowInformation("Success",
   							fmt.Sprintf("Started MariaDB with %s configuration", cfg.Name), mainWindow)
   					}
   				})
   			}()
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
   			err := stopMariaDBService()
   			currentStatus = getMariaDBStatus()
   			
   			// Update UI on main thread
   			fyne.Do(func() {
   				if err != nil {
   					dialog.ShowError(err, mainWindow)
   				} else {
   					dialog.ShowInformation("Success", "MariaDB stopped successfully", mainWindow)
   				}
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
   			
   			// Stop
   			stopErr := stopMariaDBService()
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
   				currentStatus = getMariaDBStatus()
   				
   				// Update UI on main thread
   				fyne.Do(func() {
   					if startErr != nil {
   						dialog.ShowError(startErr, mainWindow)
   					} else {
   						dialog.ShowInformation("Success", "MariaDB restarted successfully", mainWindow)
   					}
   				})
   			}
   		}()
   	}
   })

   configManagerBtn := widget.NewButton("Configuration Manager", func() {
   	showConfigurationManager()
   })

   openFolderBtn := widget.NewButton("Open Config Folder", func() {
   	openFolder(config.ConfigPath)
   })

   return widget.NewCard("Quick Actions", "", container.NewVBox(
   	container.NewBorder(nil, nil, widget.NewLabel("Start with:"), startBtn, configSelect),
   	container.NewGridWithColumns(3, stopBtn, restartBtn, configManagerBtn),
   	openFolderBtn,
   ))
}

func createConfigCard() fyne.CanvasObject {
   // Refresh configs
   scanForConfigs()

   // Create config list table
   configTable := widget.NewTable(
   	func() (int, int) {
   		return len(availableConfigs) + 1, 5 // +1 for header
   	},
   	func() fyne.CanvasObject {
   		return widget.NewLabel("cell")
   	},
   	func(i widget.TableCellID, o fyne.CanvasObject) {
   		label := o.(*widget.Label)
   		if i.Row == 0 {
   			// Header row
   			headers := []string{"Name", "Port", "Data Directory", "Description", "Status"}
   			label.SetText(headers[i.Col])
   			label.TextStyle = fyne.TextStyle{Bold: true}
   		} else {
   			// Data rows
   			cfg := availableConfigs[i.Row-1]
   			switch i.Col {
   			case 0:
   				label.SetText(cfg.Name)
   			case 1:
   				label.SetText(cfg.Port)
   			case 2:
   				label.SetText(cfg.DataDir)
   			case 3:
   				label.SetText(cfg.Description)
   			case 4:
   				if cfg.IsActive && currentStatus.IsRunning {
   					label.SetText("● Active")
   					label.TextStyle = fyne.TextStyle{Bold: true}
   				} else {
   					label.SetText("Ready")
   					label.TextStyle = fyne.TextStyle{}
   				}
   			}
   		}
   	},
   )

   // Set column widths
   configTable.SetColumnWidth(0, 150)
   configTable.SetColumnWidth(1, 80)
   configTable.SetColumnWidth(2, 250)
   configTable.SetColumnWidth(3, 200)
   configTable.SetColumnWidth(4, 80)

   // Track selected row
   var selectedRow int = -1
   configTable.OnSelected = func(cell widget.TableCellID) {
   	if cell.Row > 0 { // Skip header row
   		selectedRow = cell.Row - 1
   	}
   }

   // Action buttons
   startBtn := widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), func() {
   	if selectedRow >= 0 && selectedRow < len(availableConfigs) {
   		cfg := availableConfigs[selectedRow]
   		go func() {
   			err := startMariaDBWithConfig(cfg.Path)
   			scanForConfigs()
   			currentStatus = getMariaDBStatus()
   			
   			// Update UI on main thread
   			fyne.Do(func() {
   				if err != nil {
   					dialog.ShowError(err, mainWindow)
   				} else {
   					configTable.Refresh()
   				}
   			})
   		}()
   	}
   })

   editBtn := widget.NewButtonWithIcon("Edit", theme.DocumentIcon(), func() {
   	if selectedRow >= 0 && selectedRow < len(availableConfigs) {
   		cfg := availableConfigs[selectedRow]
   		openFileInEditor(cfg.Path)
   	}
   })

   deleteBtn := widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), func() {
   	if selectedRow >= 0 && selectedRow < len(availableConfigs) {
   		cfg := availableConfigs[selectedRow]
   		dialog.ShowConfirm("Delete Configuration",
   			fmt.Sprintf("Are you sure you want to delete %s?", cfg.Name),
   			func(confirm bool) {
   				if confirm {
   					os.Remove(cfg.Path)
   					scanForConfigs()
   					configTable.Refresh()
   				}
   			}, mainWindow)
   	}
   })

   refreshBtn := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
   	scanForConfigs()
   	currentStatus = getMariaDBStatus()
   	configTable.Refresh()
   })

   toolbar := container.NewHBox(
   	startBtn,
   	editBtn,
   	deleteBtn,
   	layout.NewSpacer(),
   	refreshBtn,
   )

   infoLabel := widget.NewLabel(fmt.Sprintf("Configuration files are stored in: %s", config.ConfigPath))

   return container.NewBorder(
   	toolbar,
   	infoLabel,
   	nil, nil,
   	configTable,
   )
}