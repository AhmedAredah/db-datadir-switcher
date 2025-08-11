# DB DataDir Switcher

A lightweight system tray utility for seamlessly switching database data directories between internal and external storage locations.

## Overview

DB DataDir Switcher enables database administrators and developers to easily switch between multiple data storage locations without manual configuration changes or data migration. Perfect for scenarios requiring:

- Development/production data separation
- External drive utilization for large datasets
- Quick switching between different database instances
- Backup and recovery workflows

## Features

- **System Tray Integration**: Runs quietly in the background with easy access via system tray
- **Hot-Swapping**: Switch data directories without data loss or corruption
- **Real-time Monitoring**: Live status updates showing current configuration and database state
- **Multiple Storage Support**: Configure internal and external storage locations
- **Process Management**: Built-in start/stop controls for database services
- **Configuration Detection**: Automatically detects current running configuration

## Requirements

### Current (Windows)
- Windows 10/11
- PowerShell 5.1 or higher
- MariaDB 10.x/11.x or MySQL 5.7/8.0
- Administrator privileges (for service management)

### Planned Support
- macOS 11+ (Big Sur and later)
- Linux (Ubuntu 20.04+, RHEL 8+, Debian 11+)

## Installation

### Windows

1. Clone the repository:
```bash
git clone https://github.com/yourusername/db-datadir-switcher.git
cd db-datadir-switcher
```

2. Configure your database paths in the configuration files:
   - Edit `my-internal.ini` for internal storage configuration
   - Edit `my-external.ini` for external storage configuration

3. Update paths in `MariaDBTrayMonitor.ps1`:
```powershell
$CONFIG_PATH = "C:\AredahScripts\MariaDBSwitcher"  # Your installation path
$EXTERNAL_DRIVE = "Z:"  # Your external drive letter
$MARIADB_BIN = "C:\Program Files\MariaDB 11.8\bin"  # Your MariaDB installation
```

4. Run the application:
```bash
runSwitcher.bat
```

Or run directly with PowerShell:
```powershell
powershell.exe -ExecutionPolicy Bypass -File MariaDBTrayMonitor.ps1
```

## Configuration

### Configuration Files

#### my-internal.ini
Defines the database configuration for internal storage:
```ini
[mysqld]
datadir=C:/Program Files/MariaDB 11.8/data
port=3306
# Additional MariaDB/MySQL settings...
```

#### my-external.ini
Defines the database configuration for external storage:
```ini
[mysqld]
datadir=Z:/MariaDB/data
port=3306
# Additional MariaDB/MySQL settings...
```

### Important Settings

| Setting | Description | Default |
|---------|-------------|---------|
| `datadir` | Database data directory path | Varies by configuration |
| `port` | Database server port | 3306 |
| `innodb_buffer_pool_size` | InnoDB buffer pool size | 8122M |

## Usage

### System Tray Operations

- **Double-click**: Open configuration switcher dialog
- **Right-click**: Access context menu
  - Show Status: Display current database status
  - Switch Configuration: Toggle between internal/external storage
  - Stop MariaDB: Safely stop the database service
  - Refresh: Update status display
  - Exit: Close the application

### Status Indicators

- **Green/Info Icon**: Database running
- **Yellow/Warning Icon**: Database stopped
- **Tooltip**: Shows current configuration (Internal/External)

## Architecture

```
db-datadir-switcher/
├── MariaDBTrayMonitor.ps1    # Main application logic (Windows)
├── my-internal.ini            # Internal storage configuration
├── my-external.ini            # External storage configuration
├── runSwitcher.bat           # Windows launcher
└── README.md                 # Documentation
```

## Safety Features

- Pre-switch validation of data directories
- Automatic detection of missing critical files
- Graceful shutdown before configuration switches
- Configuration file integrity checks
- Process state verification

## Troubleshooting

### Database Won't Start After Switch

1. Verify the data directory exists and contains valid database files
2. Check that the `mysql` system database exists in the target directory
3. Ensure proper permissions on the data directory
4. Review MariaDB/MySQL error logs

### External Drive Not Detected

1. Confirm the external drive is mounted to the configured drive letter
2. Verify the data directory path exists on the external drive
3. Check that database files are present in the external location

### Configuration Not Switching

1. Ensure MariaDB/MySQL process has fully stopped before switching
2. Verify both configuration files are properly formatted
3. Check for file permission issues

## Roadmap

- [ ] macOS support with menu bar integration
- [ ] Linux support with system tray/app indicator
- [ ] Support for PostgreSQL
- [ ] Support for MongoDB
- [ ] Configuration wizard/GUI
- [ ] Automatic backup before switch
- [ ] Multiple configuration profiles (>2)
- [ ] Docker container support
- [ ] Cloud storage integration

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request. For major changes, please open an issue first to discuss what you would like to change.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'feat: add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Built for database administrators and developers who need flexible data storage management
- Inspired by the need for seamless development/production data switching
- Special thanks to the MariaDB and MySQL communities

## Support

For issues, questions, or suggestions, please open an issue on GitHub.

---

**Note**: This tool modifies database configuration and manages database processes. Always ensure you have proper backups before switching configurations.