package gui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"mariadb-monitor/core"
)

// Global GUI variables
var (
	FyneApp             fyne.App
	MainWindow          fyne.Window
	StatusCardRef       *widget.Card
	GlobalConfigSelect  *widget.Select  // Global reference to dropdown in Quick Actions
	GlobalConfigList    *widget.List    // Global reference to list in Configurations tab
)

// Run starts the GUI application
func Run() error {
	// Initialize the Fyne app
	FyneApp = app.NewWithID("mariadb-switcher")
	FyneApp.SetIcon(nil)
	MainWindow = FyneApp.NewWindow("DBSwitcher - MariaDB Configuration Manager")
	MainWindow.Resize(fyne.NewSize(1000, 700))

	// Create main interface components
	StatusCardRef = CreateStatusCard()
	configCard := CreateConfigCard()
	quickActionsCard := CreateQuickActionsCard()

	// Auto-refresh ticker with proper UI update
	go func() {
		ticker := time.NewTicker(5 * time.Second) // Reduced to 5 seconds for faster updates
		defer ticker.Stop()
		for range ticker.C {
			oldStatus := core.CurrentStatus
			core.CurrentStatus = core.GetMariaDBStatus()
			
			// Only update UI if status changed
			if oldStatus.IsRunning != core.CurrentStatus.IsRunning || 
				oldStatus.ProcessID != core.CurrentStatus.ProcessID ||
				oldStatus.ConfigName != core.CurrentStatus.ConfigName {
				// UpdateStatusCard already calls fyne.Do() internally
				UpdateStatusCard(StatusCardRef)
			}
		}
	}()

	// Main content with tabs
	tabs := container.NewAppTabs(
		container.NewTabItem("Dashboard", container.NewVBox(
			StatusCardRef,
			quickActionsCard,
		)),
		container.NewTabItem("Configurations", configCard),
	)

	// Create menu
	MainWindow.SetMainMenu(CreateMainMenu())

	// Set main content
	MainWindow.SetContent(tabs)

	// Initial status update with UI support for credentials
	GetMariaDBStatusWithUI(MainWindow, func(status core.MariaDBStatus) {
		core.CurrentStatus = status
		UpdateStatusCard(StatusCardRef)
	})

	// Set up close handler to exit application
	MainWindow.SetCloseIntercept(func() {
		core.AppLogger.Log("Main window closing - shutting down application")
		core.AppLogger.Close()
		FyneApp.Quit()
	})

	// Show and run the main window
	MainWindow.ShowAndRun()
	return nil
}

// RunTray starts the application in system tray mode
func RunTray() error {
	CreateSystemTray()
	return nil
}