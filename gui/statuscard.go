package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"mariadb-monitor/core"
)

// CreateStatusCard creates the MariaDB status display card
func CreateStatusCard() *widget.Card {
	statusLabel := widget.NewLabel("Checking...")
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}

	versionLabel := widget.NewLabel("Version: Unknown")
	configLabel := widget.NewLabel("Config: None")
	portLabel := widget.NewLabel("Port: -")
	pidLabel := widget.NewLabel("PID: -")
	dataLabel := widget.NewLabel("Data: -")

	card := widget.NewCard("MariaDB Status", "", container.NewVBox(
		statusLabel,
		widget.NewSeparator(),
		container.NewGridWithColumns(2,
			versionLabel,
			configLabel,
			portLabel,
			pidLabel,
		),
		dataLabel,
	))

	// Store references for updates
	card.Content.(*fyne.Container).Objects[0] = statusLabel
	infoContainer := card.Content.(*fyne.Container).Objects[2].(*fyne.Container)
	infoContainer.Objects[0] = versionLabel
	infoContainer.Objects[1] = configLabel
	infoContainer.Objects[2] = portLabel
	infoContainer.Objects[3] = pidLabel
	card.Content.(*fyne.Container).Objects[3] = dataLabel

	return card
}

// UpdateStatusCard updates the status card with current MariaDB status
func UpdateStatusCard(card *widget.Card) {
	// Ensure this runs on the main UI thread
	fyne.Do(func() {
		content := card.Content.(*fyne.Container)
		statusLabel := content.Objects[0].(*widget.Label)
		infoContainer := content.Objects[2].(*fyne.Container)
		versionLabel := infoContainer.Objects[0].(*widget.Label)
		configLabel := infoContainer.Objects[1].(*widget.Label)
		portLabel := infoContainer.Objects[2].(*widget.Label)
		pidLabel := infoContainer.Objects[3].(*widget.Label)
		dataLabel := content.Objects[3].(*widget.Label)

		if core.CurrentStatus.IsRunning {
			statusLabel.SetText("✅ MariaDB is Running")
			versionLabel.SetText(fmt.Sprintf("Version: %s", core.CurrentStatus.Version))
			configLabel.SetText(fmt.Sprintf("Config: %s", core.CurrentStatus.ConfigName))
			portLabel.SetText(fmt.Sprintf("Port: %s", core.CurrentStatus.Port))
			pidLabel.SetText(fmt.Sprintf("PID: %d", core.CurrentStatus.ProcessID))
			dataLabel.SetText(fmt.Sprintf("Data: %s", core.CurrentStatus.DataPath))
		} else {
			statusLabel.SetText("🔴 MariaDB is Stopped")
			versionLabel.SetText("Version: -")
			configLabel.SetText("Config: -")
			portLabel.SetText("Port: -")
			pidLabel.SetText("PID: -")
			dataLabel.SetText("Data: -")
		}
	})
}

// FindStatusCard finds the status card in the UI (helper function)
func FindStatusCard() *widget.Card {
	if MainWindow == nil || MainWindow.Content() == nil {
		return nil
	}
	return StatusCardRef
}