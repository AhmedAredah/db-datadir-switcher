package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"mariadb-monitor/core"
	"golang.org/x/term"
)

// CLI represents the command-line interface
type CLI struct{}

// NewCLI creates a new CLI instance
func NewCLI() *CLI {
	return &CLI{}
}

// List displays all available configurations
func (c *CLI) List() error {
	fmt.Println("Available MariaDB Configurations:")
	fmt.Println("=================================")
	
	if len(core.AvailableConfigs) == 0 {
		fmt.Println("No configurations found.")
		fmt.Printf("Configuration directory: %s\n", core.AppConfig.ConfigPath)
		fmt.Println("Add .ini or .cnf files to this directory to create configurations.")
		return nil
	}
	
	// Get current status to mark active config
	status := core.GetMariaDBStatus()
	
	for i, config := range core.AvailableConfigs {
		fmt.Printf("%d. %s", i+1, config.Name)
		
		if config.Description != "" {
			fmt.Printf(" (%s)", config.Description)
		}
		
		fmt.Printf("\n   Port: %s", config.Port)
		
		if config.DataDir != "" {
			fmt.Printf("\n   Data: %s", config.DataDir)
		}
		
		fmt.Printf("\n   File: %s", config.Path)
		
		// Mark active configuration (normalize paths for comparison)
		if status.IsRunning && filepath.Clean(config.Path) == filepath.Clean(status.ConfigFile) {
			fmt.Printf("\n   Status: ✓ ACTIVE (PID: %d)", status.ProcessID)
		} else {
			fmt.Printf("\n   Status: Available")
		}
		
		fmt.Println()
	}
	
	return nil
}

// Status shows the current MariaDB status
func (c *CLI) Status() error {
	fmt.Println("MariaDB Status:")
	fmt.Println("===============")
	
	status := core.GetMariaDBStatus()
	
	if status.IsRunning {
		fmt.Printf("Status: ✓ RUNNING\n")
		fmt.Printf("Process ID: %d\n", status.ProcessID)
		fmt.Printf("Configuration: %s\n", status.ConfigName)
		fmt.Printf("Port: %s\n", status.Port)
		if status.DataPath != "" {
			fmt.Printf("Data Directory: %s\n", status.DataPath)
		}
		if status.Version != "" {
			fmt.Printf("Version: %s\n", status.Version)
		}
	} else {
		fmt.Printf("Status: ✗ STOPPED\n")
	}
	
	return nil
}

// Switch switches to a different configuration
func (c *CLI) Switch(configName string) error {
	fmt.Printf("Switching to configuration: %s\n", configName)
	
	// Find the configuration
	var targetConfig *core.MariaDBConfig
	for _, config := range core.AvailableConfigs {
		if strings.EqualFold(config.Name, configName) {
			targetConfig = &config
			break
		}
	}
	
	if targetConfig == nil {
		return fmt.Errorf("configuration '%s' not found", configName)
	}
	
	// Check if MariaDB is currently running
	if core.IsMariaDBRunning() {
		fmt.Println("MariaDB is currently running. Stopping it first...")
		
		if err := c.Stop(); err != nil {
			return fmt.Errorf("failed to stop current MariaDB instance: %v", err)
		}
		
		fmt.Println("Waiting for shutdown to complete...")
		// Brief wait to ensure complete shutdown
		core.AppLogger.Log("Waiting for complete shutdown before switching...")
		// Add a simple wait here
	}
	
	// Start with new configuration
	fmt.Printf("Starting MariaDB with %s configuration...\n", targetConfig.Name)
	
	err := core.StartMariaDBWithConfig(targetConfig.Path)
	if err != nil {
		return fmt.Errorf("failed to start MariaDB: %v", err)
	}
	
	fmt.Printf("✓ Successfully switched to %s configuration\n", targetConfig.Name)
	fmt.Printf("  Port: %s\n", targetConfig.Port)
	if targetConfig.DataDir != "" {
		fmt.Printf("  Data Directory: %s\n", targetConfig.DataDir)
	}
	
	return nil
}

