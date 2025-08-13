package gui

import (
	"fmt"
	"time"

	"github.com/getlantern/systray"
	"mariadb-monitor/core"
)

// CreateSystemTray creates and runs the system tray
func CreateSystemTray() {
	if SystrayRunning {
		core.AppLogger.Log("System tray already running, skipping creation")
		return
	}

	SystrayRunning = true
	core.AppLogger.Log("Starting system tray")
	systray.Run(onTrayReady, onTrayExit)
}

// onTrayReady initializes the system tray when it's ready
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
	for _, cfg := range core.AvailableConfigs {
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
		ticker := time.NewTicker(5 * time.Second)
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
				// Show main window
				core.AppLogger.Log("Show main window clicked")
				if MainWindow != nil {
					MainWindow.Show()
					MainWindow.RequestFocus()
				} else {
					// If no main window, create one
					Run()
				}

			case <-mStatus.ClickedCh:
				// Show status dialog
				core.AppLogger.Log("Show status clicked")
				if MainWindow == nil {
					Run()
					time.Sleep(500 * time.Millisecond) // Give window time to initialize
				}
				ShowStatusDialog()

			case <-mStop.ClickedCh:
				// Stop MariaDB
				core.AppLogger.Log("Stop MariaDB clicked from tray")
				go func() {
					creds := core.GetDefaultCredentials()
					err := core.StopMySQLWithCredentials(creds)
					if err != nil {
						core.AppLogger.Log("Failed to stop MariaDB: %v", err)
					} else {
						core.AppLogger.Log("MariaDB stopped successfully from tray")
					}
				}()

			case <-mSettings.ClickedCh:
				// Open settings
				core.AppLogger.Log("Settings clicked from tray")
				ShowSettings()

			case <-mLogs.ClickedCh:
				// Show logs
				core.AppLogger.Log("View logs clicked from tray")
				ShowLogs()

			case <-mOpenFolder.ClickedCh:
				// Open config folder
				core.AppLogger.Log("Open config folder clicked from tray")
				OpenFolder(core.AppConfig.ConfigPath)

			case <-mAbout.ClickedCh:
				// Show about dialog
				core.AppLogger.Log("About clicked from tray")
				if MainWindow == nil {
					Run()
					time.Sleep(500 * time.Millisecond) // Give window time to initialize
				}
				ShowAbout()

			case <-mExit.ClickedCh:
				// Exit application
				core.AppLogger.Log("Exit clicked from tray")
				core.AppLogger.Log("Application exiting")
				core.AppLogger.Close()
				systray.Quit()

			case <-time.After(100 * time.Millisecond):
				// Check config submenu items
				for i, subMenu := range configSubMenus {
					select {
					case <-subMenu.ClickedCh:
						if i < len(core.AvailableConfigs) {
							cfg := core.AvailableConfigs[i]
							core.AppLogger.Log("Starting MariaDB with config: %s", cfg.Name)
							go func(config core.MariaDBConfig) {
								err := core.StartMariaDBWithConfig(config.Path)
								if err != nil {
									core.AppLogger.Log("Failed to start %s: %v", config.Name, err)
								} else {
									core.AppLogger.Log("Successfully started %s", config.Name)
								}
							}(cfg)
						}
					default:
						// No click on this submenu
					}
				}
			}
		}
	}()
}

// onTrayExit handles cleanup when the system tray exits
func onTrayExit() {
	core.AppLogger.Log("System tray exiting")
	SystrayRunning = false
}

// updateTrayIcon updates the tray icon and tooltip based on MariaDB status
func updateTrayIcon() {
	status := core.GetMariaDBStatus()
	
	if status.IsRunning {
		systray.SetTitle("DBSwitcher ✓")
		// Sanitize tooltip text to prevent systray errors
		configName := status.ConfigName
		port := status.Port
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

