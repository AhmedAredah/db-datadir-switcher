package gui

import (
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

// Run starts the GUI application with default settings
func Run() error {
	return RunWithOptions(false)
}

// RunWithOptions starts the GUI application with specific options
func RunWithOptions(startMinimized bool) error {
	// Initialize the Fyne app
	FyneApp = app.NewWithID("mariadb-switcher")
	FyneApp.SetIcon(nil)
	MainWindow = FyneApp.NewWindow("DBSwitcher - MariaDB Configuration Manager")
	MainWindow.Resize(fyne.NewSize(1000, 700))

	// Create main interface components
	StatusCardRef = CreateStatusCard()
	configCard := CreateConfigCard()
	quickActionsCard := CreateQuickActionsCard()

	// Auto-refresh will be handled by StartAutoRefresh() based on user settings

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

	// Start auto-refresh
	StartAutoRefresh()
	
	if startMinimized {
		core.AppLogger.Info("Starting application minimized to system tray")
		// Create system tray but don't show main window
		CreateSystemTray()
		FyneApp.Run() // Run without showing window
	} else {
		// Show and run the main window normally
		MainWindow.ShowAndRun()
	}
	
	return nil
}

// RunTray starts the application in system tray mode
func RunTray() error {
	// Initialize the Fyne app and create window (but don't show it)
	// This ensures all GUI components are available when needed from tray
	FyneApp = app.NewWithID("mariadb-switcher")
	FyneApp.SetIcon(nil)
	
	// Create the main window but keep it hidden
	MainWindow = FyneApp.NewWindow("DBSwitcher - MariaDB Configuration Manager")
	MainWindow.Resize(fyne.NewSize(1000, 700))
	
	// Initialize components (needed for when window is shown later)
	StatusCardRef = CreateStatusCard()
	configCard := CreateConfigCard()
	quickActionsCard := CreateQuickActionsCard()
	
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
	
	// Set up close handler to hide instead of quit when in tray mode
	MainWindow.SetCloseIntercept(func() {
		core.AppLogger.Log("Main window closing - hiding to tray")
		MainWindow.Hide()
	})
	
	// Start auto-refresh
	StartAutoRefresh()
	
	// Create system tray (this starts its own event loop)
	CreateSystemTray()
	
	// Note: systray.Run() blocks, so this won't return until systray.Quit() is called
	return nil
}