package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"mariadb-monitor/core"
)

// CreateCredentialsMenu creates the credentials management menu
func CreateCredentialsMenu() *fyne.MenuItem {
	return fyne.NewMenuItem("Credentials", func() {
		ShowCredentialsDialog(MainWindow, func(creds core.MySQLCredentials) {
			// Test the connection
			if err := core.TestMySQLConnection(creds); err != nil {
				dialog.ShowError(err, MainWindow)
			} else {
				dialog.ShowInformation("Connection Test", "Successfully connected to MariaDB!", MainWindow)
			}
		}, func() {
			// User cancelled
		})
	})
}