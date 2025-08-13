package gui

import (
	"os/exec"
	"runtime"

	"fyne.io/fyne/v2"
	"mariadb-monitor/core"
)

// CreateMainMenu creates the main application menu
func CreateMainMenu() *fyne.MainMenu {
	return fyne.NewMainMenu(
		fyne.NewMenu("File",
			fyne.NewMenuItem("Open Config Folder", func() {
				OpenFolder(core.AppConfig.ConfigPath)
			}),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Settings", func() {
				ShowSettings()
			}),
			fyne.NewMenuItem("Exit", func() {
				FyneApp.Quit()
			}),
		),
		fyne.NewMenu("View",
			fyne.NewMenuItem("Refresh", func() {
				RefreshConfigurations()
				RefreshMainUI()
			}),
			fyne.NewMenuItem("Logs", func() {
				ShowLogs()
			}),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Appearance", func() {
				ShowAppearanceSettings()
			}),
		),
		fyne.NewMenu("Tools",
			CreateCredentialsMenu(),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Run in System Tray", func() {
				MainWindow.Hide()
				if !SystrayRunning {
					go CreateSystemTray()
				}
			}),
		),
		fyne.NewMenu("Help",
			fyne.NewMenuItem("About", func() {
				ShowAbout()
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

// ShowMainWindow displays the main application window
func ShowMainWindow() {
	if MainWindow != nil {
		MainWindow.Show()
		MainWindow.RequestFocus()
	} else {
		// If window doesn't exist, start the GUI
		core.AppLogger.Log("Main window not available, starting GUI")
		Run()
	}
}

// OpenFolder opens a folder in the system file explorer
func OpenFolder(path string) {
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