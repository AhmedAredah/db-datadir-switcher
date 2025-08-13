# MariaDB Switcher Refactoring Plan

## Current Structure Analysis
- **File**: main.go
- **Lines**: 3290
- **Functions**: 86
- **Current Architecture**: Monolithic with mixed GUI and core logic

## Target Architecture
```
MariaDBSwitcher/
├── main.go                  # Entry point (~150 lines)
├── core/                    # Business logic (no GUI dependencies)
│   ├── types.go            # Data structures
│   ├── config.go           # Configuration management
│   ├── mariadb.go          # MariaDB operations
│   ├── credentials.go      # Credential management
│   ├── logger.go           # Logging
│   └── utils.go            # Utilities
├── gui/                     # GUI components (Fyne)
│   ├── app.go              # GUI initialization
│   ├── mainwindow.go       # Main window
│   ├── statuscard.go       # Status display
│   ├── quickactions.go     # Quick actions card
│   ├── configcard.go       # Configuration tab
│   ├── settings.go         # Settings dialogs
│   ├── systray.go          # System tray
│   └── dialogs.go          # Various dialogs
└── cli/                     # Command-line interface
    └── commands.go         # CLI commands
```

## Function Distribution Map

### Core Package Functions (No GUI Dependencies)
#### types.go (Lines 33-72)
- Config struct
- MariaDBConfig struct
- MariaDBStatus struct
- MySQLCredentials struct

#### logger.go (Lines 88-113)
- Logger struct
- NewLogger()
- Log()
- Close()

#### config.go (Lines 393-596)
- loadConfig()
- saveConfig()
- autoDetectConfig()
- scanForConfigs()
- parseConfigFile()
- ensureConfigDirectory()
- getConfigPath()
- getAppDataDir()
- getUserConfigDir()

#### mariadb.go (Lines 809-1638, 2211-2263)
- getMariaDBStatus()
- startMariaDBWithConfig()
- stopMySQLWithCredentials()
- isMariaDBRunning()
- findProcessWithCmdLine()
- validateConfigFile()
- validateDataDirectory()
- initializeDataDir()

#### credentials.go (Lines 138-323)
- saveCredentialsToKeyring()
- loadCredentialsFromKeyring()
- deleteCredentialsFromKeyring()
- testMySQLConnection()
- validateCredentials()

#### utils.go (Various)
- pathExists()
- getExecutableName()
- isDirEmpty()
- isPortAvailable()
- isPortListening()

### GUI Package Functions
#### app.go
- Main GUI initialization
- Global GUI variables

#### mainwindow.go (Lines 2047-2120, 2738-2793)
- showMainWindow()
- createMainMenu()

#### statuscard.go (Lines 2794-2854)
- createStatusCard()
- updateStatusCard()

#### quickactions.go (Lines 2855-3072)
- createQuickActionsCard()
- refreshConfigurations()
- refreshMainUI()

#### configcard.go (Lines 3091-3290)
- createConfigCard()

#### dialogs.go (Lines 189-290, 2121-2559)
- showCredentialsDialog()
- showStatusDialog()
- showSettings()
- showLogs()
- showAbout()

#### systray.go (Lines 1914-2046)
- createSystemTray()
- onTrayReady()
- onTrayExit()
- updateTrayIcon()

### CLI Package (New Implementation)
- List() - List configurations
- Switch() - Switch configuration
- Status() - Show status
- Stop() - Stop MariaDB
- Start() - Start configuration

## Execution Timeline

### Phase 1: Core Package Creation
1. Create core directory structure
2. Move types and structures
3. Move logger implementation
4. Move configuration management
5. Move MariaDB operations
6. Move credential management
7. Move utilities

### Phase 2: GUI Package Creation
1. Create gui directory structure
2. Move window management
3. Move UI cards
4. Move dialogs
5. Move system tray
6. Update imports and references

### Phase 3: CLI Implementation
1. Create cli directory
2. Implement command structure
3. Implement individual commands
4. Add CLI credential handling

### Phase 4: Main Entry Point
1. Simplify main.go
2. Implement command routing
3. Test all interfaces

## Key Refactoring Challenges

### 1. UI/Non-UI Function Variants
Functions with both versions:
- getMariaDBStatus() vs getMariaDBStatusWithUI()
- stopMariaDBService() vs stopMariaDBServiceWithUI()

Solution: Use callback interfaces

### 2. Global Variables
Current globals need redistribution:
- Core: config, currentStatus, availableConfigs
- GUI: fyneApp, mainWindow, UI widgets

### 3. Circular Dependencies
Potential issues:
- GUI needs core
- Core callbacks might reference GUI

Solution: Interface abstractions

## Success Criteria
- [ ] All tests pass
- [ ] GUI works as before
- [ ] CLI fully functional
- [ ] No import cycles
- [ ] Clean separation of concerns
- [ ] Improved testability

## Notes
- Each stage will be tested before proceeding
- Git commits after each successful stage
- Documentation updates as needed