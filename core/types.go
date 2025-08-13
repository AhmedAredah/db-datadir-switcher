package core

// Config represents the application configuration
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
	ConfigFile  string `json:"config_file"`  // Current config file path
	ConfigName  string `json:"config_name"`  // Friendly name of config
	DataPath    string `json:"data_path"`
	ProcessID   int    `json:"process_id"`
	Port        string `json:"port"`
	ServiceName string `json:"service_name,omitempty"`
	Version     string `json:"version,omitempty"`
}

// MySQLCredentials represents database connection credentials
type MySQLCredentials struct {
	Username string
	Password string
	Host     string
	Port     string
}