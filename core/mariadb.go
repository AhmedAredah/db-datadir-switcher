package core

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// GetMariaDBStatus returns the current MariaDB status
func GetMariaDBStatus() MariaDBStatus {
	status := MariaDBStatus{
		IsRunning: false,
	}

	// Check if MariaDB is running
	status.IsRunning = IsMariaDBRunning()
	
	if !status.IsRunning {
		return status
	}

	// Get process details
	processName := AppConfig.ProcessNames[runtime.GOOS]
	if processName == "" {
		processName = "mysqld"
	}

	pid, cmdLine, found := FindProcessWithCmdLine(processName)
	if found {
		status.ProcessID = pid
		
		// Log the command line for debugging
		AppLogger.Log("DEBUG: Found MariaDB process with command line: %s", cmdLine)
		
		// Extract config file from command line
		configFile := extractConfigFromCmdLine(cmdLine)
		AppLogger.Log("DEBUG: Extracted config file: '%s'", configFile)
		
		if configFile != "" {
			status.ConfigFile = configFile
			
			// Normalize the config file path for comparison
			normalizedConfigFile := filepath.Clean(configFile)
			
			// Find matching config in our list
			for _, cfg := range AvailableConfigs {
				normalizedCfgPath := filepath.Clean(cfg.Path)
				AppLogger.Log("DEBUG: Comparing '%s' with '%s'", normalizedConfigFile, normalizedCfgPath)
				
				if normalizedCfgPath == normalizedConfigFile {
					status.ConfigName = cfg.Name
					status.Port = cfg.Port
					status.DataPath = cfg.DataDir
					AppLogger.Log("DEBUG: Matched config: %s, Port: %s", cfg.Name, cfg.Port)
					break
				}
			}
			
			if status.ConfigName == "" {
				AppLogger.Log("DEBUG: No matching config found for file: %s", configFile)
				AppLogger.Log("DEBUG: Available configs:")
				for _, cfg := range AvailableConfigs {
					AppLogger.Log("DEBUG:   - %s: %s", cfg.Name, filepath.Clean(cfg.Path))
				}
			}
		} else {
			AppLogger.Log("DEBUG: No config file found in command line")
			// If no config file, try to get port from running instance
			status.Port = getCurrentPort()
		}
	}

	// Try to get version
	status.Version = GetMariaDBVersion()

	return status
}

// IsMariaDBRunning checks if MariaDB/MySQL is running
func IsMariaDBRunning() bool {
	processName := AppConfig.ProcessNames[runtime.GOOS]
	if processName == "" {
		processName = "mysqld"
	}
	
	_, _, found := FindProcessWithCmdLine(processName)
	return found
}

// FindProcessWithCmdLine finds a process by name and returns its PID and command line
func FindProcessWithCmdLine(processName string) (int, string, bool) {
	switch runtime.GOOS {
	case "windows":
		return findWindowsProcessWithCmdLine(processName)
	default:
		return findUnixProcessWithCmdLine(processName)
	}
}

func findWindowsProcessWithCmdLine(processName string) (int, string, bool) {
	// Try WMI query for command line
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		fmt.Sprintf(`Get-WmiObject Win32_Process -Filter "Name='%s'" | Select-Object ProcessId,CommandLine | ConvertTo-Json`, processName))
	
	output, err := cmd.Output()
	if err != nil {
		return 0, "", false
	}

	outputStr := string(output)
	if strings.Contains(outputStr, "ProcessId") {
		// Parse the JSON-like output to extract PID and CommandLine
		lines := strings.Split(outputStr, "\n")
		for _, line := range lines {
			if strings.Contains(line, "ProcessId") {
				// Extract PID
				pidStart := strings.Index(line, ":") + 1
				pidEnd := strings.IndexAny(line[pidStart:], ",}")
				if pidEnd == -1 {
					pidEnd = len(line) - pidStart
				}
				pidStr := strings.TrimSpace(line[pidStart:pidStart+pidEnd])
				pid, _ := strconv.Atoi(pidStr)
				
				// Get command line from next lines
				cmdLine := ""
				for _, cmdLineLine := range lines {
					if strings.Contains(cmdLineLine, "CommandLine") {
						cmdStart := strings.Index(cmdLineLine, ":") + 1
						cmdLine = strings.Trim(cmdLineLine[cmdStart:], " \t\r\n\"")
						break
					}
				}
				
				if pid > 0 {
					return pid, cmdLine, true
				}
			}
		}
	}

	return 0, "", false
}

