package core

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// NotificationType represents different types of notifications
type NotificationType int

const (
	InfoNotification NotificationType = iota
	SuccessNotification
	WarningNotification
	ErrorNotification
)

// String returns the string representation of the notification type
func (nt NotificationType) String() string {
	switch nt {
	case InfoNotification:
		return "Info"
	case SuccessNotification:
		return "Success"
	case WarningNotification:
		return "Warning"
	case ErrorNotification:
		return "Error"
	default:
		return "Info"
	}
}

// ShowNotification displays a cross-platform system notification
func ShowNotification(title, message string, notificationType NotificationType) {
	if !AppConfig.NotificationsEnabled {
		AppLogger.Debug("Notifications disabled, skipping: %s - %s", title, message)
		return
	}

	AppLogger.Debug("Showing %s notification: %s - %s", notificationType.String(), title, message)

	switch runtime.GOOS {
	case "windows":
		showWindowsNotification(title, message, notificationType)
	case "darwin":
		showMacNotification(title, message, notificationType)
	case "linux":
		showLinuxNotification(title, message, notificationType)
	default:
		AppLogger.Warn("Notifications not supported on platform: %s", runtime.GOOS)
	}
}

// showWindowsNotification displays a notification on Windows using PowerShell
func showWindowsNotification(title, message string, notificationType NotificationType) {
	// Use PowerShell with Windows.UI.Notifications for modern toast notifications
	script := fmt.Sprintf(`
		[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
		[Windows.UI.Notifications.ToastNotification, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
		[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null

		$xml = @"
		<toast>
			<visual>
				<binding template="ToastGeneric">
					<text>%s</text>
					<text>%s</text>
				</binding>
			</visual>
		</toast>
"@

		$XmlDocument = [Windows.Data.Xml.Dom.XmlDocument]::new()
		$XmlDocument.loadXml($xml)
		$AppId = "DBSwitcher"
		try {
			[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($AppId).Show($XmlDocument)
		} catch {
			# Fallback to simple notification
			Add-Type -AssemblyName System.Windows.Forms
			[System.Windows.Forms.MessageBox]::Show("%s", "%s", "OK", "Information")
		}
	`, title, message, message, title)

	cmd := exec.Command("powershell", "-WindowStyle", "Hidden", "-Command", script)
	if err := cmd.Run(); err != nil {
		AppLogger.Debug("PowerShell notification failed: %v", err)
		// Simple fallback
		showWindowsFallbackNotification(title, message)
	}
}

// showWindowsFallbackNotification shows a simple Windows notification
func showWindowsFallbackNotification(title, message string) {
	script := fmt.Sprintf(`msg * "%s: %s"`, title, message)
	cmd := exec.Command("cmd", "/c", script)
	cmd.Run()
}

// showMacNotification displays a notification on macOS using osascript
func showMacNotification(title, message string, notificationType NotificationType) {
	script := fmt.Sprintf(`display notification "%s" with title "%s"`, 
		strings.ReplaceAll(message, `"`, `\"`), 
		strings.ReplaceAll(title, `"`, `\"`))
	
	cmd := exec.Command("osascript", "-e", script)
	if err := cmd.Run(); err != nil {
		AppLogger.Debug("macOS notification failed: %v", err)
	}
}

// showLinuxNotification displays a notification on Linux using notify-send
func showLinuxNotification(title, message string, notificationType NotificationType) {
	// Try notify-send first (most common)
	icon := getLinuxIcon(notificationType)
	cmd := exec.Command("notify-send", "-i", icon, title, message)
	if err := cmd.Run(); err != nil {
		AppLogger.Debug("notify-send failed: %v", err)
		// Try alternative methods
		showLinuxFallbackNotification(title, message)
	}
}

// showLinuxFallbackNotification tries alternative Linux notification methods
func showLinuxFallbackNotification(title, message string) {
	// Try zenity
	cmd := exec.Command("zenity", "--info", "--text="+title+": "+message)
	if err := cmd.Run(); err != nil {
		AppLogger.Debug("zenity notification failed: %v", err)
		// Try kdialog (KDE)
		cmd = exec.Command("kdialog", "--passivepopup", title+": "+message, "5")
		if err := cmd.Run(); err != nil {
			AppLogger.Debug("kdialog notification failed: %v", err)
		}
	}
}

// getLinuxIcon returns the appropriate icon name for Linux notifications
func getLinuxIcon(notificationType NotificationType) string {
	switch notificationType {
	case SuccessNotification:
		return "dialog-information"
	case WarningNotification:
		return "dialog-warning"
	case ErrorNotification:
		return "dialog-error"
	default:
		return "dialog-information"
	}
}

// NotifyMariaDBStarted shows a notification when MariaDB starts successfully
func NotifyMariaDBStarted(configName string) {
	ShowNotification("MariaDB Started", 
		fmt.Sprintf("MariaDB started successfully with configuration '%s'", configName), 
		SuccessNotification)
}

// NotifyMariaDBStopped shows a notification when MariaDB stops
func NotifyMariaDBStopped() {
	ShowNotification("MariaDB Stopped", 
		"MariaDB has been stopped", 
		InfoNotification)
}

// NotifyMariaDBError shows a notification when MariaDB encounters an error
func NotifyMariaDBError(message string) {
	ShowNotification("MariaDB Error", 
		message, 
		ErrorNotification)
}

// NotifyConfigurationSwitched shows a notification when configuration is switched
func NotifyConfigurationSwitched(configName string) {
	ShowNotification("Configuration Switched", 
		fmt.Sprintf("Switched to configuration '%s'", configName), 
		InfoNotification)
}