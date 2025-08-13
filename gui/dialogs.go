package gui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"mariadb-monitor/core"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/cmd/fyne_settings/settings"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// ShowCredentialsDialog shows the MySQL credentials dialog
func ShowCredentialsDialog(parent fyne.Window, onSuccess func(core.MySQLCredentials), onCancel func()) {
	// Create form fields
	usernameEntry := widget.NewEntry()
	usernameEntry.SetPlaceHolder("root")
	if core.SavedCredentials != nil && core.SavedCredentials.Username != "" {
		usernameEntry.SetText(core.SavedCredentials.Username)
	} else {
		usernameEntry.SetText("root") // Default suggestion
	}
	
	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("Enter password (leave empty if none)")
	if core.SavedCredentials != nil {
		passwordEntry.SetText(core.SavedCredentials.Password)
	}
	
	hostEntry := widget.NewEntry()
	hostEntry.SetPlaceHolder("localhost")
	if core.SavedCredentials != nil && core.SavedCredentials.Host != "" {
		hostEntry.SetText(core.SavedCredentials.Host)
	} else {
		hostEntry.SetText("localhost")
	}
	
	portEntry := widget.NewEntry()
	portEntry.SetPlaceHolder("3306")
	if core.SavedCredentials != nil && core.SavedCredentials.Port != "" {
		portEntry.SetText(core.SavedCredentials.Port)
	} else {
		portEntry.SetText("3306")
	}
	
	// Remember credentials checkbox options
	rememberSessionCheck := widget.NewCheck("Remember for this session", nil)
	rememberSessionCheck.SetChecked(true)
	
	rememberPermanentCheck := widget.NewCheck("Save credentials permanently (secure storage)", nil)
	rememberPermanentCheck.SetChecked(core.SavedCredentials != nil)
	
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
				creds := core.MySQLCredentials{
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
					core.SavedCredentials = &creds
				}
				
				// Save credentials permanently if requested
				if rememberPermanentCheck.Checked {
					if err := core.SaveCredentialsToKeyring(creds); err != nil {
						core.AppLogger.Log("Failed to save credentials to keyring: %v", err)
						dialog.ShowError(fmt.Errorf("Failed to save credentials: %v", err), parent)
					} else {
						core.SavedCredentials = &creds
					}
				} else if !rememberPermanentCheck.Checked && core.SavedCredentials != nil {
					// If unchecked, remove saved credentials
					if err := core.DeleteCredentialsFromKeyring(); err != nil {
						core.AppLogger.Log("Failed to delete credentials from keyring: %v", err)
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

// ShowSettings shows the settings dialog (stub for now)
func ShowSettings() {
	fyne.Do(func() {
		// Create settings window
		settingsWindow := FyneApp.NewWindow("Settings")
		settingsWindow.Resize(fyne.NewSize(500, 400))
		
		content := widget.NewCard("Settings", "Not yet implemented",
			widget.NewLabel("Settings functionality will be implemented in a future update."))
		
		settingsWindow.SetContent(container.NewVBox(content))
		settingsWindow.Show()
	})
}

// ShowAppearanceSettings shows Fyne appearance settings
func ShowAppearanceSettings() {
	fyne.Do(func() {
		if FyneApp == nil {
			FyneApp = app.NewWithID("mariadb-switcher")
		}
		
		// Create appearance settings window
		settingsWindow := FyneApp.NewWindow("Appearance Settings")
		settingsWindow.Resize(fyne.NewSize(400, 300))
		
		// Create settings content using Fyne's built-in settings
		settingsContent := settings.NewSettings().LoadAppearanceScreen(settingsWindow)
		settingsWindow.SetContent(settingsContent)
		settingsWindow.Show()
	})
}

// ShowLogs shows the application logs
func ShowLogs() {
	fyne.Do(func() {
		logWindow := FyneApp.NewWindow("Application Logs")
		logWindow.Resize(fyne.NewSize(800, 600))
		
		// Read log file
		logPath := filepath.Join(core.GetAppDataDir(), "dbswitcher.log")
		logContent := "No logs available"
		
		if data, err := os.ReadFile(logPath); err == nil {
			logContent = string(data)
			// Show only last 1000 lines to avoid overwhelming the UI
			lines := strings.Split(logContent, "\n")
			if len(lines) > 1000 {
				lines = lines[len(lines)-1000:]
				logContent = strings.Join(lines, "\n")
			}
		}
		
		logText := widget.NewEntry()
		logText.MultiLine = true
		logText.Wrapping = fyne.TextWrapWord
		logText.SetText(logContent)
		logText.Disable() // Make it read-only
		
		scrollable := container.NewScroll(logText)
		
		// Buttons
		refreshBtn := widget.NewButton("Refresh", func() {
			if data, err := os.ReadFile(logPath); err == nil {
				content := string(data)
				lines := strings.Split(content, "\n")
				if len(lines) > 1000 {
					lines = lines[len(lines)-1000:]
					content = strings.Join(lines, "\n")
				}
				logText.SetText(content)
			}
		})
		
		openLogFolderBtn := widget.NewButton("Open Log Folder", func() {
			OpenFolder(core.GetAppDataDir())
		})
		
		clearBtn := widget.NewButton("Clear Logs", func() {
			dialog.ShowConfirm("Clear Logs", "Are you sure you want to clear all logs?", func(confirmed bool) {
				if confirmed {
					if err := os.WriteFile(logPath, []byte(""), 0644); err == nil {
						logText.SetText("Logs cleared")
					}
				}
			}, logWindow)
		})
		
		toolbar := container.NewHBox(refreshBtn, openLogFolderBtn, clearBtn)
		
		content := container.NewBorder(toolbar, nil, nil, nil, scrollable)
		logWindow.SetContent(content)
		logWindow.Show()
	})
}

// ShowAbout shows the about dialog
func ShowAbout() {
	aboutContent := fmt.Sprintf(`DBSwitcher - MariaDB Configuration Manager
Version: 0.0.1

A tool for managing multiple MariaDB configurations and switching between them easily.

Features:
• Multiple configuration management
• Quick switching between databases
• System tray integration
• Command-line interface
• Secure credential storage

Configuration Directory:
%s

Log Directory:
%s

© 2025 DBSwitcher Project`, core.AppConfig.ConfigPath, core.GetAppDataDir())

	dialog.ShowInformation("About DBSwitcher", aboutContent, MainWindow)
}

// ShowStatusDialog shows a detailed status dialog
func ShowStatusDialog() {
	fyne.Do(func() {
		statusWindow := FyneApp.NewWindow("MariaDB Status Details")
		statusWindow.Resize(fyne.NewSize(500, 400))
		
		status := core.GetMariaDBStatus()
		
		var statusText string
		if status.IsRunning {
			statusText = fmt.Sprintf(`MariaDB Status: RUNNING ✅

Process ID: %d
Configuration: %s
Port: %s
Data Directory: %s
Config File: %s
Version: %s

MariaDB is currently running and accepting connections.`,
				status.ProcessID,
				status.ConfigName,
				status.Port,
				status.DataPath,
				status.ConfigFile,
				status.Version)
		} else {
			statusText = `MariaDB Status: STOPPED ❌

MariaDB is not currently running.
Use the Start button to launch MariaDB with a configuration.`
		}
		
		statusLabel := widget.NewLabel(statusText)
		statusLabel.Wrapping = fyne.TextWrapWord
		
		refreshBtn := widget.NewButton("Refresh", func() {
			newStatus := core.GetMariaDBStatus()
			var newText string
			if newStatus.IsRunning {
				newText = fmt.Sprintf(`MariaDB Status: RUNNING ✅

Process ID: %d
Configuration: %s
Port: %s
Data Directory: %s
Config File: %s
Version: %s

MariaDB is currently running and accepting connections.`,
					newStatus.ProcessID,
					newStatus.ConfigName,
					newStatus.Port,
					newStatus.DataPath,
					newStatus.ConfigFile,
					newStatus.Version)
			} else {
				newText = `MariaDB Status: STOPPED ❌

MariaDB is not currently running.
Use the Start button to launch MariaDB with a configuration.`
			}
			statusLabel.SetText(newText)
			core.CurrentStatus = newStatus
		})
		
		closeBtn := widget.NewButton("Close", func() {
			statusWindow.Close()
		})
		
		buttonContainer := container.NewHBox(refreshBtn, closeBtn)
		
		content := container.NewBorder(nil, buttonContainer, nil, nil,
			container.NewScroll(statusLabel))
		
		statusWindow.SetContent(content)
		statusWindow.Show()
	})
}

// OpenFileInEditor opens a file in the default system editor
func OpenFileInEditor(path string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("notepad", path)
	case "darwin":
		cmd = exec.Command("open", "-t", path)
	default:
		// Try common Linux editors
		editors := []string{"gedit", "kate", "xed", "nano", "vim"}
		for _, editor := range editors {
			if _, err := exec.LookPath(editor); err == nil {
				cmd = exec.Command(editor, path)
				break
			}
		}
		if cmd == nil {
			cmd = exec.Command("xdg-open", path)
		}
	}

	if cmd != nil {
		cmd.Start()
	}
}

// DeleteConfigFile deletes a configuration file
func DeleteConfigFile(path string) error {
	return os.Remove(path)
}

// openFolderCrossPlatform opens a folder in the default file manager
func openFolderCrossPlatform(path string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		// Linux and other Unix systems
		cmd = exec.Command("xdg-open", path)
	}

	return cmd.Start()
}