func findUnixProcessWithCmdLine(processName string) (int, string, bool) {
	// Use ps command to find the process
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
				pid, _ := strconv.Atoi(fields[1])
				cmdLine := strings.Join(fields[10:], " ")
				return pid, cmdLine, true
			}
		}
	}

	return 0, "", false
}

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

// getCurrentPort attempts to determine the port MariaDB is running on
func getCurrentPort() string {
	// Method 1: Try to extract port from process command line arguments
	if port := extractPortFromCmdLine(); port != "" {
		AppLogger.Log("DEBUG: Found port %s from command line arguments", port)
		return port
	}

	// Method 2: Try to query the database directly
	if port := queryDatabasePort(); port != "" {
		AppLogger.Log("DEBUG: Found port %s from database query", port)
		return port
	}

	// Method 3: Check netstat output for MariaDB/MySQL processes
	if port := getPortFromNetstat(); port != "" {
		AppLogger.Log("DEBUG: Found port %s from netstat", port)
		return port
	}

	// Method 4: Check common ports in order of likelihood
	commonPorts := []string{"3306", "3307", "3308", "3309", "3310"}
	
	for _, port := range commonPorts {
		if IsPortListening(port) {
			AppLogger.Log("DEBUG: Found service listening on port %s", port)
			return port
		}
	}
	
	AppLogger.Log("DEBUG: Could not determine port, defaulting to 3306")
	return "3306" // Default fallback
}

// extractPortFromCmdLine attempts to extract port from command line arguments
func extractPortFromCmdLine() string {
	processName := AppConfig.ProcessNames[runtime.GOOS]
	if processName == "" {
		processName = "mysqld"
	}

	_, cmdLine, found := FindProcessWithCmdLine(processName)
	if !found {
		return ""
	}

	// Look for --port= parameter
	if idx := strings.Index(cmdLine, "--port="); idx != -1 {
		start := idx + len("--port=")
		end := strings.IndexAny(cmdLine[start:], " \t\n")
		if end == -1 {
			return strings.Trim(cmdLine[start:], "\"'")
		}
		return strings.Trim(cmdLine[start:start+end], "\"'")
	}

	// Look for --port parameter with space
	parts := strings.Fields(cmdLine)
	for i, part := range parts {
		if part == "--port" && i+1 < len(parts) {
			return strings.Trim(parts[i+1], "\"'")
		}
	}

	return ""
}

// queryDatabasePort attempts to query the database for its port
func queryDatabasePort() string {
	// Try to connect with default credentials and query the port
	creds := GetDefaultCredentials()
	
	mysqlPath := filepath.Join(AppConfig.MariaDBBin, "mysql")
	if runtime.GOOS == "windows" {
		mysqlPath += ".exe"
	}

	// Build command to query the port
	args := []string{
		"-h", creds.Host,
		"-u", creds.Username,
		"-e", "SHOW VARIABLES LIKE 'port';",
		"--silent",
		"--skip-column-names",
	}

	// Add password if provided
	if creds.Password != "" {
		args = append([]string{fmt.Sprintf("-p%s", creds.Password)}, args[3:]...)
		args = append([]string{args[0], args[1], args[2]}, args[3:]...)
	}

	cmd := exec.Command(mysqlPath, args...)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Parse output: should be "port\t3306"
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) >= 2 && parts[0] == "port" {
			return strings.TrimSpace(parts[1])
		}
	}

	return ""
}

// getPortFromNetstat attempts to find MariaDB port from netstat output
func getPortFromNetstat() string {
	var cmd *exec.Cmd
	
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("netstat", "-ano")
	default:
		cmd = exec.Command("netstat", "-tlnp")
	}

	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	processName := AppConfig.ProcessNames[runtime.GOOS]
	if processName == "" {
		processName = "mysqld"
	}

	// Get the PID of MariaDB process
	pid, _, found := FindProcessWithCmdLine(processName)
	if !found {
		return ""
	}
	pidStr := strconv.Itoa(pid)

	// Parse netstat output to find ports used by this PID
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "LISTENING") || strings.Contains(line, "LISTEN") {
			fields := strings.Fields(line)
			
			// Windows format: TCP 0.0.0.0:3306 0.0.0.0:0 LISTENING 1234
			// Unix format: tcp 0 0 0.0.0.0:3306 0.0.0.0:* LISTEN 1234/mysqld
			
			var localAddr, processInfo string
			if runtime.GOOS == "windows" {
				if len(fields) >= 5 {
					localAddr = fields[1]
					processInfo = fields[4]
				}
			} else {
				if len(fields) >= 4 {
					localAddr = fields[3]
					if len(fields) >= 7 {
						processInfo = fields[6]
					}
				}
			}

			// Check if this line matches our PID
			if strings.Contains(processInfo, pidStr) {
				// Extract port from address (format: ip:port)
				if colonIdx := strings.LastIndex(localAddr, ":"); colonIdx != -1 {
					port := localAddr[colonIdx+1:]
					if port != "0" && len(port) > 0 {
						return port
					}
				}
			}
		}
	}

	return ""
}