// Start starts MariaDB with a specific configuration
func (c *CLI) Start(configName string) error {
	if configName == "" {
		return fmt.Errorf("configuration name is required")
	}
	
	// Find the configuration
	var targetConfig *core.MariaDBConfig
	for _, config := range core.AvailableConfigs {
		if strings.EqualFold(config.Name, configName) {
			targetConfig = &config
			break
		}
	}
	
	if targetConfig == nil {
		return fmt.Errorf("configuration '%s' not found", configName)
	}
	
	// Check if already running
	if core.IsMariaDBRunning() {
		status := core.GetMariaDBStatus()
		return fmt.Errorf("MariaDB is already running with configuration '%s'", status.ConfigName)
	}
	
	fmt.Printf("Starting MariaDB with %s configuration...\n", targetConfig.Name)
	
	err := core.StartMariaDBWithConfig(targetConfig.Path)
	if err != nil {
		return fmt.Errorf("failed to start MariaDB: %v", err)
	}
	
	fmt.Printf("✓ MariaDB started successfully\n")
	fmt.Printf("  Configuration: %s\n", targetConfig.Name)
	fmt.Printf("  Port: %s\n", targetConfig.Port)
	
	return nil
}

// Stop stops the running MariaDB instance
func (c *CLI) Stop() error {
	if !core.IsMariaDBRunning() {
		fmt.Println("MariaDB is not currently running.")
		return nil
	}
	
	fmt.Println("Stopping MariaDB...")
	
	// Try to get credentials for graceful shutdown
	creds, err := c.promptForCredentials()
	if err != nil {
		return fmt.Errorf("failed to get credentials: %v", err)
	}
	
	// Attempt graceful shutdown
	err = core.StopMySQLWithCredentials(creds)
	if err != nil {
		return fmt.Errorf("failed to stop MariaDB gracefully: %v", err)
	}
	
	fmt.Println("✓ MariaDB stopped successfully")
	return nil
}

// promptForCredentials prompts the user for MySQL credentials
func (c *CLI) promptForCredentials() (core.MySQLCredentials, error) {
	reader := bufio.NewReader(os.Stdin)
	
	// Try to use saved credentials first
	if core.SavedCredentials != nil {
		fmt.Printf("Use saved credentials (user: %s, host: %s)? [Y/n]: ", 
			core.SavedCredentials.Username, core.SavedCredentials.Host)
		
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(response)
		
		if response == "" || strings.ToLower(response) == "y" || strings.ToLower(response) == "yes" {
			return *core.SavedCredentials, nil
		}
	}
	
	// Prompt for new credentials
	creds := core.MySQLCredentials{}
	
	fmt.Print("MySQL Username [root]: ")
	username, _ := reader.ReadString('\n')
	username = strings.TrimSpace(username)
	if username == "" {
		username = "root"
	}
	creds.Username = username
	
	fmt.Print("MySQL Host [localhost]: ")
	host, _ := reader.ReadString('\n')
	host = strings.TrimSpace(host)
	if host == "" {
		host = "localhost"
	}
	creds.Host = host
	
	fmt.Print("MySQL Port [3306]: ")
	port, _ := reader.ReadString('\n')
	port = strings.TrimSpace(port)
	if port == "" {
		port = "3306"
	}
	creds.Port = port
	
	fmt.Print("MySQL Password (leave empty if none): ")
	
	// Hide password input
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return creds, fmt.Errorf("failed to read password: %v", err)
	}
	fmt.Println() // New line after password input
	
	creds.Password = string(passwordBytes)
	
	// Ask if user wants to save credentials
	fmt.Print("Save credentials for future use? [y/N]: ")
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(response)
	
	if strings.ToLower(response) == "y" || strings.ToLower(response) == "yes" {
		if err := core.SaveCredentialsToKeyring(creds); err != nil {
			fmt.Printf("Warning: Failed to save credentials: %v\n", err)
		} else {
			core.SavedCredentials = &creds
			fmt.Println("Credentials saved securely.")
		}
	}
	
	return creds, nil
}

// ShowHelp displays CLI help information
func (c *CLI) ShowHelp() {
	fmt.Println(`DBSwitcher CLI - MariaDB Configuration Manager

USAGE:
    dbswitcher <command> [arguments]

COMMANDS:
    list                    List all available configurations
    status                  Show current MariaDB status
    start <config>          Start MariaDB with specified configuration
    switch <config>         Switch to a different configuration (stops current, starts new)
    stop                    Stop the running MariaDB instance
    gui                     Launch the GUI interface
    tray                    Run in system tray mode
    help                    Show this help message

EXAMPLES:
    dbswitcher list                    # List all configurations
    dbswitcher status                  # Show current status
    dbswitcher start production        # Start with production config
    dbswitcher switch development      # Switch to development config
    dbswitcher stop                    # Stop MariaDB
    dbswitcher gui                     # Launch GUI

CONFIGURATION:
    Configuration files (.ini or .cnf) should be placed in:
    Windows: %APPDATA%\DBSwitcher\configs
    Linux/macOS: ~/.config/DBSwitcher

    Each configuration file should contain a [mysqld] section with
    settings like datadir, port, and an optional description.`)
}