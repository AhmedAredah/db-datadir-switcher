package core

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// StopMySQLWithCredentials gracefully stops MySQL using admin credentials
func StopMySQLWithCredentials(creds MySQLCredentials) error {
	mysqladminPath := filepath.Join(AppConfig.MariaDBBin, "mysqladmin")
	if runtime.GOOS == "windows" {
		mysqladminPath += ".exe"
	}
	
	if !PathExists(mysqladminPath) {
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
	
	// Log the command for debugging
	AppLogger.Log("Executing graceful shutdown with mysqladmin...")
	AppLogger.Log("Command: %s %s", mysqladminPath, strings.Join(args, " "))
	
	cmd := exec.Command(mysqladminPath, args...)
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		AppLogger.Log("mysqladmin shutdown error: %v\nOutput: %s", err, string(output))
		return fmt.Errorf("shutdown failed: %v", err)
	}
	
	// Wait for shutdown to complete
	time.Sleep(3 * time.Second)
	
	AppLogger.Log("MySQL shutdown command executed successfully")
	return nil
}

// ValidateConfigFile validates a MariaDB configuration file
func ValidateConfigFile(mysqldPath, configFile string) error {
	cmd := exec.Command(mysqldPath, "--defaults-file="+configFile, "--validate-config")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("config validation failed: %s", string(output))
	}
	return nil
}

// ValidateDataDirectory checks if a data directory has required files
func ValidateDataDirectory(dataDir string) bool {
	// Check for essential files/directories
	essentialPaths := []string{
		filepath.Join(dataDir, "mysql"),
		filepath.Join(dataDir, "performance_schema"),
		filepath.Join(dataDir, "ibdata1"),
	}
	
	for _, path := range essentialPaths {
		if !PathExists(path) {
			AppLogger.Log("Missing essential file/directory: %s", path)
			return false
		}
	}
	
	return true
}

// InitializeDataDir initializes a new MariaDB data directory
func InitializeDataDir(dataDir string) error {
	// Try mysql_install_db first
	installDbPath := filepath.Join(AppConfig.MariaDBBin, "mysql_install_db")
	if runtime.GOOS == "windows" {
		installDbPath += ".exe"
	}
	
	if PathExists(installDbPath) {
		cmd := exec.Command(installDbPath, "--datadir="+dataDir, "--auth-root-authentication-method=normal")
		output, err := cmd.CombinedOutput()
		if err != nil {
			AppLogger.Log("mysql_install_db failed: %v\nOutput: %s", err, string(output))
			return err
		}
		AppLogger.Log("Data directory initialized with mysql_install_db")
		return nil
	}
	
	// Try mysqld --initialize-insecure
	mysqldPath := filepath.Join(AppConfig.MariaDBBin, "mysqld")
	if runtime.GOOS == "windows" {
		mysqldPath += ".exe"
	}
	
	cmd := exec.Command(mysqldPath, "--initialize-insecure", "--datadir="+dataDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		AppLogger.Log("mysqld --initialize-insecure failed: %v\nOutput: %s", err, string(output))
		return err
	}
	
	AppLogger.Log("Data directory initialized with mysqld --initialize-insecure")
	return nil
}

// InitializeDataDirAlternative tries alternative methods to initialize data directory
func InitializeDataDirAlternative(dataDir, configFile string) error {
	mysqldPath := filepath.Join(AppConfig.MariaDBBin, "mysqld")
	if runtime.GOOS == "windows" {
		mysqldPath += ".exe"
	}
	
	// Try with config file
	cmd := exec.Command(mysqldPath, "--defaults-file="+configFile, "--initialize-insecure")
	output, err := cmd.CombinedOutput()
	if err != nil {
		AppLogger.Log("Alternative initialization failed: %v\nOutput: %s", err, string(output))
		return fmt.Errorf("failed to initialize data directory: %v", err)
	}
	
	AppLogger.Log("Data directory initialized with alternative method")
	return nil
}

// ParseMariaDBError parses MariaDB error output for common issues
func ParseMariaDBError(errorOutput string) string {
	lowerOutput := strings.ToLower(errorOutput)
	
	if strings.Contains(lowerOutput, "access denied") {
		return "Access denied - check your credentials"
	}
	if strings.Contains(lowerOutput, "port") && strings.Contains(lowerOutput, "already in use") {
		return "Port already in use - another instance might be running"
	}
	if strings.Contains(lowerOutput, "permission denied") {
		return "Permission denied - may need administrator privileges"
	}
	if strings.Contains(lowerOutput, "data directory") && strings.Contains(lowerOutput, "not empty") {
		return "Data directory is not empty - initialization might have failed"
	}
	if strings.Contains(lowerOutput, "can't create/write to file") {
		return "Cannot write to data directory - check permissions"
	}
	if strings.Contains(lowerOutput, "unknown variable") {
		return "Configuration file contains unknown variables"
	}
	if strings.Contains(lowerOutput, "plugin") && strings.Contains(lowerOutput, "not loaded") {
		return "Required plugin not loaded - check configuration"
	}
	
	// Return first non-empty line as fallback
	lines := strings.Split(errorOutput, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "[") {
			return line
		}
	}
	
	return "Unknown error - check logs for details"
}

// ExecMySQLQueryWithCredentials executes a MySQL query with provided credentials
func ExecMySQLQueryWithCredentials(variable string, creds MySQLCredentials) string {
	mysqlPath := filepath.Join(AppConfig.MariaDBBin, "mysql")
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
		AppLogger.Log("MySQL query failed for variable %s: %v", variable, err)
		return ""
	}
	
	result := strings.TrimSpace(string(output))
	AppLogger.Log("MySQL query for %s returned: %s", variable, result)
	return result
}

// FindProcessUsingPort finds which process is using a specific port
func FindProcessUsingPort(port string) {
	var cmd *exec.Cmd
	
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("netstat", "-ano", "-p", "TCP")
	case "darwin":
		cmd = exec.Command("lsof", "-i", fmt.Sprintf(":%s", port))
	default:
		cmd = exec.Command("netstat", "-tlnp")
	}
	
	output, err := cmd.Output()
	if err != nil {
		AppLogger.Log("Failed to run port check command: %v", err)
		return
	}
	
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, ":"+port) {
			AppLogger.Log("Port %s usage: %s", port, line)
		}
	}
}

// StopLinuxService stops the MariaDB service on Linux
func StopLinuxService() error {
	if AppConfig.RequireElevation {
		cmd := exec.Command("sudo", "systemctl", "stop", AppConfig.ServiceNames["linux"])
		return cmd.Run()
	}
	cmd := exec.Command("systemctl", "stop", AppConfig.ServiceNames["linux"])
	return cmd.Run()
}

// StopMacService stops the MariaDB service on macOS
func StopMacService() error {
	// Try launchctl first
	cmd := exec.Command("launchctl", "unload", "-w", 
		fmt.Sprintf("/Library/LaunchDaemons/com.mariadb.server.plist"))
	if err := cmd.Run(); err == nil {
		return nil
	}
	
	// Try brew services
	cmd = exec.Command("brew", "services", "stop", "mariadb")
	return cmd.Run()
}

// ValidateCredentials validates MySQL credentials
func ValidateCredentials(creds MySQLCredentials) error {
	if creds.Username == "" {
		return fmt.Errorf("username is required")
	}
	if creds.Host == "" {
		return fmt.Errorf("host is required")
	}
	if creds.Port == "" {
		return fmt.Errorf("port is required")
	}
	return nil
}