// GetMariaDBVersion returns the MariaDB version
func GetMariaDBVersion() string {
	mysqldPath := filepath.Join(AppConfig.MariaDBBin, "mysqld")
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

// GetDefaultDataDir returns the default data directory for MariaDB
func GetDefaultDataDir() string {
	switch runtime.GOOS {
	case "windows":
		if AppConfig.MariaDBBin != "" {
			return filepath.Join(filepath.Dir(AppConfig.MariaDBBin), "data")
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

// StartMariaDBWithConfig starts MariaDB with the specified configuration file
func StartMariaDBWithConfig(configFile string) error {
	AppLogger.Log("========================================")
	AppLogger.Log("STARTING MARIADB")
	AppLogger.Log("========================================")
	
	// Check if MariaDB is already running
	if IsMariaDBRunning() {
		AppLogger.Log("MariaDB is already running")
		return fmt.Errorf("MariaDB is already running - please stop it first")
	}

	// Validate config.MariaDBBin
	if AppConfig.MariaDBBin == "" {
		AppLogger.Log("ERROR: MariaDB binary path is empty!")
		return fmt.Errorf("MariaDB binary path not configured")
	}

	// Check if binary directory exists
	if !PathExists(AppConfig.MariaDBBin) {
		AppLogger.Log("ERROR: MariaDB binary directory does not exist: %s", AppConfig.MariaDBBin)
		return fmt.Errorf("MariaDB binary directory not found: %s", AppConfig.MariaDBBin)
	}

	// Build full mysqld path
	mysqldPath := filepath.Join(AppConfig.MariaDBBin, GetExecutableName("mysqld"))
	AppLogger.Log("Full mysqld path: %s", mysqldPath)
	
	// Check if mysqld exists
	if !PathExists(mysqldPath) {
		AppLogger.Log("ERROR: mysqld not found at: %s", mysqldPath)
		
		// Try mariadbd as alternative
		mariadbdPath := filepath.Join(AppConfig.MariaDBBin, GetExecutableName("mariadbd"))
		if PathExists(mariadbdPath) {
			AppLogger.Log("Found mariadbd instead of mysqld at: %s", mariadbdPath)
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
				AppLogger.Log("Found mysqld at: %s", foundPath)
				mysqldPath = foundPath
			} else {
				AppLogger.Log("Could not find mysqld in system PATH")
				return fmt.Errorf("mysqld not found at: %s", mysqldPath)
			}
		}
	}

	// Check if config file exists
	if !PathExists(configFile) {
		AppLogger.Log("ERROR: Configuration file not found: %s", configFile)
		return fmt.Errorf("configuration file not found: %s", configFile)
	}

	// Get absolute path for config file
	absConfigFile, err := filepath.Abs(configFile)
	if err != nil {
		AppLogger.Log("ERROR: Cannot get absolute path for config: %v", err)
		return fmt.Errorf("cannot get absolute path for config: %v", err)
	}
	AppLogger.Log("Absolute config file path: %s", absConfigFile)

	// Parse config first to validate it
	AppLogger.Log("Parsing configuration file...")
	configData := ParseConfigFile(configFile)
	AppLogger.Log("Config parsed - DataDir: %s, Port: %s", configData.DataDir, configData.Port)
	
	// Validate and prepare data directory
	if configData.DataDir != "" {
		// Convert to absolute path if relative
		if !filepath.IsAbs(configData.DataDir) {
			configData.DataDir = filepath.Join(filepath.Dir(absConfigFile), configData.DataDir)
			AppLogger.Log("Converted relative datadir to absolute: %s", configData.DataDir)
		}

		if !PathExists(configData.DataDir) {
			AppLogger.Log("Data directory does not exist, creating: %s", configData.DataDir)
			if err := os.MkdirAll(configData.DataDir, 0755); err != nil {
				AppLogger.Log("ERROR: Failed to create data directory: %v", err)
				return fmt.Errorf("failed to create data directory: %v", err)
			}
		}

		// Check if data directory is empty and needs initialization
		if isEmpty, _ := IsDirEmpty(configData.DataDir); isEmpty {
			AppLogger.Log("Data directory is empty, needs initialization")
			if err := InitializeDataDir(configData.DataDir); err != nil {
				AppLogger.Log("ERROR: Failed to initialize data directory: %v", err)
				// Try alternative initialization
				if err := InitializeDataDirAlternative(configData.DataDir, absConfigFile); err != nil {
					return fmt.Errorf("failed to initialize data directory: %v", err)
				}
			}
		} else {
			// Check for critical files in data directory
			if !ValidateDataDirectory(configData.DataDir) {
				AppLogger.Log("WARNING: Data directory may be corrupted or incomplete")
			}
		}
	}

	// Check if MySQL/MariaDB is still running - no force stop
	AppLogger.Log("Checking if all MySQL/MariaDB processes are stopped...")
	if IsMariaDBRunning() {
		return fmt.Errorf("MySQL/MariaDB is still running - please stop it gracefully with credentials before starting a new instance")
	}
	
	// Double-check the port is free
	if !IsPortAvailable(configData.Port) {
		AppLogger.Log("Port %s is still in use", configData.Port)
		return fmt.Errorf("cannot start - port %s is occupied by another process", configData.Port)
	}
	
	AppLogger.Log("Port %s is confirmed available", configData.Port)

	// First, try to validate the config file syntax
	AppLogger.Log("Validating configuration file syntax...")
	if err := ValidateConfigFile(mysqldPath, absConfigFile); err != nil {
		AppLogger.Log("WARNING: Config file validation failed: %v", err)
		// Continue anyway, as some versions don't support --validate-config
	}

	// Start the MariaDB process with better error capture
	AppLogger.Log("Starting MariaDB with configuration...")
	
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
	cmd.Dir = AppConfig.MariaDBBin
	
	// Platform-specific configuration
	if runtime.GOOS == "windows" {
		// Use CREATE_NEW_PROCESS_GROUP to detach process from parent
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true, // Hide console window
			CreationFlags: 0x00000200, // CREATE_NEW_PROCESS_GROUP - allows process to survive parent termination
		}
	}
	
	AppLogger.Log("Executing command: %s %s", mysqldPath, strings.Join(args, " "))
	
	// Start the process
	err = cmd.Start()
	if err != nil {
		AppLogger.Log("ERROR: Failed to start process: %v", err)
		return fmt.Errorf("failed to start MariaDB: %v", err)
	}
	
	AppLogger.Log("Process started with PID: %d", cmd.Process.Pid)
	
	// Release the process so it's detached from parent and can survive parent termination
	err = cmd.Process.Release()
	if err != nil {
		AppLogger.Log("WARNING: Could not release process: %v", err)
		// Continue anyway - the process group flag should still work
	} else {
		AppLogger.Log("Process successfully detached from parent")
	}
	
	// Brief wait to allow process to initialize before verification
	AppLogger.Log("Waiting 3 seconds for MariaDB to initialize...")
	time.Sleep(3 * time.Second)
	
	// Additional verification - try to connect
	AppLogger.Log("Verifying MariaDB is accessible...")
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		if IsMariaDBRunning() && IsPortListening(configData.Port) {
			AppLogger.Log("MariaDB is running and accepting connections")
			break
		}
		AppLogger.Log("Waiting for MariaDB to be ready... (%d/%d)", i+1, maxRetries)
		time.Sleep(1 * time.Second)
	}
	
	// Final verification
	if !IsMariaDBRunning() {
		stdoutStr := stdout.String()
		stderrStr := stderr.String()
		AppLogger.Log("ERROR: MariaDB process not found after startup")
		if stdoutStr != "" {
			AppLogger.Log("Final stdout: %s", stdoutStr)
		}
		if stderrStr != "" {
			AppLogger.Log("Final stderr: %s", stderrStr)
		}
		return fmt.Errorf("MariaDB failed to start - process not found. Check logs for details")
	}

	// Save the last used config
	AppConfig.LastUsedConfig = absConfigFile
	SaveConfig()
	
	// Update global status
	CurrentStatus = GetMariaDBStatus()
	
	AppLogger.Log("========================================")
	AppLogger.Log("MARIADB STARTED SUCCESSFULLY")
	AppLogger.Log("========================================")
	
	return nil
}